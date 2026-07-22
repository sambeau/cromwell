package server

import (
	"strings"
	"testing"
)

func TestRenderMarkdownStructure(t *testing.T) {
	out := string(renderMarkdown("# Heading\n\nA paragraph with **bold**, *italic*, and `code`.\n\n- one\n- two\n\n```\nfenced code\n```\n\n> a quote\n"))
	for _, want := range []string{
		"<h1", "Heading",
		"<strong>bold</strong>",
		"<em>italic</em>",
		"<code>code</code>",
		"<ul>", "<li>one</li>", "<li>two</li>",
		"<pre><code>", "fenced code",
		"<blockquote>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered markdown missing %q\n---\n%s", want, out)
		}
	}
}

// TestRenderMarkdownFullFidelity proves the reason we moved to goldmark: GFM
// tables and images render, which the bespoke floor did not do.
func TestRenderMarkdownFullFidelity(t *testing.T) {
	table := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	out := string(renderMarkdown(table))
	for _, want := range []string{"<table>", "<th", "<td", "1", "2"} {
		if !strings.Contains(out, want) {
			t.Errorf("table not rendered, missing %q\n---\n%s", want, out)
		}
	}

	img := string(renderMarkdown("![a diagram](https://example.com/d.png)\n"))
	if !strings.Contains(img, "<img") || !strings.Contains(img, `src="https://example.com/d.png"`) {
		t.Errorf("image not rendered:\n%s", img)
	}
}

// TestRenderMarkdownEscapesHTML is the load-bearing security assertion (NFR-5):
// a document body can never inject active markup into the page. bluemonday
// strips scripts, event handlers, and unknown/dangerous constructs.
func TestRenderMarkdownEscapesHTML(t *testing.T) {
	out := string(renderMarkdown("Hello <script>alert('xss')</script> and <img src=x onerror=alert(1)>.\n"))
	if strings.Contains(out, "<script") {
		t.Errorf("script tag survived sanitization:\n%s", out)
	}
	if strings.Contains(out, "onerror") {
		t.Errorf("event handler survived sanitization:\n%s", out)
	}
	if strings.Contains(out, "alert(1)") {
		t.Errorf("inline script payload survived:\n%s", out)
	}
}

// TestRenderMarkdownRejectsDangerousLinks ensures dangerous URL schemes are not
// emitted as live links or image sources (NFR-5), while safe and relative links
// are kept.
func TestRenderMarkdownRejectsDangerousLinks(t *testing.T) {
	js := string(renderMarkdown("[click](javascript:alert(1))\n"))
	if strings.Contains(js, "javascript:") {
		t.Errorf("javascript: scheme survived:\n%s", js)
	}

	dataImg := string(renderMarkdown("![x](data:text/html,<script>alert(1)</script>)\n"))
	if strings.Contains(dataImg, "data:text/html") {
		t.Errorf("data: image URL survived:\n%s", dataImg)
	}

	safe := string(renderMarkdown("[docs](https://example.com/x)\n"))
	if !strings.Contains(safe, `href="https://example.com/x"`) {
		t.Errorf("safe link dropped:\n%s", safe)
	}
	rel := string(renderMarkdown("[rel](../other.md)\n"))
	if !strings.Contains(rel, `href="../other.md"`) {
		t.Errorf("relative link dropped:\n%s", rel)
	}
}
