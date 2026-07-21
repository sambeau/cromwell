package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/rules"
	"cromwell/internal/sizing"
	"cromwell/internal/store"
)

// Phase-3 action execution (SPEC-003): recording an AI estimate, and the
// helpers that resolve entity references and enqueue estimate dispatches.

// executePhase3 handles phase-3 actions, returning handled=false for others so
// the caller falls through to phase 1/2.
func (s *Server) executePhase3(ctx context.Context, action rules.Action) (bool, error) {
	switch a := action.(type) {
	case rules.RecordAIEstimate:
		return true, s.recordAIEstimate(ctx, a)
	}
	return false, nil
}

// recordAIEstimate writes the estimator's outcome as an estimate row. The tier
// is assigned from the evidence, not the agent (FR-1.2): considered when the
// calibration corpus offered reference points for this description, rough when
// it was empty (FR-4.1).
func (s *Server) recordAIEstimate(ctx context.Context, a rules.RecordAIEstimate) error {
	_, description, err := s.entityDescription(ctx, a.RefType, a.RefID)
	if err != nil {
		return err
	}
	corpus, err := store.RetrieveCorpus(ctx, s.Store.Pool, description, 5)
	if err != nil {
		return err
	}
	tier := sizing.TierRough
	if len(corpus) > 0 {
		tier = sizing.TierConsidered
	}
	did := a.DispatchID
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.RecordEstimate(ctx, tx, a.RefType, a.RefID, a.Tokens, tier, a.Rationale, &did, a.Actor)
		return err
	})
}

// resolveRef resolves a human entity reference to (ref_type, id). Forms:
//   - "<init-path>/<slug>#<local-id>" → a task within a feature
//   - "<init-path>/<slug>"            → a feature (tried first)
//   - "<path>"                        → an initiative (fallback)
//
// Feature and nested-initiative paths are ambiguous by shape, so a feature is
// tried before an initiative — the common estimate/cost target.
func (s *Server) resolveRef(ctx context.Context, ref string) (string, uuid.UUID, error) {
	if fp, local, ok := strings.Cut(ref, "#"); ok {
		f, err := s.featureByPath(ctx, fp)
		if err != nil {
			return "", uuid.Nil, err
		}
		t, err := store.TaskByLocalID(ctx, s.Store.Pool, f.ID, local)
		if err != nil {
			return "", uuid.Nil, fmt.Errorf("task %q in %s: %w", local, fp, err)
		}
		return "task", t.ID, nil
	}
	if f, err := s.featureByPath(ctx, ref); err == nil {
		return "feature", f.ID, nil
	}
	if in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(ref, "/")); err == nil {
		return "initiative", in.ID, nil
	}
	return "", uuid.Nil, fmt.Errorf("cannot resolve reference %q (expected a feature or initiative path, or <feature>#<task-id>)", ref)
}

// enqueueEstimate queues an estimate dispatch for a feature or task, keyed so a
// replay is a no-op but a deliberate re-estimate gets a fresh row (FR-4.1).
func (s *Server) enqueueEstimate(ctx context.Context, refType string, refID uuid.UUID) error {
	if refType != "feature" && refType != "task" {
		return fmt.Errorf("only features and tasks are estimated, not %s", refType)
	}
	cfg, err := s.freshConfig()
	if err != nil {
		return err
	}
	role, ok := cfg.Assignments["estimate"]
	if !ok {
		return fmt.Errorf("config.yaml assignments has no estimate role (expected `estimate: estimator`)")
	}
	model, err := s.modelForPurpose(cfg, "estimate", role)
	if err != nil {
		return err
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		n, err := store.CountDispatchesForRef(ctx, tx, refType, refID, "estimate")
		if err != nil {
			return err
		}
		key := rules.EstimateIdempotencyKey(refType, refID, n)
		_, err = store.EnqueueDispatch(ctx, tx, "estimate", role, model, refType, refID, key)
		return err
	})
	if err == nil {
		s.Dispatcher.Kick()
	}
	return err
}
