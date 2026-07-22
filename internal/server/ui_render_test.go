package server

import (
	"bytes"
	"testing"
	"time"

	"github.com/google/uuid"

	"cromwell/internal/sizing"
	"cromwell/internal/store"
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
	sum := &DashboardSummary{
		Running:      []store.Dispatch{{ID: uuid.New(), Purpose: "review", RefID: uuid.New()}},
		Queued:       []store.Dispatch{{ID: uuid.New(), Purpose: "implement", QueueReason: strptr("waiting on contract")}},
		Escalated:    2,
		Recent:       []store.AuditEvent{{OccurredAt: now, Actor: "orchestrator", Kind: "dispatch.succeeded", RefType: "task"}},
		CostTotal:    0.42,
		BudgetCap:    5,
		BudgetWarn:   4,
		BudgetPeriod: "monthly",
		OverWarn:     false,
		Calibration: []store.CorpusRow{{
			RefType: "feature", Name: "login", EstimateTokens: 1000, EstimateTier: sizing.TierConsidered, ActualTokens: 1234,
		}},
	}
	inbox := []inboxItem{{
		Checkpoint: store.Checkpoint{ID: uuid.New(), Kind: "dispatch-failure", RefType: "task",
			Question: "The implementer failed. Retry?", CreatedAt: now, Context: []byte(`{"error":"boom"}`)},
		Options: answerOptions("dispatch-failure"),
	}}
	planning := planningView{
		Roots: []treeNode{{
			Type: "initiative", Name: "auth", Rollup: sizing.Rollup{Tokens: 2000, Tier: sizing.TierRough, Estimated: true, Decomposed: true},
			Children: []treeNode{{
				Type: "feature", Name: "login", Rollup: sizing.Rollup{Unestimated: []sizing.Ref{{Type: "task", Name: "t1"}}},
			}},
		}},
		Milestones: []store.MilestoneProgress{{
			Milestone: store.Milestone{Name: "M1"}, Progress: store.Progress{Total: 4, Done: 1},
		}},
		Roadmaps: []roadmapView{{
			Roadmap: store.Roadmap{Name: "R1"},
			Entries: []roadmapEntryView{{Position: 1, Milestone: store.Milestone{Name: "M1"}, Progress: store.Progress{Total: 4, Done: 1}}},
		}},
	}
	docs := []store.Document{{ID: uuid.New(), Type: "spec", State: "approved", OwnerType: "feature", Path: "docs/x.md", Title: "X", CreatedAt: now}}
	docPage := &docPageData{
		Document: docs[0],
		Body:     renderMarkdown("# Title\n\nSome **bold** and `code`.\n"),
		Comments: []store.Comment{{Author: "reviewer", Body: "looks good", SectionRef: "Intro"}},
	}
	cost := costView{
		Total:       1.5,
		Initiatives: []entityCost{{Name: "auth", Cost: 1.0}},
		Features:    []entityCost{{Name: "auth/login", Cost: 0.5}},
		Milestones:  []entityCost{{Name: "M1", Cost: 0.5}},
		Roadmaps:    []entityCost{{Name: "R1", Cost: 0.5}},
		Months:      []store.MonthCost{{Month: "2026-07", CostUSD: 1.5, Count: 3}},
	}

	cases := []struct {
		name      string
		data      any
		mayBeZero bool
	}{
		{"page-dashboard", pageData{Active: "dashboard", Actor: "op", Data: sum}, false},
		{"frag-queue", sum, false},
		{"frag-cost", sum, false},
		{"frag-events", sum, false},
		{"frag-calibration", sum, false},
		{"page-inbox", pageData{Active: "inbox", Actor: "op", Data: inbox}, false},
		{"frag-inbox", inbox, false},
		{"frag-inbox", []inboxItem(nil), false}, // empty inbox renders the "clear" state
		{"frag-inbox-badge", 2, false},
		{"frag-inbox-badge", 0, true}, // a zero badge is deliberately empty
		{"page-planning", pageData{Active: "planning", Actor: "op", Data: planning}, false},
		{"page-documents", pageData{Active: "documents", Actor: "op", Data: docs}, false},
		{"page-document", pageData{Active: "documents", Actor: "op", Data: docPage}, false},
		{"page-cost", pageData{Active: "cost", Actor: "op", Data: cost}, false},
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

func strptr(s string) *string { return &s }
