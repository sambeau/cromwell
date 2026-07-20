package lifecycle

// Validation: mechanical, synchronous, no judgement (DESIGN-003 §3). Runs at
// submit (failure blocks the transition) and on demand via
// `cromwell validate`. The manifest configures checks 2 and 5 only; checks
// 1, 3, and 4 are the non-configurable floor (DESIGN-004 F-6).

import (
	"fmt"
	"strings"

	"cromwell/internal/config"
	"cromwell/internal/content"
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
// Phase 1 ships min_list_items; phase 2 adds table_parses. New kinds are
// added here and listed in DESIGN-004.
func RuleKinds() map[string]bool {
	return map[string]bool{"min_list_items": true}
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
		if i := strings.Index(sec.Content, "{{"); i >= 0 && strings.Contains(sec.Content[i:], "}}") {
			report.add("placeholders", "section %q contains an unresolved {{...}} placeholder", sec.Heading)
		}
		if containsWord(sec.Content, "TODO") {
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
