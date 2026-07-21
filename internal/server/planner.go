package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"cromwell/internal/config"
	"cromwell/internal/dispatch"
	"cromwell/internal/provider"
	"cromwell/internal/rules"
	"cromwell/internal/store"
	"cromwell/internal/toolhost"
)

// Plan assembles a claimed dispatch's plan, switching on purpose — one loop,
// many tools (DESIGN-006 §4.1). Read-only document reviews carry a nil
// ToolCtx; execution purposes carry a worktree-scoped context.
func (s *Server) Plan(ctx context.Context, d *store.Dispatch) (*dispatch.Plan, error) {
	switch d.Purpose {
	case "implement-task":
		return s.planImplement(ctx, d)
	case "review-code":
		return s.planCodeReview(ctx, d)
	case "verify-feature":
		return s.planVerify(ctx, d)
	case "estimate":
		return s.planEstimate(ctx, d)
	default:
		// review-spec, review-dev-plan: read-only document review (phase 1).
		system, user, turnCap, err := s.buildReview(ctx, d)
		if err != nil {
			return nil, err
		}
		return &dispatch.Plan{
			System: system, User: user, TurnCap: turnCap,
			Tools:           []provider.ToolDef{dispatch.ReviewOutcomeTool()},
			OutcomeTool:     "submit_review",
			ValidateOutcome: validateReview,
		}, nil
	}
}

var _ dispatch.Planner = (*Server)(nil)

func validateReview(raw json.RawMessage) error {
	_, err := rules.ParseReviewOutcome(raw)
	return err
}

func validateVerification(raw json.RawMessage) error {
	_, err := rules.ParseVerificationOutcome(raw)
	return err
}

func validateEstimate(raw json.RawMessage) error {
	_, err := rules.ParseEstimateOutcome(raw)
	return err
}

// planEstimate builds an estimate plan (FR-4.1): the entity's description and
// the corpus reference points retrieved by full-text similarity, offered to
// the estimator role. Read-only — no worktree, one outcome tool. The corpus is
// retrieved here so the estimator sees real prior (estimate, actual) pairs; the
// tier the resulting row gets is assigned when the outcome lands (recordAIEstimate).
func (s *Server) planEstimate(ctx context.Context, d *store.Dispatch) (*dispatch.Plan, error) {
	cfg, err := s.freshConfig()
	if err != nil {
		return nil, err
	}
	name, description, err := s.entityDescription(ctx, d.RefType, d.RefID)
	if err != nil {
		return nil, err
	}
	role, skillBody, err := s.roleAndSkill(d.Role)
	if err != nil {
		return nil, err
	}
	corpus, err := store.RetrieveCorpus(ctx, s.Store.Pool, description, 5)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString("# Work to estimate\n\n")
	fmt.Fprintf(&b, "%s (%s)\n\n%s\n", name, d.RefType, description)
	if len(corpus) == 0 {
		b.WriteString("\n# Reference points\n\nNone: no similar completed work is in the calibration corpus yet. Estimate from judgement.\n")
	} else {
		b.WriteString("\n# Reference points from completed work (description → estimate vs actual)\n\n")
		for _, c := range corpus {
			fmt.Fprintf(&b, "- %s: estimated %d tokens (%s), actually consumed %d tokens\n  %s\n",
				c.Name, c.EstimateTokens, c.EstimateTier, c.ActualTokens, firstLine(c.Description))
		}
		b.WriteString("\nUse these as calibration anchors: prefer them over raw intuition where the work is similar.\n")
	}
	b.WriteString("\n# Your task\n\nEstimate the tokens this work will consume, then call submit_estimate with the number and your reasoning. Cite the reference points you leaned on.\n")

	turnCap := cfg.Dispatch.TurnCap
	if role.Limits != nil && role.Limits.TurnCap > 0 {
		turnCap = role.Limits.TurnCap
	}
	return &dispatch.Plan{
		System: identityWithSkill(role.Identity, skillBody), User: b.String(), TurnCap: turnCap,
		Tools:           []provider.ToolDef{dispatch.EstimateOutcomeTool()},
		OutcomeTool:     "submit_estimate",
		ValidateOutcome: validateEstimate,
	}, nil
}

// entityDescription returns a feature's or task's display name and description
// for estimation and corpus retrieval.
func (s *Server) entityDescription(ctx context.Context, refType string, refID uuid.UUID) (name, description string, err error) {
	switch refType {
	case "feature":
		f, err := store.GetFeature(ctx, s.Store.Pool, refID)
		if err != nil {
			return "", "", err
		}
		return f.Name, f.Description, nil
	case "task":
		t, err := store.GetTask(ctx, s.Store.Pool, refID)
		if err != nil {
			return "", "", err
		}
		return t.Title, t.Description, nil
	}
	return "", "", fmt.Errorf("estimate: unsupported ref_type %q", refType)
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// toolContextForFeature builds the worktree-scoped ToolContext for a
// dispatch, resolving the feature's live worktree and the role's profile and
// the project's allowed commands (DESIGN-006 §4.2).
func (s *Server) toolContextForFeature(ctx context.Context, cfg *config.Config, featureID, taskID string, role *config.Role) (*toolhost.Context, error) {
	fid, err := uuid.Parse(featureID)
	if err != nil {
		return nil, err
	}
	wt, err := store.LiveWorktreeForFeature(ctx, s.Store.Pool, fid)
	if err != nil {
		return nil, fmt.Errorf("feature has no worktree: %w", err)
	}
	root := wt.Path
	if !filepath.IsAbs(root) {
		root = filepath.Join(s.RepoRoot, root)
	}
	commands := map[string]toolhost.CommandSpec{}
	for name, c := range cfg.Commands {
		commands[name] = toolhost.CommandSpec{
			Argv: c.Argv, TimeoutSeconds: c.TimeoutSeconds, OutputCapBytes: c.OutputCapBytes,
		}
	}
	profile := map[string]bool{}
	for _, t := range role.Tools {
		profile[t] = true
	}
	return &toolhost.Context{
		WorktreeRoot: root, FeatureID: featureID, TaskID: taskID,
		Commands: commands, Profile: profile,
	}, nil
}

// planImplement builds an implement-task plan: the contract, the task, and
// the full tool profile in the worktree.
func (s *Server) planImplement(ctx context.Context, d *store.Dispatch) (*dispatch.Plan, error) {
	cfg, err := s.freshConfig()
	if err != nil {
		return nil, err
	}
	task, err := store.GetTask(ctx, s.Store.Pool, d.RefID)
	if err != nil {
		return nil, err
	}
	role, skillBody, err := s.roleAndSkill(d.Role)
	if err != nil {
		return nil, err
	}
	spec, devPlan, err := s.contractBodies(ctx, task.FeatureID)
	if err != nil {
		return nil, err
	}
	tctx, err := s.toolContextForFeature(ctx, cfg, task.FeatureID.String(), task.ID.String(), role)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString("# Task to implement\n\n")
	fmt.Fprintf(&b, "%s: %s\n\n%s\n", task.LocalID, task.Title, task.Description)
	b.WriteString("\n# Specification (the contract)\n\n" + spec + "\n")
	b.WriteString("\n# Dev-plan (how the feature is built)\n\n" + devPlan + "\n")
	b.WriteString("\n# How to work\n\n")
	b.WriteString("You are working in an isolated git worktree. Use read_file (with hash_tag for editing), edit_file, write_file, list_files, and run_command (only the project's allowed commands). Implement exactly this task — not the whole feature. When the code is complete and builds, call submit_implementation.\n")

	turnCap := cfg.Dispatch.TurnCap
	if role.Limits != nil && role.Limits.TurnCap > 0 {
		turnCap = role.Limits.TurnCap
	}
	tools := append(dispatch.ProfileToolDefs(role.Tools), dispatch.ImplementationOutcomeTool())
	return &dispatch.Plan{
		System: identityWithSkill(role.Identity, skillBody), User: b.String(), TurnCap: turnCap,
		Tools: tools, OutcomeTool: "submit_implementation", ToolCtx: tctx,
	}, nil
}

// planCodeReview builds a review-code plan: the contract, the task, and the
// task's diff, with a read-only worktree profile.
func (s *Server) planCodeReview(ctx context.Context, d *store.Dispatch) (*dispatch.Plan, error) {
	cfg, err := s.freshConfig()
	if err != nil {
		return nil, err
	}
	task, err := store.GetTask(ctx, s.Store.Pool, d.RefID)
	if err != nil {
		return nil, fmt.Errorf("code-review plan: load task %s: %w", d.RefID, err)
	}
	role, skillBody, err := s.roleAndSkill(d.Role)
	if err != nil {
		return nil, err
	}
	spec, devPlan, err := s.contractBodies(ctx, task.FeatureID)
	if err != nil {
		return nil, err
	}
	tctx, err := s.toolContextForFeature(ctx, cfg, task.FeatureID.String(), task.ID.String(), role)
	if err != nil {
		return nil, fmt.Errorf("code-review plan: tool context: %w", err)
	}
	diff := s.taskDiff(tctx.WorktreeRoot, task.BaseCommit)

	var b strings.Builder
	b.WriteString("# Task under review\n\n")
	fmt.Fprintf(&b, "%s: %s\n\n%s\n", task.LocalID, task.Title, task.Description)
	b.WriteString("\n# Specification\n\n" + spec + "\n")
	b.WriteString("\n# Dev-plan\n\n" + devPlan + "\n")
	b.WriteString("\n# The diff to review\n\n```diff\n" + diff + "\n```\n")
	b.WriteString("\nJudge whether this diff correctly and completely implements the task against the spec and dev-plan. You may read surrounding files. Complete your review by calling submit_review.\n")

	turnCap := cfg.Dispatch.TurnCap
	if role.Limits != nil && role.Limits.TurnCap > 0 {
		turnCap = role.Limits.TurnCap
	}
	tools := append(dispatch.ProfileToolDefs(role.Tools), dispatch.ReviewOutcomeTool())
	return &dispatch.Plan{
		System: identityWithSkill(role.Identity, skillBody), User: b.String(), TurnCap: turnCap,
		Tools: tools, OutcomeTool: "submit_review", ValidateOutcome: validateReview, ToolCtx: tctx,
	}, nil
}

// planVerify builds a verify-feature plan: clean context — the acceptance
// criteria only, not the dev-plan or implementation narrative (L-4) — with a
// read-only worktree profile.
func (s *Server) planVerify(ctx context.Context, d *store.Dispatch) (*dispatch.Plan, error) {
	cfg, err := s.freshConfig()
	if err != nil {
		return nil, err
	}
	feature, err := store.GetFeature(ctx, s.Store.Pool, d.RefID)
	if err != nil {
		return nil, err
	}
	role, skillBody, err := s.roleAndSkill(d.Role)
	if err != nil {
		return nil, err
	}
	spec, _, err := s.contractBodies(ctx, feature.ID)
	if err != nil {
		return nil, err
	}
	tctx, err := s.toolContextForFeature(ctx, cfg, feature.ID.String(), "", role)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	b.WriteString("# Feature to verify\n\n" + feature.Name + "\n")
	b.WriteString("\n# Specification (with acceptance criteria)\n\n" + spec + "\n")
	b.WriteString("\n# Your task\n\n")
	b.WriteString("The implementation is complete and on the worktree branch. Check the code against each acceptance criterion in the specification. You did not write this code; judge the result against the contract, not any implementation story. You may read files and run the project's allowed commands (e.g. the tests). Report each criterion met/unmet with evidence, then call submit_verification.\n")

	turnCap := cfg.Dispatch.TurnCap
	if role.Limits != nil && role.Limits.TurnCap > 0 {
		turnCap = role.Limits.TurnCap
	}
	tools := append(dispatch.ProfileToolDefs(role.Tools), dispatch.VerificationOutcomeTool())
	return &dispatch.Plan{
		System: identityWithSkill(role.Identity, skillBody), User: b.String(), TurnCap: turnCap,
		Tools: tools, OutcomeTool: "submit_verification", ValidateOutcome: validateVerification, ToolCtx: tctx,
	}, nil
}

// ---- helpers ----

func (s *Server) roleAndSkill(roleName string) (*config.Role, string, error) {
	role, err := config.LoadRole(s.CompartmentRoot, roleName)
	if err != nil {
		return nil, "", err
	}
	skillBody := ""
	if role.Skill != "" {
		skill, err := config.LoadSkill(s.CompartmentRoot, role.Skill)
		if err != nil {
			return nil, "", err
		}
		skillBody = skill.Body
	}
	return role, skillBody, nil
}

func identityWithSkill(identity, skillBody string) string {
	s := strings.TrimSpace(identity)
	if skillBody != "" {
		s += "\n\n# Procedure\n\n" + strings.TrimSpace(skillBody)
	}
	return s
}

// contractBodies returns the feature's current approved spec and dev-plan
// bodies (empty string if a document is absent — the caller decides whether
// that matters for the purpose).
func (s *Server) contractBodies(ctx context.Context, featureID uuid.UUID) (spec, devPlan string, err error) {
	if d, e := store.CurrentDocForOwner(ctx, s.Store.Pool, "spec", "feature", featureID); e == nil {
		if body, re := s.readDocFile(d.Path); re == nil {
			spec = string(body)
		}
	}
	if d, e := store.CurrentDocForOwner(ctx, s.Store.Pool, "dev_plan", "feature", featureID); e == nil {
		if body, re := s.readDocFile(d.Path); re == nil {
			devPlan = string(body)
		}
	}
	return spec, devPlan, nil
}

// taskDiff returns the task's whole contribution: the diff from the branch
// base recorded when the task's work began to the current HEAD. This spans
// every implement dispatch for the task (rework included), so review never
// misses a change introduced in an earlier commit (DESIGN-006 §5). Falls
// back to the last commit if no base was recorded.
func (s *Server) taskDiff(worktreeRoot, baseCommit string) string {
	if baseCommit != "" {
		if out, err := gitIn(worktreeRoot, "diff", baseCommit, "HEAD"); err == nil {
			return out
		}
	}
	if out, err := gitIn(worktreeRoot, "diff", "HEAD~1", "HEAD"); err == nil && strings.TrimSpace(out) != "" {
		return out
	}
	out, _ := gitIn(worktreeRoot, "show", "--format=", "HEAD")
	return out
}

// gitIn runs a git command in dir and returns stdout.
func gitIn(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	return string(out), err
}
