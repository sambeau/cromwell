package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/config"
	"subutai/internal/content"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// readDocFile reads a repo-relative document path, jailed to the repo root.
func (s *Server) readDocFile(rel string) ([]byte, error) {
	abs := filepath.Join(s.RepoRoot, rel)
	resolved, err := filepath.Abs(abs)
	if err != nil {
		return nil, err
	}
	root, _ := filepath.Abs(s.RepoRoot)
	if !strings.HasPrefix(resolved, root+string(os.PathSeparator)) {
		return nil, fmt.Errorf("path %q escapes the repository", rel)
	}
	return os.ReadFile(resolved)
}

func (s *Server) linkChecker(docPath string) lifecycle.LinkChecker {
	dir := filepath.Dir(docPath)
	return func(target string) bool {
		_, err := os.Stat(filepath.Join(s.RepoRoot, dir, target))
		return err == nil
	}
}

// RegisterDoc registers a file as a document and indexes it (FR-4.1). ownerRef
// is a human-readable owner path resolved to an id; the UI's "attach a document"
// uses RegisterDocForOwner with the id it already holds.
func (s *Server) RegisterDoc(ctx context.Context, path, docType, ownerType string, ownerRef string, actor string) (*store.Document, error) {
	ownerID, err := s.resolveOwner(ctx, ownerType, ownerRef)
	if err != nil {
		return nil, err
	}
	return s.registerDocBy(ctx, path, docType, ownerType, ownerID, actor, s.writerFor(store.ActAdded, actor, "api"))
}

// RegisterDocForOwner registers a Markdown file as a document owned by the given
// entity (owner id already resolved), indexing its sections in the same
// transaction (SPEC-007 FR-6.1, SPEC-008 FR-2.4). It is the one registration
// path the CLI's `doc add`, the UI's attach action, and the MCP attach tool all
// share (DEC-003).
func (s *Server) RegisterDocForOwner(ctx context.Context, path, docType, ownerType string, ownerID *uuid.UUID, actor string) (*store.Document, error) {
	return s.registerDocBy(ctx, path, docType, ownerType, ownerID, actor, s.writerFor(store.ActAdded, actor, ""))
}

// registerDocBy is RegisterDocForOwner, saying who added the file and from
// where (SPEC-017 FR-2.2).
func (s *Server) registerDocBy(ctx context.Context, path, docType, ownerType string, ownerID *uuid.UUID, actor string, by writerAct) (*store.Document, error) {
	if _, err := config.LoadManifest(s.CompartmentRoot, docType); err != nil && docType == "spec" {
		return nil, err // a type without a template can still be registered, but spec must have one
	}
	if docType == "conventions" {
		if err := s.secondConventions(ctx, ownerType); err != nil {
			return nil, err
		}
	}
	doc, err := s.registerDocWithIdentity(ctx, path, docType, ownerType, ownerID, actor, "", 0, by)
	if err == nil && docType == "design" {
		// A design attached over an untouched starter becomes the page's body
		// (SPEC-015 SD-15).
		s.giveWayToDesign(ctx, doc, actor)
	}
	return doc, err
}

// registerDocWithIdentity registers a file already on disk. With an ID, the
// file must already carry it in its front matter; the row records it
// (SPEC-015 FR-3.3). Attach passes none: an attached file is known by its
// path until it is adopted (SD-12).
func (s *Server) registerDocWithIdentity(ctx context.Context, path, docType, ownerType string, ownerID *uuid.UUID, actor, publicID string, revision int, by writerAct) (*store.Document, error) {
	raw, err := s.readDocFile(path)
	if err != nil {
		return nil, err
	}
	var doc *store.Document
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		doc, err = registerInTx(ctx, tx, path, raw, docType, ownerType, ownerID, nil, publicID, revision, actor, by)
		return err
	})
	return doc, err
}

func (s *Server) resolveOwner(ctx context.Context, ownerType, ownerRef string) (*uuid.UUID, error) {
	switch ownerType {
	case "project":
		return nil, nil
	case "feature":
		f, err := s.featureByPath(ctx, ownerRef)
		if err != nil {
			return nil, fmt.Errorf("owner feature %q: %w", ownerRef, err)
		}
		return &f.ID, nil
	case "initiative":
		in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(ownerRef, "/"))
		if err != nil {
			return nil, fmt.Errorf("owner initiative %q: %w", ownerRef, err)
		}
		return &in.ID, nil
	}
	return nil, fmt.Errorf("unknown owner type %q", ownerType)
}

// featureByPath resolves "auth/basic/login": initiative slug path + feature slug.
func (s *Server) featureByPath(ctx context.Context, path string) (*store.Feature, error) {
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("feature path must be <initiative-path>/<feature-slug>")
	}
	in, err := s.Store.InitiativeBySlugPath(ctx, parts[:len(parts)-1])
	if err != nil {
		return nil, err
	}
	f, err := store.ScanFeature(s.Store.Pool.QueryRow(ctx, `
		SELECT `+store.FeatureCols+`
		FROM features WHERE initiative_id = $1 AND slug = $2`,
		in.ID, parts[len(parts)-1]))
	if err != nil {
		return nil, store.ErrNotFound
	}
	return f, nil
}

// ValidateDoc runs validation without submitting (FR-5.2).
func (s *Server) ValidateDoc(ctx context.Context, path string) (*lifecycle.Report, error) {
	doc, err := store.LiveDocumentByPath(ctx, s.Store.Pool, path)
	if err != nil {
		return nil, err
	}
	manifest, err := config.LoadManifest(s.CompartmentRoot, doc.Type)
	if err != nil {
		return nil, err
	}
	raw, err := s.readDocFile(doc.Path)
	if err != nil {
		return nil, err
	}
	report := s.validateDocument(ctx, doc, manifest, raw)
	return &report, nil
}

// SubmitDoc validates synchronously; failure blocks the transition and
// returns the report. Success transitions draft → reviewing, re-indexes at
// the submitted content, and lets the rule engine queue the review
// (FR-5.2).
func (s *Server) SubmitDoc(ctx context.Context, path, actor string) (*lifecycle.Report, *store.Document, error) {
	return s.submitDocWith(ctx, path, actor, nil, nil)
}

// submitDocWith is SubmitDoc with what the caller adds in the same
// transaction (SPEC-017 NFR-1): payload joins the transition's audit row, and
// inTx runs whether or not the document passes validation, so a writing act
// is recorded even when its submission is refused.
func (s *Server) submitDocWith(ctx context.Context, path, actor string, payload map[string]any,
	inTx func(context.Context, pgx.Tx, *store.Document) error) (*lifecycle.Report, *store.Document, error) {
	doc, err := store.LiveDocumentByPath(ctx, s.Store.Pool, path)
	if err != nil {
		return nil, nil, err
	}
	// Reviewing spends agent time, and accepting a bug is the commitment of
	// scope, so a report waits for its bug's triage (SPEC-019 SD-6).
	if err := s.refuseReportBeforeAcceptance(ctx, doc); err != nil {
		return nil, nil, err
	}
	manifest, err := config.LoadManifest(s.CompartmentRoot, doc.Type)
	if err != nil {
		return nil, nil, err
	}
	raw, err := s.readDocFile(doc.Path)
	if err != nil {
		return nil, nil, err
	}
	report := s.validateDocument(ctx, doc, manifest, raw)

	from := doc.State
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		// Validation results live on the document's audit trail either way
		// (DESIGN-003 §3).
		if err := store.Audit(ctx, tx, actor, "document.validated", "document", &doc.ID,
			map[string]any{"valid": report.Valid, "issues": report.Issues}); err != nil {
			return err
		}
		if inTx != nil {
			if err := inTx(ctx, tx, doc); err != nil {
				return err
			}
		}
		if !report.Valid {
			return nil
		}
		parsed, perr := content.Parse(string(raw))
		if perr != nil {
			return perr // unreachable: validation requires a parseable doc
		}
		if err := store.ReplaceSections(ctx, tx, doc.ID, content.Hash(raw), parsed.Sections); err != nil {
			return err
		}
		if err := store.RefreshDocumentTitle(ctx, tx, doc.ID, parsed.FrontMatterString("title")); err != nil {
			return err
		}
		return store.TransitionDocument(ctx, tx, doc, lifecycle.DocSubmit, actor, payload)
	})
	if err != nil {
		return nil, nil, err
	}
	if report.Valid {
		s.Bus.Publish(bus.DocumentTransitioned{
			DocID: doc.ID, From: from, To: lifecycle.DocReviewing,
			Event: lifecycle.DocSubmit, Actor: actor,
		})
	}
	return &report, doc, nil
}

// ReviseDoc creates a successor draft for an approved document (FR-4.4):
// new row with supersedes_id, working copy at a version-suffixed path.
func (s *Server) ReviseDoc(ctx context.Context, path, actor string) (*store.Document, error) {
	return s.reviseDocBy(ctx, path, actor, s.writerFor(store.ActOpenedRevision, actor, ""))
}

// reviseDocBy is ReviseDoc, saying who opened the revision (SPEC-017 FR-2.2).
func (s *Server) reviseDocBy(ctx context.Context, path, actor string, by writerAct) (*store.Document, error) {
	doc, err := store.LiveDocumentByPath(ctx, s.Store.Pool, path)
	if err != nil {
		return nil, err
	}
	if doc.State != lifecycle.DocApproved {
		return nil, fmt.Errorf("only approved documents are revised; %s is %s (edit the draft directly)", path, doc.State)
	}
	raw, err := s.readDocFile(doc.Path)
	if err != nil {
		return nil, err
	}

	// A successor keeps its predecessor's ID at the next revision, written
	// into the working copy (SPEC-015 FR-3.4). A document with no ID revises
	// as it always has.
	revision := 0
	if doc.PublicID != "" {
		maxRev, _, err := store.IdentityUse(ctx, s.Store.Pool, doc.PublicID)
		if err != nil {
			return nil, err
		}
		revision = maxRev + 1
		stamped, err := content.SetIdentity(string(raw), doc.PublicID, revision)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", doc.Path, err)
		}
		raw = []byte(stamped)
	}
	// A decision is never edited: its revision records a ruling it lacks, or
	// appends a dated amendment (SPEC-018 SD-5, SD-7).
	if doc.Type == "decision" {
		if doc.PublicID == "" {
			return nil, errors.New("Give this decision an ID first, so its amendment can carry it.")
		}
		text, err := decisionRevisionText(string(raw), time.Now())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", doc.Path, err)
		}
		raw = []byte(text)
	}

	ext := filepath.Ext(doc.Path)
	workingPath := strings.TrimSuffix(doc.Path, ext) + ".rev" + ext
	// Another revision's working copy is never written over, and so never
	// removed by this call's failure (SPEC-018 R18-18).
	if _, err := os.Stat(filepath.Join(s.RepoRoot, workingPath)); err == nil {
		return nil, fmt.Errorf("A revision of this document is already being written at %s. Work on that one, or remove the file if it is left over.", workingPath)
	}
	if err := os.WriteFile(filepath.Join(s.RepoRoot, workingPath), raw, 0o644); err != nil {
		return nil, err
	}

	var successor *store.Document
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		predecessorID := doc.ID
		successor, err = store.RegisterDocument(ctx, tx, doc.Type, doc.OwnerType, doc.OwnerID,
			workingPath, doc.Title, content.Hash(raw), &predecessorID, actor)
		if err != nil {
			return err
		}
		if err := recordWriter(ctx, tx, successor.ID, by); err != nil {
			return err
		}
		if doc.PublicID == "" {
			return nil
		}
		if err := store.SetDocumentIdentity(ctx, tx, successor.ID, doc.PublicID, revision); err != nil {
			return identityClash(err, doc.PublicID, revision)
		}
		successor.PublicID, successor.Revision = doc.PublicID, revision
		return nil
	})
	if err != nil {
		_ = os.Remove(filepath.Join(s.RepoRoot, workingPath))
		return nil, err
	}
	return successor, nil
}

// ---- Git watcher (FR-4.2/4.3) ----

// OnPostCommit is called by the git post-commit hook: diff the commit for
// registered document paths and publish change events.
func (s *Server) OnPostCommit(ctx context.Context) error {
	out, err := s.git("diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	if err != nil {
		return err
	}
	commit, _ := s.git("rev-parse", "HEAD")
	paths := strings.Split(strings.TrimSpace(out), "\n")
	// Moves first, so a document that moved and changed in one commit is
	// checked for drift at its new path (SPEC-015 FR-4.2).
	for _, path := range paths {
		if path != "" {
			s.followMovedDocument(ctx, path)
		}
	}
	for _, path := range paths {
		if path == "" {
			continue
		}
		s.publishIfDrifted(ctx, path, strings.TrimSpace(commit))
	}
	return nil
}

// CatchUpScan re-hashes every registered live document (boot, DESIGN-002
// §8): drift from content_hash is handled exactly like a live commit —
// covering edits made while the server was down.
func (s *Server) CatchUpScan(ctx context.Context) error {
	// Documents whose files moved while the server was down are found by
	// their IDs first (SPEC-015 FR-4.3).
	s.findMovedDocuments(ctx)
	rows, err := s.Store.Pool.Query(ctx,
		`SELECT path FROM documents WHERE state <> 'superseded'`)
	if err != nil {
		return err
	}
	defer rows.Close()
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return err
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, p := range paths {
		commit, _ := s.git("log", "-1", "--format=%H", "--", p)
		s.publishIfDrifted(ctx, p, strings.TrimSpace(commit))
	}
	return nil
}

func (s *Server) publishIfDrifted(ctx context.Context, path, commit string) {
	doc, err := store.LiveDocumentByPath(ctx, s.Store.Pool, path)
	if err != nil {
		return // unregistered or gone: not ours
	}
	raw, err := s.readDocFile(path)
	if err != nil {
		return // deleted files are a phase-2 concern; the row remains
	}
	if content.Hash(raw) == doc.ContentHash {
		return
	}
	s.Bus.Publish(bus.DocumentFileChanged{Path: path, CommitHash: commit})
}

func (s *Server) git(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = s.RepoRoot
	out, err := cmd.Output()
	return string(out), err
}

// ---- Document-review prompt assembly (reused by the planner) ----

// buildReview assembles the spec/dev-plan reviewer prompt per FR-5.3,
// reading role, skill, and manifest fresh (O-6).
func (s *Server) buildReview(ctx context.Context, d *store.Dispatch) (string, string, int, error) {
	cfg, err := s.freshConfig()
	if err != nil {
		return "", "", 0, err
	}
	doc, err := store.GetDocument(ctx, s.Store.Pool, d.RefID)
	if err != nil {
		return "", "", 0, err
	}
	raw, err := s.readDocFile(doc.Path)
	if err != nil {
		return "", "", 0, err
	}
	if content.Hash(raw) != doc.ContentHash {
		return "", "", 0, fmt.Errorf("document %s changed on disk since submission; re-submit", doc.Path)
	}

	role, err := config.LoadRole(s.CompartmentRoot, d.Role)
	if err != nil {
		return "", "", 0, err
	}
	skillBody := ""
	if role.Skill != "" {
		skill, err := config.LoadSkill(s.CompartmentRoot, role.Skill)
		if err != nil {
			return "", "", 0, err
		}
		skillBody = skill.Body
	}
	manifest, err := config.LoadManifest(s.CompartmentRoot, doc.Type)
	if err != nil {
		return "", "", 0, err
	}

	in := content.ReviewPromptInput{
		System:       roleSystemPrompt(role, skillBody),
		ProjectName:  filepath.Base(s.RepoRoot),
		DocPath:      doc.Path,
		DocBody:      string(raw),
		CommentsOnly: manifest.HumanApproval(),
	}

	report := s.validateDocument(ctx, doc, manifest, raw)
	if !report.Valid {
		var b strings.Builder
		for _, is := range report.Issues {
			fmt.Fprintf(&b, "- [%s] %s\n", is.Check, is.Detail)
		}
		in.ValidationReport = b.String()
	}

	// Owning feature context + ancestor-initiative documents (direct
	// attachment tier; embeddings arrive in phase 2).
	if doc.OwnerType == "feature" && doc.OwnerID != nil {
		f, err := store.GetFeature(ctx, s.Store.Pool, *doc.OwnerID)
		if err == nil {
			ancestors, err := store.InitiativeAncestors(ctx, s.Store.Pool, f.InitiativeID)
			if err != nil {
				return "", "", 0, err
			}
			var pathParts []string
			var ancestorIDs []uuid.UUID
			for i := len(ancestors) - 1; i >= 0; i-- {
				pathParts = append(pathParts, ancestors[i].Slug)
			}
			for _, a := range ancestors {
				ancestorIDs = append(ancestorIDs, a.ID)
			}
			in.Feature = &content.FeatureContext{
				Name: f.Name, Slug: f.Slug, Description: f.Description,
				InitiativePath: strings.Join(pathParts, "/"),
			}
			// The feature's own approved design leads the background: the
			// fidelity bar (SPEC-009 FR-8) asks the reviewer to account for
			// every design decision, and a design attached to the feature
			// itself — not an ancestor initiative — must reach the prompt too.
			if own, err := store.CurrentApprovedDocForOwner(ctx, s.Store.Pool, "design", "feature", f.ID); err == nil &&
				own.ID != doc.ID {
				if body, rerr := s.readDocFile(own.Path); rerr == nil {
					in.AncestorDocs = append(in.AncestorDocs, content.AttachedDoc{
						Path: own.Path, Title: own.Title, Type: own.Type,
						ApprovedAt: own.ApprovedAt, Body: string(body),
					})
				}
			}
			docs, err := store.ApprovedDocsOwnedBy(ctx, s.Store.Pool, "initiative", ancestorIDs)
			if err != nil {
				return "", "", 0, err
			}
			for _, ad := range docs {
				// Decisions and conventions reach the prompt as the surfaced
				// block, never whole (SPEC-018 SD-11).
				if surfacedType(ad.Type) {
					continue
				}
				body, err := s.readDocFile(ad.Path)
				if err != nil {
					continue // archived or moved; provenance beats absence
				}
				in.AncestorDocs = append(in.AncestorDocs, content.AttachedDoc{
					Path: ad.Path, Title: ad.Title, Type: ad.Type,
					ApprovedAt: ad.ApprovedAt, Body: string(body),
				})
			}
		}
	}

	comments, err := store.CommentsForDocument(ctx, s.Store.Pool, doc.ID, true)
	if err != nil {
		return "", "", 0, err
	}
	var issues []store.Comment
	for _, c := range comments {
		if c.IsIssue {
			issues = append(issues, c)
			continue
		}
		in.Comments = append(in.Comments, content.CommentContext{
			Author: c.Author, SectionRef: c.SectionRef, Body: c.Body, CreatedAt: c.CreatedAt,
		})
	}

	in.HumanIssues = issuesPromptSection(issues)
	if in.Surfaced, err = s.surfacedBlock(ctx, s.scopeForDocument(ctx, doc)); err != nil {
		return "", "", 0, err
	}
	system, user := content.AssembleReviewPrompt(in)
	turnCap := cfg.Dispatch.TurnCap
	if role.Limits != nil && role.Limits.TurnCap > 0 {
		turnCap = role.Limits.TurnCap
	}
	return system, user, turnCap, nil
}
