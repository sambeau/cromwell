package reviewhealth

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestFromOutcome(t *testing.T) {
	v, maj, min := FromOutcome([]byte(`{"verdict":"request_changes","comments":[{"severity":"major"},{"severity":"minor"},{"body":"no severity"}]}`))
	if v != "request_changes" || maj != 2 || min != 1 {
		t.Errorf("review: %s %d %d", v, maj, min)
	}
	v, maj, min = FromOutcome([]byte(`{"verdict":"request_changes","criteria":[{"met":true},{"met":false},{"met":false}]}`))
	if v != "request_changes" || maj != 2 || min != 0 {
		t.Errorf("verification: %s %d %d", v, maj, min)
	}
	if v, _, _ := FromOutcome([]byte(`not json`)); v != "" {
		t.Error("a malformed outcome has no verdict")
	}
}

// TestSummarise is FR-7.2 and FR-7.3 by hand: counts, rates, findings, rounds,
// medians, and each of the three warnings, including the healthy reviewers
// the warnings must leave alone.
func TestSummarise(t *testing.T) {
	doc1, doc2 := uuid.New(), uuid.New()
	sec := time.Second
	careful := []Review{
		{Role: "spec-reviewer", Model: "m1", Purpose: "review-spec", Ref: doc1, Verdict: "request_changes", Majors: 2, Minors: 1, Duration: 80 * sec, Tokens: 1000, Output: 900},
		{Role: "spec-reviewer", Model: "m1", Purpose: "review-spec", Ref: doc1, Verdict: "approve", Minors: 1, Duration: 40 * sec, Tokens: 900, Output: 700},
		{Role: "spec-reviewer", Model: "m1", Purpose: "review-spec", Ref: doc2, Verdict: "approve", Duration: 60 * sec, Tokens: 1100, Output: 500},
		{Role: "spec-reviewer", Model: "m1", Purpose: "review-spec", Ref: doc2, Verdict: "escalate", Duration: 100 * sec, Tokens: 1200, Output: 600},
	}
	// Approves everything, in seconds, with next to no reasoning: the rubber stamp.
	var stamp []Review
	for i := 0; i < 5; i++ {
		stamp = append(stamp, Review{Role: "code-reviewer", Model: "m2", Purpose: "review-code",
			Ref: uuid.New(), Verdict: "approve", Duration: 3 * sec, Tokens: 200, Output: 40})
	}
	var harsh []Review
	for i := 0; i < 5; i++ {
		harsh = append(harsh, Review{Role: "plan-reviewer", Model: "m3", Purpose: "review-dev_plan",
			Ref: doc1, Verdict: "request_changes", Majors: 1, Duration: 90 * sec, Output: 800})
	}
	// A good verifier: approves 9 of 10, with no findings on its approvals by
	// construction, and takes its time. It must not be flagged.
	var verifier []Review
	for i := 0; i < 10; i++ {
		v := Review{Role: "verifier", Model: "m4", Purpose: "verify-feature", Ref: uuid.New(),
			Verdict: "approve", Duration: 70 * sec, Output: 1200}
		if i == 0 {
			v.Verdict, v.Majors = "request_changes", 1
		}
		verifier = append(verifier, v)
	}
	// A fast model that approves good specs with real reasoning, and raises
	// minors: flagged neither for speed nor for silence.
	var fast []Review
	for i := 0; i < 5; i++ {
		fast = append(fast, Review{Role: "spec-reviewer", Model: "fast", Purpose: "review-spec",
			Ref: uuid.New(), Verdict: "approve", Minors: 1, Duration: 8 * sec, Output: 900})
	}
	few := []Review{{Role: "tester", Model: "m5", Purpose: "review-code", Ref: uuid.New(), Verdict: "approve", Duration: sec}}

	var all []Review
	for _, g := range [][]Review{careful, stamp, harsh, verifier, fast, few} {
		all = append(all, g...)
	}
	got := Summarise(all)
	if len(got) != 6 {
		t.Fatalf("want 6 reviewers, got %d", len(got))
	}
	by := map[string]Reviewer{}
	for _, r := range got {
		by[r.Role+"/"+r.Model] = r
	}

	c := by["spec-reviewer/m1"]
	if c.Verdicts != 4 || c.Approved != 2 || c.SentBack != 1 || c.Escalated != 1 || c.ApprovalPct() != 50 {
		t.Errorf("counts: %+v", c)
	}
	if c.Majors != 2 || c.Minors != 2 || c.MajorsPerReview() != 0.5 || c.MinorsPerReview() != 0.5 {
		t.Errorf("findings: %+v", c)
	}
	if c.Reviewed != 2 || c.MaxRounds != 2 || c.MeanRounds() != 2 {
		t.Errorf("rounds: reviewed %d max %d mean %v", c.Reviewed, c.MaxRounds, c.MeanRounds())
	}
	if c.MedianTime != 70*sec || c.SlowestTime != 100*sec || c.MedianTokens != 1050 || c.MedianOutput != 650 {
		t.Errorf("medians: time %v slowest %v tokens %d output %d", c.MedianTime, c.SlowestTime, c.MedianTokens, c.MedianOutput)
	}
	if c.Flagged() {
		t.Errorf("a careful reviewer is not flagged: %v", c.Warnings)
	}

	s := by["code-reviewer/m2"]
	if len(s.Warnings) != 2 || s.Warnings[0] != WarnFastApprover || s.Warnings[1] != WarnFindsNothing {
		t.Errorf("the rubber stamp should be flagged twice: %v", s.Warnings)
	}
	if h := by["plan-reviewer/m3"]; len(h.Warnings) != 1 || h.Warnings[0] != WarnNeverApproves || h.MaxRounds != 5 {
		t.Errorf("the harsh reviewer: %+v", h)
	}
	if v := by["verifier/m4"]; v.Flagged() || v.ApprovalPct() != 90 {
		t.Errorf("a good verifier must not be flagged: %+v", v)
	}
	if f := by["spec-reviewer/fast"]; f.Flagged() {
		t.Errorf("a fast model that reasons is not flagged: %v", f.Warnings)
	}
	if f := by["tester/m5"]; f.Flagged() {
		t.Error("one verdict is too few to judge")
	}
	if !got[0].Flagged() || !got[1].Flagged() || got[2].Flagged() {
		t.Errorf("flagged reviewers sort first: %v %v %v", got[0].Role, got[1].Role, got[2].Role)
	}
}
