package server

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/lifecycle"
	"subutai/internal/sizing"
	"subutai/internal/store"
)

// Actions relocated onto entity pages (SPEC-007 FR-5). Each posts to a /ui/*
// route, calls the SAME gated, audited service method the CLI uses, and re-
// renders the acting entity's page with an inline notice or error — never a raw
// 500 (FR-2.2), never a typed entity path (SD-6, NFR-8). The entity is implied
// by the id the page itself carries in a hidden field, not by anything a human
// types. The acting actor is the single configured operator (SD-4).

// renderEntity re-renders an entity's page (by ref type and id) with a notice or
// error banner. It is the shared response for every entity mutation, so the
// page reflects the change without a reload.
func (s *Server) renderEntity(w http.ResponseWriter, r *http.Request, refType string, id uuid.UUID, notice, errMsg string) {
	ctx := r.Context()
	switch refType {
	case "project":
		page, err := s.projectPage(ctx, notice, errMsg)
		if err != nil {
			s.uiError(w, err)
			return
		}
		pushPageURL(w, r, "/ui/project")
		s.render(w, "page-entity", s.page(r.Context(), "browse", page))
	case "initiative":
		in, err := store.GetInitiative(ctx, s.Store.Pool, id)
		if err != nil {
			s.uiError(w, err)
			return
		}
		page, err := s.initiativePage(ctx, in, notice, errMsg)
		if err != nil {
			s.uiError(w, err)
			return
		}
		pushPageURL(w, r, "/ui/i/"+page.Path)
		s.render(w, "page-entity", s.page(r.Context(), "browse", page))
	case "feature":
		f, err := store.GetFeature(ctx, s.Store.Pool, id)
		if err != nil {
			s.uiError(w, err)
			return
		}
		page, err := s.featurePage(ctx, f, notice, errMsg)
		if err != nil {
			s.uiError(w, err)
			return
		}
		pushPageURL(w, r, "/ui/f/"+page.Path)
		s.render(w, "page-entity", s.page(r.Context(), "browse", page))
	case "milestone":
		s.renderMilestonePage(w, r, id, notice, errMsg)
	case "checklist":
		s.renderChecklistPage(w, r, id, notice, errMsg)
	default:
		s.uiError(w, errUnknownRef(refType))
	}
}

// pushPageURL tells HTMX the address of the page a boosted form post rendered.
// Every body is hx-boost'ed, so HTMX otherwise puts the post's route — such
// as /ui/checklist/new — in the address bar. Reloading that address, which an
// editor does when it closes after a change (SPEC-010 FR-6.4), then lands on
// the wrong page. With the header, the address bar shows the page itself.
func pushPageURL(w http.ResponseWriter, r *http.Request, url string) {
	if r.Method == http.MethodPost && r.Header.Get("HX-Boosted") != "" && url != "" {
		w.Header().Set("HX-Push-Url", url)
	}
}

func errUnknownRef(refType string) error { return &refError{refType} }

type refError struct{ t string }

func (e *refError) Error() string { return "unknown entity kind: " + e.t }

// formID parses the id hidden field; project pages carry the empty string.
func formID(r *http.Request) (uuid.UUID, error) {
	return uuid.Parse(strings.TrimSpace(r.FormValue("id")))
}

// --- Edit the description and title (FR-4.1) ---

func (s *Server) handleEntityDescribe(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	refType := strings.TrimSpace(r.FormValue("ref_type"))
	id, err := formID(r)
	if err != nil {
		http.Error(w, "bad entity id", http.StatusBadRequest)
		return
	}
	var name, desc *string
	if r.Form.Has("name") {
		v := strings.TrimSpace(r.FormValue("name"))
		if v != "" {
			name = &v
		}
	}
	if r.Form.Has("description") {
		v := strings.TrimSpace(r.FormValue("description"))
		desc = &v // an empty description is a legitimate clear
	}
	err = s.Store.WithTx(r.Context(), func(tx pgx.Tx) error {
		return store.UpdateEntityFields(r.Context(), tx, refType, id, name, desc, s.uiActor())
	})
	if err != nil {
		s.renderEntity(w, r, refType, id, "", err.Error())
		return
	}
	// A feature gaining a description is an authoring trigger (SPEC-009
	// FR-4.3): under an approved design, describing it is what releases its
	// spec. An emptied description is not.
	if refType == "feature" && desc != nil && *desc != "" {
		s.Bus.Publish(bus.FeatureDescribed{FeatureID: id})
	}
	s.renderEntity(w, r, refType, id, "The description was updated.", "")
}

// --- Attach a document (FR-6.1) ---

func (s *Server) handleEntityAttach(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	ownerType := strings.TrimSpace(r.FormValue("owner_type"))
	// file_path names a FILE in the repository, never an entity path — the
	// distinction is in the field name so the no-typed-entity-path rule (SD-6,
	// NFR-8) can be checked absolutely, with no exemption.
	path := strings.TrimSpace(r.FormValue("file_path"))
	docType := strings.TrimSpace(r.FormValue("doc_type"))
	if docType == "" {
		docType = "design"
	}
	var ownerID *uuid.UUID
	if ownerType != "project" {
		id, err := formID(r)
		if err != nil {
			http.Error(w, "bad owner id", http.StatusBadRequest)
			return
		}
		ownerID = &id
	}
	if path == "" {
		s.renderEntity(w, r, ownerType, deref(ownerID), "", "Enter the path of the Markdown file in the repository to attach.")
		return
	}
	_, err := s.registerDocBy(r.Context(), path, docType, ownerType, ownerID, s.uiActor(),
		writerAct{Act: store.ActAdded, Kind: store.WriterPerson, Actor: s.uiActor(), Via: "ui"})
	if err != nil {
		s.renderEntity(w, r, ownerType, deref(ownerID), "", err.Error())
		return
	}
	s.renderEntity(w, r, ownerType, deref(ownerID), "Document attached: "+path, "")
}

// --- Make this the main document (FR-3.1) ---

func (s *Server) handleDocumentPrimary(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	docID, err := uuid.Parse(strings.TrimSpace(r.FormValue("doc_id")))
	if err != nil {
		http.Error(w, "bad document id", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		s.notFoundOrErr(w, r, "document", docID.String(), err)
		return
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.SetPrimaryDocument(ctx, tx, docID, s.uiActor())
	})
	// Re-render the owning entity's page — its body now shows this document.
	ownerType, ownerID := doc.OwnerType, deref(doc.OwnerID)
	if err != nil {
		s.renderEntity(w, r, ownerType, ownerID, "", err.Error())
		return
	}
	s.renderEntity(w, r, ownerType, ownerID, "This is now the main document for the page.", "")
}

// --- Estimates (FR-5.1) ---

func (s *Server) handleEntityEstimateSet(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	refType := strings.TrimSpace(r.FormValue("ref_type"))
	id, err := formID(r)
	if err != nil {
		http.Error(w, "bad entity id", http.StatusBadRequest)
		return
	}
	tokens, terr := strconv.ParseInt(strings.TrimSpace(r.FormValue("tokens")), 10, 64)
	if terr != nil || tokens <= 0 {
		s.renderEntity(w, r, refType, id, "", "Enter a positive number of tokens for the estimate.")
		return
	}
	tier := sizing.TierRough
	if r.FormValue("cite_corpus") != "" {
		tier = sizing.TierConsidered
	}
	rationale := strings.TrimSpace(r.FormValue("rationale"))
	err = s.Store.WithTx(r.Context(), func(tx pgx.Tx) error {
		_, e := store.RecordEstimate(r.Context(), tx, refType, id, tokens, tier, rationale, nil, s.uiActor())
		return e
	})
	if err != nil {
		s.renderEntity(w, r, refType, id, "", err.Error())
		return
	}
	s.renderEntity(w, r, refType, id, "Estimate set to "+humanTokens(tokens)+" tokens ("+string(tier)+").", "")
}

func (s *Server) handleEntityEstimateAI(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	refType := strings.TrimSpace(r.FormValue("ref_type"))
	id, err := formID(r)
	if err != nil {
		http.Error(w, "bad entity id", http.StatusBadRequest)
		return
	}
	if err := s.enqueueEstimate(r.Context(), refType, id); err != nil {
		s.renderEntity(w, r, refType, id, "", err.Error())
		return
	}
	s.renderEntity(w, r, refType, id, "The estimator was dispatched; the estimate will appear here when it completes.", "")
}

// --- Feature lifecycle (FR-5.1) ---

func (s *Server) handleEntityFeatureStart(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	id, err := formID(r)
	if err != nil {
		http.Error(w, "bad feature id", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	f, err := store.GetFeature(ctx, s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "feature", id.String(), err)
		return
	}
	path, err := s.featurePath(ctx, f)
	if err != nil {
		s.uiError(w, err)
		return
	}
	if _, err := s.StartFeature(ctx, path, s.uiActor()); err != nil {
		s.renderEntity(w, r, "feature", id, "", err.Error())
		return
	}
	s.renderEntity(w, r, "feature", id, "Work started on this feature.", "")
}

func (s *Server) handleEntityFeatureAbandon(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	id, err := formID(r)
	if err != nil {
		http.Error(w, "bad feature id", http.StatusBadRequest)
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if reason == "" {
		s.renderEntity(w, r, "feature", id, "", "Abandoning a feature requires a reason.")
		return
	}
	ctx := r.Context()
	f, err := store.GetFeature(ctx, s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "feature", id.String(), err)
		return
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.TransitionFeature(ctx, tx, f, lifecycle.FeatAbandon, s.uiActor(), map[string]any{"reason": reason})
	})
	if err != nil {
		s.renderEntity(w, r, "feature", id, "", err.Error())
		return
	}
	s.renderEntity(w, r, "feature", id, "This feature was abandoned.", "")
}

// --- Initiative archive (FR-5.1, gate G5) ---

func (s *Server) handleEntityInitiativeArchive(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	id, err := formID(r)
	if err != nil {
		http.Error(w, "bad initiative id", http.StatusBadRequest)
		return
	}
	ctx := r.Context()
	in, err := store.GetInitiative(ctx, s.Store.Pool, id)
	if err != nil {
		s.notFoundOrErr(w, r, "initiative", id.String(), err)
		return
	}
	n, err := store.NonTerminalFeatureCount(ctx, s.Store.Pool, in.ID)
	if err != nil {
		s.uiError(w, err)
		return
	}
	spikes, err := store.OpenSpikeIDsUnderInitiative(ctx, s.Store.Pool, in.ID)
	if err != nil {
		s.uiError(w, err)
		return
	}
	g := lifecycle.G5Open(n, spikes)
	reason := strings.TrimSpace(r.FormValue("reason"))
	actor := s.uiActor()
	var raised *store.Checkpoint
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if e := store.Audit(ctx, tx, actor, "gate.evaluated", "initiative", &in.ID,
			map[string]any{"gate": string(g.Gate), "pass": g.Pass, "reason": g.Reason}); e != nil {
			return e
		}
		if g.Pass {
			return store.ArchiveInitiative(ctx, tx, in.ID, actor, reason)
		}
		var e error
		raised, e = store.CreateCheckpoint(ctx, tx, "gate-override", "initiative", in.ID,
			"Archive blocked by gate G5: "+g.Reason+". Override?",
			map[string]any{"gate": "G5", "reason": g.Reason, "requested_by": actor})
		return e
	})
	if err != nil {
		s.renderEntity(w, r, "initiative", id, "", err.Error())
		return
	}
	if g.Pass {
		s.renderEntity(w, r, "initiative", id, "This initiative was archived.", "")
		return
	}
	s.notifyCheckpointRaised(raised)
	s.renderEntity(w, r, "initiative", id, "",
		"Archiving is blocked by gate G5 ("+g.Reason+"). A request to override it is now waiting in your Inbox.")
}

// --- Create a sub-initiative or a feature (FR-5.1) ---

func (s *Server) handleEntityInitiativeCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	name := strings.TrimSpace(r.FormValue("name"))
	// parent_id empty means a top-level initiative created from the project page.
	var parentID *uuid.UUID
	renderType, renderID := "project", uuid.Nil
	if pid := strings.TrimSpace(r.FormValue("parent_id")); pid != "" {
		id, err := uuid.Parse(pid)
		if err != nil {
			http.Error(w, "bad parent id", http.StatusBadRequest)
			return
		}
		parentID = &id
		renderType, renderID = "initiative", id
	}
	if slug == "" || name == "" {
		s.renderEntity(w, r, renderType, renderID, "", "A short slug and a name are both required to create an initiative.")
		return
	}
	in, designPath, err := s.createInitiative(r.Context(), parentID, slug, name,
		strings.TrimSpace(r.FormValue("description")), s.uiActor(), startDesignWanted(r))
	if err != nil {
		s.renderEntity(w, r, renderType, renderID, "", err.Error())
		return
	}
	s.renderEntity(w, r, renderType, renderID, createdNotice("Initiative", in.PublicID, name, designPath), "")
}

func (s *Server) handleEntityFeatureCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	initID, err := uuid.Parse(strings.TrimSpace(r.FormValue("initiative_id")))
	if err != nil {
		http.Error(w, "bad initiative id", http.StatusBadRequest)
		return
	}
	slug := strings.TrimSpace(r.FormValue("slug"))
	name := strings.TrimSpace(r.FormValue("name"))
	if slug == "" || name == "" {
		s.renderEntity(w, r, "initiative", initID, "", "A short slug and a name are both required to create a feature.")
		return
	}
	f, designPath, err := s.createFeature(r.Context(), initID, slug, name,
		strings.TrimSpace(r.FormValue("description")), s.uiActor(), startDesignWanted(r))
	if err != nil {
		s.renderEntity(w, r, "initiative", initID, "", err.Error())
		return
	}
	s.Bus.Publish(bus.FeatureCreated{FeatureID: f.ID})
	s.renderEntity(w, r, "initiative", initID, createdNotice("Feature", f.PublicID, name, designPath), "")
}

// --- Document review from the document page (FR-11, SPEC-006 FR-5 behaviour) ---

// handleEntityDocumentReview approves or requests changes on a human-gated
// review from the document's own page. For an agent-approved type it is the
// same RespondCheckpoint + CheckpointResponded path the inbox uses — a second
// surface, not a new authority (SPEC-006 SD-4). For a human-approved type
// there is no checkpoint to answer: the decision is the human's own, and it
// goes through the gated service methods directly (SPEC-009 FR-2.3). The
// document is identified by its id, carried by the page, so no path is typed.
func (s *Server) handleEntityDocumentReview(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		s.uiError(w, err)
		return
	}
	docID, err := uuid.Parse(strings.TrimSpace(r.FormValue("doc_id")))
	if err != nil {
		http.Error(w, "bad document id", http.StatusBadRequest)
		return
	}
	decision := strings.TrimSpace(r.FormValue("decision"))
	ctx := r.Context()
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		s.notFoundOrErr(w, r, "document", docID.String(), err)
		return
	}
	if decision != "approve" && decision != "request_changes" {
		s.renderDocumentPage(w, r, doc.Path, "", "Choose whether to approve the document or request changes to it.")
		return
	}
	if s.humanApprovalType(doc.Type) {
		s.handleHumanDocumentDecision(w, r, doc, decision)
		return
	}
	cp, err := s.openReviewCheckpoint(ctx, doc.ID)
	if err != nil || cp == nil {
		s.renderDocumentPage(w, r, doc.Path, "", "This document has no review waiting for a decision.")
		return
	}
	response := map[string]any{"decision": decision}
	if reason := strings.TrimSpace(r.FormValue("reason")); reason != "" {
		response["reason"] = reason
	}
	actor := s.uiActor()
	var answered *store.Checkpoint
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		answered, e = store.RespondCheckpoint(ctx, tx, cp.ID, response, actor)
		return e
	})
	if err != nil {
		s.renderDocumentPage(w, r, doc.Path, "", err.Error())
		return
	}
	// The same event the inbox respond and the CLI publish, so the orchestrator
	// resumes identically.
	s.Bus.Publish(busCheckpointResponded(answered, actor))

	notice := "The document was approved and will now advance."
	if decision == "request_changes" {
		notice = "Changes were requested; the document goes back to its author."
	}
	s.renderDocumentPage(w, r, doc.Path, notice, "")
}

// handleHumanDocumentDecision is the direct decision path for a human-approved
// document type (SPEC-009 FR-2.3). The authority gate and the audit live in
// the service methods; this only carries the form to them and reports back in
// plain words.
func (s *Server) handleHumanDocumentDecision(w http.ResponseWriter, r *http.Request, doc *store.Document, decision string) {
	ctx := r.Context()
	actor := s.uiActor()
	if decision == "approve" {
		if err := s.HumanApproveDocument(ctx, doc.ID, actor); err != nil {
			s.renderDocumentPage(w, r, doc.Path, "", err.Error())
			return
		}
		// Re-read for the path: approving a revision moves the file to the
		// canonical path the predecessor held.
		path := doc.Path
		if fresh, err := store.GetDocument(ctx, s.Store.Pool, doc.ID); err == nil {
			path = fresh.Path
		}
		// Approving a design records what we want and starts nothing (DEC-006
		// decision 1), so the notice says what does start the work.
		notice := "The document was approved. Nothing starts until someone sends its features to development."
		switch doc.Type {
		case "decision":
			notice = fmt.Sprintf("%s is accepted. Agents working on its branch of the project are told its ruling from their next dispatch.", doc.PublicID)
		case "conventions":
			notice = "The conventions are accepted. Every agent is told them from its next dispatch."
		}
		s.renderDocumentPage(w, r, path, notice, "")
		return
	}
	reason := strings.TrimSpace(r.FormValue("reason"))
	if err := s.HumanReturnDocument(ctx, doc.ID, actor, reason); err != nil {
		s.renderDocumentPage(w, r, doc.Path, "", err.Error())
		return
	}
	s.renderDocumentPage(w, r, doc.Path, "Changes were requested; the document goes back to its author with your reason attached.", "")
}

// renderDocumentPage re-renders a document's page with a notice or error.
func (s *Server) renderDocumentPage(w http.ResponseWriter, r *http.Request, path, notice, errMsg string) {
	ctx := r.Context()
	view, err := s.documentViewByPath(ctx, path, notice, errMsg)
	if err != nil {
		s.notFoundOrErr(w, r, "document", path, err)
		return
	}
	page := entityDocPage{
		docPageData: view,
		OwnerCrumb:  s.ownerCrumb(ctx, view.Document.OwnerType, view.Document.OwnerID),
	}
	page.Breadcrumbs = []crumb{page.OwnerCrumb, docCrumb(view.Document)}
	s.render(w, "page-entity-document", s.page(r.Context(), "documents", page))
}

func deref(id *uuid.UUID) uuid.UUID {
	if id == nil {
		return uuid.Nil
	}
	return *id
}
