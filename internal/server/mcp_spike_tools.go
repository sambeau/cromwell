package server

// The chat agent's spike tools (SPEC-021 FR-9). Writing down a spike's
// question is planning authoring under DEC-004: it creates an idea and
// dispatches nothing, so create_spike takes no quote. Starting and closing a
// spike are a person's acts in the web UI (SD-5, SD-10), so no tool here
// starts, closes or answers one. Running a spike a person started for the chat
// agent is doing, as DEC-007 decision 3 has it for a task: claim_spike,
// save_spike_findings and submit_spike (SPEC-021 FR-13.5 to FR-13.7). Judging
// its findings stays a person's (SD-25), so there is no tool to release,
// extend, end or approve one (FR-17.2).

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"subutai/internal/store"
)

// spikeRefRe matches a spike's public ID, such as "SPK-003". It is
// case-sensitive, so an initiative whose slug is "spk-001" isn't taken for one.
var spikeRefRe = regexp.MustCompile(`^SPK-\d+$`)

// spikeNotDeliverableRefusal is what adding a spike to a milestone says
// (SD-11).
const spikeNotDeliverableRefusal = "A spike can't be a milestone deliverable, because it ships nothing. Add the feature its findings led to instead."

func (s *Server) mcpSpikeTools() []mcpTool {
	return []mcpTool{
		{
			Name: "create_spike",
			Description: "Write down a question to investigate as a spike, on an initiative or a feature. This creates the " +
				"spike; it does not start it. A person starts a spike from the web UI, where they see the budget, and its " +
				"findings are written when its run ends.",
			Schema: objectSchema(map[string]any{
				"on":       stringProp("The initiative or feature the question belongs to, by path such as \"auth/login\", or by ID such as \"INIT-003\" or \"FEAT-012\"."),
				"question": stringProp("The question to answer, in one sentence, at most 500 characters."),
				"budget":   map[string]any{"type": "integer", "description": "Optional. A token budget for this spike, a positive whole number. Leave it out to use the project's default."},
			}, "on", "question"),
			Handler: s.mcpCreateSpike,
		},
		{
			Name: "claim_spike",
			Description: "Claim a spike that a person started for the chat agent to run, so that you and the person " +
				"investigate its question in a throwaway working copy. The person gave it a time box: when the time box " +
				"ends, the spike ends, whether you have finished or not, with whatever findings you have saved. " +
				"Work only inside the working copy this returns, and don't commit, branch, tag or stash there: a spike " +
				"keeps no code. Save your findings with save_spike_findings early and often, and call submit_spike when " +
				"you have an answer or know the question can't be answered. A person reads the findings and decides " +
				"whether the question is answered: you can't close the spike, release your claim or move its time box, " +
				"and you can't approve its findings. Calling this again on a spike you hold renews the claim. " +
				"Use list_spikes to find a spike that is running.",
			Schema: objectSchema(map[string]any{
				"spike": stringProp("The spike's ID, such as \"SPK-003\"."),
			}, "spike"),
			Handler: s.mcpClaimSpike,
		},
		{
			Name: "save_spike_findings",
			Description: "Save your findings so far on the spike you claimed, in the findings template's sections. It replaces " +
				"what was saved before, so send the whole text each time. When the spike's time box ends, whatever you " +
				"have saved is what is kept, so save early and often.",
			Schema: objectSchema(map[string]any{
				"spike":    stringProp("The ID of the spike you claimed, such as \"SPK-003\"."),
				"findings": stringProp("Your findings so far, as sections, starting with Answer and What we found."),
			}, "spike", "findings"),
			Handler: s.mcpSaveSpikeFindings,
		},
		{
			Name: "submit_spike",
			Description: "Hand in your findings on the spike you claimed, and end the spike as concluded. Give the findings " +
				"here, or leave them out to submit what you last saved with save_spike_findings. They must have the " +
				"template's required sections, or the submit is refused and nothing changes. A person then reads them " +
				"and decides whether the question is answered: you can't close the spike or approve its findings.",
			Schema: objectSchema(map[string]any{
				"spike":    stringProp("The ID of the spike you claimed, such as \"SPK-003\"."),
				"findings": stringProp("Optional. Your findings, as sections. Leave it out to submit the saved draft."),
			}, "spike"),
			Handler: s.mcpSubmitSpike,
		},
		{
			Name: "list_spikes",
			Description: "List spikes, with each one's ID, question, state (idea, running, ended or closed), owner, budget " +
				"(or the default it would get) and tokens used. Filter by the initiative or feature they were written on, " +
				"and by state.",
			Schema: objectSchema(map[string]any{
				"on":    stringProp("Optional. An initiative or feature, by path or ID, to list only its spikes."),
				"state": stringProp("Optional. \"idea\", \"running\", \"ended\" or \"closed\"."),
			}),
			Handler: s.mcpListSpikes,
		},
		{
			Name: "get_spike",
			Description: "Read one spike by its ID, such as \"SPK-003\": everything list_spikes gives, plus the draft while " +
				"it runs, how it ended, how it was closed, its findings' ID, path and state, its run's ID, and the spikes " +
				"it follows and is followed by.",
			Schema: objectSchema(map[string]any{
				"spike": stringProp("The spike's ID, such as \"SPK-003\"."),
			}, "spike"),
			Handler: s.mcpGetSpike,
		},
	}
}

// mcpSpikeOn resolves "on" for the spike tools: an initiative or a feature by
// path or ID. A spike's ID is refused with a sentence.
func (s *Server) mcpSpikeOn(ctx context.Context, ref string) (string, uuid.UUID, error) {
	ref = strings.TrimSpace(ref)
	if spikeRefRe.MatchString(ref) {
		return "", uuid.Nil, errors.New("A spike is written on an initiative or a feature, not on another spike.")
	}
	return s.mcpOn(ctx, ref)
}

func (s *Server) mcpCreateSpike(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	on, ok := argString(args, "on")
	if !ok {
		return nil, errors.New("say where the spike goes, with \"on\": an initiative or feature, by path or ID")
	}
	ownerType, ownerID, err := s.mcpSpikeOn(ctx, on)
	if err != nil {
		return nil, err
	}
	question, _ := args["question"].(string)
	var budget *int64
	n, present, err := argInt(args, "budget")
	if err != nil {
		return nil, err
	}
	if present {
		b := int64(n)
		budget = &b
	}
	sp, err := s.CreateSpike(ctx, ownerType, ownerID, question, budget, s.mcpActor(), "mcp")
	if err != nil {
		return nil, err
	}
	out := s.spikeResult(ctx, sp)
	out["next"] = "A person can start it from its page in the web UI."
	return out, nil
}

func (s *Server) mcpListSpikes(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	var f store.SpikeFilter
	if st, ok := argString(args, "state"); ok {
		switch st {
		case store.SpikeIdea, store.SpikeRunning, store.SpikeEnded, store.SpikeClosed:
			f.State = st
		default:
			return nil, fmt.Errorf("state is idea, running, ended or closed, not %q", st)
		}
	}
	if on, ok := argString(args, "on"); ok {
		ownerType, id, err := s.mcpSpikeOn(ctx, on)
		if err != nil {
			return nil, err
		}
		if ownerType == "feature" {
			f.FeatureID = &id
		} else {
			f.InitiativeID = &id
			f.DirectOnly = true
		}
	}
	spikes, err := store.ListSpikes(ctx, s.Store.Pool, f)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(spikes))
	for i := range spikes {
		out = append(out, s.spikeResult(ctx, &spikes[i]))
	}
	return map[string]any{"spikes": out}, nil
}

func (s *Server) mcpGetSpike(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	ref, _ := argString(args, "spike")
	if !spikeRefRe.MatchString(ref) {
		return nil, fmt.Errorf("there is no spike %q; spikes have IDs like \"SPK-003\", and list_spikes shows them", ref)
	}
	sp, err := store.SpikeByPublicID(ctx, s.Store.Pool, strings.ToUpper(ref))
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("there is no spike %q; list_spikes shows what exists", ref)
	}
	if err != nil {
		return nil, err
	}
	out := s.spikeResult(ctx, sp)
	if sp.State == store.SpikeRunning && sp.Draft != "" {
		out["draft"] = sp.Draft
	}
	if sp.EndedHow != "" {
		out["ended_how"] = sp.EndedHow
		out["end_note"] = sp.EndNote
	}
	if sp.ClosedAs != "" {
		out["closed_as"] = sp.ClosedAs
	}
	out["findings"] = nil
	if d, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "findings", "spike", sp.ID); err == nil {
		out["findings"] = map[string]any{"id": d.PublicID, "path": d.Path, "state": string(d.State)}
	}
	out["run_id"] = nil
	if runs, err := store.RunsForRef(ctx, s.Store.Pool, "spike", sp.ID); err == nil {
		for i := len(runs) - 1; i >= 0; i-- {
			if runs[i].Purpose == "run-spike" {
				out["run_id"] = runs[i].ID.String()
				break
			}
		}
	}
	out["follows"] = nil
	if sp.FollowsID != nil {
		if p, err := store.GetSpike(ctx, s.Store.Pool, *sp.FollowsID); err == nil {
			out["follows"] = p.PublicID
		}
	}
	followed := []string{}
	if next, err := store.SpikesFollowing(ctx, s.Store.Pool, sp.ID); err == nil {
		for _, n := range next {
			followed = append(followed, n.PublicID)
		}
	}
	out["followed_by"] = followed
	return out, nil
}

// mcpClaimSpike claims, or renews a claim on, a spike for the chat agent
// (SPEC-021 FR-13.4).
func (s *Server) mcpClaimSpike(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	ref, ok := argString(args, "spike")
	if !ok {
		return nil, errors.New("say which spike to claim, with \"spike\": its ID, such as \"SPK-003\"")
	}
	res, err := s.ClaimSpike(ctx, ref, s.ChatClaimant())
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"spike": s.spikeResult(ctx, res.Spike),
		"working_copy": map[string]any{
			"path": res.WorkingCopy.Path, "base_commit": res.WorkingCopy.BaseCommit, "detached": true,
		},
		"contract":  res.Contract,
		"time_left": res.TimeLeft,
		"rules":     res.Rules,
		"next":      spikeNextSentence,
	}
	if res.Spike.DeadlineAt != nil {
		out["deadline"] = res.Spike.DeadlineAt.UTC().Format(time.RFC3339)
	}
	if res.Renewed {
		out["renewed"] = true
	}
	return out, nil
}

// mcpSaveSpikeFindings replaces the claimed spike's draft (FR-13.6).
func (s *Server) mcpSaveSpikeFindings(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	ref, ok := argString(args, "spike")
	if !ok {
		return nil, errors.New("say which spike to save findings on, with \"spike\": its ID, such as \"SPK-003\"")
	}
	text, _ := args["findings"].(string)
	sp, err := s.SaveSpikeFindings(ctx, ref, s.ChatClaimant(), text)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"saved":     "Saved. If the time box ends now, this is what is kept.",
		"time_left": spikeTimeLeft(sp, time.Now()),
	}
	if sp.DeadlineAt != nil {
		out["deadline"] = sp.DeadlineAt.UTC().Format(time.RFC3339)
	}
	return out, nil
}

// mcpSubmitSpike hands the claimed spike's findings in, which ends it as
// concluded (FR-13.7).
func (s *Server) mcpSubmitSpike(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	ref, ok := argString(args, "spike")
	if !ok {
		return nil, errors.New("say which spike to submit, with \"spike\": its ID, such as \"SPK-003\"")
	}
	findings, _ := args["findings"].(string)
	sp, err := s.SubmitSpike(ctx, ref, s.ChatClaimant(), findings)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"spike":    s.spikeResult(ctx, sp),
		"findings": nil,
		"next": "A person reads the findings on the spike's page and decides whether the question is answered. " +
			"You can't close the spike or approve its findings.",
	}
	if d, err := store.CurrentDocForOwner(ctx, s.Store.Pool, "findings", "spike", sp.ID); err == nil {
		out["findings"] = map[string]any{"id": d.PublicID, "path": d.Path}
	}
	return out, nil
}

// spikeBudgetSourceWords is where a budget came from, as the MCP tools say it.
var spikeBudgetSourceWords = map[string]string{
	store.BudgetFromOverride:    "set on the spike",
	store.BudgetFromDefault:     "project default",
	store.BudgetFromStartScreen: "given at the start",
}

// spikeResult is a spike as list_spikes shows it, and the start of get_spike's.
func (s *Server) spikeResult(ctx context.Context, sp *store.Spike) map[string]any {
	out := map[string]any{
		"id": sp.PublicID, "question": sp.Question, "state": sp.State,
		"tokens_used": sp.TokensUsed, "created_via": sp.CreatedVia,
	}
	// Who runs it, and what limit it has (FR-16.2). A chat or person spike's
	// tokens aren't measured, so its count is null rather than 0.
	// Read once: the executor's sentence and the claim object are both made
	// from it. A read that fails leaves a spike with no run to describe.
	facts, err := readSpikeRunFacts(ctx, s.Store.Pool, sp)
	if err != nil {
		s.Log.Warn("a spike's run facts couldn't be read", "spike", sp.PublicID, "err", err)
		facts = spikeRunFacts{}
	}
	exec := spikeExecutorSentence(sp, facts, time.Now())
	executor := map[string]any{"sentence": exec.Sentence}
	if exec.Kind != "" {
		executor["kind"], executor["who"], executor["model"], executor["run_id"] = exec.Kind, exec.Who, exec.Model, exec.RunID
	}
	out["executor"], out["measured"] = executor, exec.Measured
	if unmeasuredExecutor(sp.Executor) {
		out["tokens_used"] = nil
		out["time_box_hours"] = nil
		if sp.TimeBoxHours != nil {
			out["time_box_hours"] = *sp.TimeBoxHours
		}
		out["deadline"] = nil
		if sp.DeadlineAt != nil {
			out["deadline"] = sp.DeadlineAt.UTC().Format(time.RFC3339)
		}
		out["claim"] = nil
		if c := facts.Claim; c != nil {
			out["claim"] = map[string]any{
				"kind": c.Kind, "who": whoWords(c.Kind, c.Actor, ""), "state": string(c.State),
				"since":            c.ClaimedAt.UTC().Format(time.RFC3339),
				"last_activity":    claimActivityWords(c.LastActivity),
				"last_activity_at": c.LastActivityAt.UTC().Format(time.RFC3339),
			}
		}
	}
	owner := map[string]any{}
	if o := s.spikeOwnerRef(ctx, sp); o.ID != "" {
		owner = map[string]any{"id": o.ID, "name": o.Name, "type": o.Type}
		if o.Path != "" {
			owner["path"] = o.Path
		}
	}
	out["owner"] = owner
	if sp.TokenBudget != nil {
		out["budget"], out["budget_source"] = *sp.TokenBudget, spikeBudgetSourceWords[store.BudgetFromStartScreen]
	} else if cfg, err := s.freshConfig(); err == nil {
		budget, source := spikeBudgetFor(cfg, sp)
		out["budget"], out["budget_source"] = budget, spikeBudgetSourceWords[source]
	}
	return out
}
