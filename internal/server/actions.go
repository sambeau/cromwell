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

	"cromwell/internal/bus"
	"cromwell/internal/config"
	"cromwell/internal/content"
	"cromwell/internal/lifecycle"
	"cromwell/internal/rules"
	"cromwell/internal/store"
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
		snap.Doc = docSnap(doc)
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
			return snap, ignoreNotFound(loadDoc(e.RefID))
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
		snap.Doc = docSnap(doc)
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

func docSnap(d *store.Document) *rules.DocSnap {
	snap := &rules.DocSnap{
		ID: d.ID, Type: d.Type, State: d.State,
		ContentHash: d.ContentHash, OwnerType: d.OwnerType, Path: d.Path,
		IsSuccessor: d.SupersedesID != nil,
	}
	if d.OwnerID != nil {
		snap.OwnerID = *d.OwnerID
	}
	return snap
}

// execute runs one action. Each case is transactional with its audit rows
// (O-3) and publishes follow-up events after commit.
func (s *Server) execute(ctx context.Context, action rules.Action) error {
	switch a := action.(type) {
	case rules.QueueReview:
		return s.queueReview(ctx, a)
	case rules.ApproveDocument:
		return s.approveDocument(ctx, a.DocID, a.Actor)
	case rules.ReturnForChanges:
		return s.returnForChanges(ctx, a)
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
	case rules.ReindexDocument:
		return s.reindexDocument(ctx, a.DocID)
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
		_, err := store.EnqueueDispatch(ctx, tx, purpose, role.Name, model, "document", a.DocID, a.IdempotencyKey)
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
			"A .cromwell/ configuration error is blocking dispatch. Fix the file and respond to resume.",
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
// commit.
func (s *Server) approveDocument(ctx context.Context, docID uuid.UUID, actor string) error {
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
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.TransitionDocument(ctx, tx, doc, lifecycle.DocApprove, actor, nil); err != nil {
			return err
		}
		if predecessor != nil {
			// Supersede atomically with the successor's approval (L-2).
			if err := store.TransitionDocument(ctx, tx, predecessor, lifecycle.DocSupersede, "lifecycle-engine",
				map[string]any{"superseded_by": doc.ID.String()}); err != nil {
				return err
			}
			archived := filepath.Join("docs/_superseded", filepath.Base(canonicalPath))
			if err := store.UpdateDocumentPath(ctx, tx, predecessor.ID, archived); err != nil {
				return err
			}
			if err := store.UpdateDocumentPath(ctx, tx, doc.ID, canonicalPath); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if predecessor != nil {
		if err := s.takeOverCanonicalPath(doc.Path, canonicalPath); err != nil {
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

	s.Bus.Publish(bus.DocumentTransitioned{
		DocID: doc.ID, From: from, To: lifecycle.DocApproved,
		Event: lifecycle.DocApprove, Actor: actor,
	})
	return nil
}

// takeOverCanonicalPath performs the revision file operations in one
// server-authored commit (DESIGN-003 §5): predecessor archived under
// docs/_superseded/, successor moved to the canonical path.
func (s *Server) takeOverCanonicalPath(successorPath, canonicalPath string) error {
	archiveDir := filepath.Join(s.RepoRoot, "docs/_superseded")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		return err
	}
	archived := filepath.Join("docs/_superseded", filepath.Base(canonicalPath))
	steps := [][]string{
		{"git", "mv", canonicalPath, archived},
		{"git", "mv", successorPath, canonicalPath},
		{"git", "commit", "-m",
			fmt.Sprintf("cromwell: revision of %s approved; predecessor archived", canonicalPath),
			"--author", "cromwell <cromwell@localhost>"},
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
	doc, err := store.GetDocument(ctx, s.Store.Pool, a.DocID)
	if err != nil {
		return err
	}
	from := doc.State
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.TransitionDocument(ctx, tx, doc, lifecycle.DocRequestChanges, a.Actor,
			map[string]any{"comments": len(a.Comments)}); err != nil {
			return err
		}
		for _, c := range a.Comments {
			if err := store.InsertComment(ctx, tx, doc.ID, a.DispatchID, a.Actor, c.SectionRef, c.Body); err != nil {
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
