package server

// Phase-2 integration suite (SPEC-002 §3): the implementation loop end to
// end with a mock provider driving real tool calls against real worktrees.
// Reuses the phase-1 harness.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"cromwell/internal/lifecycle"
	"cromwell/internal/provider"
	"cromwell/internal/store"
)

const devPlanOneTask = `---
title: Login form — dev plan
type: dev_plan
owner: auth/login
---

# Login form — dev plan

## Approach

Add a single greeting helper as a stand-in for the login handler.

## Tasks

| id | title | depends_on | description |
|----|-------|------------|-------------|
| T1 | Greeting helper | | Add greet.go with a Greet function returning a welcome string |
`

// approveDoc drives one document (spec or dev-plan) through submit → mock
// approve → approved.
func (h *harness) approveDoc(path string) {
	h.t.Helper()
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 10, Output: 10})
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": path}); code != 200 {
		h.t.Fatalf("submit %s: %d %v", path, code, out)
	}
	h.eventually("approved "+path, func() bool { return h.docState(path) == lifecycle.DocApproved })
}

// addDevPlan writes, registers, and returns the dev-plan path.
func (h *harness) addDevPlan(body string) string {
	h.t.Helper()
	path := "docs/plans/login.md"
	if err := os.MkdirAll(filepath.Join(h.root, "docs/plans"), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, path), []byte(body), 0o644); err != nil {
		h.t.Fatal(err)
	}
	git(h.t, h.root, "add", "-A")
	git(h.t, h.root, "commit", "-qm", "dev-plan draft")
	if code, out := h.call("POST", "/api/docs", map[string]string{
		"path": path, "type": "dev_plan", "owner_type": "feature", "owner_ref": "auth/login"}); code != 201 {
		h.t.Fatalf("register dev-plan: %d %v", code, out)
	}
	return path
}

func (h *harness) tasks(feature string) []store.Task {
	h.t.Helper()
	f, err := h.srv.featureByPath(context.Background(), feature)
	if err != nil {
		h.t.Fatal(err)
	}
	ts, err := store.TasksForFeature(context.Background(), h.srv.Store.Pool, f.ID)
	if err != nil {
		h.t.Fatal(err)
	}
	return ts
}

// TestFullImplementationLoop is the phase-2 vertical slice: approved spec +
// dev-plan → decompose → start → implement → code-review → verify → merge →
// done, no human in the loop (SPEC-002 §1).
func TestFullImplementationLoop(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	h.approveDoc(specPath)
	// G1 needs both: after spec only, the feature is still idea.
	if h.featureState("auth/login") != lifecycle.FeatIdea {
		t.Fatalf("feature should still be idea after spec-only approval")
	}

	devPlan := h.addDevPlan(devPlanOneTask)
	h.approveDoc(devPlan)
	// Decomposition + G1 → ready, with one task.
	h.eventually("feature ready with a task", func() bool {
		return h.featureState("auth/login") == lifecycle.FeatReady && len(h.tasks("auth/login")) == 1
	})

	// Script the whole execution chain (FIFO, all sequential for one feature):
	// implement (write_file + submit_implementation), code review, verify.
	h.mock.RespondToolUse("write_file",
		`{"path":"greet.go","content":"package main\n\nfunc Greet() string { return \"welcome\" }\n"}`,
		provider.Usage{Input: 50, Output: 20})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"added greet.go","files_changed":["greet.go"]}`, provider.Usage{Input: 30, Output: 10})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"correct and in scope"}`, provider.Usage{Input: 40, Output: 10})
	h.mock.RespondOutcome("submit_verification", `{"criteria":[{"id":"AC1","met":true,"evidence":"greet.go present"}],"verdict":"approve","reasoning":"met"}`, provider.Usage{Input: 40, Output: 15})

	// Start the feature: worktree + dispatch.
	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		t.Fatalf("start: %d %v", code, out)
	}

	h.eventually("feature done", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })

	// The implemented file is on main (merged, FR-9.2).
	if _, err := os.Stat(filepath.Join(h.root, "greet.go")); err != nil {
		t.Errorf("greet.go should be merged to main: %v", err)
	}
	// The task is done.
	if ts := h.tasks("auth/login"); len(ts) != 1 || ts[0].State != lifecycle.TaskDone {
		t.Errorf("task should be done: %+v", ts)
	}
	// The tool-call ledger recorded the implementer's write_file (FR-6.5).
	var toolCalls int
	_ = h.srv.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM tool_calls`).Scan(&toolCalls)
	if toolCalls < 1 {
		t.Errorf("tool_calls ledger should have the write_file call, got %d", toolCalls)
	}
	// Cost accrued across all four execution dispatches plus two reviews.
	rollup, _ := h.srv.Store.CostRollup(context.Background())
	var total float64
	for _, r := range rollup {
		total += r.CostUSD
	}
	if total <= 0 {
		t.Errorf("cost should be positive across the loop, got %f", total)
	}

	// The worktree is GC'd (heartbeat runs it; trigger once).
	h.srv.GCWorktrees(context.Background())
	if _, err := store.LiveWorktreeForFeature(context.Background(), h.srv.Store.Pool, mustFeatureID(t, h, "auth/login")); err != store.ErrNotFound {
		t.Errorf("worktree should be GC'd after done")
	}
}

// TestDependencyOrdering is FR-4.2: a two-task chain dispatches T1 first; T2
// becomes ready only when T1 is done.
func TestDependencyOrdering(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)

	twoTasks := strings.Replace(devPlanOneTask,
		"| T1 | Greeting helper | | Add greet.go with a Greet function returning a welcome string |\n",
		"| T1 | First | | Add a.go with package main |\n| T2 | Second | T1 | Add b.go with a Bee function |\n", 1)
	devPlan := h.addDevPlan(twoTasks)
	h.approveDoc(devPlan)
	// Wait for the feature to reach ready (G1) AND its tasks to exist:
	// decomposition and the G1 advance are separate committed actions.
	h.eventually("ready with two tasks", func() bool {
		return h.featureState("auth/login") == lifecycle.FeatReady && len(h.tasks("auth/login")) == 2
	})

	// Only T1 is ready initially (T2 depends on it).
	ts := h.tasks("auth/login")
	byID := map[string]store.Task{}
	for _, tk := range ts {
		byID[tk.LocalID] = tk
	}
	if byID["T1"].State != lifecycle.TaskReady || byID["T2"].State != lifecycle.TaskPending {
		t.Fatalf("T1 ready, T2 pending expected: T1=%s T2=%s", byID["T1"].State, byID["T2"].State)
	}

	// Script both tasks' full chains in order: T1 impl+review, T2 impl+review,
	// then verify.
	h.mock.RespondToolUse("write_file", `{"path":"a.go","content":"package main\n"}`, provider.Usage{Input: 20, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"a.go"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondToolUse("write_file", `{"path":"b.go","content":"package main\n\nfunc Bee() {}\n"}`, provider.Usage{Input: 20, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"b.go"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_verification", `{"criteria":[{"id":"AC1","met":true}],"verdict":"approve","reasoning":"met"}`, provider.Usage{Input: 20, Output: 5})

	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		t.Fatalf("start: %d %v", code, out)
	}
	h.eventually("feature done via both tasks", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })

	// Both files landed; the second depended on the first completing.
	for _, f := range []string{"a.go", "b.go"} {
		if _, err := os.Stat(filepath.Join(h.root, f)); err != nil {
			t.Errorf("%s should be merged: %v", f, err)
		}
	}
}

// TestCodeReviewRequestChanges is FR-8.1: request_changes returns the task to
// active and re-dispatches the implementer against the kept worktree.
func TestCodeReviewRequestChanges(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)
	devPlan := h.addDevPlan(devPlanOneTask)
	h.approveDoc(devPlan)
	h.eventually("ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })

	// Round 1: implement then request_changes.
	h.mock.RespondToolUse("write_file", `{"path":"greet.go","content":"package main\n"}`, provider.Usage{Input: 20, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"v1"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"request_changes","comments":[{"body":"missing Greet function"}],"reasoning":"incomplete"}`, provider.Usage{Input: 10, Output: 5})
	// Round 2: implement again then approve, then verify.
	h.mock.RespondToolUse("write_file", `{"path":"greet.go","content":"package main\n\nfunc Greet() string { return \"hi\" }\n"}`, provider.Usage{Input: 20, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"v2 with Greet"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"now correct"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_verification", `{"criteria":[{"id":"AC1","met":true}],"verdict":"approve","reasoning":"met"}`, provider.Usage{Input: 10, Output: 5})

	h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"})
	h.eventually("done after rework", func() bool { return h.featureState("auth/login") == lifecycle.FeatDone })

	// The final merged file has the second version.
	body, err := os.ReadFile(filepath.Join(h.root, "greet.go"))
	if err != nil || !strings.Contains(string(body), "func Greet") {
		t.Errorf("merged file should be the reworked version: %v %s", err, body)
	}
	// There were two implement dispatches for the one task (re-dispatch).
	var implCount int
	_ = h.srv.Store.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM dispatches WHERE purpose = 'implement-task'`).Scan(&implCount)
	if implCount != 2 {
		t.Errorf("expected 2 implement dispatches (v1 + rework), got %d", implCount)
	}
}

// TestVerificationRequestChanges is FR-9.2: an unmet criterion becomes a task
// and the feature returns to active.
func TestVerificationRequestChanges(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)
	devPlan := h.addDevPlan(devPlanOneTask)
	h.approveDoc(devPlan)
	h.eventually("ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })

	h.mock.RespondToolUse("write_file", `{"path":"greet.go","content":"package main\n"}`, provider.Usage{Input: 20, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"v1"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 10, Output: 5})
	// Verification finds a gap → one unmet criterion.
	h.mock.RespondOutcome("submit_verification", `{"criteria":[{"id":"AC1","met":false,"evidence":"lockout not implemented"}],"verdict":"request_changes","reasoning":"one gap"}`, provider.Usage{Input: 20, Output: 5})

	h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"})

	h.eventually("feature back to active with a new task", func() bool {
		if h.featureState("auth/login") != lifecycle.FeatActive {
			return false
		}
		for _, tk := range h.tasks("auth/login") {
			if strings.HasPrefix(tk.LocalID, "V") {
				return true
			}
		}
		return false
	})
}

// TestRevisionInFlight is FR-10.1: revising a spec of an active feature blocks
// new dispatches and raises the checkpoint.
func TestRevisionInFlight(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)
	devPlan := h.addDevPlan(devPlanOneTask)
	h.approveDoc(devPlan)
	h.eventually("ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })

	// Start, but do not script the implementer yet — so the implement
	// dispatch will sit (its provider call will fail for lack of a scripted
	// response, but the point is the feature is active). Instead, script the
	// implementer to hang by giving no response and asserting revision
	// blocking at the state level.
	h.mock.RespondToolUse("write_file", `{"path":"greet.go","content":"package main\n"}`, provider.Usage{Input: 20, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"v1"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 10, Output: 5})
	h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"})
	// Let the one task finish so the feature is active with a done task.
	h.eventually("task done, feature active or review", func() bool {
		for _, tk := range h.tasks("auth/login") {
			if tk.State == lifecycle.TaskDone {
				return true
			}
		}
		return false
	})

	// Revise the spec and submit the successor while the feature is in flight.
	code, out := h.call("POST", "/api/docs/revise", map[string]string{"path": specPath})
	if code != 201 {
		t.Fatalf("revise: %d %v", code, out)
	}
	revPath := out["Path"].(string)
	revised := strings.Replace(validSpec, "fifteen minutes", "twenty minutes", 1)
	if err := os.WriteFile(filepath.Join(h.root, revPath), []byte(revised), 0o644); err != nil {
		t.Fatal(err)
	}
	git(h.t, h.root, "add", "-A")
	git(h.t, h.root, "commit", "-qm", "spec revision")
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 10, Output: 5})
	h.call("POST", "/api/docs/submit", map[string]string{"path": revPath})

	// spec_stale set and a revision-in-flight checkpoint raised.
	h.eventually("revision-in-flight checkpoint", func() bool {
		for _, cp := range h.callList("GET", "/api/inbox") {
			if cp["Kind"] == "revision-in-flight" {
				return true
			}
		}
		return false
	})
	stale, _ := store.FeatureSpecStale(context.Background(), h.srv.Store.Pool, mustFeatureID(t, h, "auth/login"))
	if !stale {
		t.Error("feature should be spec_stale during a revision in flight")
	}
}

// TestDevPlanCycleRejected is FR-1.2: a dev-plan whose task table has a cycle
// is rejected at submit.
func TestDevPlanCycleRejected(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)

	cyclic := strings.Replace(devPlanOneTask,
		"| T1 | Greeting helper | | Add greet.go with a Greet function returning a welcome string |\n",
		"| T1 | First | T2 | needs T2 |\n| T2 | Second | T1 | needs T1 |\n", 1)
	devPlan := h.addDevPlan(cyclic)
	code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": devPlan})
	if code != 422 {
		t.Fatalf("cyclic dev-plan should be rejected at submit: %d %v", code, out)
	}
	if !strings.Contains(fmt.Sprint(out), "cycle") {
		t.Errorf("rejection should mention the cycle: %v", out)
	}
}

// TestMergeConflict is FR-9.3: a branch that does not merge cleanly raises a
// merge-conflict checkpoint; the feature stays in review, unmerged.
func TestMergeConflict(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.approveDoc(specPath)
	// The task edits README.md, which we will also change on main to force a
	// conflict at merge time.
	conflictPlan := strings.Replace(devPlanOneTask,
		"| T1 | Greeting helper | | Add greet.go with a Greet function returning a welcome string |\n",
		"| T1 | Edit readme | | Change README.md heading |\n", 1)
	devPlan := h.addDevPlan(conflictPlan)
	h.approveDoc(devPlan)
	h.eventually("ready", func() bool { return h.featureState("auth/login") == lifecycle.FeatReady })

	// The implementer rewrites README.md on the branch.
	h.mock.RespondToolUse("write_file", `{"path":"README.md","content":"# branch version\n"}`, provider.Usage{Input: 20, Output: 5})
	h.mock.RespondOutcome("submit_implementation", `{"summary":"readme"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 10, Output: 5})
	h.mock.RespondOutcome("submit_verification", `{"criteria":[{"id":"AC1","met":true}],"verdict":"approve","reasoning":"met"}`, provider.Usage{Input: 10, Output: 5})

	if code, out := h.call("POST", "/api/features/start", map[string]string{"path": "auth/login"}); code != 200 {
		t.Fatalf("start: %d %v", code, out)
	}

	// Advance main under the feature so README.md conflicts with the branch.
	if err := os.WriteFile(filepath.Join(h.root, "README.md"), []byte("# main version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(h.t, h.root, "commit", "-aqm", "conflicting main change")

	h.eventually("merge-conflict checkpoint", func() bool {
		for _, cp := range h.callList("GET", "/api/inbox") {
			if cp["Kind"] == "merge-conflict" {
				return true
			}
		}
		return false
	})
	// The feature did not merge: it stays in review.
	if got := h.featureState("auth/login"); got != lifecycle.FeatReview {
		t.Errorf("feature should stay in review on conflict, got %s", got)
	}
	if body, _ := os.ReadFile(filepath.Join(h.root, "README.md")); !strings.Contains(string(body), "main version") {
		t.Errorf("main README should be unchanged by the failed merge")
	}
}

func mustFeatureID(t *testing.T, h *harness, path string) uuid.UUID {
	t.Helper()
	f, err := h.srv.featureByPath(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	return f.ID
}
