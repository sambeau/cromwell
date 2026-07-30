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
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

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
		// A project that has not bound the purpose to a role cannot author.
		// That is a configuration problem for a human, not a silent stall.
		return s.configErrorCheckpoint(ctx, "feature", featureID,
			fmt.Errorf("no role is assigned to %q in config.yaml, so nothing can write this document", purpose))
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
