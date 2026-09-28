package server

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"cromwell/internal/store"
)

// The SPEC-010 suite: milestones and roadmaps edited from the web UI and over
// MCP, against real Postgres, asserting the audited outcome of each act.

// postPlan posts a form the way the page would. editor=true is an HTMX post
// from inside an editor; editor=false is a plain form post, which on these
// pages is boosted by HTMX (hx-boost on the body), so it carries both headers.
func (h *harness) postPlan(path string, form map[string]string, editor bool) (int, string) {
	h.t.Helper()
	vals := url.Values{}
	for k, v := range form {
		vals.Set(k, v)
	}
	req, _ := http.NewRequest("POST", h.api.URL+path, strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	if !editor {
		req.Header.Set("HX-Boosted", "true")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

// planFixture is auth → login (from setupFeatureWithSpec) plus a second tree,
// billing → invoices, for cross-tree membership.
type planFixture struct {
	auth, billing, login, invoices uuid.UUID
}

func (h *harness) setupPlanFixture() planFixture {
	h.t.Helper()
	h.setupFeatureWithSpec()
	h.call("POST", "/api/initiatives", map[string]string{"slug": "billing", "name": "Billing"})
	h.call("POST", "/api/features", map[string]string{"initiative_path": "billing", "slug": "invoices", "name": "Invoices"})
	ctx := context.Background()
	auth, err := h.srv.Store.InitiativeBySlugPath(ctx, []string{"auth"})
	if err != nil {
		h.t.Fatal(err)
	}
	billing, err := h.srv.Store.InitiativeBySlugPath(ctx, []string{"billing"})
	if err != nil {
		h.t.Fatal(err)
	}
	login, err := h.srv.featureByPath(ctx, "auth/login")
	if err != nil {
		h.t.Fatal(err)
	}
	invoices, err := h.srv.featureByPath(ctx, "billing/invoices")
	if err != nil {
		h.t.Fatal(err)
	}
	return planFixture{auth: auth.ID, billing: billing.ID, login: login.ID, invoices: invoices.ID}
}

// auditCount counts audit rows of a kind by an actor.
func (h *harness) auditCount(kind, actor string) int {
	h.t.Helper()
	var n int
	if err := h.srv.Store.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_events WHERE kind = $1 AND actor = $2`, kind, actor).Scan(&n); err != nil {
		h.t.Fatal(err)
	}
	return n
}

func (h *harness) milestoneNamed(name string) *store.Milestone {
	h.t.Helper()
	m, err := store.MilestoneByName(context.Background(), h.srv.Store.Pool, name)
	if err != nil {
		h.t.Fatalf("milestone %q: %v", name, err)
	}
	return m
}

// TestUIPlanEditing is SPEC-010's web half: a two-milestone roadmap for an
// initiative, built entirely through the /ui/* handlers — created on the
// owner's page, filled from both ends, ordered, and one milestone locked.
func TestUIPlanEditing(t *testing.T) {
	h := newHarness(t)
	fx := h.setupPlanFixture()
	ctx := context.Background()
	actor := h.srv.uiActor()
	authID := fx.auth.String()

	// FR-2: create two milestones and a roadmap on the initiative's page.
	code, page := h.postPlan("/ui/milestone/new", map[string]string{
		"owner_type": "initiative", "id": authID, "name": "Auth beta",
		"target_date": "2026-12-31", "description": "People can sign in."}, false)
	if code != 200 {
		t.Fatalf("create milestone: %d", code)
	}
	mustContain(t, "create notice", page, "Milestone created: Auth beta.")
	h.postPlan("/ui/milestone/new", map[string]string{"owner_type": "initiative", "id": authID, "name": "Auth GA"}, false)
	_, page = h.postPlan("/ui/roadmap/new", map[string]string{"owner_type": "initiative", "id": authID, "name": "Auth plan"}, false)
	mustContain(t, "roadmap notice", page, "Roadmap created: Auth plan.")

	beta, ga := h.milestoneNamed("Auth beta"), h.milestoneNamed("Auth GA")
	if beta.OwnerType != "initiative" || beta.OwnerID == nil || *beta.OwnerID != fx.auth {
		t.Fatalf("Auth beta owner = %s %v, want the auth initiative", beta.OwnerType, beta.OwnerID)
	}
	if beta.TargetDate == nil || beta.TargetDate.Format("2006-01-02") != "2026-12-31" {
		t.Errorf("target date = %v", beta.TargetDate)
	}
	rm, err := store.RoadmapByName(ctx, h.srv.Store.Pool, "Auth plan")
	if err != nil || rm.OwnerType != "initiative" {
		t.Fatalf("roadmap: %+v %v", rm, err)
	}
	if n := h.auditCount("milestone.created", actor); n != 2 {
		t.Errorf("milestone.created by %s = %d, want 2", actor, n)
	}

	// FR-2.1: it shows in the initiative's own plan, and only there (D-9).
	_, authPage := h.getUI("/ui/i/auth")
	mustContain(t, "plan section", authPage, "Auth beta")
	mustContain(t, "edit button", authPage, "/ui/m/"+beta.ID.String()+"/edit")
	mustContain(t, "roadmap edit", authPage, "/ui/r/"+rm.ID.String()+"/edit")
	_, projPage := h.getUI("/ui/project")
	if strings.Contains(projPage, "Auth beta") {
		t.Error("an initiative's milestone appears in the project's own plan")
	}
	mustContain(t, "project can plan", projPage, "Nothing is planned here yet")

	// FR-2.4: a blank name or a bad date is refused in a sentence.
	_, page = h.postPlan("/ui/milestone/new", map[string]string{"owner_type": "project", "id": "", "name": "  "}, false)
	mustContain(t, "blank name", page, "A milestone needs a name")
	_, page = h.postPlan("/ui/milestone/new", map[string]string{"owner_type": "project", "id": "", "name": "x", "target_date": "soon"}, false)
	mustContain(t, "bad date", page, "The target date must be a date")
	if ms, _ := store.ListMilestones(ctx, h.srv.Store.Pool); len(ms) != 2 {
		t.Errorf("milestones after refusals = %d, want 2", len(ms))
	}

	// FR-6.1: the editor is a fragment that opens itself.
	code, ed := h.getUI("/ui/m/" + beta.ID.String() + "/edit")
	if code != 200 {
		t.Fatalf("editor: %d", code)
	}
	mustContain(t, "autoshow dialog", ed, `<dialog class="modal modal--wide" id="dlg-milestone" data-autoshow`)
	if strings.Contains(ed, "<html") {
		t.Error("the editor fragment is a whole page")
	}
	// FR-3.2: with no search, the picker shows the owner's subtree only.
	mustContain(t, "subtree candidate", ed, "Login form")
	if strings.Contains(ed, "Invoices") {
		t.Error("the default picker reaches outside the owner's subtree")
	}
	// FR-3.3: a search reaches the whole project.
	_, cands := h.getUI("/ui/m/" + beta.ID.String() + "/candidates?q=invoi")
	mustContain(t, "project-wide search", cands, "Invoices")

	// FR-3.1: add from the member's end — the feature's own page.
	_, featPage := h.getUI("/ui/f/auth/login")
	mustContain(t, "member action", featPage, "Add to a milestone…")
	mustContain(t, "choice", featPage, `<option value="`+beta.ID.String()+`">Auth beta</option>`)
	_, page = h.postPlan("/ui/milestone/member/add", map[string]string{
		"milestone_id": beta.ID.String(), "member_type": "feature", "member_id": fx.login.String(), "from": "member"}, false)
	mustContain(t, "member-side notice", page, "Login form was added to Auth beta.")
	mustContain(t, "rail shows membership", page, `href="/ui/m/`+beta.ID.String()+`"`)

	// FR-3.2: add from the milestone's end, inside the editor — cross-tree.
	code, frag := h.postPlan("/ui/milestone/member/add", map[string]string{
		"milestone_id": beta.ID.String(), "member_type": "initiative", "member_id": fx.billing.String()}, true)
	if code != 200 || strings.Contains(frag, "<html") {
		t.Fatalf("editor add returned %d and a whole page", code)
	}
	mustContain(t, "editor fragment", frag, `id="milestone-editor" data-changed="true"`)
	mustContain(t, "editor notice", frag, "Billing was added to Auth beta.")
	members, _ := store.Members(ctx, h.srv.Store.Pool, beta.ID)
	if len(members) != 2 {
		t.Fatalf("members = %+v, want login and billing", members)
	}
	if n := h.auditCount("milestone.member_added", actor); n != 2 {
		t.Errorf("member_added audit rows = %d", n)
	}

	// A milestone inside another, and the loop it may not close (FR-1.5).
	h.postPlan("/ui/milestone/member/add", map[string]string{
		"milestone_id": ga.ID.String(), "member_type": "milestone", "member_id": beta.ID.String()}, true)
	_, frag = h.postPlan("/ui/milestone/member/add", map[string]string{
		"milestone_id": beta.ID.String(), "member_type": "milestone", "member_id": ga.ID.String()}, true)
	mustContain(t, "cycle refused", frag, "can&#39;t contain itself")
	// The milestone page offers the member's end too, minus the loop.
	_, mPage := h.getUI("/ui/m/" + beta.ID.String())
	mustContain(t, "milestone is part of", mPage, "Auth GA")
	if strings.Contains(mPage, `<option value="`+ga.ID.String()+`"`) {
		t.Error("Auth beta is offered a milestone it is already in")
	}

	// Removal from both ends, with the reason kept (SD-6).
	_, frag = h.postPlan("/ui/milestone/member/remove", map[string]string{
		"milestone_id": beta.ID.String(), "member_type": "initiative", "member_id": fx.billing.String(),
		"reason": "Billing ships later"}, true)
	mustContain(t, "remove notice", frag, "Billing was taken out of Auth beta.")
	var reason string
	_ = h.srv.Store.Pool.QueryRow(ctx, `SELECT payload->>'reason' FROM audit_events
		WHERE kind = 'milestone.member_removed' AND ref_id = $1`, beta.ID).Scan(&reason)
	if reason != "Billing ships later" {
		t.Errorf("removal reason on the trail = %q", reason)
	}
	h.postPlan("/ui/milestone/member/add", map[string]string{
		"milestone_id": beta.ID.String(), "member_type": "feature", "member_id": fx.invoices.String()}, true)
	_, page = h.postPlan("/ui/milestone/member/remove", map[string]string{
		"milestone_id": beta.ID.String(), "member_type": "feature", "member_id": fx.invoices.String(), "from": "member"}, false)
	mustContain(t, "member-side removal", page, "Invoices was taken out of Auth beta.")

	// FR-4: place both on the roadmap, then reorder.
	for _, m := range []*store.Milestone{beta, ga} {
		_, frag = h.postPlan("/ui/roadmap/entry/place", map[string]string{
			"roadmap_id": rm.ID.String(), "milestone_id": m.ID.String(), "place": "0"}, true)
	}
	mustContain(t, "placed", frag, "Auth GA is now number 2 on the roadmap.")
	_, frag = h.postPlan("/ui/roadmap/entry/place", map[string]string{
		"roadmap_id": rm.ID.String(), "milestone_id": ga.ID.String(), "place": "1"}, true)
	mustContain(t, "moved", frag, "Auth GA is now number 1 on the roadmap.")
	entries, _ := store.RoadmapEntries(ctx, h.srv.Store.Pool, rm.ID)
	if len(entries) != 2 || entries[0].MilestoneID != ga.ID || entries[1].MilestoneID != beta.ID {
		t.Fatalf("roadmap order = %+v, want GA then beta", entries)
	}
	_, rPage := h.getUI("/ui/r/" + rm.ID.String())
	if strings.Index(rPage, "Auth GA") > strings.Index(rPage, "Auth beta") {
		t.Error("the roadmap page does not show the new order")
	}
	// Take one off and put it back.
	_, frag = h.postPlan("/ui/roadmap/entry/remove", map[string]string{
		"roadmap_id": rm.ID.String(), "milestone_id": beta.ID.String()}, true)
	mustContain(t, "taken off", frag, "Auth beta was taken off the roadmap.")
	h.postPlan("/ui/roadmap/entry/place", map[string]string{
		"roadmap_id": rm.ID.String(), "milestone_id": beta.ID.String()}, true)
	if n := h.auditCount("roadmap.entry_removed", actor); n != 1 {
		t.Errorf("entry_removed audit rows = %d", n)
	}

	// FR-5: G4 refuses while nothing is done — disabled with the reason, and a
	// post is refused in the same words.
	_, ed = h.getUI("/ui/m/" + beta.ID.String() + "/edit")
	mustContain(t, "lock disabled", ed, "its one feature isn&#39;t done")
	_, frag = h.postPlan("/ui/milestone/lock", map[string]string{"milestone_id": beta.ID.String(), "confirm": "permanent"}, true)
	mustContain(t, "G4 refusal", frag, "can&#39;t be locked yet")
	// Without the confirmation nothing is locked, whatever the gate says.
	if _, err := h.srv.Store.Pool.Exec(ctx, `UPDATE features SET state = 'done' WHERE id = $1`, fx.login); err != nil {
		t.Fatal(err)
	}
	_, frag = h.postPlan("/ui/milestone/lock", map[string]string{"milestone_id": beta.ID.String()}, true)
	mustContain(t, "confirm needed", frag, "Locking is permanent, so it needs the confirmation")
	if h.milestoneNamed("Auth beta").LockedAt != nil {
		t.Fatal("locked without the confirmation")
	}
	_, ed = h.getUI("/ui/m/" + beta.ID.String() + "/edit")
	mustContain(t, "confirm step", ed, "Locking is permanent.")
	_, frag = h.postPlan("/ui/milestone/lock", map[string]string{"milestone_id": beta.ID.String(), "confirm": "permanent"}, true)
	mustContain(t, "locked notice", frag, "This milestone is now locked.")
	if h.milestoneNamed("Auth beta").LockedAt == nil {
		t.Fatal("the milestone did not lock")
	}
	if n := h.auditCount("milestone.locked", actor); n != 1 {
		t.Errorf("milestone.locked by %s = %d", actor, n)
	}
	// FR-3.5: a locked milestone's editor offers nothing to change.
	_, ed = h.getUI("/ui/m/" + beta.ID.String() + "/edit")
	mustContain(t, "locked editor", ed, "This milestone is locked.")
	if strings.Contains(ed, "/ui/milestone/member/add") {
		t.Error("a locked milestone still offers to add")
	}

	// NFR-4 (review R10-6): no page or fragment here takes a typed path.
	for _, p := range []string{"/ui/i/auth", "/ui/project", "/ui/f/auth/login",
		"/ui/m/" + beta.ID.String(), "/ui/m/" + ga.ID.String() + "/edit",
		"/ui/m/" + ga.ID.String() + "/candidates?q=a", "/ui/r/" + rm.ID.String(),
		"/ui/r/" + rm.ID.String() + "/edit"} {
		_, body := h.getUI(p)
		for _, banned := range typedPathFieldNames {
			if strings.Contains(body, banned) {
				t.Errorf("%s renders a typed entity-path field %s", p, banned)
			}
		}
	}
}

// TestMCPPlanTools is SPEC-010's chat half: the same two-milestone roadmap for
// an initiative, built over POST /mcp, each write audited to the MCP actor, and
// no way to lock.
func TestMCPPlanTools(t *testing.T) {
	h := newHarness(t)
	h.setupPlanFixture()
	actor := h.srv.mcpActor()

	call := func(name string, args map[string]any) map[string]any {
		t.Helper()
		out, isErr, msg := h.callTool(name, args)
		if isErr {
			t.Fatalf("%s: %s", name, msg)
		}
		return out
	}
	callErr := func(name string, args map[string]any, want string) {
		t.Helper()
		_, isErr, msg := h.callTool(name, args)
		if !isErr {
			t.Fatalf("%s should have failed", name)
		}
		if !strings.Contains(msg, want) {
			t.Errorf("%s error = %q, want it to say %q", name, msg, want)
		}
	}

	// FR-7.1, FR-7.2: create with an owner, and at the project level.
	beta := call("create_milestone", map[string]any{"name": "Auth beta", "owner_path": "auth",
		"target_date": "2026-12-31", "description": "People can sign in."})
	if owner := beta["owner"].(map[string]any); owner["type"] != "initiative" || owner["path"] != "auth" {
		t.Errorf("owner = %v", owner)
	}
	if beta["target_date"] != "2026-12-31" {
		t.Errorf("target_date = %v", beta["target_date"])
	}
	ga := call("create_milestone", map[string]any{"name": "Auth GA", "owner_path": "auth"})
	call("create_milestone", map[string]any{"name": "Launch"})
	callErr("create_milestone", map[string]any{"name": "x", "owner_path": "nope"}, "no initiative at \"nope\"")
	callErr("create_milestone", map[string]any{"name": "x", "target_date": "soon"}, "YYYY-MM-DD")
	rm := call("create_roadmap", map[string]any{"name": "Auth plan", "owner_path": "auth"})
	betaID, gaID, rmID := beta["id"].(string), ga["id"].(string), rm["id"].(string)

	// FR-7.3, FR-7.4: fill by id and by name, across trees; refuse a loop.
	call("add_milestone_member", map[string]any{"milestone": betaID, "member_type": "feature", "member": "auth/login"})
	call("add_milestone_member", map[string]any{"milestone": "Auth beta", "member_type": "initiative", "member": "billing"})
	call("add_milestone_member", map[string]any{"milestone": "Auth GA", "member_type": "milestone", "member": "Auth beta"})
	callErr("add_milestone_member", map[string]any{"milestone": betaID, "member_type": "milestone", "member": gaID}, "can't contain itself")
	callErr("add_milestone_member", map[string]any{"milestone": betaID, "member_type": "task", "member": "x"}, "member_type must be")
	out := call("remove_milestone_member", map[string]any{"milestone": betaID, "member_type": "initiative",
		"member": "billing", "reason": "Billing ships later"})
	mustContain(t, "change note", out["change"].(string), "Billing was taken out")
	callErr("remove_milestone_member", map[string]any{"milestone": betaID, "member_type": "initiative", "member": "billing"},
		"isn't directly in the milestone")

	// FR-7.5, FR-7.6: place, reorder, take off.
	call("place_roadmap_entry", map[string]any{"roadmap": rmID, "milestone": betaID})
	call("place_roadmap_entry", map[string]any{"roadmap": "Auth plan", "milestone": "Auth GA"})
	got := call("place_roadmap_entry", map[string]any{"roadmap": rmID, "milestone": gaID, "position": 1})
	order := got["milestones"].([]any)
	if len(order) != 2 || order[0].(map[string]any)["name"] != "Auth GA" || order[1].(map[string]any)["position"] != float64(2) {
		t.Fatalf("order after moving GA first = %v", order)
	}
	call("place_roadmap_entry", map[string]any{"roadmap": rmID, "milestone": "Launch"})
	got = call("remove_roadmap_entry", map[string]any{"roadmap": rmID, "milestone": "Launch"})
	if len(got["milestones"].([]any)) != 2 {
		t.Errorf("after taking Launch off: %v", got["milestones"])
	}
	callErr("remove_roadmap_entry", map[string]any{"roadmap": rmID, "milestone": "Launch"}, "isn't on the roadmap")

	// FR-7.7: lists, filtered by owner or not.
	all := call("list_milestones", map[string]any{})["milestones"].([]any)
	if len(all) != 3 {
		t.Errorf("all milestones = %d, want 3", len(all))
	}
	authOnly := call("list_milestones", map[string]any{"owner_type": "initiative", "owner_path": "auth"})["milestones"].([]any)
	if len(authOnly) != 2 {
		t.Errorf("auth's milestones = %d, want 2", len(authOnly))
	}
	projOnly := call("list_milestones", map[string]any{"owner_type": "project"})["milestones"].([]any)
	if len(projOnly) != 1 || projOnly[0].(map[string]any)["name"] != "Launch" {
		t.Errorf("project's milestones = %v", projOnly)
	}
	callErr("list_milestones", map[string]any{"owner_type": "feature"}, "owner_type must be")
	rms := call("list_roadmaps", map[string]any{"owner_type": "initiative", "owner_path": "auth"})["roadmaps"].([]any)
	if len(rms) != 1 {
		t.Errorf("auth's roadmaps = %v", rms)
	}

	// FR-7.8: the detail says what is in it and why it can't lock yet.
	detail := call("get_milestone", map[string]any{"milestone": "Auth beta"})
	if ms := detail["members"].([]any); len(ms) != 1 || ms[0].(map[string]any)["path"] != "auth/login" {
		t.Errorf("members = %v", detail["members"])
	}
	lock := detail["lock"].(map[string]any)
	if lock["could_lock_now"] != false {
		t.Errorf("could_lock_now = %v", lock["could_lock_now"])
	}
	mustContain(t, "why not", lock["why_not"].(string), "isn't done")
	road := call("get_roadmap", map[string]any{"roadmap": "Auth plan"})
	if road["milestones"].([]any)[0].(map[string]any)["name"] != "Auth GA" {
		t.Errorf("get_roadmap order = %v", road["milestones"])
	}

	// NFR-7: every write is on the trail as the chat agent.
	for kind, want := range map[string]int{
		"milestone.created": 3, "milestone.member_added": 3, "milestone.member_removed": 1,
		"roadmap.created": 1, "roadmap.entry_set": 4, "roadmap.entry_removed": 1,
	} {
		if n := h.auditCount(kind, actor); n != want {
			t.Errorf("%s by %s = %d, want %d", kind, actor, n, want)
		}
	}

	// SD-4: there is no lock over MCP; the milestone is still open.
	resp := h.rpc("tools/call", map[string]any{"name": "lock_milestone", "arguments": map[string]any{"milestone": betaID}})
	if resp.Error == nil || resp.Error.Code != rpcMethodNotFound {
		t.Errorf("lock_milestone should be unknown; got %+v", resp)
	}
	if h.milestoneNamed("Auth beta").LockedAt != nil {
		t.Error("the milestone was locked over MCP")
	}
}
