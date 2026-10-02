package server

// Decisions and conventions (SPEC-018): validation of a decision's revision
// and of what it supersedes, what happens when one is accepted, the surfaced
// block every dispatch receives, and creating decisions and the conventions
// document.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/config"
	"subutai/internal/content"
	"subutai/internal/ident"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// surfacedTypes are the document types whose accepted text is pushed into
// prompts rather than attached whole (SD-11).
func surfacedType(docType string) bool { return docType == "decision" || docType == "conventions" }

// ---- Validation (FR-3.3, FR-3.6, FR-4) ----

// validateDocument is the one validation every route uses: Validate, Submit
// and the reviewer's report. A decision that revises an accepted one is held
// to the append-only check instead of the template (FR-3.3); a new decision's
// supersedes: list is checked against what it may supersede (FR-3.6).
func (s *Server) validateDocument(ctx context.Context, doc *store.Document, manifest *config.Manifest, raw []byte) lifecycle.Report {
	switch doc.Type {
	case "decision":
	case "conventions":
		return s.validateConventions(manifest, doc, raw)
	default:
		return lifecycle.Validate(manifest, string(raw), s.linkChecker(doc.Path))
	}
	var report lifecycle.Report
	if doc.SupersedesID != nil {
		report = s.validateDecisionRevision(ctx, doc, manifest, raw)
	} else {
		report = lifecycle.Validate(manifest, string(raw), s.linkChecker(doc.Path))
	}
	// Everything about a decision names it by its number (R18-8).
	if doc.PublicID == "" {
		report.Fail("identity", "This decision has no number yet. Give it its number with Give it an ID, then submit it.")
	}
	if doc.SupersedesID != nil {
		return report
	}
	if parsed, err := content.Parse(string(raw)); err == nil {
		for _, why := range s.supersessionProblems(ctx, doc.OwnerType, doc.OwnerID, parsed.FrontMatterList("supersedes")) {
			report.Fail("supersedes", "%s", why)
		}
	}
	return report
}

// validateConventions is the template's checks plus one against the
// configured cap (R18-11): the conventions always go into the block, so they
// may use at most half of a dispatch's cap, leaving the rest for decisions.
func (s *Server) validateConventions(m *config.Manifest, doc *store.Document, raw []byte) lifecycle.Report {
	report := lifecycle.Validate(m, string(raw), s.linkChecker(doc.Path))
	cfg, err := s.freshConfig()
	if err != nil {
		return report
	}
	if parsed, err := content.Parse(string(raw)); err == nil {
		body := lifecycle.SurfacedBody(parsed.Body)
		if n, max := content.EstimateTokens(body), cfg.SurfacingMaxTokens()/2; n > max {
			report.Fail("rule:max_tokens", "The conventions come to about %d tokens, and they can use at most %d, half of a dispatch's cap of %d, so that decisions still fit. Shorten them.", n, max, cfg.SurfacingMaxTokens())
		}
	}
	return report
}

func (s *Server) validateDecisionRevision(ctx context.Context, doc *store.Document, manifest *config.Manifest, raw []byte) lifecycle.Report {
	var report lifecycle.Report
	prev, err := store.GetDocument(ctx, s.Store.Pool, *doc.SupersedesID)
	if err != nil {
		report.Fail("amendment", "The accepted decision this revises can't be found.")
		return report
	}
	label := revisionLabel(prev)
	if prev.State != lifecycle.DocApproved {
		if by := s.supersededByName(ctx, prev); by != "" {
			report.Fail("amendment", "%s was superseded by %s, so it can't be amended. Detach this draft.", prev.PublicID, by)
		} else {
			report.Fail("amendment", "%s is no longer accepted, so it can't be amended. Detach this draft.", label)
		}
		return report
	}
	prevRaw, err := s.readDocFile(prev.Path)
	if err != nil {
		report.Fail("amendment", "The accepted text of %s can't be read at %s, so an amendment can't be checked against it. Restore the file in git first.", label, prev.Path)
		return report
	}
	if content.Hash(prevRaw) != prev.ContentHash {
		report.Fail("amendment", "The accepted text of %s has changed on disk since it was accepted, so an amendment can't be checked against it. Restore %s in git first.", label, prev.Path)
		return report
	}
	return lifecycle.ValidateDecisionRevision(manifest, label, string(prevRaw), string(raw), s.linkChecker(doc.Path))
}

func revisionLabel(d *store.Document) string {
	if d.PublicID == "" {
		return d.Path
	}
	return fmt.Sprintf("%s revision %d", d.PublicID, d.Revision)
}

// supersessionProblems says, one sentence each, why a decision owned by the
// given owner can't supersede the IDs named (FR-3.6): each must be a decision
// accepted now, owned by the same owner or one below it.
func (s *Server) supersessionProblems(ctx context.Context, ownerType string, ownerID *uuid.UUID, ids []string) []string {
	var out []string
	for _, raw := range ids {
		r, ok := ident.Parse(strings.ToUpper(strings.TrimSpace(raw)))
		if !ok || r.Shape != ident.ShapeDecision {
			out = append(out, fmt.Sprintf("%q in supersedes isn't a decision number such as DEC-005.", raw))
			continue
		}
		cur, err := store.CurrentDocumentByPublicID(ctx, s.Store.Pool, r.ID)
		if err != nil || cur.Type != "decision" || cur.State != lifecycle.DocApproved {
			out = append(out, fmt.Sprintf("%s isn't an accepted decision, so this can't supersede it.", r.ID))
			continue
		}
		if !s.ownerCovers(ctx, ownerType, ownerID, cur.OwnerType, cur.OwnerID) {
			out = append(out, fmt.Sprintf("%s belongs to %s, and a decision of %s can't supersede it: that would change the rules for everything under %s. Make this decision belong to %s or above, or leave %s out.",
				r.ID, s.ownerName(ctx, cur.OwnerType, cur.OwnerID), s.ownerName(ctx, ownerType, ownerID),
				s.ownerName(ctx, cur.OwnerType, cur.OwnerID), s.ownerName(ctx, cur.OwnerType, cur.OwnerID), r.ID))
		}
	}
	return out
}

// ownerCovers reports whether owner A is the same as owner B or above it:
// the project covers everything; an initiative covers itself and the
// initiatives below it.
func (s *Server) ownerCovers(ctx context.Context, aType string, aID *uuid.UUID, bType string, bID *uuid.UUID) bool {
	if aType == "project" {
		return true
	}
	if bType == "project" || aID == nil || bID == nil {
		return false
	}
	anc, err := store.InitiativeAncestors(ctx, s.Store.Pool, *bID)
	if err != nil {
		return false
	}
	for _, a := range anc {
		if a.ID == *aID {
			return true
		}
	}
	return false
}

// ownerName is an owner as a person reads it: "the project" or "INIT-004 Auth".
func (s *Server) ownerName(ctx context.Context, ownerType string, ownerID *uuid.UUID) string {
	if ownerType == "project" || ownerID == nil {
		return "the project"
	}
	if in, err := store.GetInitiative(ctx, s.Store.Pool, *ownerID); err == nil {
		return withID(in.PublicID, in.Name)
	}
	return ownerType
}

// checkSurfacedCaps is FR-4.3: the routes to accepted that skip Submit still
// check the surfaced fields' caps, read from the project's own manifest.
func (s *Server) checkSurfacedCaps(docType string, raw []byte) error {
	if !surfacedType(docType) {
		return nil
	}
	m, err := config.LoadManifest(s.CompartmentRoot, docType)
	if err != nil {
		return nil // no template: nothing to hold it to
	}
	parsed, err := content.Parse(string(raw))
	if err != nil {
		if docType == "conventions" {
			return fmt.Errorf("This conventions document can't be read: %v", err)
		}
		return nil // a decision with no front matter is surfaced by its title
	}
	var report lifecycle.Report
	report.Valid = true
	lifecycle.CheckSurfacedCaps(&report, m, parsed)
	if report.Valid {
		return nil
	}
	return fmt.Errorf("It can't be recorded as accepted: %s Adopt it as a draft and shorten it first.", report.String())
}

// ---- Acceptance (FR-3.5, SD-16) ----

// onAccepted runs in the transaction that accepts a document through
// review: a person's approval, directly or relayed (SD-16). It re-checks what
// is being accepted, records what will be surfaced, and a new decision
// supersedes what it names.
func (s *Server) onAccepted(ctx context.Context, tx pgx.Tx, doc *store.Document) error {
	return s.acceptSurfaced(ctx, tx, doc, false)
}

// acceptSurfaced is onAccepted. adopted marks the routes that record a verdict
// given outside Subutai (adoption as approved, "This was already approved"),
// which check only the surfaced caps, not the template (SD-3, FR-4.3).
func (s *Server) acceptSurfaced(ctx context.Context, tx pgx.Tx, doc *store.Document, adopted bool) error {
	if !surfacedType(doc.Type) {
		return nil
	}
	if doc.Type == "decision" && doc.PublicID == "" {
		return errors.New("This decision has no number, so agents couldn't be told which decision it is. Give it its number with Give it an ID, then accept it.")
	}
	raw, err := s.readDocFile(doc.Path)
	if err != nil {
		return fmt.Errorf("The file for this document can't be read at %s, so what agents would be told can't be recorded. Restore it in git first.", doc.Path)
	}
	// What is accepted must be what was checked (R18-7): the text submitted,
	// still passing the checks, including the caps.
	if !adopted {
		if content.Hash(raw) != doc.ContentHash {
			return fmt.Errorf("%s changed on disk since it was submitted, so it can't be accepted as it stands: agents would be told text nobody checked. Submit it again.", doc.Path)
		}
		if m, err := config.LoadManifest(s.CompartmentRoot, doc.Type); err == nil {
			if report := s.validateDocument(ctx, doc, m, raw); !report.Valid {
				return fmt.Errorf("This can't be accepted as it stands: %s", report.String())
			}
		}
	} else if err := s.checkSurfacedCaps(doc.Type, raw); err != nil {
		return err
	}

	if doc.Type == "decision" {
		if err := store.LockDecisionID(ctx, tx, doc.PublicID); err != nil {
			return err
		}
		// An amendment's decision may have been superseded while it waited.
		if doc.SupersedesID != nil {
			st, err := store.DocumentStateForUpdate(ctx, tx, *doc.SupersedesID)
			if err != nil {
				return err
			}
			if st != string(lifecycle.DocApproved) {
				prev := &store.Document{PublicID: doc.PublicID}
				if by := s.supersededByName(ctx, prev); by != "" {
					return fmt.Errorf("%s was superseded by %s while this amendment waited, so it can't be amended. Detach this draft; its working copy can then be deleted.", doc.PublicID, by)
				}
				return fmt.Errorf("%s is no longer accepted, so this amendment can't be accepted.", doc.PublicID)
			}
		}
	}
	if err := store.RecordSurfacedText(ctx, tx, surfacedTextOf(doc, raw)); err != nil {
		return err
	}
	if doc.Type != "decision" || doc.SupersedesID != nil {
		return nil // an amendment can't change supersedes (FR-3.3)
	}
	parsed, err := content.Parse(string(raw))
	if err != nil {
		return nil
	}
	for _, rawID := range parsed.FrontMatterList("supersedes") {
		r, ok := ident.Parse(strings.ToUpper(strings.TrimSpace(rawID)))
		if !ok || r.Shape != ident.ShapeDecision || r.ID == doc.PublicID {
			continue
		}
		if err := store.LockDecisionID(ctx, tx, r.ID); err != nil {
			return err
		}
		old, err := store.AcceptedDecisionForUpdate(ctx, tx, r.ID)
		if err == store.ErrNotFound {
			why := r.ID + " is no longer an accepted decision"
			if by := s.supersededByName(ctx, &store.Document{PublicID: r.ID}); by != "" {
				why = r.ID + " was already superseded by " + by
			}
			return fmt.Errorf("%s, so accepting this would supersede nothing. Take %s out of supersedes, and submit it again.", why, r.ID)
		}
		if err != nil {
			return err
		}
		if err := store.TransitionDocument(ctx, tx, old, lifecycle.DocSupersede, "lifecycle-engine",
			map[string]any{"superseded_by": doc.ID.String(), "superseded_by_id": doc.PublicID}); err != nil {
			return err
		}
		if err := store.RecordSupersession(ctx, tx, old.ID, doc.ID); err != nil {
			return err
		}
		// An amendment of it that is in review can no longer be accepted;
		// it goes back to draft, where Submit explains why (FR-3.5.3).
		if succ, err := store.LiveSuccessorOf(ctx, tx, old.ID); err == nil && succ.State == lifecycle.DocReviewing {
			if err := store.TransitionDocument(ctx, tx, succ, lifecycle.DocWithdraw, "lifecycle-engine",
				map[string]any{"why": "the decision it amends was superseded by " + doc.PublicID}); err != nil {
				return err
			}
		}
	}
	return nil
}

// surfacedTextOf is what a document tells agents (SD-1, SD-13): a decision's
// ruling and reason, or its title when it records no ruling; the conventions'
// body below its title.
func surfacedTextOf(doc *store.Document, raw []byte) store.SurfacedText {
	st := store.SurfacedText{DocumentID: doc.ID}
	parsed, err := content.Parse(string(raw))
	if doc.Type == "conventions" {
		if err == nil {
			st.Ruling = lifecycle.SurfacedBody(parsed.Body)
		}
		return st
	}
	if err == nil {
		st.Ruling = strings.TrimSpace(parsed.FrontMatterString("ruling"))
		st.Reason = strings.TrimSpace(parsed.FrontMatterString("reason"))
	}
	if st.Ruling == "" {
		title := doc.Title
		if title == "" || title == doc.Path {
			if h := firstHeading(string(raw)); h != "" {
				title = h
			}
		}
		st.Ruling, st.Reason, st.FromTitle = title, "", true
	}
	return st
}

// supersededByName is the ID of the decision that superseded d, or "".
func (s *Server) supersededByName(ctx context.Context, d *store.Document) string {
	var id string
	_ = s.Store.Pool.QueryRow(ctx, `
		SELECT COALESCE(b.public_id, '') FROM decision_supersessions x
		JOIN documents b ON b.id = x.superseding_id
		JOIN documents a ON a.id = x.superseded_id
		WHERE a.public_id = $1 ORDER BY x.created_at DESC LIMIT 1`, d.PublicID).Scan(&id)
	return id
}

// BackfillSurfaced records the surfaced text of decisions and conventions
// accepted before migration 0012 (SD-14). A file whose hash has moved since it
// was recorded is left out and logged: its integrity question already exists,
// and an unapproved edit must never reach a prompt.
func (s *Server) BackfillSurfaced(ctx context.Context) error {
	docs, err := store.AcceptedWithoutSurfacedText(ctx, s.Store.Pool)
	if err != nil {
		return err
	}
	for i := range docs {
		d := &docs[i]
		raw, err := s.readDocFile(d.Path)
		if err != nil || content.Hash(raw) != d.ContentHash {
			s.Log.Warn("an accepted decision's file has changed or is missing, so it isn't surfaced until that is resolved", "path", d.Path)
			continue
		}
		if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.RecordSurfacedText(ctx, tx, surfacedTextOf(d, raw))
		}); err != nil {
			return err
		}
	}
	return nil
}

// ---- Surfacing (FR-6) ----

// surfaceScope is where a dispatch sits in the tree: its initiative (nil for
// the project), and a decision it must not be told about (the one under
// review).
type surfaceScope struct {
	InitiativeID *uuid.UUID
	Exclude      string
}

// scopeForFeature is a feature's branch.
func (s *Server) scopeForFeature(ctx context.Context, featureID uuid.UUID) surfaceScope {
	f, err := store.GetFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return surfaceScope{}
	}
	id := f.InitiativeID
	return surfaceScope{InitiativeID: &id}
}

// scopeForDocument is the branch of a document's owner. A decision under
// review is excluded from its own prompt (SD-9a).
func (s *Server) scopeForDocument(ctx context.Context, doc *store.Document) surfaceScope {
	sc := surfaceScope{}
	switch {
	case doc.OwnerType == "feature" && doc.OwnerID != nil:
		sc = s.scopeForFeature(ctx, *doc.OwnerID)
	case doc.OwnerType == "initiative" && doc.OwnerID != nil:
		id := *doc.OwnerID
		sc.InitiativeID = &id
	case doc.OwnerType == "spike" && doc.OwnerID != nil:
		// A spike's findings are on its initiative's branch (SPEC-021
		// Appendix A).
		if sp, err := store.GetSpike(ctx, s.Store.Pool, *doc.OwnerID); err == nil {
			id := sp.InitiativeID
			sc.InitiativeID = &id
		}
	}
	if doc.Type == "decision" {
		sc.Exclude = doc.PublicID
	}
	return sc
}

// surfaced builds the block for a scope (FR-6.1 to FR-6.4). Errors reading the
// rows are returned: a dispatch that can't be told the project's decisions
// shouldn't run as if there were none.
func (s *Server) surfaced(ctx context.Context, sc surfaceScope) (content.SurfaceResult, error) {
	cfg, err := s.freshConfig()
	if err != nil {
		return content.SurfaceResult{}, err
	}
	in, err := s.surfaceInput(ctx, sc)
	if err != nil {
		return content.SurfaceResult{}, err
	}
	in.MaxTokens = cfg.SurfacingMaxTokens()
	in.MaxDecisions = cfg.SurfacingMaxDecisions()
	return content.SurfacedBlock(in), nil
}

// surfacedBlock is the block's text, "" when the project has nothing
// accepted.
func (s *Server) surfacedBlock(ctx context.Context, sc surfaceScope) (string, error) {
	r, err := s.surfaced(ctx, sc)
	return r.Block, err
}

func (s *Server) surfaceInput(ctx context.Context, sc surfaceScope) (content.SurfaceInput, error) {
	var in content.SurfaceInput
	var branch []store.Initiative // nearest first
	if sc.InitiativeID != nil {
		var err error
		if branch, err = store.InitiativeAncestors(ctx, s.Store.Pool, *sc.InitiativeID); err != nil {
			return in, err
		}
	}
	ids := make([]uuid.UUID, len(branch))
	for i, b := range branch {
		ids[i] = b.ID
	}
	decisions, err := store.AcceptedDecisions(ctx, s.Store.Pool, ids)
	if err != nil {
		return in, err
	}
	if _, conv, err := store.AcceptedConventions(ctx, s.Store.Pool); err == nil && conv != nil {
		in.Conventions = conv.Ruling
	} else if err != nil && err != store.ErrNotFound {
		return in, err
	}

	// Owners from the project outwards-in: project, then the outermost
	// initiative down to the nearest.
	owners := []content.SurfaceOwner{{}}
	index := map[uuid.UUID]int{}
	for i := len(branch) - 1; i >= 0; i-- {
		index[branch[i].ID] = len(owners)
		owners = append(owners, content.SurfaceOwner{Label: withID(branch[i].PublicID, branch[i].Name)})
	}
	for _, d := range decisions {
		if d.PublicID == "" || d.PublicID == sc.Exclude {
			continue
		}
		oi := 0
		if d.OwnerType == "initiative" && d.OwnerID != nil {
			var ok bool
			if oi, ok = index[*d.OwnerID]; !ok {
				continue
			}
		}
		sd := content.SurfaceDecision{ID: d.PublicID, Title: d.Title, Ruling: d.Ruling, Reason: d.Reason, FromTitle: d.FromTitle || !d.Recorded}
		if r, ok := ident.Parse(d.PublicID); ok {
			sd.Number = int(r.Number)
		}
		if d.ApprovedAt != nil {
			sd.ApprovedAt = *d.ApprovedAt
		}
		owners[oi].Decisions = append(owners[oi].Decisions, sd)
	}
	in.Owners = owners
	return in, nil
}

// withSurfaced puts the block after the project's name at the head of a user
// message (SD-10).
func withSurfaced(b *strings.Builder, project, block string) {
	b.WriteString("# Project\n\n" + project + "\n")
	if block != "" {
		b.WriteString("\n" + block)
	}
}

// ---- Creating decisions and conventions (FR-2, FR-5.4) ----

// NewDecision is a request to create a decision draft.
type NewDecision struct {
	Title      string
	OwnerType  string // project | initiative
	OwnerID    *uuid.UUID
	Supersedes []string
	Actor      string
	Via        string // ui | mcp
}

// CreateDecision mints the next DEC- number and writes the decision template
// to its default home (SD-4), registered as a draft and committed (FR-2.1,
// FR-2.3).
func (s *Server) CreateDecision(ctx context.Context, req NewDecision) (*store.Document, error) {
	title := strings.TrimSpace(req.Title)
	if title == "" {
		return nil, errors.New("Give the decision a title: a few words saying what was decided.")
	}
	switch req.OwnerType {
	case "project":
		req.OwnerID = nil
	case "initiative":
		if req.OwnerID == nil {
			return nil, errors.New("Say which initiative the decision belongs to.")
		}
	default:
		return nil, errors.New("A decision belongs to the project or to an initiative.")
	}
	var supersedes []string
	for _, id := range req.Supersedes {
		if id = strings.ToUpper(strings.TrimSpace(id)); id != "" {
			supersedes = append(supersedes, id)
		}
	}
	if probs := s.supersessionProblems(ctx, req.OwnerType, req.OwnerID, supersedes); len(probs) > 0 {
		return nil, errors.New(strings.Join(probs, " "))
	}
	tmpl, err := os.ReadFile(filepath.Join(s.CompartmentRoot, "templates", "decision", "template.md"))
	if err != nil {
		return nil, errors.New("This project has no decision template yet. Upgrade its starter pack to add templates/decision.")
	}
	ownerField := "project"
	home := filepath.Join("docs", "decisions")
	if req.OwnerType == "initiative" {
		if ownerField, err = s.initiativePath(ctx, *req.OwnerID); err != nil {
			return nil, err
		}
		if home, err = defaultHome(ctx, s.Store.Pool, *req.OwnerID); err != nil {
			return nil, err
		}
	}

	var doc *store.Document
	var abs, path string
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		id, err := store.MintDecisionID(ctx, tx)
		if err != nil {
			return err
		}
		path = filepath.ToSlash(filepath.Join(home, id+"-"+slugify(title)+".md"))
		abs = filepath.Join(s.RepoRoot, path)
		if _, err := os.Stat(abs); err == nil {
			return fmt.Errorf("A file already sits at %s, and Subutai won't write over it. Move that file, or adopt it instead.", path)
		}
		body := fillDecisionTemplate(string(tmpl), title, ownerField, supersedes)
		if body, err = stampIdentity(body, id, 1); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			return err
		}
		by := s.writerFor(store.ActStarted, req.Actor, req.Via)
		if doc, err = registerInTx(ctx, tx, path, []byte(body), "decision", req.OwnerType, req.OwnerID, nil, id, 1, req.Actor, by); err != nil {
			return err
		}
		return store.Audit(ctx, tx, req.Actor, "document.started", "document", &doc.ID,
			map[string]any{"path": path, "public_id": id, "hash": content.Hash([]byte(body)), "via": req.Via, "supersedes": supersedes})
	})
	if err != nil {
		if abs != "" && doc == nil {
			_ = os.Remove(abs)
		}
		return nil, err
	}
	s.commitDocument(path, fmt.Sprintf("subutai: start decision %s", doc.PublicID))
	if doc.OwnerID != nil {
		s.notifyEntityChanged(doc.OwnerType, *doc.OwnerID)
	}
	return doc, nil
}

// fillDecisionTemplate puts the title, owner and supersedes into the template.
// A project's own template that lacks the pack's placeholders is left as it
// is apart from what can be found.
func fillDecisionTemplate(tmpl, title, owner string, supersedes []string) string {
	const titlePH = "{{the decision, as a short title}}"
	const ownerPH = "{{project, or the initiative's path}}"
	out := strings.Replace(tmpl, `"`+titlePH+`"`, strconv.Quote(title), 1)
	out = strings.ReplaceAll(out, titlePH, title)
	out = strings.Replace(out, `"`+ownerPH+`"`, strconv.Quote(owner), 1)
	list := "[]"
	if len(supersedes) > 0 {
		list = "[" + strings.Join(supersedes, ", ") + "]"
	}
	out = strings.Replace(out, "supersedes: []", "supersedes: "+list, 1)
	return out
}

var slugRun = regexp.MustCompile(`[^a-z0-9]+`)

// slugify turns a title into a file-name slug: lower case, hyphens, at most
// about fifty characters, cut at a hyphen.
func slugify(title string) string {
	s := strings.Trim(slugRun.ReplaceAllString(strings.ToLower(title), "-"), "-")
	if len(s) > 50 {
		s = s[:50]
		if i := strings.LastIndexByte(s, '-'); i > 20 {
			s = s[:i]
		}
	}
	if s == "" {
		s = "decision"
	}
	return s
}

// conventionsPath is the conventions document's default home (SD-13).
const conventionsPath = "docs/conventions.md"

// StartConventions writes the conventions template to docs/conventions.md,
// registered as the project's draft and committed (FR-5.4).
func (s *Server) StartConventions(ctx context.Context, actor, via string) (*store.Document, error) {
	if cur, err := store.LiveConventions(ctx, s.Store.Pool); err == nil {
		return nil, fmt.Errorf("This project already has its conventions document, at %s. Revise that one instead.", cur.Path)
	} else if err != store.ErrNotFound {
		return nil, err
	}
	tmpl, err := os.ReadFile(filepath.Join(s.CompartmentRoot, "templates", "conventions", "template.md"))
	if err != nil {
		return nil, errors.New("This project has no conventions template yet. Upgrade its starter pack to add templates/conventions.")
	}
	abs := filepath.Join(s.RepoRoot, conventionsPath)
	if _, err := os.Stat(abs); err == nil {
		return nil, fmt.Errorf("A file already sits at %s, and Subutai won't write over it. Adopt it as the conventions document instead.", conventionsPath)
	}
	var doc *store.Document
	written := false
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := lockIdentity(ctx, tx, "project", nil, "conventions"); err != nil {
			return err
		}
		id, rev, err := store.NextDocumentIdentity(ctx, tx, "project", nil, "conventions")
		if err != nil {
			return err
		}
		body, err := stampIdentity(string(tmpl), id, rev)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			return err
		}
		written = true
		if doc, err = registerInTx(ctx, tx, conventionsPath, []byte(body), "conventions", "project", nil, nil, id, rev, actor,
			s.writerFor(store.ActStarted, actor, via)); err != nil {
			return err
		}
		return store.Audit(ctx, tx, actor, "document.started", "document", &doc.ID,
			map[string]any{"path": conventionsPath, "public_id": id, "hash": content.Hash([]byte(body)), "via": via})
	})
	if err != nil {
		if written {
			_ = os.Remove(abs)
		}
		return nil, err
	}
	s.commitDocument(conventionsPath, "subutai: start the project conventions")
	return doc, nil
}

// secondConventions refuses a second live conventions document, or one owned
// by anything but the project (FR-5.2).
func (s *Server) secondConventions(ctx context.Context, ownerType string) error {
	if ownerType != "project" {
		return errors.New("The conventions document belongs to the project, not to an initiative or feature.")
	}
	if cur, err := store.LiveConventions(ctx, s.Store.Pool); err == nil {
		return fmt.Errorf("This project already has its conventions document, at %s. Revise that one instead.", cur.Path)
	} else if err != store.ErrNotFound {
		return err
	}
	return nil
}

// ---- Revising a decision (FR-3.1, FR-3.2, SD-7) ----

// decisionRevisionText is what a decision's new revision starts as: a ruling
// to record, for a decision that has none, or the accepted text with a dated
// amendment skeleton appended. raw already carries the new revision's
// identity.
func decisionRevisionText(raw string, now time.Time) (string, error) {
	parsed, err := content.Parse(raw)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(parsed.FrontMatterString("ruling")) == "" {
		return content.AddFrontMatterLines(raw, []string{
			`ruling: "{{What was decided, in at most 75 words, as agents should be told it.}}"`,
			`reason: "{{Why, in one line of at most 25 words.}}"`,
		})
	}
	n := len(lifecycle.AmendmentHeadings(parsed.Body)) + 1
	nl := "\n"
	if strings.Contains(raw, "\r\n") {
		nl = "\r\n"
	}
	text := strings.TrimRight(raw, " \t\r\n") + nl + nl +
		fmt.Sprintf("## Amendment %d — {{what changed}} (%s)", n, now.Format("2006-01-02")) + nl + nl +
		"{{What the amendment changes, and why. If it changes the ruling, restate the ruling and reason in the front matter too.}}" + nl
	return text, nil
}

// decisionHasRuling reports whether an accepted decision records a ruling,
// which decides whether its page offers an amendment or recording one.
func (s *Server) decisionHasRuling(ctx context.Context, doc *store.Document) bool {
	if t, err := store.GetSurfacedText(ctx, s.Store.Pool, doc.ID); err == nil {
		return !t.FromTitle
	}
	raw, err := s.readDocFile(doc.Path)
	if err != nil {
		return false
	}
	return !surfacedTextOf(doc, raw).FromTitle
}

// ---- The viewer's rows (FR-7) ----

// decisionRow is one decision ID as the viewer and the MCP reads show it: its
// newest revision, with what it supersedes and what superseded it.
type decisionRow struct {
	Doc          store.Document
	OwnerLabel   string
	State        string // draft | in review | accepted | superseded
	Line         string // the surfaced line, as agents read it
	FromTitle    bool
	Ruling       string
	Reason       string
	Supersedes   []string
	SupersededBy []string
	Amendments   []string
	LeftOut      bool
}

// decisionRows lists every decision, newest revision per ID, in number order.
func (s *Server) decisionRows(ctx context.Context) ([]decisionRow, error) {
	docs, err := store.DecisionDocuments(ctx, s.Store.Pool)
	if err != nil {
		return nil, err
	}
	sups, err := store.AllSupersessions(ctx, s.Store.Pool)
	if err != nil {
		return nil, err
	}
	// The newest revision that isn't superseded represents the ID; if every
	// revision is superseded, the newest of them.
	pick := map[string]store.Document{}
	var order []string
	for _, d := range docs {
		key := d.PublicID
		if key == "" {
			key = d.Path
		}
		cur, seen := pick[key]
		if !seen {
			order = append(order, key)
			pick[key] = d
			continue
		}
		curGone, dGone := cur.State == lifecycle.DocSuperseded, d.State == lifecycle.DocSuperseded
		if (curGone && !dGone) || (curGone == dGone && d.Revision > cur.Revision) {
			pick[key] = d
		}
	}
	// A decision another one superseded shows as superseded, even while an
	// amendment of it is left over as a draft (FR-3.5.3).
	retired := map[string]bool{}
	for _, x := range sups {
		retired[x.SupersededID] = true
	}
	for _, d := range docs {
		if retired[d.PublicID] && d.State == lifecycle.DocSuperseded {
			if cur := pick[d.PublicID]; cur.State != lifecycle.DocSuperseded || d.Revision > cur.Revision {
				pick[d.PublicID] = d
			}
		}
	}
	sort.SliceStable(order, func(i, j int) bool {
		a, aok := ident.Parse(order[i])
		b, bok := ident.Parse(order[j])
		if aok && bok {
			return a.Number < b.Number
		}
		return aok && !bok
	})
	var rows []decisionRow
	for _, key := range order {
		d := pick[key]
		// While an amendment is open, the accepted revision is what agents
		// are told; show that one, with the state of the ID as a whole.
		shown := d
		if d.State != lifecycle.DocApproved && d.SupersedesID != nil {
			if prev, err := store.GetDocument(ctx, s.Store.Pool, *d.SupersedesID); err == nil && prev.State == lifecycle.DocApproved {
				shown = *prev
			}
		}
		row := decisionRow{Doc: shown, OwnerLabel: s.ownerName(ctx, shown.OwnerType, shown.OwnerID), State: decisionState(d.State)}
		if d.State != shown.State {
			row.State = "accepted, with an amendment " + decisionState(d.State)
		}
		if t, err := store.GetSurfacedText(ctx, s.Store.Pool, shown.ID); err == nil {
			row.Ruling, row.Reason, row.FromTitle = t.Ruling, t.Reason, t.FromTitle
		} else if raw, err := s.readDocFile(shown.Path); err == nil {
			st := surfacedTextOf(&shown, raw)
			row.Ruling, row.Reason, row.FromTitle = st.Ruling, st.Reason, st.FromTitle
		}
		label := ""
		if shown.OwnerType == "initiative" {
			label = row.OwnerLabel
		}
		row.Line = content.SurfacedLine(label, content.SurfaceDecision{ID: shown.PublicID, Title: shown.Title, Ruling: row.Ruling, Reason: row.Reason, FromTitle: row.FromTitle})
		if raw, err := s.readDocFile(shown.Path); err == nil {
			if p, err := content.Parse(string(raw)); err == nil {
				row.Amendments = lifecycle.AmendmentHeadings(p.Body)
				if shown.State != lifecycle.DocApproved && shown.State != lifecycle.DocSuperseded {
					row.Supersedes = upperAll(p.FrontMatterList("supersedes"))
				}
			} else {
				row.Amendments = lifecycle.AmendmentHeadings(string(raw))
			}
		}
		for _, x := range sups {
			if x.SupersedingID == shown.PublicID && !hasString(row.Supersedes, x.SupersededID) {
				row.Supersedes = append(row.Supersedes, x.SupersededID)
			}
			if x.SupersededID == shown.PublicID && !hasString(row.SupersededBy, x.SupersedingID) {
				row.SupersededBy = append(row.SupersededBy, x.SupersedingID)
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func decisionState(st lifecycle.DocumentState) string {
	switch st {
	case lifecycle.DocApproved:
		return "accepted"
	case lifecycle.DocReviewing:
		return "in review"
	}
	return string(st)
}

func upperAll(xs []string) []string {
	out := make([]string, 0, len(xs))
	for _, x := range xs {
		out = append(out, strings.ToUpper(strings.TrimSpace(x)))
	}
	return out
}

func hasString(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

// leftOutAnywhere is every decision the cap leaves out on some branch of the
// tree, with the branches (initiatives) where it happens (FR-7.2).
func (s *Server) leftOutAnywhere(ctx context.Context) (map[string][]string, error) {
	out := map[string][]string{}
	scopes := []surfaceScope{{}}
	var names []string
	names = append(names, "the project")
	rows, err := s.Store.Pool.Query(ctx, `SELECT id, public_id, name FROM initiatives WHERE NOT archived ORDER BY public_id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id uuid.UUID
		var pid, name string
		if err := rows.Scan(&id, &pid, &name); err != nil {
			rows.Close()
			return nil, err
		}
		iid := id
		scopes = append(scopes, surfaceScope{InitiativeID: &iid})
		names = append(names, withID(pid, name))
	}
	rows.Close()
	for i, sc := range scopes {
		r, err := s.surfaced(ctx, sc)
		if err != nil {
			return nil, err
		}
		for _, id := range r.LeftOut {
			out[id] = append(out[id], names[i])
		}
	}
	return out, nil
}
