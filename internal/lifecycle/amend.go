package lifecycle

// The append-only check for a decision's new revision (SPEC-018 FR-3.3,
// SD-5). An accepted decision is never edited (DESIGN-010 §11): a revision of
// one keeps the accepted text exactly and adds a dated amendment after it, or,
// for a decision with no recorded ruling, records one and changes nothing
// else. Pure, so the whole matrix is table-testable.

import (
	"reflect"
	"regexp"
	"strings"

	"subutai/internal/config"
	"subutai/internal/content"
)

// amendmentHeading is the dated heading an amendment opens with:
// "## Amendment 2 — reviews and relay (2026-09-30)".
var amendmentHeading = regexp.MustCompile(`^## Amendment\b.*\(\d{4}-\d{2}-\d{2}\)\s*$`)

// AmendmentHeadings lists the amendment headings a decision already has.
func AmendmentHeadings(body string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(line, "## Amendment") {
			out = append(out, strings.TrimSpace(strings.TrimPrefix(line, "## ")))
		}
	}
	return out
}

// frontMatterMayChange are the keys a decision's revision may change.
var frontMatterMayChange = map[string]bool{"ruling": true, "reason": true, "id": true, "revision": true}

// ValidateDecisionRevision checks a decision's revision against the accepted
// text it revises. prevLabel names that text for people ("DEC-006 revision
// 1"). resolves checks links in the appended part only: the accepted text was
// checked when it was accepted, and an adopted decision's old links may no
// longer resolve.
func ValidateDecisionRevision(m *config.Manifest, prevLabel, prevRaw, nextRaw string, resolves LinkChecker) Report {
	report := Report{Valid: true}
	next, err := content.Parse(nextRaw)
	if err != nil {
		report.add("front_matter", "%v", err)
		return report
	}
	prev, err := content.Parse(prevRaw)
	if err != nil {
		report.add("front_matter", "The accepted text of %s can't be read: %v", prevLabel, err)
		return report
	}
	if t := next.FrontMatterString("type"); t != "" && t != "decision" {
		report.add("front_matter", "type is %q but this is a decision", t)
	}

	// 1. The accepted text is kept.
	prevBody := strings.TrimRight(prev.Body, " \t\n")
	if !strings.HasPrefix(next.Body, prevBody) {
		report.add("amendment", "An accepted decision is never edited: the text of %s must be kept exactly, and an amendment added after it. The first difference is on line %d.",
			prevLabel, firstDifferentLine(prev.Body, next.Body)+bodyLineOffset(nextRaw))
		return report
	}
	appended := strings.TrimSpace(next.Body[len(prevBody):])

	// 3. Front matter changes only in the surfaced fields and identity.
	keys := map[string]bool{}
	for k := range prev.FrontMatter {
		keys[k] = true
	}
	for k := range next.FrontMatter {
		keys[k] = true
	}
	for k := range keys {
		if frontMatterMayChange[k] {
			continue
		}
		if !reflect.DeepEqual(prev.FrontMatter[k], next.FrontMatter[k]) {
			report.add("amendment", "An amendment can't change the decision's %s. Only its ruling and reason may be restated; to change more, supersede it with a new decision.", k)
		}
	}

	// 2. Only a dated amendment follows, or a ruling recorded for the first
	// time.
	prevRuling := strings.TrimSpace(prev.FrontMatterString("ruling"))
	nextRuling := strings.TrimSpace(next.FrontMatterString("ruling"))
	if appended == "" {
		if prevRuling != "" || nextRuling == "" {
			report.add("amendment", "Nothing was appended. An amendment adds a dated section after the accepted text, headed like \"## Amendment 1 — what changed (2026-09-30)\".")
		}
	} else {
		first := strings.SplitN(appended, "\n", 2)[0]
		if !amendmentHeading.MatchString(first) {
			report.add("amendment", "The text added after %s must start with a dated amendment heading, like \"## Amendment 1 — what changed (2026-09-30)\"; it starts %q.", prevLabel, first)
		}
		if i := strings.Index(appended, "{{"); i >= 0 && strings.Contains(appended[i:], "}}") {
			report.add("placeholders", "The amendment contains an unresolved {{...}} placeholder.")
		}
		if containsWord(appended, "TODO") {
			report.add("placeholders", "The amendment contains TODO.")
		}
		if resolves != nil {
			for _, target := range (&content.Doc{Raw: appended}).Links() {
				if !resolves(target) {
					report.add("links", "link target %q does not resolve", target)
				}
			}
		}
	}

	// 4. The surfaced fields' caps and placeholders.
	for _, field := range []string{"ruling", "reason"} {
		if v := next.FrontMatterString(field); strings.Contains(v, "{{") {
			report.add("placeholders", "front-matter field %q contains an unresolved {{...}} placeholder", field)
		}
	}
	if nextRuling != "" && strings.TrimSpace(next.FrontMatterString("reason")) == "" {
		report.add("front_matter", "A decision with a ruling needs a reason too: one line saying why.")
	}
	CheckSurfacedCaps(&report, m, next)
	return report
}

// CheckSurfacedCaps applies a manifest's field caps (max_words and one_line on
// front-matter fields, and max_words on the body) and nothing else. It is what
// the routes to accepted that skip Submit check (FR-4.3).
func CheckSurfacedCaps(report *Report, m *config.Manifest, doc *content.Doc) {
	for _, rule := range m.Rules {
		switch {
		case rule.Kind == "max_words" && (rule.Field != "" || rule.Body):
			checkMaxWords(report, rule, doc)
		case rule.Kind == "one_line" && rule.Field != "":
			if v := doc.FrontMatterString(rule.Field); strings.ContainsAny(strings.TrimSpace(v), "\r\n") {
				report.add("rule:one_line", "The %s must be on one line, because it is pushed into agents' prompts as one; it has a line break.", fieldWords(rule.Field))
			}
		}
	}
}

// Fail adds an issue to a report, for checks made outside this package.
func (r *Report) Fail(check, format string, args ...any) { r.add(check, format, args...) }

func firstDifferentLine(a, b string) int {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) && i < len(bl); i++ {
		if al[i] != bl[i] {
			return i + 1
		}
	}
	if len(al) < len(bl) {
		return len(al) + 1
	}
	return len(bl) + 1
}

// bodyLineOffset is how many lines precede the body: the front matter block.
func bodyLineOffset(raw string) int {
	norm := strings.ReplaceAll(strings.TrimPrefix(raw, "\uFEFF"), "\r\n", "\n")
	if !strings.HasPrefix(norm, "---\n") {
		return 0
	}
	rest := norm[4:]
	n := 1
	for _, line := range strings.Split(rest, "\n") {
		n++
		if line == "---" {
			return n - 1
		}
	}
	return 0
}

// String renders a report's issues as one sentence each, for notices.
func (r Report) String() string {
	var parts []string
	for _, is := range r.Issues {
		parts = append(parts, is.Detail)
	}
	return strings.Join(parts, " ")
}
