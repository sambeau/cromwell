package server

// The chat agent's task tools (SPEC-020 FR-3). Claiming and submitting a task
// are doing, not relaying: DEC-007 decision 3 lets the chat agent claim and
// submit work, and DESIGN-010 §5c says "It may claim and submit work". They
// take no quote (SD-15). Releasing a claim is a person's act in the web UI
// (SD-5), and the chat agent holds no verdict: its work gets an independent
// code review, and the feature an independent verification.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"

	"subutai/internal/config"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

func (s *Server) mcpClaimTools() []mcpTool {
	return []mcpTool{
		{
			Name: "claim_task",
			Description: "Claim a task of a feature that is being built, so that you and the person implement it in the " +
				"feature's working copy instead of an agent. The feature's other agents wait while you hold the claim, so " +
				"submit promptly, or tell the person if you can't finish. The only way forward is submit_task: you can't " +
				"release a claim, and a person can, in the web UI. Your work gets an independent code review, and the " +
				"feature an independent verification, and you can't review, approve or verify it. A claim with no activity " +
				"in the working copy for the project's expiry time asks the person whether anyone is still working on it. " +
				"Edit files only inside the working copy this returns, and don't commit: Subutai commits your work when " +
				"you submit it. Calling this again on a task you hold renews the claim, or resumes it after a send-back, " +
				"when the result includes the reviewer's comments. Use get_feature to find a task you can claim.",
			Schema: objectSchema(map[string]any{
				"task": stringProp("The task's ID, such as \"FEAT-023-T03\", or \"BUG-007-T01\" for a bug's task."),
			}, "task"),
			Handler: s.mcpClaimTask,
		},
		{
			Name: "submit_task",
			Description: "Hand the task you claimed to an independent code reviewer, with a summary of what you did. " +
				"Subutai commits the working copy for you. If the reviewer asks for changes, the task comes back to you " +
				"with the reviewer's comments, and claim_task resumes it. Submitting is the only way forward from a " +
				"claim, and you can't approve your own work.",
			Schema: objectSchema(map[string]any{
				"task":    stringProp("The ID of the task you claimed, such as \"FEAT-023-T03\"."),
				"summary": stringProp("One or two sentences saying what you did, for the reviewer to read."),
			}, "task", "summary"),
			Handler: s.mcpSubmitTask,
		},
	}
}

func (s *Server) mcpClaimTask(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	ref, ok := argString(args, "task")
	if !ok {
		return nil, errors.New("say which task to claim, with \"task\": its ID, such as \"FEAT-023-T03\"")
	}
	res, err := s.ClaimWork(ctx, ref, s.ChatClaimant())
	if err != nil {
		return nil, err
	}
	entry, err := s.mcpTaskEntryFor(ctx, res.Task)
	if err != nil {
		return nil, err
	}
	hours := config.DefaultClaimExpiryHours
	if cfg, err := s.freshConfig(); err == nil {
		hours = int(cfg.ClaimExpiry() / time.Hour)
	}
	out := map[string]any{
		"task":    entry,
		"feature": map[string]any{"id": res.Feature.PublicID, "path": res.FeaturePath, "name": res.Feature.Name},
		"working_copy": map[string]any{
			"path": res.WorkingCopy.Path, "branch": res.WorkingCopy.Branch, "base_commit": res.WorkingCopy.BaseCommit,
		},
		"contract": res.Contract,
		"rules":    res.Rules,
		"expires": fmt.Sprintf("If nothing changes in the working copy for %d hours, the person will be asked whether anyone is still working on this.",
			hours),
		"next": "Implement the task in the working copy, run the project's build and tests, then call submit_task with a summary.",
	}
	if res.ReviewComments != nil && res.Round > 1 {
		out["review_comments"] = res.ReviewComments
	}
	if res.Renewed {
		out["renewed"] = true
	}
	if res.Resumed {
		out["resumed"] = true
	}
	return out, nil
}

func (s *Server) mcpSubmitTask(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	ref, ok := argString(args, "task")
	if !ok {
		return nil, errors.New("say which task to submit, with \"task\": its ID, such as \"FEAT-023-T03\"")
	}
	summary, _ := argString(args, "summary")
	res, err := s.SubmitWork(ctx, ref, s.ChatClaimant(), summary)
	if err != nil {
		return nil, err
	}
	entry, err := s.mcpTaskEntryFor(ctx, res.Task)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"task":   entry,
		"review": map[string]any{"run_id": res.ReviewRunID, "model": res.ReviewModel},
		"commit": res.Commit,
		"next": "A code reviewer will review this. get_feature shows its state and any comments. " +
			"If it comes back, call claim_task to resume it. You can't review or approve it yourself.",
	}
	if res.Notice != "" {
		out["notice"] = res.Notice
	}
	return out, nil
}

// mcpTaskEntryFor describes one task, looking up its siblings for the IDs of
// what it depends on.
func (s *Server) mcpTaskEntryFor(ctx context.Context, t *store.Task) (map[string]any, error) {
	tasks, err := store.TasksForFeature(ctx, s.Store.Pool, t.FeatureID)
	if err != nil {
		return nil, err
	}
	return s.mcpTaskEntry(ctx, t, taskIDs(tasks))
}

func taskIDs(tasks []store.Task) map[uuid.UUID]string {
	ids := make(map[uuid.UUID]string, len(tasks))
	for _, t := range tasks {
		ids[t.ID] = t.PublicID
	}
	return ids
}

// mcpTaskEntry is a task as MCP shows it (FR-3.6): where it stands, who is
// executing it (FR-1.8), its latest claim, the reviewer's comments after a
// send-back, and whether the chat agent can claim it now, and if not why.
func (s *Server) mcpTaskEntry(ctx context.Context, t *store.Task, ids map[uuid.UUID]string) (map[string]any, error) {
	q := s.Store.Pool
	deps := make([]string, 0, len(t.DependsOn))
	for _, d := range t.DependsOn {
		if id, ok := ids[d]; ok {
			deps = append(deps, id)
		}
	}
	line, err := s.executorLine(ctx, q, t)
	if err != nil {
		return nil, err
	}
	executor := map[string]any{"sentence": line.Sentence}
	if line.Kind != "" {
		executor["kind"], executor["who"], executor["model"] = line.Kind, line.Who, line.Model
		executor["run_id"], executor["measured"] = line.RunID, line.Measured
	}
	out := map[string]any{
		"id": t.PublicID, "title": t.Title, "state": string(t.State), "depends_on": deps,
		"executor": executor, "url": "/ui/t/" + t.PublicID,
	}
	if c, err := store.LatestClaimFor(ctx, q, "task", t.ID); err == nil {
		claim := map[string]any{
			"kind": c.Kind, "who": whoWords(c.Kind, c.Actor, ""), "state": string(c.State),
			"since":         c.ClaimedAt.UTC().Format(time.RFC3339),
			"last_activity": c.LastActivityAt.UTC().Format(time.RFC3339),
		}
		if c.State == lifecycle.ClaimEnded && c.EndReason == "released" {
			claim["released_by"] = c.EndedBy
		}
		out["claim"] = claim
	} else if !errors.Is(err, store.ErrNotFound) {
		return nil, err
	}
	if round, err := store.CurrentTaskRound(ctx, q, t.ID); err == nil && round > 1 {
		rc, err := s.reviewComments(ctx, q, t.ID)
		if err != nil {
			return nil, err
		}
		if rc != nil {
			out["review_comments"] = rc
		}
	}
	why, err := s.claimRefusalFor(ctx, t)
	if err != nil {
		return nil, err
	}
	out["claimable"] = why == ""
	if why != "" {
		out["why_not"] = why
	}
	return out, nil
}

// mcpFeatureTasks are a feature's tasks in plan order, for get_feature.
func (s *Server) mcpFeatureTasks(ctx context.Context, featureID uuid.UUID) ([]map[string]any, error) {
	tasks, err := store.TasksForFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return nil, err
	}
	ids := taskIDs(tasks)
	out := make([]map[string]any, 0, len(tasks))
	for i := range tasks {
		e, err := s.mcpTaskEntry(ctx, &tasks[i], ids)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, nil
}
