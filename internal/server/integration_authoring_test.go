package server

// The revision-cascade integration suite (SPEC-009 FR-9): a revised design
// never leaves a stale specification standing silently. Reuses the phase-1
// harness; the reviewer mocks follow the phase-2 patterns.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"

	"subutai/internal/bus"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// onlyWriteSpec comments out the starter's write-dev-plan assignment, so a
// test that counts spec dispatches keeps the mock provider's scripted FIFO
// deterministic: a sent feature whose spec is approved would otherwise set an
// unscripted plan writer off. The starter enables both (SPEC-011 FR-11.1).
func (h *harness) onlyWriteSpec() {
	h.t.Helper()
	h.editConfig("  write-dev-plan: dev-plan-author", "  # write-dev-plan: dev-plan-author")
}

// editConfig replaces one line of the test project's config.yaml.
func (h *harness) editConfig(old, new string) {
	h.t.Helper()
	cfgPath := filepath.Join(h.root, ".subutai/config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		h.t.Fatal(err)
	}
	replaced := strings.Replace(string(data), old, new, 1)
	if replaced == string(data) {
		h.t.Fatalf("starter config no longer carries %q", old)
	}
	if err := os.WriteFile(cfgPath, []byte(replaced), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// send sends a feature to development as the operator would (SPEC-011).
func (h *harness) send(featurePath string, hold bool) {
	h.t.Helper()
	f, err := h.srv.featureByPath(context.Background(), featurePath)
	if err != nil {
		h.t.Fatal(err)
	}
	if err := h.srv.SendToDevelopment(context.Background(), f.ID, hold, "sam"); err != nil {
		h.t.Fatalf("send %s: %v", featurePath, err)
	}
}

// countPurpose counts a feature's dispatches of one purpose.
func (h *harness) countPurpose(featurePath, purpose string) int {
	h.t.Helper()
	f, err := h.srv.featureByPath(context.Background(), featurePath)
	if err != nil {
		h.t.Fatal(err)
	}
	n, err := store.CountDispatchesForRef(context.Background(), h.srv.Store.Pool, "feature", f.ID, purpose)
	if err != nil {
		h.t.Fatal(err)
	}
	return n
}

// quiet waits for the dispatch queue to drain, so a deliberately unscripted
// dispatch has failed and rested before the next step is scripted.
func (h *harness) quiet() {
	h.t.Helper()
	h.eventually("dispatch queue quiet", func() bool {
		var busy int
		err := h.srv.Store.Pool.QueryRow(context.Background(),
			`SELECT count(*) FROM dispatches WHERE state IN ('queued','running')`).Scan(&busy)
		return err == nil && busy == 0
	})
}

func cascadeSpec(feature string) string {
	return fmt.Sprintf(`---
title: Spec for %[1]s
type: spec
owner: pf/%[1]s
---

# Spec for %[1]s

## Overview

The %[1]s behaviour, written against the first design.

## Behaviour

The %[1]s behaviour does what its design says, no more.

## Acceptance criteria

- The %[1]s behaviour works as the design describes
`, feature)
}

func cascadeDevPlan(feature string) string {
	return fmt.Sprintf(`---
title: Dev plan for %[1]s
type: dev_plan
owner: pf/%[1]s
---

# Dev plan for %[1]s

## Approach

One helper, one task.

## Tasks

| id | title | depends_on | description |
|----|-------|------------|-------------|
| T1 | The %[1]s helper | | Add the helper the spec asks for |
`, feature)
}

const cascadeDesignV1 = `---
title: Platform design
type: design
owner: pf
---

# Platform design

## What this is for

The platform needs its three behaviours designed in one place.

## The shape of it

Three features, each a thin helper over one shared core.

## Decisions

- One shared core rather than three, because the behaviours overlap.
`

// registerDoc writes a file, commits it, and registers it as a document.
func (h *harness) registerDoc(path, docType, ownerType, ownerRef, body string) {
	h.t.Helper()
	if err := os.MkdirAll(filepath.Join(h.root, filepath.Dir(path)), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, path), []byte(body), 0o644); err != nil {
		h.t.Fatal(err)
	}
	git(h.t, h.root, "add", "-A")
	git(h.t, h.root, "commit", "-qm", "draft "+path)
	if code, out := h.call("POST", "/api/docs", map[string]string{
		"path": path, "type": docType, "owner_type": ownerType, "owner_ref": ownerRef}); code != 201 {
		h.t.Fatalf("register %s: %d %v", path, code, out)
	}
}

// approveDesignAsHuman drives a design through submit and the human approve
// action (SPEC-009 FR-2). A fresh project has no design reviewer (SPEC-011
// FR-10), so a submitted design simply waits for the person.
func (h *harness) approveDesignAsHuman(path string) {
	h.t.Helper()
	ctx := context.Background()
	if code, out := h.call("POST", "/api/docs/submit", map[string]string{"path": path}); code != 200 {
		h.t.Fatalf("submit %s: %d %v", path, code, out)
	}
	doc, err := store.LiveDocumentByPath(ctx, h.srv.Store.Pool, path)
	if err != nil {
		h.t.Fatal(err)
	}
	h.eventually("design in review "+path, func() bool {
		d, err := store.GetDocument(ctx, h.srv.Store.Pool, doc.ID)
		return err == nil && d.State == lifecycle.DocReviewing
	})
	if err := h.srv.HumanApproveDocument(ctx, doc.ID, "sam"); err != nil {
		h.t.Fatalf("human approve %s: %v", path, err)
	}
	h.eventually("design approved "+path, func() bool {
		d, err := store.GetDocument(ctx, h.srv.Store.Pool, doc.ID)
		return err == nil && d.State == lifecycle.DocApproved
	})
}

// pendingByKind returns the pending checkpoints of one kind.
func (h *harness) pendingByKind(kind string) []store.Checkpoint {
	h.t.Helper()
	pending, err := h.srv.Store.PendingCheckpoints(context.Background())
	if err != nil {
		h.t.Fatal(err)
	}
	var out []store.Checkpoint
	for _, cp := range pending {
		if cp.Kind == kind {
			out = append(out, cp)
		}
	}
	return out
}

// reviseAndApproveDesign creates the successor draft, amends it, and carries
// it through review and human approval.
func (h *harness) reviseAndApproveDesign(path, amendment string) {
	h.t.Helper()
	code, out := h.call("POST", "/api/docs/revise", map[string]string{"path": path})
	if code != 201 {
		h.t.Fatalf("revise %s: %d %v", path, code, out)
	}
	revPath := out["Path"].(string)
	raw, err := os.ReadFile(filepath.Join(h.root, revPath))
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.root, revPath), append(raw, []byte(amendment)...), 0o644); err != nil {
		h.t.Fatal(err)
	}
	git(h.t, h.root, "add", "-A")
	git(h.t, h.root, "commit", "-qm", "design revision draft")
	h.approveDesignAsHuman(revPath)
}

// TestSendIsTheTrigger is SPEC-011 FR-2.3 over SPEC-009 FR-4.3/4.4:
// approving a design, creating a described feature under it, and describing
// a placeholder all only re-check, and start nothing, because nothing is sent.
// Send to development is what sets the spec writer off — once, however often
// the event replays — and an undescribed feature can't be sent (FR-4.1).
func TestSendIsTheTrigger(t *testing.T) {
	h := newHarness(t)
	h.onlyWriteSpec()
	ctx := context.Background()

	if code, out := h.call("POST", "/api/initiatives", map[string]string{"slug": "pf", "name": "Platform"}); code != 201 {
		t.Fatalf("create initiative: %d %v", code, out)
	}
	// A feature described before the design exists: approving the design
	// must not spec it (DEC-006 decision 1).
	if code, out := h.call("POST", "/api/features", map[string]string{
		"initiative_path": "pf", "slug": "early", "name": "The early behaviour",
		"description": "Everything the early behaviour must do."}); code != 201 {
		t.Fatalf("create early feature: %d %v", code, out)
	}
	designPath := "docs/design/pf.md"
	h.registerDoc(designPath, "design", "initiative", "pf", cascadeDesignV1)
	h.approveDesignAsHuman(designPath)

	// A described feature created under the approved design: still nothing.
	if code, out := h.call("POST", "/api/features", map[string]string{
		"initiative_path": "pf", "slug": "told", "name": "The told behaviour",
		"description": "Everything the told behaviour must do."}); code != 201 {
		t.Fatalf("create described feature: %d %v", code, out)
	}
	// An undescribed feature, then described through the UI action.
	if code, out := h.call("POST", "/api/features", map[string]string{
		"initiative_path": "pf", "slug": "stub", "name": "The stub behaviour"}); code != 201 {
		t.Fatalf("create undescribed feature: %d %v", code, out)
	}
	stub, err := h.srv.featureByPath(ctx, "pf/stub")
	if err != nil {
		t.Fatal(err)
	}
	if err := h.srv.SendToDevelopment(ctx, stub.ID, false, "sam"); err == nil ||
		!strings.Contains(err.Error(), "no description") {
		t.Fatalf("an undescribed feature must not be sendable; got %v", err)
	}
	if code, body := h.postForm("/ui/entity/describe", map[string]string{
		"ref_type": "feature", "id": stub.ID.String(),
		"description": "Now the stub behaviour is real."}); code != 200 {
		t.Fatalf("describe: %d\n%s", code, truncate(body, 300))
	}
	// Let every re-check the events above caused run, then look.
	h.quiet()
	for _, f := range []string{"early", "told", "stub"} {
		if got := h.countPurpose("pf/"+f, "write-spec"); got != 0 {
			t.Fatalf("nothing is sent, so %s must not be specced; got %d dispatches", f, got)
		}
	}
	if got := len(h.pendingByKind("config-error")) + len(h.pendingByKind("dispatch-failure")); got != 0 {
		t.Fatalf("an unsent feature raises nothing; got %d checkpoints", got)
	}

	// The heartbeat sweep leaves unsent features alone (FR-2.4).
	h.srv.ReconcileAuthoringSweep(ctx)
	h.quiet()
	if got := h.countPurpose("pf/told", "write-spec"); got != 0 {
		t.Fatalf("the sweep must not spec an unsent feature; got %d", got)
	}

	// Send one of them: exactly that one is specced.
	h.send("pf/told", false)
	h.eventually("write-spec dispatched for the sent feature", func() bool {
		return h.countPurpose("pf/told", "write-spec") == 1
	})
	told, err := h.srv.featureByPath(ctx, "pf/told")
	if err != nil {
		t.Fatal(err)
	}
	// A replayed send, and a sweep, dispatch nothing more (FR-4.6 of SPEC-009).
	h.quiet()
	h.srv.Bus.Publish(bus.FeatureSent{FeatureID: told.ID})
	h.srv.ReconcileAuthoringSweep(ctx)
	h.quiet()
	if got := h.countPurpose("pf/told", "write-spec"); got != 1 {
		t.Fatalf("a replayed send must not double-dispatch; got %d", got)
	}
	if got := h.countPurpose("pf/stub", "write-spec"); got != 0 {
		t.Fatalf("the unsent feature beside it must stay quiet; got %d", got)
	}
	// Sending twice is refused in a sentence.
	if err := h.srv.SendToDevelopment(ctx, told.ID, false, "sam"); err == nil ||
		!strings.Contains(err.Error(), "already been sent") {
		t.Fatalf("a second send must be refused; got %v", err)
	}
	var sentRows int
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE kind = 'feature.sent'`).Scan(&sentRows)
	if sentRows != 1 {
		t.Errorf("one feature.sent audit row expected; got %d", sentRows)
	}

	// NFR-2 / DoD 4: an authoring dispatch carries a nil ToolCtx. Documents
	// live in the main repository, never a worktree — the property most
	// likely to be eroded by a later change, so it is pinned here.
	for _, purpose := range []string{"write-spec", "write-dev-plan"} {
		role := "spec-author"
		if purpose == "write-dev-plan" {
			role = "dev-plan-author"
		}
		plan, err := h.srv.Plan(ctx, &store.Dispatch{Purpose: purpose, Role: role, RefType: "feature", RefID: told.ID})
		if err != nil {
			t.Fatalf("plan %s: %v", purpose, err)
		}
		if plan.ToolCtx != nil {
			t.Errorf("%s must carry a nil ToolCtx (NFR-2); got %+v", purpose, plan.ToolCtx)
		}
		if plan.OutcomeTool != "submit_document" {
			t.Errorf("%s outcome tool = %q, want submit_document", purpose, plan.OutcomeTool)
		}
	}
}

// TestDesignRevisionCascade is SPEC-009 FR-9's acceptance, both halves:
// revising a design over three specced features raises exactly one checkpoint
// listing the three specs; answering invalidate on two supersedes those two
// and their dev-plans and produces two write-spec dispatches while the kept
// third is untouched; and a later revision over a single remaining spec
// raises no checkpoint at all, invalidating directly.
func TestDesignRevisionCascade(t *testing.T) {
	h := newHarness(t)
	h.onlyWriteSpec()
	ctx := context.Background()

	if code, out := h.call("POST", "/api/initiatives", map[string]string{"slug": "pf", "name": "Platform"}); code != 201 {
		t.Fatalf("create initiative: %d %v", code, out)
	}
	features := []string{"alpha", "beta", "gamma"}
	for _, f := range features {
		if code, out := h.call("POST", "/api/features", map[string]string{
			"initiative_path": "pf", "slug": f, "name": "The " + f + " behaviour",
			"description": "Everything the " + f + " behaviour must do."}); code != 201 {
			t.Fatalf("create feature %s: %d %v", f, code, out)
		}
	}

	// The first design approves without a cascade, and starts nothing.
	designPath := "docs/design/pf.md"
	h.registerDoc(designPath, "design", "initiative", "pf", cascadeDesignV1)
	h.approveDesignAsHuman(designPath)
	if got := len(h.pendingByKind("design-revision")); got != 0 {
		t.Fatalf("a first design approval must not raise a revision checkpoint; got %d", got)
	}

	// Specs written ahead in chat (DESIGN-010 §5a), then all three features
	// sent: a step whose document exists is skipped, so no spec is written.
	specPaths := map[string]string{}
	for _, f := range features {
		p := "docs/specs/" + f + ".md"
		h.registerDoc(p, "spec", "feature", "pf/"+f, cascadeSpec(f))
		specPaths[f] = p
		h.send("pf/"+f, false)
	}
	// Three approved specs; alpha also gets an approved dev-plan, which
	// decomposes and passes G1, so alpha reaches ready.
	for _, f := range features {
		h.approveDoc(specPaths[f])
	}
	devPlanPath := "docs/plans/alpha.md"
	h.registerDoc(devPlanPath, "dev_plan", "feature", "pf/alpha", cascadeDevPlan("alpha"))
	h.approveDoc(devPlanPath)
	h.eventually("alpha ready", func() bool { return h.featureState("pf/alpha") == lifecycle.FeatReady })
	h.quiet() // alpha's closing estimate is unscripted; let it rest
	for _, f := range features {
		if got := h.countPurpose("pf/"+f, "write-spec"); got != 0 {
			t.Fatalf("a spec written ahead is used as it is; %s got %d write-spec dispatches", f, got)
		}
	}

	// Revision one: all three specs are affected → exactly one checkpoint.
	h.reviseAndApproveDesign(designPath, "- The core grows a second seam, because the first one leaked.\n")
	var cp store.Checkpoint
	h.eventually("one design-revision checkpoint", func() bool {
		cps := h.pendingByKind("design-revision")
		if len(cps) != 1 {
			return false
		}
		cp = cps[0]
		return true
	})
	var cctx struct {
		Affected []struct {
			SpecDocID   string `json:"spec_doc_id"`
			FeatureName string `json:"feature_name"`
		} `json:"affected"`
	}
	if err := json.Unmarshal(cp.Context, &cctx); err != nil || len(cctx.Affected) != 3 {
		t.Fatalf("checkpoint should list all three specs: %v %s", err, cp.Context)
	}

	// FR-9.3: the pending question blocks the ready feature from starting.
	if _, err := h.srv.StartFeature(ctx, "pf/alpha", "sam"); err == nil ||
		!strings.Contains(err.Error(), "Inbox") {
		t.Fatalf("start should be blocked by the pending revision question, got: %v", err)
	}

	// Answer per spec through the inbox form: invalidate alpha and beta,
	// keep gamma.
	specIDByFeature := map[string]string{}
	for _, a := range cctx.Affected {
		switch {
		case strings.Contains(a.FeatureName, "alpha"):
			specIDByFeature["alpha"] = a.SpecDocID
		case strings.Contains(a.FeatureName, "beta"):
			specIDByFeature["beta"] = a.SpecDocID
		default:
			specIDByFeature["gamma"] = a.SpecDocID
		}
	}
	form := map[string]string{
		"id":                                   cp.ID.String(),
		"decision_" + specIDByFeature["alpha"]: "invalidate",
		"decision_" + specIDByFeature["beta"]:  "invalidate",
		"decision_" + specIDByFeature["gamma"]: "keep",
		"reason":                               "the seam change rewrites alpha and beta",
	}
	if code, body := h.postForm("/ui/respond", form); code != 200 {
		t.Fatalf("respond: %d\n%s", code, truncate(body, 400))
	}

	docState := func(id string) lifecycle.DocumentState {
		d, err := store.GetDocument(ctx, h.srv.Store.Pool, uuid.MustParse(id))
		if err != nil {
			return "missing"
		}
		return d.State
	}
	h.eventually("alpha and beta specs superseded, gamma kept", func() bool {
		return docState(specIDByFeature["alpha"]) == lifecycle.DocSuperseded &&
			docState(specIDByFeature["beta"]) == lifecycle.DocSuperseded &&
			docState(specIDByFeature["gamma"]) == lifecycle.DocApproved
	})

	// FR-9.4a: alpha's dev-plan went with its spec, mechanically.
	devPlan, err := store.LiveDocumentByPath(ctx, h.srv.Store.Pool, devPlanPath)
	if err == nil && devPlan.State != lifecycle.DocSuperseded {
		t.Errorf("alpha's dev-plan should be superseded with its spec; state = %s", devPlan.State)
	}
	// The invalidated files are archived in a server-authored commit: alpha's
	// spec and dev-plan, beta's spec, and the revision takeover's design v1 —
	// and every archive succeeded, so no integrity checkpoint stands. The row
	// updates land before the file moves, so the directory is polled.
	h.eventually("four files archived under docs/_superseded", func() bool {
		entries, _ := os.ReadDir(filepath.Join(h.root, "docs/_superseded"))
		return len(entries) == 4
	})
	if got := len(h.pendingByKind("document-integrity")); got != 0 {
		t.Errorf("every archive should succeed; %d document-integrity checkpoints stand", got)
	}
	// Alpha lost its contract and is an idea again, so the chain can re-run.
	if got := h.featureState("pf/alpha"); got != lifecycle.FeatIdea {
		t.Errorf("alpha should return to idea when its contract is invalidated; state = %s", got)
	}

	// FR-9.4: exactly the two invalidated features get fresh write-spec
	// dispatches; the kept one gets none.
	countWriteSpec := func(feature string) int {
		f, err := h.srv.featureByPath(ctx, "pf/"+feature)
		if err != nil {
			t.Fatal(err)
		}
		n, err := store.CountDispatchesForRef(ctx, h.srv.Store.Pool, "feature", f.ID, "write-spec")
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	h.eventually("write-spec dispatched for the invalidated features", func() bool {
		return countWriteSpec("alpha") == 1 && countWriteSpec("beta") == 1
	})
	if got := countWriteSpec("gamma"); got != 0 {
		t.Errorf("the kept spec must not be re-authored; gamma has %d write-spec dispatches", got)
	}

	// FR-9.5: the keep is on the audit trail via the checkpoint's answer.
	var responses int
	_ = h.srv.Store.Pool.QueryRow(ctx, `
		SELECT count(*) FROM audit_events
		WHERE kind = 'checkpoint.responded' AND payload::text LIKE '%keep%'`).Scan(&responses)
	if responses == 0 {
		t.Error("the keep decision should appear in the audit trail")
	}

	// Let the (deliberately unscripted) write-spec dispatches fail and rest.
	h.quiet()

	// Revision two: gamma now holds the single remaining approved spec, so
	// FR-9.1 invalidates it directly — no checkpoint, no question.
	h.reviseAndApproveDesign(designPath, "- A third seam, discovered the hard way.\n")
	h.eventually("gamma's spec invalidated without a checkpoint", func() bool {
		return docState(specIDByFeature["gamma"]) == lifecycle.DocSuperseded
	})
	if got := len(h.pendingByKind("design-revision")); got != 0 {
		t.Errorf("a single affected spec must not raise a checkpoint; got %d", got)
	}
	h.eventually("gamma re-authored", func() bool { return countWriteSpec("gamma") == 1 })
}
