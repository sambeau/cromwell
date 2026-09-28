package rules

import (
	"fmt"

	"github.com/google/uuid"
)

// Send to development (SPEC-011): the actions and outcome parts the review
// loop, the hold and human issues add. The decisions that use them are in
// rules.go, beside the paths they extend.

// ReviseAuthoredDocument sends a spec or plan that came back to draft to its
// author agent again (FR-3.1). The server checks that the feature is sent and
// applies the round cap (FR-3.3); Force is a person's "allow another round".
type ReviseAuthoredDocument struct {
	DocID     uuid.UUID
	FeatureID uuid.UUID
	DocType   string
	Force     bool
}

func (ReviseAuthoredDocument) ActionKind() string { return "revise_authored_document" }

// HoldDocument records an agent approval of a held spec without applying it
// (FR-5.4): the spec stays in reviewing and waits for a person.
type HoldDocument struct {
	DocID      uuid.UUID
	Actor      string
	DispatchID *uuid.UUID
	Reasoning  string
	Comments   []ReviewComment
}

func (HoldDocument) ActionKind() string { return "hold_document" }

// AnswerIssues stores a reviewer's answers to human issues (SD-6). Only
// addressed and does_not_apply answers are passed: an issue not addressed
// stays open.
type AnswerIssues struct {
	DocID      uuid.UUID
	Actor      string
	DispatchID *uuid.UUID
	Answers    []IssueAnswer
}

func (AnswerIssues) ActionKind() string { return "answer_issues" }

// EstimateFeature queues the chain's closing estimate (FR-7.1). The server
// decides whether the feature is sent and not yet estimated since.
type EstimateFeature struct {
	FeatureID uuid.UUID
}

func (EstimateFeature) ActionKind() string { return "estimate_feature" }

// The three answers a reviewer may give a human issue.
const (
	IssueAddressed    = "addressed"
	IssueDoesNotApply = "does_not_apply"
	IssueNotAddressed = "not_addressed"
)

// IssueAnswer is one entry in submit_review's issues list.
type IssueAnswer struct {
	IssueID string `json:"issue_id"`
	Status  string `json:"status"`
	Note    string `json:"note,omitempty"`
}

// AnsweredIssues returns the answers that resolve an issue.
func (o *ReviewOutcome) AnsweredIssues() []IssueAnswer {
	var out []IssueAnswer
	for _, a := range o.Issues {
		if a.Status == IssueAddressed || a.Status == IssueDoesNotApply {
			out = append(out, a)
		}
	}
	return out
}

// CheckIssueCoverage enforces the must-address rule (SD-6): an approval must
// answer every open issue with addressed or does_not_apply. A send-back need
// not, because the issues it leaves open go back to the author with it.
func CheckIssueCoverage(o *ReviewOutcome, open []uuid.UUID) error {
	if o.Verdict != "approve" || len(open) == 0 {
		return nil
	}
	answered := map[string]bool{}
	for _, a := range o.AnsweredIssues() {
		if id, err := uuid.Parse(a.IssueID); err == nil {
			answered[id.String()] = true
		}
	}
	for _, id := range open {
		if !answered[id.String()] {
			return fmt.Errorf("review outcome: cannot approve until human issue %s is answered — "+
				"add it to issues as addressed (saying how) or does_not_apply (saying why), "+
				"or request changes if it still stands", id)
		}
	}
	return nil
}
