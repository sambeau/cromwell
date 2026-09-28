package server

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"subutai/internal/store"
)

// TestChecklistPagesRender renders every SPEC-014 template in each of its
// shapes and checks the parts of the contract that live in markup: the page is
// a list of checkboxes, each a one-click tick or untick with a way to add a
// note (FR-4.2, FR-4.3), who ticked each job is shown, with the person's words
// for a relayed tick (SD-8), the editor is a numbered list whose ends have no
// Up or Down (FR-5.2), a ticked job's title can't be changed (R14-2), an
// owner's plan offers New checklist and lists its checklists (FR-3), a
// checklist in a milestone's editor shows its jobs ticked (FR-2.6), and no
// table or typed entity path appears anywhere (NFR-4, NFR-5).
func TestChecklistPagesRender(t *testing.T) {
	tmpl, err := loadUITemplates()
	if err != nil {
		t.Fatal(err)
	}
	cID := uuid.New()
	by, ago := "operator", time.Now().Add(-2*time.Hour)
	jobs := []jobRow{
		{ID: uuid.New(), Place: 1, Title: "Get the API key", Note: "It is in the vault.", Ticked: true,
			TickedBy: by, TickedAt: ago, First: true},
		{ID: uuid.New(), Place: 2, Title: "Sign the contract", Ticked: true, TickedBy: "chat-agent",
			TickedAt: ago, Relayed: true, Quote: "I've signed it"},
		{ID: uuid.New(), Place: 3, Title: "Choose an icon", Last: true},
	}
	card := checklistCard{ID: cID, Name: "Launch paperwork", URL: checklistURL(cID), Jobs: 3, Ticked: 2}
	choices := []milestoneChoiceGroup{{Label: "Planned in Project", Options: []milestoneChoice{{ID: uuid.New(), Name: "Launch"}}}}
	page := checklistPage{
		Checklist: store.Checklist{ID: cID, Name: "Launch paperwork", Description: "The jobs only people can do."},
		Card:      card, Owner: crumb{Label: "Project", URL: "/ui/project"}, Jobs: jobs,
		MemberOf: []milestoneCard{sampleMilestoneCard()}, MilestoneChoices: choices, Notice: "Ticked: Get the API key.",
	}
	empty := checklistPage{Checklist: store.Checklist{ID: cID, Name: "Empty"},
		Card: checklistCard{ID: cID, Name: "Empty"}, Owner: crumb{Label: "Project", URL: "/ui/project"}}
	editor := &checklistEditor{Checklist: page.Checklist, Card: card, Owner: page.Owner, Jobs: jobs,
		Notice: "Added: Choose an icon.", Changed: true}
	initiative := &entityPage{Kind: "initiative", RefType: "initiative", ID: uuid.New(), Title: "Authentication",
		OwnedChecklists: []checklistCard{card}}
	mEditor := &milestoneEditor{
		Milestone: store.Milestone{ID: uuid.New(), Name: "Launch", State: "open"},
		Card:      sampleMilestoneCard(), Owner: crumb{Label: "Project", URL: "/ui/project"},
		Members: []memberRow{{ID: cID, Label: "Launch paperwork", URL: card.URL, Kind: "checklist", Sub: card.Status()}},
		CanLock: true, LeafCount: 2, LeafDone: 1,
	}

	render := func(name string, data any) string {
		t.Helper()
		var buf bytes.Buffer
		if err := tmpl.t.ExecuteTemplate(&buf, name, data); err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		return buf.String()
	}
	pages := map[string]string{
		"checklist page":   render("page-checklist", pageData{Data: page}),
		"empty checklist":  render("page-checklist", pageData{Data: empty}),
		"editor":           render("checklist-editor-dialog", editor),
		"initiative page":  render("page-entity", pageData{Data: initiative}),
		"milestone editor": render("milestone-editor", mEditor),
	}

	// FR-4.2, FR-4.3: checkboxes that tick and untick, with a note if wanted.
	cp := pages["checklist page"]
	mustContain(t, "tick", cp, `aria-label="Tick Choose an icon"`)
	mustContain(t, "untick", cp, `aria-label="Untick Get the API key"`)
	mustContain(t, "checkbox role", cp, `role="checkbox" aria-checked="true"`)
	mustContain(t, "tick route", cp, `action="/ui/job/tick"`)
	mustContain(t, "with a note", cp, "Tick with a note…")
	mustContain(t, "note shown", cp, "It is in the vault.")
	mustContain(t, "status", cp, "2 of 3 jobs ticked")
	// SD-8: who ticked it, and the person's words for a relay.
	mustContain(t, "who", cp, "Ticked by operator")
	mustContain(t, "relayed words", cp, "relaying the words “<span class=\"job-quote\">I&#39;ve signed it</span>”")
	// FR-4.1, FR-4.4: edit and add to a milestone from the page.
	mustContain(t, "edit button", cp, "/ui/c/"+cID.String()+"/edit")
	mustContain(t, "joins milestones", cp, `name="member_type" value="checklist"`)
	mustContain(t, "part of", cp, "Part of")
	mustContain(t, "empty", pages["empty checklist"], "This checklist has no jobs yet, so it isn't done")

	// FR-5: the editor.
	ed := pages["editor"]
	mustContain(t, "autoshow", ed, "data-autoshow")
	mustContain(t, "changed", ed, `data-changed="true"`)
	mustContain(t, "move down", ed, "Move Get the API key down")
	mustContain(t, "move up", ed, "Move Choose an icon up")
	if strings.Contains(ed, "Move Get the API key up") || strings.Contains(ed, "Move Choose an icon down") {
		t.Error("an end of the list offers a move past it")
	}
	mustContain(t, "add a job", ed, `action="/ui/job/add"`)
	mustContain(t, "remove", ed, `action="/ui/job/remove"`)
	mustContain(t, "ticked title fixed", ed, "The title can't change while the job is ticked.")
	mustContain(t, "unticked title editable", ed, `name="title" value="Choose an icon" required`)
	if strings.Contains(ed, `action="/ui/job/tick"`) {
		t.Error("the editor ticks jobs; ticking belongs to the page (SD-5)")
	}

	// FR-3: the owner's plan.
	ip := pages["initiative page"]
	mustContain(t, "new checklist", ip, `action="/ui/checklist/new"`)
	mustContain(t, "listed", ip, "Launch paperwork")
	mustContain(t, "edit from owner", ip, "/ui/c/"+cID.String()+"/edit")

	// FR-2.6: in a milestone's editor, a checklist shows its jobs ticked.
	mustContain(t, "member status", pages["milestone editor"], "2 of 3 jobs ticked · checklist")
	mustContain(t, "items", pages["milestone editor"], "features and checklists alike")

	for name, body := range pages {
		if strings.Contains(body, "<table") {
			t.Errorf("%s renders a table; a checklist is a list of checkboxes (D-10)", name)
		}
		for _, banned := range typedPathFieldNames {
			if strings.Contains(body, banned) {
				t.Errorf("%s renders a typed entity-path field %s (NFR-4)", name, banned)
			}
		}
	}
}
