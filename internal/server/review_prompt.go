package server

// Prompt and outcome-tool parts of the review loop (SPEC-011 FR-3.2, FR-6.3).
// The review outcome tool gains its issues field here rather than in
// internal/dispatch, which the tool definition otherwise lives in, so the
// dispatch package is untouched (NFR-7).

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"subutai/internal/dispatch"
	"subutai/internal/lifecycle"
	"subutai/internal/provider"
	"subutai/internal/rules"
	"subutai/internal/store"
)

// revisionContext writes the current draft and what must change into an
// author's prompt when the feature's current document of the type is a draft
// that has something to answer: review findings or human issues. It reports
// whether the dispatch is a revision.
func (s *Server) revisionContext(ctx context.Context, b *strings.Builder, featureID uuid.UUID, docType string) (bool, error) {
	cur, err := store.CurrentDocForOwner(ctx, s.Store.Pool, docType, "feature", featureID)
	if err == store.ErrNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if cur.State != lifecycle.DocDraft {
		return false, nil
	}
	comments, err := store.CommentsForDocument(ctx, s.Store.Pool, cur.ID, true)
	if err != nil {
		return false, err
	}
	var findings, issues []store.Comment
	for _, c := range comments {
		switch {
		case c.IsIssue:
			issues = append(issues, c)
		case c.Severity == rules.SeverityMinor:
			// Minor findings don't send work back (C-1); leave them out so
			// the author doesn't chase them.
		default:
			findings = append(findings, c)
		}
	}
	if len(findings) == 0 && len(issues) == 0 {
		// Nothing to answer: a fresh draft, or the cascade's successor, is
		// written from the design as before.
		return false, nil
	}
	body, err := s.readDocFile(cur.Path)
	if err != nil {
		return false, nil
	}
	fmt.Fprintf(b, "\n# The current draft (%s), which you are revising\n\n%s\n", cur.Path, strings.TrimSpace(string(body)))
	if len(issues) > 0 {
		b.WriteString("\n# Issues a person raised, which must be dealt with\n\n")
		b.WriteString("The reviewer can't approve until each of these is addressed, or it explains why one doesn't apply. Deal with every one.\n\n")
		for _, c := range issues {
			writeIssueLine(b, c)
		}
	}
	if len(findings) > 0 {
		b.WriteString("\n# Findings from review\n\n")
		b.WriteString("Some may come from earlier rounds and already be dealt with; check each against the draft.\n\n")
		for _, c := range findings {
			ref := ""
			if c.SectionRef != "" {
				ref = " (on " + c.SectionRef + ")"
			}
			fmt.Fprintf(b, "- %s%s: %s\n", c.Author, ref, c.Body)
		}
	}
	return true, nil
}

func writeIssueLine(b *strings.Builder, c store.Comment) {
	ref := ""
	if c.SectionRef != "" {
		ref = " (on " + c.SectionRef + ")"
	}
	fmt.Fprintf(b, "- Issue %s, raised by %s%s: %s\n", c.ID, c.Author, ref, c.Body)
	if c.Quote != "" {
		fmt.Fprintf(b, "  Their words, relayed from chat: %q\n", c.Quote)
	}
}

// issuesPromptSection lists a document's open issues for its reviewer
// (FR-6.3), with the instruction for answering them.
func issuesPromptSection(issues []store.Comment) string {
	if len(issues) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n## Issues a person raised, which you must answer\n\n")
	b.WriteString("A person raised these. Each is must-address. For each one, add an entry to `issues` in submit_review: " +
		"`addressed` with a note saying how the document now deals with it, `does_not_apply` with a note saying why, " +
		"or `not_addressed` if it still stands. You can't approve while any is not addressed.\n\n")
	for _, c := range issues {
		writeIssueLine(&b, c)
	}
	return b.String()
}

// reviewOutcomeTool is the review outcome tool with the issues field added.
func reviewOutcomeTool() provider.ToolDef {
	t := dispatch.ReviewOutcomeTool()
	schema := map[string]any{}
	for k, v := range t.InputSchema {
		schema[k] = v
	}
	props := map[string]any{}
	if p, ok := schema["properties"].(map[string]any); ok {
		for k, v := range p {
			props[k] = v
		}
	}
	props["issues"] = map[string]any{
		"type":        "array",
		"description": "Your answer to each issue a person raised on this document. Required for every open issue when you approve.",
		"items": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"issue_id": map[string]any{"type": "string", "description": "The issue's id, as given in the prompt"},
				"status": map[string]any{
					"type":        "string",
					"enum":        []string{rules.IssueAddressed, rules.IssueDoesNotApply, rules.IssueNotAddressed},
					"description": "addressed: the document now deals with it. does_not_apply: it doesn't bear on this document, and the note says why. not_addressed: it still stands, so you must request changes.",
				},
				"note": map[string]any{"type": "string", "description": "How it was addressed, or why it doesn't apply"},
			},
			"required": []string{"issue_id", "status"},
		},
	}
	schema["properties"] = props
	t.InputSchema = schema
	return t
}

// validateReviewWithIssues checks the outcome and, inside the reviewer's
// turn, that an approval answers every issue open when the review began
// (SD-6), so a reviewer that forgets one can correct itself.
func validateReviewWithIssues(open []uuid.UUID) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		o, err := rules.ParseReviewOutcome(raw)
		if err != nil {
			return err
		}
		return rules.CheckIssueCoverage(o, open)
	}
}
