// Package content implements the content store's file-side machinery:
// parsing Markdown documents into front matter and heading-based sections
// (DESIGN-001 §5, vision §10), hashing, and (later phases) retrieval and
// prompt assembly inputs.
package content

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"subutai/internal/config"
)

// Section is one heading-delimited block. Position is 0-based document
// order. A preamble before the first heading becomes a level-0 section with
// an empty heading.
type Section struct {
	Position int
	Heading  string
	Level    int
	Content  string
}

// Doc is a parsed Markdown document.
type Doc struct {
	FrontMatter map[string]any
	Sections    []Section
	Raw         string
}

// Hash returns the content hash recorded in documents.content_hash.
func Hash(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// Parse splits raw Markdown into front matter and sections. A missing or
// malformed front matter is an error — the validation engine reports it as
// check 1 rather than panicking downstream.
func Parse(raw string) (*Doc, error) {
	fm, body, err := config.SplitFrontMatter(raw)
	if err != nil {
		return nil, fmt.Errorf("front matter: %w", err)
	}
	meta := map[string]any{}
	if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
		return nil, fmt.Errorf("front matter: %w", err)
	}

	doc := &Doc{FrontMatter: meta, Raw: raw}
	var current *Section
	flush := func() {
		if current != nil {
			current.Content = strings.TrimSpace(current.Content)
			doc.Sections = append(doc.Sections, *current)
		}
	}
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if !inFence {
			if level, heading, ok := parseHeading(line); ok {
				flush()
				current = &Section{Position: len(doc.Sections), Heading: heading, Level: level}
				continue
			}
		}
		if current == nil {
			if strings.TrimSpace(line) == "" {
				continue
			}
			current = &Section{Position: 0, Heading: "", Level: 0}
		}
		current.Content += line + "\n"
	}
	flush()
	return doc, nil
}

func parseHeading(line string) (level int, heading string, ok bool) {
	trimmed := strings.TrimLeft(line, "#")
	level = len(line) - len(trimmed)
	if level == 0 || level > 6 || !strings.HasPrefix(trimmed, " ") {
		return 0, "", false
	}
	return level, strings.TrimSpace(trimmed), true
}

// SectionByHeading finds a section by case-insensitive heading match at any
// level (DESIGN-004 §7), or nil.
func (d *Doc) SectionByHeading(heading string) *Section {
	for i := range d.Sections {
		if strings.EqualFold(d.Sections[i].Heading, heading) {
			return &d.Sections[i]
		}
	}
	return nil
}

// FrontMatterString returns a front-matter value as a string, "" if absent
// or not scalar.
func (d *Doc) FrontMatterString(key string) string {
	v, ok := d.FrontMatter[key]
	if !ok {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	default:
		return fmt.Sprintf("%v", v)
	}
}

// Links returns the targets of inline Markdown links [text](target),
// skipping code fences and external URLs.
func (d *Doc) Links() []string {
	var links []string
	inFence := false
	for _, line := range strings.Split(d.Raw, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		rest := line
		for {
			_, after, ok := strings.Cut(rest, "](")
			if !ok {
				break
			}
			target, tail, ok := strings.Cut(after, ")")
			if !ok {
				break
			}
			target = strings.TrimSpace(target)
			if target != "" &&
				!strings.Contains(target, "://") &&
				!strings.HasPrefix(target, "#") &&
				!strings.HasPrefix(target, "mailto:") {
				// Strip an in-file anchor from a relative link.
				if base, _, found := strings.Cut(target, "#"); found {
					target = base
				}
				if target != "" {
					links = append(links, target)
				}
			}
			rest = tail
		}
	}
	return links
}
