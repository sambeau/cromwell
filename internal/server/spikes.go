package server

// Spikes (SPEC-021, DESIGN-010 §10). A spike is a question with a budget: one
// agent works on it in a throwaway worktree, saves its findings as it goes,
// and the findings are written up when the run ends, however it ends. A spike
// is its own row (SD-1), so nothing here touches the feature code, and there
// is no merge: the worktree is detached, discarded at the end, and a run that
// keeps its code on a branch is caught (SD-2). Every act is a service method
// that the web UI, the MCP tools and the rules share (NFR-1).
//
// This file holds creating, starting and closing a spike, the findings
// document, and the run's tool. Ending a run and its worktree are in
// spikes_end.go; the run's plan is in spikes_plan.go.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/config"
	"subutai/internal/content"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
	"subutai/internal/toolhost"
)

// The refusals a person reads (NFR-7). Each is a full sentence.
var (
	// ErrSpikeStarted is what starting a spike that isn't an idea says.
	ErrSpikeStarted = errors.New("This spike has already been started.")
	// ErrSpikeNotClosable is what closing a spike that can't be closed says.
	ErrSpikeNotClosable = errors.New("This spike can't be closed now: it is still running, or it is already closed.")
	// ErrNoSpikeRunner is what starting says when run-spike has no role.
	ErrNoSpikeRunner = errors.New("Nobody is assigned to run spikes, so this spike can't start. Assign `run-spike` to a role in `config.yaml`.")
)

// spikeOwnsDocumentsRefusal is what attaching or adopting a document to a spike
// says (Appendix A): its one document is written by its run.
const spikeOwnsDocumentsRefusal = "A spike's only document is its findings, which its run writes."

// spikeQuestionMax is the longest question a spike may ask (FR-1.1).
const spikeQuestionMax = 500

// ---- Creating (FR-1.3) ----

// spikeOwner resolves where a spike hangs (SD-3): a live initiative, or a
// feature that isn't done or abandoned, whose initiative it hangs under.
func spikeOwner(ctx context.Context, q store.Querier, ownerType string, ownerID uuid.UUID) (uuid.UUID, *uuid.UUID, error) {
	switch ownerType {
	case "initiative":
		in, err := store.GetInitiative(ctx, q, ownerID)
		if err != nil {
			return uuid.Nil, nil, err
		}
		if in.Archived {
			return uuid.Nil, nil, errors.New("That initiative is archived, so a spike can't be created on it.")
		}
		return in.ID, nil, nil
	case "feature":
		f, err := store.GetFeature(ctx, q, ownerID)
		if err != nil {
			return uuid.Nil, nil, err
		}
		if f.State.Terminal() {
			return uuid.Nil, nil, fmt.Errorf("That %s is %s, so a spike can't be created on it.", bugOrFeature(f), f.State)
		}
		in, err := store.GetInitiative(ctx, q, f.InitiativeID)
		if err != nil {
			return uuid.Nil, nil, err
		}
		if in.Archived {
			return uuid.Nil, nil, errors.New("That initiative is archived, so a spike can't be created on it.")
		}
		id := f.ID
		return f.InitiativeID, &id, nil
	}
	return uuid.Nil, nil, errors.New("A spike is created on an initiative or a feature.")
}

// checkSpikeQuestion trims and checks a question, in sentences.
func checkSpikeQuestion(q string) (string, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return "", errors.New("A spike needs a question: one sentence saying what it should find out.")
	}
	if n := utf8.RuneCountInString(q); n > spikeQuestionMax {
		return "", fmt.Errorf("A spike's question is at most %d characters; this one is %d.", spikeQuestionMax, n)
	}
	return q, nil
}

func checkSpikeBudget(budget *int64) error {
	if budget != nil && *budget <= 0 {
		return errors.New("A spike's budget is a positive whole number of tokens.")
	}
	return nil
}

// createSpikeIn is CreateSpike in the caller's transaction. Every refusal
// happens before the row is inserted, so a refusal costs no number (FR-1.3).
func (s *Server) createSpikeIn(ctx context.Context, tx pgx.Tx, ownerType string, ownerID uuid.UUID, question string,
	budget *int64, follows *uuid.UUID, actor, via string) (*store.Spike, error) {
	question, err := checkSpikeQuestion(question)
	if err != nil {
		return nil, err
	}
	if err := checkSpikeBudget(budget); err != nil {
		return nil, err
	}
	initiativeID, featureID, err := spikeOwner(ctx, tx, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	return store.CreateSpike(ctx, tx, store.NewSpike{
		InitiativeID: initiativeID, FeatureID: featureID, Question: question,
		BudgetOverride: budget, FollowsID: follows, CreatedBy: actor, CreatedVia: via,
	})
}

// CreateSpike writes down a question on an initiative or a feature (ownerType
// "initiative" or "feature"), with an optional budget of its own. It is the one
// method the web UI and the MCP tool call. It dispatches nothing: a spike is
// started only by a person, in the web UI (SD-5, NFR-2). via is ui or mcp.
func (s *Server) CreateSpike(ctx context.Context, ownerType string, ownerID uuid.UUID, question string,
	budget *int64, actor, via string) (*store.Spike, error) {
	var sp *store.Spike
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		sp, err = s.createSpikeIn(ctx, tx, ownerType, ownerID, question, budget, nil, actor, via)
		return err
	})
	if err != nil {
		return nil, err
	}
	s.notifySpikeChanged(sp)
	return sp, nil
}

// notifySpikeChanged signals the spike's page and its owner's page, which
// lists it, after the commit that changed it (presentation only).
func (s *Server) notifySpikeChanged(sp *store.Spike) {
	s.notifyEntityChanged("spike", sp.ID)
	s.notifyEntityChanged(sp.Owner())
}

// ---- Starting (FR-3.3) ----

// spikeRunnerFor says who would run a spike: the run-spike role and the model
// it would be dispatched on, or the sentence that says why nobody can. The
// start screen and StartSpike both ask, so what the screen promises is what
// the start does (FR-3.2).
func (s *Server) spikeRunnerFor(cfg *config.Config) (role, model, refusal string) {
	role, ok := cfg.Assignments["run-spike"]
	if !ok || role == "" {
		return "", "", ErrNoSpikeRunner.Error()
	}
	model, err := s.modelForPurpose(cfg, "run-spike", role)
	if err != nil {
		return "", "", fmt.Sprintf("The spike runner's role, %s, can't be loaded (%v), so this spike can't start.", role, err)
	}
	if _, ok := cfg.Models[model]; !ok {
		return "", "", fmt.Sprintf("The spike runner's model, %s, isn't configured in config.yaml, so this spike can't start.", model)
	}
	return role, model, ""
}

// spikeBudgetFor is the budget a start with no figure of its own would give:
// the spike's override, else the project's default (SD-6).
func spikeBudgetFor(cfg *config.Config, sp *store.Spike) (budget int64, source string) {
	if sp.BudgetOverride != nil {
		return *sp.BudgetOverride, store.BudgetFromOverride
	}
	return cfg.SpikeDefaultTokenBudget(), store.BudgetFromDefault
}

// spikeWorktreeRel is where a spike's worktree lives, relative to the
// repository, as StartFeature's is (FR-3.3).
func (s *Server) spikeWorktreeRel(id uuid.UUID) string {
	return filepath.Join(filepath.Base(s.CompartmentRoot), "worktrees", store.ShortID("spk", id))
}

// StartSpike moves an idea to running and queues its run (FR-3.3). budget is
// the figure entered on the start screen; 0 means the spike's own override, or
// else the project default. The database commits first, as StartFeature's
// does, and the planner makes the worktree before the run's first call.
// Starting is a person's act in the web UI; no other caller exists (SD-5).
func (s *Server) StartSpike(ctx context.Context, spikeID uuid.UUID, budget int64, actor string) (*store.Spike, error) {
	return s.startSpike(ctx, spikeID, budget, actor, true)
}

// startSpike is StartSpike, with the dispatcher kicked only when kick is set,
// so a test can look at a started spike before its run begins.
func (s *Server) startSpike(ctx context.Context, spikeID uuid.UUID, budget int64, actor string, kick bool) (*store.Spike, error) {
	sp, err := store.GetSpike(ctx, s.Store.Pool, spikeID)
	if err != nil {
		return nil, err
	}
	if sp.State != store.SpikeIdea {
		return nil, ErrSpikeStarted
	}
	cfg, err := s.freshConfig()
	if err != nil {
		return nil, err
	}
	role, model, refusal := s.spikeRunnerFor(cfg)
	if refusal != "" {
		return nil, errors.New(refusal)
	}
	if _, err := os.ReadFile(filepath.Join(s.CompartmentRoot, "templates", "findings", "template.md")); err != nil {
		return nil, errors.New("This project has no findings template yet (templates/findings in its .subutai folder), so a spike can't start. Copy it from a fresh subutai init.")
	}
	if _, err := config.LoadManifest(s.CompartmentRoot, lifecycle.DocTypeFindings); err != nil {
		return nil, errors.New("This project's findings template has no manifest, so a spike can't start. Copy templates/findings from a fresh subutai init.")
	}
	if budget != 0 {
		if err := checkSpikeBudget(&budget); err != nil {
			return nil, err
		}
	}
	source := store.BudgetFromStartScreen
	if budget == 0 {
		budget, source = spikeBudgetFor(cfg, sp)
	} else if want, from := spikeBudgetFor(cfg, sp); want == budget {
		source = from
	}
	head, err := gitIn(s.RepoRoot, "rev-parse", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("The main checkout's current commit couldn't be read, so this spike can't start: %v", err)
	}
	// The refs as they stand, so the leak check can tell what the run changed
	// (FR-6.3).
	refs, err := s.snapshotRefs()
	if err != nil {
		return nil, fmt.Errorf("The repository's branches and tags couldn't be read, so this spike can't start: %v", err)
	}

	var started *store.Spike
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var err error
		started, err = store.StartSpike(ctx, tx, sp.ID, store.SpikeStart{
			Budget: budget, BudgetSource: source, BaseCommit: strings.TrimSpace(head),
			WorktreePath: s.spikeWorktreeRel(sp.ID), RefsAtStart: refs, Actor: actor,
		})
		if err != nil {
			return err
		}
		_, err = store.EnqueueDispatch(ctx, tx, "run-spike", role, model, "spike", sp.ID, "run-spike:"+sp.ID.String())
		return err
	})
	if errors.Is(err, store.ErrSpikeNotIdea) {
		return nil, ErrSpikeStarted
	}
	if err != nil {
		return nil, err
	}
	if kick {
		s.Dispatcher.Kick()
	}
	s.notifySpikeChanged(started)
	return started, nil
}

// ---- Closing (FR-7) ----

// SpikeCloseAgain is the third way a person closes an ended spike (FR-7.1),
// after store.SpikeAnswered and store.SpikeUnanswered: closing it without an
// answer and asking the same question again.
const SpikeCloseAgain = "again"

// CloseSpike closes a spike as a person has read it: as "answered" or
// "unanswered", or as "again", which closes it unanswered and creates a
// second spike with the same owner and question, a link back and the budget
// entered as its override, in one transaction (FR-7.2, FR-7.3). It returns the
// closed spike, or, for "again", the new one. A spike that can't be closed
// is refused with ErrSpikeNotClosable. Closing is a person's act in the web
// UI (SD-10).
func (s *Server) CloseSpike(ctx context.Context, id uuid.UUID, as string, budget *int64, actor string) (*store.Spike, error) {
	if as != store.SpikeAnswered && as != store.SpikeUnanswered && as != SpikeCloseAgain {
		return nil, errors.New("A spike is closed as answered, closed without an answer, or asked again.")
	}
	if as == SpikeCloseAgain {
		if err := checkSpikeBudget(budget); err != nil {
			return nil, err
		}
	}
	var closed, result *store.Spike
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if as == SpikeCloseAgain {
			// Only an ended spike is asked again: an idea has no answer to
			// improve on, and a running one isn't over.
			cur, err := store.GetSpike(ctx, tx, id)
			if err != nil {
				return err
			}
			if cur.State != store.SpikeEnded {
				return ErrSpikeNotClosable
			}
		}
		how := as
		if as == SpikeCloseAgain {
			how = store.SpikeUnanswered
		}
		var err error
		closed, err = store.CloseSpike(ctx, tx, id, how, actor)
		if errors.Is(err, store.ErrSpikeCannotClose) {
			return ErrSpikeNotClosable
		}
		if err != nil {
			return err
		}
		result = closed
		if as != SpikeCloseAgain {
			return nil
		}
		// The new spike's budget is the one entered; none entered keeps the
		// budget this spike had.
		if budget == nil {
			budget = closed.BudgetOverride
		}
		ownerType, ownerID := closed.Owner()
		follows := closed.ID
		result, err = s.createSpikeIn(ctx, tx, ownerType, ownerID, closed.Question, budget, &follows, actor, "ui")
		return err
	})
	if err != nil {
		return nil, err
	}
	s.notifySpikeChanged(closed)
	if result != closed {
		s.notifySpikeChanged(result)
	}
	return result, nil
}

// ---- The findings document (FR-2) ----

// findingsEnd is how a run ended, as the findings say it (FR-2.3).
type findingsEnd struct {
	How     string // store.SpikeConcluded ...
	Note    string // for a failure, its last error
	Used    int64
	Budget  int64
	TurnCap int
	// Refs are the refs that kept the run's code (FR-6.3); CheckFailed is why
	// the leak check couldn't run, when it couldn't.
	Refs        []string
	CheckFailed string
}

const (
	headingQuestion = "Question"
	headingAnswer   = "Answer"
	headingFound    = "What we found"
	headingHow      = "How we found out"
	headingNext     = "What to do next"
	headingEnded    = "How this spike ended"

	nothingSaved = "Nothing was saved before the run stopped."
)

// notAnswered is the Answer a run that stopped has when it saved none (SD-8).
func notAnswered(how string) string { return store.SpikeEndingOf(how).NotAnswered }

// oneLine puts text on one line, for a title or a sentence.
func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// endedSentence is the server's How this spike ended section (FR-2.3).
func endedSentence(e findingsEnd) string {
	used, budget := groupThousands(e.Used), groupThousands(e.Budget)
	lead := store.SpikeEndingOf(e.How).Lead
	var b strings.Builder
	switch e.How {
	case store.SpikeConcluded:
		fmt.Fprintf(&b, "%s. It used %s of its %s tokens.", lead, used, budget)
	case store.SpikeBudget:
		switch {
		case e.Used > e.Budget:
			fmt.Fprintf(&b, "%s. It used %s tokens, which is more than its budget of %s.", lead, used, budget)
		case e.Used == e.Budget:
			fmt.Fprintf(&b, "%s. It used all of its %s tokens.", lead, budget)
		default:
			// The run never begins a call it can see would cross the budget
			// (SD-7), so it can stop with some of the budget left.
			fmt.Fprintf(&b, "%s, because its next step would have gone over. It used %s of its %s tokens.", lead, used, budget)
		}
		b.WriteString(" The findings above are what it had saved by then.")
	case store.SpikeTurnLimit:
		fmt.Fprintf(&b, "%s of %d turns, having used %s of its %s tokens. The findings above are what it had saved by then.",
			lead, e.TurnCap, used, budget)
	default:
		note := strings.TrimRight(oneLine(e.Note), ". ")
		if note == "" {
			note = "no reason was recorded"
		}
		fmt.Fprintf(&b, "%s: %s. It used %s of its %s tokens. The findings above are what it had saved by then.",
			store.SpikeEndingOf(store.SpikeFailed).Lead, note, used, budget)
	}
	return b.String()
}

// keptParagraph is what follows the How this spike ended sentence when the
// leak check found code kept, or couldn't run (FR-2.3, FR-6.3); "" otherwise.
func keptParagraph(e findingsEnd) string {
	var b strings.Builder
	if len(e.Refs) > 0 {
		fmt.Fprintf(&b, "\n\nCode from this spike was kept on %s, outside its working copy. Subutai hasn't deleted it; the Inbox asks what to do.",
			backtickJoin(e.Refs))
	}
	if e.CheckFailed != "" {
		fmt.Fprintf(&b, "\n\nSubutai couldn't check whether code from this spike was kept: %s.", strings.TrimRight(oneLine(e.CheckFailed), ". "))
	}
	return b.String()
}

// findingsTitle is "SPK-003: <the question, shortened to 80 characters>".
func findingsTitle(publicID, question string) string {
	return publicID + ": " + shortQuestion(question)
}

// shortQuestion is a question on one line, at most 80 characters.
func shortQuestion(question string) string {
	q := oneLine(question)
	if r := []rune(q); len(r) > 80 {
		q = strings.TrimSpace(string(r[:79])) + "…"
	}
	return q
}

// findingsSection is one level-2 section of a draft.
type findingsSection struct{ Heading, Body string }

// splitFindings splits a draft into what comes before its first level-2
// heading and its level-2 sections, leaving fenced code alone.
func splitFindings(body string) (preamble string, secs []findingsSection) {
	body = strings.ReplaceAll(body, "\r\n", "\n")
	var cur *findingsSection
	var pre strings.Builder
	inFence := false
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		}
		if h, ok := strings.CutPrefix(line, "## "); ok && !inFence {
			if cur != nil {
				secs = append(secs, *cur)
			}
			cur = &findingsSection{Heading: strings.TrimSpace(h)}
			continue
		}
		if cur == nil {
			pre.WriteString(line + "\n")
		} else {
			cur.Body += line + "\n"
		}
	}
	if cur != nil {
		secs = append(secs, *cur)
	}
	for i := range secs {
		secs[i].Body = strings.TrimSpace(secs[i].Body)
	}
	return strings.TrimSpace(pre.String()), secs
}

// dropTitle removes level-1 headings from the text before a draft's first
// section, because the document supplies its own. A "# " line in fenced code
// is a shell comment, not a heading, and stays.
func dropTitle(s string) string {
	var kept []string
	inFence := false
	for _, l := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inFence = !inFence
		}
		if !inFence && strings.HasPrefix(l, "# ") {
			continue
		}
		kept = append(kept, l)
	}
	return strings.TrimSpace(strings.Join(kept, "\n"))
}

// needsFill says an Answer or What we found is a template's blank rather than
// a reader's: empty, an unfilled placeholder, or a to-do. The one rule that
// finish_spike's check and the findings' fill-ins (SD-8) share.
func needsFill(s string) bool {
	t := strings.ToLower(strings.TrimSpace(s))
	switch strings.Trim(t, ".…-_ ") {
	case "", "todo", "tbd", "tba", "n/a", "none yet":
		return true
	}
	return strings.Contains(t, "{{")
}

var todoWord = regexp.MustCompile(`\bTODO\b`)

// closeFences closes a code fence a piece of text left open, so what is
// written after it is still structure.
func closeFences(s string) string {
	open := false
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			open = !open
		}
	}
	if open {
		return s + "\n```"
	}
	return s
}

// defuse rewrites what validation would take for an unfinished template:
// braces pairs and the word TODO, outside fenced code. A run that has stopped
// can't be asked to fix its findings, so the words are changed instead of
// the document being refused.
func defuse(s string) string {
	var out []string
	inFence := false
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
		} else if !inFence {
			line = strings.ReplaceAll(line, "{{", "{ {")
			line = todoWord.ReplaceAllString(line, "to-do")
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// findingsFrontFallback is the front matter a project without the template
// gets.
const findingsFrontFallback = "---\ntitle: {{title}}\ntype: findings\nowner: {{owner}}\n---\n"

// findingsFront is the template's front matter, filled in. A project whose
// template has gone missing still gets its findings: the run has ended and
// can't be run again, so the document falls back on the shape the starter
// pack ships.
func (s *Server) findingsFront(title, owner string) string {
	front := findingsFrontFallback
	if raw, err := os.ReadFile(filepath.Join(s.CompartmentRoot, "templates", "findings", "template.md")); err == nil {
		norm := strings.ReplaceAll(string(raw), "\r\n", "\n")
		if fm, _, err := config.SplitFrontMatter(norm); err == nil {
			front = "---\n" + fm + "\n---\n"
		}
	}
	// The template may quote its placeholders; the quoted forms come first so
	// they win, and the value brings its own quotes.
	return strings.NewReplacer(
		`"{{title}}"`, yamlQuote(title), "{{title}}", yamlQuote(title),
		`"{{owner}}"`, yamlQuote(owner), "{{owner}}", yamlQuote(owner)).Replace(front)
}

// buildFindings builds the findings file's text from the template's front
// matter, the question on the row, the draft (or finish_spike's body), SD-8's
// fill-ins, and the server's How this spike ended, which comes last and
// replaces any in the draft (FR-2.2, FR-2.3). Sections are written in the
// template's order, whatever order the draft had them in. With defuseText
// set, the run's own text that validation would take for an unfinished
// template is rewritten (a run that has stopped can't be sent back to fix it).
// The title and the Question section are the person's words and are never
// rewritten: they say the question as it is on the row (FR-2.1).
func (s *Server) buildFindings(publicID, question, ownerPath, draft string, e findingsEnd, defuseText bool) string {
	clean := func(t string) string {
		t = closeFences(strings.TrimSpace(t))
		if defuseText {
			t = defuse(t)
		}
		return t
	}
	title := findingsTitle(publicID, question)
	pre, secs := splitFindings(draft)
	got := map[string]string{}
	var extras []findingsSection
	for _, sec := range secs {
		key := strings.ToLower(sec.Heading)
		switch key {
		case strings.ToLower(headingQuestion), strings.ToLower(headingEnded):
			continue // the server's, from the row and from the run
		case strings.ToLower(headingAnswer), strings.ToLower(headingFound),
			strings.ToLower(headingHow), strings.ToLower(headingNext):
			if got[key] != "" {
				got[key] += "\n\n"
			}
			got[key] += sec.Body
		default:
			extras = append(extras, sec)
		}
	}
	if pre = dropTitle(pre); pre != "" {
		// Text before any heading is what the agent found, said without one.
		k := strings.ToLower(headingFound)
		if got[k] != "" {
			pre += "\n\n"
		}
		got[k] = pre + got[k]
	}
	answer := strings.TrimSpace(got[strings.ToLower(headingAnswer)])
	if needsFill(answer) {
		answer = notAnswered(e.How)
	}
	found := strings.TrimSpace(got[strings.ToLower(headingFound)])
	if needsFill(found) {
		found = nothingSaved
	}

	var b strings.Builder
	b.WriteString(s.findingsFront(title, ownerPath))
	b.WriteString("\n# " + title + "\n")
	section := func(heading, body string) {
		fmt.Fprintf(&b, "\n## %s\n\n%s\n", heading, body)
	}
	section(headingQuestion, closeFences(strings.TrimSpace(oneLineKeep(question))))
	section(headingAnswer, clean(answer))
	section(headingFound, clean(found))
	for _, h := range []string{headingHow, headingNext} {
		if body := strings.TrimSpace(got[strings.ToLower(h)]); body != "" {
			section(h, clean(body))
		}
	}
	for _, x := range extras {
		if strings.TrimSpace(x.Heading) != "" {
			section(x.Heading, clean(x.Body))
		}
	}
	section(headingEnded, clean(endedSentence(e)+keptParagraph(e)))
	return b.String()
}

// oneLineKeep collapses a question's line breaks, so a question can't open a
// section of its own, and otherwise leaves it as written.
func oneLineKeep(q string) string {
	if !strings.ContainsAny(q, "\r\n") {
		return q
	}
	return oneLine(q)
}

// findingsCheckQuestion stands in for the question when findings are checked:
// the question is the person's, so a TODO or {{ in it must not fail findings
// that the run can't change.
const findingsCheckQuestion = "The question."

// checkFindings builds a spike's findings as writeFindings would, with the
// question left out, and validates them: it is the run's own text that is
// checked.
func (s *Server) checkFindings(sp *store.Spike, ownerPath, draft string, e findingsEnd) error {
	return s.validateFindings(s.buildFindings(sp.PublicID, findingsCheckQuestion, ownerPath, draft, e, false))
}

// validateFindings checks findings text as validation of the document would,
// and says why not, as one sentence.
func (s *Server) validateFindings(text string) error {
	manifest, err := config.LoadManifest(s.CompartmentRoot, lifecycle.DocTypeFindings)
	if err != nil {
		return errors.New("This project's findings template has no manifest, so findings can't be checked.")
	}
	if rep := lifecycle.Validate(manifest, text, nil); !rep.Valid {
		return errors.New(reportSentence(&rep))
	}
	return nil
}

// spikeOwnerPath is the owner's path, for the findings' front matter.
func (s *Server) spikeOwnerPath(ctx context.Context, sp *store.Spike) string {
	if ownerType, ownerID := sp.Owner(); ownerType == "feature" {
		if f, err := store.GetFeature(ctx, s.Store.Pool, ownerID); err == nil {
			if p, err := s.featurePath(ctx, f); err == nil {
				return p
			}
		}
	}
	p, _ := s.initiativePath(ctx, sp.InitiativeID)
	return p
}

// writeFindings writes the findings file for a spike whose run has ended and
// registers it, in the caller's transaction: a draft owned by the spike, its
// primary document, written by Subutai (FR-2.2, SD-9). The file is written
// first; undo removes it if the transaction fails. A document that fails
// validation even after the words that look unfinished were rewritten is still
// written: the run can't be sent back, and a person can edit the findings.
func (s *Server) writeFindings(ctx context.Context, tx pgx.Tx, sp *store.Spike, draft string, e findingsEnd) (doc *store.Document, undo func(), err error) {
	undo = func() {}
	ownerPath := s.spikeOwnerPath(ctx, sp)
	defuseText := s.checkFindings(sp, ownerPath, draft, e) != nil
	text := s.buildFindings(sp.PublicID, sp.Question, ownerPath, draft, e, defuseText)
	if defuseText {
		if verr := s.validateFindings(s.buildFindings(sp.PublicID, findingsCheckQuestion, ownerPath, draft, e, true)); verr != nil {
			s.Log.Warn("findings don't validate; written as they are", "spike", sp.PublicID, "err", verr)
		}
	}
	oid := sp.ID
	id, revision, err := store.NextDocumentIdentity(ctx, tx, "spike", &oid, lifecycle.DocTypeFindings)
	if err != nil {
		return nil, undo, err
	}
	home, err := defaultHome(ctx, tx, sp.InitiativeID)
	if err != nil {
		return nil, undo, err
	}
	path := filepath.Join(home, id) + ".md"
	if !s.isOrphanFindings(path, id) {
		path = s.freePath(filepath.Join(home, id))
	}
	abs := filepath.Join(s.RepoRoot, path)
	body, err := stampIdentity(text, id, revision)
	if err != nil {
		return nil, undo, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, undo, err
	}
	if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
		return nil, undo, err
	}
	undo = func() { _ = os.Remove(abs) }
	doc, err = registerInTx(ctx, tx, path, []byte(body), lifecycle.DocTypeFindings, "spike", &oid, nil,
		id, revision, "subutai", writerAct{Act: store.ActWrote, Kind: store.WriterSystem, Actor: "subutai"})
	if err != nil {
		return nil, undo, err
	}
	if err := store.SetPrimaryDocument(ctx, tx, doc.ID, "subutai"); err != nil {
		return nil, undo, err
	}
	return doc, undo, nil
}

// isOrphanFindings says the file at path is what an earlier ending wrote and
// then stopped before registering and committing: it carries the findings ID
// this ending is about to use, and git has never seen it. It is written again
// rather than left beside a copy.
func (s *Server) isOrphanFindings(path, id string) bool {
	raw, err := os.ReadFile(filepath.Join(s.RepoRoot, path))
	if err != nil {
		return false
	}
	if got, _, ok := content.ReadIdentity(string(raw)); !ok || got != id {
		return false
	}
	out, err := gitIn(s.RepoRoot, "ls-files", "--", path)
	return err == nil && strings.TrimSpace(out) == ""
}

// ---- save_findings (FR-4.3) ----

// toolSaveFindings replaces the spike's draft. It touches no file: the draft
// is on the spike's row until the run ends (SD-8).
func (s *Server) toolSaveFindings(ctx context.Context, tctx *toolhost.Context, input json.RawMessage) (string, bool) {
	var args struct {
		Findings string `json:"findings"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return "invalid arguments: " + err.Error(), true
	}
	id, err := uuid.Parse(tctx.SpikeID)
	if err != nil {
		return "save_findings is only for a spike's run.", true
	}
	if strings.TrimSpace(args.Findings) == "" {
		return "There is nothing to save: give the findings as sections, such as ## Answer and ## What we found.", true
	}
	if err := store.SaveSpikeDraft(ctx, s.Store.Pool, id, args.Findings); err != nil {
		if errors.Is(err, store.ErrSpikeNotRunning) {
			return "This spike's run has already ended, so nothing more can be saved.", true
		}
		return "could not save the findings: " + err.Error(), true
	}
	return "Saved. If the run stops now, this is what is kept.", false
}
