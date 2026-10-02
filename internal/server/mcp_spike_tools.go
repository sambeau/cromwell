package server

// The chat agent's spike tools (SPEC-021 FR-9). Writing down a spike's
// question is planning authoring under DEC-004: it creates an idea and
// dispatches nothing, so create_spike takes no quote. Starting and closing a
// spike are a person's acts in the web UI (SD-5, SD-10), so no tool here
// starts, runs, closes or answers one.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/google/uuid"

	"subutai/internal/store"
)

// spikeRefRe matches a spike's public ID, such as "SPK-003".
var spikeRefRe = regexp.MustCompile(`(?i)^SPK-\d+$`)

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

// spikeResult is a spike as list_spikes shows it, and the start of get_spike's.
func (s *Server) spikeResult(ctx context.Context, sp *store.Spike) map[string]any {
	out := map[string]any{
		"id": sp.PublicID, "question": sp.Question, "state": sp.State,
		"tokens_used": sp.TokensUsed, "created_via": sp.CreatedVia,
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
		out["budget"], out["budget_source"] = *sp.TokenBudget, "given at the start"
	} else if cfg, err := s.freshConfig(); err == nil {
		budget, source := spikeBudgetFor(cfg, sp)
		out["budget"], out["budget_source"] = budget, map[string]string{"override": "set on the spike", "default": "project default"}[source]
	}
	return out
}
