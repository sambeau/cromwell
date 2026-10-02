package server

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"subutai/internal/sizing"
	"subutai/internal/store"
)

// TestUITemplatesParseAndRender exercises every page and fragment template with
// representative data, so a template bug (an undefined func, a bad field
// reference) is caught in a fast unit test rather than only at request time.
func TestUITemplatesParseAndRender(t *testing.T) {
	tmpl, err := loadUITemplates()
	if err != nil {
		t.Fatalf("loadUITemplates: %v", err)
	}

	now := time.Now().Add(-3 * time.Minute)
	featSize := sizing.Rollup{Tokens: 1200, Tier: sizing.TierConsidered, Estimated: true, Complete: true}
	sum := &DashboardSummary{
		Running:   []store.Dispatch{{ID: uuid.New(), Purpose: "review", RefID: uuid.New()}},
		Queued:    []store.Dispatch{{ID: uuid.New(), Purpose: "implement", QueueReason: strptr("waiting on contract")}},
		Escalated: 2,
		Waiting: []store.Checkpoint{{ID: uuid.New(), Kind: "review-escalation",
			Question: "The reviewer could not decide. Approve?"}},
		Recent:  []store.AuditEvent{{OccurredAt: now, Actor: "orchestrator", Kind: "dispatch.succeeded", RefType: "task"}},
		Project: sizing.Rollup{Tokens: 5000, Tier: sizing.TierRough, Estimated: true, Decomposed: true},
		ByState: []featureStateGroup{{State: "ready", Features: []entityWork{
			{Name: "Login form", URL: "/ui/f/auth/login", Size: featSize}}}},
		Unestimated:    []sizing.Ref{{Type: "feature", Name: "Password reset"}},
		TopInitiatives: []entityWork{{Name: "Authentication", URL: "/ui/i/auth", Size: featSize}},
		Milestones:     []milestoneCard{sampleMilestoneCard()},
		Roadmaps:       []roadmapCard{{ID: uuid.New(), Name: "First release", URL: "/ui/r/x", Milestones: []milestoneCard{sampleMilestoneCard()}}},
		ProjectDocs:    []docCard{{ID: uuid.New(), Title: "Vision", Type: "design", State: "approved", URL: "/ui/d/docs/vision.md"}},
		Calibration: []store.CorpusRow{{
			RefType: "feature", Name: "login", EstimateTokens: 1000, EstimateTier: sizing.TierConsidered, ActualTokens: 1234,
		}},
	}
	inbox := []inboxItem{{
		Checkpoint: store.Checkpoint{ID: uuid.New(), Kind: "dispatch-failure", RefType: "task",
			Question: "The implementer failed. Retry?", CreatedAt: now, Context: []byte(`{"error":"boom"}`)},
		Options: answerOptions("dispatch-failure"),
	}}
	docs := []store.Document{{ID: uuid.New(), Type: "spec", State: "approved", OwnerType: "feature", Path: "docs/x.md", Title: "X", CreatedAt: now}}
	docPage := &docPageData{
		Document: docs[0],
		Body:     renderMarkdown("# Title\n\nSome **bold** and `code`.\n"),
		Comments: []store.Comment{{Author: "reviewer", Body: "looks good", SectionRef: "Intro"}},
	}
	work := workView{
		Project:     sizing.Rollup{Tokens: 5000, Tier: sizing.TierRough, Estimated: true},
		Initiatives: []entityWork{{Name: "auth", URL: "/ui/i/auth", Size: featSize}},
		Features:    []entityWork{{Name: "auth/login", URL: "/ui/f/auth/login", Size: sizing.Rollup{}}},
		Milestones:  []milestoneCard{sampleMilestoneCard()},
	}

	// The entity page in each of its three shapes, plus a feature whose start
	// is gated (the disabled-with-a-reason case, FR-2.2).
	featurePage := &entityPage{
		Kind: "feature", RefType: "feature", ID: uuid.New(), Title: "Login form",
		Path: "auth/login", Description: "How a person signs in to the product.",
		Editable: true, State: "idea", Size: featSize,
		Breadcrumbs: []crumb{{Label: "Project", URL: "/ui/project"},
			{Label: "Authentication", URL: "/ui/i/auth"}, {Label: "Login form", Here: true}},
		HasBody: true, Body: renderMarkdown("## Design\n\nThe form has two fields.\n"),
		BodyDoc:  &docCard{ID: uuid.New(), Title: "Login design", Type: "design", State: "approved", URL: "/ui/d/docs/login.md", IsPrimary: true},
		CanStart: false, StartReason: featureStartReason("idea", true, false), CanAbandon: true,
		Documents: []docCard{{ID: uuid.New(), Title: "Login spec", Type: "spec", State: "approved", URL: "/ui/d/docs/spec.md"}},
		MemberOf:  []milestoneCard{sampleMilestoneCard()},
		Activity:  []store.AuditEvent{{OccurredAt: now, Actor: "operator", Kind: "feature.created"}},
		Notice:    "The description was updated.",
	}
	initiativePage := &entityPage{
		Kind: "initiative", RefType: "initiative", ID: uuid.New(), Title: "Authentication",
		Path: "auth", Description: "Everything about proving who a person is.", Editable: true,
		Breadcrumbs: []crumb{{Label: "Project", URL: "/ui/project"}, {Label: "Authentication", Here: true}},
		Size:        sizing.Rollup{Tokens: 3000, Tier: sizing.TierRough, Estimated: true, Decomposed: true},
		Children: []childCard{{Kind: "feature", Name: "Login form", URL: "/ui/f/auth/login",
			State: "idea", Size: featSize, Description: "How a person signs in."}},
		OwnedMilestones: []milestoneCard{sampleMilestoneCard()},
		OwnedRoadmaps:   []roadmapCard{{ID: uuid.New(), Name: "First release", URL: "/ui/r/x", Milestones: []milestoneCard{sampleMilestoneCard()}}},
		Error:           "Archiving is blocked by gate G5.",
	}
	projectPage := &entityPage{
		Kind: "project", RefType: "project", Title: "Project",
		Breadcrumbs: []crumb{{Label: "Project", Here: true}},
		Size:        sizing.Rollup{Tokens: 5000, Tier: sizing.TierRough, Estimated: true},
		Children:    []childCard{{Kind: "initiative", Name: "Authentication", URL: "/ui/i/auth", Size: featSize}},
	}
	// The empty-state body (FR-3.1): no design document attached yet.
	emptyBodyPage := &entityPage{
		Kind: "feature", RefType: "feature", ID: uuid.New(), Title: "Password reset",
		Breadcrumbs: []crumb{{Label: "Project", URL: "/ui/project"}, {Label: "Password reset", Here: true}},
		State:       "ready", CanStart: true, CanAbandon: true,
	}

	mPage := milestonePage{
		Milestone: store.Milestone{ID: uuid.New(), Name: "First release", Description: "What ships first."},
		Card:      sampleMilestoneCard(),
		Owner:     crumb{Label: "Authentication", URL: "/ui/i/auth"},
		Members: []memberRow{
			{Label: "Login form", URL: "/ui/f/auth/login", Done: true, Kind: "feature"},
			{Label: "Password reset", URL: "/ui/f/auth/reset", Done: false, Kind: "feature"},
		},
	}
	rPage := roadmapPage{
		Roadmap: store.Roadmap{ID: uuid.New(), Name: "The road to version one"},
		Owner:   crumb{Label: "Project", URL: "/ui/project"},
		Card:    roadmapCard{Name: "The road to version one", Milestones: []milestoneCard{sampleMilestoneCard()}},
	}
	tPage := taskPage{
		Task:        store.Task{ID: uuid.New(), Title: "Add the password field", State: "pending", Description: "A short task."},
		FeatureName: "Login form", FeatureURL: "/ui/f/auth/login",
	}
	edPage := entityDocPage{
		docPageData: docPage,
		Breadcrumbs: []crumb{{Label: "Login form", URL: "/ui/f/auth/login"}, {Label: "X", Here: true}},
		OwnerCrumb:  crumb{Label: "Login form", URL: "/ui/f/auth/login"},
	}

	cases := []struct {
		name      string
		data      any
		mayBeZero bool
	}{
		{"page-dashboard", pageData{Active: "home", Actor: "op", Data: sum}, false},
		{"frag-queue", sum, false},
		{"frag-work", sum, false},
		{"frag-events", sum, false},
		{"frag-calibration", sum, false},
		{"page-inbox", pageData{Active: "inbox", Actor: "op", Data: inbox}, false},
		{"frag-inbox", inbox, false},
		{"frag-inbox", []inboxItem(nil), false}, // empty inbox renders the "clear" state
		{"frag-inbox-badge", 2, false},
		{"frag-inbox-badge", 0, true}, // a zero badge is deliberately empty
		{"page-documents", pageData{Active: "documents", Actor: "op", Data: docs}, false},

		// The workflow surface (SPEC-007).
		{"page-entity", pageData{Active: "browse", Actor: "op", Data: featurePage}, false},
		{"page-entity", pageData{Active: "browse", Actor: "op", Data: initiativePage}, false},
		{"page-entity", pageData{Active: "browse", Actor: "op", Data: projectPage}, false},
		{"page-entity", pageData{Active: "browse", Actor: "op", Data: emptyBodyPage}, false},
		{"page-milestone", pageData{Active: "browse", Actor: "op", Data: mPage}, false},
		{"page-roadmap", pageData{Active: "browse", Actor: "op", Data: rPage}, false},
		{"page-task", pageData{Active: "browse", Actor: "op", Data: tPage}, false},
		{"page-entity-document", pageData{Active: "documents", Actor: "op", Data: edPage}, false},
		{"page-entity-document", pageData{Active: "documents", Actor: "op", Data: gatedEntityDocPage(edPage)}, false},
		{"page-notfound", pageData{Active: "browse", Actor: "op", Data: map[string]string{"Kind": "feature", "Path": "auth/nope"}}, false},

		{"page-work", pageData{Active: "work", Actor: "op", Data: work}, false},
		{"page-error", pageData{Actor: "op", Data: "something broke"}, false},
	}
	for _, c := range cases {
		var buf bytes.Buffer
		if err := tmpl.t.ExecuteTemplate(&buf, c.name, c.data); err != nil {
			t.Errorf("render %s: %v", c.name, err)
			continue
		}
		if buf.Len() == 0 && !c.mayBeZero {
			t.Errorf("render %s produced no output", c.name)
		}
	}
}

// TestGatedActionShowsPlainReason is FR-2.2's acceptance criterion: an action
// whose gate is not satisfied renders disabled with the reason in plain words,
// never hidden and never a silent failure.
func TestGatedActionShowsPlainReason(t *testing.T) {
	tmpl, err := loadUITemplates()
	if err != nil {
		t.Fatal(err)
	}
	page := &entityPage{
		Kind: "feature", RefType: "feature", ID: uuid.New(), Title: "Login form",
		Breadcrumbs: []crumb{{Label: "Project", URL: "/ui/project"}},
		State:       "idea", CanStart: false, StartReason: featureStartReason("idea", true, false), CanAbandon: true,
		// A sent feature: until then the blocked action is Send, not Start
		// building (SPEC-011 FR-4.1).
		Send: sendCard{Show: true, Sent: true, SentBy: "op"}, ShowStartCard: true,
	}
	var buf bytes.Buffer
	if err := tmpl.t.ExecuteTemplate(&buf, "page-entity", pageData{Active: "browse", Actor: "op", Data: page}); err != nil {
		t.Fatal(err)
	}
	html := buf.String()
	if !strings.Contains(html, "disabled") {
		t.Error("a blocked Start action should render disabled, not be hidden")
	}
	if !strings.Contains(html, "specification is approved") {
		t.Errorf("the gate reason should be in plain words; got:\n%s", html)
	}
}

// TestUnmeasuredActualReadsAsAtLeast is SPEC-020 FR-7.3: an unmeasured
// entity's actual reads as a sentence, not a bare number.
func TestUnmeasuredActualReadsAsAtLeast(t *testing.T) {
	tmpl, err := loadUITemplates()
	if err != nil {
		t.Fatal(err)
	}
	render := func(unmeasured bool) string {
		page := &entityPage{
			Kind: "feature", RefType: "feature", ID: uuid.New(), Title: "Login form",
			Breadcrumbs: []crumb{{Label: "Project", URL: "/ui/project"}},
			State:       "building", Done: 12400, DoneUnmeasured: unmeasured,
		}
		var buf bytes.Buffer
		if err := tmpl.t.ExecuteTemplate(&buf, "page-entity", pageData{Active: "browse", Actor: "op", Data: page}); err != nil {
			t.Fatal(err)
		}
		return buf.String()
	}
	const want = "Not measured: at least 12,400 tokens, plus work done in chat or by a person."
	if !strings.Contains(render(true), want) {
		t.Errorf("an unmeasured actual should read %q", want)
	}
	if strings.Contains(render(false), "Not measured") {
		t.Error("a measured actual should not carry the sentence")
	}
}

// TestRenderedTemplatesCarryNoCurrency is the money check of DoD 3 (D-4, SD-5):
// no rendered page shows a currency figure or a money field. The engine's money
// machinery stays dormant underneath — this asserts it never reaches the human
// surface. Template comments are stripped first, so a comment explaining that
// this page replaced the old Cost view is not itself a violation; what is
// checked is the markup and the data fields it binds to.
func TestRenderedTemplatesCarryNoCurrency(t *testing.T) {
	// A currency symbol before a digit or a template action, the money-
	// formatting helpers, and the money-bearing field names.
	banned := []*regexp.Regexp{
		regexp.MustCompile(`\$\s*[\d{]`),
		regexp.MustCompile(`\b(usd|usd2|clampPct)\b`),
		regexp.MustCompile(`(?i)\b(cost(total|usd)?|budget(cap|warn|period)?|overcap|overwarn|cap_usd)\b`),
	}
	comments := regexp.MustCompile(`(?s)\{\{/\*.*?\*/\}\}`)

	entries, err := uiTemplatesFS.ReadDir("ui/templates")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		body, err := uiTemplatesFS.ReadFile("ui/templates/" + e.Name())
		if err != nil {
			t.Fatal(err)
		}
		src := comments.ReplaceAllString(string(body), "")
		for _, re := range banned {
			if m := re.FindString(src); m != "" {
				t.Errorf("template %s contains %q; money must not appear on the human surface (D-4)", e.Name(), m)
			}
		}
	}
}

func strptr(s string) *string { return &s }

func sampleMilestoneCard() milestoneCard {
	return milestoneCard{ID: uuid.New(), Name: "First release", URL: "/ui/m/x",
		Done: 3, Total: 4, DoneTokens: 3000, TotalTokens: 5000}
}

// gatedDocPage returns a copy of a document page marked human-gated, so the
// review controls render.
func gatedDocPage(p *docPageData) *docPageData {
	cp := *p
	id := uuid.New()
	cp.ReviewCheckpointID = &id
	return &cp
}

func gatedEntityDocPage(p entityDocPage) entityDocPage {
	cp := p
	cp.docPageData = gatedDocPage(p.docPageData)
	return cp
}

// A feature whose specification is approved but whose dev-plan is missing was
// told "work can start once this feature's specification is approved" — an
// instruction to do the thing already done, leaving the reader stuck. The
// reason must name the half of the contract that is actually missing.
func TestFeatureStartReasonNamesTheMissingHalf(t *testing.T) {
	cases := []struct {
		name                          string
		specApproved, devPlanApproved bool
		wantContains, wantAbsent      string
	}{
		{"neither approved", false, false, "specification and an approved dev-plan", ""},
		{"spec missing", false, true, "specification is approved", "dev-plan is approved."},
		{"dev-plan missing", true, false, "dev-plan is approved", "once this feature's specification is approved"},
		{"both approved", true, true, "moment away", "once this"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := featureStartReason("idea", c.specApproved, c.devPlanApproved)
			if !strings.Contains(got, c.wantContains) {
				t.Errorf("reason %q should mention %q", got, c.wantContains)
			}
			if c.wantAbsent != "" && strings.Contains(got, c.wantAbsent) {
				t.Errorf("reason %q must not say %q — it is already true", got, c.wantAbsent)
			}
		})
	}
}
