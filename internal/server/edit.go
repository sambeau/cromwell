package server

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/config"
	"subutai/internal/content"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// Editing in the browser (SPEC-016). The editor works on the plain file in
// the main working tree: it holds the hash of the file it loaded, and a save
// that finds the file moved on stops rather than overwrite it (SD-6).

// bom is the UTF-8 byte-order mark. The text box shows a file without it,
// and a save puts it back (FR-1.2).
const bom = "\uFEFF"

// EditRequest is one save from the editor.
type EditRequest struct {
	DocID    uuid.UUID
	BaseHash string // the hash of the file as the editor loaded it
	Text     string // the text box, as the browser sent it
	Commit   bool   // Save & commit rather than Save
	Subject  string // an optional commit subject of the person's own
	Actor    string
	// ShownState and ShownPath are the document's state and path when the
	// editor opened. The lock is on the file's content; these catch a change
	// of state or place that leaves the content alone (SD-6).
	ShownState string
	ShownPath  string
}

// EditResult says what a save did.
type EditResult struct {
	Doc       *store.Document
	Wrote     bool   // the file was written
	Commit    string // the short hash of the commit, when one was made
	CommitErr error  // why the commit failed, after the file was written
	Withdrew  bool   // a reviewing document went back to draft (SD-9)
}

// EditConflict is a save refused because the file changed on disk since the
// editor opened it (SD-7). It carries the file as it is now, so the page can
// show what changed.
type EditConflict struct {
	Current string
}

func (e *EditConflict) Error() string {
	return "This file has changed on disk since you opened it, so your save was stopped rather than overwrite that change. Nothing was written."
}

// EditStale is a save refused because the document changed state or moved
// since the editor opened it, though its text may not have (SD-6).
type EditStale struct{ Why string }

func (e *EditStale) Error() string { return e.Why }

func staleWhy(doc *store.Document, shownState, shownPath string) string {
	if shownPath != "" && doc.Path != shownPath {
		return fmt.Sprintf("This document moved to %s since you opened the editor, so your save was stopped. Nothing was written. Copy your text, then open the editor again.", doc.Path)
	}
	if shownState == "" || string(doc.State) == shownState {
		return ""
	}
	what := map[lifecycle.DocumentState]string{
		lifecycle.DocReviewing:  "was submitted for review",
		lifecycle.DocApproved:   "was approved",
		lifecycle.DocDraft:      "went back to draft",
		lifecycle.DocSuperseded: "was superseded",
	}[doc.State]
	if what == "" {
		what = "changed state"
	}
	return fmt.Sprintf("This document %s since you opened the editor, so your save was stopped: saving now would do something you weren't told about. Nothing was written. Copy your text, then open the editor again to see what saving means now.", what)
}

// editText is how the editor presents a file: without a byte-order mark,
// which the text box would lose.
func editText(raw []byte) string {
	return strings.TrimPrefix(string(raw), bom)
}

// fileFromText turns the browser's text back into file bytes with the file's
// own conventions: browsers send CRLF, so the text takes the file's line
// endings, and a byte-order mark the file had is put back (FR-1.2).
func fileFromText(text string, like []byte) []byte {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if strings.Contains(string(like), "\r\n") {
		text = strings.ReplaceAll(text, "\n", "\r\n")
	}
	if strings.HasPrefix(string(like), bom) {
		text = bom + text
	}
	return []byte(text)
}

// editRefusal says why a document can't be edited in the browser at all, or
// "" when it can. It covers what is true whatever the text: superseded and
// approved documents, the author at work, and a question in the Inbox that
// an edit would answer by the back door (SD-8, SD-11, SD-12).
func (s *Server) editRefusal(ctx context.Context, doc *store.Document) string {
	switch doc.State {
	case lifecycle.DocSuperseded:
		return "A superseded document is a record, so it isn't edited."
	case lifecycle.DocApproved:
		return "This document is approved, so it isn't changed in place. Editing it starts a revision."
	}
	if err := s.refuseIfAuthorAtWork(ctx, doc); err != nil {
		return "Its author agent is revising this document now, and will write the whole of it, so it can't be edited here until the revision is done."
	}
	if why := s.authorDueRefusal(ctx, doc); why != "" {
		return why
	}
	if doc.State == lifecycle.DocReviewing {
		if err := s.refuseIfPending(ctx, doc); err != nil {
			return err.Error()
		}
		if why := s.revisionInFlightRefusal(ctx, doc); why != "" {
			return why
		}
	}
	return ""
}

// authorDueRefusal refuses a sent feature's spec or plan that carries
// findings its author agent is due to address: the heartbeat sends the author
// back to any such draft that has changed, and it writes the whole document,
// so an edit here would only start that (SD-11, R16-4). While the Inbox asks
// whether to allow the author another round, the person may take it over.
func (s *Server) authorDueRefusal(ctx context.Context, doc *store.Document) string {
	if doc.OwnerType != "feature" || doc.OwnerID == nil || (doc.Type != "spec" && doc.Type != "dev_plan") {
		return ""
	}
	if doc.State != lifecycle.DocDraft && doc.State != lifecycle.DocReviewing {
		return ""
	}
	waits, err := s.waitsForAuthor(ctx, doc)
	if err != nil || !waits {
		return ""
	}
	f, err := store.GetFeature(ctx, s.Store.Pool, *doc.OwnerID)
	if err != nil {
		return ""
	}
	if sent, _ := s.featureSent(ctx, f); !sent {
		return ""
	}
	if s.pendingKindOn(ctx, "authoring-deadlock", "document", doc.ID) {
		return ""
	}
	return fmt.Sprintf("This %s has review findings its author agent is due to address, and %s has been sent to development, so the author agent will revise it and write the whole of it. It can be edited here once the author has finished, or if the Inbox asks you whether to allow the author another round.",
		docTypeWords(doc.Type), withID(f.PublicID, f.Name))
}

// revisionInFlightRefusal refuses to withdraw a revised spec or plan whose
// submission paused a feature being built: the Inbox is asking what the
// revision means for work in flight, and withdrawing it would leave that
// question about a submission that no longer exists (R16-6).
func (s *Server) revisionInFlightRefusal(ctx context.Context, doc *store.Document) string {
	if doc.SupersedesID == nil || doc.OwnerType != "feature" || doc.OwnerID == nil || (doc.Type != "spec" && doc.Type != "dev_plan") {
		return ""
	}
	if !s.pendingKindOn(ctx, "revision-in-flight", "feature", *doc.OwnerID) {
		return ""
	}
	return fmt.Sprintf("Submitting this revised %s paused new tasks on its feature, and a question in the Inbox asks what it means for work in flight. Answer it first; the revision can be edited after that.", docTypeWords(doc.Type))
}

// pendingKindOn reports whether a question of a kind is waiting about a ref.
func (s *Server) pendingKindOn(ctx context.Context, kind, refType string, refID uuid.UUID) bool {
	pending, err := s.Store.PendingCheckpoints(ctx)
	if err != nil {
		return false
	}
	for _, cp := range pending {
		if cp.Kind == kind && cp.RefType == refType && cp.RefID == refID {
			return true
		}
	}
	return false
}

// reviseRefusal says why an approved document can't have a revision started
// from the editor, or "" when it can (SD-11, SD-12).
func (s *Server) reviseRefusal(ctx context.Context, doc *store.Document) string {
	if doc.Type == "decision" {
		return "An accepted decision is never edited. If the project changes its mind, a new decision supersedes this one, or a dated amendment is appended to it."
	}
	if _, err := config.LoadManifest(s.CompartmentRoot, doc.Type); err != nil {
		return fmt.Sprintf("A revision of an approved %s can't be submitted for review yet, because documents of this type have no template, so it can't be started here. Until a person can rule on any type of document, change it by writing a new %s.", doc.Type, doc.Type)
	}
	if err := s.refuseIfPending(ctx, doc); err != nil {
		return err.Error()
	}
	return ""
}

// SaveEdit is Save and Save & commit (SPEC-016 FR-2). The checks run in the
// order the spec gives: the lifecycle, the lock, identity, and for a commit
// whether the file holds uncommitted work that isn't Subutai's. Only then is
// anything changed.
func (s *Server) SaveEdit(ctx context.Context, req EditRequest) (*EditResult, error) {
	doc, err := store.GetDocument(ctx, s.Store.Pool, req.DocID)
	if err != nil {
		return nil, err
	}
	if why := staleWhy(doc, req.ShownState, req.ShownPath); why != "" {
		return nil, &EditStale{Why: why}
	}
	if why := s.editRefusal(ctx, doc); why != "" {
		return nil, errors.New(why)
	}
	abs, err := s.editableFile(doc.Path)
	if err != nil {
		return nil, err
	}

	// One save at a time: a second save from another tab sees the first's
	// write as a change on disk (FR-3.2).
	s.editMu.Lock()
	defer s.editMu.Unlock()

	current, err := s.readDocFile(doc.Path)
	if err != nil {
		return nil, errors.New("The file for this document can't be read from the working tree, so there's nothing to save over. Restore it in git first.")
	}
	if content.Hash(current) != req.BaseHash {
		return nil, &EditConflict{Current: editText(current)}
	}
	next := fileFromText(req.Text, current)
	if err := checkIdentityKept(doc, current, next); err != nil {
		return nil, err
	}
	changed := string(next) != string(current)

	res := &EditResult{Doc: doc}
	dirty, err := s.differsFromHead(doc.Path, current)
	if err != nil {
		return nil, err
	}
	// Whether the file the editor loaded held only committed text or text
	// Subutai wrote. Only such a save counts as Subutai's own later (R16-12).
	own := !dirty
	if dirty {
		if own, err = s.subutaiWrote(ctx, doc, content.Hash(current)); err != nil {
			return nil, err
		}
	}
	if req.Commit {
		if !changed && !dirty {
			return res, nil
		}
		if dirty {
			if !own {
				return nil, fmt.Errorf("%s has changes that aren't committed, made outside this editor. Commit or discard them first, or choose Save, which writes the file and leaves committing to you.", doc.Path)
			}
		}
	} else if !changed {
		return res, nil
	}

	if changed && doc.State == lifecycle.DocReviewing {
		if err := s.withdrawForEdit(ctx, doc, req.Actor); err != nil {
			return nil, err
		}
		res.Withdrew = true
	}
	if changed {
		if err := writeFileAtomically(abs, next); err != nil {
			return nil, err
		}
		res.Wrote = true
	}
	if req.Commit {
		res.Commit, res.CommitErr = s.commitEdit(doc, req.Subject, req.Actor)
	}

	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		payload := map[string]any{
			"path": doc.Path, "from": req.BaseHash, "to": content.Hash(next),
			"wrote": res.Wrote, "commit": res.Commit, "withdrew": res.Withdrew,
			"own": own,
		}
		if res.CommitErr != nil {
			payload["commit_error"] = res.CommitErr.Error()
		}
		return store.Audit(ctx, tx, req.Actor, "document.edited", "document", &doc.ID, payload)
	}); err != nil {
		return nil, err
	}

	// The watcher's own event, so the rule engine reindexes the document as
	// it would a committed vim edit; the heartbeat doesn't rehash files, so
	// without this a plain Save would stay unindexed until the next commit
	// (SD-13).
	s.Bus.Publish(bus.DocumentFileChanged{Path: doc.Path, CommitHash: res.Commit})
	s.notifyEntityChanged("document", doc.ID)
	return res, nil
}

// checkIdentityKept refuses a text that removes or changes the document's
// `id:` and `revision:` lines, or gives a document an ID it didn't have
// (SD-10).
func checkIdentityKept(doc *store.Document, current, next []byte) error {
	if doc.PublicID != "" {
		id, rev, ok := content.ReadIdentity(string(next))
		if !ok || id != doc.PublicID || rev != doc.Revision {
			return fmt.Errorf("This document's ID is %s, at revision %d, and the text you saved doesn't say so. Put back the lines \"id: %s\" and \"revision: %d\" in its front matter and save again. They are how Subutai recognises this file wherever it moves.",
				doc.PublicID, doc.Revision, doc.PublicID, doc.Revision)
		}
		return nil
	}
	had, has := content.ReadDeclaredID(string(current)), content.ReadDeclaredID(string(next))
	if has == had {
		return nil
	}
	if had == "" {
		return errors.New("This document has no ID yet, and an ID can't be added by editing it. Take the id: line out and save, then use Give it an ID on the document's page, which registers the ID properly.")
	}
	return fmt.Errorf("The file's front matter says \"id: %s\", and the text you saved changes that. Put the line back as it was and save again.", had)
}

// differsFromHead reports whether a file's content differs from what HEAD
// holds for its path, or git doesn't track it at all (SD-5). The index is
// not consulted: Save & commit commits the file's content.
func (s *Server) differsFromHead(path string, current []byte) (bool, error) {
	cmd := exec.Command("git", "show", "HEAD:"+filepath.ToSlash(path))
	cmd.Dir = s.RepoRoot
	head, err := cmd.Output()
	if err != nil {
		return true, nil // not in HEAD: untracked, or a repository with no commits
	}
	return string(head) != string(current), nil
}

// subutaiWrote reports whether a file's content is something Subutai wrote
// itself: the editor's last save of the document, made over a file that was
// itself clean or Subutai's, or the working copy that Revise wrote for a
// successor (SD-5). Both record the hash on the audit trail when they write.
func (s *Server) subutaiWrote(ctx context.Context, doc *store.Document, hash string) (bool, error) {
	var last string
	err := s.Store.Pool.QueryRow(ctx, `SELECT COALESCE(payload->>'to', '') FROM audit_events
		WHERE ref_id = $1 AND kind = 'document.edited' AND (payload->>'wrote')::boolean
		  AND COALESCE((payload->>'own')::boolean, false)
		ORDER BY occurred_at DESC, id DESC LIMIT 1`, doc.ID).Scan(&last)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	if last == hash {
		return true, nil
	}
	var revised bool
	err = s.Store.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM audit_events
		WHERE ref_id = $1 AND kind = 'document.revision_created' AND payload->>'hash' = $2)`,
		doc.ID, hash).Scan(&revised)
	if err != nil {
		return false, err
	}
	if !revised {
		return false, nil
	}
	// The revision's working copy is Subutai's only until someone else has
	// edited it: a later browser save that wrote other text, or a commit.
	return last == "", nil
}

// withdrawForEdit takes a reviewing document back to draft before its file
// changes: the review was of the text as submitted (SD-9). A queued review is
// cancelled and a hold cleared, as a person's send-back does; no issue is
// recorded, because an edit isn't an objection.
func (s *Server) withdrawForEdit(ctx context.Context, doc *store.Document, actor string) error {
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if err := s.cancelQueuedReviews(ctx, tx, doc, actor); err != nil {
			return err
		}
		if err := store.ClearDocumentHold(ctx, tx, doc.ID); err != nil {
			return err
		}
		return store.TransitionDocument(ctx, tx, doc, lifecycle.DocWithdraw, actor,
			map[string]any{"cause": "edited in the browser"})
	}); err != nil {
		return err
	}
	s.Bus.Publish(bus.DocumentTransitioned{
		DocID: doc.ID, From: lifecycle.DocReviewing, To: lifecycle.DocDraft,
		Event: lifecycle.DocWithdraw, Actor: actor,
	})
	return nil
}

// editableFile is the absolute path the editor may write for a document: a
// regular file inside the repository, reached without a symbolic link
// (R16-9). It never creates a file that isn't there.
func (s *Server) editableFile(rel string) (string, error) {
	outside := fmt.Errorf("The file for this document, %s, isn't a plain file inside the repository, so the editor won't write it.", rel)
	root, err := filepath.EvalSymlinks(s.RepoRoot)
	if err != nil {
		return "", err
	}
	abs := filepath.Join(s.RepoRoot, rel)
	fi, err := os.Lstat(abs)
	if err != nil {
		return "", errors.New("The file for this document can't be read from the working tree, so there's nothing to save over. Restore it in git first.")
	}
	if !fi.Mode().IsRegular() {
		return "", outside
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return "", outside
	}
	if dir != root && !strings.HasPrefix(dir, root+string(os.PathSeparator)) {
		return "", outside
	}
	return filepath.Join(dir, filepath.Base(abs)), nil
}

// writeFileAtomically writes beside the file and renames into place, keeping
// the file's mode, so no reader sees half a file (SD-6).
func writeFileAtomically(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".subutai-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, mode); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// commitEdit commits the document's file and nothing else, with the operator
// as author and Subutai as committer (SD-3). It returns the short hash.
func (s *Server) commitEdit(doc *store.Document, subject, actor string) (string, error) {
	subject = strings.TrimSpace(strings.ReplaceAll(subject, "\n", " "))
	name := doc.PublicID
	if name == "" {
		name = doc.Path
	}
	if subject == "" {
		subject = "Edit " + name + " in the browser"
	}
	state := string(doc.State)
	if doc.State == lifecycle.DocReviewing {
		state = "draft" // it was withdrawn from review before the write
	}
	body := doc.Path + ", " + state
	if doc.Revision > 0 {
		body += fmt.Sprintf(", revision %d", doc.Revision)
	}
	body += ".\nSaved from the Subutai web editor by " + actor + "."

	if _, err := gitIn(s.RepoRoot, "add", "--", doc.Path); err != nil {
		return "", err
	}
	cmd := exec.Command("git", "commit", "--only", "-m", subject, "-m", body,
		"--author", commitAuthor(actor), "--", doc.Path)
	cmd.Dir = s.RepoRoot
	cmd.Env = append(os.Environ(), "GIT_COMMITTER_NAME=subutai", "GIT_COMMITTER_EMAIL=subutai@localhost")
	if out, err := cmd.CombinedOutput(); err != nil {
		// Leave nothing staged for the next commit to sweep up (R16-19).
		_, _ = gitIn(s.RepoRoot, "reset", "-q", "--", doc.Path)
		return "", fmt.Errorf("git refused the commit, perhaps because of a hook, a merge in progress or commit signing: %s", strings.TrimSpace(string(out)))
	}
	short, err := gitIn(s.RepoRoot, "rev-parse", "--short", "HEAD")
	return strings.TrimSpace(short), err
}

var (
	authorWithEmail = regexp.MustCompile(`^[^<>]+ <[^<>\s]+>$`)
	unsafeInEmail   = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
)

// commitAuthor is the git author for the configured operator: ui_actor as it
// stands if it is written "Name <email>", otherwise the name with a made-up
// local address (SD-3).
func commitAuthor(actor string) string {
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "operator"
	}
	if authorWithEmail.MatchString(actor) {
		return actor
	}
	actor = strings.TrimSpace(strings.NewReplacer("<", "", ">", "").Replace(actor))
	if actor == "" {
		actor = "operator"
	}
	local := strings.Trim(unsafeInEmail.ReplaceAllString(actor, "-"), "-.")
	if local == "" {
		local = "operator"
	}
	return actor + " <" + local + "@localhost>"
}

// StartRevisionForEdit opens a successor of an approved document for the
// editor, through the same Revise as the document page (FR-4.3). An open
// revision is returned as it is.
func (s *Server) StartRevisionForEdit(ctx context.Context, docID uuid.UUID, actor string) (*store.Document, bool, error) {
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return nil, false, err
	}
	if doc.State != lifecycle.DocApproved {
		return nil, false, fmt.Errorf("Only an approved document is revised; this one is %s, so it is edited as it stands.", doc.State)
	}
	if succ, err := s.liveSuccessor(ctx, doc.ID); err != nil {
		return nil, false, err
	} else if succ != nil {
		return succ, false, nil
	}
	if why := s.reviseRefusal(ctx, doc); why != "" {
		return nil, false, errors.New(why)
	}
	succ, err := s.ReviseDoc(ctx, doc.Path, actor)
	if err != nil {
		return nil, false, err
	}
	s.notifyEntityChanged("document", doc.ID)
	return succ, true, nil
}

// editWarnings are the sentences the editor shows before a person starts:
// what saving does to a document under review, and what a change means for
// work in flight (FR-4.2, FR-4.4).
func (s *Server) editWarnings(ctx context.Context, doc *store.Document) []string {
	var out []string
	if doc.State == lifecycle.DocReviewing {
		w := "This document is under review. Saving a change takes it back to draft and cancels its review, because the review was of the text as submitted. Submit it again when you're done."
		if s.agentReviewedType(doc.Type) {
			w += " A review already running will finish, and its verdict will be set aside."
		}
		out = append(out, w)
	}
	if doc.State == lifecycle.DocDraft && (doc.Type == "spec" || doc.Type == "dev_plan") && doc.OwnerType == "feature" && doc.OwnerID != nil {
		// A sent feature's draft that waits for its author is refused
		// (authorDueRefusal), unless the Inbox asks about another round.
		if waits, err := s.waitsForAuthor(ctx, doc); err == nil && waits {
			if s.pendingKindOn(ctx, "authoring-deadlock", "document", doc.ID) {
				out = append(out, "This draft still has review findings, and the Inbox asks whether to give its author agent another round. If you address them yourself, submit it when you're done, and answer the question so the author isn't sent back to it.")
			} else {
				out = append(out, "This draft has open review findings. Its feature hasn't been sent to development, so no author agent will revise it: address them yourself, and submit it when you're done.")
			}
		}
	}
	words := docTypeWords(doc.Type)
	switch {
	case doc.OwnerType == "feature" && doc.OwnerID != nil && (doc.Type == "spec" || doc.Type == "dev_plan"):
		f, err := store.GetFeature(ctx, s.Store.Pool, *doc.OwnerID)
		if err != nil {
			break
		}
		name := withID(f.PublicID, f.Name)
		if f.State == lifecycle.FeatActive || f.State == lifecycle.FeatReview {
			out = append(out, fmt.Sprintf("%s is being built. Its tasks are given the approved %s, so a change reaches them only once a revision is approved. Submitting a revised %s stops new tasks starting until you answer a question in the Inbox.", name, words, words))
		} else if sent, _ := s.featureSent(ctx, f); sent {
			out = append(out, fmt.Sprintf("%s has been sent to development, and this %s is part of its contract. The next steps are written from the approved %s, so your change reaches them once it is approved.", name, words, words))
		}
	case doc.Type == "design" && (doc.State == lifecycle.DocApproved || doc.SupersedesID != nil):
		// A revision of a design is what starts the cascade; a first design
		// has no specifications written from it yet.
		if names := s.sentBuildersOf(ctx, doc); len(names) > 0 {
			verb, have := "builds", "has"
			if len(names) > 1 {
				verb, have = "build", "have"
			}
			out = append(out, fmt.Sprintf("%s %s from this design and %s been sent to development, so approving a revision of it starts the cascade over their specifications.",
				joinAnd(names), verb, have))
		}
	}
	return out
}

// sentBuildersOf names the sent or building features that build from a
// design: its own feature, or its initiative's direct child features (G0).
func (s *Server) sentBuildersOf(ctx context.Context, doc *store.Document) []string {
	if doc.OwnerID == nil {
		return nil
	}
	var features []store.Feature
	switch doc.OwnerType {
	case "feature":
		f, err := store.GetFeature(ctx, s.Store.Pool, *doc.OwnerID)
		if err != nil {
			return nil
		}
		features = []store.Feature{*f}
	case "initiative":
		fs, err := store.FeaturesForInitiative(ctx, s.Store.Pool, *doc.OwnerID)
		if err != nil {
			return nil
		}
		features = fs
	}
	var names []string
	for i := range features {
		if sent, _ := s.featureSent(ctx, &features[i]); sent {
			names = append(names, withID(features[i].PublicID, features[i].Name))
		}
	}
	return names
}

func joinAnd(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " and " + xs[len(xs)-1]
}

// revisionNotes is what the approved document's editor page says before a
// revision is started (FR-4.3).
func (s *Server) revisionNotes(ctx context.Context, doc *store.Document) []string {
	next := ""
	if doc.PublicID != "" {
		if maxRev, _, err := store.IdentityUse(ctx, s.Store.Pool, doc.PublicID); err == nil {
			next = fmt.Sprintf(" with the same ID at revision %d", maxRev+1)
		}
	}
	ext := filepath.Ext(doc.Path)
	working := strings.TrimSuffix(doc.Path, ext) + ".rev" + ext
	notes := []string{fmt.Sprintf("This document is approved, so it isn't changed in place. Editing it starts a revision: a successor draft%s, in a working copy at %s. The original stays approved, and in force, until the revision is approved; then the revision takes its place and the original is archived.", next, working)}
	switch doc.Type {
	case "design":
		notes = append(notes, "Approving a revised design starts the cascade. Subutai finds the approved specifications written from this design. If there is one, it is superseded, or, for a feature being built, rewritten as a revision; if there are several, one question in the Inbox asks, spec by spec, whether to keep or redo each. A feature not yet sent to development is left without a specification until it is sent.")
	case "spec":
		if doc.OwnerType == "feature" && doc.OwnerID != nil {
			if f, err := store.GetFeature(ctx, s.Store.Pool, *doc.OwnerID); err == nil && (f.State == lifecycle.FeatIdea || f.State == lifecycle.FeatReady) {
				notes = append(notes, "Approving a revised specification also supersedes this feature's approved plan, so the plan is written again, and a feature that was ready to build goes back to waiting for it.")
			}
		}
	}
	return notes
}
