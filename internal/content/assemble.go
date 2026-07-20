package content

import (
	"fmt"
	"strings"
	"time"
)

// Prompt assembly (DESIGN-002 §4 step 3). Deterministic order — role
// identity → skill procedure → project context → auto-surfaced documents
// (with provenance) → entity context → task instruction — so stable prefixes
// maximise provider prompt caching. Identity and skill form the system
// prompt; the rest is one user message.

type AttachedDoc struct {
	Path       string
	Title      string
	Type       string
	ApprovedAt *time.Time
	Body       string
}

type FeatureContext struct {
	Name           string
	Slug           string
	Description    string
	InitiativePath string // e.g. "auth / basic"
}

type CommentContext struct {
	Author     string
	SectionRef string
	Body       string
	CreatedAt  time.Time
}

type ReviewPromptInput struct {
	RoleIdentity     string
	SkillBody        string
	ProjectName      string
	AncestorDocs     []AttachedDoc // approved docs on ancestor initiatives
	Feature          *FeatureContext
	DocPath          string
	DocBody          string
	ValidationReport string // rendered report text
	Comments         []CommentContext
}

// AssembleReviewPrompt renders the system and user strings for a reviewer
// dispatch (FR-5.3).
func AssembleReviewPrompt(in ReviewPromptInput) (system, user string) {
	var sys strings.Builder
	sys.WriteString(strings.TrimSpace(in.RoleIdentity))
	if in.SkillBody != "" {
		sys.WriteString("\n\n# Procedure\n\n")
		sys.WriteString(strings.TrimSpace(in.SkillBody))
	}

	var u strings.Builder
	u.WriteString("# Project\n\n")
	u.WriteString(in.ProjectName + "\n")

	if len(in.AncestorDocs) > 0 {
		u.WriteString("\n# Background documents\n")
		for _, d := range in.AncestorDocs {
			// Provenance line per vision §10.
			when := ""
			if d.ApprovedAt != nil {
				when = ", approved " + d.ApprovedAt.Format("2006-01-02")
			}
			u.WriteString(fmt.Sprintf("\n## %s (%s from %s%s)\n\n%s\n", d.Title, d.Type, d.Path, when, strings.TrimSpace(d.Body)))
		}
	}

	if in.Feature != nil {
		u.WriteString("\n# Feature under review\n\n")
		u.WriteString(fmt.Sprintf("%s (%s), under initiative %s\n", in.Feature.Name, in.Feature.Slug, in.Feature.InitiativePath))
		if in.Feature.Description != "" {
			u.WriteString("\n" + strings.TrimSpace(in.Feature.Description) + "\n")
		}
	}

	u.WriteString("\n# Task\n\n")
	u.WriteString("Review the document below. Complete your review by calling the `submit_review` tool with your verdict — approve, request_changes (with comments), or escalate (with reasoning). Do not finish without calling it.\n")

	u.WriteString("\n## Validation report\n\n")
	if strings.TrimSpace(in.ValidationReport) == "" {
		u.WriteString("All mechanical checks passed.\n")
	} else {
		u.WriteString(strings.TrimSpace(in.ValidationReport) + "\n")
	}

	if len(in.Comments) > 0 {
		u.WriteString("\n## Unresolved comments from prior review rounds\n\n")
		for _, c := range in.Comments {
			ref := ""
			if c.SectionRef != "" {
				ref = " (on " + c.SectionRef + ")"
			}
			u.WriteString(fmt.Sprintf("- %s%s: %s\n", c.Author, ref, c.Body))
		}
		u.WriteString("\nVerify these were addressed rather than re-reviewing cold.\n")
	}

	u.WriteString(fmt.Sprintf("\n## Document under review (%s)\n\n%s\n", in.DocPath, strings.TrimSpace(in.DocBody)))

	return sys.String(), u.String()
}

// RenderValidationIssues renders a report's issues as prompt/CLI text.
func RenderValidationIssues(issues []struct{ Check, Detail string }) string {
	var b strings.Builder
	for _, is := range issues {
		fmt.Fprintf(&b, "- [%s] %s\n", is.Check, is.Detail)
	}
	return b.String()
}
