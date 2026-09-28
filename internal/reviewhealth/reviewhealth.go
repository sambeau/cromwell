// Package reviewhealth measures how each agent reviewer behaves (SPEC-012
// FR-7, DESIGN-010 §8, audit C-7): how often it approves, how much it finds,
// how many rounds its work takes, and how long it takes to decide. A reviewer
// that approves nearly everything in seconds is broken, and the warnings make
// that stand out (SD-9).
//
// Everything here is pure: it takes the verdicts the store read and returns
// the numbers, so the arithmetic is table-testable.
package reviewhealth

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Review is one agent verdict.
type Review struct {
	Purpose  string
	Role     string
	Model    string
	Ref      uuid.UUID // the document or task reviewed, or the feature verified
	Verdict  string    // approve | request_changes | escalate
	Majors   int
	Minors   int
	Duration time.Duration // from the run starting to its verdict
	Tokens   int64         // everything the run used
	Output   int64         // what it wrote: its reasoning, findings and verdict
}

// FromOutcome reads a verdict and its findings from a run's outcome (FR-7.2):
// review comments count by severity, a comment without one counting as major
// as the review loop treats it; a verification's unmet criteria count as
// majors.
func FromOutcome(raw json.RawMessage) (verdict string, majors, minors int) {
	var o struct {
		Verdict  string `json:"verdict"`
		Comments []struct {
			Severity string `json:"severity"`
		} `json:"comments"`
		Criteria []struct {
			Met bool `json:"met"`
		} `json:"criteria"`
	}
	if err := json.Unmarshal(raw, &o); err != nil {
		return "", 0, 0
	}
	for _, c := range o.Comments {
		if c.Severity == "minor" {
			minors++
		} else {
			majors++
		}
	}
	for _, c := range o.Criteria {
		if !c.Met {
			majors++
		}
	}
	return o.Verdict, majors, minors
}

// The warning rules (SD-9). The page states them in words from these values,
// so the words and the rules can't drift apart.
//
// Speed alone isn't a warning: a document review is one model call, so its
// time is the model's latency. A quick verdict is suspicious when the
// reviewer also wrote almost nothing. Findings alone aren't one either for a
// verifier: it can't approve with an unmet criterion, so its approvals carry
// no findings by construction, and that rule applies to reviews only.
const (
	MinVerdicts          = 5
	HighApproval         = 0.90
	LowApproval          = 0.10
	FastVerdict          = 30 * time.Second
	TerseOutput          = 300 // output tokens: a verdict with next to no reasoning
	FewFindingsPerReview = 0.2 // fewer than one finding in five reviews
)

// Warning kinds.
const (
	WarnFastApprover  = "fast-approver"
	WarnFindsNothing  = "finds-nothing"
	WarnNeverApproves = "never-approves"
)

// Reviewer is one role and model's numbers.
type Reviewer struct {
	Role     string
	Model    string
	Purposes []string

	Verdicts  int
	Approved  int
	SentBack  int
	Escalated int

	Majors int
	Minors int

	Reviewed  int // distinct documents, tasks or features
	MaxRounds int

	MedianTime  time.Duration
	SlowestTime time.Duration

	MedianTokens int64
	MedianOutput int64

	// ReviewVerdicts and ReviewFindings count review-* verdicts only, the
	// ones the "finds almost nothing" rule reads.
	ReviewVerdicts int
	ReviewFindings int

	Warnings []string
}

// ApprovalRate is approvals over verdicts, 0 with no verdicts.
func (r Reviewer) ApprovalRate() float64 {
	if r.Verdicts == 0 {
		return 0
	}
	return float64(r.Approved) / float64(r.Verdicts)
}

// ApprovalPct is the approval rate as a whole percentage.
func (r Reviewer) ApprovalPct() int { return int(r.ApprovalRate()*100 + 0.5) }

// MeanRounds is reviews per thing reviewed.
func (r Reviewer) MeanRounds() float64 {
	if r.Reviewed == 0 {
		return 0
	}
	return float64(r.Verdicts) / float64(r.Reviewed)
}

// MajorsPerReview and MinorsPerReview are findings per verdict.
func (r Reviewer) MajorsPerReview() float64 { return per(r.Majors, r.Verdicts) }
func (r Reviewer) MinorsPerReview() float64 { return per(r.Minors, r.Verdicts) }

func per(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) / float64(d)
}

// Flagged reports whether any warning applies.
func (r Reviewer) Flagged() bool { return len(r.Warnings) > 0 }

// Summarise groups verdicts by role and model and computes each group's
// numbers and warnings. Flagged reviewers sort first, then the busiest.
func Summarise(reviews []Review) []Reviewer {
	type key struct{ role, model string }
	groups := map[key][]Review{}
	var order []key
	for _, rv := range reviews {
		k := key{rv.Role, rv.Model}
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], rv)
	}
	out := make([]Reviewer, 0, len(order))
	for _, k := range order {
		out = append(out, summariseOne(k.role, k.model, groups[k]))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Flagged() != out[j].Flagged() {
			return out[i].Flagged()
		}
		if out[i].Verdicts != out[j].Verdicts {
			return out[i].Verdicts > out[j].Verdicts
		}
		return out[i].Role+out[i].Model < out[j].Role+out[j].Model
	})
	return out
}

func summariseOne(role, model string, rs []Review) Reviewer {
	r := Reviewer{Role: role, Model: model}
	seenPurpose := map[string]bool{}
	rounds := map[uuid.UUID]int{}
	var times []time.Duration
	var tokens, outputs []int64
	for _, rv := range rs {
		if strings.HasPrefix(rv.Purpose, "review-") {
			r.ReviewVerdicts++
			r.ReviewFindings += rv.Majors + rv.Minors
		}
		if !seenPurpose[rv.Purpose] {
			seenPurpose[rv.Purpose] = true
			r.Purposes = append(r.Purposes, rv.Purpose)
		}
		r.Verdicts++
		switch rv.Verdict {
		case "approve":
			r.Approved++
		case "request_changes":
			r.SentBack++
		case "escalate":
			r.Escalated++
		}
		r.Majors += rv.Majors
		r.Minors += rv.Minors
		rounds[rv.Ref]++
		times = append(times, rv.Duration)
		tokens = append(tokens, rv.Tokens)
		outputs = append(outputs, rv.Output)
	}
	sort.Strings(r.Purposes)
	r.Reviewed = len(rounds)
	for _, n := range rounds {
		if n > r.MaxRounds {
			r.MaxRounds = n
		}
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	sort.Slice(tokens, func(i, j int) bool { return tokens[i] < tokens[j] })
	sort.Slice(outputs, func(i, j int) bool { return outputs[i] < outputs[j] })
	if n := len(times); n > 0 {
		r.MedianTime = medianDuration(times)
		r.SlowestTime = times[n-1]
		r.MedianTokens = medianInt(tokens)
		r.MedianOutput = medianInt(outputs)
	}
	r.Warnings = warnings(r)
	return r
}

func medianDuration(s []time.Duration) time.Duration {
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

func medianInt(s []int64) int64 {
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}

// warnings applies SD-9's rules.
func warnings(r Reviewer) []string {
	if r.Verdicts < MinVerdicts {
		return nil
	}
	var w []string
	rate := r.ApprovalRate()
	if rate >= HighApproval {
		if r.MedianTime < FastVerdict && r.MedianOutput < TerseOutput {
			w = append(w, WarnFastApprover)
		}
		if r.ReviewVerdicts >= MinVerdicts && per(r.ReviewFindings, r.ReviewVerdicts) < FewFindingsPerReview {
			w = append(w, WarnFindsNothing)
		}
	}
	if rate <= LowApproval {
		w = append(w, WarnNeverApproves)
	}
	return w
}
