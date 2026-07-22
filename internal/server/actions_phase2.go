package server

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/config"
	"cromwell/internal/lifecycle"
	"cromwell/internal/rules"
	"cromwell/internal/store"
)

// executePhase2 handles the implementation-loop actions. Returns (handled).
func (s *Server) executePhase2(ctx context.Context, action rules.Action) (bool, error) {
	switch a := action.(type) {
	case rules.DecomposeDevPlan:
		return true, s.decomposeDevPlan(ctx, a.FeatureID, a.DevPlanDocID, false)
	case rules.ReDecomposeDevPlan:
		return true, s.decomposeDevPlan(ctx, a.FeatureID, a.DevPlanDocID, true)
	case rules.DispatchReadyTasks:
		return true, s.dispatchReadyTasks(ctx, a.FeatureID)
	case rules.CompleteImplementation:
		return true, s.completeImplementation(ctx, a.TaskID, a.DispatchID, a.Summary)
	case rules.ApproveTaskCode:
		return true, s.approveTaskCode(ctx, a.TaskID, a.Actor)
	case rules.ReturnTaskCode:
		return true, s.returnTaskCode(ctx, a)
	case rules.MergeFeature:
		return true, s.mergeFeature(ctx, a.FeatureID, a.Actor)
	case rules.ReturnFeatureForCriteria:
		return true, s.returnFeatureForCriteria(ctx, a)
	case rules.MarkRevisionInFlight:
		return true, s.markRevisionInFlight(ctx, a.FeatureID, a.DocType)
	case rules.ClearSpecStale:
		return true, s.clearSpecStale(ctx, a.FeatureID, a.Continue)
	}
	return false, nil
}

// decomposeDevPlan creates or reconciles tasks from a dev-plan's task table
// (DESIGN-005 §4). On first decomposition every row is created; on revision
// the plan reconciles non-destructively (DP-4).
func (s *Server) decomposeDevPlan(ctx context.Context, featureID, devPlanID uuid.UUID, redecompose bool) error {
	doc, err := store.GetDocument(ctx, s.Store.Pool, devPlanID)
	if err != nil {
		return err
	}
	raw, err := s.readDocFile(doc.Path)
	if err != nil {
		return err
	}
	rows, err := lifecycle.ParseDevPlanTasks(string(raw))
	if err != nil {
		return fmt.Errorf("dev-plan task table did not parse at decomposition (should have failed validation): %w", err)
	}

	var existing []lifecycle.ExistingTask
	if redecompose {
		existing, err = store.ExistingTasksForPlan(ctx, s.Store.Pool, featureID)
		if err != nil {
			return err
		}
	}
	plan := lifecycle.PlanDecomposition(rows, existing)

	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		// Map local id → task UUID for dependency resolution (existing +
		// newly created).
		idOf := map[string]uuid.UUID{}
		current, err := store.TasksForFeature(ctx, tx, featureID)
		if err != nil {
			return err
		}
		for _, t := range current {
			if t.LocalID != "" {
				idOf[t.LocalID] = t.ID
			}
		}
		for _, r := range plan.Creates {
			t, err := store.CreateTask(ctx, tx, featureID, r.Position, r.LocalID, r.Title, r.Body, "orchestrator")
			if err != nil {
				return err
			}
			idOf[r.LocalID] = t.ID
		}
		for _, r := range plan.Updates {
			if id, ok := idOf[r.LocalID]; ok {
				if err := store.UpdateTaskFields(ctx, tx, id, r.Position, r.Title, r.Body); err != nil {
					return err
				}
			}
		}
		// Resolve dependencies for all rows in the new table.
		byLocal := map[string]lifecycle.TaskRow{}
		for _, r := range rows {
			byLocal[r.LocalID] = r
		}
		for _, r := range rows {
			deps := make([]uuid.UUID, 0, len(r.DependsOn))
			for _, dep := range r.DependsOn {
				if id, ok := idOf[dep]; ok {
					deps = append(deps, id)
				}
			}
			if id, ok := idOf[r.LocalID]; ok {
				if err := store.SetTaskDependencies(ctx, tx, id, deps); err != nil {
					return err
				}
			}
		}
		for _, localID := range plan.DeleteLocalIDs {
			if id, ok := idOf[localID]; ok {
				if err := store.DeleteTask(ctx, tx, id, "orchestrator"); err != nil {
					return err
				}
			}
		}
		// Mark dependency-free tasks ready immediately (DESIGN-005 §5).
		for _, r := range plan.Creates {
			if len(r.DependsOn) == 0 {
				t := store.Task{ID: idOf[r.LocalID], State: lifecycle.TaskPending}
				if err := store.TransitionTask(ctx, tx, &t, lifecycle.TaskReadyEvent, "orchestrator", nil); err != nil {
					return err
				}
			}
		}
		return store.Audit(ctx, tx, "orchestrator", "devplan.decomposed", "feature", &featureID,
			map[string]any{"created": len(plan.Creates), "updated": len(plan.Updates),
				"deleted": len(plan.DeleteLocalIDs), "kept_mismatches": plan.KeptMismatches})
	})
}

// dispatchReadyTasks enqueues an implement-task dispatch for each
// dispatchable (ready) task, moving it ready → active in the same
// transaction; the governor serialises them per feature (DESIGN-006 §5).
func (s *Server) dispatchReadyTasks(ctx context.Context, featureID uuid.UUID) error {
	cfg, err := s.freshConfig()
	if err != nil {
		return s.configErrorCheckpoint(ctx, "feature", featureID, err)
	}
	if _, ok := cfg.Assignments["implement-task"]; !ok {
		return s.configErrorCheckpoint(ctx, "feature", featureID,
			fmt.Errorf("config.yaml assignments has no implement-task role"))
	}
	tasks, err := s.Store.DispatchableTasks(ctx, featureID)
	if err != nil {
		return err
	}
	// The branch HEAD now becomes the base for any task starting its work, so
	// code review later diffs the task's whole contribution (DESIGN-006 §5).
	var head string
	if wt, werr := store.LiveWorktreeForFeature(ctx, s.Store.Pool, featureID); werr == nil {
		head, _ = gitIn(s.worktreeAbs(wt.Path), "rev-parse", "HEAD")
		head = strings.TrimSpace(head)
	}
	for i := range tasks {
		task := tasks[i]
		err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			fresh, err := store.GetTask(ctx, tx, task.ID)
			if err != nil {
				return err
			}
			if head != "" {
				if err := store.SetTaskBaseCommit(ctx, tx, task.ID, head); err != nil {
					return err
				}
			}
			// Move ready → active as we enqueue; the dispatch runs the work.
			if err := store.TransitionTask(ctx, tx, fresh, lifecycle.TaskClaim, "orchestrator", nil); err != nil {
				return err
			}
			return s.enqueueImplementTx(ctx, tx, cfg, task.ID)
		})
		if err != nil {
			return err
		}
	}
	if len(tasks) > 0 {
		s.Dispatcher.Kick()
	}
	return nil
}

// enqueueImplementTx enqueues an implement-task dispatch for a task within
// the caller's transaction, computing the re-dispatch idempotency key from
// how many implement dispatches the task already has (DESIGN-006 §7). The
// task's state transition (ready → active on first dispatch, or already
// active on a code-review rework) is the caller's responsibility.
func (s *Server) enqueueImplementTx(ctx context.Context, tx pgx.Tx, cfg *config.Config, taskID uuid.UUID) error {
	role := cfg.Assignments["implement-task"]
	model, err := s.modelForPurpose(cfg, "implement-task", role)
	if err != nil {
		return err
	}
	n, err := store.CountDispatchesForRef(ctx, tx, "task", taskID, "implement-task")
	if err != nil {
		return err
	}
	key := rules.ImplementIdempotencyKey(taskID, n)
	_, err = store.EnqueueDispatch(ctx, tx, "implement-task", role, model, "task", taskID, key)
	return err
}

// completeImplementation commits the implementer's worktree changes to the
// feature branch, moves the task to review, and queues code review
// (DESIGN-006 §5).
func (s *Server) completeImplementation(ctx context.Context, taskID, dispatchID uuid.UUID, summary string) error {
	task, err := store.GetTask(ctx, s.Store.Pool, taskID)
	if err != nil {
		return err
	}
	wt, err := store.LiveWorktreeForFeature(ctx, s.Store.Pool, task.FeatureID)
	if err != nil {
		return err
	}
	root := s.worktreeAbs(wt.Path)

	// Commit the changes (server-authored, message names the task). If there
	// is nothing to commit, the implementer produced no diff — a failure the
	// code reviewer should see, so we still proceed with an empty commit
	// marker rather than silently dropping the task.
	commitMsg := fmt.Sprintf("cromwell: %s — %s\n\n%s", task.LocalID, task.Title, summary)
	if err := s.commitWorktree(root, commitMsg); err != nil {
		return fmt.Errorf("committing task work: %w", err)
	}

	cfg, err := s.freshConfig()
	if err != nil {
		return err
	}
	reviewerRole, ok := cfg.Assignments["review-code"]
	if !ok {
		return s.configErrorCheckpoint(ctx, "task", taskID, fmt.Errorf("config.yaml assignments has no review-code role"))
	}
	model, err := s.modelForPurpose(cfg, "review-code", reviewerRole)
	if err != nil {
		return s.configErrorCheckpoint(ctx, "task", taskID, err)
	}
	head, _ := gitIn(root, "rev-parse", "HEAD")
	key := rules.CodeReviewIdempotencyKey(taskID, strings.TrimSpace(head))

	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.TransitionTask(ctx, tx, task, lifecycle.TaskImplemented, "orchestrator",
			map[string]any{"summary": summary}); err != nil {
			return err
		}
		_, err := store.EnqueueDispatch(ctx, tx, "review-code", reviewerRole, model, "task", taskID, key)
		return err
	})
	if err == nil {
		s.Dispatcher.Kick()
	}
	return err
}

// approveTaskCode records a code-review approval: task → done, readiness
// re-evaluation, then G2 (DESIGN-006 §5).
func (s *Server) approveTaskCode(ctx context.Context, taskID uuid.UUID, actor string) error {
	task, err := store.GetTask(ctx, s.Store.Pool, taskID)
	if err != nil {
		return err
	}
	featureID := task.FeatureID
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.TransitionTask(ctx, tx, task, lifecycle.TaskApprove, actor, nil); err != nil {
			return err
		}
		// Readiness re-evaluation: dependents whose deps are now all done.
		deps, err := store.ReadyDependents(ctx, tx, featureID, taskID)
		if err != nil {
			return err
		}
		for i := range deps {
			d := deps[i]
			if err := store.TransitionTask(ctx, tx, &d, lifecycle.TaskReadyEvent, "orchestrator", nil); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Evaluate G2: all tasks terminal → feature to review + verify; else
	// dispatch newly-ready tasks.
	return s.evaluateTasksGate(ctx, featureID)
}

// evaluateTasksGate runs G2 and either advances the feature to review
// (queuing verification) or dispatches ready tasks (DESIGN-006 §5-6).
func (s *Server) evaluateTasksGate(ctx context.Context, featureID uuid.UUID) error {
	total, done, terminal, err := store.TaskCounts(ctx, s.Store.Pool, featureID)
	if err != nil {
		return err
	}
	stale, err := store.FeatureSpecStale(ctx, s.Store.Pool, featureID)
	if err != nil {
		return err
	}
	g := lifecycle.G2(total, done, terminal, stale)
	feature, err := store.GetFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return err
	}

	if !g.Pass {
		if err := s.auditGate(ctx, featureID, g); err != nil {
			return err
		}
		// Not complete — dispatch any tasks that just became ready.
		return s.dispatchReadyTasks(ctx, featureID)
	}
	if feature.State != lifecycle.FeatActive {
		return nil // already advanced
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.auditGateTx(ctx, tx, featureID, g); err != nil {
			return err
		}
		return store.TransitionFeature(ctx, tx, feature, lifecycle.FeatTasksComplete, "orchestrator", nil)
	})
	if err != nil {
		return err
	}
	return s.queueVerification(ctx, featureID)
}

func (s *Server) queueVerification(ctx context.Context, featureID uuid.UUID) error {
	cfg, err := s.freshConfig()
	if err != nil {
		return s.configErrorCheckpoint(ctx, "feature", featureID, err)
	}
	role, ok := cfg.Assignments["verify-feature"]
	if !ok {
		return s.configErrorCheckpoint(ctx, "feature", featureID, fmt.Errorf("config.yaml assignments has no verify-feature role"))
	}
	model, err := s.modelForPurpose(cfg, "verify-feature", role)
	if err != nil {
		return s.configErrorCheckpoint(ctx, "feature", featureID, err)
	}
	wt, err := store.LiveWorktreeForFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return err
	}
	head, _ := gitIn(s.worktreeAbs(wt.Path), "rev-parse", "HEAD")
	key := rules.VerifyIdempotencyKey(featureID, strings.TrimSpace(head))
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, err := store.EnqueueDispatch(ctx, tx, "verify-feature", role, model, "feature", featureID, key)
		return err
	})
	if err == nil {
		s.Dispatcher.Kick()
	}
	return err
}

// returnTaskCode records a code-review request_changes: task → active,
// comments attached, implementer re-dispatched against the kept worktree.
func (s *Server) returnTaskCode(ctx context.Context, a rules.ReturnTaskCode) error {
	task, err := store.GetTask(ctx, s.Store.Pool, a.TaskID)
	if err != nil {
		return err
	}
	cfg, err := s.freshConfig()
	if err != nil {
		return s.configErrorCheckpoint(ctx, "task", a.TaskID, err)
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		// review → active, then re-dispatch the implementer against the same
		// worktree — the diff is amended, not restarted (DESIGN-006 §5).
		if err := store.TransitionTask(ctx, tx, task, lifecycle.TaskRequestChanges, a.Actor,
			map[string]any{"comments": len(a.Comments)}); err != nil {
			return err
		}
		// Code-review comments are recorded in the audit payload (task-scoped
		// comment storage is a phase-3 nicety) so they are not lost.
		if err := store.Audit(ctx, tx, a.Actor, "task.review_comments", "task", &a.TaskID,
			map[string]any{"comments": a.Comments}); err != nil {
			return err
		}
		return s.enqueueImplementTx(ctx, tx, cfg, a.TaskID)
	})
	if err != nil {
		return err
	}
	s.Dispatcher.Kick()
	return nil
}

// mergeFeature records a verification approval (G3): merge the branch, feature
// → done, worktree GC (DESIGN-006 §6). A non-clean merge raises a checkpoint.
func (s *Server) mergeFeature(ctx context.Context, featureID uuid.UUID, actor string) error {
	feature, err := store.GetFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return err
	}
	wt, err := store.LiveWorktreeForFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return err
	}
	if merr := s.mergeBranch(wt.Branch); merr != nil {
		// Non-clean merge: checkpoint, stay in review (FR-9.3).
		var cp *store.Checkpoint
		err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			var e error
			cp, e = store.CreateCheckpoint(ctx, tx, "merge-conflict", "feature", featureID,
				fmt.Sprintf("Feature branch %s does not merge cleanly into main. Resolve conflicts, then respond.", wt.Branch),
				map[string]any{"error": merr.Error(), "branch": wt.Branch})
			return e
		})
		if err == nil {
			s.notifyCheckpointRaised(cp)
		}
		return err
	}
	g := lifecycle.G3(true, true)
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.auditGateTx(ctx, tx, featureID, g); err != nil {
			return err
		}
		if err := store.TransitionFeature(ctx, tx, feature, lifecycle.FeatVerified, actor, nil); err != nil {
			return err
		}
		// Worktree GC happens on the heartbeat (terminal feature); mark the
		// row removed and remove the git worktree now that main has the code.
		if err := store.MarkWorktreeRemoved(ctx, tx, wt.ID); err != nil {
			return err
		}
		return store.Audit(ctx, tx, "orchestrator", "feature.merged", "feature", &featureID,
			map[string]any{"branch": wt.Branch})
	})
	// The git worktree directory is removed by the heartbeat GC pass.
}

// returnFeatureForCriteria records a verification request_changes: unmet
// criteria become tasks, feature → active (DESIGN-006 §6).
func (s *Server) returnFeatureForCriteria(ctx context.Context, a rules.ReturnFeatureForCriteria) error {
	feature, err := store.GetFeature(ctx, s.Store.Pool, a.FeatureID)
	if err != nil {
		return err
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		existing, err := store.TasksForFeature(ctx, tx, a.FeatureID)
		if err != nil {
			return err
		}
		pos := len(existing)
		for i, c := range a.Unmet {
			localID := fmt.Sprintf("V%d", pos+i+1)
			t, err := store.CreateTask(ctx, tx, a.FeatureID, pos+i,
				localID, "Address unmet criterion: "+c.ID, c.Evidence, "orchestrator")
			if err != nil {
				return err
			}
			// Verification-derived tasks have no dependencies → ready now.
			if err := store.TransitionTask(ctx, tx, t, lifecycle.TaskReadyEvent, "orchestrator", nil); err != nil {
				return err
			}
		}
		// Feature review → active to resume the loop (DESIGN-006 §6).
		return store.TransitionFeature(ctx, tx, feature, lifecycle.FeatRework, a.Actor,
			map[string]any{"unmet_criteria": len(a.Unmet)})
	})
	if err != nil {
		return err
	}
	return s.dispatchReadyTasks(ctx, a.FeatureID)
}

// markRevisionInFlight sets spec_stale and raises the revision-in-flight
// checkpoint (DESIGN-005 §6, FR-10.1).
func (s *Server) markRevisionInFlight(ctx context.Context, featureID uuid.UUID, docType string) error {
	tasks, err := store.TasksForFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return err
	}
	var inflight, doneList []string
	for _, t := range tasks {
		switch {
		case t.State == lifecycle.TaskActive || t.State == lifecycle.TaskReview:
			inflight = append(inflight, t.LocalID)
		case t.State == lifecycle.TaskDone:
			doneList = append(doneList, t.LocalID)
		}
	}
	var cp *store.Checkpoint
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := store.SetSpecStale(ctx, tx, featureID, true, "orchestrator",
			"successor "+docType+" submitted"); err != nil {
			return err
		}
		var e error
		cp, e = store.CreateCheckpoint(ctx, tx, "revision-in-flight", "feature", featureID,
			fmt.Sprintf("A revised %s was submitted while this feature is in flight. New task dispatches are blocked. Continue (apply the revision to later work) or pause?", docType),
			map[string]any{"in_flight_tasks": inflight, "done_tasks": doneList})
		return e
	})
	if err == nil {
		s.notifyCheckpointRaised(cp)
	}
	return err
}

// clearSpecStale resolves a revision-in-flight: clears the flag and, on
// continue, resumes dispatch (FR-10.2).
func (s *Server) clearSpecStale(ctx context.Context, featureID uuid.UUID, cont bool) error {
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.SetSpecStale(ctx, tx, featureID, false, "orchestrator", "revision resolved")
	})
	if err != nil {
		return err
	}
	if cont {
		return s.dispatchReadyTasks(ctx, featureID)
	}
	return nil
}

// ---- git and worktree helpers ----

func (s *Server) worktreeAbs(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(s.RepoRoot, path)
}

func (s *Server) commitWorktree(root, message string) error {
	if _, err := gitIn(root, "add", "-A"); err != nil {
		return err
	}
	// --allow-empty so a task that (wrongly) produced no diff still commits,
	// surfacing to the code reviewer rather than vanishing.
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", message, "--author", "cromwell <cromwell@localhost>")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git commit: %v: %s", err, out)
	}
	return nil
}

// mergeBranch merges branch into main (the default branch) with a
// server-authored merge commit; a conflict aborts and returns an error.
func (s *Server) mergeBranch(branch string) error {
	if _, err := gitIn(s.RepoRoot, "merge", "--no-ff", "--no-edit", branch); err != nil {
		_, _ = gitIn(s.RepoRoot, "merge", "--abort")
		return fmt.Errorf("merge of %s failed: %w", branch, err)
	}
	return nil
}

func (s *Server) auditGate(ctx context.Context, featureID uuid.UUID, g lifecycle.GateResult) error {
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return s.auditGateTx(ctx, tx, featureID, g)
	})
}

func (s *Server) auditGateTx(ctx context.Context, tx pgx.Tx, featureID uuid.UUID, g lifecycle.GateResult) error {
	return store.Audit(ctx, tx, "orchestrator", "gate.evaluated", "feature", &featureID,
		map[string]any{"gate": string(g.Gate), "pass": g.Pass, "reason": g.Reason})
}

// modelForPurpose resolves the model for a purpose: the role's model, unless
// config.yaml routing overrides it (DESIGN-004 §4).
func (s *Server) modelForPurpose(cfg *config.Config, purpose, roleName string) (string, error) {
	role, err := config.LoadRole(s.CompartmentRoot, roleName)
	if err != nil {
		return "", err
	}
	if override, ok := cfg.Routing[purpose]; ok {
		return override, nil
	}
	return role.Model, nil
}
