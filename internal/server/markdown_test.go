package server

import (
	"strings"
	"testing"
)

func TestRenderMarkdownStructure(t *testing.T) {
	out := string(renderMarkdown("# Heading\n\nA paragraph with **bold**, *italic*, and `code`.\n\n- one\n- two\n\n```\nfenced code\n```\n\n> a quote\n"))
	for _, want := range []string{
		"<h1>Heading</h1>",
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

// TestRenderMarkdownEscapesHTML is the load-bearing security assertion (NFR-5):
// a document body can never inject markup into the page. Every angle bracket and
// every attribute-breaking quote in source must be escaped.
func TestRenderMarkdownEscapesHTML(t *testing.T) {
	out := string(renderMarkdown("Hello <script>alert('xss')</script> & \"quotes\".\n"))
	if strings.Contains(out, "<script>") {
		t.Errorf("script tag survived escaping:\n%s", out)
	}
	for _, want := range []string{"&lt;script&gt;", "&amp;"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected escaped %q in:\n%s", want, out)
		}
	}
}

// TestRenderMarkdownRejectsDangerousLinks ensures a javascript: URL is not
// emitted as a live href (NFR-5); it falls back to literal source.
func TestRenderMarkdownRejectsDangerousLinks(t *testing.T) {
	out := string(renderMarkdown("[click](javascript:alert(1))\n"))
	if strings.Contains(out, "href=\"javascript:") {
		t.Errorf("javascript: scheme became a live link:\n%s", out)
	}

	safe := string(renderMarkdown("[docs](https://example.com/x)\n"))
	if !strings.Contains(safe, `<a href="https://example.com/x">docs</a>`) {
		t.Errorf("safe link not rendered as anchor:\n%s", safe)
	}
	rel := string(renderMarkdown("[rel](../other.md)\n"))
	if !strings.Contains(rel, `<a href="../other.md">rel</a>`) {
		t.Errorf("relative link not rendered as anchor:\n%s", rel)
	}
}
