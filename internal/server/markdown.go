package server

import (
	"html"
	"html/template"
	"strings"
)

// renderMarkdown is a small, dependency-free Markdown-to-HTML renderer for the
// document read view (DESIGN-007 §5; SPEC-004 §6 "the honest floor"). It covers
// the fidelity a review reader needs — ATX headings, fenced code, blockquotes,
// bullet and ordered lists, horizontal rules, paragraphs, and inline code /
// bold / italic / links — and nothing more; richer rendering is production
// work. Every fragment of source text is HTML-escaped before emission, so a
// document body can never inject markup into the page (NFR-5).
//
// It is deliberately not a CommonMark implementation: the read view is
// read-only and the body already lives in git, reviewed in the author's editor.
func renderMarkdown(src string) template.HTML {
	var b strings.Builder
	lines := strings.Split(strings.ReplaceAll(src, "\r\n", "\n"), "\n")

	inCode := false   // inside a ``` fence
	codeFence := ""   // the fence token that opened the block
	var para []string // buffered paragraph lines
	listType := ""    // "ul" | "ol" | "" (none open)
	inQuote := false  // inside a blockquote

	flushPara := func() {
		if len(para) == 0 {
			return
		}
		b.WriteString("<p>")
		b.WriteString(string(renderInline(strings.Join(para, " "))))
		b.WriteString("</p>\n")
		para = para[:0]
	}
	closeList := func() {
		if listType != "" {
			b.WriteString("</" + listType + ">\n")
			listType = ""
		}
	}
	closeQuote := func() {
		if inQuote {
			b.WriteString("</blockquote>\n")
			inQuote = false
		}
	}

	for _, ln := range lines {
		trimmed := strings.TrimSpace(ln)

		// Fenced code blocks take precedence over everything.
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fence := trimmed[:3]
			if inCode && fence == codeFence {
				b.WriteString("</code></pre>\n")
				inCode = false
				codeFence = ""
				continue
			}
			if !inCode {
				flushPara()
				closeList()
				closeQuote()
				inCode = true
				codeFence = fence
				b.WriteString("<pre><code>")
				continue
			}
		}
		if inCode {
			b.WriteString(html.EscapeString(ln))
			b.WriteString("\n")
			continue
		}

		// Blank line: paragraph / list / quote break.
		if trimmed == "" {
			flushPara()
			closeList()
			closeQuote()
			continue
		}

		// Horizontal rule.
		if trimmed == "---" || trimmed == "***" || trimmed == "___" {
			flushPara()
			closeList()
			closeQuote()
			b.WriteString("<hr>\n")
			continue
		}

		// ATX headings.
		if h := headingLevel(trimmed); h > 0 {
			flushPara()
			closeList()
			closeQuote()
			text := strings.TrimSpace(trimmed[h:])
			tag := "h" + string(rune('0'+h))
			b.WriteString("<" + tag + ">")
			b.WriteString(string(renderInline(text)))
			b.WriteString("</" + tag + ">\n")
			continue
		}

		// Blockquote.
		if strings.HasPrefix(trimmed, ">") {
			flushPara()
			closeList()
			if !inQuote {
				b.WriteString("<blockquote>\n")
				inQuote = true
			}
			text := strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
			b.WriteString("<p>")
			b.WriteString(string(renderInline(text)))
			b.WriteString("</p>\n")
			continue
		}

		// List items.
		if item, ordered, ok := listItem(trimmed); ok {
			flushPara()
			closeQuote()
			want := "ul"
			if ordered {
				want = "ol"
			}
			if listType != want {
				closeList()
				b.WriteString("<" + want + ">\n")
				listType = want
			}
			b.WriteString("<li>")
			b.WriteString(string(renderInline(item)))
			b.WriteString("</li>\n")
			continue
		}

		// Otherwise it is paragraph text.
		closeList()
		closeQuote()
		para = append(para, trimmed)
	}
	flushPara()
	closeList()
	closeQuote()
	if inCode {
		b.WriteString("</code></pre>\n")
	}
	return template.HTML(b.String())
}

// headingLevel returns 1..6 for an ATX heading line ("## Title"), else 0.
func headingLevel(s string) int {
	n := 0
	for n < len(s) && s[n] == '#' {
		n++
	}
	if n >= 1 && n <= 6 && n < len(s) && s[n] == ' ' {
		return n
	}
	return 0
}

// listItem reports whether s is a bullet ("- ", "* ", "+ ") or ordered ("1. ")
// list item and returns the item text.
func listItem(s string) (text string, ordered bool, ok bool) {
	for _, m := range []string{"- ", "* ", "+ "} {
		if strings.HasPrefix(s, m) {
			return strings.TrimSpace(s[len(m):]), false, true
		}
	}
	// Ordered: digits followed by ". ".
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i > 0 && i+1 < len(s) && s[i] == '.' && s[i+1] == ' ' {
		return strings.TrimSpace(s[i+2:]), true, true
	}
	return "", false, false
}

// renderInline escapes text and applies inline spans: `code`, **bold**,
// *italic*, and [label](url) links. Escaping happens first, so no source text
// can inject markup; the span markers are then matched over the escaped text.
func renderInline(s string) template.HTML {
	s = html.EscapeString(s)
	s = spanReplace(s, "`", "<code>", "</code>")
	s = spanReplace(s, "**", "<strong>", "</strong>")
	s = spanReplace(s, "*", "<em>", "</em>")
	s = renderLinks(s)
	return template.HTML(s)
}

// spanReplace wraps the text between paired occurrences of marker in open/close
// tags. Unpaired markers are left as literal text.
func spanReplace(s, marker, open, close string) string {
	var b strings.Builder
	parts := strings.Split(s, marker)
	if len(parts) < 3 {
		return s
	}
	for i, p := range parts {
		if i == 0 {
			b.WriteString(p)
			continue
		}
		// Odd indices open a span; even indices close it — but only if a
		// closing marker actually followed (i.e. there is a further part).
		if i%2 == 1 && i+1 < len(parts) {
			b.WriteString(open)
			b.WriteString(p)
			b.WriteString(close)
		} else {
			b.WriteString(p)
		}
	}
	return b.String()
}

// renderLinks turns [label](url) into anchors, admitting only http(s)/relative
// URLs so a body cannot smuggle a javascript: scheme (NFR-5).
func renderLinks(s string) string {
	var b strings.Builder
	for {
		open := strings.Index(s, "[")
		if open < 0 {
			b.WriteString(s)
			break
		}
		mid := strings.Index(s[open:], "](")
		if mid < 0 {
			b.WriteString(s)
			break
		}
		mid += open
		end := strings.Index(s[mid:], ")")
		if end < 0 {
			b.WriteString(s)
			break
		}
		end += mid
		label := s[open+1 : mid]
		url := s[mid+2 : end]
		b.WriteString(s[:open])
		if safeURL(url) {
			b.WriteString(`<a href="` + url + `">` + label + `</a>`)
		} else {
			// Reproduce the literal source for an unsafe/odd URL.
			b.WriteString("[" + label + "](" + url + ")")
		}
		s = s[end+1:]
	}
	return b.String()
}

func safeURL(u string) bool {
	lu := strings.ToLower(strings.TrimSpace(u))
	if lu == "" {
		return false
	}
	if strings.HasPrefix(lu, "http://") || strings.HasPrefix(lu, "https://") {
		return true
	}
	// Relative links (no scheme) are fine; anything with a scheme we don't
	// recognise (javascript:, data:, ...) is rejected.
	return !strings.Contains(lu, ":")
}
