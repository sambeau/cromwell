package server

// Bugs (SPEC-019, DESIGN-010 §9). Anyone reports; a person decides whether it
// is fixed; from acceptance a bug travels the feature pipeline.
//
// A bug is a features row with kind 'bug' (SD-1), so almost everything after
// acceptance is the feature code as it stands. What lives here is what only a
// bug has: reporting it, with its report written from the bug_report
// template (FR-1, FR-3); triage (FR-2); and the one fact the pipeline asks of
// a bug, that its spec is its report (FR-4). Every act is a service method the
// web UI, the MCP tools and the agent tool share (NFR-1).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/config"
	"subutai/internal/ident"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
	"subutai/internal/toolhost"
)

// ---- The spec of a bug is its report (FR-4.2, SD-16) ----

// specTypeOf is the document type that is a feature row's spec: a bug's
// report, or a feature's spec.
func specTypeOf(f *store.Feature) string {
	if f != nil && f.IsBug() {
		return lifecycle.DocTypeBugReport
	}
	return lifecycle.DocTypeSpec
}

// specTypeFor is specTypeOf by row id. A missing row reads as a feature.
func (s *Server) specTypeFor(ctx context.Context, featureID uuid.UUID) string {
	f, err := store.GetFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return lifecycle.DocTypeSpec
	}
	return specTypeOf(f)
}

// bugOrFeature names a row the way a prompt should.
func bugOrFeature(f *store.Feature) string {
	if f.IsBug() {
		return "bug"
	}
	return "feature"
}

// bugAuthorNote is what the spec author is told when it revises a bug's
// report (SPEC-019 SD-7): the report is the spec, and its reproduction is the
// reporter's evidence, not the author's to invent.
const bugAuthorNote = "# This is a bug report\n\n" +
	"The document is a bug's report, which serves as its specification. It translates no design: " +
	"it says how to reproduce a defect, what should happen, and what happens instead. Revise it so the " +
	"findings and issues below are dealt with, making the reproduction and the acceptance criteria clear " +
	"and testable. Never invent reproduction steps, expected behaviour or facts the report and the findings " +
	"don't support: where something the reviewer asks for isn't known, say so plainly in Notes. Keep the " +
	"acceptance criterion \"The defect no longer reproduces\"."

// contractDocType maps a contract type a caller asks for to the one the row
// has: "spec" is a bug's report.
func (s *Server) contractDocType(ctx context.Context, featureID uuid.UUID, docType string) string {
	if docType == lifecycle.DocTypeSpec {
		return s.specTypeFor(ctx, featureID)
	}
	return docType
}

// checkContractOwner is the ownership rule for the spec half of a contract
// (R19-4): a bug's report belongs to a bug, and a bug has no spec — its
// report is its spec. Every registration path runs it.
func checkContractOwner(ctx context.Context, q store.Querier, docType, ownerType string, ownerID *uuid.UUID) error {
	if !lifecycle.IsSpecType(docType) {
		return nil
	}
	isBug := false
	if ownerType == "feature" && ownerID != nil {
		if f, err := store.GetFeature(ctx, q, *ownerID); err == nil {
			isBug = f.IsBug()
		}
	}
	switch {
	case docType == lifecycle.DocTypeBugReport && !isBug:
		return errors.New("A bug report belongs to a bug. Report a bug to write one, rather than attaching a report to a feature.")
	case docType == lifecycle.DocTypeSpec && isBug:
		return errors.New("A bug has no separate specification: its report is its specification. Attach the file as a note, or revise the report.")
	}
	return nil
}

// ---- Reporting (FR-1.3, FR-1.4, FR-3) ----

// BugReport is everything a report says, and who is making it.
type BugReport struct {
	InitiativeID    uuid.UUID
	OriginFeatureID *uuid.UUID
	Title           string
	Summary         string
	Steps           string
	Expected        string
	Actual          string
	Where           string
	Notes           string

	ReporterKind string // store.Reporter*
	Actor        string
	DispatchID   *uuid.UUID
	TaskID       *uuid.UUID
	Via          string // ui | mcp | agent | review
}

// reportOn resolves where a bug hangs (SD-3): an initiative, or a feature (a
// bug included) as its origin, whose initiative it hangs under.
func (s *Server) reportOn(ctx context.Context, ownerType string, ownerID uuid.UUID) (uuid.UUID, *uuid.UUID, error) {
	switch ownerType {
	case "initiative":
		in, err := store.GetInitiative(ctx, s.Store.Pool, ownerID)
		if err != nil {
			return uuid.Nil, nil, err
		}
		if in.Archived {
			return uuid.Nil, nil, errors.New("That initiative is archived, so a bug can't be reported on it.")
		}
		return in.ID, nil, nil
	case "feature":
		f, err := store.GetFeature(ctx, s.Store.Pool, ownerID)
		if err != nil {
			return uuid.Nil, nil, err
		}
		id := f.ID
		return f.InitiativeID, &id, nil
	}
	return uuid.Nil, nil, fmt.Errorf("a bug hangs off an initiative or a feature, not %s", ownerType)
}

// checkReport refuses a report with a required part missing, in a sentence.
func checkReport(r *BugReport) error {
	r.Title = strings.TrimSpace(r.Title)
	r.Steps = strings.TrimSpace(r.Steps)
	r.Expected = strings.TrimSpace(r.Expected)
	r.Actual = strings.TrimSpace(r.Actual)
	r.Summary = strings.TrimSpace(r.Summary)
	r.Where = strings.TrimSpace(r.Where)
	r.Notes = strings.TrimSpace(r.Notes)
	var missing []string
	for _, f := range []struct{ name, v string }{
		{"a title", r.Title}, {"the steps to reproduce it", r.Steps},
		{"what should happen", r.Expected}, {"what happens instead", r.Actual},
	} {
		if f.v == "" {
			missing = append(missing, f.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("A bug report needs %s.", joinWords(missing))
	}
	if strings.ContainsAny(r.Title, "\r\n") {
		return errors.New("A bug's title is one line.")
	}
	return nil
}

func joinWords(ws []string) string {
	switch len(ws) {
	case 0:
		return ""
	case 1:
		return ws[0]
	}
	return strings.Join(ws[:len(ws)-1], ", ") + " and " + ws[len(ws)-1]
}

// ReportBug creates a bug and its report (FR-1.3, FR-1.4): the row, its bugs
// row and audit rows, and the report from the project's template at its
// default home, registered as a draft, the bug's main document, and
// committed on its own. Nothing is dispatched: a reported bug waits for
// triage (NFR-2).
func (s *Server) ReportBug(ctx context.Context, r BugReport) (*store.Feature, *store.Document, error) {
	if err := checkReport(&r); err != nil {
		return nil, nil, err
	}
	tmpl, err := os.ReadFile(filepath.Join(s.CompartmentRoot, "templates", "bug_report", "template.md"))
	if err != nil {
		return nil, nil, errors.New("This project has no bug report template yet (templates/bug_report in its " +
			".subutai folder), so a bug can't be reported. Copy it from a fresh subutai init.")
	}
	if r.Summary == "" {
		r.Summary = asSentence(r.Title)
	}
	// Check the report before anything is minted, so a refusal costs no
	// number and the reporter hears at once what to change (R19-3, R19-6).
	manifest, err := config.LoadManifest(s.CompartmentRoot, lifecycle.DocTypeBugReport)
	if err != nil {
		return nil, nil, errors.New("This project's bug report template has no manifest, so a bug can't be reported. Copy templates/bug_report from a fresh subutai init.")
	}
	if rep := lifecycle.Validate(manifest, fillBugReport(string(tmpl), r, "owner"), nil); !rep.Valid {
		return nil, nil, fmt.Errorf("The report wouldn't pass its template's checks: %s", reportSentence(&rep))
	}
	var f *store.Feature
	var doc *store.Document
	var path string
	undo := func() {}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		f, err = store.CreateBug(ctx, tx, store.NewBug{
			InitiativeID: r.InitiativeID, OriginFeatureID: r.OriginFeatureID,
			Title: r.Title, Description: r.Summary,
			ReporterKind: r.ReporterKind, ReportedBy: r.Actor,
			ReportedDispatchID: r.DispatchID, SourceTaskID: r.TaskID, Via: r.Via,
		})
		if err != nil {
			return err
		}
		ip, err := initiativePathIn(ctx, tx, f.InitiativeID)
		if err != nil {
			return err
		}
		oid := f.ID
		id, revision, err := store.NextDocumentIdentity(ctx, tx, "feature", &oid, lifecycle.DocTypeBugReport)
		if err != nil {
			return err
		}
		home, err := defaultHome(ctx, tx, f.InitiativeID)
		if err != nil {
			return err
		}
		path = s.freePath(filepath.Join(home, id))
		abs := filepath.Join(s.RepoRoot, path)
		body, err := stampIdentity(fillBugReport(string(tmpl), r, ip+"/"+f.Slug), id, revision)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			return err
		}
		undo = func() { _ = os.Remove(abs) }
		doc, err = registerInTx(ctx, tx, path, []byte(body), lifecycle.DocTypeBugReport, "feature", &oid, nil,
			id, revision, r.Actor, s.bugWriter(r))
		if err != nil {
			return err
		}
		return store.SetPrimaryDocument(ctx, tx, doc.ID, r.Actor)
	})
	if err != nil {
		undo()
		return nil, nil, err
	}
	s.commitDocument(path, fmt.Sprintf("subutai: %s reported, %s", f.PublicID, f.Name))
	s.notifyEntityChanged("feature", f.ID)
	s.notifyEntityChanged("triage", uuid.Nil)
	s.Bus.Publish(bus.FeatureCreated{FeatureID: f.ID})
	return f, doc, nil
}

// bugWriter records who wrote the report: whoever reported the bug
// (SPEC-017 FR-2).
func (s *Server) bugWriter(r BugReport) writerAct {
	switch r.ReporterKind {
	case store.ReporterAgent, store.ReporterReview:
		w := writerAct{Act: store.ActWrote, Kind: store.WriterAgent, Actor: r.Actor, DispatchID: r.DispatchID, Via: "agent"}
		if r.DispatchID != nil {
			if d, err := store.GetDispatch(context.Background(), s.Store.Pool, *r.DispatchID); err == nil {
				w.Model = d.Model
			}
		}
		return w
	case store.ReporterChat:
		return writerAct{Act: store.ActWrote, Kind: store.WriterChat, Actor: r.Actor, Via: "mcp"}
	}
	return writerAct{Act: store.ActWrote, Kind: store.WriterPerson, Actor: r.Actor, Via: "ui"}
}

// asSentence ends a title with a full stop, for a summary nobody wrote.
func asSentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s[len(s)-1:], ".!?") {
		return s
	}
	return s + "."
}

// fillBugReport fills the template's placeholders from the report, in one
// pass, so a reporter's text that happens to contain "{{notes}}" isn't taken
// for a placeholder. A section whose placeholder has nothing to say is taken
// out whole, heading and all, so an optional section is omitted rather than
// left empty. Quoted material — a log, a trace, a heading — is fenced, so it
// stays text (R19-3).
func fillBugReport(tmpl string, r BugReport, owner string) string {
	values := map[string]string{
		"title":    r.Title,
		"owner":    owner,
		"summary":  r.Summary,
		"steps":    numbered(r.Steps),
		"expected": r.Expected,
		"actual":   literal(r.Actual),
		"where":    r.Where,
		"notes":    literal(r.Notes),
	}
	// Drop the sections with nothing in them.
	lines := strings.Split(tmpl, "\n")
	var kept []string
	for i := 0; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end := i + 1
			for end < len(lines) && !strings.HasPrefix(lines[end], "## ") {
				end++
			}
			body := strings.TrimSpace(strings.Join(lines[i+1:end], "\n"))
			if key, ok := strings.CutPrefix(body, "{{"); ok {
				if key, ok = strings.CutSuffix(key, "}}"); ok && strings.TrimSpace(values[key]) == "" {
					i = end - 1
					continue
				}
			}
		}
		kept = append(kept, lines[i])
	}
	pairs := []string{`"{{title}}"`, yamlQuote(r.Title), `"{{owner}}"`, yamlQuote(owner)}
	for k, v := range values {
		pairs = append(pairs, "{{"+k+"}}", v)
	}
	out := strings.NewReplacer(pairs...).Replace(strings.Join(kept, "\n"))
	return strings.TrimRight(out, "\n") + "\n"
}

// literal fences text that would otherwise be read as the document's own
// structure or as an unfinished placeholder: a heading line, template braces,
// or the word TODO.
func literal(v string) string {
	v = strings.TrimSpace(v)
	if v == "" || strings.Contains(v, "```") {
		return v
	}
	risky := strings.Contains(v, "{{") || strings.Contains(v, "TODO")
	for _, l := range strings.Split(v, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "#") {
			risky = true
		}
	}
	if !risky {
		return v
	}
	return "```text\n" + v + "\n```"
}

// numbered makes the steps a numbered list when they aren't a list already,
// because the template's rule wants at least one list item.
func numbered(steps string) string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(steps), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			lines = append(lines, l)
		}
	}
	for _, l := range lines {
		if isListItem(l) {
			return strings.Join(lines, "\n")
		}
	}
	for i := range lines {
		lines[i] = fmt.Sprintf("%d. %s", i+1, lines[i])
	}
	return strings.Join(lines, "\n")
}

func isListItem(l string) bool {
	if strings.HasPrefix(l, "- ") || strings.HasPrefix(l, "* ") || strings.HasPrefix(l, "+ ") {
		return true
	}
	i := 0
	for i < len(l) && l[i] >= '0' && l[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(l) && (l[i] == '.' || l[i] == ')') && l[i+1] == ' '
}

// ---- Triage (FR-2) ----

// Triage decisions, as a person or a relay names them.
const (
	decideAccept    = "accept"
	decideReject    = "reject"
	decideDuplicate = "duplicate"
)

// DecideTriage is a person's triage decision (FR-2.2), from the queue, the
// bug's page or a relay. Accepting makes the bug committed scope; rejecting
// it or marking it a duplicate abandons its row with the reason, in the same
// transaction (SD-2).
func (s *Server) DecideTriage(ctx context.Context, bugID uuid.UUID, decision, reason, duplicateOf string, act relayAct) (*store.Bug, error) {
	if act.Via == "mcp" && strings.TrimSpace(act.Quote) == "" {
		return nil, errors.New("A triage decision relayed from chat needs the person's own words, quoted.")
	}
	reason = strings.TrimSpace(reason)
	d := store.TriageDecision{Actor: act.Actor, Via: act.Via, Quote: strings.TrimSpace(act.Quote), Reason: reason}
	var original *store.Bug
	switch decision {
	case decideAccept:
		d.Decision = store.TriageAccepted
	case decideReject:
		d.Decision = store.TriageRejected
		if reason == "" {
			return nil, errors.New("Say why it's rejected, so whoever reported it can see.")
		}
	case decideDuplicate:
		d.Decision = store.TriageDuplicate
		o, err := s.bugByRef(ctx, duplicateOf)
		if err != nil {
			return nil, err
		}
		if o.Feature.ID == bugID {
			return nil, errors.New("A bug can't be a duplicate of itself. Name the bug it repeats.")
		}
		if o.Triage == store.TriageDuplicate && o.DuplicateOf != nil {
			root, err := store.GetBug(ctx, s.Store.Pool, *o.DuplicateOf)
			name := "the bug it repeats"
			if err == nil {
				name = root.Feature.PublicID
			}
			return nil, fmt.Errorf("%s is itself a duplicate, of %s. Mark this one a duplicate of %s instead.",
				o.Feature.PublicID, name, name)
		}
		original = o
		id := o.Feature.ID
		d.DuplicateOf = &id
		if d.Reason == "" {
			d.Reason = "Duplicate of " + o.Feature.PublicID + "."
		}
	default:
		return nil, fmt.Errorf("A triage decision is accept, reject or duplicate, not %q.", decision)
	}
	var b *store.Bug
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		b, err = store.GetBugForUpdate(ctx, tx, bugID)
		if err == store.ErrNotFound {
			return errors.New("That isn't a bug, so it has no triage.")
		}
		if err != nil {
			return err
		}
		if b.Triage != store.TriageReported {
			return fmt.Errorf("%s was already %s%s, so there is nothing to triage.",
				b.Feature.PublicID, b.Triage, whenWords(b))
		}
		if err := store.RecordTriage(ctx, tx, b, d); err != nil {
			return err
		}
		if d.Decision != store.TriageAccepted && !b.Feature.State.Terminal() {
			why := "rejected in triage: " + d.Reason
			if original != nil {
				why = "marked in triage as a duplicate of " + original.Feature.PublicID
			}
			if err := store.TransitionFeature(ctx, tx, &b.Feature, lifecycle.FeatAbandon, act.Actor,
				map[string]any{"reason": why, "via": act.Via, "triage": d.Decision}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	s.notifyEntityChanged("feature", bugID)
	s.notifyEntityChanged("triage", uuid.Nil)
	return store.GetBug(ctx, s.Store.Pool, bugID)
}

func whenWords(b *store.Bug) string {
	if b.DecidedAt == nil {
		return ""
	}
	return " on " + b.DecidedAt.Format("2 January 2006")
}

// bugByRef resolves a bug by its ID, "BUG-007", or its row id.
func (s *Server) bugByRef(ctx context.Context, ref string) (*store.Bug, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("Name the bug it repeats, by its ID, such as BUG-004.")
	}
	if id, err := uuid.Parse(ref); err == nil {
		b, err := store.GetBug(ctx, s.Store.Pool, id)
		if err == store.ErrNotFound {
			return nil, errors.New("That isn't a bug.")
		}
		return b, err
	}
	r, ok := ident.Parse(ref)
	if !ok || r.Shape != ident.ShapeEntity || r.Kind.Name != "bug" {
		return nil, fmt.Errorf("%q isn't a bug's ID. A bug's ID looks like BUG-004.", ref)
	}
	f, err := store.FeatureByPublicID(ctx, s.Store.Pool, r.ID)
	if err == store.ErrNotFound {
		return nil, fmt.Errorf("There is no %s.", r.ID)
	}
	if err != nil {
		return nil, err
	}
	return store.GetBug(ctx, s.Store.Pool, f.ID)
}

// ---- Sending a bug (FR-4.1, FR-4.3) ----

// bugSendReadiness is sendReadiness's G0 and description checks, for a bug:
// acceptance stands in for G0, and a report stands in for the description
// (DESIGN-010 §9, §17a item 6).
func (s *Server) bugSendReadiness(ctx context.Context, f *store.Feature) (bool, string) {
	b, err := store.GetBug(ctx, s.Store.Pool, f.ID)
	if err != nil {
		return false, "This bug's triage record is missing, so it can't be sent."
	}
	switch b.Triage {
	case store.TriageReported:
		return false, "This bug hasn't been triaged yet. A person accepts it in the triage queue before it can be sent to development."
	case store.TriageRejected:
		return false, "This bug was rejected in triage, so it won't be sent to development."
	case store.TriageDuplicate:
		return false, "This bug was marked as a duplicate in triage, so it won't be sent to development. Send the bug it repeats."
	}
	if _, err := store.CurrentDocForOwner(ctx, s.Store.Pool, lifecycle.DocTypeBugReport, "feature", f.ID); err != nil {
		return false, "This bug has no report attached, and its report is its specification. Attach or write one before sending it."
	}
	// A bug found while its origin is being built may be in code that
	// isn't on the main line yet, and a bug's worktree is made from the
	// main line. So it waits for the origin to merge (R19-2).
	if b.OriginFeatureID != nil {
		if o, err := store.GetFeature(ctx, s.Store.Pool, *b.OriginFeatureID); err == nil &&
			(o.State == lifecycle.FeatActive || o.State == lifecycle.FeatReview) {
			return false, fmt.Sprintf("This bug was reported on %s while it is being built, so the code it describes may not be on the main line yet. "+
				"It can be sent once %s has merged.", o.PublicID, o.PublicID)
		}
	}
	return true, ""
}

// checkReportSubmittable validates a bug's draft report before the send
// writes the mark, so a report that can't be submitted stops the send with
// its problems (FR-4.3). It returns the report to submit after the mark, or
// nil when there is nothing to submit.
func (s *Server) checkReportSubmittable(ctx context.Context, f *store.Feature) (*store.Document, error) {
	doc, err := store.CurrentDocForOwner(ctx, s.Store.Pool, lifecycle.DocTypeBugReport, "feature", f.ID)
	if err != nil || doc.State != lifecycle.DocDraft {
		return nil, ignoreNotFound(err)
	}
	if waits, err := s.waitsForAuthor(ctx, doc); err != nil || waits {
		return nil, err // the author revises it (FR-4.2)
	}
	report, err := s.ValidateDoc(ctx, doc.Path)
	if err != nil {
		return nil, err
	}
	if !report.Valid {
		return nil, fmt.Errorf("Its report doesn't pass validation yet, so it wasn't sent. %s", reportSentence(report))
	}
	return doc, nil
}

// refuseReportBeforeAcceptance refuses to submit any document a bug owns —
// its report, or a plan written ahead in chat — while the bug waits for
// triage, or once triage has closed it (SD-6, FR-4.4, R19-4). Reviewing
// spends agent time, and accepting a bug is the commitment of scope.
func (s *Server) refuseReportBeforeAcceptance(ctx context.Context, doc *store.Document) error {
	if doc.OwnerType != "feature" || doc.OwnerID == nil {
		return nil
	}
	b, err := store.GetBug(ctx, s.Store.Pool, *doc.OwnerID)
	if err != nil {
		return ignoreNotFound(err)
	}
	switch b.Triage {
	case store.TriageAccepted:
		return nil
	case store.TriageReported:
		return errors.New("A bug's documents are reviewed once the bug has been accepted in triage.")
	}
	return fmt.Errorf("This bug was %s in triage, so its documents aren't reviewed.", b.Triage)
}

// ---- Agents at work (FR-3.3, SD-10) ----

// maxReportsPerRun is how many bugs one run may file (SD-10).
const maxReportsPerRun = 3

// toolReportBug is the report_bug tool: an agent files a defect outside its
// task, and carries on. Every refusal goes back to the agent as a tool error.
func (s *Server) toolReportBug(ctx context.Context, tctx *toolhost.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Title    string `json:"title"`
		Steps    string `json:"steps"`
		Expected string `json:"expected"`
		Actual   string `json:"actual"`
		Notes    string `json:"notes"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "invalid arguments: " + err.Error(), true
	}
	dispatchID, err := uuid.Parse(tctx.DispatchID)
	if err != nil {
		return "report_bug can't tell which run is calling it, so nothing was filed.", true
	}
	featureID, err := uuid.Parse(tctx.FeatureID)
	if err != nil {
		return "report_bug can't tell which feature this run is working on, so nothing was filed.", true
	}
	n, err := store.CountBugsByDispatch(ctx, s.Store.Pool, dispatchID)
	if err != nil {
		return "could not check this run's reports: " + err.Error(), true
	}
	if n >= maxReportsPerRun {
		return fmt.Sprintf("This run has already filed %d bug reports, the most one run may file. "+
			"Mention anything else in your outcome's reasoning, and get on with your task.", n), true
	}
	if dup, err := store.OpenReportedBugByTitle(ctx, s.Store.Pool, featureID, args.Title); err == nil {
		return fmt.Sprintf("%s, %q, already reports this and is waiting for triage, so nothing was filed. Carry on with your task.",
			dup.Feature.PublicID, dup.Feature.Name), true
	}
	f, err := store.GetFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return "could not load this run's feature: " + err.Error(), true
	}
	where := "Found by the " + roleWords(tctx.Role) + " while working on " + f.PublicID + ", " + f.Name + "."
	var taskID *uuid.UUID
	if tid, err := uuid.Parse(tctx.TaskID); err == nil {
		if t, err := store.GetTask(ctx, s.Store.Pool, tid); err == nil {
			where = "Found by the " + roleWords(tctx.Role) + " while working on " + t.PublicID + ", " + t.Title + "."
			taskID = &tid
		}
	}
	where += " The run is " + dispatchID.String() + "."
	bug, _, err := s.ReportBug(ctx, BugReport{
		InitiativeID: f.InitiativeID, OriginFeatureID: &f.ID,
		Title: args.Title, Steps: args.Steps, Expected: args.Expected, Actual: args.Actual,
		Where: where, Notes: args.Notes,
		ReporterKind: store.ReporterAgent, Actor: tctx.Role, DispatchID: &dispatchID, TaskID: taskID, Via: "agent",
	})
	if err != nil {
		return err.Error(), true
	}
	return fmt.Sprintf("Filed %s: %s. It waits for a person to triage it. Carry on with your own task; don't fix it here.",
		bug.PublicID, bug.Name), false
}

// ---- Minor findings from code review (FR-3.4, SD-9) ----

// fileMinorFindings files a merged feature's code-review minor findings as one
// bug on it, reported by the code reviewer (SD-9). They are filed at the
// merge, not the approval, because the code they describe reaches the main
// line only then, and a bug's worktree is made from the main line (R19-2).
// Best effort: the merge has already happened.
func (s *Server) fileMinorFindings(ctx context.Context, f *store.Feature) (*store.Feature, error) {
	sets, err := store.MinorFindingsForFeature(ctx, s.Store.Pool, f.ID)
	if err != nil || len(sets) == 0 {
		return nil, err
	}
	var actual strings.Builder
	actual.WriteString("The code reviews approved these tasks, and noted minor findings that didn't send the work back:\n")
	n := 0
	reviewer, run := sets[len(sets)-1].Reviewer, sets[len(sets)-1].DispatchID
	for _, set := range sets {
		fmt.Fprintf(&actual, "\nOn %s, %s:\n\n", set.TaskPublicID, strings.TrimSpace(set.TaskTitle))
		for _, c := range set.Comments {
			n++
			if c.SectionRef != "" {
				fmt.Fprintf(&actual, "- %s (at %s)\n", strings.TrimSpace(c.Body), c.SectionRef)
			} else {
				fmt.Fprintf(&actual, "- %s\n", strings.TrimSpace(c.Body))
			}
		}
		if set.DispatchID != nil {
			reviewer, run = set.Reviewer, set.DispatchID
		}
	}
	if run == nil {
		if run, err = store.LatestReviewRunForFeature(ctx, s.Store.Pool, f.ID); err != nil || run == nil {
			return nil, fmt.Errorf("no code review run to name as the reporter: %v", err)
		}
	}
	bug, _, err := s.ReportBug(ctx, BugReport{
		InitiativeID: f.InitiativeID, OriginFeatureID: &f.ID,
		Title:   fmt.Sprintf("Minor review findings on %s: %s", f.PublicID, strings.TrimSpace(f.Name)),
		Summary: fmt.Sprintf("The code reviews of %s approved its tasks with %d minor finding(s), filed here so they aren't lost.", f.PublicID, n),
		Steps: "1. Read the changes " + f.PublicID + " made, now on the main line.\n" +
			"2. Look at each place a finding names.",
		Expected:     "Each finding is dealt with, or recorded as not worth doing.",
		Actual:       strings.TrimSpace(actual.String()),
		Where:        fmt.Sprintf("The code reviews of %s, %s.", f.PublicID, f.Name),
		ReporterKind: store.ReporterReview, Actor: reviewer, DispatchID: run, Via: "review",
	})
	return bug, err
}
