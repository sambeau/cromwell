package content

import (
	"strings"
	"testing"
	"time"
)

func day(n int) time.Time { return time.Date(2026, 9, n, 12, 0, 0, 0, time.UTC) }

func sampleInput() SurfaceInput {
	return SurfaceInput{
		Conventions: "## Code\n\n- Errors are sentences.\n\n```\n# not a heading\n```",
		Owners: []SurfaceOwner{
			{Decisions: []SurfaceDecision{
				{ID: "DEC-002", Number: 2, Title: "DEC-002: State store — Postgres", FromTitle: true, ApprovedAt: day(1)},
				{ID: "DEC-001", Number: 1, Title: "Go", Ruling: "The server is written in Go.", Reason: "One static binary.", ApprovedAt: day(1)},
			}},
			{Label: "INIT-001 Auth", Decisions: []SurfaceDecision{
				{ID: "DEC-005", Number: 5, Title: "Sessions", Ruling: "Sessions live in Postgres,\n  not in memory.", Reason: "Restarts keep people signed in.", ApprovedAt: day(3)},
			}},
		},
		MaxTokens: 1500,
	}
}

func TestSurfacedBlockBytes(t *testing.T) {
	got := SurfacedBlock(sampleInput())
	want := "# Project decisions and conventions\n\n" +
		"These are binding: people decided them. If the task, or a document you are given, contradicts one, follow the decision and say so in your outcome; don't work around it.\n" +
		"\n## Conventions\n\n### Code\n\n- Errors are sentences.\n\n```\n# not a heading\n```\n" +
		"\n## Decisions\n\n" +
		"- DEC-001: The server is written in Go. Why: One static binary.\n" +
		"- DEC-002: State store — Postgres\n" +
		"- DEC-005 (INIT-001 Auth): Sessions live in Postgres, not in memory. Why: Restarts keep people signed in.\n"
	if got.Block != want {
		t.Fatalf("block:\n%s\nwant:\n%s", got.Block, want)
	}
	if strings.Join(got.Included, ",") != "DEC-001,DEC-002,DEC-005" || len(got.LeftOut) != 0 {
		t.Errorf("included %v, left out %v", got.Included, got.LeftOut)
	}
	if again := SurfacedBlock(sampleInput()); again.Block != got.Block {
		t.Error("the same inputs gave different bytes")
	}
}

func TestSurfacedBlockEmpty(t *testing.T) {
	if r := SurfacedBlock(SurfaceInput{Owners: []SurfaceOwner{{}}, MaxTokens: 1500}); r.Block != "" {
		t.Errorf("nothing accepted should give no block, got %q", r.Block)
	}
}

// The cap admits nearest owner first and, within an owner, the most recently
// accepted first; the first that doesn't fit ends admission (SD-9).
func TestSurfacedBlockTokenCap(t *testing.T) {
	in := sampleInput()
	full := SurfacedBlock(in)
	// Room for everything but one project decision.
	in.MaxTokens = full.Tokens - 5
	r := SurfacedBlock(in)
	if r.Tokens > in.MaxTokens {
		t.Fatalf("block is %d tokens, over the cap of %d", r.Tokens, in.MaxTokens)
	}
	if strings.Join(r.LeftOut, ",") != "DEC-001" {
		t.Fatalf("left out %v: the project's oldest should go first", r.LeftOut)
	}
	if !strings.Contains(r.Block, "- DEC-005 (INIT-001 Auth)") {
		t.Error("the nearest decision must be kept")
	}
	if !strings.Contains(r.Block, "\nLeft out for space: DEC-001 (Go)") {
		t.Errorf("the left-out line should name what was left out:\n%s", r.Block)
	}
}

func TestSurfacedBlockCountCap(t *testing.T) {
	in := sampleInput()
	in.MaxDecisions = 1
	r := SurfacedBlock(in)
	if strings.Join(r.Included, ",") != "DEC-005" {
		t.Fatalf("with a count cap of one, only the nearest should be told: %v", r.Included)
	}
	// Within the project, the tie on acceptance time goes to the higher
	// number, so DEC-002 is before DEC-001 in the left-out order.
	if strings.Join(r.LeftOut, ",") != "DEC-002,DEC-001" {
		t.Errorf("left out %v", r.LeftOut)
	}
}

func TestSurfacedBlockLeftOutLineIsBounded(t *testing.T) {
	var ds []SurfaceDecision
	for i := 1; i <= 25; i++ {
		ds = append(ds, SurfaceDecision{ID: "DEC-" + string(rune('A'+i)), Number: i, Title: "T", Ruling: "R", Reason: "W", ApprovedAt: day(1)})
	}
	r := SurfacedBlock(SurfaceInput{Owners: []SurfaceOwner{{Decisions: ds}}, MaxDecisions: 2, MaxTokens: 1500})
	if !strings.Contains(r.Block, "and 13 more.") {
		t.Errorf("the left-out line should name ten then count the rest:\n%s", r.Block)
	}
}

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens("abcdefg"); got != 2 {
		t.Errorf("7 characters = %d tokens, want 2", got)
	}
	if got := EstimateTokens("——"); got != 1 {
		t.Errorf("code points, not bytes: got %d", got)
	}
}

func TestSurfacedLineDropsTheIDFromATitle(t *testing.T) {
	got := SurfacedLine("", SurfaceDecision{ID: "DEC-001", Title: "DEC-001: Server Language — Go", FromTitle: true})
	if got != "DEC-001: Server Language — Go" {
		t.Errorf("got %q", got)
	}
}
