package server

// The authoring chain (SPEC-009 Stage 1, retriggered by SPEC-011). Approving a
// design records what we want and starts nothing; a person pressing Send to
// development is what commits resources (DEC-006). From the send, the
// orchestrator carries a feature to decomposition and an estimate with no
// further human involvement, and stops at gate 2 — Start building.
//
// The chain is expressed as two invariants rather than a sequence of steps
// (FR-4), because width-first planning means approvals, features, descriptions
// and sends arrive in any order:
//
//	every sent feature G0 admits, and that has a description, has a current spec
//	every sent feature with an approved spec has a current dev-plan
//
// Stating them this way means the heartbeat can reconcile them: a feature that
// should have a spec and does not eventually gets one even if an event was
// lost. A list of rules would give no such property.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/config"
	"subutai/internal/lifecycle"
	"subutai/internal/rules"
	"subutai/internal/store"
)

// designApproved reports whether an owner's current design document is
// approved. A missing design is not an error — it is simply not approved,
// the same reading evaluateContractGate takes of a missing spec.
//
// An approved design stays in force while a revision of it is open: the
// predecessor is superseded only when its successor is approved, so G0 asks
// whether an approved, unsuperseded design exists rather than reading the
// newest document, which would be the revision's draft (found by the
// SPEC-011 walkthrough: Revise made every feature unsendable).
func (s *Server) designApproved(ctx context.Context, ownerType string, ownerID uuid.UUID) bool {
	var ok bool
	err := s.Store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM documents
		WHERE type = 'design' AND owner_type = $1 AND owner_id = $2 AND state = 'approved')`,
		ownerType, ownerID).Scan(&ok)
	return err == nil && ok
}

// specReady evaluates G0 for a feature: its own design, or its immediate
// parent initiative's. Only the immediate parent is consulted, so a top-level
// design never releases features buried under half-formed sub-initiatives
// (FR-3.2).
func (s *Server) specReady(ctx context.Context, f *store.Feature) lifecycle.GateResult {
	return lifecycle.G0(
		s.designApproved(ctx, "feature", f.ID),
		s.designApproved(ctx, "initiative", f.InitiativeID),
	)
}

// reconcileFeatureAuthoring restores both invariants for one feature. It is
// the single place the chain advances, called from every trigger, and it is
// safe to call at any time on any feature — which is what lets the heartbeat
// use it as a safety net.
func (s *Server) reconcileFeatureAuthoring(ctx context.Context, featureID uuid.UUID) error {
	return s.restoreAuthoring(ctx, featureID, false)
}

// restoreAuthoring queues whatever restores the invariants. A revision of a
// draft that waits for its author is bounded by the round cap (SPEC-011
// FR-3.3) unless a person allowed another round (force).
func (s *Server) restoreAuthoring(ctx context.Context, featureID uuid.UUID, force bool) error {
	need, err := s.neededAuthoring(ctx, featureID)
	if err != nil || need.Purpose == "" {
		return err
	}
	switch {
	case need.Purpose == "estimate":
		return s.enqueueEstimate(ctx, "feature", featureID)
	case need.Revising != nil:
		return s.queueRevision(ctx, need, force)
	}
	return s.queueAuthoring(ctx, need.Purpose, featureID)
}

// authoringNeed is what the invariants owe a feature: a purpose to dispatch,
// and for a revision, the draft the author is to revise.
type authoringNeed struct {
	Feature  *store.Feature
	Purpose  string // write-spec | write-dev-plan | estimate | ""
	Revising *store.Document
}

// neededAuthoring evaluates the invariants for a feature (SPEC-011 FR-2) and
// names what would restore them, or nothing when they hold (or the feature is
// not in a state they govern).
func (s *Server) neededAuthoring(ctx context.Context, featureID uuid.UUID) (authoringNeed, error) {
	var none authoringNeed
	f, err := store.GetFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		if err == store.ErrNotFound {
			return none, nil
		}
		return none, err
	}
	// A feature being built has its contract; re-authoring one mid-flight is
	// the revision cascade's job, not this one. A ready feature is still
	// governed: an issue on its approved spec reopens it (SD-7), and a
	// feature prepared in chat is sent from ready to be estimated (SD-2).
	if f.State != lifecycle.FeatIdea && f.State != lifecycle.FeatReady {
		return none, nil
	}
	// Nothing runs before Send (SPEC-011 FR-2.1, FR-2.2, SD-2): the
	// invariants need the mark, so a design approval, a new feature or a
	// description re-checks and finds nothing to do for an unsent feature.
	if sent, err := s.featureSent(ctx, f); err != nil || !sent {
		return none, err
	}

	// Invariant 1 — the spec.
	spec, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "spec", "feature", f.ID)
	if err == store.ErrNotFound {
		// An undescribed feature is not dispatched, not checkpointed, and not
		// complained about: in width-first planning a placeholder feature is
		// entirely normal (SPEC-009 FR-4.5). Send refuses it anyway.
		if f.State != lifecycle.FeatIdea || f.Description == "" {
			return none, nil
		}
		if g := s.specReady(ctx, f); !g.Pass {
			return none, nil
		}
		return authoringNeed{Feature: f, Purpose: "write-spec"}, nil
	}
	if err != nil {
		return none, err
	}
	if spec.State == lifecycle.DocDraft {
		if waits, err := s.waitsForAuthor(ctx, spec); err != nil || !waits {
			return none, err
		}
		return authoringNeed{Feature: f, Purpose: "write-spec", Revising: spec}, nil
	}
	if spec.State != lifecycle.DocApproved {
		return none, nil
	}

	// Invariant 2 — the dev-plan, once the spec is approved.
	plan, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "dev_plan", "feature", f.ID)
	if err == store.ErrNotFound {
		return authoringNeed{Feature: f, Purpose: "write-dev-plan"}, nil
	}
	if err != nil {
		return none, err
	}
	if plan.State == lifecycle.DocDraft {
		if waits, err := s.waitsForAuthor(ctx, plan); err != nil || !waits {
			return none, err
		}
		return authoringNeed{Feature: f, Purpose: "write-dev-plan", Revising: plan}, nil
	}
	if plan.State != lifecycle.DocApproved {
		return none, nil
	}

	// The closing estimate (FR-2.2a), owed once per send and per plan.
	if owed, err := s.estimateOwed(ctx, f, plan); err != nil || !owed {
		return none, err
	}
	return authoringNeed{Feature: f, Purpose: "estimate"}, nil
}

// waitsForAuthor reports whether a draft is the author's to revise (SPEC-011
// FR-2.1): it carries an open human issue or a major review finding. A draft
// with nothing against it belongs to whoever is writing it.
func (s *Server) waitsForAuthor(ctx context.Context, d *store.Document) (bool, error) {
	var waits bool
	err := s.Store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM document_comments
		WHERE document_id = $1 AND NOT resolved AND (is_issue OR severity = 'major'))`, d.ID).Scan(&waits)
	return waits, err
}

// estimateOwed: the project assigns an estimator, and no estimate has been
// queued since the later of the send and the plan's approval (FR-2.2a).
func (s *Server) estimateOwed(ctx context.Context, f *store.Feature, plan *store.Document) (bool, error) {
	cfg, err := s.freshConfig()
	if err != nil || cfg.Assignments["estimate"] == "" {
		return false, nil
	}
	send, err := store.GetFeatureSend(ctx, s.Store.Pool, f.ID)
	if err == store.ErrNotFound {
		return false, nil // building features are sent, but not re-estimated here
	}
	if err != nil {
		return false, err
	}
	since := send.SentAt
	if plan.ApprovedAt != nil && plan.ApprovedAt.After(since) {
		since = *plan.ApprovedAt
	}
	prior, err := store.DispatchesForRefSince(ctx, s.Store.Pool, "feature", f.ID, []string{"estimate"}, since)
	return err == nil && len(prior) == 0, err
}

// queueRevision dispatches the author to revise a draft that waits for it
// (FR-3.1), unless the author is already at work, and raises an
// authoring-deadlock question instead once the draft has had
// dispatch.max_review_rounds reviews (FR-3.3).
func (s *Server) queueRevision(ctx context.Context, need authoringNeed, force bool) error {
	doc, f := need.Revising, need.Feature
	if live, err := store.LiveDispatchForRef(ctx, s.Store.Pool, "feature", f.ID, need.Purpose); err != nil || live {
		return err
	}
	if !force {
		cfg, err := s.freshConfig()
		if err != nil {
			return s.configErrorCheckpoint(ctx, "document", doc.ID, err)
		}
		rounds, err := store.CountDispatchesForRef(ctx, s.Store.Pool, "document", doc.ID, "review-"+doc.Type)
		if err != nil {
			return err
		}
		if rounds >= cfg.Dispatch.MaxReviewRounds {
			var cp *store.Checkpoint
			err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
				var e error
				cp, e = store.CreateCheckpoint(ctx, tx, "authoring-deadlock", "document", doc.ID,
					fmt.Sprintf("The %s for %s has been sent back %d times. Allow its author another round, or stop and edit it yourself?",
						docTypeWords(doc.Type), f.Name, rounds),
					map[string]any{"document": doc.Path, "rounds": rounds})
				return e
			})
			if err == nil && cp != nil {
				s.notifyCheckpointRaised(cp)
			}
			if isUniqueViolation(err) {
				return nil // the question is already pending
			}
			return err
		}
	}
	return s.queueAuthoring(ctx, need.Purpose, f.ID)
}

// ReconcileAuthoringSweep is the heartbeat's safety net (FR-4.7): every
// sent, forming feature whose invariants are unsatisfied and whose needed
// work has not been attempted gets its dispatch, so a lost trigger event
// cannot strand a feature. Not-attempted is the boundary on purpose: a
// failed dispatch already has a dispatch-failure checkpoint governing its
// retry, and a sweep that re-enqueued it every thirty seconds would override
// a human's answer to that question. For a revision it reads "not attempted
// since the draft last changed" (SPEC-011 FR-2.4).
func (s *Server) ReconcileAuthoringSweep(ctx context.Context) {
	// Sent features only (SPEC-011 FR-2.4): an unsent feature has no gap to
	// fill, because nothing is owed to it until someone sends it.
	ids, err := store.SentFormingFeatureIDs(ctx, s.Store.Pool)
	if err != nil {
		s.Log.Error("authoring sweep", "err", err)
		return
	}
	for _, id := range ids {
		need, err := s.neededAuthoring(ctx, id)
		if err != nil || need.Purpose == "" {
			continue
		}
		attempted, err := s.attemptedFor(ctx, need)
		if err != nil || attempted {
			continue
		}
		if err := s.restoreAuthoring(ctx, id, false); err != nil {
			s.Log.Error("authoring sweep", "feature", id, "err", err)
		}
	}
}

// attemptedFor reports whether the needed work has already been tried: ever,
// for a first draft; since the draft last changed, for a revision; since the
// send or the plan, for an estimate (which estimateOwed already reads).
func (s *Server) attemptedFor(ctx context.Context, need authoringNeed) (bool, error) {
	if need.Purpose == "estimate" {
		return false, nil
	}
	if need.Revising == nil {
		n, err := store.CountDispatchesForRef(ctx, s.Store.Pool, "feature", need.Feature.ID, need.Purpose)
		return n > 0, err
	}
	var attempted bool
	err := s.Store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM dispatches d, documents doc
		WHERE doc.id = $3 AND d.ref_type = 'feature' AND d.ref_id = $1 AND d.purpose = $2
		  AND d.queued_at >= doc.updated_at)`, need.Feature.ID, need.Purpose, need.Revising.ID).Scan(&attempted)
	return attempted, err
}

// reconcileAuthoringScope expands an owner into the features it releases and
// restores the invariants over each. An initiative reaches only its *direct*
// child features: G0's one-level rule lives here as much as in the gate, so a
// design approved at the top of a tree never specs work under a sub-initiative
// whose own design nobody has approved (FR-3.2).
func (s *Server) reconcileAuthoringScope(ctx context.Context, ownerType string, ownerID uuid.UUID) error {
	switch ownerType {
	case "feature":
		return s.reconcileFeatureAuthoring(ctx, ownerID)
	case "initiative":
		features, err := store.FeaturesForInitiative(ctx, s.Store.Pool, ownerID)
		if err != nil {
			return err
		}
		for i := range features {
			if err := s.reconcileFeatureAuthoring(ctx, features[i].ID); err != nil {
				return err
			}
		}
	}
	// A design owned by the project releases nothing on its own: a project is
	// not an initiative, and its features live below one.
	return nil
}

// featureSent is the one predicate for "has this feature been sent to
// development" (SPEC-011 FR-1.2): it carries the mark, or it is being built.
// Start building is a stronger commitment than Send, so a feature started
// before the mark existed, or with its documents written by hand, still gets
// its spec rewritten when its design is revised mid-build (SD-3). A ready
// feature without the mark is not sent: DESIGN-010 §5 leaves it without a
// spec until someone sends it.
func (s *Server) featureSent(ctx context.Context, f *store.Feature) (bool, error) {
	if f.State == lifecycle.FeatActive || f.State == lifecycle.FeatReview {
		return true, nil
	}
	_, err := store.GetFeatureSend(ctx, s.Store.Pool, f.ID)
	if err == store.ErrNotFound {
		return false, nil
	}
	return err == nil, err
}

// queueAuthoring enqueues an authoring dispatch for a feature. The idempotency
// key counts the dispatches this feature has already had for the purpose, so a
// replayed event — or a heartbeat reconciling the same invariant a second time
// — cannot double-dispatch (FR-4.6).
func (s *Server) queueAuthoring(ctx context.Context, purpose string, featureID uuid.UUID) error {
	cfg, err := s.freshConfig()
	if err != nil {
		return s.configErrorCheckpoint(ctx, "feature", featureID, err)
	}
	role := cfg.Assignments[purpose]
	if role == "" {
		// A project that has not assigned an authoring role has not opted into
		// the chain, and its humans write these documents by hand as they
		// always have. That is a working project, not a broken one — so this
		// is silence, not a checkpoint. Raising one here would mean every
		// project that predates the authoring chain started complaining the
		// moment it upgraded.
		return nil
	}
	model, err := s.modelForPurpose(cfg, purpose, role)
	if err != nil {
		return s.configErrorCheckpoint(ctx, "feature", featureID, err)
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		// Serialise on the feature row, then refuse a second live dispatch
		// of the purpose: an event and the heartbeat sweep can both find the
		// same gap at once, and with the count-based key they would each
		// enqueue one (SPEC-011, found by TestIssueOnASentFeaturesSpec...).
		if _, err := tx.Exec(ctx, `SELECT id FROM features WHERE id = $1 FOR UPDATE`, featureID); err != nil {
			return err
		}
		if live, err := store.LiveDispatchForRef(ctx, tx, "feature", featureID, purpose); err != nil || live {
			return err
		}
		// Nor beside a failed one. Its retry, automatic or answered at its
		// dispatch-failure checkpoint, reuses that row; a re-check event (a
		// replayed send, a description edit) enqueueing a fresh dispatch
		// would go round the person's answer, which the sweep already
		// refuses to do (attemptedFor).
		if failed, err := store.LatestDispatchFailedForRef(ctx, tx, "feature", featureID, purpose); err != nil || failed {
			return err
		}
		n, err := store.CountDispatchesForRef(ctx, tx, "feature", featureID, purpose)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%s:%s:%d", purpose, featureID, n)
		_, err = store.EnqueueDispatch(ctx, tx, purpose, role, model, "feature", featureID, key)
		return err
	})
	if err == nil {
		// Kick after commit, as every enqueue path must — a queued dispatch
		// nobody kicks waits for an unrelated event to happen along.
		s.Dispatcher.Kick()
	}
	return err
}

// ---- The revision cascade (SPEC-009 FR-9) ----

// affectedSpec is one spec a design revision bears on: the feature, its
// current approved spec, and enough display context for the checkpoint.
type affectedSpec struct {
	Feature *store.Feature
	Spec    *store.Document
}

// affectedSpecs returns the current approved specs in a design's G0 scope —
// the same one-level expansion the reconciler uses. A spec still in draft or
// reviewing is deliberately not affected (SPEC-009 §6 open question 2, decided
// here): it has no downstream derivations to invalidate, and its pending
// review reads against the revised design — FR-8's coverage bar is its
// correction mechanism.
func (s *Server) affectedSpecs(ctx context.Context, ownerType string, ownerID uuid.UUID) ([]affectedSpec, error) {
	var features []store.Feature
	switch ownerType {
	case "feature":
		f, err := store.GetFeature(ctx, s.Store.Pool, ownerID)
		if err != nil {
			if err == store.ErrNotFound {
				return nil, nil
			}
			return nil, err
		}
		features = []store.Feature{*f}
	case "initiative":
		var err error
		features, err = store.FeaturesForInitiative(ctx, s.Store.Pool, ownerID)
		if err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}
	var out []affectedSpec
	for i := range features {
		f := features[i]
		spec, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "spec", "feature", f.ID)
		if err == store.ErrNotFound {
			continue
		}
		if err != nil {
			return nil, err
		}
		if spec.State == lifecycle.DocApproved {
			out = append(out, affectedSpec{Feature: &features[i], Spec: spec})
		}
	}
	return out, nil
}

// cascadeDesignRevision runs after a successor design is approved. One
// affected spec is invalidated without a question (FR-9.1); two or more raise
// a single design-revision checkpoint listing every one (FR-9.2). The count
// is of specs, never designs (FR-9.2b).
func (s *Server) cascadeDesignRevision(ctx context.Context, a rules.CascadeDesignRevision) error {
	affected, err := s.affectedSpecs(ctx, a.OwnerType, a.OwnerID)
	if err != nil {
		return err
	}
	switch len(affected) {
	case 0:
		return nil
	case 1:
		return s.invalidateSpecForRevision(ctx, affected[0], a.DesignDocID, "orchestrator")
	}

	ownerName := a.OwnerType
	if a.OwnerType == "initiative" {
		if path, err := s.initiativePath(ctx, a.OwnerID); err == nil {
			ownerName = path
		}
	} else if a.OwnerType == "feature" {
		if f, err := store.GetFeature(ctx, s.Store.Pool, a.OwnerID); err == nil {
			ownerName = f.Name
		}
	}
	list := make([]map[string]any, 0, len(affected))
	for _, af := range affected {
		list = append(list, map[string]any{
			"spec_doc_id":   af.Spec.ID.String(),
			"spec_path":     af.Spec.Path,
			"feature_id":    af.Feature.ID.String(),
			"feature_name":  af.Feature.Name,
			"feature_state": string(af.Feature.State),
		})
	}
	var cp *store.Checkpoint
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		cp, e = store.CreateCheckpoint(ctx, tx, "design-revision", "document", a.DesignDocID,
			fmt.Sprintf("The design for %s has been revised. Decide for each specification below whether it still stands: keep it, or invalidate it so a fresh one is written against the revised design.", ownerName),
			map[string]any{"design_doc_id": a.DesignDocID.String(), "affected": list})
		return e
	})
	if err == nil && cp != nil {
		s.notifyCheckpointRaised(cp)
	}
	return err
}

// applyDesignRevision executes an answered design-revision checkpoint:
// invalidate the named specs; the kept ones are already recorded on the
// checkpoint's answer (FR-9.5). Already-superseded specs are skipped so a
// replayed answer is harmless.
func (s *Server) applyDesignRevision(ctx context.Context, a rules.ApplyDesignRevision) error {
	for _, specID := range a.Invalidate {
		spec, err := store.GetDocument(ctx, s.Store.Pool, specID)
		if err != nil {
			if err == store.ErrNotFound {
				continue
			}
			return err
		}
		if spec.State != lifecycle.DocApproved || spec.OwnerType != "feature" || spec.OwnerID == nil {
			continue
		}
		f, err := store.GetFeature(ctx, s.Store.Pool, *spec.OwnerID)
		if err != nil {
			if err == store.ErrNotFound {
				continue
			}
			return err
		}
		if err := s.invalidateSpecForRevision(ctx, affectedSpec{Feature: f, Spec: spec}, a.DesignDocID, a.Actor); err != nil {
			return err
		}
	}
	return nil
}

// invalidateSpecForRevision makes one feature's spec no longer current, in the
// way its state allows:
//
// A forming (idea) feature takes the mechanical path (FR-9.4, FR-9.4a): the
// spec and its approved dev-plan are superseded and their files archived in
// one transaction and one server-authored commit. Superseding precedes the
// replacement deliberately — the reverse of approval-time takeover (SPEC-009
// §6 open question 1) — which is what lets the reconciler author the fresh
// spec. A dev-plan still in draft or reviewing cannot legally supersede and is
// left alone: once the replacement spec is approved its review continues
// against it.
//
// An in-flight (active or review) feature must not lose its current spec —
// work is running against it. Instead the invalidation opens a successor
// draft, exactly as a human revision would, and dispatches write-spec to fill
// it. Submitting that successor fires the existing MarkRevisionInFlight path
// unchanged (FR-9.6): two checkpoints in sequence, because they are two
// different decisions.
func (s *Server) invalidateSpecForRevision(ctx context.Context, af affectedSpec, designDocID uuid.UUID, actor string) error {
	f, spec := af.Feature, af.Spec

	if f.State == lifecycle.FeatActive || f.State == lifecycle.FeatReview {
		if _, err := s.ReviseDoc(ctx, spec.Path, actor); err != nil {
			return err
		}
		err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.Audit(ctx, tx, actor, "design.revision_cascade", "document", &spec.ID,
				map[string]any{"design": designDocID.String(), "feature": f.ID.String(), "route": "successor"})
		})
		if err != nil {
			return err
		}
		return s.queueAuthoring(ctx, "write-spec", f.ID)
	}

	// The mechanical path. Archive targets are computed first so the row
	// updates and the file moves agree.
	var moves []archivedMove

	devPlan, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "dev_plan", "feature", f.ID)
	if err != nil && err != store.ErrNotFound {
		return err
	}
	hasDevPlan := err == nil && devPlan.State == lifecycle.DocApproved

	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		payload := map[string]any{"cause": "design-revision", "design": designDocID.String()}
		if err := store.TransitionDocument(ctx, tx, spec, lifecycle.DocSupersede, "lifecycle-engine", payload); err != nil {
			return err
		}
		to := archiveTarget(spec)
		moves = append(moves, archivedMove{spec.Path, to})
		if err := store.UpdateDocumentPath(ctx, tx, spec.ID, to); err != nil {
			return err
		}
		if hasDevPlan {
			// A dev-plan is a decomposition of one spec; when that spec goes,
			// it goes — 1:1 and mechanical, no checkpoint (FR-9.4a).
			if err := store.TransitionDocument(ctx, tx, devPlan, lifecycle.DocSupersede, "lifecycle-engine",
				map[string]any{"cause": "design-revision", "spec": spec.ID.String()}); err != nil {
				return err
			}
			to := archiveTarget(devPlan)
			moves = append(moves, archivedMove{devPlan.Path, to})
			if err := store.UpdateDocumentPath(ctx, tx, devPlan.ID, to); err != nil {
				return err
			}
		}
		if err := store.Audit(ctx, tx, actor, "design.revision_cascade", "document", &spec.ID,
			map[string]any{"design": designDocID.String(), "feature": f.ID.String(), "route": "supersede",
				"dev_plan_superseded": hasDevPlan}); err != nil {
			return err
		}
		// A ready feature has just lost its contract; it is an idea again,
		// which is what lets the authoring invariant write it a fresh spec.
		// Its tasks are kept — decomposition reconciles by local id when the
		// replacement dev-plan arrives, and work that happened is never
		// erased silently.
		if f.State == lifecycle.FeatReady {
			return store.TransitionFeature(ctx, tx, f, lifecycle.FeatContractInvalidated, "lifecycle-engine",
				map[string]any{"cause": "design-revision", "design": designDocID.String()})
		}
		return nil
	})
	if err != nil {
		return err
	}

	if err := s.archiveInvalidatedFiles(moves); err != nil {
		// Rows and files have diverged; surface loudly, as approval-time
		// takeover does.
		s.Log.Error("revision cascade file archive failed", "spec", spec.ID, "err", err)
		var cp *store.Checkpoint
		if e := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			var cerr error
			cp, cerr = store.CreateCheckpoint(ctx, tx, "document-integrity", "document", spec.ID,
				"An invalidated specification was superseded but its file could not be archived; repo files need manual reconciliation.",
				map[string]any{"error": err.Error()})
			return cerr
		}); e == nil {
			s.notifyCheckpointRaised(cp)
		}
	}
	return nil
}

// archivedMove is one file's journey to docs/_superseded/.
type archivedMove struct{ from, to string }

// archiveInvalidatedFiles moves invalidated documents under docs/_superseded/
// in one server-authored commit. The short-id suffix keeps two features'
// authored spec.md files from colliding there.
func (s *Server) archiveInvalidatedFiles(moves []archivedMove) error {
	if len(moves) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Join(s.RepoRoot, "docs/_superseded"), 0o755); err != nil {
		return err
	}
	for _, m := range moves {
		if _, err := gitIn(s.RepoRoot, "mv", m.from, m.to); err != nil {
			return fmt.Errorf("git mv %s %s: %w", m.from, m.to, err)
		}
	}
	if _, err := gitIn(s.RepoRoot, "commit", "-m", "subutai: design revision invalidated documents; archived",
		"--author", "subutai <subutai@localhost>"); err != nil {
		return fmt.Errorf("archive commit: %w", err)
	}
	return nil
}

// pendingDesignRevisionFor reports whether a pending design-revision
// checkpoint lists the feature among its affected specs — a filter over
// PendingCheckpoints, as openReviewCheckpoint is.
func (s *Server) pendingDesignRevisionFor(ctx context.Context, featureID uuid.UUID) (bool, error) {
	pending, err := s.Store.PendingCheckpoints(ctx)
	if err != nil {
		return false, err
	}
	want := featureID.String()
	for _, cp := range pending {
		if cp.Kind != "design-revision" {
			continue
		}
		var cctx struct {
			Affected []struct {
				FeatureID string `json:"feature_id"`
			} `json:"affected"`
		}
		if err := json.Unmarshal(cp.Context, &cctx); err != nil {
			continue
		}
		for _, a := range cctx.Affected {
			if a.FeatureID == want {
				return true, nil
			}
		}
	}
	return false, nil
}

// AuthoredDocument is the payload of a submit_document outcome.
type AuthoredDocument struct {
	Body      string `json:"body"`
	Reasoning string `json:"reasoning"`
}

// validateAuthoredDocument returns the dispatch's outcome validator, closing
// over the manifest so a structurally-wrong document is rejected *within the
// agent's turn* and it can correct and resubmit (FR-6.4). Rejecting after the
// dispatch instead would mean a whole new dispatch to fix a missing heading.
//
// Links are not checked here: the file does not exist yet, so a link checker
// would fail every relative reference. They are checked on submission, once
// the document is on disk where its neighbours are.
func validateAuthoredDocument(m *config.Manifest) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		var out AuthoredDocument
		if err := json.Unmarshal(raw, &out); err != nil {
			return fmt.Errorf("document outcome: %w", err)
		}
		if strings.TrimSpace(out.Body) == "" {
			return fmt.Errorf("document outcome: the body is empty — submit the whole file, front matter included")
		}
		report := lifecycle.Validate(m, out.Body, func(string) bool { return true })
		if report.Valid {
			return nil
		}
		var b strings.Builder
		b.WriteString("the document does not match the required structure:\n")
		for _, iss := range report.Issues {
			fmt.Fprintf(&b, "- %s: %s\n", iss.Check, iss.Detail)
		}
		b.WriteString("Fix these and call submit_document again.")
		return errors.New(b.String())
	}
}

// fileAuthoredDocument writes an authored document into the repository,
// registers it, and submits it for review — one path, so a document can never
// exist on disk without a row, or as a row pointing at nothing.
//
// The agent chose none of this: not the path, not the owner, not the moment it
// is committed. That is the standing convention for implicit context (vision
// §8), and it is why submit_document takes a body and nothing else.
func (s *Server) fileAuthoredDocument(ctx context.Context, featureID uuid.UUID, docType, body, actor string) error {
	f, err := store.GetFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return err
	}

	// If the feature already holds a current draft of this type, the dispatch
	// exists to fill it: the revision cascade opens a successor draft for an
	// in-flight feature (FR-9.6) and the authored body belongs in that
	// document, at its path, keeping the supersedes link that lets
	// MarkRevisionInFlight and approval-time takeover run unchanged. A current
	// document in any other state means the invariant should never have
	// dispatched; refuse rather than plant a second live spec.
	path := ""
	register := true
	if cur, err := store.CurrentDocForOwner(ctx, s.Store.Pool, docType, "feature", f.ID); err == nil {
		if cur.State != lifecycle.DocDraft {
			return fmt.Errorf("the feature already has a current %s in %s; nothing to author", docType, cur.State)
		}
		path = cur.Path
		register = false
	} else if err != store.ErrNotFound {
		return err
	}
	if register {
		if path, err = s.authoredDocPath(ctx, f, docType); err != nil {
			return err
		}
	}

	abs := filepath.Join(s.RepoRoot, path)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return err
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		return err
	}
	if register {
		if _, err := s.RegisterDocForOwner(ctx, path, docType, "feature", &f.ID, actor); err != nil {
			return err
		}
	}
	// Committing is for durability and history, not for detection: the git
	// hook only ever sees commits, and an agent's write is uncommitted at the
	// moment it happens, so the notification comes from SubmitDoc's own
	// transition rather than from git (FR-7.2).
	s.commitDocument(path, fmt.Sprintf("subutai: %s authored for %s", docType, f.Slug))
	_, _, err = s.SubmitDoc(ctx, path, actor)
	return err
}

// authoredDocPath is where a document of a type lives for a feature. Specs and
// dev-plans sit beside the design they translate, under a docs/ tree keyed by
// the initiative path, so a person browsing the repository finds a feature's
// papers together.
func (s *Server) authoredDocPath(ctx context.Context, f *store.Feature, docType string) (string, error) {
	initPath, err := s.initiativePath(ctx, f.InitiativeID)
	if err != nil {
		return "", err
	}
	// An author never writes over a file it didn't register — a detached
	// draft left on disk, say (SPEC-011 FR-9.3): the new document goes beside
	// it with a numbered name.
	base := filepath.Join("docs", initPath, f.Slug, docType)
	path := base + ".md"
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(s.RepoRoot, path)); os.IsNotExist(err) {
			return path, nil
		}
		path = fmt.Sprintf("%s-%d.md", base, n)
	}
}

// commitDocument commits one path with subutai as the author. A failure is
// logged rather than raised: the document is registered and submitted either
// way, and a repository that cannot be committed to is a problem for a person,
// not a reason to lose the work.
func (s *Server) commitDocument(path, message string) {
	if _, err := gitIn(s.RepoRoot, "add", "--", path); err != nil {
		s.Log.Warn("could not stage authored document", "path", path, "err", err)
		return
	}
	if _, err := gitIn(s.RepoRoot, "commit", "-m", message, "--author", "subutai <subutai@localhost>", "--", path); err != nil {
		s.Log.Warn("could not commit authored document", "path", path, "err", err)
	}
}
