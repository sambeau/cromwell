package server

// The claim service (SPEC-020 FR-2): a person or the chat agent doing a
// piece of work instead of a dispatched agent. MCP and the web UI both call
// ClaimWork, SubmitWork and ReleaseClaim (NFR-1); what differs by kind of
// claimed item lives behind the claimable interface (FR-2.10, SD-2), which
// M14 implements again for spikes.
//
// Git work in a feature's working copy is serialised per working copy by an
// in-process lock (FR-2.8), held across "git, then the transaction", and no
// git runs inside a database transaction (NFR-6). The transaction takes its
// row locks in one order, the feature, then the task, then the dispatch rows
// (FR-2.7).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/ident"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// Claimant is who is claiming, submitting or resuming: the chat agent over
// MCP, or a person in the web UI (SD-5).
type Claimant struct {
	Kind  string // chat | person
	Actor string // the MCP actor, or the person (ui_actor)
	Via   string // mcp | ui
}

// ChatClaimant is the chat agent, as the MCP surface presents it.
func (s *Server) ChatClaimant() Claimant {
	return Claimant{Kind: store.WriterChat, Actor: s.mcpActor(), Via: "mcp"}
}

// PersonClaimant is the person at the web UI.
func (s *Server) PersonClaimant() Claimant {
	return Claimant{Kind: store.WriterPerson, Actor: s.uiActor(), Via: "ui"}
}

func (c Claimant) check() error {
	if (c.Kind != store.WriterChat && c.Kind != store.WriterPerson) || (c.Via != "mcp" && c.Via != "ui") || c.Actor == "" {
		return errors.New("a claim is made by the chat agent over MCP or by a person in the web UI")
	}
	return nil
}

// holds reports whether the claim is this claimant's, from this surface (SD-5).
func (c Claimant) holds(cl *store.Claim) bool {
	return cl != nil && cl.Kind == c.Kind && cl.Actor == c.Actor && cl.Via == c.Via
}

// ClaimRefusal is a refusal of FR-2.4: a full sentence that says what to do
// instead. MCP shows it as a tool error, and the UI as a banner.
type ClaimRefusal struct {
	Sentence string
}

func (e *ClaimRefusal) Error() string { return e.Sentence }

func refuse(format string, args ...any) error {
	return &ClaimRefusal{Sentence: fmt.Sprintf(format, args...)}
}

// AsClaimRefusal returns the refusal an error is or wraps.
func AsClaimRefusal(err error) (*ClaimRefusal, bool) {
	var r *ClaimRefusal
	if errors.As(err, &r) {
		return r, true
	}
	return nil, false
}

// WorkingCopy is the feature's working copy as a claimant is given it.
type WorkingCopy struct {
	Path       string // absolute
	Branch     string
	BaseCommit string
}

// ClaimResult is what a claim, a renewal or a resumption returns (FR-3.2).
type ClaimResult struct {
	Task        *store.Task
	Feature     *store.Feature
	FeaturePath string
	Claim       *store.Claim
	WorkingCopy WorkingCopy
	// Contract holds "spec" and "dev_plan" ({id, path, body}), "task" ({id,
	// title, description}) and, when there is one, "decisions" (the surfaced
	// block as text).
	Contract map[string]any
	// ReviewComments is the latest code review's, in a round after the first.
	ReviewComments *ReviewComments
	// Rules are sentences: the claim's rules, then the commands the project
	// allows its implementers.
	Rules []string
	// Renewed is true for a claim the claimant already held open; Resumed for
	// a returned claim taken up again after a send-back.
	Renewed, Resumed bool
	// Cancelled are the implement dispatches the claim cancelled to take the
	// task.
	Cancelled []uuid.UUID
	Round     int
}

// SubmitResult is what a submit returns (FR-3.4).
type SubmitResult struct {
	Task        *store.Task
	ReviewRunID string // the queued code review, "" when an identical one already exists
	ReviewModel string
	Commit      string
	// Notice is set when the feature's spec is being revised (FR-2.5).
	Notice string
}

// ---- The claimable interface (FR-2.10) ----

// claimTarget is an item loaded for the claim service.
type claimTarget struct {
	RefType string
	RefID   uuid.UUID
	Label   string // its public ID, "FEAT-023-T03"
	Title   string
	Feature *store.Feature // the feature whose working copy it holds, or nil
	// Worktree is the feature's live worktree, nil when it has none.
	Worktree *store.Worktree
	Path     string // the working copy, absolute; "" when there is none
	Task     *store.Task
	// Spike is the spike, for a spike's claim (SPEC-021 FR-13.1); nil for a task.
	Spike *store.Spike
}

// submitted is what prepareSubmit did in git.
type submitted struct {
	Summary   string
	Commit    string // the head after the commit
	Committed bool   // whether Subutai made a commit (and so records it)
	Model     string // the code review's model
	Role      string
	// ChatReviewer is true when the model was chosen by the project's reviewer
	// for the chat agent's work and differs from the usual one (FR-8.4).
	ChatReviewer bool
}

// released is what prepareRelease did in git.
type released struct {
	Commit    string
	Committed bool
}

// claimable is what differs by kind of claimed item (SPEC-020 FR-2.10). The
// record, the activity rules, the sweep and the execution record are the
// claim service's.
type claimable interface {
	// load returns the item, the feature whose working copy it holds (or
	// nil), and the working copy's path.
	load(ctx context.Context, q store.Querier, refID uuid.UUID) (*claimTarget, error)
	// refusal says why it can't be claimed or resumed now, or "".
	refusal(ctx context.Context, tx pgx.Tx, t *claimTarget, who Claimant, resume bool) string
	// onClaim moves the item's own state, inside the claim's transaction. It
	// returns the dispatches it cancelled.
	onClaim(ctx context.Context, tx pgx.Tx, t *claimTarget, c *store.Claim) ([]uuid.UUID, error)
	// prepareSubmit does the git work, outside any transaction, holding the
	// working copy's lock; onSubmit records it, inside the transaction. It
	// returns the queued review's run.
	prepareSubmit(ctx context.Context, t *claimTarget, c *store.Claim, summary string) (submitted, error)
	onSubmit(ctx context.Context, tx pgx.Tx, t *claimTarget, c *store.Claim, sub submitted) (reviewRun string, err error)
	// prepareRelease and onRelease do the same for a release.
	prepareRelease(ctx context.Context, t *claimTarget, c *store.Claim) (released, error)
	onRelease(ctx context.Context, tx pgx.Tx, t *claimTarget, c *store.Claim, r released, by string) error
	// staleQuestion is the "still working on this?" question and its release
	// answer's words; deadlineQuestion the same for a time box.
	staleQuestion(t *claimTarget, c *store.Claim, idle time.Duration) (question, releaseLabel string)
	deadlineQuestion(t *claimTarget, c *store.Claim) (question, releaseLabel string)
	// contract is what the claimant is given.
	contract(ctx context.Context, t *claimTarget, c *store.Claim) (map[string]any, error)
	// lock takes the item's row locks, first in the claim's transaction
	// (SPEC-021 FR-13.2): a task's are its feature's and its own, a spike's its
	// own.
	lock(ctx context.Context, tx pgx.Tx, t *claimTarget) error
	// rules are the claimant's rules, said once in Go so the chat skills can be
	// checked against them: the claim's own sentences, then the commands the
	// project allows.
	rules() []string
	// releaseConsequence is what the release answer does, for the Inbox, beside
	// the label the stale and deadline questions return (FR-14.2).
	releaseConsequence() string
}

// renewalRefuser is the optional hook for a claimable whose renewal is judged
// too (SPEC-021 FR-13.2): a spike's is refused after its deadline. A task's
// renewal is never refused.
type renewalRefuser interface {
	renewalRefusal(ctx context.Context, tx pgx.Tx, t *claimTarget, who Claimant) string
}

// claimables is the registry by ref_type. M13 registers task; M14 registers
// spike.
func (s *Server) claimables() map[string]claimable {
	return map[string]claimable{"task": taskClaims{s: s}, "spike": spikeClaims{s: s}}
}

// ---- The per-working-copy lock (FR-2.8) ----

// withWorkingCopy runs fn holding the lock for the working copy at path. The
// server is one process, so an in-process lock serialises git work in it:
// claiming, submitting, releasing, the claim sweep's fingerprint, the branch
// watch and the implementer's completion each hold it across "git, then the
// transaction". An empty path (no working copy) takes no lock.
func (s *Server) withWorkingCopy(path string, fn func() error) error {
	if path == "" {
		return fn()
	}
	key := filepath.Clean(path)
	m, _ := s.copyLocks.LoadOrStore(key, new(lockedCopy))
	l := m.(*lockedCopy)
	l.mu.Lock()
	defer l.mu.Unlock()
	return fn()
}

// ---- The fingerprint (FR-5.2) ----

// worktreeFingerprint is a hash of the branch head and of `git
// --no-optional-locks status --porcelain=v1 -z --untracked-files=all`,
// together with the size and modification time of each file status lists. It
// changes on a commit, an edit, a new file or a deletion. --no-optional-locks
// means it never takes git's index lock from under the claimant.
func worktreeFingerprint(path string) (string, error) {
	head, err := gitIn(path, "--no-optional-locks", "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	status, err := gitIn(path, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(strings.TrimSpace(head)))
	h.Write([]byte{0})
	h.Write([]byte(status))
	entries := strings.Split(status, "\x00")
	for i := 0; i < len(entries); i++ {
		e := entries[i]
		if len(e) < 4 {
			continue
		}
		name := e[3:]
		if e[0] == 'R' || e[0] == 'C' || e[1] == 'R' || e[1] == 'C' {
			i++ // a rename or copy is followed by its original path
		}
		if fi, err := os.Lstat(filepath.Join(path, name)); err == nil {
			fmt.Fprintf(h, "\n%s|%d|%d", name, fi.Size(), fi.ModTime().UnixNano())
		} else {
			fmt.Fprintf(h, "\n%s|gone", name)
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// workingCopyChanged reports whether anything in the working copy differs
// from the commit it started at, committed or not: edits to tracked files
// and new files alike (FR-2.5). An unknown start counts as a change.
func workingCopyChanged(path, startHead string) (bool, error) {
	if startHead == "" {
		return true, nil
	}
	diff, err := gitIn(path, "--no-optional-locks", "diff", "--name-only", startHead)
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(diff) != "" {
		return true, nil
	}
	others, err := gitIn(path, "--no-optional-locks", "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(others) != "", nil
}

// workingCopyDirty reports uncommitted changes.
func workingCopyDirty(path string) (bool, error) {
	out, err := gitIn(path, "--no-optional-locks", "status", "--porcelain=v1", "--untracked-files=all")
	return strings.TrimSpace(out) != "", err
}

// specRevisedNotice is what a submit says while the feature's spec is being
// revised.
const specRevisedNotice = "The spec is being revised; the reviewer reviews this against the approved spec."

// claimActivityWords says what a claim's last activity was, in words a person
// reads: the panel, the Inbox and MCP all use it, so a machine word such as
// sent_back is never printed.
func claimActivityWords(activity string) string {
	switch activity {
	case "claimed":
		return "claimed"
	case "renewed":
		return "renewed"
	case "resumed":
		return "resumed"
	case "submitted":
		return "submitted"
	case "sent_back":
		return "sent back by its code reviewer"
	case "kept":
		return "kept by a person"
	case "worktree":
		return "a change in the working copy"
	case "findings":
		return "findings saved"
	}
	return "something else"
}

// ---- Resolving what is claimed ----

const onlyTasksSentence = "Only tasks can be claimed. Verification is always done by Subutai's verifier."

// resolveClaimRef reads a task's public ID. Anything else, a feature's or a
// bug's ID included, is refused with FR-6.3's sentence.
func (s *Server) resolveClaimRef(ctx context.Context, ref string) (claimable, uuid.UUID, error) {
	r, ok := ident.Parse(ref)
	if ok && r.Shape == ident.ShapeEntity && r.Kind.Name == "spike" {
		return nil, uuid.Nil, refuse("%s is a spike, not a task. Use claim_spike to run it.", r.ID)
	}
	if !ok || r.Shape != ident.ShapeTask {
		return nil, uuid.Nil, refuse("%s", onlyTasksSentence)
	}
	t, err := store.TaskByPublicID(ctx, s.Store.Pool, r.ID)
	if errors.Is(err, store.ErrNotFound) {
		return nil, uuid.Nil, refuse("There is no task %s. Use get_feature to see a feature's tasks.", r.ID)
	}
	if err != nil {
		return nil, uuid.Nil, err
	}
	return s.claimables()["task"], t.ID, nil
}

// ---- Claiming (FR-2.3) ----

// ClaimWork claims a task for the chat agent or a person, renews a claim the
// claimant already holds open, or resumes one a code review sent back. ref is
// the task's public ID. A refusal is a *ClaimRefusal.
func (s *Server) ClaimWork(ctx context.Context, ref string, who Claimant) (*ClaimResult, error) {
	if err := who.check(); err != nil {
		return nil, err
	}
	c, refID, err := s.resolveClaimRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	t, err := c.load(ctx, s.Store.Pool, refID)
	if err != nil {
		return nil, err
	}
	var res *ClaimResult
	err = s.withWorkingCopy(t.Path, func() error {
		var err error
		res, err = s.claimLocked(ctx, c, t, who)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.notifyClaimTarget(t)
	return res, nil
}

// notifyClaimTarget signals the pages of the claimed item after the commit
// that changed its claim: a task's and its feature's, or a spike's and its
// owner's. It assumes neither.
func (s *Server) notifyClaimTarget(t *claimTarget) {
	if t.Spike != nil {
		s.notifySpikeChanged(t.Spike)
		return
	}
	s.notifyEntityChanged(t.RefType, t.RefID)
	if t.Feature != nil {
		s.notifyEntityChanged("feature", t.Feature.ID)
	}
}

func (s *Server) claimLocked(ctx context.Context, c claimable, t *claimTarget, who Claimant) (*ClaimResult, error) {
	// The watch comes first, as it does before a completion, a submit and a
	// release (FR-5.7): a commit made just before this claim is judged as
	// unclaimed work, and not swallowed because a claim has opened.
	if t.Worktree != nil {
		s.watchBranchLogged(ctx, t.Worktree)
	}
	// Step 1, outside any transaction: the branch head and the working copy's
	// fingerprint.
	var head, seen string
	if t.Path != "" {
		if _, err := os.Stat(t.Path); err == nil {
			head = headOf(t.Path)
			seen, _ = worktreeFingerprint(t.Path)
		}
	}

	res := &ClaimResult{}
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		*res = ClaimResult{}
		if err := c.lock(ctx, tx, t); err != nil {
			return err
		}
		// Look again under the locks: the item may have moved on.
		fresh, err := c.load(ctx, tx, t.RefID)
		if err != nil {
			return err
		}
		cur, err := store.LockCurrentClaimFor(ctx, tx, t.RefType, t.RefID)
		if errors.Is(err, store.ErrNotFound) {
			cur = nil
		} else if err != nil {
			return err
		}

		switch {
		case who.holds(cur) && cur.State == lifecycle.ClaimOpen:
			// A renewal: the claimant says it is still on it (SD-8). A spike's
			// is refused after its deadline (SPEC-021 FR-13.2).
			if rr, ok := c.(renewalRefuser); ok {
				if r := rr.renewalRefusal(ctx, tx, fresh, who); r != "" {
					return refuse("%s", r)
				}
			}
			if err := store.TransitionClaim(ctx, tx, cur, lifecycle.ClaimEventRenew, who.Actor, "renewed", nil); err != nil {
				return err
			}
			res.Renewed = true
		case who.holds(cur) && cur.State == lifecycle.ClaimReturned:
			if r := c.refusal(ctx, tx, fresh, who, true); r != "" {
				return refuse("%s", r)
			}
			if err := store.TransitionClaim(ctx, tx, cur, lifecycle.ClaimEventResume, who.Actor, "resumed", nil); err != nil {
				return err
			}
			if _, err := store.RecordClaimExecution(ctx, tx, t.RefType, t.RefID, cur, head); err != nil {
				return err
			}
			res.Resumed = true
		default:
			if r := c.refusal(ctx, tx, fresh, who, false); r != "" {
				return refuse("%s", r)
			}
			var fid *uuid.UUID
			if fresh.Feature != nil {
				fid = &fresh.Feature.ID
			}
			// The base is the head now, when no one has begun this task: the
			// submit's check, and the code review's diff, then see the
			// claimant's work and not a sibling's (step 3).
			earlier, err := store.ExecutionsFor(ctx, tx, t.RefType, t.RefID)
			if err != nil {
				return err
			}
			cur, err = store.CreateClaim(ctx, tx, t.RefType, t.RefID, fid, who.Kind, who.Actor, who.Via, seen)
			if err != nil {
				return err
			}
			if res.Cancelled, err = c.onClaim(ctx, tx, fresh, cur); err != nil {
				return err
			}
			if len(earlier) == 0 && head != "" && t.Task != nil {
				if err := store.ResetTaskBaseCommit(ctx, tx, t.RefID, head); err != nil {
					return err
				}
			}
			if _, err := store.RecordClaimExecution(ctx, tx, t.RefType, t.RefID, cur, head); err != nil {
				return err
			}
		}
		res.Claim = cur
		return nil
	})
	if errors.Is(err, store.ErrStaleState) {
		look := "get_feature"
		if t.Spike != nil {
			look = "get_spike"
		}
		return nil, refuse("%s changed while you were claiming it. Look at it again with %s, then try once more.", t.Label, look)
	}
	if err != nil {
		return nil, err
	}
	return s.claimResult(ctx, c, t, res)
}

// claimResult fills what a claim returns, reading the state it left.
func (s *Server) claimResult(ctx context.Context, c claimable, t *claimTarget, res *ClaimResult) (*ClaimResult, error) {
	fresh, err := c.load(ctx, s.Store.Pool, t.RefID)
	if err != nil {
		return nil, err
	}
	res.Task = fresh.Task
	res.Feature = fresh.Feature
	if fresh.Feature != nil {
		if res.FeaturePath, err = s.featurePath(ctx, fresh.Feature); err != nil {
			return nil, err
		}
	}
	res.WorkingCopy = WorkingCopy{Path: fresh.Path}
	if fresh.Worktree != nil {
		res.WorkingCopy.Branch = fresh.Worktree.Branch
	}
	if fresh.Task != nil {
		res.WorkingCopy.BaseCommit = fresh.Task.BaseCommit
	}
	if res.Contract, err = c.contract(ctx, fresh, res.Claim); err != nil {
		return nil, err
	}
	// The round and the reviewer's comments are a task's (SPEC-021 FR-13.2).
	if fresh.Task != nil {
		if res.ReviewComments, err = s.reviewComments(ctx, s.Store.Pool, t.RefID); err != nil {
			return nil, err
		}
		if res.Round, err = store.CurrentTaskRound(ctx, s.Store.Pool, t.RefID); err != nil {
			return nil, err
		}
	}
	res.Rules = c.rules()
	return res, nil
}

// claimRules are the sentences a claimant is given: how to work, and the
// commands the project allows its implementers (FR-3.2). They are said once,
// here, so the chat skill can be checked against them (FR-9.2).
func (s *Server) claimRules() []string {
	return s.withCommandRules(claimRuleSentences)
}

// withCommandRules is the sentences, then one for each command the project
// allows its implementers, in name order.
func (s *Server) withCommandRules(sentences []string) []string {
	rules := append([]string(nil), sentences...)
	cfg, err := s.freshConfig()
	if err != nil {
		return rules
	}
	names := make([]string, 0, len(cfg.Commands))
	for n := range cfg.Commands {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		rules = append(rules, fmt.Sprintf("The project allows the command %s, which runs: %s.", n, strings.Join(cfg.Commands[n].Argv, " ")))
	}
	return rules
}

// claimRuleSentences are the claim's own rules (FR-3.1, FR-9.1).
var claimRuleSentences = []string{
	"Edit files only inside the working copy named in this result.",
	"Don't commit: Subutai commits your work when you submit it.",
	"Run the project's build and tests before you submit.",
	"Submitting with a summary is the only way forward, and you can't release the claim yourself: if you can't finish, tell the person, who can release it in the web UI.",
	"Your work gets an independent code review and the feature an independent verification, and you can't review, approve or verify it.",
	"Only one task in a feature can be worked at a time, and the feature's agents wait while you hold the claim, so submit promptly.",
}

// ---- Submitting (FR-2.5) ----

// SubmitWork hands a claimed task back: it commits the working copy, moves
// the task to review and queues the independent code review. who must be the
// claim's holder, from the surface that made it (SD-5). A refusal is a
// *ClaimRefusal.
func (s *Server) SubmitWork(ctx context.Context, ref string, who Claimant, summary string) (*SubmitResult, error) {
	if err := who.check(); err != nil {
		return nil, err
	}
	c, refID, err := s.resolveClaimRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return nil, refuse("Say in a sentence or two what you did; a submit needs a summary for the reviewer.")
	}
	t, err := c.load(ctx, s.Store.Pool, refID)
	if err != nil {
		return nil, err
	}
	var res *SubmitResult
	err = s.withWorkingCopy(t.Path, func() error {
		var err error
		res, err = s.submitLocked(ctx, c, t, who, summary)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.Dispatcher.Kick()
	s.notifyEntityChanged("task", t.RefID)
	if t.Feature != nil {
		s.notifyEntityChanged("feature", t.Feature.ID)
	}
	return res, nil
}

func (s *Server) submitLocked(ctx context.Context, c claimable, t *claimTarget, who Claimant, summary string) (*SubmitResult, error) {
	cur, err := store.CurrentClaimFor(ctx, s.Store.Pool, t.RefType, t.RefID)
	if errors.Is(err, store.ErrNotFound) {
		cur = nil
	} else if err != nil {
		return nil, err
	}
	if cur == nil || !who.holds(cur) || cur.State != lifecycle.ClaimOpen {
		return nil, refuse("%s", s.submitRefusal(ctx, t, who, cur))
	}
	if t.Path == "" {
		return nil, refuse("%s's working copy is missing. A person needs to retry creating it from the Inbox.", t.Feature.PublicID)
	}
	exec, err := s.latestClaimExecution(ctx, cur)
	if err != nil {
		return nil, err
	}
	changed, err := workingCopyChanged(t.Path, exec.StartHead)
	if err != nil {
		return nil, err
	}
	if !changed {
		return nil, refuse("Nothing in the working copy has changed since you claimed %s. Make the change, then submit; or ask a person to release the claim.", t.Label)
	}

	if t.Worktree != nil {
		s.watchBranchLogged(ctx, t.Worktree)
	}
	sub, err := c.prepareSubmit(ctx, t, cur, summary)
	if err != nil {
		return nil, err
	}

	var res SubmitResult
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		res = SubmitResult{Commit: sub.Commit, ReviewModel: sub.Model}
		if err := store.LockFeatureAndTask(ctx, tx, t.Feature.ID, t.RefID); err != nil {
			return err
		}
		// A release may have won the lock since: the commit then stands as the
		// released work, and the submit is refused (FR-2.5 step 5).
		held, err := store.LockCurrentClaimFor(ctx, tx, t.RefType, t.RefID)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
		if err != nil || held.ID != cur.ID || held.State != lifecycle.ClaimOpen {
			return refuse("%s", s.submitRefusal(ctx, t, who, claimOrNil(held, err)))
		}
		if sub.Committed {
			if err := store.RecordWorktreeCommit(ctx, tx, t.Worktree.ID, sub.Commit, "submit"); err != nil {
				return err
			}
		}
		if err := store.TransitionClaim(ctx, tx, held, lifecycle.ClaimEventSubmit, who.Actor, "submitted",
			map[string]any{"summary": summary, "commit": sub.Commit, "model": sub.Model, "chat_reviewer": sub.ChatReviewer}); err != nil {
			return err
		}
		if err := store.MarkClaimExecutionSubmitted(ctx, tx, held.ID); err != nil {
			return err
		}
		res.ReviewRunID, err = c.onSubmit(ctx, tx, t, held, sub)
		return err
	})
	if errors.Is(err, store.ErrStaleState) {
		return nil, refuse("%s changed while you were submitting it. Look at it again with get_feature.", t.Label)
	}
	if err != nil {
		return nil, err
	}
	if res.Task, err = store.GetTask(ctx, s.Store.Pool, t.RefID); err != nil {
		return nil, err
	}
	if stale, _ := s.specBeingRevised(ctx, t.Feature.ID); stale {
		res.Notice = specRevisedNotice
	}
	return &res, nil
}

// claimOrNil is the claim a lookup returned, or nil when it returned an error
// (a claim that is gone reads as no claim).
func claimOrNil(c *store.Claim, err error) *store.Claim {
	if err != nil {
		return nil
	}
	return c
}

// submitRefusal says why a submit isn't the claimant's to make, naming a
// release and who made it when that is the reason.
func (s *Server) submitRefusal(ctx context.Context, t *claimTarget, who Claimant, cur *store.Claim) string {
	switch {
	case cur == nil:
		if last, err := store.LatestClaimFor(ctx, s.Store.Pool, t.RefType, t.RefID); err == nil && last.State == lifecycle.ClaimEnded {
			switch last.EndReason {
			case "released":
				return fmt.Sprintf("The claim on %s was released by %s, so the work can't be submitted. What was in the working copy was kept, and an agent has the task now; get_feature shows where it stands.", t.Label, last.EndedBy)
			case "done":
				return fmt.Sprintf("%s is already done, so there is nothing to submit.", t.Label)
			case "abandoned":
				return fmt.Sprintf("The claim on %s ended because the work was abandoned, so there is nothing to submit.", t.Label)
			}
		}
		return fmt.Sprintf("You haven't claimed %s, so there is nothing to submit. Claim it first.", t.Label)
	case !who.holds(cur):
		return fmt.Sprintf("%s is claimed by %s, not by you, so you can't submit it.", t.Label, whoWords(cur.Kind, cur.Actor, ""))
	case cur.State == lifecycle.ClaimSubmitted:
		return fmt.Sprintf("%s was already submitted and is in code review. If the reviewer asks for changes, it comes back to you; get_feature shows the comments.", t.Label)
	case cur.State == lifecycle.ClaimReturned:
		return fmt.Sprintf("%s was sent back by its code reviewer. Resume it by claiming it again, then submit.", t.Label)
	}
	return fmt.Sprintf("%s can't be submitted from here.", t.Label)
}

// latestClaimExecution is the execution row of the claim's current round: the
// one whose start head the submit compares with.
func (s *Server) latestClaimExecution(ctx context.Context, c *store.Claim) (*store.Execution, error) {
	execs, err := store.ExecutionsFor(ctx, s.Store.Pool, c.RefType, c.RefID)
	if err != nil {
		return nil, err
	}
	for i := len(execs) - 1; i >= 0; i-- {
		if execs[i].ClaimID != nil && *execs[i].ClaimID == c.ID {
			return &execs[i], nil
		}
	}
	return nil, errors.New("the claim has no execution record")
}

// ---- Releasing (FR-2.9, SD-7) ----

// ReleaseClaim ends a claim that is open or returned and hands the task to an
// agent, keeping the work: anything uncommitted in the working copy is
// committed first, and an implement dispatch is queued. It is for the web UI
// (SD-5); by is the person. A submitted claim can't be released. A refusal is
// a *ClaimRefusal.
func (s *Server) ReleaseClaim(ctx context.Context, ref string, by string) error {
	c, refID, err := s.resolveClaimRef(ctx, ref)
	if err != nil {
		return err
	}
	t, err := c.load(ctx, s.Store.Pool, refID)
	if err != nil {
		return err
	}
	if err := s.withWorkingCopy(t.Path, func() error { return s.releaseLocked(ctx, c, t, by) }); err != nil {
		return err
	}
	s.Dispatcher.Kick()
	s.notifyEntityChanged("task", t.RefID)
	if t.Feature != nil {
		s.notifyEntityChanged("feature", t.Feature.ID)
	}
	return nil
}

func (s *Server) releaseLocked(ctx context.Context, c claimable, t *claimTarget, by string) error {
	cur, err := store.CurrentClaimFor(ctx, s.Store.Pool, t.RefType, t.RefID)
	if errors.Is(err, store.ErrNotFound) {
		return refuse("%s has no claim to release.", t.Label)
	}
	if err != nil {
		return err
	}
	if cur.State == lifecycle.ClaimSubmitted {
		return refuse("%s is in code review. Wait for the verdict.", t.Label)
	}

	if t.Worktree != nil {
		s.watchBranchLogged(ctx, t.Worktree)
	}
	r, err := c.prepareRelease(ctx, t, cur)
	if err != nil {
		return err
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.LockFeatureAndTask(ctx, tx, t.Feature.ID, t.RefID); err != nil {
			return err
		}
		held, err := store.LockCurrentClaimFor(ctx, tx, t.RefType, t.RefID)
		if errors.Is(err, store.ErrNotFound) || (err == nil && held.ID != cur.ID) {
			return refuse("The claim on %s ended while you were releasing it.", t.Label)
		}
		if err != nil {
			return err
		}
		if held.State == lifecycle.ClaimSubmitted {
			return refuse("%s is in code review. Wait for the verdict.", t.Label)
		}
		if r.Committed {
			if err := store.RecordWorktreeCommit(ctx, tx, t.Worktree.ID, r.Commit, "release"); err != nil {
				return err
			}
		}
		if err := store.TransitionClaim(ctx, tx, held, lifecycle.ClaimEventRelease, by, "",
			map[string]any{"released_by": by, "commit": r.Commit}); err != nil {
			return err
		}
		return c.onRelease(ctx, tx, t, held, r, by)
	})
	if errors.Is(err, store.ErrStaleState) {
		return refuse("%s changed while you were releasing it. Look at it again, then try once more.", t.Label)
	}
	return err
}

// ---- Specs under revision ----

// specBeingRevised reports whether a feature's contract is under revision
// (SD-3): its spec is stale, or a revision-in-flight question is pending.
func (s *Server) specBeingRevised(ctx context.Context, featureID uuid.UUID) (bool, error) {
	return specBeingRevised(ctx, s.Store.Pool, featureID)
}

func specBeingRevised(ctx context.Context, q store.Querier, featureID uuid.UUID) (bool, error) {
	var stale, pending bool
	err := q.QueryRow(ctx, `
		SELECT f.spec_stale,
		       EXISTS (SELECT 1 FROM checkpoints c WHERE c.kind = 'revision-in-flight'
		               AND c.ref_type = 'feature' AND c.ref_id = f.id AND c.state = 'pending')
		FROM features f WHERE f.id = $1`, featureID).Scan(&stale, &pending)
	return stale || pending, err
}

// ---- The reviewer's comments (FR-2.12) ----

// ReviewFinding is one finding of a code review.
type ReviewFinding struct {
	Severity string `json:"severity"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Text     string `json:"text"`
}

// ReviewComments are the latest code review's comments on a task and the run
// that made them, whoever implemented the round they were made on.
type ReviewComments struct {
	RunID    string          `json:"run_id,omitempty"`
	Comments []ReviewFinding `json:"comments"`
	At       time.Time       `json:"at"`
}

var fileLineRe = regexp.MustCompile(`^(.+?):(\d+)$`)

// reviewComments reads the latest task.review_comments audit row, written by
// returnTaskCode, and nil when the task has never been sent back (FR-2.12).
// The claim's result, the task page, get_feature and planImplement all use it.
func (s *Server) reviewComments(ctx context.Context, q store.Querier, taskID uuid.UUID) (*ReviewComments, error) {
	var raw []byte
	var at time.Time
	err := q.QueryRow(ctx, `
		SELECT payload, occurred_at FROM audit_events
		WHERE kind = 'task.review_comments' AND ref_type = 'task' AND ref_id = $1
		ORDER BY occurred_at DESC, id DESC LIMIT 1`, taskID).Scan(&raw, &at)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var p struct {
		DispatchID string `json:"dispatch_id"`
		Comments   []struct {
			SectionRef string `json:"section_ref"`
			Body       string `json:"body"`
			Severity   string `json:"severity"`
		} `json:"comments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	rc := &ReviewComments{RunID: p.DispatchID, At: at, Comments: []ReviewFinding{}}
	for _, c := range p.Comments {
		f := ReviewFinding{Severity: c.Severity, File: c.SectionRef, Text: c.Body}
		if f.Severity == "" {
			f.Severity = "major" // an empty severity reads as major (rules.ReviewComment)
		}
		if m := fileLineRe.FindStringSubmatch(c.SectionRef); m != nil {
			f.File = m[1]
			f.Line, _ = strconv.Atoi(m[2])
		}
		rc.Comments = append(rc.Comments, f)
	}
	return rc, nil
}

// reviewCommentsText is the comments as the implementer is shown them.
func reviewCommentsText(rc *ReviewComments) string {
	var b strings.Builder
	for _, f := range rc.Comments {
		fmt.Fprintf(&b, "- [%s] ", f.Severity)
		if f.File != "" {
			b.WriteString(f.File)
			if f.Line > 0 {
				fmt.Fprintf(&b, ":%d", f.Line)
			}
			b.WriteString(": ")
		}
		b.WriteString(f.Text + "\n")
	}
	return b.String()
}

// ---- Who executed a task (FR-1.4) ----

// ExecutorLine says who executed a task and carries the fields of FR-1.8.
type ExecutorLine struct {
	Sentence string
	Kind     string // agent | chat | person; "" when nobody has started
	Who      string
	Model    string
	RunID    string
	// Measured is false when any of the task's executions was by the chat
	// agent or a person (SD-13).
	Measured bool
}

// executorLine reads a task's executions and latest claim and says who
// executed it.
func (s *Server) executorLine(ctx context.Context, q store.Querier, t *store.Task) (ExecutorLine, error) {
	execs, err := store.ExecutionsFor(ctx, q, "task", t.ID)
	if err != nil {
		return ExecutorLine{}, err
	}
	claim, err := store.LatestClaimFor(ctx, q, "task", t.ID)
	if errors.Is(err, store.ErrNotFound) {
		claim = nil
	} else if err != nil {
		return ExecutorLine{}, err
	}
	return executorSentence(t, execs, claim, time.Now()), nil
}

func sameExecutor(a, b store.Execution) bool { return a.Kind == b.Kind && a.Actor == b.Actor }

// executorSentence is executorLine without the reads. It names the latest
// round's executor, an earlier round's when it differs, and a released claim's
// whose work an agent finished (FR-1.4). An inferred row adds nothing: before
// 0014 every executor was an agent. now is the time "ago" counts from.
func executorSentence(t *store.Task, execs []store.Execution, claim *store.Claim, now time.Time) ExecutorLine {
	if len(execs) == 0 {
		return ExecutorLine{Sentence: "Nobody has started this task yet.", Measured: true}
	}
	last := execs[len(execs)-1]
	out := ExecutorLine{Kind: last.Kind, Who: whoWords(last.Kind, last.Actor, last.Model), Model: last.Model, Measured: true}
	if last.DispatchID != nil {
		out.RunID = last.DispatchID.String()
	}
	for _, e := range execs {
		if !e.Measured {
			out.Measured = false
		}
	}

	var lead string
	active := t.State == lifecycle.TaskActive || t.State == lifecycle.TaskReady
	var earlierUnfinished *store.Execution
	for i := range execs[:len(execs)-1] {
		e := execs[i]
		if e.SubmittedAt == nil && e.Round == last.Round && !sameExecutor(e, last) {
			earlierUnfinished = &execs[i]
		}
	}
	var previous *store.Execution
	for i := len(execs) - 2; i >= 0; i-- {
		if execs[i].Round < last.Round {
			previous = &execs[i]
			break
		}
	}
	verb := "Implemented by "
	if active {
		verb = "Being implemented by "
	}
	switch {
	case earlierUnfinished != nil:
		lead = verb + out.Who + ", from work " + whoWords(earlierUnfinished.Kind, earlierUnfinished.Actor, earlierUnfinished.Model) + " started"
	case previous != nil && !sameExecutor(*previous, last):
		if active {
			lead = "Being reworked by " + out.Who + ", after " + whoWords(previous.Kind, previous.Actor, previous.Model) + " implemented it"
		} else {
			lead = "Implemented by " + whoWords(previous.Kind, previous.Actor, previous.Model) + ", then reworked by " + out.Who
		}
	default:
		lead = verb + out.Who
	}
	if claim != nil && last.ClaimID != nil && *last.ClaimID == claim.ID {
		switch claim.State {
		case lifecycle.ClaimOpen:
			lead += ", who claimed it " + agoWords(claim.ClaimedAt, now)
		case lifecycle.ClaimReturned:
			lead = "Implemented by " + out.Who + ", then sent back by the code reviewer; the claim is waiting to be resumed"
		}
	}
	out.Sentence = lead + "."
	return out
}
