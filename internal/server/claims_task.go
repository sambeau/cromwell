package server

// The task claimable (SPEC-020 FR-2.10): what claiming, submitting and
// releasing a task mean. A task is claimed in its feature's one working copy,
// and moves along the path a dispatched implementer's start and completion
// already move it (DEC-004 as read by DEC-007 decision 3).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/config"
	"subutai/internal/lifecycle"
	"subutai/internal/rules"
	"subutai/internal/store"
)

type taskClaims struct{ s *Server }

// taskSuffix is the task's own part of its ID: "T03" of "FEAT-023-T03".
func taskSuffix(public string) string {
	if i := strings.LastIndex(public, "-"); i >= 0 {
		return public[i+1:]
	}
	return public
}

// taskNumber is how a task is named inside a commit message: its plan's id,
// as the implementer's completion names it.
func taskNumber(t *store.Task) string {
	if t.LocalID != "" {
		return t.LocalID
	}
	return taskSuffix(t.PublicID)
}

func (tc taskClaims) load(ctx context.Context, q store.Querier, refID uuid.UUID) (*claimTarget, error) {
	task, err := store.GetTask(ctx, q, refID)
	if err != nil {
		return nil, err
	}
	f, err := store.GetFeature(ctx, q, task.FeatureID)
	if err != nil {
		return nil, err
	}
	t := &claimTarget{RefType: "task", RefID: refID, Label: task.PublicID, Title: task.Title, Feature: f, Task: task}
	wt, err := store.LiveWorktreeForFeature(ctx, q, f.ID)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if err == nil {
		t.Worktree = wt
		t.Path = tc.s.worktreeAbs(wt.Path)
	}
	return t, nil
}

// heldBySentence is FR-2.4's refusal when someone else holds the task.
func heldBySentence(label string, cur *store.Claim, who Claimant) string {
	holder := whoWords(cur.Kind, cur.Actor, "")
	if who.Kind == store.WriterChat {
		return fmt.Sprintf("%s is claimed by %s. Only a person can release a claim, in the web UI.", label, holder)
	}
	return fmt.Sprintf("%s is claimed by %s. Release the claim from the task's page if the work should go elsewhere.", label, holder)
}

const inReviewTail = "If the reviewer asks for changes, it comes back to whoever implemented it; get_feature shows the comments."

func (tc taskClaims) refusal(ctx context.Context, tx pgx.Tx, t *claimTarget, who Claimant, resume bool) string {
	s := tc.s
	task, f, label := t.Task, t.Feature, t.Label

	// Who holds it now.
	cur, err := store.CurrentClaimFor(ctx, tx, "task", t.RefID)
	switch {
	case err == nil && cur.State == lifecycle.ClaimSubmitted:
		return fmt.Sprintf("%s is in code review. %s", label, inReviewTail)
	case err == nil && !who.holds(cur):
		return heldBySentence(label, cur, who)
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return fmt.Sprintf("%s can't be claimed just now: %v.", label, err)
	}

	// The feature must be being built, and its spec not under revision.
	switch f.State {
	case lifecycle.FeatActive:
	case lifecycle.FeatReview:
		return fmt.Sprintf("%s is being verified, so its tasks can't be claimed now.", f.PublicID)
	case lifecycle.FeatDone, lifecycle.FeatAbandoned:
		return fmt.Sprintf("%s is %s, so its tasks can't be claimed.", f.PublicID, f.State)
	default:
		return fmt.Sprintf("%s isn't being built yet. A person starts building from the feature's page; you can't.", f.PublicID)
	}
	if !resume {
		if revised, _ := specBeingRevised(ctx, tx, f.ID); revised {
			return fmt.Sprintf("%s's %s is being revised. Claiming waits until a person decides whether building continues.", f.PublicID, specWord(f))
		}
		switch task.State {
		case lifecycle.TaskPending:
			return fmt.Sprintf("%s can't be claimed yet: it waits for %s to be done. Claim one of those, or wait.", label, tc.waitingFor(ctx, tx, task))
		case lifecycle.TaskDone:
			return fmt.Sprintf("%s is already done. Choose another task from get_feature.", label)
		case lifecycle.TaskAbandoned:
			return fmt.Sprintf("%s was abandoned. Choose another task from get_feature.", label)
		case lifecycle.TaskReview:
			return fmt.Sprintf("%s is in code review. %s", label, inReviewTail)
		}
	}

	// The working copy must exist, and be free.
	if t.Path == "" {
		return fmt.Sprintf("%s's working copy is missing. A person needs to retry creating it from the Inbox.", f.PublicID)
	}
	if _, err := os.Stat(t.Path); err != nil {
		return fmt.Sprintf("%s's working copy is missing. A person needs to retry creating it from the Inbox.", f.PublicID)
	}
	if running, err := tc.runningTask(ctx, tx, f.ID); err == nil && running != "" {
		return fmt.Sprintf("An agent is implementing %s in this feature's working copy. Claim a task when it has finished; get_feature shows when.", running)
	}
	if !resume {
		// An attempt the stall sweep gave up on may still be running tools.
		if cfg, err := s.freshConfig(); err == nil {
			window := time.Duration(cfg.Dispatch.StallSeconds) * time.Second
			if recent, err := store.RecentImplementActivity(ctx, tx, t.RefID, window); err == nil && recent {
				return fmt.Sprintf("An agent is implementing %s in this feature's working copy. Claim a task when it has finished; get_feature shows when.", label)
			}
		}
	}
	if open, err := store.OpenClaimForFeature(ctx, tx, f.ID); err == nil && open.RefID != t.RefID {
		other := open.RefType
		if ot, err := store.GetTask(ctx, tx, open.RefID); err == nil {
			other = ot.PublicID
		}
		return fmt.Sprintf("%s is claimed by %s, in this feature's working copy. Only one task in a feature can be worked at a time; wait for it to be submitted.",
			other, whoWords(open.Kind, open.Actor, ""))
	}
	return ""
}

// specWord is what a feature's contract is called: a bug's is its report.
func specWord(f *store.Feature) string {
	if f.IsBug() {
		return "report"
	}
	return "spec"
}

// waitingFor names the tasks a pending task waits on that aren't done.
func (tc taskClaims) waitingFor(ctx context.Context, q store.Querier, task *store.Task) string {
	var names []string
	for _, id := range task.DependsOn {
		if d, err := store.GetTask(ctx, q, id); err == nil && d.State != lifecycle.TaskDone {
			names = append(names, taskSuffix(d.PublicID))
		}
	}
	if len(names) == 0 {
		return "its earlier tasks"
	}
	return joinWords(names)
}

// runningTask names the task an implement dispatch is running on in the
// feature, or "".
func (tc taskClaims) runningTask(ctx context.Context, q store.Querier, featureID uuid.UUID) (string, error) {
	var public string
	err := q.QueryRow(ctx, `
		SELECT t.public_id FROM dispatches d JOIN tasks t ON t.id = d.ref_id AND d.ref_type = 'task'
		WHERE d.purpose = 'implement-task' AND d.state = 'running' AND t.feature_id = $1
		ORDER BY d.started_at LIMIT 1`, featureID).Scan(&public)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return public, err
}

func (tc taskClaims) onClaim(ctx context.Context, tx pgx.Tx, t *claimTarget, c *store.Claim) ([]uuid.UUID, error) {
	task, err := store.GetTask(ctx, tx, t.RefID)
	if err != nil {
		return nil, err
	}
	who := whoWords(c.Kind, c.Actor, "")
	switch task.State {
	case lifecycle.TaskReady:
		// The same move a dispatched implementer's start makes (FR-2.3 step 2).
		return nil, store.TransitionTask(ctx, tx, task, lifecycle.TaskClaim, c.Actor,
			map[string]any{"claim_id": c.ID.String(), "kind": c.Kind, "via": c.Via})
	case lifecycle.TaskActive:
		// Taking an active task nobody is running: its queued and failed
		// dispatches are cancelled, and its dispatch-failure question
		// withdrawn. This is the claim's own act, not anyone directing a
		// dispatch (SD-15).
		cancelled, err := store.CancelTaskImplementDispatches(ctx, tx, t.RefID, c.Actor, "claimed by "+who)
		if err != nil {
			return nil, err
		}
		withdrawn, err := store.WithdrawImplementFailure(ctx, tx, t.RefID, c.Actor, "claimed by "+who)
		if err != nil {
			return nil, err
		}
		ids := make([]string, len(cancelled))
		for i, id := range cancelled {
			ids[i] = id.String()
		}
		return cancelled, store.Audit(ctx, tx, c.Actor, "task.taken_by_claim", "task", &t.RefID,
			map[string]any{"claim_id": c.ID.String(), "kind": c.Kind, "via": c.Via,
				"cancelled_dispatches": ids, "withdrawn_checkpoints": withdrawn})
	}
	return nil, fmt.Errorf("task %s is %s and can't be claimed", t.Label, task.State)
}

func (tc taskClaims) prepareSubmit(ctx context.Context, t *claimTarget, c *store.Claim, summary string) (submitted, error) {
	s := tc.s
	cfg, err := s.freshConfig()
	if err != nil {
		return submitted{}, err
	}
	role, ok := cfg.Assignments["review-code"]
	if !ok {
		return submitted{}, errors.New("config.yaml assignments has no review-code role")
	}
	model, err := s.codeReviewModel(ctx, s.Store.Pool, cfg, t.RefID, role)
	if err != nil {
		return submitted{}, err
	}
	sub := submitted{Summary: summary, Model: model, Role: role}

	msg := fmt.Sprintf("subutai: %s — %s\n\n%s\n\nSubutai-Executor: %s\nSubutai-Claim: %s",
		taskNumber(t.Task), t.Title, summary, c.Kind, c.ID)
	// A claimant who committed anyway (SD-18) leaves nothing to commit; the
	// commit is theirs, so it isn't recorded as Subutai's.
	dirty, err := workingCopyDirty(t.Path)
	if err != nil {
		return submitted{}, err
	}
	if dirty {
		if err := s.commitWorktree(t.Path, msg); err != nil {
			return submitted{}, fmt.Errorf("committing the claimed work: %w", err)
		}
		sub.Committed = true
	}
	sub.Commit = headOf(t.Path)
	return sub, nil
}

func (tc taskClaims) onSubmit(ctx context.Context, tx pgx.Tx, t *claimTarget, c *store.Claim, sub submitted) (string, error) {
	task, err := store.GetTask(ctx, tx, t.RefID)
	if err != nil {
		return "", err
	}
	if err := store.TransitionTask(ctx, tx, task, lifecycle.TaskImplemented, c.Actor,
		map[string]any{"summary": sub.Summary, "claim_id": c.ID.String()}); err != nil {
		return "", err
	}
	key := rules.CodeReviewIdempotencyKey(t.RefID, sub.Commit)
	d, err := store.EnqueueDispatch(ctx, tx, "review-code", sub.Role, sub.Model, "task", t.RefID, key)
	if err != nil || d == nil {
		return "", err
	}
	return d.ID.String(), nil
}

func (tc taskClaims) prepareRelease(ctx context.Context, t *claimTarget, c *store.Claim) (released, error) {
	r := released{}
	if t.Path == "" {
		return r, nil
	}
	// Only an open claim holds the working copy; what is uncommitted in it is
	// that claim's work. A returned claim's work was committed at its submit,
	// and the copy may now be an agent's.
	if c.State == lifecycle.ClaimOpen {
		dirty, err := workingCopyDirty(t.Path)
		if err != nil {
			return r, err
		}
		if dirty {
			msg := fmt.Sprintf("subutai: %s — work left by %s, released\n\nSubutai-Executor: %s\nSubutai-Claim: %s",
				taskNumber(t.Task), whoWords(c.Kind, c.Actor, ""), c.Kind, c.ID)
			if err := tc.s.commitWorktree(t.Path, msg); err != nil {
				return r, fmt.Errorf("committing the work left in the working copy: %w", err)
			}
			r.Committed = true
		}
	}
	r.Commit = headOf(t.Path)
	return r, nil
}

func (tc taskClaims) onRelease(ctx context.Context, tx pgx.Tx, t *claimTarget, c *store.Claim, r released, by string) error {
	task, err := store.GetTask(ctx, tx, t.RefID)
	if err != nil {
		return err
	}
	if err := store.TransitionTask(ctx, tx, task, lifecycle.TaskRelease, by,
		map[string]any{"claim_id": c.ID.String(), "released_by": by}); err != nil {
		return err
	}
	cfg, err := tc.s.freshConfig()
	if err != nil {
		return err
	}
	return tc.s.enqueueImplementTx(ctx, tx, cfg, t.RefID)
}

func (tc taskClaims) staleQuestion(t *claimTarget, c *store.Claim, idle time.Duration) (string, string) {
	who := whoWords(c.Kind, c.Actor, "")
	hours := int(idle.Hours())
	if c.State == lifecycle.ClaimReturned {
		return fmt.Sprintf("%s, %s, was sent back by its code reviewer %s, and nobody has resumed it. The task is waiting. Is someone still working on it?",
			t.Label, t.Title, agoWords(c.LastActivityAt, time.Now())), "Release it to an agent"
	}
	return fmt.Sprintf("%s, %s, was claimed by %s %s, and nothing has changed in its working copy for %d hours. This feature's agents are waiting. Is someone still working on it?",
		t.Label, t.Title, who, agoWords(c.ClaimedAt, time.Now()), hours), "Release it to an agent"
}

func (tc taskClaims) deadlineQuestion(t *claimTarget, c *store.Claim) (string, string) {
	return fmt.Sprintf("%s, %s, claimed by %s, has passed its deadline. This feature's agents are waiting. Should it go on?",
		t.Label, t.Title, whoWords(c.Kind, c.Actor, "")), "Release it to an agent"
}

// contract is what the claimant is given (FR-3.2): the approved spec and
// plan, the task, and the surfaced decisions an implementer would be told.
func (tc taskClaims) contract(ctx context.Context, t *claimTarget, c *store.Claim) (map[string]any, error) {
	s := tc.s
	out := map[string]any{}
	for key, docType := range map[string]string{"spec": "spec", "dev_plan": "dev_plan"} {
		if doc, body := s.contractDoc(ctx, t.Feature.ID, docType); doc != nil {
			out[key] = map[string]any{"id": doc.PublicID, "path": doc.Path, "body": body}
		}
	}
	out["task"] = map[string]any{"id": t.Label, "title": t.Title, "description": t.Task.Description}
	block, err := s.surfacedBlock(ctx, s.scopeForFeature(ctx, t.Feature.ID))
	if err != nil {
		return nil, err
	}
	if block != "" {
		out["decisions"] = block
	}
	return out, nil
}

// endTaskClaim ends the task's claim, if it has one that hasn't ended, with
// the event of the task's own ending: done on approval, abandon on abandonment
// (SPEC-020 FR-2.6, SD-16). The caller holds the feature and task locks, and
// the task's transition is in the same transaction.
func endTaskClaim(ctx context.Context, tx pgx.Tx, taskID uuid.UUID, e lifecycle.ClaimEvent, actor string, payload map[string]any) error {
	claim, err := store.LockCurrentClaimFor(ctx, tx, "task", taskID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// The task ending can't be held up by the claim's own order of events: a
	// claim that wasn't submitted when its task is approved is abandoned.
	if _, err := lifecycle.ClaimTransition(claim.State, e); err != nil {
		e = lifecycle.ClaimEventAbandon
	}
	return store.TransitionClaim(ctx, tx, claim, e, actor, "", payload)
}

// branchHeadForTask reads the head of the branch of the task's feature, for
// the dispatcher to record as the agent's start head (FR-1.1). It is "" when
// there is no working copy to read.
func (s *Server) branchHeadForTask(ctx context.Context, taskID uuid.UUID) string {
	task, err := store.GetTask(ctx, s.Store.Pool, taskID)
	if err != nil {
		return ""
	}
	wt, err := store.LiveWorktreeForFeature(ctx, s.Store.Pool, task.FeatureID)
	if err != nil {
		return ""
	}
	return headOf(s.worktreeAbs(wt.Path))
}

// contractDoc is the feature's current approved document of the type, its
// newest when none is approved, and its body: the same choice contractBody
// makes for an implementer (SPEC-016 R16-5). A bug's spec is its report.
func (s *Server) contractDoc(ctx context.Context, featureID uuid.UUID, docType string) (*store.Document, string) {
	docType = s.contractDocType(ctx, featureID, docType)
	d, err := store.CurrentApprovedDocForOwner(ctx, s.Store.Pool, docType, "feature", featureID)
	if err != nil {
		if d, err = store.CurrentDocForOwner(ctx, s.Store.Pool, docType, "feature", featureID); err != nil {
			return nil, ""
		}
	}
	body, err := s.readDocFile(d.Path)
	if err != nil {
		return d, ""
	}
	return d, string(body)
}

// latestWriterIsChat reports whether the document's latest writing act (wrote,
// revised or added), counting its earlier revisions', was the chat agent's
// (SPEC-020 FR-8.3).
func (s *Server) latestWriterIsChat(ctx context.Context, docID uuid.UUID) (bool, error) {
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return false, err
	}
	ws, err := s.writerHistory(ctx, *doc)
	if err != nil {
		return false, err
	}
	for i := len(ws) - 1; i >= 0; i-- {
		switch ws[i].Act {
		case store.ActWrote, store.ActRevised, store.ActAdded:
			return ws[i].Kind == store.WriterChat, nil
		}
	}
	return false, nil
}

// codeReviewModel is the model for a task's code review: the reviewer's usual
// one, or, when the latest execution of the latest round is the chat agent's,
// the project's reviewer for the chat agent's work (SPEC-020 FR-8.2, FR-8.3).
func (s *Server) codeReviewModel(ctx context.Context, q store.Querier, cfg *config.Config, taskID uuid.UUID, reviewerRole string) (string, error) {
	usual, err := s.modelForPurpose(cfg, "review-code", reviewerRole)
	if err != nil {
		return "", err
	}
	execs, err := store.ExecutionsFor(ctx, q, "task", taskID)
	if err != nil {
		return "", err
	}
	if n := len(execs); n > 0 && execs[n-1].Kind == store.WriterChat {
		return cfg.ChatReviewModel(usual), nil
	}
	return usual, nil
}
