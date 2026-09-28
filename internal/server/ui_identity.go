package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"subutai/internal/content"
	"subutai/internal/ident"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// The web UI's half of documents with identity (SPEC-015): /ui/id/<ID>, adopt,
// give a registered document an ID, record an adopted draft as already
// approved, and the create actions that start a design.

// --- /ui/id/<ID> (SD-18) ---

// handleUIByID redirects an ID to its page. Lower-case is accepted here,
// because this route only ever means an ID.
func (s *Server) handleUIByID(w http.ResponseWriter, r *http.Request) {
	raw := r.PathValue("id")
	target, err := s.urlForID(r.Context(), raw)
	if err != nil || target == "" {
		s.uiNotFound(w, r, "thing with the ID", raw)
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

// urlForID is the page an ID names, or "" when it names nothing.
func (s *Server) urlForID(ctx context.Context, raw string) (string, error) {
	ref, ok := ident.Parse(raw)
	if !ok {
		return "", nil
	}
	pool := s.Store.Pool
	switch ref.Shape {
	case ident.ShapeEntity:
		switch ref.Kind.Name {
		case "initiative":
			in, err := store.InitiativeByPublicID(ctx, pool, ref.ID)
			if err != nil {
				return "", ignoreNotFound(err)
			}
			path, err := s.initiativePath(ctx, in.ID)
			return "/ui/i/" + path, err
		case "feature":
			f, err := store.FeatureByPublicID(ctx, pool, ref.ID)
			if err != nil {
				return "", ignoreNotFound(err)
			}
			path, err := s.featurePath(ctx, f)
			return "/ui/f/" + path, err
		case "milestone":
			m, err := store.MilestoneByPublicID(ctx, pool, ref.ID)
			if err != nil {
				return "", ignoreNotFound(err)
			}
			return milestoneURL(m.ID), nil
		case "roadmap":
			rm, err := store.RoadmapByPublicID(ctx, pool, ref.ID)
			if err != nil {
				return "", ignoreNotFound(err)
			}
			return roadmapURL(rm.ID), nil
		case "checklist":
			// SPEC-017 FR-3.4, the M8 follow-up SD-19 left for after M5.
			c, err := store.ChecklistByPublicID(ctx, pool, ref.ID)
			if err != nil {
				return "", ignoreNotFound(err)
			}
			return checklistURL(c.ID), nil
		}
		// Bugs and spikes don't exist yet.
		return "", nil
	case ident.ShapeTask:
		t, err := store.TaskByPublicID(ctx, pool, ref.ID)
		if err != nil {
			return "", ignoreNotFound(err)
		}
		return "/ui/t/" + t.ID.String(), nil
	case ident.ShapeDecision, ident.ShapeDocument:
		var d *store.Document
		var err error
		if ref.Revision > 0 {
			d, err = store.DocumentByIdentity(ctx, pool, ref.ID, ref.Revision)
		} else {
			d, err = store.CurrentDocumentByPublicID(ctx, pool, ref.ID)
		}
		if err != nil {
			return "", ignoreNotFound(err)
		}
		if d.State == lifecycle.DocSuperseded {
			// Superseded revisions have no page of their own; the current one
			// is the useful destination.
			if cur, err := store.CurrentDocumentByPublicID(ctx, pool, ref.ID); err == nil && cur.State != lifecycle.DocSuperseded {
				d = cur
			} else if rec, err := store.SupersededDecisionAtPath(ctx, pool, d.Path); err == nil && rec.PublicID == ref.ID {
				// ...except a decision another superseded: it is a record in
				// place, with its page (SPEC-018 SD-6).
				d = rec
			} else {
				return "", nil
			}
		}
		return "/ui/d/" + d.Path, nil
	}
	return "", nil
}

// --- A document's old path after a move (FR-4.5) ---

// movedFrom finds where a document that used to be at path now is, from the
// newest document.moved row that names path as its origin.
func (s *Server) movedFrom(ctx context.Context, path string) string {
	var id uuid.UUID
	if err := s.Store.Pool.QueryRow(ctx, `SELECT ref_id FROM audit_events
		WHERE kind = 'document.moved' AND payload->>'from' = $1
		ORDER BY occurred_at DESC LIMIT 1`, path).Scan(&id); err != nil {
		return ""
	}
	d, err := store.GetDocument(ctx, s.Store.Pool, id)
	if err != nil || d.Path == path || d.State == lifecycle.DocSuperseded {
		return ""
	}
	return d.Path
}

// --- The document page's identity panel (FR-4.6, FR-5.6, FR-5.8) ---

// docIdentity is what the document page says and offers about its ID.
type docIdentity struct {
	// Lost is set when the file no longer carries the ID it is registered
	// with, so a move wouldn't be followed.
	Lost bool
	// CanGiveID offers "Give it an ID" to a document known only by its path.
	CanGiveID bool
	// CanRecordApproved offers "This was already approved" to an adopted
	// draft of a type with no template.
	CanRecordApproved bool
}

func (s *Server) docIdentityFor(ctx context.Context, d *store.Document) docIdentity {
	var out docIdentity
	if d.PublicID == "" {
		out.CanGiveID = d.State != lifecycle.DocSuperseded && s.checkAdoptable(ctx, d) == nil
		return out
	}
	if raw, err := s.readDocFile(d.Path); err == nil {
		id, rev, ok := content.ReadIdentity(string(raw))
		out.Lost = !ok || id != d.PublicID || rev != d.Revision
	}
	out.CanRecordApproved = s.canRecordAlreadyApproved(d)
	return out
}

// --- Adopt (FR-5.6) ---

// handleEntityAdopt adopts a file for the page it was posted from. Only the
// file's path is typed; the owner is the page, carried by id (NFR-4).
func (s *Server) handleEntityAdopt(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ownerType := strings.TrimSpace(r.FormValue("owner_type"))
	var ownerID *uuid.UUID
	renderID := uuid.Nil
	if ownerType != "project" {
		id, err := formID(r)
		if err != nil {
			http.Error(w, "bad owner id", http.StatusBadRequest)
			return
		}
		ownerID, renderID = &id, id
	}
	state := lifecycle.DocDraft
	if r.FormValue("state") == "approved" {
		state = lifecycle.DocApproved
	}
	res, err := s.AdoptDocument(r.Context(), AdoptRequest{
		Path:      r.FormValue("file_path"),
		DocType:   strings.TrimSpace(r.FormValue("doc_type")),
		OwnerType: ownerType, OwnerID: ownerID, State: state,
		Actor: s.uiActor(), Via: "ui",
	})
	if err != nil {
		s.renderEntity(w, r, ownerType, renderID, "", err.Error())
		return
	}
	s.renderEntity(w, r, ownerType, renderID, adoptNotice(res), "")
}

// adoptNotice says what adopt did, in a sentence or three.
func adoptNotice(res *AdoptResult) string {
	d := res.Doc
	n := fmt.Sprintf("%s is now %s", d.Path, d.PublicID)
	if d.State == lifecycle.DocApproved {
		n += ", recorded as approved by you"
	}
	n += "."
	if res.Committed {
		n += " Its ID was written into its front matter and committed."
	} else {
		n += " Its ID was written into its front matter, but that change couldn't be committed (" +
			res.CommitError + "); commit it yourself."
	}
	if res.MadeMain {
		n += " It is now the page's main document, in place of the empty starter design."
	}
	return n
}

// handleDocAdopt is "Give it an ID" on a document known only by its path.
func (s *Server) handleDocAdopt(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	res, err := s.AdoptDocument(r.Context(), AdoptRequest{
		Path: doc.Path, Actor: s.uiActor(), Via: "ui",
	})
	if err != nil {
		s.afterDocAct(w, r, doc, "", err)
		return
	}
	s.afterDocAct(w, r, res.Doc, adoptNotice(res), nil)
}

// handleDocAlreadyApproved is FR-5.8.
func (s *Server) handleDocAlreadyApproved(w http.ResponseWriter, r *http.Request) {
	doc, ok := s.docForm(w, r)
	if !ok {
		return
	}
	if _, err := s.RecordAlreadyApproved(r.Context(), doc.ID, s.uiActor()); err != nil {
		s.afterDocAct(w, r, doc, "", err)
		return
	}
	s.afterDocAct(w, r, doc, "Recorded as approved by you, before it came into Subutai.", nil)
}

// --- Creating work creates its documents (FR-6) ---

// startDesignWanted reads the create dialogs' opt-out (FR-6.4). A form that
// didn't offer the box, such as an older page, gets the default: on.
func startDesignWanted(r *http.Request) bool {
	if r.FormValue("start_design_offered") == "" {
		return true
	}
	return r.FormValue("start_design") != ""
}

// createdNotice says what a create made, including its design document.
func createdNotice(kind, publicID, name, designPath string) string {
	n := fmt.Sprintf("%s created: %s %s.", kind, publicID, name)
	if designPath != "" {
		n += " Its design document was started at " + designPath + ", from the template, and committed."
	}
	return n
}
