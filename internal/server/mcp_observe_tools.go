package server

// Reading the work from chat (SPEC-017 FR-4, the M6 handoff's follow-up 1):
// a feature's timeline, and one agent run with its conclusion and a compact
// transcript. Both are read-only. They read what the web UI already shows —
// featureTimeline and buildRunPage, the functions the pages use — so they
// are authoring-safe under DEC-004 and add no authority.

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"subutai/internal/store"
	"subutai/internal/timeline"
)

// Transcript limits for get_agent_run (SPEC-017 SD-10).
const (
	runTextLimit   = 2000
	runResultLimit = 500
	runTurnLimit   = 60
)

func (s *Server) mcpObserveTools() []mcpTool {
	return []mcpTool{
		{
			Name: "get_timeline",
			Description: "Return a feature's journey: its major moments in order, such as sent to development, " +
				"spec written, spec approved, plan approved and building, each saying who caused it — an agent " +
				"run, a person, the chat agent, or Subutai itself — and the agent runs in each step. Use it to " +
				"answer \"how is it going?\". It changes nothing.",
			Schema: objectSchema(map[string]any{
				"feature": stringProp("The feature's path, for example \"auth/login\", or its ID, such as \"FEAT-012\"."),
			}, "feature"),
			Handler: s.mcpGetTimeline,
		},
		{
			Name: "get_agent_run",
			Description: "Return one agent run: what it was asked to do, what it concluded (a verdict and its " +
				"reasons, findings, or the document it wrote), and a compact transcript of what it did, turn by " +
				"turn. Long text is cut, and says so; the run's page, at the url given, has everything. Use it " +
				"to answer \"why did the reviewer say that?\". It changes nothing.",
			Schema: objectSchema(map[string]any{
				"run_id":               stringProp("The run's id, as get_timeline gives it."),
				"attempt":              map[string]any{"type": "integer", "description": "Optional. Which attempt to read, from 1; the latest if left out."},
				"include_tool_results": boolProp("Optional, false unless you say otherwise. Whether to include what each tool call returned, cut short. Tool results can hold what a tool read, such as a file with a secret in it, so ask for them only when you need them."),
			}, "run_id"),
			Handler: s.mcpGetAgentRun,
		},
	}
}

func (s *Server) mcpGetTimeline(r *http.Request, args map[string]any) (any, error) {
	ref, ok := argString(args, "feature")
	if !ok {
		return nil, errors.New("say which feature, by its path or its ID, in \"feature\"")
	}
	ctx := r.Context()
	f, path, err := s.featureByRef(ctx, ref)
	if err != nil {
		return nil, fmt.Errorf("there is no feature at or called %q; call get_tree to see what exists", ref)
	}
	view, err := s.featureTimeline(ctx, f.ID)
	if err != nil {
		return nil, err
	}
	line := make([]any, 0, len(view.Line))
	for _, st := range view.Line {
		entry := map[string]any{"label": st.Label}
		if st.Count > 1 {
			entry["times"] = st.Count
		}
		line = append(line, entry)
	}
	moments := make([]any, 0, len(view.Moments))
	for _, m := range view.Moments {
		moments = append(moments, s.mcpMoment(r, m))
	}
	return map[string]any{
		"feature": map[string]any{"id": f.PublicID, "path": path, "name": f.Name, "state": string(f.State)},
		"line":    line, "moments": moments, "url": "/ui/f/" + path,
	}, nil
}

func (s *Server) mcpMoment(r *http.Request, m momentView) map[string]any {
	out := map[string]any{
		"label": m.Label, "at": m.At.Format(time.RFC3339), "by": m.By, "who": momentWho(m),
	}
	if m.Relayed {
		out["relayed"] = true
	}
	if m.RefName != "" || m.RefURL != "" {
		about := map[string]any{"type": m.RefType, "name": m.RefName}
		if m.RefType == "document" && m.RefID != uuid.Nil {
			if d, err := store.GetDocument(r.Context(), s.Store.Pool, m.RefID); err == nil && d.PublicID != "" {
				about["id"] = d.PublicID
			}
		}
		if m.RefURL != "" {
			about["url"] = m.RefURL
		}
		out["about"] = about
	}
	if m.Cause != nil {
		out["cause"] = mcpRunRow(*m.Cause)
	}
	runs := make([]any, 0, len(m.Runs))
	for _, run := range m.Runs {
		runs = append(runs, mcpRunRow(run))
	}
	if len(runs) > 0 {
		out["runs"] = runs
	}
	return out
}

// momentWho says who caused a moment, as the timeline on the page does.
func momentWho(m momentView) string {
	switch {
	case m.Relayed:
		return "a person, relayed by the chat agent"
	case m.By == timeline.ByPerson:
		return m.Actor
	case m.By == timeline.ByChat:
		return "the chat agent"
	case m.By == timeline.ByAgent && m.Cause != nil:
		return whoWords(store.WriterAgent, m.Cause.Role, m.Cause.Model)
	case m.By == timeline.ByAgent:
		return "an agent"
	}
	return "Subutai"
}

func mcpRunRow(r runRow) map[string]any {
	out := map[string]any{
		"run_id": r.ID.String(), "what": r.What, "state": r.State,
		"role": r.Role, "model": r.Model, "attempt": r.Attempt, "tokens": r.Tokens,
	}
	if r.Verdict != "" {
		out["verdict"] = r.Verdict
	}
	return out
}

func (s *Server) mcpGetAgentRun(r *http.Request, args map[string]any) (any, error) {
	raw, ok := argString(args, "run_id")
	if !ok {
		return nil, errors.New("say which run in \"run_id\"; get_timeline lists them")
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%q isn't a run id; get_timeline lists them", raw)
	}
	ctx := r.Context()
	d, err := store.GetDispatch(ctx, s.Store.Pool, id)
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("there is no agent run %s", raw)
	}
	if err != nil {
		return nil, err
	}
	attempt := ""
	if v, present := args["attempt"]; present {
		n, ok := v.(float64)
		if !ok || n != float64(int(n)) || int(n) < 1 || int(n) > d.Attempt {
			return nil, fmt.Errorf("this run has %s; ask for one from 1 to %d, or leave attempt out for the latest",
				plural(d.Attempt, "one attempt", strconv.Itoa(d.Attempt)+" attempts"), d.Attempt)
		}
		attempt = strconv.Itoa(int(n))
	}
	p, err := s.buildRunPage(ctx, d, attempt)
	if err != nil {
		return nil, err
	}
	url := "/ui/run/" + d.ID.String()
	if !p.IsLatest {
		url += "?attempt=" + strconv.Itoa(p.Attempt)
	}
	out := map[string]any{
		"run_id": d.ID.String(), "what": p.Heading, "state": p.Run.State,
		"role": d.Role, "model": d.Model, "attempt": p.Attempt, "attempts": d.Attempt,
		"tokens_in": p.TokensIn, "tokens_out": p.TokensOut, "url": url,
	}
	if p.Took != "" {
		out["took"] = p.Took
	}
	if p.About.Label != "" {
		out["about"] = map[string]any{"name": p.About.Label, "url": p.About.URL}
	}
	if p.Feature.Label != "" {
		out["feature"] = map[string]any{"name": p.Feature.Label, "url": p.Feature.URL}
	}
	out["conclusion"] = mcpConclusion(p.Conclusion)
	// Whether the verdict counted: a stale one is dropped, one that missed a
	// new issue runs again, and a held one waits (SPEC-017 R17-10).
	if p.Conclusion.Kind == "verdict" && p.IsLatest {
		out["verdict_applied"] = s.verdictApplied(r, d)
	}
	withResults, err := argBoolDefault(args, "include_tool_results", false)
	if err != nil {
		return nil, err
	}
	if p.HasTranscript {
		out["transcript"] = mcpTranscript(p, withResults)
	} else if p.Pruned != nil {
		out["transcript_note"] = "The transcript was pruned on " + p.Pruned.Format("2 January 2006") + "; only its conclusion is kept."
	} else {
		out["transcript_note"] = "This run has no transcript."
	}
	return out, nil
}

func mcpConclusion(c conclusion) map[string]any {
	out := map[string]any{"kind": c.Kind}
	put := func(k, v string) {
		if v != "" {
			out[k] = v
		}
	}
	put("verdict", c.Verdict)
	put("in_words", c.Plain)
	put("reasoning", c.Reasoning)
	put("summary", c.Summary)
	put("error", c.Error)
	if len(c.Findings) > 0 {
		fs := make([]any, 0, len(c.Findings))
		for _, f := range c.Findings {
			fs = append(fs, map[string]any{"severity": f.Severity, "section": f.Section, "body": f.Body})
		}
		out["findings"] = fs
	}
	if len(c.Criteria) > 0 {
		cs := make([]any, 0, len(c.Criteria))
		for _, cr := range c.Criteria {
			cs = append(cs, map[string]any{"id": cr.ID, "met": cr.Met, "evidence": cr.Evidence})
		}
		out["criteria"] = cs
	}
	if len(c.Files) > 0 {
		out["files"] = c.Files
	}
	if c.Words > 0 {
		out["words"] = c.Words
	}
	return out
}

// cut shortens text to a limit, saying whether it did.
func cut(text string, limit int) (string, bool) {
	r := []rune(text)
	if len(r) <= limit {
		return text, false
	}
	return string(r[:limit]) + "…", true
}

// cutEntry is one piece of the transcript, cut to its limit. A piece the
// store already shortened says so too.
func cutEntry(e store.TranscriptEntry, limit int) map[string]any {
	text, cutHere := cut(e.Content, limit)
	out := map[string]any{"text": text}
	if cutHere || e.Truncated {
		out["truncated"] = true
	}
	return out
}

// mcpTranscript is the compact transcript (SD-10): the prompt, then each
// turn's text and tool calls; at most runTurnLimit turns, keeping the first
// and the last.
func mcpTranscript(p *runPage, withResults bool) map[string]any {
	out := map[string]any{}
	if p.Prompt != nil {
		out["prompt"] = cutEntry(*p.Prompt, runTextLimit)
	}
	turns := p.Turns
	omitted := 0
	if len(turns) > runTurnLimit {
		head := runTurnLimit / 2
		tail := runTurnLimit - head
		omitted = len(turns) - runTurnLimit
		turns = append(append([]turnView{}, turns[:head]...), turns[len(turns)-tail:]...)
	}
	ts := make([]any, 0, len(turns))
	for _, t := range turns {
		items := make([]any, 0, len(t.Items))
		for _, it := range t.Items {
			item := map[string]any{"kind": it.Kind}
			switch it.Kind {
			case "tool":
				item["tool"] = it.Entry.ToolName
				if it.Arg != "" {
					item["about"] = it.Arg
				}
				if it.Result != nil && withResults {
					res := cutEntry(*it.Result, runResultLimit)
					if it.Result.IsError {
						res["is_error"] = true
					}
					item["result"] = res
				}
			case "outcome":
				item["tool"] = it.Entry.ToolName
				for k, v := range cutEntry(it.Entry, runTextLimit) {
					item[k] = v
				}
			default:
				for k, v := range cutEntry(it.Entry, runTextLimit) {
					item[k] = v
				}
			}
			items = append(items, item)
		}
		turn := map[string]any{"turn": t.N, "items": items}
		if t.HasTokens {
			turn["tokens_in"], turn["tokens_out"] = t.In, t.Out
		}
		ts = append(ts, turn)
	}
	out["turns"] = ts
	if !withResults {
		out["tool_results"] = "left out; ask with include_tool_results to see them, cut to 500 characters each"
	}
	if omitted > 0 {
		out["turns_omitted"] = omitted
		out["note"] = fmt.Sprintf("%d turns in the middle are left out here; the run's page has them all.", omitted)
	}
	return out
}

// verdictApplied says what became of a review run's verdict.
func (s *Server) verdictApplied(r *http.Request, d *store.Dispatch) string {
	v, err := store.VerdictForRun(r.Context(), s.Store.Pool, d.ID)
	if err == nil {
		if v.Kind == store.GiverPerson {
			return "The reviewer escalated, and a person ruled: " + verdictSentence(*v)
		}
		if v.Held {
			return "Applied, and held for a person: " + verdictSentence(*v)
		}
		return "Applied: " + verdictSentence(*v)
	}
	if d.State != "succeeded" {
		return "Not applied: the run didn't finish."
	}
	return "Not applied. The document had changed or moved on by the time the verdict arrived, or an issue was raised while the review ran and it was reviewed again, or the reviewer escalated to a person who hasn't answered yet."
}
