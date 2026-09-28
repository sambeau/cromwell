package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/config"
	"subutai/internal/content"
	"subutai/internal/lifecycle"
	"subutai/internal/rules"
	"subutai/internal/store"
)

// handle fetches the snapshot for an event, runs the rule engine, and
// executes the resulting actions in order.
func (s *Server) handle(ctx context.Context, ev bus.Event) error {
	snap, err := s.snapshot(ctx, ev)
	if err != nil {
		return fmt.Errorf("snapshot: %w", err)
	}
	for _, action := range rules.Decide(ev, snap) {
		if err := s.execute(ctx, action); err != nil {
			return fmt.Errorf("action %s: %w", action.ActionKind(), err)
		}
	}
	return nil
}

// snapshot loads exactly the state the rules may consult for this event.
func (s *Server) snapshot(ctx context.Context, ev bus.Event) (rules.Snapshot, error) {
	var snap rules.Snapshot
	loadDoc := func(id uuid.UUID) error {
		doc, err := store.GetDocument(ctx, s.Store.Pool, id)
		if err != nil {
			return err
		}
		snap.Doc = s.docSnap(doc)
		// The review loop's inputs (SPEC-011): whether an agent approval
		// would be held, and which human issues are open.
		snap.Doc.Held = s.specHeld(ctx, doc)
		if snap.Doc.OpenIssues, err = s.openIssueIDs(ctx, doc.ID); err != nil {
			return err
		}
		if doc.OwnerType == "feature" && doc.OwnerID != nil {
			f, err := store.GetFeature(ctx, s.Store.Pool, *doc.OwnerID)
			if err == nil {
				snap.OwnerFeature = &rules.FeatureSnap{ID: f.ID, State: f.State}
			} else if err != store.ErrNotFound {
				return err
			}
		}
		return nil
	}

	loadTask := func(id uuid.UUID) error {
		t, err := store.GetTask(ctx, s.Store.Pool, id)
		if err != nil {
			return err
		}
		// Completed code reviews, including the one whose outcome is being
		// decided: on the Nth request_changes the count is N, so a cap of 3
		// gives the implementer two revisions and stops on the third
		// rejection (audit §3.3a).
		rounds, err := store.CountDispatchesForRef(ctx, s.Store.Pool, "task", t.ID, "review-code")
		if err != nil {
			return err
		}
		roundCap := 0
		if cfg, cerr := s.freshConfig(); cerr == nil {
			roundCap = cfg.Dispatch.MaxReviewRounds
		}
		snap.Task = &rules.TaskSnap{
			ID: t.ID, FeatureID: t.FeatureID, State: t.State,
			ReviewRounds: rounds, ReviewRoundCap: roundCap,
		}
		return nil
	}
	loadFeature := func(id uuid.UUID) error {
		f, err := store.GetFeature(ctx, s.Store.Pool, id)
		if err != nil {
			return err
		}
		snap.RefFeature = &rules.FeatureSnap{ID: f.ID, State: f.State}
		return nil
	}

	switch e := ev.(type) {
	case bus.DocumentTransitioned:
		return snap, ignoreNotFound(loadDoc(e.DocID))
	case bus.DispatchSucceeded:
		switch e.RefType {
		case "document":
			if err := ignoreNotFound(loadDoc(e.RefID)); err != nil || snap.Doc == nil {
				return snap, err
			}
			// A verdict applies only to the content it reviewed (SPEC-011
			// FR-3.5): review keys carry the content hash they were queued at.
			if d, err := store.GetDispatch(ctx, s.Store.Pool, e.DispatchID); err == nil &&
				strings.HasPrefix(d.IdempotencyKey, "review:") &&
				!strings.Contains(d.IdempotencyKey, ":"+snap.Doc.ContentHash) {
				snap.Doc.StaleVerdict = true
			}
			return snap, nil
		case "task":
			return snap, ignoreNotFound(loadTask(e.RefID))
		case "feature":
			return snap, ignoreNotFound(loadFeature(e.RefID))
		}
	case bus.DocumentFileChanged:
		doc, err := store.LiveDocumentByPath(ctx, s.Store.Pool, e.Path)
		if err == store.ErrNotFound {
			return snap, nil
		}
		if err != nil {
			return snap, err
		}
		snap.Doc = s.docSnap(doc)
	case bus.CheckpointResponded:
		switch e.RefType {
		case "document":
			return snap, ignoreNotFound(loadDoc(e.RefID))
		case "task":
			return snap, ignoreNotFound(loadTask(e.RefID))
		case "feature":
			return snap, ignoreNotFound(loadFeature(e.RefID))
		}
	}
	return snap, nil
}

func ignoreNotFound(err error) error {
	if err == store.ErrNotFound {
		return nil
	}
	return err
}

func (s *Server) docSnap(d *store.Document) *rules.DocSnap {
	snap := &rules.DocSnap{
		ID: d.ID, Type: d.Type, State: d.State,
		ContentHash: d.ContentHash, OwnerType: d.OwnerType, Path: d.Path,
		IsSuccessor:   d.SupersedesID != nil,
		HumanApproval: s.humanApprovalType(d.Type),
	}
	if d.OwnerID != nil {
		snap.OwnerID = *d.OwnerID
	}
	return snap
}

// humanApprovalType reports whether a document type's approval belongs to a
// person (SPEC-009 FR-2.1). A type with no manifest defaults to agent
// approval, which is what every type had before the field existed.
func (s *Server) humanApprovalType(docType string) bool {
	m, err := config.LoadManifest(s.CompartmentRoot, docType)
	return err == nil && m.HumanApproval()
}

// execute runs one action. Each case is transactional with its audit rows
// (O-3) and publishes follow-up events after commit.
func (s *Server) execute(ctx context.Context, action rules.Action) error {
	switch a := action.(type) {
	case rules.QueueReview:
		return s.queueReview(ctx, a)
	case rules.ApproveDocument:
		return s.approveDocument(ctx, a)
	case rules.ReturnForChanges:
		return s.returnForChanges(ctx, a)
	case rules.RecordReviewComments:
		return s.recordReviewComments(ctx, a)
	case rules.RaiseCheckpoint:
		var cp *store.Checkpoint
		err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			var e error
			cp, e = store.CreateCheckpoint(ctx, tx, a.CPKind, a.RefType, a.RefID, a.Question, a.Context)
			return e
		})
		if err == nil {
			s.notifyCheckpointRaised(cp)
		}
		return err
	case rules.EvaluateContractGate:
		return s.evaluateContractGate(ctx, a.FeatureID)
	case rules.ReconcileAuthoring:
		return s.reconcileAuthoringScope(ctx, a.OwnerType, a.OwnerID)
	case rules.CascadeDesignRevision:
		return s.cascadeDesignRevision(ctx, a)
	case rules.ApplyDesignRevision:
		return s.applyDesignRevision(ctx, a)
	case rules.ReindexDocument:
		return s.reindexDocument(ctx, a.DocID)
	case rules.ReviseAuthoredDocument:
		return s.reviseAuthoredDocument(ctx, a)
	case rules.HoldDocument:
		return s.holdDocument(ctx, a)
	case rules.AnswerIssues:
		return s.answerIssues(ctx, a)
	case rules.EstimateFeature:
		return s.estimateFeature(ctx, a.FeatureID)
	case rules.ArchiveInitiative:
		return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.ArchiveInitiative(ctx, tx, a.InitiativeID, a.Actor, a.Reason)
		})
	case rules.RetryDispatch:
		err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.RequeueDispatch(ctx, tx, a.DispatchID)
		})
		if err == nil {
			s.Dispatcher.Kick()
		}
		return err
	case rules.CancelDispatch:
		return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.MarkDispatchCancelled(ctx, tx, a.DispatchID, a.Actor)
		})
	case rules.KickQueue:
		s.Dispatcher.Kick()
		return nil
	}
	// Phase-2 implementation-loop actions.
	if handled, err := s.executePhase2(ctx, action); handled {
		return err
	}
	// Phase-3 planning actions.
	if handled, err := s.executePhase3(ctx, action); handled {
		return err
	}
	return fmt.Errorf("unknown action %T", action)
}

// queueReview resolves the reviewer role from the document type's manifest
// (F-4), the model from role + routing override, and enqueues idempotently.
func (s *Server) queueReview(ctx context.Context, a rules.QueueReview) error {
	cfg, err := s.freshConfig()
	if err != nil {
		return s.configErrorCheckpoint(ctx, "document", a.DocID, err)
	}
	manifest, err := config.LoadManifest(s.CompartmentRoot, a.DocType)
	if err != nil {
		return s.configErrorCheckpoint(ctx, "document", a.DocID, err)
	}
	// A type with no reviewer — a design, now its reviewer is retired
	// (SPEC-011 FR-10.2) — waits in reviewing for a person.
	if manifest.ReviewerRole == "" {
		return nil
	}
	// With agent spec review switched off, a submitted spec is held for a
	// person rather than reviewed (FR-5.3). A person's request is the one
	// exception: a one-off review, whose approval is held again.
	if a.DocType == "spec" && !a.Fresh && !cfg.AgentSpecReview() {
		err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.SetDocumentHold(ctx, tx, a.DocID, nil, "orchestrator",
				"agent review is switched off for this project, so the spec waits for a person")
		})
		if err == nil {
			s.notifyEntityChanged("document", a.DocID)
		}
		return err
	}
	key, err := s.reviewKey(ctx, a)
	if err != nil {
		return err
	}
	role, err := config.LoadRole(s.CompartmentRoot, manifest.ReviewerRole)
	if err != nil {
		return s.configErrorCheckpoint(ctx, "document", a.DocID, err)
	}
	purpose := "review-" + a.DocType
	model := role.Model
	if override, ok := cfg.Routing[purpose]; ok {
		model = override
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.EnqueueDispatch(ctx, tx, purpose, role.Name, model, "document", a.DocID, key)
		return err
	})
	if err == nil {
		s.Dispatcher.Kick()
	}
	return err
}

// configErrorCheckpoint implements F-8: deterministic config failures skip
// the retry policy and surface as config-error checkpoints naming the file.
func (s *Server) configErrorCheckpoint(ctx context.Context, refType string, refID uuid.UUID, cause error) error {
	var cp *store.Checkpoint
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		cp, e = store.CreateCheckpoint(ctx, tx, "config-error", refType, refID,
			"A .subutai/ configuration error is blocking dispatch. Fix the file and respond to resume.",
			map[string]any{"error": cause.Error()})
		return e
	})
	if err == nil {
		s.notifyCheckpointRaised(cp)
	}
	return err
}

// approveDocument executes the approve transition; if the document revises a
// predecessor, the same transaction supersedes it (DESIGN-003 §5) and the
// lifecycle engine performs the repo file operations in a server-authored
// commit. Findings that rode along on the approval — a reviewer's minors, a
// human's remarks — join the comment thread in the same transaction rather
// than being discarded (C-1a).
func (s *Server) approveDocument(ctx context.Context, a rules.ApproveDocument) error {
	// From the rules engine an approval is an agent's, with its run, or a
	// person's answer to a review escalation (SPEC-017 FR-2.3).
	v := store.Verdict{DocumentID: a.DocID, Verdict: store.VerdictApprove, Kind: store.GiverPerson,
		Actor: a.Actor, Via: "escalation", DispatchID: s.escalatedReview(ctx, a.DocID)}
	if a.DispatchID != nil {
		v = s.agentVerdict(ctx, a.DocID, store.VerdictApprove, a.Actor, *a.DispatchID)
	}
	return s.approveDocumentAs(ctx, a, v)
}

// approveDocumentAs is approveDocument with the verdict it records.
func (s *Server) approveDocumentAs(ctx context.Context, a rules.ApproveDocument, v store.Verdict) error {
	docID, actor := a.DocID, a.Actor
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return err
	}
	var canonicalPath string
	var predecessor *store.Document
	if doc.SupersedesID != nil {
		predecessor, err = store.GetDocument(ctx, s.Store.Pool, *doc.SupersedesID)
		if err != nil {
			return err
		}
		canonicalPath = predecessor.Path
	}

	from := doc.State
	var archived string
	var planMoves []archivedMove
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.TransitionDocument(ctx, tx, doc, lifecycle.DocApprove, actor, verdictPayload(v)); err != nil {
			return err
		}
		if err := store.RecordVerdict(ctx, tx, v); err != nil {
			return err
		}
		if err := store.ClearDocumentHold(ctx, tx, doc.ID); err != nil {
			return err
		}
		// An approval with no agent review behind it is a person's — directly,
		// or on an escalation — and settles the open human issues by that
		// decision (SPEC-011 SD-6).
		if a.DispatchID == nil {
			n, err := store.SettleIssuesByApproval(ctx, tx, doc.ID, actor)
			if err != nil {
				return err
			}
			if n > 0 {
				if err := store.Audit(ctx, tx, actor, "document.issues_settled", "document", &doc.ID,
					map[string]any{"issues": n}); err != nil {
					return err
				}
			}
		}
		for _, c := range a.Comments {
			if err := store.InsertComment(ctx, tx, doc.ID, a.DispatchID, actor, c.SectionRef, c.Body, c.Severity); err != nil {
				return err
			}
		}
		if predecessor != nil {
			// Supersede atomically with the successor's approval (L-2).
			if err := store.TransitionDocument(ctx, tx, predecessor, lifecycle.DocSupersede, "lifecycle-engine",
				map[string]any{"superseded_by": doc.ID.String()}); err != nil {
				return err
			}
			// The archive name carries the superseded row's short id: a
			// document revised twice — or two documents sharing a basename —
			// must not collide in docs/_superseded/.
			archived = s.archiveTarget(predecessor)
			if err := store.UpdateDocumentPath(ctx, tx, predecessor.ID, archived); err != nil {
				return err
			}
			if err := store.UpdateDocumentPath(ctx, tx, doc.ID, canonicalPath); err != nil {
				return err
			}
			// A revised spec takes its plan with the old one (SPEC-011
			// FR-6.6): the plan decomposed a contract that no longer stands.
			var e error
			if planMoves, e = s.supersedePlanWithSpec(ctx, tx, doc); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if predecessor != nil {
		if err := s.takeOverCanonicalPath(doc.Path, canonicalPath, archived); err != nil {
			// Files and states have diverged; the catch-up scan will keep
			// flagging it. Surface loudly.
			s.Log.Error("revision file takeover failed", "doc", doc.ID, "err", err)
			var cp *store.Checkpoint
			if e := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
				var cerr error
				cp, cerr = store.CreateCheckpoint(ctx, tx, "document-integrity", "document", doc.ID,
					"Revision approved but the file takeover commit failed; repo files need manual reconciliation.",
					map[string]any{"error": err.Error()})
				return cerr
			}); e == nil {
				s.notifyCheckpointRaised(cp)
			}
		}
		doc.Path = canonicalPath
	}
	if err := s.archiveInvalidatedFiles(planMoves); err != nil {
		s.Log.Error("superseded plan archive failed", "doc", doc.ID, "err", err)
	}

	s.Bus.Publish(bus.DocumentTransitioned{
		DocID: doc.ID, From: from, To: lifecycle.DocApproved,
		Event: lifecycle.DocApprove, Actor: actor,
	})
	return nil
}

// takeOverCanonicalPath performs the revision file operations in one
// server-authored commit (DESIGN-003 §5): predecessor archived under
// docs/_superseded/, successor moved to the canonical path.
func (s *Server) takeOverCanonicalPath(successorPath, canonicalPath, archived string) error {
	archiveDir := filepath.Join(s.RepoRoot, "docs/_superseded")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	steps := [][]string{
		{"git", "mv", canonicalPath, archived},
		{"git", "mv", successorPath, canonicalPath},
		{"git", "commit", "-m",
			fmt.Sprintf("subutai: revision of %s approved; predecessor archived", canonicalPath),
			"--author", "subutai <subutai@localhost>"},
	}
	for _, argv := range steps {
		cmd := exec.Command(argv[0], argv[1:]...)
		cmd.Dir = s.RepoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			status := exec.Command("git", "status", "--short")
			status.Dir = s.RepoRoot
			st, _ := status.CombinedOutput()
			return fmt.Errorf("%s: %v: %s (git status: %s)", strings.Join(argv, " "), err, out, st)
		}
	}
	return nil
}

func (s *Server) returnForChanges(ctx context.Context, a rules.ReturnForChanges) error {
	v := store.Verdict{DocumentID: a.DocID, Verdict: store.VerdictSendBack, Kind: store.GiverPerson,
		Actor: a.Actor, Via: "escalation", DispatchID: s.escalatedReview(ctx, a.DocID)}
	if a.DispatchID != nil {
		v = s.agentVerdict(ctx, a.DocID, store.VerdictSendBack, a.Actor, *a.DispatchID)
	}
	return s.returnForChangesAs(ctx, a, v)
}

// returnForChangesAs is returnForChanges with the verdict it records.
func (s *Server) returnForChangesAs(ctx context.Context, a rules.ReturnForChanges, v store.Verdict) error {
	doc, err := store.GetDocument(ctx, s.Store.Pool, a.DocID)
	if err != nil {
		return err
	}
	from := doc.State
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		payload := verdictPayload(v)
		payload["comments"] = len(a.Comments)
		if err := store.TransitionDocument(ctx, tx, doc, lifecycle.DocRequestChanges, a.Actor, payload); err != nil {
			return err
		}
		if err := store.RecordVerdict(ctx, tx, v); err != nil {
			return err
		}
		if err := store.ClearDocumentHold(ctx, tx, doc.ID); err != nil {
			return err
		}
		for _, c := range a.Comments {
			// A person sending a spec or plan back — on an escalation — raises
			// an issue: their objection must be addressed (SPEC-011 SD-15).
			if a.DispatchID == nil && (doc.Type == "spec" || doc.Type == "dev_plan") {
				if _, err := store.InsertIssue(ctx, tx, doc.ID, a.Actor, c.SectionRef, c.Body, "ui", ""); err != nil {
					return err
				}
				continue
			}
			// An agent finding is stored at its effective severity — empty
			// reads as major, the fail-closed rule — while a human's comment
			// (no dispatch) is not a classified finding and stays unmarked.
			severity := c.Severity
			if a.DispatchID != nil && severity == "" {
				severity = rules.SeverityMajor
			}
			if err := store.InsertComment(ctx, tx, doc.ID, a.DispatchID, a.Actor, c.SectionRef, c.Body, severity); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.Bus.Publish(bus.DocumentTransitioned{
		DocID: doc.ID, From: from, To: lifecycle.DocDraft,
		Event: lifecycle.DocRequestChanges, Actor: a.Actor,
	})
	return nil
}

// recordReviewComments files a comments-only review outcome (SPEC-009
// FR-2.2). The reviewer's overall reasoning leads the thread — for a clean
// document it is the whole of the review, "what I checked and that it holds" —
// followed by its per-section comments. The document's state is deliberately
// not touched: for a human-approved type the reviewer joins the discussion and
// a person closes it.
func (s *Server) recordReviewComments(ctx context.Context, a rules.RecordReviewComments) error {
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if a.Reasoning != "" {
			if err := store.InsertComment(ctx, tx, a.DocID, a.DispatchID, a.Actor, "", a.Reasoning, ""); err != nil {
				return err
			}
		}
		for _, c := range a.Comments {
			// A design reviewer's comments gate nothing, so they carry no
			// severity (C-1a): classification belongs to findings that decide
			// whether work goes back.
			if err := store.InsertComment(ctx, tx, a.DocID, a.DispatchID, a.Actor, c.SectionRef, c.Body, ""); err != nil {
				return err
			}
		}
		return store.Audit(ctx, tx, a.Actor, "document.commented", "document", &a.DocID,
			map[string]any{"comments": len(a.Comments)})
	})
}

// escalatedReview is the review run a person's answer to an escalation rules
// on: the one named by the document's latest answered review-escalation
// question (SPEC-017 FR-2.3, M6 follow-up 2). Nil when there is none.
func (s *Server) escalatedReview(ctx context.Context, docID uuid.UUID) *uuid.UUID {
	var raw *string
	err := s.Store.Pool.QueryRow(ctx, `SELECT context->>'dispatch_id' FROM checkpoints
		WHERE kind = 'review-escalation' AND ref_type = 'document' AND ref_id = $1 AND state = 'answered'
		ORDER BY answered_at DESC NULLS LAST LIMIT 1`, docID).Scan(&raw)
	if err != nil || raw == nil {
		return nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil
	}
	return &id
}

// verdictPayload is what a verdict adds to its transition's audit row: the
// review run behind an agent's (M6 follow-up 2), or how a person's came, so
// the timeline can tell a relayed verdict from the chat agent's own acts
// (SPEC-017 FR-2.7).
func verdictPayload(v store.Verdict) map[string]any {
	p := map[string]any{"verdict_by": v.Kind}
	if v.DispatchID != nil {
		p["dispatch_id"] = v.DispatchID.String()
	}
	if v.Via != "" {
		p["via"] = v.Via
	}
	return p
}

// HumanApproveDocument is the human approve action (SPEC-009 FR-2.3): the
// single place a person moves a human-approved document out of reviewing. The
// authority gate is here rather than in the handler so no surface can reach
// the transition without it; everything beneath is the same audited,
// transactional path an agent verdict uses, and the transition table itself
// rejects any state but reviewing — there is no force path.
func (s *Server) HumanApproveDocument(ctx context.Context, docID uuid.UUID, actor string) error {
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return err
	}
	if !s.humanApprovalType(doc.Type) {
		return fmt.Errorf("a %s is decided by its agent reviewer; it comes to you only when the reviewer escalates", doc.Type)
	}
	return s.approveDocumentAs(ctx, rules.ApproveDocument{DocID: docID, Actor: actor},
		personVerdict(docID, store.VerdictApprove, relayAct{Actor: actor, Via: "ui"}))
}

// HumanReturnDocument is the request-changes half of FR-2.3: the document goes
// back to draft with the person's reason attached as a comment for its author.
func (s *Server) HumanReturnDocument(ctx context.Context, docID uuid.UUID, actor, reason string) error {
	return s.humanReturnDocument(ctx, docID, reason, relayAct{Actor: actor, Via: "ui"})
}

// humanReturnDocument is HumanReturnDocument for a person's act from either
// surface, recorded as their verdict (SPEC-017 FR-2.3).
func (s *Server) humanReturnDocument(ctx context.Context, docID uuid.UUID, reason string, act relayAct) error {
	actor := act.Actor
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return err
	}
	if !s.humanApprovalType(doc.Type) {
		return fmt.Errorf("a %s is decided by its agent reviewer; it comes to you only when the reviewer escalates", doc.Type)
	}
	if strings.TrimSpace(reason) == "" {
		return fmt.Errorf("requesting changes needs a reason the author can act on")
	}
	return s.returnForChangesAs(ctx, rules.ReturnForChanges{
		DocID: docID, Actor: actor,
		Comments: []rules.ReviewComment{{Body: strings.TrimSpace(reason)}},
	}, personVerdict(docID, store.VerdictSendBack, act))
}

// evaluateContractGate runs G1 and, on pass, advances the feature — the
// gate.evaluated audit row precedes the transition row in the same
// transaction (FR-3.2).
func (s *Server) evaluateContractGate(ctx context.Context, featureID uuid.UUID) error {
	f, err := store.GetFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return err
	}
	if f.State != lifecycle.FeatIdea {
		return nil
	}
	specApproved := false
	spec, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "spec", "feature", f.ID)
	if err != nil && err != store.ErrNotFound {
		return err
	}
	if err == nil {
		specApproved = spec.State == lifecycle.DocApproved
	}
	devPlanApproved := false
	devPlan, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "dev_plan", "feature", f.ID)
	if err != nil && err != store.ErrNotFound {
		return err
	}
	if err == nil {
		devPlanApproved = devPlan.State == lifecycle.DocApproved
	}
	// Phase 2: the contract is spec AND dev-plan (FR-2.1, DESIGN-005 §3).
	g := lifecycle.G1(specApproved, true, devPlanApproved)

	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.Audit(ctx, tx, "orchestrator", "gate.evaluated", "feature", &f.ID,
			map[string]any{"gate": string(g.Gate), "pass": g.Pass, "reason": g.Reason}); err != nil {
			return err
		}
		if !g.Pass {
			return nil
		}
		return store.TransitionFeature(ctx, tx, f, lifecycle.FeatContractApproved, "orchestrator", nil)
	})
}

// reindexDocument re-parses the file and rebuilds the section index.
func (s *Server) reindexDocument(ctx context.Context, docID uuid.UUID) error {
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return err
	}
	raw, err := os.ReadFile(filepath.Join(s.RepoRoot, doc.Path))
	if err != nil {
		return err
	}
	parsed, err := content.Parse(string(raw))
	if err != nil {
		// Unparseable drafts stay registered but unindexed; validation at
		// submit reports the problem to the author.
		s.Log.Warn("reindex: parse failed", "path", doc.Path, "err", err)
		return nil
	}
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.ReplaceSections(ctx, tx, doc.ID, content.Hash(raw), parsed.Sections); err != nil {
			return err
		}
		if err := store.RefreshDocumentTitle(ctx, tx, doc.ID, parsed.FrontMatterString("title")); err != nil {
			return err
		}
		return store.Audit(ctx, tx, "orchestrator", "document.indexed", "document", &doc.ID,
			map[string]any{"sections": len(parsed.Sections)})
	})
}

// ReconcileGates recomputes G1 for all idea-state features (boot duty 4,
// DESIGN-002 §3): a spec approval whose follow-up was lost to a crash is
// repaired here.
func (s *Server) ReconcileGates(ctx context.Context) error {
	rows, err := s.Store.Pool.Query(ctx, `SELECT id FROM features WHERE state = 'idea'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, id := range ids {
		if err := s.evaluateContractGate(ctx, id); err != nil {
			s.Log.Error("gate reconciliation", "feature", id, "err", err)
		}
	}
	return nil
}
