package lifecycle

// Validation: mechanical, synchronous, no judgement (DESIGN-003 §3). Runs at
// submit (failure blocks the transition) and on demand via
// `subutai validate`. The manifest configures checks 2 and 5 only; checks
// 1, 3, and 4 are the non-configurable floor (DESIGN-004 F-6).

import (
	"fmt"
	"strings"

	"subutai/internal/config"
	"subutai/internal/content"
)

// Issue is one validation finding. Check identifies the failing check
// ("front_matter", "sections", "links", "placeholders", or "rule:<kind>").
type Issue struct {
	Check  string `json:"check"`
	Detail string `json:"detail"`
}

// Report is the structured result printed by the CLI and recorded on the
// document's audit trail; reviewers receive the latest report (DESIGN-003 §3).
type Report struct {
	Valid  bool    `json:"valid"`
	Issues []Issue `json:"issues"`
}

func (r *Report) add(check, format string, args ...any) {
	r.Valid = false
	r.Issues = append(r.Issues, Issue{Check: check, Detail: fmt.Sprintf(format, args...)})
}

// LinkChecker reports whether a link target (as written in the document)
// resolves. The server wires repo-relative file existence; tests inject.
type LinkChecker func(target string) bool

// RuleKinds is the registry of manifest rule kinds (DESIGN-004 §7, F-6).
// Phase 1 shipped min_list_items; phase 2 adds table_parses; SPEC-018 adds
// max_words and one_line, which keep surfaced text short by construction;
// SPEC-019 adds contains_text, which a bug report's built-in criterion needs.
// New kinds are added here and listed in DESIGN-004.
func RuleKinds() map[string]bool {
	return map[string]bool{"min_list_items": true, "table_parses": true, "max_words": true, "one_line": true, "contains_text": true}
}

// Validate runs the full check suite for a document of the manifest's type.
// raw is the file content; resolves may be nil (skips link checking — used
// only in unit tests, never by the server).
func Validate(m *config.Manifest, raw string, resolves LinkChecker) Report {
	report := Report{Valid: true}

	doc, err := content.Parse(raw)
	if err != nil {
		report.add("front_matter", "%v", err)
		return report // nothing else is checkable without a parse
	}

	// Check 1: front matter well-formed, required fields, type agreement.
	for _, field := range m.FrontMatter.Required {
		if doc.FrontMatterString(field) == "" {
			report.add("front_matter", "required field %q is missing or empty", field)
		}
	}
	if declared := doc.FrontMatterString("type"); declared != "" && declared != m.Type {
		report.add("front_matter", "type is %q but this template is %q", declared, m.Type)
	}

	// Check 2: required sections, ordered when the manifest says strict.
	lastPos := -1
	for _, ref := range m.Sections.Required {
		sec := doc.SectionByHeading(ref.Heading)
		if sec == nil {
			report.add("sections", "required section %q is missing", ref.Heading)
			continue
		}
		if ref.Level != 0 && sec.Level != ref.Level {
			report.add("sections", "section %q is level %d, manifest requires level %d", ref.Heading, sec.Level, ref.Level)
		}
		if m.Sections.Order == "strict" {
			if sec.Position < lastPos {
				report.add("sections", "section %q is out of order", ref.Heading)
			}
			lastPos = sec.Position
		}
	}

	// Check 3: internal links resolve.
	if resolves != nil {
		for _, target := range doc.Links() {
			if !resolves(target) {
				report.add("links", "link target %q does not resolve", target)
			}
		}
	}

	// Check 4: no unresolved template placeholders — front matter included
	// (the template ships placeholder titles and owners).
	for _, field := range m.FrontMatter.Required {
		if v := doc.FrontMatterString(field); strings.Contains(v, "{{") {
			report.add("placeholders", "front-matter field %q contains an unresolved {{...}} placeholder", field)
		}
	}
	for _, sec := range doc.Sections {
		// Fenced code is quoted text — a log line, a stack trace, a template
		// — not the writer's own placeholder, so it is left out of this check
		// (SPEC-019 R19-3). The parser already treats it as text.
		prose := outsideFences(sec.Content)
		if i := strings.Index(prose, "{{"); i >= 0 && strings.Contains(prose[i:], "}}") {
			report.add("placeholders", "section %q contains an unresolved {{...}} placeholder", sec.Heading)
		}
		if containsWord(prose, "TODO") {
			report.add("placeholders", "section %q contains TODO", sec.Heading)
		}
	}

	// Check 5: type-specific rules from the registered set.
	for _, rule := range m.Rules {
		switch rule.Kind {
		case "min_list_items":
			sec := doc.SectionByHeading(rule.Section)
			if sec == nil {
				// Reported by check 2 if the section is also required;
				// a rule on a truly absent section still fails.
				report.add("rule:min_list_items", "section %q is missing", rule.Section)
				continue
			}
			if n := countListItems(sec.Content); n < rule.Min {
				report.add("rule:min_list_items",
					"section %q has %d list item(s), needs at least %d", rule.Section, n, rule.Min)
			}
		case "table_parses":
			// The dev-plan's task table must be present, well-formed, and
			// acyclic before review (DESIGN-005 §2, FR-1.2).
			sec := doc.SectionByHeading(rule.Section)
			if sec == nil {
				report.add("rule:table_parses", "section %q is missing", rule.Section)
				continue
			}
			if _, err := ParseTaskTable(sec.Content); err != nil {
				report.add("rule:table_parses", "%v", err)
			}
		case "max_words":
			checkMaxWords(&report, rule, doc)
		case "one_line":
			if v := doc.FrontMatterString(rule.Field); strings.ContainsAny(strings.TrimSpace(v), "\r\n") {
				report.add("rule:one_line", "The %s must be on one line, because it is pushed into agents' prompts as one; it has a line break.", fieldWords(rule.Field))
			}
		case "contains_text":
			// A section must say something in so many words: a bug report's
			// Acceptance criteria must keep "The defect no longer reproduces".
			// Case and spacing are forgiven, not wording.
			sec := doc.SectionByHeading(rule.Section)
			if sec == nil {
				report.add("rule:contains_text", "section %q is missing", rule.Section)
				continue
			}
			// The text must open a list item: "The defect no longer reproduces"
			// is a criterion of its own, not a phrase inside another
			// (SPEC-019 SD-5, R19-14).
			if !listItemStartsWith(sec.Content, rule.Text) {
				report.add("rule:contains_text", "section %q must have a list item that starts %q", rule.Section, rule.Text)
			}
		default:
			// Unknown kinds are caught at config load (DESIGN-004 §9);
			// reaching here means the caller skipped that gate.
			report.add("rule:"+rule.Kind, "unknown rule kind")
		}
	}

	return report
}

func countListItems(body string) int {
	n := 0
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || startsNumbered(t) {
			n++
		}
	}
	return n
}

func startsNumbered(t string) bool {
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(t) && t[i] == '.' && t[i+1] == ' '
}

func containsWord(body, word string) bool {
	for i := 0; ; {
		j := strings.Index(body[i:], word)
		if j < 0 {
			return false
		}
		j += i
		before := j == 0 || !isWordChar(body[j-1])
		afterIdx := j + len(word)
		after := afterIdx >= len(body) || !isWordChar(body[afterIdx])
		if before && after {
			return true
		}
		i = afterIdx
	}
}

func isWordChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}

// checkMaxWords is the max_words rule (SPEC-018 FR-1.2): a front-matter
// field, a section, or the whole body is at most rule.Max words.
func checkMaxWords(report *Report, rule config.Rule, doc *content.Doc) {
	switch {
	case rule.Field != "":
		if n := CountWords(doc.FrontMatterString(rule.Field)); n > rule.Max {
			report.add("rule:max_words", "The %s is %d words; it can be at most %d, because it is pushed into agents' prompts. %s",
				fieldWords(rule.Field), n, rule.Max, shortenHint(rule.Field))
		}
	case rule.Section != "":
		sec := doc.SectionByHeading(rule.Section)
		if sec == nil {
			return // a missing required section is reported by check 2
		}
		if n := CountWords(sec.Content); n > rule.Max {
			report.add("rule:max_words", "Section %q is %d words; it can be at most %d.", rule.Section, n, rule.Max)
		}
	case rule.Body:
		if n := CountWords(SurfacedBody(doc.Body)); n > rule.Max {
			report.add("rule:max_words", "This document is %d words below its title; it can be at most %d, because all of it is pushed into every agent's prompt. Keep the five to fifteen points that change what an agent writes.", n, rule.Max)
		}
	}
}

func fieldWords(field string) string {
	switch field {
	case "ruling":
		return "ruling"
	case "reason":
		return "reason"
	}
	return "front-matter field " + field
}

func shortenHint(field string) string {
	switch field {
	case "ruling":
		return "Keep the rule itself here, and put the argument in Context."
	case "reason":
		return "Give the reason in one line; the full argument belongs in Context."
	}
	return ""
}

// CountWords counts runs of non-space characters.
func CountWords(s string) int { return len(strings.Fields(s)) }

// SurfacedBody is a document body as it is pushed into prompts: without a
// leading level-1 heading, and trimmed (SPEC-018 FR-5, FR-6.3).
func SurfacedBody(body string) string {
	b := strings.TrimLeft(body, "\r\n\t ")
	if strings.HasPrefix(b, "# ") {
		if i := strings.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		} else {
			b = ""
		}
	}
	return strings.TrimSpace(b)
}

// normaliseSpace lower-cases s and collapses runs of white space, so a
// contains_text rule doesn't fail on a line break or a capital.
func normaliseSpace(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// outsideFences returns the lines of s that aren't inside a fenced code
// block.
func outsideFences(s string) string {
	var b strings.Builder
	in := false
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			in = !in
			continue
		}
		if !in {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// listItemStartsWith reports whether a list item in s, outside fenced code,
// begins with text, ignoring case and spacing. A list item runs on across
// indented continuation lines.
func listItemStartsWith(s, text string) bool {
	want := normaliseSpace(text)
	lines := strings.Split(outsideFences(s), "\n")
	for i, line := range lines {
		item, ok := listItemText(line)
		if !ok {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			l := lines[j]
			if strings.TrimSpace(l) == "" || !(strings.HasPrefix(l, " ") || strings.HasPrefix(l, "\t")) {
				break
			}
			if _, next := listItemText(strings.TrimSpace(l)); next {
				break
			}
			item += " " + l
		}
		if strings.HasPrefix(normaliseSpace(item), want) {
			return true
		}
	}
	return false
}

// listItemText returns a list line's text after its marker.
func listItemText(line string) (string, bool) {
	t := strings.TrimSpace(line)
	for _, m := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(t, m) {
			return t[len(m):], true
		}
	}
	i := 0
	for i < len(t) && t[i] >= '0' && t[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(t) && (t[i] == '.' || t[i] == ')') && t[i+1] == ' ' {
		return t[i+2:], true
	}
	return "", false
}
