package server

// End-to-end integration suite (SPEC-001 §3): real Postgres, real HTTP
// handlers, real git repo, mock provider. Each test inits a fresh project
// via the actual starter pack (FR-1.1) and drives the API the CLI uses.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
	"subutai/internal/provider"
	"subutai/internal/starter"
	"subutai/internal/store"
	"subutai/internal/testdb"
)

type harness struct {
	t      *testing.T
	root   string
	srv    *Server
	api    *httptest.Server
	mock   *provider.Mock
	cancel context.CancelFunc
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	url := testdb.URL(t, "subutai_server_test")
	t.Setenv("SUBUTAI_DATABASE_URL", url)
	t.Setenv("ANTHROPIC_API_KEY", "sk-test-unused") // mock provider; key never used

	// Fresh database.
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	_ = conn.Close(ctx)

	// Fresh git repo, initialised by the real init path (FR-1.1).
	root := t.TempDir()
	git(t, root, "init", "-q")
	git(t, root, "config", "user.email", "test@test")
	git(t, root, "config", "user.name", "tester")
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("# test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, root, "add", "-A")
	git(t, root, "commit", "-qm", "initial")
	if err := starter.Init(ctx, root, "/usr/bin/true"); err != nil {
		t.Fatalf("init: %v", err)
	}
	// A second init must refuse (FR-1.2 / D-3).
	if err := starter.Init(ctx, root, "/usr/bin/true"); err == nil {
		t.Fatal("second init should refuse")
	}

	runCtx, cancel := context.WithCancel(ctx)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv, err := New(runCtx, root, log)
	if err != nil {
		t.Fatal(err)
	}
	mock := &provider.Mock{}
	srv.Dispatcher.Providers = func(string) (provider.Provider, error) { return mock, nil }
	srv.Dispatcher.BackoffBase = time.Millisecond

	orchDone := make(chan struct{})
	dispDone := make(chan struct{})
	go func() { defer close(orchDone); srv.orchestrate(runCtx) }()
	go func() { defer close(dispDone); srv.Dispatcher.Run(runCtx) }()
	srv.Dispatcher.Kick()

	api := httptest.NewServer(srv.routes())
	h := &harness{t: t, root: root, srv: srv, api: api, mock: mock, cancel: cancel}
	t.Cleanup(func() {
		api.Close()
		cancel()
		// Drain before TempDir removal: an in-flight action may be running
		// a server-authored git commit inside the repo.
		<-orchDone
		<-dispDone
		srv.Store.Close()
	})
	return h
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func (h *harness) call(method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var reader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		reader = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, h.api.URL+path, reader)
	req.Header.Set("X-Subutai-Actor", "sam")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]any
	_ = json.Unmarshal(data, &out)
	return resp.StatusCode, out
}

func (h *harness) callList(method, path string) []map[string]any {
	h.t.Helper()
	req, _ := http.NewRequest(method, h.api.URL+path, nil)
	req.Header.Set("X-Subutai-Actor", "sam")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, _ := io.ReadAll(resp.Body)
	var out []map[string]any
	_ = json.Unmarshal(data, &out)
	return out
}

func (h *harness) eventually(what string, cond func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for %s", what)
}

const validSpec = `---
title: Login form
type: spec
owner: auth/login
---

# Login form

## Overview

Users sign in with an email address and password.

## Behaviour

Submitting valid credentials creates a session cookie. Invalid credentials
show an inline error without clearing the email field. Five consecutive
failures locks the account for fifteen minutes.

## Acceptance criteria

- Valid credentials produce a session cookie and redirect to the dashboard
- Invalid credentials show an error and preserve the email field
- The sixth attempt within fifteen minutes is rejected with a lockout notice
`

// setupFeatureWithSpec creates auth → login and registers the spec file.
func (h *harness) setupFeatureWithSpec() string {
	h.t.Helper()
	if code, out := h.call("POST", "/api/initiatives", map[string]string{"slug": "auth", "name": "Authentication"}); code != 201 {
		h.t.Fatalf("create initiative: %d %v", code, out)
	}
	if code, out := h.call("POST", "/api/features", map[string]string{
		"initiative_path": "auth", "slug": "login", "name": "Login form"}); code != 201 {
		h.t.Fatalf("create feature: %d %v", code, out)
	}
	specPath := "docs/specs/login.md"
	if err := os.MkdirAll(filepath.Join(h.root, "docs/specs"), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, specPath), []byte(validSpec), 0o644); err != nil {
		h.t.Fatal(err)
	}
	git(h.t, h.root, "add", "-A")
	git(h.t, h.root, "commit", "-qm", "spec draft")
	if code, out := h.call("POST", "/api/docs", map[string]string{
		"path": specPath, "type": "spec", "owner_type": "feature", "owner_ref": "auth/login"}); code != 201 {
		h.t.Fatalf("register doc: %d %v", code, out)
	}
	return specPath
}

func (h *harness) docState(path string) lifecycle.DocumentState {
	h.t.Helper()
	doc, err := store.LiveDocumentByPath(context.Background(), h.srv.Store.Pool, path)
	if err != nil {
		return "missing"
	}
	return doc.State
}

func (h *harness) featureState(path string) lifecycle.FeatureState {
	h.t.Helper()
	f, err := h.srv.featureByPath(context.Background(), path)
	if err != nil {
		h.t.Fatal(err)
	}
	return f.State
}

// TestApprovePathEndToEnd is the spec-review slice: submit → validate →
// agent-review approve → G1 evaluated, with the complete audit sequence and
// exact cost (FR-5.4, FR-7.1, FR-7.2). Under the phase-2 two-part contract
// (FR-2.1) the spec alone does not advance the feature — G1 holds at idea
// until the dev-plan is also approved (see TestFullImplementationLoop for the
// full contract path).
func TestApprovePathEndToEnd(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	h.mock.RespondOutcome("submit_review",
		`{"verdict":"approve","reasoning":"clear, complete, testable"}`,
		provider.Usage{Input: 1000, Output: 200})

	code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})
	if code != 200 {
		t.Fatalf("submit: %d %v", code, out)
	}

	h.eventually("G1 evaluated", func() bool {
		events, _ := h.srv.Store.AuditTail(context.Background(), "", nil, 100)
		for _, e := range events {
			if e.Kind == "gate.evaluated" {
				return true
			}
		}
		return false
	})
	if got := h.docState(specPath); got != lifecycle.DocApproved {
		t.Errorf("doc state = %s, want approved", got)
	}
	// Spec approved but dev-plan absent: G1 fails, feature stays idea (FR-2.1).
	if got := h.featureState("auth/login"); got != lifecycle.FeatIdea {
		t.Errorf("feature state = %s, want idea (dev-plan not yet approved)", got)
	}

	// FR-7.1: the complete audit sequence, no gaps.
	events, err := h.srv.Store.AuditTail(context.Background(), "", nil, 100)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, e := range events {
		kinds = append(kinds, e.Kind)
	}
	want := []string{
		"initiative.created", "feature.created", "document.registered",
		"document.validated", "document.transition", // submit
		"dispatch.queued", "dispatch.running", "dispatch.succeeded",
		"document.transition", // approve
		"gate.evaluated",      // G1 evaluated (fails: dev-plan absent), no feature transition
	}
	if fmt.Sprint(kinds) != fmt.Sprint(want) {
		t.Errorf("audit sequence:\n got %v\nwant %v", kinds, want)
	}

	// FR-3.2: gate.evaluated precedes the feature transition.
	// FR-7.2: cost = snapshot prices × reported tokens.
	// 1000 in × $3/M + 200 out × $15/M = 0.006
	rollup, err := h.srv.Store.CostRollup(context.Background())
	if err != nil || len(rollup) != 1 {
		t.Fatalf("rollup: %v %v", rollup, err)
	}
	if diff := rollup[0].CostUSD - 0.006; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("cost = %f, want 0.006", rollup[0].CostUSD)
	}

	// FR-5.3: deterministic prompt block order.
	if len(h.mock.Requests) != 1 {
		t.Fatalf("provider calls = %d", len(h.mock.Requests))
	}
	req := h.mock.Requests[0]
	if !strings.Contains(req.System, "specification reviewer") || !strings.Contains(req.System, "# Procedure") {
		t.Error("system prompt missing role identity or skill procedure")
	}
	user := req.Messages[0].Blocks[0].Text
	order := []string{"# Project", "# Feature under review", "# Task", "## Validation report", "## Document under review"}
	last := -1
	for _, marker := range order {
		i := strings.Index(user, marker)
		if i < 0 || i < last {
			t.Errorf("prompt block %q missing or out of order", marker)
		}
		last = i
	}
}

// TestValidationBlocksSubmit is FR-5.2: a spec missing its acceptance
// criteria is rejected with the section named, and no dispatch is queued.
func TestValidationBlocksSubmit(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	bad := strings.Replace(validSpec, "## Acceptance criteria", "## Criteria", 1)
	if err := os.WriteFile(filepath.Join(h.root, specPath), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})
	if code != 422 {
		t.Fatalf("submit should 422: %d %v", code, out)
	}
	if !strings.Contains(fmt.Sprint(out), "Acceptance criteria") {
		t.Errorf("report should name the section: %v", out)
	}
	if h.docState(specPath) != lifecycle.DocDraft {
		t.Error("doc must stay draft")
	}
	queued, _ := h.srv.Store.QueuedDispatches(context.Background())
	if len(queued) != 0 {
		t.Error("no dispatch may be queued on validation failure")
	}
}

// TestRequestChangesRound covers FR-5.4 request_changes and FR-5.5: the
// second-round prompt contains the first round's comments.
func TestRequestChangesRound(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	h.mock.RespondOutcome("submit_review",
		`{"verdict":"request_changes","comments":[{"section_ref":"Behaviour","body":"Define the lockout reset rule"}],"reasoning":"one gap"}`,
		provider.Usage{Input: 900, Output: 150})

	h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})
	h.eventually("doc back to draft", func() bool { return h.docState(specPath) == lifecycle.DocDraft })

	// Comments visible (FR-5.4).
	comments := h.callList("GET", "/api/docs/comments?path="+specPath)
	if len(comments) != 1 || !strings.Contains(fmt.Sprint(comments[0]), "lockout reset") {
		t.Fatalf("comments: %v", comments)
	}

	// Author edits and resubmits; round 2 approves.
	improved := strings.Replace(validSpec, "locks the account for fifteen minutes.",
		"locks the account for fifteen minutes; the counter resets on success.", 1)
	if err := os.WriteFile(filepath.Join(h.root, specPath), []byte(improved), 0o644); err != nil {
		t.Fatal(err)
	}
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"approve","reasoning":"comment addressed"}`,
		provider.Usage{Input: 950, Output: 100})
	h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})
	h.eventually("approved", func() bool { return h.docState(specPath) == lifecycle.DocApproved })

	if len(h.mock.Requests) != 2 {
		t.Fatalf("provider calls = %d", len(h.mock.Requests))
	}
	round2 := h.mock.Requests[1].Messages[0].Blocks[0].Text
	if !strings.Contains(round2, "Unresolved comments from prior review rounds") ||
		!strings.Contains(round2, "Define the lockout reset rule") {
		t.Error("second-round prompt must include the first-round comment thread (FR-5.5)")
	}
}

// TestEscalatePath covers FR-5.4 escalate and FR-6: checkpoint with
// reasoning, doc stays reviewing, human answer advances it.
func TestEscalatePath(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	h.mock.RespondOutcome("submit_review",
		`{"verdict":"escalate","reasoning":"lockout policy is a product decision"}`,
		provider.Usage{Input: 800, Output: 120})
	h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})

	var checkpointID string
	h.eventually("escalation checkpoint", func() bool {
		pending := h.callList("GET", "/api/inbox")
		for _, cp := range pending {
			if cp["Kind"] == "review-escalation" {
				checkpointID = cp["ID"].(string)
				return true
			}
		}
		return false
	})
	if h.docState(specPath) != lifecycle.DocReviewing {
		t.Error("doc must stay reviewing while escalated")
	}

	// Replaying the triggering event must not duplicate (FR-6.2) — the
	// idempotency is in checkpoint creation itself, covered in store tests;
	// here assert exactly one pending.
	if pending := h.callList("GET", "/api/inbox"); len(pending) != 1 {
		t.Errorf("pending checkpoints = %d, want 1", len(pending))
	}

	code, _ := h.call("POST", "/api/respond", map[string]any{
		"id": checkpointID, "response": map[string]any{"decision": "approve", "reason": "policy is fine"}})
	if code != 200 {
		t.Fatalf("respond: %d", code)
	}
	// The human's approve advances the spec; the feature stays idea until the
	// dev-plan half of the contract is also approved (FR-2.1).
	h.eventually("approved after human answer", func() bool {
		return h.docState(specPath) == lifecycle.DocApproved && h.featureState("auth/login") == lifecycle.FeatIdea
	})
}

// TestBudgetCap is FR-7.3: a cap below one review's projected cost queues
// the dispatch with reason budget and raises one checkpoint; raising the
// cap and responding releases it.
func TestBudgetCap(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	configPath := filepath.Join(h.root, ".subutai/config.yaml")
	original, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	strangled := strings.Replace(string(original), "cap_usd: 50.00", "cap_usd: 0.50", 1)
	if err := os.WriteFile(configPath, []byte(strangled), 0o644); err != nil {
		t.Fatal(err)
	}

	h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})

	var checkpointID string
	h.eventually("budget checkpoint + queued reason", func() bool {
		queued, _ := h.srv.Store.QueuedDispatches(context.Background())
		if len(queued) != 1 || queued[0].QueueReason == nil || *queued[0].QueueReason != "budget" {
			return false
		}
		for _, cp := range h.callList("GET", "/api/inbox") {
			if cp["Kind"] == "budget" {
				checkpointID = cp["ID"].(string)
				return true
			}
		}
		return false
	})

	// Raise the cap (fresh config read, O-6), answer, and the review runs.
	if err := os.WriteFile(configPath, original, 0o644); err != nil {
		t.Fatal(err)
	}
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"approve","reasoning":"fine"}`, provider.Usage{Input: 500, Output: 80})
	h.call("POST", "/api/respond", map[string]any{
		"id": checkpointID, "response": map[string]any{"proceed": true}})
	h.eventually("released and approved", func() bool { return h.docState(specPath) == lifecycle.DocApproved })
}

// TestCrashRecovery is FR-8.1: a stalled running dispatch is failed by the
// sweep, requeued, and completed; the idempotency key admits exactly one
// live dispatch throughout.
func TestCrashRecovery(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	// The first provider call hangs the dispatch by failing transiently
	// until the attempt exhausts in-call retries.
	h.mock.Fail(&provider.TransientError{Status: 529, Err: fmt.Errorf("overloaded")})
	h.mock.Fail(&provider.TransientError{Status: 529, Err: fmt.Errorf("overloaded")})
	h.mock.Fail(&provider.TransientError{Status: 529, Err: fmt.Errorf("overloaded")})
	h.mock.Fail(&provider.TransientError{Status: 529, Err: fmt.Errorf("overloaded")})
	h.mock.RespondOutcome("submit_review",
		`{"verdict":"approve","reasoning":"fine"}`, provider.Usage{Input: 500, Output: 80})

	h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})

	// Attempt 1 fails after backoff (ms-scale in tests); the retry sweep
	// (heartbeat duty) requeues; attempt 2 approves.
	h.eventually("failed attempt recorded", func() bool {
		var n int
		_ = h.srv.Store.Pool.QueryRow(context.Background(),
			`SELECT count(*) FROM dispatches WHERE state = 'failed'`).Scan(&n)
		return n == 1
	})
	h.srv.Dispatcher.RetrySweep(context.Background())
	h.eventually("approved on retry", func() bool { return h.docState(specPath) == lifecycle.DocApproved })

	// Exactly one dispatch row for the idempotency key, attempt 2.
	var count, attempt int
	if err := h.srv.Store.Pool.QueryRow(context.Background(),
		`SELECT count(*), max(attempt) FROM dispatches`).Scan(&count, &attempt); err != nil {
		t.Fatal(err)
	}
	if count != 1 || attempt != 2 {
		t.Errorf("dispatches count=%d attempt=%d; want 1 row at attempt 2", count, attempt)
	}
}

// TestExhaustedRetriesRaiseCheckpoint is FR-8.2's tail: attempts exhausted →
// dispatch-failure checkpoint with the error chain, no silent stall.
func TestExhaustedRetriesRaiseCheckpoint(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()

	for i := 0; i < 12; i++ { // 4 in-call tries × 3 attempts
		h.mock.Fail(&provider.TransientError{Status: 500, Err: fmt.Errorf("boom")})
	}
	h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		h.srv.Dispatcher.RetrySweep(context.Background())
		found := false
		for _, cp := range h.callList("GET", "/api/inbox") {
			if cp["Kind"] == "dispatch-failure" && strings.Contains(fmt.Sprint(cp["Context"]), "boom") {
				found = true
			}
		}
		if found {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("no dispatch-failure checkpoint after exhausted retries")
}

// TestIntegrityViolation is FR-4.3 both ways: live commit and boot catch-up.
func TestIntegrityViolation(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 1, Output: 1})
	h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})
	h.eventually("approved", func() bool { return h.docState(specPath) == lifecycle.DocApproved })

	// Live path: edit + commit + hook.
	tampered := strings.Replace(validSpec, "fifteen minutes", "five minutes", 1)
	if err := os.WriteFile(filepath.Join(h.root, specPath), []byte(tampered), 0o644); err != nil {
		t.Fatal(err)
	}
	git(h.t, h.root, "add", "-A")
	git(h.t, h.root, "commit", "-qm", "tamper")
	h.call("POST", "/api/hook/post-commit", map[string]any{})

	h.eventually("integrity checkpoint", func() bool {
		for _, cp := range h.callList("GET", "/api/inbox") {
			if cp["Kind"] == "document-integrity" && strings.Contains(fmt.Sprint(cp["Context"]), "commit") {
				return true
			}
		}
		return false
	})

	// Boot catch-up path: answer the pending one, tamper again with no hook,
	// then run the scan the server does at boot (DESIGN-002 §8).
	pending := h.callList("GET", "/api/inbox")
	h.call("POST", "/api/respond", map[string]any{"id": pending[0]["ID"], "response": map[string]any{"answer": "acknowledged"}})
	tampered2 := strings.Replace(tampered, "five minutes", "two minutes", 1)
	if err := os.WriteFile(filepath.Join(h.root, specPath), []byte(tampered2), 0o644); err != nil {
		t.Fatal(err)
	}
	git(h.t, h.root, "add", "-A")
	git(h.t, h.root, "commit", "-qm", "tamper offline")
	if err := h.srv.CatchUpScan(context.Background()); err != nil {
		t.Fatal(err)
	}
	h.eventually("catch-up integrity checkpoint", func() bool {
		for _, cp := range h.callList("GET", "/api/inbox") {
			if cp["Kind"] == "document-integrity" {
				return true
			}
		}
		return false
	})
}

// TestRevisionSupersession is FR-4.4: revise an approved doc, approve the
// successor, predecessor superseded atomically, canonical path re-pointed.
func TestRevisionSupersession(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"ok"}`, provider.Usage{Input: 1, Output: 1})
	h.call("POST", "/api/docs/submit", map[string]string{"path": specPath})
	h.eventually("approved", func() bool { return h.docState(specPath) == lifecycle.DocApproved })

	code, out := h.call("POST", "/api/docs/revise", map[string]string{"path": specPath})
	if code != 201 {
		t.Fatalf("revise: %d %v", code, out)
	}
	revPath := out["Path"].(string)
	if revPath == specPath {
		t.Fatal("successor must draft at a working path")
	}

	// Edit the revision, commit it (git mv needs tracked files), submit.
	revised := strings.Replace(validSpec, "fifteen minutes", "thirty minutes", 1)
	if err := os.WriteFile(filepath.Join(h.root, revPath), []byte(revised), 0o644); err != nil {
		t.Fatal(err)
	}
	git(h.t, h.root, "add", "-A")
	git(h.t, h.root, "commit", "-qm", "revision draft")
	h.mock.RespondOutcome("submit_review", `{"verdict":"approve","reasoning":"tighter"}`, provider.Usage{Input: 1, Output: 1})
	h.call("POST", "/api/docs/submit", map[string]string{"path": revPath})

	h.eventually("successor approved at canonical path", func() bool {
		doc, err := store.LiveDocumentByPath(context.Background(), h.srv.Store.Pool, specPath)
		if err != nil || doc.State != lifecycle.DocApproved || doc.SupersedesID == nil {
			return false
		}
		// The server-authored takeover commit lands after the DB transaction;
		// wait for the files too.
		body, err := os.ReadFile(filepath.Join(h.root, specPath))
		return err == nil && strings.Contains(string(body), "thirty minutes")
	})

	// Exactly one non-superseded spec owned by the feature; predecessor
	// superseded; files moved in a server-authored commit.
	ctx := context.Background()
	var live, superseded int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM documents WHERE type='spec' AND state <> 'superseded'`).Scan(&live)
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM documents WHERE type='spec' AND state = 'superseded'`).Scan(&superseded)
	if live != 1 || superseded != 1 {
		t.Errorf("live=%d superseded=%d; want 1/1", live, superseded)
	}
	// The archive name carries the superseded row's short id so repeated
	// revisions never collide in docs/_superseded/.
	if matches, _ := filepath.Glob(filepath.Join(h.root, "docs/_superseded/login-*.md")); len(matches) != 1 {
		t.Errorf("predecessor file should be archived under docs/_superseded/ with its short id; found %v", matches)
	}
	content, err := os.ReadFile(filepath.Join(h.root, specPath))
	if err != nil || !strings.Contains(string(content), "thirty minutes") {
		t.Error("canonical path should hold the successor's content")
	}
}

// TestArchiveOverride is FR-3.1: G5 refusal raises a gate-override
// checkpoint; answering override archives and audits both.
func TestArchiveOverride(t *testing.T) {
	h := newHarness(t)
	h.setupFeatureWithSpec() // leaves a non-terminal feature under auth

	code, out := h.call("POST", "/api/initiatives/archive", map[string]string{"path": "auth", "reason": "descoping"})
	if code != 409 {
		t.Fatalf("archive should be blocked: %d %v", code, out)
	}

	var checkpointID string
	h.eventually("gate-override checkpoint", func() bool {
		for _, cp := range h.callList("GET", "/api/inbox") {
			if cp["Kind"] == "gate-override" {
				checkpointID = cp["ID"].(string)
				return true
			}
		}
		return false
	})
	h.call("POST", "/api/respond", map[string]any{
		"id": checkpointID, "response": map[string]any{"override": true, "reason": "band descoped"}})

	h.eventually("archived", func() bool {
		var archived bool
		_ = h.srv.Store.Pool.QueryRow(context.Background(),
			`SELECT archived FROM initiatives WHERE slug = 'auth'`).Scan(&archived)
		return archived
	})
}

// TestSearchAndStatus rounds out FR-4.1 and FR-2.1's status surface.
func TestSearchAndStatus(t *testing.T) {
	h := newHarness(t)
	specPath := h.setupFeatureWithSpec()
	_ = specPath

	hits := h.callList("GET", "/api/search?q=lockout")
	if len(hits) == 0 {
		t.Error("search should find the registered spec by section content")
	}
	code, out := h.call("GET", "/api/status", nil)
	if code != 200 || out["config"] != "ok" || out["schema_version"].(float64) < 1 {
		t.Errorf("status: %d %v", code, out)
	}
}
