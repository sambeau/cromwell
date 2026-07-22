package server

import (
	"bytes"
	"html/template"
	"sync"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	goldmarkhtml "github.com/yuin/goldmark/renderer/html"
)

// renderMarkdown renders a document body to safe HTML for the read view
// (DESIGN-007 §5; SPEC-004 §6). It is a battle-hardened, community-maintained
// stack rather than a bespoke renderer, so full design documents — tables,
// images, the GFM extensions — render with real fidelity:
//
//   - goldmark (the CommonMark engine Hugo uses) parses and renders, with the
//     GFM extension (tables, strikethrough, task lists, autolinks) and raw HTML
//     passed through;
//   - bluemonday then sanitizes the result, so the security of the read view
//     rests on a maintained allowlist, not on the renderer never emitting HTML.
//     Scripts, event handlers, and dangerous URL schemes (javascript:, data:)
//     are stripped; headings, tables, images, and http(s)/relative links are
//     kept (NFR-5).
//
// The body already lives in git and is authored in the user's editor; this view
// is read-only (CC-5), so a viewed document may load images by URL — desired for
// design docs, and the reason sanitization, not blanket escaping, is the model.
func renderMarkdown(src string) template.HTML {
	md, policy := mdEngine()
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		// Fall back to escaped source rather than render nothing.
		return template.HTML(template.HTMLEscapeString(src))
	}
	return template.HTML(policy.SanitizeBytes(buf.Bytes()))
}

var (
	mdOnce    sync.Once
	mdEngineV goldmark.Markdown
	mdPolicy  *bluemonday.Policy
)

func mdEngine() (goldmark.Markdown, *bluemonday.Policy) {
	mdOnce.Do(func() {
		mdEngineV = goldmark.New(
			goldmark.WithExtensions(extension.GFM),
			// Pass raw HTML through goldmark; bluemonday is the safety net, so a
			// design doc that embeds a little HTML still renders (sanitized).
			goldmark.WithRendererOptions(goldmarkhtml.WithUnsafe()),
		)
		mdPolicy = docSanitizerPolicy()
	})
	return mdEngineV, mdPolicy
}

// docSanitizerPolicy is the allowlist for rendered document bodies: bluemonday's
// user-generated-content base, extended for the fidelity a design doc needs —
// relative links between docs, GFM tables, and task-list checkboxes — and no
// more. It strips scripts, event handlers, and dangerous URL schemes.
func docSanitizerPolicy() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()

	// Internal design docs link to each other by relative path.
	p.AllowRelativeURLs(true)
	p.AllowURLSchemes("http", "https", "mailto")

	// GFM tables (goldmark emits align attributes for column alignment).
	p.AllowElements("table", "thead", "tbody", "tfoot", "tr", "th", "td", "caption")
	p.AllowAttrs("align").OnElements("th", "td", "caption")
	p.AllowAttrs("colspan", "rowspan").OnElements("th", "td")

	// GFM task lists render as disabled checkboxes.
	p.AllowAttrs("type", "checked", "disabled").OnElements("input")

	return p
}
