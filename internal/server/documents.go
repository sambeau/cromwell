package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/bus"
	"cromwell/internal/config"
	"cromwell/internal/content"
	"cromwell/internal/dispatch"
	"cromwell/internal/lifecycle"
	"cromwell/internal/store"
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

// RegisterDoc registers a file as a document and indexes it (FR-4.1).
func (s *Server) RegisterDoc(ctx context.Context, path, docType, ownerType string, ownerRef string, actor string) (*store.Document, error) {
	if _, err := config.LoadManifest(s.CompartmentRoot, docType); err != nil && docType == "spec" {
		return nil, err // a type without a template can still be registered, but spec must have one
	}
	raw, err := s.readDocFile(path)
	if err != nil {
		return nil, err
	}
	parsed, perr := content.Parse(string(raw))
	title := path
	if perr == nil {
		if t := parsed.FrontMatterString("title"); t != "" {
			title = t
		}
	}

	var doc *store.Document
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		ownerID, err := s.resolveOwner(ctx, ownerType, ownerRef)
		if err != nil {
			return err
		}
		doc, err = store.RegisterDocument(ctx, tx, docType, ownerType, ownerID, path, title, content.Hash(raw), nil, actor)
		if err != nil {
			return err
		}
		if perr == nil {
			return store.ReplaceSections(ctx, tx, doc.ID, content.Hash(raw), parsed.Sections)
		}
		return nil
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
	var f store.Feature
	err = s.Store.Pool.QueryRow(ctx, `
		SELECT id, initiative_id, slug, name, description, state, created_at
		FROM features WHERE initiative_id = $1 AND slug = $2`,
		in.ID, parts[len(parts)-1]).
		Scan(&f.ID, &f.InitiativeID, &f.Slug, &f.Name, &f.Description, &f.State, &f.CreatedAt)
	if err != nil {
		return nil, store.ErrNotFound
	}
	return &f, nil
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
	report := lifecycle.Validate(manifest, string(raw), s.linkChecker(doc.Path))
	return &report, nil
}

// SubmitDoc validates synchronously; failure blocks the transition and
// returns the report. Success transitions draft → reviewing, re-indexes at
// the submitted content, and lets the rule engine queue the review
// (FR-5.2).
func (s *Server) SubmitDoc(ctx context.Context, path, actor string) (*lifecycle.Report, *store.Document, error) {
	doc, err := store.LiveDocumentByPath(ctx, s.Store.Pool, path)
	if err != nil {
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
	report := lifecycle.Validate(manifest, string(raw), s.linkChecker(doc.Path))

	from := doc.State
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		// Validation results live on the document's audit trail either way
		// (DESIGN-003 §3).
		if err := store.Audit(ctx, tx, actor, "document.validated", "document", &doc.ID,
			map[string]any{"valid": report.Valid, "issues": report.Issues}); err != nil {
			return err
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
		return store.TransitionDocument(ctx, tx, doc, lifecycle.DocSubmit, actor, nil)
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

	ext := filepath.Ext(doc.Path)
	workingPath := strings.TrimSuffix(doc.Path, ext) + ".rev" + ext
	if err := os.WriteFile(filepath.Join(s.RepoRoot, workingPath), raw, 0o644); err != nil {
		return nil, err
	}

	var successor *store.Document
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		predecessorID := doc.ID
		successor, err = store.RegisterDocument(ctx, tx, doc.Type, doc.OwnerType, doc.OwnerID,
			workingPath, doc.Title, content.Hash(raw), &predecessorID, actor)
		return err
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
	for _, path := range strings.Split(strings.TrimSpace(out), "\n") {
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

// ---- Prompt builder (dispatch.PromptBuilder) ----

// BuildReview assembles the reviewer prompt per FR-5.3, reading role, skill,
// and manifest fresh (O-6).
func (s *Server) BuildReview(ctx context.Context, d *store.Dispatch) (string, string, int, error) {
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
		RoleIdentity: role.Identity,
		SkillBody:    skillBody,
		ProjectName:  filepath.Base(s.RepoRoot),
		DocPath:      doc.Path,
		DocBody:      string(raw),
	}

	report := lifecycle.Validate(manifest, string(raw), s.linkChecker(doc.Path))
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
			docs, err := store.ApprovedDocsOwnedBy(ctx, s.Store.Pool, "initiative", ancestorIDs)
			if err != nil {
				return "", "", 0, err
			}
			for _, ad := range docs {
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
	for _, c := range comments {
		in.Comments = append(in.Comments, content.CommentContext{
			Author: c.Author, SectionRef: c.SectionRef, Body: c.Body, CreatedAt: c.CreatedAt,
		})
	}

	system, user := content.AssembleReviewPrompt(in)
	turnCap := cfg.Dispatch.TurnCap
	if role.Limits != nil && role.Limits.TurnCap > 0 {
		turnCap = role.Limits.TurnCap
	}
	return system, user, turnCap, nil
}

var _ dispatch.PromptBuilder = (*Server)(nil)
