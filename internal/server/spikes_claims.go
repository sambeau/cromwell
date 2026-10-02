package server

// Claiming a spike (SPEC-021 FR-13, FR-14): the chat agent or a person running
// a spike in its throwaway working copy, through SPEC-020's claim service.
// Claiming and renewing go through claimLocked; saving findings, submitting
// and releasing are the spike's own, because the task's commit, review and
// implementer don't apply to a spike (FR-13.2). The lock order is SD-27's:
// the worktree's lock, then spikeEndMu, then the spike's row, then its claim.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/config"
	"subutai/internal/ident"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// spikeClaims is the claimable for a spike (FR-13.1).
type spikeClaims struct {
	noSubmitRelease
	s *Server
}

// spikeClaimRuleSentences are a spike claimant's own rules (FR-13.4), said
// once, here, so the chat skill can be checked against them.
var spikeClaimRuleSentences = []string{
	"Work only inside the working copy named in this result. It is a throwaway copy: it is discarded when the spike ends, and nothing in it is kept or merged.",
	"Don't commit, branch, tag or stash there: a spike keeps no code, and Subutai reports any it finds kept.",
	"Save your findings with save_spike_findings early and often. When the time box ends, whatever you have saved is what is kept.",
	"When you have an answer, or know the question can't be answered, call submit_spike with your findings in the template's sections.",
	"A person reads the findings and decides whether the question is answered. You can't close the spike, release your claim or move its time box, and you can't approve its findings on your own judgement: only a person's verdict, in their words, approves them.",
}

// spikeReleaseConsequence is what Release the claim does (FR-14.2).
const spikeReleaseConsequence = "The claim ends. The spike keeps running, its draft and working copy are kept, and its executor can claim it again before the time box ends."

// spikeNextSentence is what a claim's result says to do next (FR-13.4).
const spikeNextSentence = "Work on the question in the working copy, save findings as you go with save_spike_findings, and call submit_spike when you are done."

// spikeTimeBoxEndedBeforeSubmit is what a submit that lost the race to the
// time box's ending says (FR-13.7 step 4).
const spikeTimeBoxEndedBeforeSubmit = "The time box ended before this arrived, so the spike ended with the findings saved before it."

// spikeTimeLeft is FR-13.4's sentence: when the time box ends, in UTC, and how
// long is left.
func spikeTimeLeft(sp *store.Spike, now time.Time) string {
	if sp.DeadlineAt == nil {
		return ""
	}
	left := sp.DeadlineAt.Sub(now)
	if left <= 0 {
		return fmt.Sprintf("The time box ended at %s. The spike ends with whatever findings were saved.", spikeClock(*sp.DeadlineAt))
	}
	return fmt.Sprintf("The time box ends at %s, in %s. Then the spike ends with whatever findings you have saved.",
		spikeClock(*sp.DeadlineAt), spikeSpanWords(left))
}

// ---- The claimable ----

func (sc spikeClaims) load(ctx context.Context, q store.Querier, refID uuid.UUID) (*claimTarget, error) {
	sp, err := store.GetSpike(ctx, q, refID)
	if err != nil {
		return nil, err
	}
	t := &claimTarget{RefType: "spike", RefID: refID, Label: sp.PublicID, Title: sp.Question, Spike: sp}
	if sp.WorktreePath != "" && sp.WorktreeRemovedAt == nil {
		t.Path = sc.s.worktreeAbs(sp.WorktreePath)
	}
	return t, nil
}

// spikeStateWords is the sentence for a spike that isn't running, or "" for one
// that is. consequence is what the idea can't do, as the caller's refusal puts
// it: "it can't be claimed", "there is nothing to save".
func spikeStateWords(sp *store.Spike, consequence string) string {
	switch sp.State {
	case store.SpikeIdea:
		return fmt.Sprintf("%s isn't running, so %s. A person starts a spike from its page in the web UI.", sp.PublicID, consequence)
	case store.SpikeEnded, store.SpikeClosed:
		return fmt.Sprintf("%s has ended. A person reads its findings on its page.", sp.PublicID)
	}
	return ""
}

// spikeStateRefusal is FR-13.1's refusals for a spike that can't be claimed
// whoever asks, and for an executor that isn't the claimant's; "" when neither
// stands.
func spikeStateRefusal(sp *store.Spike, who Claimant) string {
	if r := spikeStateWords(sp, "it can't be claimed"); r != "" {
		return r
	}
	switch sp.Executor {
	case store.ExecutorChat:
		if who.Kind != store.WriterChat {
			return fmt.Sprintf("%s is to be run in chat, not in the web UI. Ask the chat agent to run it.", sp.PublicID)
		}
	case store.ExecutorPerson:
		if who.Kind != store.WriterPerson {
			return fmt.Sprintf("%s is to be run by a person in the web UI, not in chat.", sp.PublicID)
		}
	default:
		return fmt.Sprintf("%s is run by the spike runner, an agent, so it can't be claimed.", sp.PublicID)
	}
	return ""
}

// spikeDeadlineRefusal is the time box's sentence once its deadline has
// passed, and "" before it.
func spikeDeadlineRefusal(sp *store.Spike, now time.Time) string {
	if sp.DeadlineAt != nil && !sp.DeadlineAt.After(now) {
		return fmt.Sprintf("%s's time box ended at %s, so it is ending with the findings that were saved.",
			sp.PublicID, spikeClock(*sp.DeadlineAt))
	}
	return ""
}

// spikeSubmitted says whether a claim of the spike ended done, which makes the
// spike's ending final: it is ending as concluded, and nothing may claim it
// again (SPEC-021 FR-13.7). It reads every claim, not only the latest.
func spikeSubmitted(ctx context.Context, q store.Querier, sp *store.Spike) (bool, error) {
	return store.ClaimEndedDoneFor(ctx, q, "spike", sp.ID)
}

// spikeBarredRefusal is FR-13.1's refusals that no remake can mend, in its
// order: the state and the executor, a submit that has ended the claim for
// good, and the deadline. It is "" when none stands. A claim and a renewal are
// both judged by it, and a claim judges it before anything is made.
func spikeBarredRefusal(ctx context.Context, q store.Querier, sp *store.Spike, who Claimant, now time.Time) string {
	if r := spikeStateRefusal(sp, who); r != "" {
		return r
	}
	done, err := spikeSubmitted(ctx, q, sp)
	if err != nil {
		return fmt.Sprintf("%s can't be claimed just now: %v.", sp.PublicID, err)
	}
	if done {
		return fmt.Sprintf("%s has been submitted, and is ending. A person reads its findings on its page.", sp.PublicID)
	}
	return spikeDeadlineRefusal(sp, now)
}

// refusal is a claim's refusal, in FR-13.1's order. resume is a task's; a
// spike's claim is never returned, so there is nothing to resume.
func (sc spikeClaims) refusal(ctx context.Context, tx pgx.Tx, t *claimTarget, who Claimant, _ bool) string {
	sp := t.Spike
	if r := spikeBarredRefusal(ctx, tx, sp, who, time.Now()); r != "" {
		return r
	}
	cur, err := store.CurrentClaimFor(ctx, tx, "spike", t.RefID)
	switch {
	case err == nil && !who.holds(cur):
		return heldBySentence(sp.PublicID, cur, who)
	case err != nil && !errors.Is(err, store.ErrNotFound):
		return fmt.Sprintf("%s can't be claimed just now: %v.", sp.PublicID, err)
	}
	if t.Path == "" {
		return fmt.Sprintf("%s's working copy couldn't be made, so it can't be claimed yet: its directory is gone, and a person can ask for the spike to be claimed again.", sp.PublicID)
	}
	if _, err := os.Stat(filepath.Join(t.Path, ".git")); err != nil {
		return fmt.Sprintf("%s's working copy couldn't be made, so it can't be claimed yet: %v.", sp.PublicID, err)
	}
	return ""
}

// renewalRefusal is a renewal's refusal: only the state, a submit and the
// deadline are judged (FR-13.2). The claim is already the claimant's, and its
// working copy is whatever it left.
func (sc spikeClaims) renewalRefusal(ctx context.Context, tx pgx.Tx, t *claimTarget, who Claimant) string {
	return spikeBarredRefusal(ctx, tx, t.Spike, who, time.Now())
}

// onClaim gives the claim the spike's deadline and cancels nothing: a spike has
// no dispatches to take over.
func (sc spikeClaims) onClaim(ctx context.Context, tx pgx.Tx, t *claimTarget, c *store.Claim) ([]uuid.UUID, error) {
	if t.Spike.DeadlineAt == nil {
		return nil, fmt.Errorf("%s has no time box, so it can't be claimed", t.Label)
	}
	return nil, store.SetClaimDeadline(ctx, tx, c.ID, *t.Spike.DeadlineAt)
}

// noSubmitRelease stands in for the claimable's submit and release methods: a
// spike has its own, SubmitSpike and ReleaseSpikeClaim, because a task's commit,
// review and implementer don't apply to it (FR-13.2).
type noSubmitRelease struct{}

var errSpikeOwnMethod = errors.New("a spike has its own submit and release: SubmitSpike and ReleaseSpikeClaim")

func (noSubmitRelease) prepareSubmit(context.Context, *claimTarget, *store.Claim, string) (submitted, error) {
	return submitted{}, errSpikeOwnMethod
}

func (noSubmitRelease) onSubmit(context.Context, pgx.Tx, *claimTarget, *store.Claim, submitted) (string, error) {
	return "", errSpikeOwnMethod
}

func (noSubmitRelease) prepareRelease(context.Context, *claimTarget, *store.Claim) (released, error) {
	return released{}, errSpikeOwnMethod
}

func (noSubmitRelease) onRelease(context.Context, pgx.Tx, *claimTarget, *store.Claim, released, string) error {
	return errSpikeOwnMethod
}

func (sc spikeClaims) staleQuestion(t *claimTarget, c *store.Claim, idle time.Duration) (string, string) {
	end := "a time that isn't recorded"
	if d := t.Spike.DeadlineAt; d != nil {
		end = spikeDeadlineWords(*d)
	}
	return fmt.Sprintf("%s, %s, was claimed by %s %s, and nothing has changed in its working copy or its findings for %d hours. Its time box ends at %s. Is someone still working on it?",
		t.Label, t.Title, whoWords(c.Kind, c.Actor, ""), agoWords(c.ClaimedAt, time.Now()), int(idle.Hours()), end), "Release the claim"
}

// deadlineQuestion is never asked of a spike (SD-20): its deadline ends it. It
// says the stale question's words, for completeness.
func (sc spikeClaims) deadlineQuestion(t *claimTarget, c *store.Claim) (string, string) {
	return sc.staleQuestion(t, c, time.Since(c.LastActivityAt))
}

func (sc spikeClaims) lock(ctx context.Context, tx pgx.Tx, t *claimTarget) error {
	_, err := store.LockSpike(ctx, tx, t.RefID)
	return err
}

// rules are the sentences a spike's claimant is given: the rules, then the
// commands the project allows, as claim_task's rules list them.
func (sc spikeClaims) rules() []string { return sc.s.withCommandRules(spikeClaimRuleSentences) }

func (sc spikeClaims) releaseConsequence() string { return spikeReleaseConsequence }

// endAtDeadline ends the spike at its time box, for the claim sweep
// (FR-14.4). The ending is EndSpike's: serialised with the heartbeat's, and
// conditional on the spike still running.
func (sc spikeClaims) endAtDeadline(ctx context.Context, t *claimTarget) error {
	return sc.s.EndSpike(ctx, t.RefID, store.SpikeTimeBox, "")
}

// contract is what the claimant is given (FR-13.4).
func (sc spikeClaims) contract(ctx context.Context, t *claimTarget, c *store.Claim) (map[string]any, error) {
	s, sp := sc.s, t.Spike
	out := map[string]any{"question": sp.Question}
	if from := s.spikeOwnerSection(ctx, sp, t.Path); from != "" {
		out["where_it_came_from"] = from
	}
	block, err := s.surfacedBlock(ctx, surfaceScope{InitiativeID: &sp.InitiativeID})
	if err != nil {
		return nil, err
	}
	if block != "" {
		out["decisions"] = block
	}
	if id, body := s.lastSpikeFindings(ctx, sp); body != "" {
		out["earlier_findings"] = map[string]any{"spike": id, "body": body}
	}
	if d := strings.TrimSpace(sp.Draft); d != "" {
		out["draft"] = d
	}
	out["findings_template"] = s.findingsTemplateSections()
	return out, nil
}

// findingsPurposes say what each of the findings template's sections is for.
var findingsPurposes = map[string]string{
	headingQuestion: "Written by Subutai from the spike's question; leave it out.",
	headingAnswer:   "The answer, in a sentence or two, or why the question can't be answered.",
	headingFound:    "What you found out: the evidence and what it means.",
	headingHow:      "How you found out: what you read, ran or measured, so a person can check it.",
	headingNext:     "What to do next: the feature to build, the question to ask again, or nothing.",
	headingEnded:    "Written by Subutai when the spike ends; leave it out.",
}

// findingsTemplateSections are the findings template's sections, in the
// template's order, with whether the manifest requires each and what it is for
// (FR-13.4). A project whose template has gone missing gets the manifest's
// order, and one with neither gets none.
func (s *Server) findingsTemplateSections() []map[string]any {
	required := map[string]bool{}
	var order []string
	if m, err := config.LoadManifest(s.CompartmentRoot, lifecycle.DocTypeFindings); err == nil {
		for _, r := range m.Sections.Required {
			required[strings.ToLower(r.Heading)] = true
			order = append(order, r.Heading)
		}
		for _, r := range m.Sections.Optional {
			order = append(order, r.Heading)
		}
	}
	if raw, err := os.ReadFile(filepath.Join(s.CompartmentRoot, "templates", "findings", "template.md")); err == nil {
		var fromTemplate []string
		for _, line := range strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n") {
			if h, ok := strings.CutPrefix(line, "## "); ok {
				fromTemplate = append(fromTemplate, strings.TrimSpace(h))
			}
		}
		if len(fromTemplate) > 0 {
			order = fromTemplate
		}
	}
	out := make([]map[string]any, 0, len(order))
	for _, h := range order {
		out = append(out, map[string]any{"heading": h, "required": required[strings.ToLower(h)], "for": findingsPurposes[h]})
	}
	return out
}

// ---- Resolving, and the holder's refusals ----

// resolveSpikeRef reads a spike's public ID.
func (s *Server) resolveSpikeRef(ctx context.Context, ref string) (*store.Spike, error) {
	r, ok := ident.Parse(ref)
	if !ok || r.Shape != ident.ShapeEntity || r.Kind.Name != "spike" {
		return nil, refuse("%q isn't a spike's ID. Spikes have IDs like SPK-003, and list_spikes shows them.", ref)
	}
	sp, err := store.SpikeByPublicID(ctx, s.Store.Pool, r.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, refuse("There is no spike %s. list_spikes shows what exists.", r.ID)
	}
	return sp, err
}

// spikeHolderRefusal says why a save or a submit isn't the claimant's to make
// (FR-13.6, FR-13.7 step 1), or "". action is "save" or "submit". The deadline
// is left to the caller: a submit that has passed it once is honoured
// (FR-13.7 step 4).
func spikeHolderRefusal(sp *store.Spike, cur *store.Claim, who Claimant, action string) string {
	if r := spikeStateWords(sp, "there is nothing to "+action); r != "" {
		return r
	}
	if cur == nil {
		if action == "save" {
			return fmt.Sprintf("You haven't claimed %s, so there are no findings to save. Claim it first.", sp.PublicID)
		}
		return fmt.Sprintf("You haven't claimed %s, so there is nothing to submit. Claim it first.", sp.PublicID)
	}
	if !who.holds(cur) {
		if action == "save" {
			return fmt.Sprintf("%s is claimed by %s, not by you, so you can't save its findings.", sp.PublicID, whoWords(cur.Kind, cur.Actor, ""))
		}
		return fmt.Sprintf("%s is claimed by %s, not by you, so you can't submit it.", sp.PublicID, whoWords(cur.Kind, cur.Actor, ""))
	}
	if cur.State != lifecycle.ClaimOpen {
		return fmt.Sprintf("Your claim on %s isn't open, so there is nothing to %s.", sp.PublicID, action)
	}
	return ""
}

// spikeActRefusal is spikeHolderRefusal, then the deadline: a save, or a
// submit's first look (FR-13.6, FR-13.7 step 1).
func spikeActRefusal(sp *store.Spike, cur *store.Claim, who Claimant, action string, now time.Time) string {
	if r := spikeHolderRefusal(sp, cur, who, action); r != "" {
		return r
	}
	return spikeDeadlineRefusal(sp, now)
}

// currentSpikeClaim is the spike's claim that hasn't ended, or nil.
func currentSpikeClaim(ctx context.Context, q store.Querier, id uuid.UUID, lock bool) (*store.Claim, error) {
	var c *store.Claim
	var err error
	if lock {
		tx, ok := q.(pgx.Tx)
		if !ok {
			return nil, errors.New("a claim can be locked only inside a transaction")
		}
		c, err = store.LockCurrentClaimFor(ctx, tx, "spike", id)
	} else {
		c, err = store.CurrentClaimFor(ctx, q, "spike", id)
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	return c, err
}

// ---- Claiming (FR-13.3, FR-13.4) ----

// SpikeClaimResult is what claiming a spike returns (FR-13.4).
type SpikeClaimResult struct {
	Spike       *store.Spike
	Claim       *store.Claim
	WorkingCopy WorkingCopy // Path and BaseCommit; Branch is empty, the worktree is detached
	// Contract holds "question", "where_it_came_from", "decisions",
	// "earlier_findings" ({spike, body}), "draft" and "findings_template",
	// each when there is one.
	Contract map[string]any
	Rules    []string
	// TimeLeft is FR-13.4's sentence about the time box.
	TimeLeft string
	// Renewed is true for a claim the claimant already held open.
	Renewed bool
}

// ClaimSpike claims a spike for the chat agent or a person, or renews a claim
// the claimant already holds (FR-13.3). ref is the spike's public ID. Before
// the claim, holding the worktree's lock, the spike is read again and refused
// if nothing can mend it (it ended, or was submitted, or its deadline passed
// while this waited), and only then is the worktree made if it is missing, as
// the planner makes an agent spike's (FR-4.2); a remake of a working copy that
// existed first checks what it kept, and raises the leak check's checkpoint if
// it couldn't (FR-13.3). A refusal is a *ClaimRefusal.
func (s *Server) ClaimSpike(ctx context.Context, ref string, who Claimant) (*SpikeClaimResult, error) {
	if err := who.check(); err != nil {
		return nil, err
	}
	sp, err := s.resolveSpikeRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	sc := spikeClaims{s: s}
	lockPath := ""
	if sp.WorktreePath != "" {
		lockPath = s.worktreeAbs(sp.WorktreePath)
	}
	var t *claimTarget
	var res *ClaimResult
	err = s.withWorkingCopy(lockPath, func() error {
		// An ending may have held the lock first: what is read now is what
		// the ending left, and nothing is made for a spike that is over.
		var err error
		if t, err = sc.load(ctx, s.Store.Pool, sp.ID); err != nil {
			return err
		}
		if r := spikeBarredRefusal(ctx, s.Store.Pool, t.Spike, who, time.Now()); r != "" {
			return refuse("%s", r)
		}
		if t.Path != "" {
			if _, err := s.ensureSpikeWorktree(ctx, t.Spike); err != nil {
				return refuse("%s's working copy couldn't be made, so it can't be claimed yet: %v.", sp.PublicID, err)
			}
		}
		res, err = s.claimLocked(ctx, sc, t, who)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.notifyClaimTarget(t)
	fresh, err := store.GetSpike(ctx, s.Store.Pool, sp.ID)
	if err != nil {
		return nil, err
	}
	res.WorkingCopy.BaseCommit = fresh.BaseCommit
	return &SpikeClaimResult{
		Spike: fresh, Claim: res.Claim, WorkingCopy: res.WorkingCopy, Contract: res.Contract,
		Rules: res.Rules, TimeLeft: spikeTimeLeft(fresh, time.Now()), Renewed: res.Renewed,
	}, nil
}

// ---- Saving the draft (FR-13.6) ----

// SaveSpikeFindings replaces the spike's draft with text, as the claimant
// holding its open claim, before the deadline. It is activity: a pending
// claim-stale is withdrawn. A refusal is a *ClaimRefusal.
func (s *Server) SaveSpikeFindings(ctx context.Context, ref string, who Claimant, text string) (*store.Spike, error) {
	if err := who.check(); err != nil {
		return nil, err
	}
	sp, err := s.resolveSpikeRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" {
		return nil, refuse("There are no findings to save. Write them as sections, starting with Answer and What we found.")
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		// The spike's row, then its claim (SD-27), and the holder, the state
		// and the deadline are looked at again under them.
		locked, err := store.LockSpike(ctx, tx, sp.ID)
		if err != nil {
			return err
		}
		cur, err := currentSpikeClaim(ctx, tx, sp.ID, true)
		if err != nil {
			return err
		}
		if r := spikeActRefusal(locked, cur, who, "save", time.Now()); r != "" {
			return refuse("%s", r)
		}
		if err := store.SaveSpikeDraft(ctx, tx, sp.ID, text); err != nil {
			return err
		}
		_, err = store.RecordSpikeFindingsActivity(ctx, tx, cur.ID, who.Actor)
		return err
	})
	if errors.Is(err, store.ErrSpikeNotRunning) {
		return nil, refuse("%s has ended. A person reads its findings on its page.", sp.PublicID)
	}
	if err != nil {
		return nil, err
	}
	fresh, err := store.GetSpike(ctx, s.Store.Pool, sp.ID)
	if err != nil {
		return nil, err
	}
	s.notifySpikeChanged(fresh)
	return fresh, nil
}

// ---- Submitting (FR-13.7) ----

// SubmitSpike hands a claimed spike's findings in and ends the spike as
// concluded (SD-23). findings is the text given, or, when blank, the saved
// draft. It returns the ended spike. A refusal is a *ClaimRefusal.
func (s *Server) SubmitSpike(ctx context.Context, ref string, who Claimant, findings string) (*store.Spike, error) {
	if err := who.check(); err != nil {
		return nil, err
	}
	sp, err := s.resolveSpikeRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	// Step 1: the claimant holds the open claim, and the deadline hasn't passed.
	cur, err := currentSpikeClaim(ctx, s.Store.Pool, sp.ID, false)
	if err != nil {
		return nil, err
	}
	if r := spikeActRefusal(sp, cur, who, "submit", time.Now()); r != "" {
		return nil, refuse("%s", r)
	}
	// Step 2: the findings, validated as finish_spike validates them, so the
	// refusal names what is missing and nothing has changed.
	findings = strings.TrimSpace(findings)
	if findings == "" {
		findings = strings.TrimSpace(sp.Draft)
	}
	if findings == "" {
		return nil, refuse("There are no findings to submit, and none are saved. Write them as sections, starting with Answer and What we found.")
	}
	raw, _ := json.Marshal(map[string]string{"findings": findings})
	if err := s.validateFinishSpike(sp, s.spikeOwnerPath(ctx, sp))(raw); err != nil {
		return nil, refuse("The findings can't be submitted yet: %v. Nothing was changed.", err)
	}

	// Steps 3 to 5: the worktree's lock, then spikeEndMu, held until the spike
	// has ended, so no ending can fall between the claim's and the spike's.
	err = s.withSpikeEndLocks(sp, func() error {
		err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			locked, err := store.LockSpike(ctx, tx, sp.ID)
			if err != nil {
				return err
			}
			held, err := currentSpikeClaim(ctx, tx, sp.ID, true)
			if err != nil {
				return err
			}
			// An ending that won the locks first has ended the claim. The
			// deadline isn't judged again: a submit that passed step 1 is
			// honoured, even if the deadline passes while it runs.
			if locked.State != store.SpikeRunning || held == nil || held.ID != cur.ID {
				if locked.State == store.SpikeEnded && locked.EndedHow == store.SpikeTimeBox {
					return refuse("%s", spikeTimeBoxEndedBeforeSubmit)
				}
				if r := spikeHolderRefusal(locked, held, who, "submit"); r != "" {
					return refuse("%s", r)
				}
				return refuse("Your claim on %s changed while this was arriving. Read its page, and submit again if the claim is still yours.", sp.PublicID)
			}
			if r := spikeHolderRefusal(locked, held, who, "submit"); r != "" {
				return refuse("%s", r)
			}
			if err := store.SaveSpikeDraft(ctx, tx, sp.ID, findings); err != nil {
				return err
			}
			if err := store.TransitionClaim(ctx, tx, held, lifecycle.ClaimEventSubmit, who.Actor, "submitted", nil); err != nil {
				return err
			}
			if err := store.MarkClaimExecutionSubmitted(ctx, tx, held.ID); err != nil {
				return err
			}
			return store.TransitionClaim(ctx, tx, held, lifecycle.ClaimEventDone, who.Actor, "", nil)
		})
		if errors.Is(err, store.ErrStaleState) {
			return refuse("%s", spikeTimeBoxEndedBeforeSubmit)
		}
		if err != nil {
			return err
		}
		// The claim has ended done, so the ending no longer depends on this
		// request: a client that goes away must not stop it. If the server
		// stops here, reconciliation finishes it (FR-12.4).
		return s.endSpikeLocked(context.WithoutCancel(ctx), sp.ID, store.SpikeConcluded, "")
	})
	if err != nil {
		return nil, err
	}
	return store.GetSpike(ctx, s.Store.Pool, sp.ID)
}

// ---- Releasing (FR-14.1) ----

// ReleaseSpikeClaim ends a spike's claim, open or returned, as released by a
// person in the web UI. The spike keeps running: its draft and its working
// copy are kept, and nothing is committed or queued (SD-22). A refusal is a
// *ClaimRefusal.
func (s *Server) ReleaseSpikeClaim(ctx context.Context, ref string, by string) error {
	sp, err := s.resolveSpikeRef(ctx, ref)
	if err != nil {
		return err
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := store.LockSpike(ctx, tx, sp.ID); err != nil {
			return err
		}
		held, err := currentSpikeClaim(ctx, tx, sp.ID, true)
		if err != nil {
			return err
		}
		if held == nil {
			return refuse("%s has no claim to release.", sp.PublicID)
		}
		if _, err := lifecycle.ClaimTransition(held.State, lifecycle.ClaimEventRelease); err != nil {
			return refuse("%s's claim is %s, so it can't be released.", sp.PublicID, held.State)
		}
		return store.TransitionClaim(ctx, tx, held, lifecycle.ClaimEventRelease, by, "",
			map[string]any{"released_by": by})
	})
	if errors.Is(err, store.ErrStaleState) {
		return refuse("The claim on %s ended while you were releasing it.", sp.PublicID)
	}
	if err != nil {
		return err
	}
	s.notifySpikeChanged(sp)
	return nil
}
