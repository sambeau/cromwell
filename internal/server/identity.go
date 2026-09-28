package server

// Documents with identity (SPEC-015, DESIGN-010 §7). A document's ID lives in
// its own front matter, so wherever the file goes Subutai finds it again; new
// documents get a default home, one folder per initiative; existing files are
// adopted where they sit. The database is the record: the front matter is
// how a moved file is found (SD-10).

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/config"
	"subutai/internal/content"
	"subutai/internal/ident"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// watcherActor is who moves a document's row when its file moves.
const watcherActor = "git-watcher"

// --- Where documents go ---

// defaultHome is the folder Subutai creates documents in for an initiative and
// the features under it: docs/work/<INIT-ID>-<slug>, flat, so a nested
// initiative's folder sits beside its parent's (SD-14). Slugs can't be
// renamed, so the name is derived rather than stored.
func defaultHome(ctx context.Context, q store.Querier, initiativeID uuid.UUID) (string, error) {
	in, err := store.GetInitiative(ctx, q, initiativeID)
	if err != nil {
		return "", err
	}
	return filepath.Join("docs", "work", in.PublicID+"-"+in.Slug), nil
}

// freePath returns base+".md", or the first base+".<n>.md" that no file
// occupies: Subutai never writes over a file it didn't register.
func (s *Server) freePath(base string) string {
	path := base + ".md"
	for n := 2; ; n++ {
		if _, err := os.Stat(filepath.Join(s.RepoRoot, path)); os.IsNotExist(err) {
			return path
		}
		path = fmt.Sprintf("%s.%d.md", base, n)
	}
}

// archiveTarget is where a superseded document's file rests. A document with
// an ID is archived by it and its revision, docs/_superseded/FEAT-023-spec.r1.md
// (SD-9). One without, or one whose ID-named archive somehow exists already,
// keeps the older name: its basename with the row id's tail, so repeated
// revisions and same-named documents never collide there. The tail, not the
// head: ids are UUIDv7, whose leading characters are the timestamp and collide
// for rows created in the same instant (the same reason ShortID reads the tail).
func (s *Server) archiveTarget(d *store.Document) string {
	if d.PublicID != "" && d.Revision > 0 {
		p := filepath.Join("docs/_superseded", ident.ArchiveName(d.PublicID, d.Revision))
		if _, err := os.Stat(filepath.Join(s.RepoRoot, p)); os.IsNotExist(err) {
			return p
		}
	}
	ext := filepath.Ext(d.Path)
	base := strings.TrimSuffix(filepath.Base(d.Path), ext)
	id := d.ID.String()
	return filepath.Join("docs/_superseded", fmt.Sprintf("%s-%s%s", base, id[len(id)-8:], ext))
}

// --- Registering with an identity ---

// stampIdentity writes id: and revision: into a document body, or returns it
// unchanged when there is no ID to write.
func stampIdentity(body, id string, revision int) (string, error) {
	if id == "" {
		return body, nil
	}
	return content.SetIdentity(body, id, revision)
}

// registerInTx registers a file already on disk, with the content given, in
// the caller's transaction: the row, its ID if it has one, and its section
// index, all together.
func registerInTx(ctx context.Context, tx pgx.Tx, path string, raw []byte, docType, ownerType string, ownerID, supersedes *uuid.UUID, publicID string, revision int, actor string, by writerAct) (*store.Document, error) {
	parsed, perr := content.Parse(string(raw))
	title := path
	if perr == nil {
		if t := parsed.FrontMatterString("title"); t != "" {
			title = t
		}
	}
	doc, err := store.RegisterDocument(ctx, tx, docType, ownerType, ownerID, path, title, content.Hash(raw), supersedes, actor)
	if err != nil {
		return nil, err
	}
	// Who wrote it, in the same transaction (SPEC-017 FR-2.2).
	if err := recordWriter(ctx, tx, doc.ID, by); err != nil {
		return nil, err
	}
	if publicID != "" {
		if err := store.SetDocumentIdentity(ctx, tx, doc.ID, publicID, revision); err != nil {
			return nil, identityClash(err, publicID, revision)
		}
		doc.PublicID, doc.Revision = publicID, revision
	}
	if perr == nil {
		if err := store.ReplaceSections(ctx, tx, doc.ID, content.Hash(raw), parsed.Sections); err != nil {
			return nil, err
		}
	}
	return doc, nil
}

func identityClash(err error, id string, revision int) error {
	if isUniqueViolation(err) {
		return fmt.Errorf("%s revision %d is already registered; try again", id, revision)
	}
	return err
}

// --- Starter designs (FR-6) ---

// startDesign creates an initiative's or feature's design document in the
// same transaction that created the entity (SD-15, FR-6.1): the project's
// design template, named for the entity, in its initiative's folder, with its
// ID in the front matter, registered and indexed. The file is written now and
// committed by the caller once the transaction has committed; undo removes it
// if the transaction fails.
func (s *Server) startDesign(ctx context.Context, tx pgx.Tx, ownerType string, ownerID uuid.UUID, actor string) (path string, undo func(), err error) {
	undo = func() {}
	var name, ownerPath string
	var initiativeID uuid.UUID
	switch ownerType {
	case "initiative":
		in, err := store.GetInitiative(ctx, tx, ownerID)
		if err != nil {
			return "", undo, err
		}
		name, initiativeID = in.Name, in.ID
		if ownerPath, err = initiativePathIn(ctx, tx, in.ID); err != nil {
			return "", undo, err
		}
	case "feature":
		f, err := store.GetFeature(ctx, tx, ownerID)
		if err != nil {
			return "", undo, err
		}
		name, initiativeID = f.Name, f.InitiativeID
		ip, err := initiativePathIn(ctx, tx, f.InitiativeID)
		if err != nil {
			return "", undo, err
		}
		ownerPath = ip + "/" + f.Slug
	default:
		return "", undo, fmt.Errorf("only initiatives and features start with a design, not %q", ownerType)
	}
	if err := lockIdentity(ctx, tx, ownerType, &ownerID, "design"); err != nil {
		return "", undo, err
	}
	id, revision, err := store.NextDocumentIdentity(ctx, tx, ownerType, &ownerID, "design")
	if err != nil {
		return "", undo, err
	}
	home, err := defaultHome(ctx, tx, initiativeID)
	if err != nil {
		return "", undo, err
	}
	path = filepath.Join(home, id+".md")
	abs := filepath.Join(s.RepoRoot, path)
	if _, err := os.Stat(abs); err == nil {
		return "", undo, fmt.Errorf("a file already sits at %s, and Subutai won't write over it. "+
			"Move that file, or untick \"Start a design document\" and attach it instead", path)
	}
	body, err := stampIdentity(s.designStarter(name, ownerPath), id, revision)
	if err != nil {
		return "", undo, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", undo, err
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		return "", undo, err
	}
	undo = func() { _ = os.Remove(abs) }
	oid := ownerID
	doc, err := registerInTx(ctx, tx, path, []byte(body), "design", ownerType, &oid, nil, id, revision, actor,
		s.writerFor(store.ActStarted, actor, ""))
	if err != nil {
		undo()
		return "", func() {}, err
	}
	// The hash it started with, so an untouched starter can be told from one
	// a person has begun (SD-15).
	if err := store.Audit(ctx, tx, actor, "document.started", "document", &doc.ID,
		map[string]any{"path": path, "public_id": id, "hash": content.Hash([]byte(body))}); err != nil {
		undo()
		return "", func() {}, err
	}
	return path, undo, nil
}

// initiativePathIn is initiativePath, inside a transaction that may have just
// created the initiative.
func initiativePathIn(ctx context.Context, q store.Querier, id uuid.UUID) (string, error) {
	anc, err := store.InitiativeAncestors(ctx, q, id)
	if err != nil {
		return "", err
	}
	parts := make([]string, len(anc))
	for i, a := range anc {
		parts[len(anc)-1-i] = a.Slug
	}
	return strings.Join(parts, "/"), nil
}

// designStarter is the design template filled in with the entity's name and
// owner path (FR-6.2). The template's other placeholders stay for a person to
// fill in: validation won't let the draft be submitted until they have.
func (s *Server) designStarter(name, ownerPath string) string {
	raw, err := os.ReadFile(filepath.Join(s.CompartmentRoot, "templates", "design", "template.md"))
	if err != nil {
		return fmt.Sprintf("---\ntitle: %s\ntype: design\nowner: %s\n---\n\n# %s\n\n"+
			"## What this is for\n\n## The shape of it\n\n## Decisions\n",
			yamlQuote(name), yamlQuote(ownerPath), name)
	}
	lines := strings.SplitAfter(string(raw), "\n")
	inFront := false
	for i, l := range lines {
		trimmed := strings.TrimRight(l, "\r\n")
		if trimmed == "---" {
			if i == 0 {
				inFront = true
				continue
			}
			if inFront {
				inFront = false
				continue
			}
		}
		if !inFront {
			continue
		}
		ending := l[len(trimmed):]
		switch {
		case strings.HasPrefix(trimmed, "title:"):
			lines[i] = "title: " + yamlQuote(name) + ending
		case strings.HasPrefix(trimmed, "owner:"):
			lines[i] = "owner: " + yamlQuote(ownerPath) + ending
		}
	}
	return strings.ReplaceAll(strings.Join(lines, ""), "{{what is being designed}}", name)
}

// yamlQuote is a double-quoted YAML scalar.
func yamlQuote(s string) string { return strconv.Quote(s) }

// --- Adopt in place (FR-5) ---

// AdoptRequest is what a person, or the chat agent, asks adopt to do.
type AdoptRequest struct {
	Path      string
	DocType   string
	OwnerType string
	OwnerID   *uuid.UUID
	// State is draft or approved. The MCP tool only ever asks for draft
	// (SD-11).
	State lifecycle.DocumentState
	Actor string
	// Via is "ui" or "mcp", for the audit rows.
	Via string
}

// AdoptResult is what adopt did.
type AdoptResult struct {
	Doc *store.Document
	// Committed is false when the change couldn't be committed; the document
	// has its ID either way, and CommitError says why.
	Committed   bool
	CommitError string
	// MadeMain is set when the adopted design took over from an untouched
	// starter as the owner's main document (SD-15).
	MadeMain bool
}

// AdoptDocument gives an existing file an ID, a type, a lifecycle state and an
// owner, touching only its front matter, and commits that change as Subutai
// (SD-11, FR-5.1). A file that is already registered, without an ID, keeps
// its type, owner and state and gains an ID.
//
// Writing the two identity lines is the one change Subutai makes to an
// approved document or accepted decision (SD-11): the stored hash is
// re-recorded in the same transaction, so nothing reads it as drift.
func (s *Server) AdoptDocument(ctx context.Context, req AdoptRequest) (*AdoptResult, error) {
	path := filepath.ToSlash(filepath.Clean(strings.TrimSpace(req.Path)))
	path = strings.TrimPrefix(path, "./")
	if path == "" || path == "." {
		return nil, errors.New("Say which file to adopt, by its path in the repository.")
	}
	raw, err := s.readDocFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("There is no file at %s.", path)
		}
		return nil, err
	}
	if err := s.committedAndClean(path); err != nil {
		return nil, err
	}

	existing, err := store.LiveDocumentByPath(ctx, s.Store.Pool, path)
	if err != nil && err != store.ErrNotFound {
		return nil, err
	}
	if existing != nil {
		if err := s.checkAdoptable(ctx, existing); err != nil {
			return nil, err
		}
		req.DocType, req.OwnerType, req.OwnerID, req.State = existing.Type, existing.OwnerType, existing.OwnerID, existing.State
	} else if err := s.checkAdoptRequest(ctx, req); err != nil {
		return nil, err
	}

	if existing == nil && req.State == lifecycle.DocApproved {
		if err := s.checkSurfacedCaps(req.DocType, raw); err != nil {
			return nil, err
		}
	}

	abs := filepath.Join(s.RepoRoot, path)
	var doc *store.Document
	var newRaw string
	written := false
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		// Two adoptions for one owner and type at once would otherwise pick the
		// same ID (SD-7).
		if err := lockIdentity(ctx, tx, req.OwnerType, req.OwnerID, req.DocType); err != nil {
			return err
		}
		id, revision, err := s.adoptIdentity(ctx, tx, path, string(raw), req)
		if err != nil {
			return err
		}
		if newRaw, err = content.SetIdentity(string(raw), id, revision); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if err := os.WriteFile(abs, []byte(newRaw), 0o644); err != nil {
			return err
		}
		written = true
		hash := content.Hash([]byte(newRaw))
		if existing != nil {
			doc = existing
			if err := store.SetDocumentIdentity(ctx, tx, doc.ID, id, revision); err != nil {
				return identityClash(err, id, revision)
			}
			doc.PublicID, doc.Revision = id, revision
			if parsed, perr := content.Parse(newRaw); perr == nil {
				if err := store.ReplaceSections(ctx, tx, doc.ID, hash, parsed.Sections); err != nil {
					return err
				}
			} else if _, err := tx.Exec(ctx, `UPDATE documents SET content_hash = $2 WHERE id = $1`, doc.ID, hash); err != nil {
				return err
			}
			doc.ContentHash = hash
		} else {
			by := writerAct{Act: store.ActAdded, Kind: store.WriterPerson, Actor: req.Actor, Via: req.Via}
			if req.Via == "mcp" {
				by.Kind = store.WriterChat
			}
			if doc, err = registerInTx(ctx, tx, path, []byte(newRaw), req.DocType, req.OwnerType, req.OwnerID, nil, id, revision, req.Actor, by); err != nil {
				return err
			}
			if req.State == lifecycle.DocApproved {
				if err := s.recordAlreadyApproved(ctx, tx, doc, req.Actor, req.Via, "adopt"); err != nil {
					return err
				}
			}
		}
		if doc.Title == path {
			if h := firstHeading(newRaw); h != "" {
				if err := store.RefreshDocumentTitle(ctx, tx, doc.ID, h); err != nil {
					return err
				}
				doc.Title = h
			}
		}
		payload := map[string]any{"path": path, "public_id": id, "revision": revision,
			"type": doc.Type, "owner_type": doc.OwnerType, "state": string(doc.State), "via": req.Via,
			"registered_before": existing != nil}
		if doc.OwnerID != nil {
			payload["owner_id"] = doc.OwnerID.String()
		}
		return store.Audit(ctx, tx, req.Actor, "document.adopted", "document", &doc.ID, payload)
	})
	if err != nil {
		if written {
			_ = os.WriteFile(abs, raw, 0o644)
		}
		return nil, err
	}

	res := &AdoptResult{Doc: doc, Committed: true}
	if err := s.commitPath(path, fmt.Sprintf("subutai: adopt %s as %s", path, doc.PublicID)); err != nil {
		res.Committed, res.CommitError = false, err.Error()
		s.Log.Warn("could not commit an adopted document", "path", path, "err", err)
	}
	if existing == nil && doc.Type == "design" {
		res.MadeMain = s.giveWayToDesign(ctx, doc, req.Actor)
	}
	if doc.OwnerID != nil {
		s.notifyEntityChanged(doc.OwnerType, *doc.OwnerID)
	}
	s.notifyEntityChanged("document", doc.ID)
	if existing == nil && doc.State == lifecycle.DocApproved {
		s.publishAlreadyApproved(doc, req.Actor)
	}
	return res, nil
}

// checkAdoptRequest checks the type, owner and state asked for an
// unregistered file (SD-11).
func (s *Server) checkAdoptRequest(ctx context.Context, req AdoptRequest) error {
	if !ident.IsDocType(req.DocType) {
		return fmt.Errorf("%q isn't a kind of document Subutai knows. Choose one of: %s.",
			req.DocType, strings.Join(ident.DocTypes, ", "))
	}
	if req.DocType == "decision" && req.OwnerType == "feature" {
		return errors.New("A decision belongs to the project or an initiative, not to a feature.")
	}
	if req.DocType == "conventions" {
		if err := s.secondConventions(ctx, req.OwnerType); err != nil {
			return err
		}
	}
	switch req.State {
	case lifecycle.DocDraft:
	case lifecycle.DocApproved:
		if req.Via != "ui" {
			return errors.New("Only a person can adopt a file as already approved, in the web UI. Adopt it as a draft.")
		}
		if !s.approvableByAdoption(req.DocType) {
			return fmt.Errorf("A %s is adopted as a draft, then submitted and reviewed like any other: approving one can let a feature go ahead. Adopt it as a draft.", strings.ReplaceAll(req.DocType, "_", "-"))
		}
	default:
		return errors.New("A file is adopted as a draft or as already approved.")
	}
	if req.DocType == "spec" || req.DocType == "dev_plan" {
		if req.OwnerType != "feature" || req.OwnerID == nil {
			return fmt.Errorf("A %s belongs to a feature.", strings.ReplaceAll(req.DocType, "_", "-"))
		}
		if cur, err := store.CurrentDocForOwner(ctx, s.Store.Pool, req.DocType, "feature", *req.OwnerID); err == nil {
			return fmt.Errorf("This feature already has a %s, %s. A feature has one; revise that one instead, or detach it first.",
				strings.ReplaceAll(req.DocType, "_", "-"), cur.Path)
		} else if err != store.ErrNotFound {
			return err
		}
	}
	return nil
}

// approvableByAdoption is whether a type may be adopted as already approved:
// a design, a decision or the conventions, or a type with no template, which
// can't be submitted (SD-11). Decisions and conventions have templates since
// SPEC-018, but an existing record of a ruling is still adopted as it stands
// (SPEC-018 SD-3); their surfaced caps are checked instead.
func (s *Server) approvableByAdoption(docType string) bool {
	return docType == "design" || surfacedType(docType) || !s.hasTemplate(docType)
}

// hasTemplate reports whether a document type has a manifest, and so can be
// submitted and reviewed.
func (s *Server) hasTemplate(docType string) bool {
	_, err := config.LoadManifest(s.CompartmentRoot, docType)
	return err == nil
}

// recordAlreadyApproved marks a draft approved as a verdict a person already
// gave outside Subutai (SD-11, FR-5.3, FR-5.8): a document.human_verdict row,
// as a direct approval writes, then the state and approved_at.
func (s *Server) recordAlreadyApproved(ctx context.Context, tx pgx.Tx, doc *store.Document, actor, via, how string) error {
	if err := store.Audit(ctx, tx, actor, "document.human_verdict", "document", &doc.ID,
		map[string]any{"verdict": "approve", "via": via, "already_approved": how}); err != nil {
		return err
	}
	if err := store.MarkDocumentAdoptedApproved(ctx, tx, doc.ID); err != nil {
		return err
	}
	if err := store.RecordVerdict(ctx, tx, personVerdict(doc.ID, store.VerdictApprove, relayAct{Actor: actor, Via: via})); err != nil {
		return err
	}
	doc.State = lifecycle.DocApproved
	return s.acceptSurfaced(ctx, tx, doc, true)
}

// publishAlreadyApproved gives an approval by adoption an approval's
// consequences, through the same event (FR-5.3).
func (s *Server) publishAlreadyApproved(doc *store.Document, actor string) {
	s.Bus.Publish(bus.DocumentTransitioned{
		DocID: doc.ID, From: lifecycle.DocDraft, To: lifecycle.DocApproved,
		Event: lifecycle.DocApprove, Actor: actor,
	})
}

// RecordAlreadyApproved is FR-5.8: a person says an adopted draft of a type
// with no template was already approved. It is adopting it as approved, after
// the fact, and only in the web UI.
func (s *Server) RecordAlreadyApproved(ctx context.Context, docID uuid.UUID, actor string) (*store.Document, error) {
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return nil, err
	}
	if !s.canRecordAlreadyApproved(doc) {
		return nil, errors.New("Only a draft with an ID, of a kind that has no template to review it against, or an adopted decision, can be recorded as already approved.")
	}
	if raw, err := s.readDocFile(doc.Path); err == nil {
		if err := s.checkSurfacedCaps(doc.Type, raw); err != nil {
			return nil, err
		}
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return s.recordAlreadyApproved(ctx, tx, doc, actor, "ui", "recorded")
	})
	if err != nil {
		return nil, err
	}
	if doc.OwnerID != nil {
		s.notifyEntityChanged(doc.OwnerType, *doc.OwnerID)
	}
	s.publishAlreadyApproved(doc, actor)
	return doc, nil
}

// canRecordAlreadyApproved is when FR-5.8's button is offered.
func (s *Server) canRecordAlreadyApproved(d *store.Document) bool {
	if d.State != lifecycle.DocDraft || d.PublicID == "" || d.SupersedesID != nil {
		return false
	}
	if d.Type == "decision" {
		// An adopted decision, which won't have the template's headings;
		// one started from the template is submitted (SPEC-018 SD-3).
		return s.wasAdopted(d.ID)
	}
	return !s.hasTemplate(d.Type)
}

// wasAdopted reports whether a document came into Subutai by adoption.
func (s *Server) wasAdopted(docID uuid.UUID) bool {
	var n int
	_ = s.Store.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_events
		WHERE kind = 'document.adopted' AND ref_type = 'document' AND ref_id = $1`, docID).Scan(&n)
	return n > 0
}

// lockIdentity serialises choosing a document ID for one owner and type
// (SD-7), for the rest of the transaction.
func lockIdentity(ctx context.Context, tx pgx.Tx, ownerType string, ownerID *uuid.UUID, docType string) error {
	key := ownerType + ":" + docType
	if ownerID != nil {
		key += ":" + ownerID.String()
	}
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "subutai-identity:"+key)
	return err
}

// firstHeading is a document's first level-1 heading, the title of a file
// whose front matter has none (SD-11).
func firstHeading(raw string) string {
	inFence := false
	for _, line := range strings.Split(raw, "\n") {
		t := strings.TrimRight(line, "\r")
		if strings.HasPrefix(strings.TrimSpace(t), "```") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(t, "# ") {
			return strings.TrimSpace(t[2:])
		}
	}
	return ""
}

// committedAndClean refuses a file git doesn't track or that has uncommitted
// changes, so adopt's commit holds nothing but Subutai's two lines (SD-11).
func (s *Server) committedAndClean(path string) error {
	if _, err := gitIn(s.RepoRoot, "ls-files", "--error-unmatch", "--", path); err != nil {
		return fmt.Errorf("%s isn't committed yet. Commit it first, so that adopting it commits nothing but its new ID.", path)
	}
	clean, err := s.fileIsClean(path)
	if err != nil {
		return err
	}
	if !clean {
		return fmt.Errorf("%s has changes that aren't committed. Commit them first, so that adopting it commits nothing but its new ID.", path)
	}
	return nil
}

// fileIsClean reports whether a tracked file has no uncommitted changes.
func (s *Server) fileIsClean(path string) (bool, error) {
	out, err := gitIn(s.RepoRoot, "status", "--porcelain", "--", path)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) == "", nil
}

// checkAdoptable refuses a registered document adopt can't give an ID to.
func (s *Server) checkAdoptable(ctx context.Context, d *store.Document) error {
	if d.PublicID != "" {
		return fmt.Errorf("%s already has an ID: it is %s.", d.Path, d.PublicID)
	}
	if d.State == lifecycle.DocReviewing {
		return fmt.Errorf("%s is being reviewed. Changing it now would make the review's verdict stale, so give it an ID once the review is over.", d.Path)
	}
	if succ, err := store.LiveSuccessorOf(ctx, s.Store.Pool, d.ID); err == nil {
		return fmt.Errorf("%s has a revision open, at %s. Give it an ID once that revision is approved or detached, so the two can't end up with different IDs.", d.Path, succ.Path)
	} else if err != store.ErrNotFound {
		return err
	}
	if d.SupersedesID != nil {
		pred, err := store.GetDocument(ctx, s.Store.Pool, *d.SupersedesID)
		if err != nil {
			return err
		}
		if pred.State != lifecycle.DocSuperseded {
			return fmt.Errorf("%s is an open revision of %s. Give it an ID once this revision is approved.", d.Path, pred.Path)
		}
	}
	return s.refuseIfAuthorAtWork(ctx, d)
}

// adoptIdentity is the ID and revision an adopted file takes (FR-5.4, FR-5.5):
// one it already carries if that fits its owner and type, otherwise a new one.
// The revision is 1, or one more than a kept ID has had; the file's own
// revision: isn't trusted.
func (s *Server) adoptIdentity(ctx context.Context, tx pgx.Tx, path, raw string, req AdoptRequest) (string, int, error) {
	declared := content.ReadDeclaredID(raw)

	if req.DocType == "decision" {
		candidate := ""
		if declared != "" {
			r, ok := ident.Parse(declared)
			if !ok || r.Shape != ident.ShapeDecision || r.Revision != 0 {
				return "", 0, fmt.Errorf("%s says its ID is %q, which isn't a decision number such as DEC-005. Correct or remove its id: line, then adopt it.", path, declared)
			}
			candidate = r.ID
		} else {
			candidate = ident.DecisionFromFileName(filepath.Base(path))
		}
		if candidate == "" {
			id, err := store.MintDecisionID(ctx, tx)
			return id, 1, err
		}
		maxRev, live, err := store.IdentityUse(ctx, tx, candidate)
		if err != nil {
			return "", 0, err
		}
		if live {
			if declared == "" {
				return "", 0, nameTaken(ctx, tx, path, candidate)
			}
			return "", 0, alreadyTaken(ctx, tx, path, candidate)
		}
		// Every revision superseded means another decision superseded it:
		// the file is a record, and adopting it again would bring a retired
		// ruling back into every prompt (SPEC-018 R18-1).
		if maxRev > 0 {
			by := s.supersededByName(ctx, &store.Document{PublicID: candidate})
			if by == "" {
				by = "another decision"
			}
			return "", 0, fmt.Errorf("%s was superseded by %s and is kept as a record, so %s can't be adopted as %s again. Start a new decision instead.", candidate, by, path, candidate)
		}
		r, _ := ident.Parse(candidate)
		if err := store.AdvanceIdent(ctx, tx, "DEC", r.Number); err != nil {
			return "", 0, err
		}
		return candidate, maxRev + 1, nil
	}

	owner, err := store.OwnerPublicID(ctx, tx, req.OwnerType, req.OwnerID)
	if err != nil {
		return "", 0, err
	}
	fits := func(id string) (ident.Ref, bool) {
		r, ok := ident.Parse(id)
		return r, ok && r.Shape == ident.ShapeDocument && r.Revision == 0 && r.Entity == owner && r.DocType == req.DocType
	}
	candidate := ""
	if declared != "" {
		r, ok := fits(declared)
		if !ok {
			return "", 0, fmt.Errorf("%s says its ID is %q, which doesn't fit a %s of %s. Correct or remove its id: line, then adopt it.",
				path, declared, ident.TypeSlug(req.DocType), owner)
		}
		candidate = r.ID
	} else if r, ok := fits(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))); ok {
		candidate = r.ID
	}
	if candidate != "" {
		maxRev, live, err := store.IdentityUse(ctx, tx, candidate)
		if err != nil {
			return "", 0, err
		}
		if !live {
			return candidate, maxRev + 1, nil
		}
		if declared != "" {
			return "", 0, alreadyTaken(ctx, tx, path, candidate)
		}
	}
	return store.NextDocumentIdentity(ctx, tx, req.OwnerType, req.OwnerID, req.DocType)
}

// nameTaken refuses a decision whose file name claims a number another
// document has: minting a different number for a file named DEC-005-… would
// leave its name contradicting its ID.
func nameTaken(ctx context.Context, q store.Querier, path, id string) error {
	where := ""
	if other, err := store.CurrentDocumentByPublicID(ctx, q, id); err == nil {
		where = ", at " + other.Path
	}
	return fmt.Errorf("%s is named as %s, but %s is already registered%s. Rename the file, then adopt it.", path, id, id, where)
}

func alreadyTaken(ctx context.Context, q store.Querier, path, id string) error {
	where := ""
	if other, err := store.CurrentDocumentByPublicID(ctx, q, id); err == nil {
		where = ", at " + other.Path
	}
	return fmt.Errorf("%s says its ID is %s, but another document already has that ID%s. Correct or remove its id: line, then adopt it.", path, id, where)
}

// --- An untouched starter gives way (SD-15) ---

// isUntouchedStarter reports whether a document is a starter design nobody
// has changed: still a draft, and its file's hash the one Subutai recorded
// when it wrote it.
func (s *Server) isUntouchedStarter(ctx context.Context, d *store.Document) bool {
	if d.Type != "design" || d.State != lifecycle.DocDraft {
		return false
	}
	var hash string
	err := s.Store.Pool.QueryRow(ctx, `SELECT payload->>'hash' FROM audit_events
		WHERE kind = 'document.started' AND ref_id = $1 ORDER BY occurred_at DESC LIMIT 1`, d.ID).Scan(&hash)
	if err != nil || hash == "" {
		return false
	}
	raw, err := s.readDocFile(d.Path)
	return err == nil && content.Hash(raw) == hash
}

// giveWayToDesign marks a newly attached or adopted design as its owner's
// main document when the page would otherwise show an untouched starter
// (SD-15, FR-6.6). It reports whether it did.
func (s *Server) giveWayToDesign(ctx context.Context, d *store.Document, actor string) bool {
	if d.Type != "design" || d.IsPrimary {
		return false
	}
	body, err := store.PrimaryDocForOwner(ctx, s.Store.Pool, d.OwnerType, d.OwnerID)
	if err != nil || body.ID == d.ID || !s.isUntouchedStarter(ctx, body) {
		return false
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.SetPrimaryDocument(ctx, tx, d.ID, actor)
	})
	if err != nil {
		s.Log.Warn("could not make the new design the main document", "doc", d.ID, "err", err)
		return false
	}
	d.IsPrimary = true
	return true
}

// --- The watcher follows a moved document (FR-4) ---

// followMovedDocument applies SD-10 to one path a commit touched: if the file
// there carries the ID and revision of a registered document at another path,
// and that isn't a copy, the document's row moves to it.
func (s *Server) followMovedDocument(ctx context.Context, path string) {
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return
	}
	raw, err := s.readDocFile(path)
	if err != nil {
		return
	}
	id, revision, ok := content.ReadIdentity(string(raw))
	if !ok {
		return
	}
	s.followTo(ctx, path, id, revision)
}

func (s *Server) followTo(ctx context.Context, path, id string, revision int) {
	// Rule 1: a registered path is never taken over.
	var taken bool
	if err := s.Store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM documents WHERE path = $1)`, path).Scan(&taken); err != nil || taken {
		return
	}
	doc, err := store.DocumentByIdentity(ctx, s.Store.Pool, id, revision)
	if err != nil || doc.Path == path {
		return
	}
	// Rule 2: a copy isn't a move.
	if old, err := s.readDocFile(doc.Path); err == nil {
		if oid, orev, ok := content.ReadIdentity(string(old)); ok && oid == id && orev == revision {
			s.recordCopy(ctx, doc, path)
			return
		}
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.MoveDocument(ctx, tx, doc, path, watcherActor)
	})
	if err != nil {
		s.Log.Warn("could not follow a moved document", "id", id, "from", doc.Path, "to", path, "err", err)
		return
	}
	s.Log.Info("followed a moved document", "id", id, "from", doc.Path, "to", path)
	if doc.OwnerID != nil {
		s.notifyEntityChanged(doc.OwnerType, *doc.OwnerID)
	}
	s.notifyEntityChanged("document", doc.ID)
}

// recordCopy notes, once per path, that a file claims a registered document's
// ID while the document is still where it was.
func (s *Server) recordCopy(ctx context.Context, doc *store.Document, path string) {
	var seen bool
	if err := s.Store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM audit_events
		WHERE kind = 'document.copy_ignored' AND ref_id = $1 AND payload->>'path' = $2)`,
		doc.ID, path).Scan(&seen); err != nil || seen {
		return
	}
	_ = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.Audit(ctx, tx, watcherActor, "document.copy_ignored", "document", &doc.ID,
			map[string]any{"path": path, "registered_path": doc.Path, "public_id": doc.PublicID})
	})
}

// findMovedDocuments is the catch-up half of FR-4.3: documents with an ID
// whose file isn't at their path are looked for, once, among the tracked
// Markdown files, which are read only if something is missing.
//
// It runs at boot and on every heartbeat, so a move that arrived with no
// post-commit hook (a pull, a merge, a rebase) is followed too. A document
// that is still missing is looked for again only once HEAD has moved, so a
// deleted file doesn't cost a scan every heartbeat.
func (s *Server) findMovedDocuments(ctx context.Context) {
	docs, err := store.IdentifiedDocuments(ctx, s.Store.Pool)
	if err != nil {
		s.Log.Warn("catch-up: identified documents", "err", err)
		return
	}
	type key struct {
		id  string
		rev int
	}
	missing := map[key]bool{}
	var names []string
	for _, d := range docs {
		if _, err := os.Stat(filepath.Join(s.RepoRoot, d.Path)); os.IsNotExist(err) {
			missing[key{d.PublicID, d.Revision}] = true
			names = append(names, d.PublicID+".r"+strconv.Itoa(d.Revision))
		}
	}
	if len(missing) == 0 {
		return
	}
	head, _ := gitIn(s.RepoRoot, "rev-parse", "HEAD")
	scanKey := strings.TrimSpace(head) + " " + strings.Join(names, ",")
	s.moveScanMu.Lock()
	seen := s.lastMoveScan == scanKey
	s.lastMoveScan = scanKey
	s.moveScanMu.Unlock()
	if seen {
		return
	}
	out, err := gitIn(s.RepoRoot, "ls-files", "-z", "--", "*.md")
	if err != nil {
		s.Log.Warn("catch-up: list tracked files", "err", err)
		return
	}
	for _, p := range strings.Split(out, "\x00") {
		if p == "" {
			continue
		}
		raw, err := s.readDocFile(p)
		if err != nil {
			continue
		}
		id, rev, ok := content.ReadIdentity(string(raw))
		if ok && missing[key{id, rev}] {
			s.followTo(ctx, p, id, rev)
		}
	}
}

// --- Detach takes the ID out of the file (SD-13) ---

// stripDetachedIdentity removes a detached document's id: and revision: lines
// from its file, leaving the change uncommitted like the detach itself, so the
// file stops claiming an ID that now belongs to nobody.
//
// If the file had no other uncommitted changes, the removal is committed, so
// the checkout stays clean; otherwise it is left with the person's own
// changes. It reports which: "committed", "uncommitted", or "" when there was
// no ID to take out.
func (s *Server) stripDetachedIdentity(d *store.Document) string {
	if d.PublicID == "" {
		return ""
	}
	abs := filepath.Join(s.RepoRoot, d.Path)
	raw, err := os.ReadFile(abs)
	if err != nil {
		return ""
	}
	stripped, err := content.StripIdentity(string(raw))
	if err != nil || stripped == string(raw) {
		return ""
	}
	_, trackErr := gitIn(s.RepoRoot, "ls-files", "--error-unmatch", "--", d.Path)
	clean, _ := s.fileIsClean(d.Path)
	if err := os.WriteFile(abs, []byte(stripped), 0o644); err != nil {
		s.Log.Warn("could not take the ID out of a detached document", "path", d.Path, "err", err)
		return ""
	}
	if trackErr == nil && clean {
		if err := s.commitPath(d.Path, fmt.Sprintf("subutai: %s detached; its ID %s taken out", d.Path, d.PublicID)); err == nil {
			return "committed"
		}
	}
	return "uncommitted"
}

// --- Creating work creates its documents (FR-6) ---

// createInitiative creates an initiative and, unless opted out, its starter
// design (SD-15): one transaction for both rows, then one commit for the file.
// The web UI and the MCP tool share it; the HTTP API doesn't (SD-15).
func (s *Server) createInitiative(ctx context.Context, parentID *uuid.UUID, slug, name, description, actor string, withDesign bool) (*store.Initiative, string, error) {
	var in *store.Initiative
	var path string
	undo := func() {}
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		if in, err = store.CreateInitiative(ctx, tx, parentID, slug, name, description, actor); err != nil {
			return err
		}
		if withDesign {
			path, undo, err = s.startDesign(ctx, tx, "initiative", in.ID, actor)
		}
		return err
	})
	if err != nil {
		undo()
		return nil, "", err
	}
	if path != "" {
		s.commitDocument(path, fmt.Sprintf("subutai: start the design of %s, %s", in.PublicID, in.Name))
	}
	return in, path, nil
}

// createFeature is createInitiative for a feature. The caller publishes
// FeatureCreated, as it always has.
func (s *Server) createFeature(ctx context.Context, initiativeID uuid.UUID, slug, name, description, actor string, withDesign bool) (*store.Feature, string, error) {
	var f *store.Feature
	var path string
	undo := func() {}
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		if f, err = store.CreateFeature(ctx, tx, initiativeID, slug, name, description, actor); err != nil {
			return err
		}
		if withDesign {
			path, undo, err = s.startDesign(ctx, tx, "feature", f.ID, actor)
		}
		return err
	})
	if err != nil {
		undo()
		return nil, "", err
	}
	if path != "" {
		s.commitDocument(path, fmt.Sprintf("subutai: start the design of %s, %s", f.PublicID, f.Name))
	}
	return f, path, nil
}

// --- Entities by path or ID (FR-7.5) ---

// initiativeByRef resolves an initiative from its slug path ("auth/basic") or
// its ID ("INIT-014"), returning its path either way.
func (s *Server) initiativeByRef(ctx context.Context, ref string) (*store.Initiative, string, error) {
	// An ID is recognised as it is shown, in capitals: slugs are lower-case,
	// so an initiative slugged "init-001" is never mistaken for one.
	if r, ok := ident.Parse(ref); ok && r.Shape == ident.ShapeEntity && r.Kind.Name == "initiative" && ref == r.ID {
		in, err := store.InitiativeByPublicID(ctx, s.Store.Pool, r.ID)
		if err != nil {
			return nil, ref, err
		}
		path, err := s.initiativePath(ctx, in.ID)
		return in, path, err
	}
	in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(ref, "/"))
	return in, ref, err
}

// featureByRef resolves a feature from its path ("auth/login") or its ID
// ("FEAT-023"), returning its path either way.
func (s *Server) featureByRef(ctx context.Context, ref string) (*store.Feature, string, error) {
	if r, ok := ident.Parse(ref); ok && r.Shape == ident.ShapeEntity && r.Kind.Name == "feature" && ref == r.ID {
		f, err := store.FeatureByPublicID(ctx, s.Store.Pool, r.ID)
		if err != nil {
			return nil, ref, err
		}
		path, err := s.featurePath(ctx, f)
		return f, path, err
	}
	f, err := s.featureByPath(ctx, ref)
	return f, ref, err
}
