package server

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"subutai/internal/config"
	"subutai/internal/content"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
	"subutai/internal/textdiff"
)

// The browser editor's pages (SPEC-016). The logic lives in edit.go; this
// file turns it into pages and fragments.

// editorPage is the editor for one document, or the page that explains why
// it can't be edited as it stands.
type editorPage struct {
	Doc         store.Document
	OwnerCrumb  crumb
	Breadcrumbs []crumb
	DocURL      string
	EditURL     string

	Text     string // what goes in the text box
	Base     string // the file as the editor loaded it
	BaseHash string
	Subject  string
	Preview  template.HTML

	StateLine string
	Warnings  []string
	Refusal   string // the document can't be edited here at all

	// Approved documents are edited by starting a revision (FR-4.3).
	Approved      bool
	RevisionNotes []string
	ReviseWhy     string // why a revision can't be started from here

	Notice   string
	Error    string
	Conflict *conflictView
}

func (p editorPage) headTitle() string { return "Editing " + withID(p.Doc.PublicID, p.Doc.Title) }
func (p editorPage) headCrumbs() []crumb {
	return append(append([]crumb(nil), p.Breadcrumbs...), crumb{Label: "Edit", Here: true})
}

// conflictView is what a stopped save shows (SD-7).
type conflictView struct {
	OnDisk diffView // from the base to the file now
	Mine   diffView // from the base to the person's text
	// Stale is a save stopped for a change of state, place or permission
	// rather than of the file: there is no change on disk to show.
	Stale bool
}

type diffView struct {
	Hunks          [][]diffLine
	TooLarge       bool
	Empty          bool
	Removed, Added int
	Before, After  string // shown whole when the diff is too large
}

type diffLine struct {
	Class string // same | removed | added
	Mark  string
	No    int
	Text  string
}

func makeDiffView(before, after string) diffView {
	d := textdiff.Compare(before, after, 3)
	v := diffView{TooLarge: d.TooLarge, Empty: d.Empty()}
	v.Removed, v.Added = d.Counts()
	if d.TooLarge {
		v.Before, v.After = before, after
		return v
	}
	for _, h := range d.Hunks {
		var lines []diffLine
		for _, l := range h.Lines {
			dl := diffLine{Text: l.Text, No: l.NewNo}
			switch l.Kind {
			case textdiff.Removed:
				dl.Class, dl.Mark, dl.No = "removed", "-", l.OldNo
			case textdiff.Added:
				dl.Class, dl.Mark = "added", "+"
			default:
				dl.Class, dl.Mark = "same", " "
			}
			lines = append(lines, dl)
		}
		v.Hunks = append(v.Hunks, lines)
	}
	return v
}

// editPath is the editor's address for a document path.
func editPath(docPath string) string { return "/ui/edit/" + docPath }

// previewHTML renders a text's prose as the document page does (FR-1.3).
func previewHTML(text string) template.HTML {
	text = strings.ReplaceAll(strings.TrimPrefix(text, bom), "\r\n", "\n")
	if _, md, err := config.SplitFrontMatter(text); err == nil {
		text = md
	}
	return renderMarkdown(text)
}

// editorView builds the editor page for a document. text, base and baseHash
// are the editor's state after a refused save; empty, they are read from
// the file.
func (s *Server) editorView(ctx context.Context, doc *store.Document) (*editorPage, error) {
	p := &editorPage{
		Doc:        *doc,
		OwnerCrumb: s.ownerCrumb(ctx, doc.OwnerType, doc.OwnerID),
		DocURL:     "/ui/d/" + doc.Path,
		EditURL:    editPath(doc.Path),
	}
	p.Breadcrumbs = []crumb{p.OwnerCrumb, {ID: idFor(doc.PublicID, doc.Title), Label: doc.Title, URL: p.DocURL}}
	switch doc.State {
	case lifecycle.DocDraft:
		p.StateLine = "This document is a draft, so it is edited in place."
	case lifecycle.DocReviewing:
		p.StateLine = "This document is under review."
	case lifecycle.DocApproved:
		p.StateLine = "This document is approved."
	case lifecycle.DocSuperseded:
		p.StateLine = "This document has been superseded."
	}
	p.Warnings = s.editWarnings(ctx, doc)

	if doc.State == lifecycle.DocApproved {
		p.Approved = true
		p.RevisionNotes = s.revisionNotes(ctx, doc)
		p.ReviseWhy = s.reviseRefusal(ctx, doc)
		return p, nil
	}
	if p.Refusal = s.editRefusal(ctx, doc); p.Refusal != "" {
		return p, nil
	}
	raw, err := s.readDocFile(doc.Path)
	if err != nil {
		p.Refusal = "The file for this document can't be read from the working tree, so there's nothing to edit. Restore it in git first."
		return p, nil
	}
	p.Text = editText(raw)
	p.Base = p.Text
	p.BaseHash = content.Hash(raw)
	p.Preview = previewHTML(p.Text)
	return p, nil
}

func (s *Server) renderEditor(w http.ResponseWriter, r *http.Request, p *editorPage) {
	s.render(w, "page-edit", s.page(r.Context(), "documents", p))
}

// GET /ui/edit/<path>
func (s *Server) handleUIEdit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	path := r.PathValue("path")
	doc, err := store.LiveDocumentByPath(ctx, s.Store.Pool, path)
	if err == store.ErrNotFound {
		if to := s.movedFrom(ctx, path); to != "" {
			http.Redirect(w, r, editPath(to), http.StatusFound)
			return
		}
	}
	if err != nil {
		s.notFoundOrErr(w, r, "document", path, err)
		return
	}
	// An approved document with a revision open is edited through the
	// revision (FR-4.3).
	if doc.State == lifecycle.DocApproved {
		if succ, err := s.liveSuccessor(ctx, doc.ID); err == nil && succ != nil {
			http.Redirect(w, r, editPath(succ.Path), http.StatusFound)
			return
		}
	}
	p, err := s.editorView(ctx, doc)
	if err != nil {
		s.uiError(w, err)
		return
	}
	if r.URL.Query().Get("opened") == "1" && doc.SupersedesID != nil {
		p.Notice = "A revision was opened. The original stays approved until this is."
	}
	s.renderEditor(w, r, p)
}

// sameOrigin refuses a cross-site post to the editor, which writes and
// commits files (R16-9). Browsers send Sec-Fetch-Site or Origin with a form
// post; a request with neither (a script on the machine) is let through, as
// it could reach the file directly anyway.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "":
	default:
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	return err == nil && u.Host == r.Host
}

func (s *Server) refuseCrossSite(w http.ResponseWriter, r *http.Request) bool {
	if sameOrigin(r) {
		return false
	}
	http.Error(w, "The editor only accepts saves from its own pages.", http.StatusForbidden)
	return true
}

// POST /ui/edit/save
func (s *Server) handleUIEditSave(w http.ResponseWriter, r *http.Request) {
	if s.refuseCrossSite(w, r) {
		return
	}
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	req := EditRequest{
		DocID:    doc.ID,
		BaseHash: r.FormValue("base_hash"),
		Text:     r.FormValue("text"),
		Commit:   r.FormValue("action") == "commit",
		Subject:  r.FormValue("subject"),
		Actor:    s.uiActor(),

		ShownState: r.FormValue("shown_state"),
		ShownPath:  r.FormValue("shown_path"),
	}
	res, err := s.SaveEdit(ctx, req)
	if err != nil {
		// A refused save keeps the person's text in the editor (FR-2.3).
		p, verr := s.editorView(ctx, doc)
		if verr != nil {
			s.uiError(w, verr)
			return
		}
		base := strings.ReplaceAll(r.FormValue("base"), "\r\n", "\n")
		text := strings.ReplaceAll(req.Text, "\r\n", "\n")
		p.Text, p.Base, p.BaseHash, p.Subject = text, base, req.BaseHash, req.Subject
		p.Preview = previewHTML(text)
		p.Error = err.Error()
		var conflict *EditConflict
		var stale *EditStale
		switch {
		case errors.As(err, &conflict):
			p.Conflict = &conflictView{
				OnDisk: makeDiffView(base, conflict.Current),
				Mine:   makeDiffView(base, text),
			}
		case errors.As(err, &stale):
			// The document changed state or moved: keep the text to copy,
			// and offer the editor afresh (SD-6).
			p.Conflict = &conflictView{Mine: makeDiffView(base, text), Stale: true}
			p.Approved, p.Refusal = false, ""
			p.EditURL = editPath(doc.Path)
		}
		if p.Refusal != "" || p.Approved {
			// The document can no longer be edited at all (its author
			// started, say): say so, and keep the text to copy.
			if p.Refusal != "" {
				p.Error = p.Refusal
			}
			p.Refusal, p.Approved = "", false
			p.Conflict = &conflictView{Mine: makeDiffView(base, text), Stale: true}
		}
		s.renderEditor(w, r, p)
		return
	}

	var notice string
	switch {
	case res.CommitErr != nil:
		s.renderDocumentPage(w, r, doc.Path, "",
			"Your text was saved to the file, but it couldn't be committed, so commit it yourself: "+res.CommitErr.Error())
		return
	case res.Commit != "" && res.Wrote:
		notice = "Saved and committed as " + res.Commit + "."
	case res.Commit != "":
		notice = "Committed as " + res.Commit + ". The file already held your text."
	case res.Wrote:
		notice = "Saved. The file is written; it isn't committed yet."
	default:
		notice = "Nothing changed, so nothing was saved."
	}
	if res.Withdrew {
		notice += " It went back to draft, so submit it again when you're done."
	}
	s.renderDocumentPage(w, r, doc.Path, notice, "")
}

// POST /ui/edit/preview — the preview fragment (FR-1.3).
func (s *Server) handleUIEditPreview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, previewHTML(r.FormValue("text")))
}

// POST /ui/edit/revise — start a revision of an approved document and open
// its editor (FR-4.3).
func (s *Server) handleUIEditRevise(w http.ResponseWriter, r *http.Request) {
	if s.refuseCrossSite(w, r) {
		return
	}
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	succ, _, err := s.StartRevisionForEdit(r.Context(), doc.ID, s.uiActor())
	if err != nil {
		p, verr := s.editorView(r.Context(), doc)
		if verr != nil {
			s.uiError(w, verr)
			return
		}
		p.Error = err.Error()
		s.renderEditor(w, r, p)
		return
	}
	http.Redirect(w, r, editPath(succ.Path)+"?opened=1", http.StatusSeeOther)
}

// GET /ui/frag/edit-fresh?doc_id=…&base=… — whether the file still matches
// what the editor loaded (FR-1.5, FR-3.3). Empty while it does.
func (s *Server) handleFragEditFresh(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	id, err := uuid.Parse(r.URL.Query().Get("doc_id"))
	if err != nil {
		return
	}
	doc, err := store.GetDocument(r.Context(), s.Store.Pool, id)
	if err != nil {
		return
	}
	raw, err := s.readDocFile(doc.Path)
	if err == nil && content.Hash(raw) == r.URL.Query().Get("base") {
		return
	}
	s.render(w, "edit-stale", map[string]string{"EditURL": editPath(doc.Path)})
}

// editFreshURL is the freshness fragment's address for the editor.
func (p editorPage) FreshURL() string {
	return "/ui/frag/edit-fresh?doc_id=" + p.Doc.ID.String() + "&base=" + url.QueryEscape(p.BaseHash)
}
