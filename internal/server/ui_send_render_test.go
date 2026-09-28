package server

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"cromwell/internal/lifecycle"
	"cromwell/internal/store"
)

// TestSendAndReviewTemplatesRender covers SPEC-011's new markup: the send
// screen, the feature page's send card in each state, and the document page's
// actions for a held spec, a spec under agent-review-off, and a draft. Each
// is scanned for typed entity paths (NFR-9) and for the words a person needs.
func TestSendAndReviewTemplatesRender(t *testing.T) {
	tmpl, err := loadUITemplates()
	if err != nil {
		t.Fatalf("loadUITemplates: %v", err)
	}
	render := func(name string, data any) string {
		t.Helper()
		var buf bytes.Buffer
		if err := tmpl.t.ExecuteTemplate(&buf, name, data); err != nil {
			t.Fatalf("render %s: %v", name, err)
		}
		return buf.String()
	}

	steps := []sendStep{
		{Name: "Write the specification", Who: "spec-author on claude-sonnet-5", Forecast: 4800, Forecasts: true},
		{Name: "Review the specification", Who: "spec-reviewer on claude-sonnet-5"},
		{Name: "Write the dev-plan", Who: "", Note: "Nobody is assigned to write the dev-plan, so it won't be written until you write it or assign write-dev-plan in config.yaml."},
		{Name: "Review the dev-plan", Who: "dev-plan-reviewer on claude-sonnet-5", Done: true, DoneNote: "already approved"},
	}
	scr := sendScreen{
		Title: "Send features of Platform to development", OwnerKind: "initiative", OwnerID: uuid.New(),
		BackURL: "/ui/i/pf", Workers: 4, Running: 1, Free: 3, AnySendable: true, AgentReview: true,
		Features: []sendFeature{
			{ID: uuid.New(), Name: "Alpha", URL: "/ui/f/pf/alpha", CanSend: true, Steps: steps,
				Reviewer: "The spec reviewer (spec-reviewer on claude-sonnet-5) approves the specification."},
			{ID: uuid.New(), Name: "Stub", URL: "/ui/f/pf/stub", CanSend: false,
				Reason: "This feature has no description yet. Describe what it should do before sending it, because the spec is written from that description."},
		},
	}
	send := render("page-send", pageData{Active: "browse", Actor: "op", Data: scr})
	for _, want := range []string{
		`action="/ui/send"`, `name="feature" value="`, "disabled", "no description yet",
		"3 of 4 agent slots are free", "No forecast yet", "Who reviews the specification", "about 5k",
		"no forecast yet", "Nobody is assigned", "already approved", `name="hold" value="1"`,
	} {
		mustContain(t, "send screen", send, want)
	}
	forced := scr
	forced.AgentReview, forced.HoldForced, forced.HoldDefault = false, true, true
	mustContain(t, "forced hold", render("page-send", pageData{Data: forced}), "No setting lets a specification through unreviewed")

	// The feature page's send card in its three states.
	base := entityPage{Kind: "feature", RefType: "feature", ID: uuid.New(), Title: "Alpha", State: "idea"}
	sendable := base
	sendable.Send = sendCard{Show: true, CanSend: true}
	mustContain(t, "send button", render("page-entity", pageData{Data: &sendable}), `href="/ui/send?feature=`)
	blocked := base
	blocked.Send = sendCard{Show: true, Reason: "Neither this feature's design nor its initiative's design is approved yet."}
	mustContain(t, "blocked send", render("page-entity", pageData{Data: &blocked}), "aria-describedby=\"why-send\"")
	sent := base
	sent.Send = sendCard{Show: true, Sent: true, SentBy: "sam", SentAt: time.Now().Add(-time.Minute), Hold: true, CanWithdraw: true}
	sentPage := render("page-entity", pageData{Data: &sent})
	mustContain(t, "sent mark", sentPage, "By sam")
	mustContain(t, "withdraw", sentPage, `action="/ui/send/withdraw"`)
	mustContain(t, "hold shown", sentPage, "held for a person")
	ready := base
	ready.State, ready.CanStart = "ready", true
	mustContain(t, "gate 2 renamed", render("page-entity", pageData{Data: &ready}), "Start building")
	init := entityPage{Kind: "initiative", RefType: "initiative", ID: uuid.New(), Title: "Platform", CanSendAny: true}
	mustContain(t, "initiative send", render("page-entity", pageData{Data: &init}), `href="/ui/send?initiative=`)

	// The document page.
	doc := func(state lifecycle.DocumentState, a docActions) string {
		p := entityDocPage{docPageData: &docPageData{
			Document: store.Document{ID: uuid.New(), Type: "spec", State: state, Path: "docs/pf/alpha/spec.md", Title: "Alpha spec"},
			Comments: []store.Comment{{Author: "sam", Body: "The spec forgets timezones.", IsIssue: true, Via: "mcp", Quote: "it forgets timezones"},
				{Author: "spec-reviewer", Body: "Answered.", IsIssue: true, Resolved: true, AddressedBy: "spec-reviewer", AddressedNote: "Section 2 now names UTC."}},
			Actions: a,
		}}
		return render("page-entity-document", pageData{Data: p})
	}
	held := doc(lifecycle.DocReviewing, docActions{AgentReviewed: true, Held: true, HeldByAgent: true, CanRelease: true,
		CanApprove: true, CanSendBack: true, CanRaiseIssue: true, CanRequestReview: true, OpenIssueCount: 1})
	for _, want := range []string{"waiting for you", "Let the reviewer decide", `action="/ui/document/approve"`,
		`action="/ui/document/send-back"`, `action="/ui/document/issue"`, `action="/ui/document/request-review"`,
		"open issue", "relayed from chat", "it forgets timezones", "Section 2 now names UTC", "1 issue raised by a person is still open"} {
		mustContain(t, "held spec page", held, want)
	}
	off := doc(lifecycle.DocReviewing, docActions{AgentReviewed: true, Held: true, AgentOff: true, CanApprove: true, CanSendBack: true})
	mustContain(t, "agent review off", off, "Agent review is switched off")
	mustNotContain(t, "no release with agent review off", off, "Let the reviewer decide</button>")
	draft := doc(lifecycle.DocDraft, docActions{CanSubmit: true, CanDetach: true, CanRaiseIssue: true, OpenIssueCount: 2})
	for _, want := range []string{`action="/ui/document/submit"`, `action="/ui/document/detach"`, `name="confirm"`, "2 open issues"} {
		mustContain(t, "draft page", draft, want)
	}
	approved := doc(lifecycle.DocApproved, docActions{CanRevise: true})
	mustContain(t, "revise", approved, `action="/ui/document/revise"`)
	pending := doc(lifecycle.DocReviewing, docActions{AgentReviewed: true, Pending: "A question about this document is waiting in the Inbox."})
	mustContain(t, "pending question", pending, "Open the Inbox")
	mustNotContain(t, "no acts while pending", pending, `action="/ui/document/approve"`)

	for name, body := range map[string]string{"send": send, "held": held, "draft": draft, "sent": sentPage} {
		for _, banned := range typedPathFieldNames {
			if strings.Contains(body, banned) {
				t.Errorf("%s renders a typed entity-path field %s (NFR-9)", name, banned)
			}
		}
	}
}
