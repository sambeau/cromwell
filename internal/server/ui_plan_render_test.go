package server

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"cromwell/internal/store"
)

// TestPlanEditorsRender renders every SPEC-010 template in each of its shapes
// and checks the parts of the contract that live in markup: the lock is
// disabled with a plain reason when G4 would refuse (FR-5.1), the confirm step
// says the lock is permanent (FR-5.2), a milestone is edited as a checklist and
// a roadmap as a numbered list with no table anywhere (D-10, NFR-5), and no
// field invites a typed entity path (NFR-4).
func TestPlanEditorsRender(t *testing.T) {
	tmpl, err := loadUITemplates()
	if err != nil {
		t.Fatal(err)
	}
	target := time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	mID := uuid.New()
	picker := memberPicker{MilestoneID: mID, ScopeLabel: "Authentication and everything inside it",
		Candidates: []store.MemberCandidate{
			{Kind: "initiative", ID: uuid.New(), Name: "Basic sign-in", Path: "auth/basic"},
			{Kind: "feature", ID: uuid.New(), Name: "Login form", Path: "auth/basic/login"},
			{Kind: "milestone", ID: uuid.New(), Name: "Basic done"},
		}, More: true}
	members := []memberRow{
		{ID: uuid.New(), Label: "Login form", URL: "/ui/f/auth/login", Done: true, Kind: "feature"},
		{ID: uuid.New(), Label: "Passkeys", URL: "/ui/i/auth/passkeys", Kind: "initiative"},
	}
	canLock := &milestoneEditor{
		Milestone: store.Milestone{ID: mID, Name: "Auth beta", TargetDate: &target, State: "open"},
		Card:      sampleMilestoneCard(), Owner: crumb{Label: "Authentication", URL: "/ui/i/auth"},
		Members: members, CanLock: true, LeafCount: 4, LeafDone: 1, Picker: picker,
		Notice: "Login form was added to Auth beta.", Changed: true,
	}
	blocked := &milestoneEditor{
		Milestone: store.Milestone{ID: mID, Name: "Auth beta", State: "open"},
		Card:      sampleMilestoneCard(), Owner: crumb{Label: "Project", URL: "/ui/project"},
		Members: members, LockReason: g4Plain(3, 0), LeafCount: 3,
		Picker: memberPicker{MilestoneID: mID, Query: "zzz", ScopeLabel: "the whole project"},
		Error:  "This milestone is locked, so what it contains is fixed.",
	}
	locked := &milestoneEditor{
		Milestone: store.Milestone{ID: mID, Name: "Auth beta", State: "locked"},
		Card:      sampleMilestoneCard(), Owner: crumb{Label: "Project", URL: "/ui/project"},
		Members: members, Locked: true,
	}
	rmID := uuid.New()
	a, b := sampleMilestoneCard(), sampleMilestoneCard()
	b.Name, b.Locked = "General release", true
	roadmap := &roadmapEditor{
		Roadmap:   store.Roadmap{ID: rmID, Name: "Auth plan"},
		Owner:     crumb{Label: "Authentication", URL: "/ui/i/auth"},
		Entries:   []roadmapEntryRow{{Place: 1, Card: a, First: true}, {Place: 2, Card: b, Last: true}},
		Here:      []milestoneChoice{{ID: uuid.New(), Name: "Auth GA"}},
		Elsewhere: []milestoneChoice{{ID: uuid.New(), Name: "Launch"}},
	}
	emptyRoadmap := &roadmapEditor{Roadmap: store.Roadmap{ID: rmID, Name: "Empty"},
		Owner: crumb{Label: "Project", URL: "/ui/project"}}
	choices := []milestoneChoiceGroup{{Label: "Planned in Project", Options: []milestoneChoice{{ID: uuid.New(), Name: "Launch"}}}}
	featurePage := &entityPage{
		Kind: "feature", RefType: "feature", ID: uuid.New(), Title: "Login form",
		MemberOf: []milestoneCard{sampleMilestoneCard()}, MilestoneChoices: choices,
	}
	emptyInitiative := &entityPage{Kind: "initiative", RefType: "initiative", ID: uuid.New(), Title: "Authentication"}
	mPage := milestonePage{
		Milestone: store.Milestone{ID: mID, Name: "Auth beta", TargetDate: &target},
		Card:      sampleMilestoneCard(), Owner: crumb{Label: "Authentication", URL: "/ui/i/auth"},
		Members: members, MilestoneChoices: choices, Notice: "Added.",
	}

	render := func(name string, data any) string {
		t.Helper()
		var buf bytes.Buffer
		if err := tmpl.t.ExecuteTemplate(&buf, name, data); err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		if buf.Len() == 0 {
			t.Fatalf("render %s produced nothing", name)
		}
		return buf.String()
	}
	pages := map[string]string{
		"editor, lockable":      render("milestone-editor-dialog", canLock),
		"editor, G4 would fail": render("milestone-editor", blocked),
		"editor, locked":        render("milestone-editor", locked),
		"candidates":            render("member-candidates", picker),
		"roadmap editor":        render("roadmap-editor-dialog", roadmap),
		"empty roadmap editor":  render("roadmap-editor", emptyRoadmap),
		"feature page":          render("page-entity", pageData{Data: featurePage}),
		"empty initiative page": render("page-entity", pageData{Data: emptyInitiative}),
		"milestone page":        render("page-milestone", pageData{Data: mPage}),
	}

	// FR-6: the loaded editor opens itself and reloads the page if it changed.
	mustContain(t, "autoshow", pages["editor, lockable"], `data-autoshow`)
	mustContain(t, "changed marker", pages["editor, lockable"], `data-changed="true"`)
	// FR-5.2: the confirm step is in the page and says what locking means.
	mustContain(t, "permanent", pages["editor, lockable"], "Locking is permanent")
	mustContain(t, "no unlock", pages["editor, lockable"], "can't be unlocked")
	mustContain(t, "confirm value", pages["editor, lockable"], `name="confirm" value="permanent"`)
	// FR-5.1: the disabled lock with G4's reason in plain words.
	mustContain(t, "disabled lock", pages["editor, G4 would fail"], "disabled")
	mustContain(t, "plain reason", pages["editor, G4 would fail"], "none of its 3 features is done")
	if strings.Contains(pages["editor, G4 would fail"], `value="permanent"`) {
		t.Error("an unlockable milestone still offers the lock form")
	}
	// FR-3.5: a locked milestone offers no add, remove or lock.
	for _, banned := range []string{"/ui/milestone/member/add", "/ui/milestone/member/remove", "/ui/milestone/lock"} {
		if strings.Contains(pages["editor, locked"], banned) {
			t.Errorf("the locked editor still posts to %s", banned)
		}
	}
	// FR-3.2 / FR-3.3: the picker says what it shows, and that there is more.
	mustContain(t, "scope label", pages["editor, lockable"], "Authentication and everything inside it")
	mustContain(t, "more", pages["candidates"], "Showing the first 50")
	mustContain(t, "no match", pages["editor, G4 would fail"], "Nothing else in the project matches")
	// FR-4: move controls respect the ends; the place select offers positions.
	mustContain(t, "move down", pages["roadmap editor"], "Move First release down")
	mustContain(t, "move up", pages["roadmap editor"], "Move General release up")
	if strings.Contains(pages["roadmap editor"], "Move First release up") {
		t.Error("the first entry offers Move up")
	}
	mustContain(t, "after option", pages["roadmap editor"], "After First release")
	mustContain(t, "groups", pages["roadmap editor"], "Planned elsewhere")
	// FR-2.1: an initiative with no plan still offers to start one.
	mustContain(t, "empty plan", pages["empty initiative page"], "Nothing is planned here yet")
	mustContain(t, "new milestone", pages["empty initiative page"], `action="/ui/milestone/new"`)
	mustContain(t, "new roadmap", pages["empty initiative page"], `action="/ui/roadmap/new"`)
	// FR-3.1: the member's end, on a feature and on a milestone.
	mustContain(t, "add to a milestone", pages["feature page"], "Add to a milestone…")
	mustContain(t, "grouped choice", pages["feature page"], `optgroup label="Planned in Project"`)
	mustContain(t, "milestone joins milestones", pages["milestone page"], `name="member_type" value="milestone"`)
	mustContain(t, "milestone page edits", pages["milestone page"], "/ui/m/"+mID.String()+"/edit")

	// D-10: a checklist and a numbered list, never a table.
	mustContain(t, "checklist", pages["editor, lockable"], `class="check-item"`)
	mustContain(t, "ordered list", pages["roadmap editor"], "<ol")
	// NFR-4: nothing here takes a typed entity path.
	for name, body := range pages {
		if strings.Contains(body, "<table") {
			t.Errorf("%s renders a table; the plan is two lists (D-10)", name)
		}
		for _, banned := range typedPathFieldNames {
			if strings.Contains(body, banned) {
				t.Errorf("%s renders a typed entity-path field %s (NFR-4)", name, banned)
			}
		}
	}
}

// typedPathFieldNames are the form field names that would mean an entity is
// being typed rather than chosen (SPEC-007 SD-6, SPEC-010 NFR-4).
var typedPathFieldNames = []string{`name="ref"`, `name="path"`, `name="parent_path"`,
	`name="initiative_path"`, `name="milestone"`, `name="roadmap"`, `name="member"`, `name="owner_path"`}
