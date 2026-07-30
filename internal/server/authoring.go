package server

// The authoring chain (SPEC-009 Stage 1). Approving a design document is the
// single act that means "ready to spec"; from there the orchestrator carries a
// feature to decomposition with no further human involvement, and stops at
// gate 2 — a human starting the work.
//
// The chain is expressed as two invariants rather than a sequence of steps
// (FR-4), because width-first planning means approvals, features and
// descriptions arrive in any order:
//
//	every feature G0 admits, and that has a description, has a current spec
//	every feature with an approved spec has a current dev-plan
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

	"cromwell/internal/config"
	"cromwell/internal/lifecycle"
	"cromwell/internal/store"
)

// designApproved reports whether an owner's current design document is
// approved. A missing design is not an error — it is simply not approved,
// the same reading evaluateContractGate takes of a missing spec.
func (s *Server) designApproved(ctx context.Context, ownerType string, ownerID uuid.UUID) bool {
	d, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "design", ownerType, ownerID)
	return err == nil && d.State == lifecycle.DocApproved
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
	f, err := store.GetFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		if err == store.ErrNotFound {
			return nil
		}
		return err
	}
	// A feature past the forming stage has a contract already; re-authoring
	// one mid-flight is the revision cascade's job, not this one.
	if f.State != lifecycle.FeatIdea {
		return nil
	}

	// Invariant 1 — the spec.
	spec, specErr := store.CurrentDocForOwner(ctx, s.Store.Pool, "spec", "feature", f.ID)
	hasSpec := specErr == nil
	if !hasSpec {
		// An undescribed feature is not dispatched, not checkpointed, and not
		// complained about: in width-first planning a placeholder feature is
		// entirely normal, and a checkpoint here would be noise at exactly the
		// moment the system should be quiet (FR-4.5).
		if f.Description == "" {
			return nil
		}
		if g := s.specReady(ctx, f); !g.Pass {
			return nil
		}
		return s.queueAuthoring(ctx, "write-spec", f.ID)
	}

	// Invariant 2 — the dev-plan, once the spec is approved.
	if spec.State != lifecycle.DocApproved {
		return nil
	}
	if _, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "dev_plan", "feature", f.ID); err == nil {
		return nil // already has one; nothing to restore
	} else if err != store.ErrNotFound {
		return err
	}
	return s.queueAuthoring(ctx, "write-dev-plan", f.ID)
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
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		n, err := store.CountDispatchesForRef(ctx, tx, "feature", featureID, purpose)
		if err != nil {
			return err
		}
		key := fmt.Sprintf("%s:%s:%d", purpose, featureID, n)
		_, err = store.EnqueueDispatch(ctx, tx, purpose, role, model, "feature", featureID, key)
		return err
	})
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
	path, err := s.authoredDocPath(ctx, f, docType)
	if err != nil {
		return err
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
	if _, err := s.RegisterDocForOwner(ctx, path, docType, "feature", &f.ID, actor); err != nil {
		return err
	}
	// Committing is for durability and history, not for detection: the git
	// hook only ever sees commits, and an agent's write is uncommitted at the
	// moment it happens, so the notification comes from SubmitDoc's own
	// transition rather than from git (FR-7.2).
	s.commitDocument(path, fmt.Sprintf("cromwell: %s authored for %s", docType, f.Slug))
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
	return filepath.Join("docs", initPath, f.Slug, docType+".md"), nil
}

// commitDocument commits one path with cromwell as the author. A failure is
// logged rather than raised: the document is registered and submitted either
// way, and a repository that cannot be committed to is a problem for a person,
// not a reason to lose the work.
func (s *Server) commitDocument(path, message string) {
	if _, err := gitIn(s.RepoRoot, "add", "--", path); err != nil {
		s.Log.Warn("could not stage authored document", "path", path, "err", err)
		return
	}
	if _, err := gitIn(s.RepoRoot, "commit", "-m", message, "--author", "cromwell <cromwell@localhost>", "--", path); err != nil {
		s.Log.Warn("could not commit authored document", "path", path, "err", err)
	}
}
