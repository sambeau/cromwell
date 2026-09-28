package server

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"subutai/internal/store"
)

// The SPEC-014 suite: checklists and jobs kept from the web UI and over MCP,
// against real Postgres, asserting the audited outcome of each act and how a
// checklist holds a milestone open.

func (h *harness) checklistNamed(name string) *store.Checklist {
	h.t.Helper()
	c, err := store.ChecklistByName(context.Background(), h.srv.Store.Pool, name)
	if err != nil {
		h.t.Fatalf("checklist %q: %v", name, err)
	}
	return c
}

func (h *harness) jobsOf(checklistID uuid.UUID) []store.Job {
	h.t.Helper()
	jobs, err := store.Jobs(context.Background(), h.srv.Store.Pool, checklistID)
	if err != nil {
		h.t.Fatal(err)
	}
	return jobs
}

// milestoneItems is a milestone's live "X of Y items done".
func (h *harness) milestoneItems(id uuid.UUID) (done, total int) {
	h.t.Helper()
	p, err := store.LiveProgress(context.Background(), h.srv.Store.Pool, id)
	if err != nil {
		h.t.Fatal(err)
	}
	return p.Done, p.Total
}

// boostedPostURL posts a form as a boosted page would and returns the address
// HTMX is told to show for the page it gets back.
func (h *harness) boostedPostURL(path string, form map[string]string) string {
	h.t.Helper()
	vals := url.Values{}
	for k, v := range form {
		vals.Set(k, v)
	}
	req, _ := http.NewRequest("POST", h.api.URL+path, strings.NewReader(vals.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.Header.Set("HX-Boosted", "true")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.Header.Get("HX-Push-Url")
}

// TestUIChecklists is SPEC-014's web half: a checklist created on an
// initiative's page, filled in its editor, put in a milestone from its own
// page and from the milestone's picker, holding the milestone open until its
// jobs are ticked on the checklist page.
func TestUIChecklists(t *testing.T) {
	h := newHarness(t)
	fx := h.setupPlanFixture()
	ctx := context.Background()
	actor := h.srv.uiActor()
	authID := fx.auth.String()

	// FR-3: create from the initiative's page; a blank name is refused.
	_, page := h.postPlan("/ui/checklist/new", map[string]string{
		"owner_type": "initiative", "id": authID, "name": "Launch paperwork",
		"description": "The jobs only people can do."}, false)
	mustContain(t, "create notice", page, "Checklist created: Launch paperwork.")
	_, page = h.postPlan("/ui/checklist/new", map[string]string{"owner_type": "initiative", "id": authID, "name": "  "}, false)
	mustContain(t, "blank name", page, "A checklist needs a name")
	cl := h.checklistNamed("Launch paperwork")
	if cl.OwnerType != "initiative" || cl.OwnerID == nil || *cl.OwnerID != fx.auth {
		t.Fatalf("owner = %s %v, want the auth initiative", cl.OwnerType, cl.OwnerID)
	}
	if n := h.auditCount("checklist.created", actor); n != 1 {
		t.Errorf("checklist.created by %s = %d, want 1", actor, n)
	}
	_, authPage := h.getUI("/ui/i/auth")
	mustContain(t, "listed on its owner", authPage, "Launch paperwork")
	_, projectPage := h.getUI("/ui/project")
	// The name also appears in the navigation, so look only at the plan section.
	i := strings.Index(projectPage, `id="planning"`)
	if i < 0 {
		t.Fatalf("the project page has no plan section")
	}
	plan := projectPage[i:]
	plan = plan[:strings.Index(plan, "</details>")]
	mustNotContain(t, "not in the project's plan", plan, "Launch paperwork")
	cid := cl.ID.String()

	// FR-5: the editor fragment, and adding, changing, moving and removing.
	code, frag := h.getUI("/ui/c/" + cid + "/edit")
	if code != 200 || strings.Contains(frag, "<html") {
		t.Fatalf("editor fragment: %d, whole page = %v", code, strings.Contains(frag, "<html"))
	}
	mustContain(t, "editor dialog", frag, `id="dlg-checklist"`)
	if code, _ := h.getUI("/ui/c/" + uuid.NewString() + "/edit"); code != 404 {
		t.Errorf("unknown checklist's editor: %d, want 404", code)
	}
	for _, title := range []string{"Get the API key", "Sign the contract", "Choose an icon"} {
		_, frag = h.postPlan("/ui/job/add", map[string]string{"checklist_id": cid, "title": title}, true)
		mustContain(t, "added", frag, "Added: "+title+".")
		mustContain(t, "changed", frag, `data-changed="true"`)
	}
	_, frag = h.postPlan("/ui/job/add", map[string]string{"checklist_id": cid, "title": " "}, true)
	mustContain(t, "blank title", frag, "A job needs a title")
	jobs := h.jobsOf(cl.ID)
	key, contract, icon := jobs[0], jobs[1], jobs[2]
	_, frag = h.postPlan("/ui/job/move", map[string]string{"checklist_id": cid, "job_id": icon.ID.String(), "place": "1"}, true)
	mustContain(t, "moved", frag, "Choose an icon is now number 1 on the checklist.")
	_, frag = h.postPlan("/ui/job/edit", map[string]string{"checklist_id": cid, "job_id": icon.ID.String(),
		"title": "Choose the app icon", "note": "Two options from the designer."}, true)
	mustContain(t, "changed", frag, "Changed: Choose the app icon.")
	_, frag = h.postPlan("/ui/job/add", map[string]string{"checklist_id": cid, "title": "Book a launch party"}, true)
	party := h.jobsOf(cl.ID)[3]
	_, frag = h.postPlan("/ui/job/remove", map[string]string{"checklist_id": cid, "job_id": party.ID.String(),
		"reason": "Not this time."}, true)
	mustContain(t, "removed", frag, "Removed: Book a launch party.")
	var titles []string
	for _, j := range h.jobsOf(cl.ID) {
		titles = append(titles, j.Title)
	}
	if strings.Join(titles, "|") != "Choose the app icon|Get the API key|Sign the contract" {
		t.Errorf("order = %v", titles)
	}
	for kind, want := range map[string]int{"job.added": 4, "job.moved": 1, "job.edited": 1, "job.removed": 1} {
		if n := h.auditCount(kind, actor); n != want {
			t.Errorf("%s by %s = %d, want %d", kind, actor, n, want)
		}
	}
	// A boosted post from outside the editor gets the checklist page.
	_, page = h.postPlan("/ui/job/move", map[string]string{"checklist_id": cid, "job_id": icon.ID.String(), "place": "3"}, false)
	mustContain(t, "boosted gets the page", page, `<h1 class="t-page">Launch paperwork</h1>`)

	// FR-6: into a milestone from the milestone's picker; FR-4.4: out and back
	// in from the checklist's own page.
	h.postPlan("/ui/milestone/new", map[string]string{"owner_type": "initiative", "id": authID, "name": "Auth beta"}, false)
	beta := h.milestoneNamed("Auth beta")
	_, frag = h.getUI("/ui/m/" + beta.ID.String() + "/edit")
	mustContain(t, "picker offers it", frag, `name="member_type" value="checklist"`)
	_, frag = h.postPlan("/ui/milestone/member/add", map[string]string{
		"milestone_id": beta.ID.String(), "member_type": "checklist", "member_id": cid}, true)
	mustContain(t, "added from the picker", frag, "Launch paperwork was added to Auth beta.")
	mustContain(t, "member row", frag, "0 of 3 jobs ticked · checklist")
	_, page = h.postPlan("/ui/milestone/member/remove", map[string]string{
		"milestone_id": beta.ID.String(), "member_type": "checklist", "member_id": cid, "from": "member",
		"reason": "Wrong milestone."}, false)
	mustContain(t, "taken out from its page", page, "Launch paperwork was taken out of Auth beta.")
	_, page = h.postPlan("/ui/milestone/member/add", map[string]string{
		"milestone_id": beta.ID.String(), "member_type": "checklist", "member_id": cid, "from": "member"}, false)
	mustContain(t, "added from its page", page, "Launch paperwork was added to Auth beta.")
	mustContain(t, "part of", page, "Part of")

	// FR-2: with a done feature beside it, the milestone is 1 of 2 until every
	// job is ticked.
	h.postPlan("/ui/milestone/member/add", map[string]string{
		"milestone_id": beta.ID.String(), "member_type": "feature", "member_id": fx.login.String()}, true)
	h.markDone("auth/login")
	if done, total := h.milestoneItems(beta.ID); done != 1 || total != 2 {
		t.Errorf("before ticking: %d of %d, want 1 of 2", done, total)
	}
	_, mPage := h.getUI("/ui/m/" + beta.ID.String())
	mustContain(t, "not complete", mPage, "1 of 2 items")

	// FR-4: tick from the page, with and without a note; refusals in sentences.
	_, page = h.postPlan("/ui/job/tick", map[string]string{"checklist_id": cid, "job_id": key.ID.String(),
		"tick": "true", "note": "It is in the team vault."}, false)
	mustContain(t, "ticked", page, "Ticked: Get the API key.")
	mustContain(t, "who", page, "Ticked by "+actor)
	mustContain(t, "note", page, "It is in the team vault.")
	_, page = h.postPlan("/ui/job/tick", map[string]string{"checklist_id": cid, "job_id": key.ID.String(), "tick": "true"}, false)
	mustContain(t, "twice", page, "That job is already ticked.")
	_, page = h.postPlan("/ui/job/tick", map[string]string{"checklist_id": cid, "job_id": contract.ID.String(), "tick": "false"}, false)
	mustContain(t, "untick unticked", page, "That job isn&#39;t ticked.")
	// A ticked job's title can't change (R14-2); its note can.
	_, frag = h.postPlan("/ui/job/edit", map[string]string{"checklist_id": cid, "job_id": key.ID.String(),
		"title": "Something else", "note": ""}, true)
	mustContain(t, "rename refused", frag, "A ticked job&#39;s title can&#39;t change")
	// A job that has gone, or is on another checklist, is answered on the page (R14-8).
	_, page = h.postPlan("/ui/job/tick", map[string]string{"checklist_id": cid, "job_id": party.ID.String(), "tick": "true"}, false)
	mustContain(t, "gone", page, "That job is no longer on this checklist")

	for _, j := range []store.Job{contract, icon} {
		h.postPlan("/ui/job/tick", map[string]string{"checklist_id": cid, "job_id": j.ID.String(), "tick": "true"}, false)
	}
	if done, total := h.milestoneItems(beta.ID); done != 2 || total != 2 {
		t.Errorf("after ticking every job: %d of %d, want 2 of 2", done, total)
	}
	_, page = h.getUI("/ui/c/" + cid)
	mustContain(t, "done", page, "Done: every job is ticked")
	_, mPage = h.getUI("/ui/m/" + beta.ID.String())
	mustContain(t, "complete", mPage, "2 of 2 items")

	// Untick one: it is open again, and the trail has both ticks' surface.
	_, page = h.postPlan("/ui/job/tick", map[string]string{"checklist_id": cid, "job_id": icon.ID.String(),
		"tick": "false", "note": "The designer wants another look."}, false)
	mustContain(t, "unticked", page, "Unticked: Choose the app icon.")
	if done, _ := h.milestoneItems(beta.ID); done != 1 {
		t.Errorf("after an untick: %d done, want 1", done)
	}
	if n := h.auditCount("job.ticked", actor); n != 3 {
		t.Errorf("job.ticked by %s = %d, want 3", actor, n)
	}
	var via string
	if err := h.srv.Store.Pool.QueryRow(ctx, `
		SELECT payload->>'via' FROM audit_events WHERE kind = 'job.unticked' AND ref_id = $1`, cl.ID).Scan(&via); err != nil || via != "ui" {
		t.Errorf("job.unticked via = %q (%v), want ui", via, err)
	}

	// Shipping with the checklist unticked records it as not shipped (SD-4).
	_, frag = h.postPlan("/ui/milestone/lock", map[string]string{"milestone_id": beta.ID.String()}, true)
	mustContain(t, "shipped", frag, "The one not done is recorded as not shipped.")
	var notShipped string
	if err := h.srv.Store.Pool.QueryRow(ctx, `
		SELECT payload->'not_shipped'->0->>'name' FROM audit_events WHERE kind = 'milestone.locked' AND ref_id = $1`,
		beta.ID).Scan(&notShipped); err != nil || notShipped != "Launch paperwork" {
		t.Errorf("not_shipped = %q (%v), want the checklist", notShipped, err)
	}

	// A page rendered in answer to a boosted post tells HTMX its own address,
	// so an editor that reloads after a change reloads that page and not the
	// post's route.
	if got := h.boostedPostURL("/ui/checklist/new", map[string]string{"owner_type": "initiative", "id": authID, "name": "Second"}); got != "/ui/i/auth" {
		t.Errorf("after creating: HX-Push-Url = %q, want /ui/i/auth", got)
	}
	if got := h.boostedPostURL("/ui/job/tick", map[string]string{"checklist_id": cid, "job_id": icon.ID.String(), "tick": "true"}); got != "/ui/c/"+cid {
		t.Errorf("after ticking: HX-Push-Url = %q, want the checklist page", got)
	}

	// An unknown checklist is a 404.
	if code, _ := h.getUI("/ui/c/" + uuid.NewString()); code != 404 {
		t.Errorf("unknown checklist page: %d, want 404", code)
	}

	// NFR-4: no page or fragment here takes a typed path.
	for _, p := range []string{"/ui/i/auth", "/ui/c/" + cid, "/ui/c/" + cid + "/edit", "/ui/m/" + beta.ID.String() + "/edit"} {
		_, body := h.getUI(p)
		for _, banned := range typedPathFieldNames {
			if strings.Contains(body, banned) {
				t.Errorf("%s renders a typed entity-path field %s", p, banned)
			}
		}
	}
	// The activity feed reads the new kinds as sentences.
	if got := eventLabel("job.ticked"); got != "ticked a job" {
		t.Errorf("eventLabel(job.ticked) = %q", got)
	}
}

// TestMCPChecklistTools is SPEC-014's chat half: a checklist built and kept
// over POST /mcp as planning, put in a milestone, and ticked only by relaying
// a person's quoted words.
func TestMCPChecklistTools(t *testing.T) {
	h := newHarness(t)
	h.setupPlanFixture()
	ctx := context.Background()
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

	// FR-7.1: create with an owner and its first jobs.
	cl := call("create_checklist", map[string]any{"name": "Launch paperwork", "owner_path": "auth",
		"jobs": []any{"Get the API key", "Sign the contract"}})
	if owner := cl["owner"].(map[string]any); owner["type"] != "initiative" || owner["path"] != "auth" {
		t.Errorf("owner = %v", owner)
	}
	if cl["jobs_total"] != float64(2) || cl["done"] != false {
		t.Errorf("new checklist = %v", cl)
	}
	callErr("create_checklist", map[string]any{"name": "x", "owner_path": "nope"}, "no initiative at \"nope\"")
	callErr("create_checklist", map[string]any{"name": "x", "jobs": "one"}, "list of job titles")
	cid := cl["id"].(string)

	// FR-7.2 to FR-7.5: add, rename, move, remove.
	out := call("add_job", map[string]any{"checklist": "Launch paperwork", "title": "Choose an icon", "position": 1})
	jobs := out["jobs"].([]any)
	if jobs[0].(map[string]any)["title"] != "Choose an icon" {
		t.Errorf("added at position 1: %v", jobs)
	}
	call("rename_job", map[string]any{"checklist": cid, "job": "Choose an icon", "title": "Choose the app icon",
		"note": "Two options from the designer."})
	call("move_job", map[string]any{"checklist": cid, "job": "Choose the app icon", "position": 3})
	call("add_job", map[string]any{"checklist": cid, "title": "Book a party"})
	out = call("remove_job", map[string]any{"checklist": cid, "job": "Book a party", "reason": "Not this time."})
	var titles []string
	for _, j := range out["jobs"].([]any) {
		titles = append(titles, j.(map[string]any)["title"].(string))
	}
	if strings.Join(titles, "|") != "Get the API key|Sign the contract|Choose the app icon" {
		t.Errorf("order = %v", titles)
	}
	callErr("add_job", map[string]any{"checklist": cid, "title": " "}, "a job needs a title")
	callErr("move_job", map[string]any{"checklist": cid, "job": "Nope", "position": 1}, "has no job called")
	callErr("get_checklist", map[string]any{"checklist": "Nothing"}, "no checklist called")

	// FR-7.8: into a milestone; the milestone reads it back, counted, not done.
	call("create_milestone", map[string]any{"name": "Auth beta", "owner_path": "auth"})
	call("add_milestone_member", map[string]any{"milestone": "Auth beta", "member_type": "checklist", "member": "Launch paperwork"})
	call("add_milestone_member", map[string]any{"milestone": "Auth beta", "member_type": "feature", "member": "auth/login"})
	h.markDone("auth/login")
	m := call("get_milestone", map[string]any{"milestone": "Auth beta"})
	if m["items_done"] != float64(1) || m["items_total"] != float64(2) {
		t.Errorf("milestone items = %v of %v, want 1 of 2", m["items_done"], m["items_total"])
	}
	var member map[string]any
	for _, raw := range m["members"].([]any) {
		if mm := raw.(map[string]any); mm["type"] == "checklist" {
			member = mm
		}
	}
	if member == nil || member["id"] != cid || member["done"] != false || member["path"] != nil {
		t.Errorf("checklist member = %v", member)
	}
	callErr("add_milestone_member", map[string]any{"milestone": "Auth beta", "member_type": "checklist", "member": "Nope"}, "no checklist called")

	// FR-7.9: no tick without the person's words, and nothing written.
	callErr("relay_tick_job", map[string]any{"checklist": cid, "job": "Get the API key", "ticked": true}, "the person's words are required")
	callErr("relay_tick_job", map[string]any{"checklist": cid, "job": "Get the API key", "quote": "done"}, "say whether the job is done")
	if n := h.auditCount("job.ticked", actor); n != 0 {
		t.Fatalf("a refused relay wrote %d ticks", n)
	}
	out = call("relay_tick_job", map[string]any{"checklist": cid, "job": "Get the API key", "ticked": true,
		"note": "It is in the vault.", "quote": "I've got the API key, it's in the vault"})
	job := out["job"].(map[string]any)
	if job["ticked"] != true || job["ticked_via"] != "mcp" || job["ticked_by"] != actor ||
		job["ticked_quote"] != "I've got the API key, it's in the vault" || job["note"] != "It is in the vault." {
		t.Errorf("relayed job = %v", job)
	}
	var quote, via string
	if err := h.srv.Store.Pool.QueryRow(ctx, `
		SELECT payload->>'quote', payload->>'via' FROM audit_events WHERE kind = 'job.ticked' AND actor = $1`, actor).Scan(&quote, &via); err != nil ||
		quote != "I've got the API key, it's in the vault" || via != "mcp" {
		t.Errorf("job.ticked audit: quote %q via %q (%v)", quote, via, err)
	}
	callErr("relay_tick_job", map[string]any{"checklist": cid, "job": "Get the API key", "ticked": true, "quote": "again"}, "already ticked")
	// A ticked job's title can't change (R14-2).
	callErr("rename_job", map[string]any{"checklist": cid, "job": "Get the API key", "title": "Sign the contract"}, "can't change")
	// A job id from another checklist is refused (R14-8).
	other := call("create_checklist", map[string]any{"name": "Other", "jobs": []any{"Elsewhere"}})
	otherJob := other["jobs"].([]any)[0].(map[string]any)["id"].(string)
	callErr("relay_tick_job", map[string]any{"checklist": cid, "job": otherJob, "ticked": true, "quote": "done"}, "has no job called")

	// Unticking is relayed the same way.
	call("relay_tick_job", map[string]any{"checklist": cid, "job": "Get the API key", "ticked": false,
		"quote": "Actually the key was revoked"})
	if n := h.auditCount("job.unticked", actor); n != 1 {
		t.Errorf("job.unticked by the chat agent = %d, want 1", n)
	}

	// SD-7 (R14-3): the chat agent can't finish a checklist by removing its
	// last unticked job; ticking every job does finish it.
	for _, title := range []string{"Get the API key", "Sign the contract"} {
		call("relay_tick_job", map[string]any{"checklist": cid, "job": title, "ticked": true, "quote": "done: " + title})
	}
	callErr("remove_job", map[string]any{"checklist": cid, "job": "Choose the app icon"}, "would leave the checklist done")
	out = call("relay_tick_job", map[string]any{"checklist": cid, "job": "Choose the app icon", "ticked": true,
		"quote": "We picked the blue icon"})
	if out["checklist"].(map[string]any)["done"] != true {
		t.Errorf("after every job is ticked: %v", out["checklist"])
	}
	m = call("get_milestone", map[string]any{"milestone": "Auth beta"})
	if m["items_done"] != float64(2) {
		t.Errorf("milestone items done = %v, want 2", m["items_done"])
	}

	// FR-7.6, FR-7.7: list and get.
	list := call("list_checklists", map[string]any{"owner_type": "initiative", "owner_path": "auth"})
	if cls := list["checklists"].([]any); len(cls) != 1 || cls[0].(map[string]any)["name"] != "Launch paperwork" {
		t.Errorf("auth's checklists = %v", list)
	}
	if all := call("list_checklists", map[string]any{}); len(all["checklists"].([]any)) != 2 {
		t.Errorf("every checklist = %v", all)
	}
	got := call("get_checklist", map[string]any{"checklist": "Launch paperwork"})
	if ms := got["milestones"].([]any); len(ms) != 1 || ms[0].(map[string]any)["name"] != "Auth beta" {
		t.Errorf("milestones = %v", got["milestones"])
	}
	// The relayed words show on the checklist page (SD-8).
	_, page := h.getUI("/ui/c/" + cid)
	mustContain(t, "quote on the page", page, "We picked the blue icon")

	// Every write is on the trail under the MCP actor.
	for kind, want := range map[string]int{"checklist.created": 2, "job.added": 5, "job.edited": 1,
		"job.moved": 2, "job.removed": 1, "job.ticked": 4, "milestone.member_added": 2} {
		if n := h.auditCount(kind, actor); n != want {
			t.Errorf("%s by the chat agent = %d, want %d", kind, n, want)
		}
	}
}
