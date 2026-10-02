package server

// The run-spike plan (SPEC-021 FR-4): the prompt, the tools, the worktree and
// the budget the dispatcher enforces. The planner makes the worktree, because
// the database commits first and the worktree follows from it (R21-3).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"subutai/internal/content"
	"subutai/internal/dispatch"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
	"subutai/internal/toolhost"
)

// spikeBudgetSentence is what the agent is told about its budget (FR-4.1).
const spikeBudgetSentence = "The run stops without warning when it reaches this many tokens. " +
	"Save your findings with `save_findings` early and often: whatever you have saved when it stops is what is kept."

// spikeResumeSentence is what a later attempt is told about the draft (FR-4.1).
const spikeResumeSentence = "An earlier attempt at this spike stopped. Build on these findings rather than starting again."

// spikeWorkingCopy is the "Your working copy" section (FR-4.1).
const spikeWorkingCopy = "This is a throwaway copy of the project's main line. Nothing you write here is kept, and Subutai never merges it: " +
	"the copy is discarded when the run ends. Use it for scaffolding, to read code and to run what you need to find out. " +
	"Findings are saved only with `save_findings`, never in a file."

// spikeOwnerSection is "Where it came from": the owner's name and description,
// and the paths of its current documents that are in the working copy as
// committed (FR-4.1).
func (s *Server) spikeOwnerSection(ctx context.Context, sp *store.Spike, worktree string) string {
	var b strings.Builder
	ownerType, ownerID := sp.Owner()
	if ownerType == "feature" {
		if f, err := store.GetFeature(ctx, s.Store.Pool, ownerID); err == nil {
			fmt.Fprintf(&b, "This spike was written down on the %s %s, %s.\n", bugOrFeature(f), f.PublicID, f.Name)
			if d := strings.TrimSpace(f.Description); d != "" {
				b.WriteString("\n" + d + "\n")
			}
		}
	} else if in, err := store.GetInitiative(ctx, s.Store.Pool, ownerID); err == nil {
		fmt.Fprintf(&b, "This spike was written down on the initiative %s, %s.\n", in.PublicID, in.Name)
		if d := strings.TrimSpace(in.Description); d != "" {
			b.WriteString("\n" + d + "\n")
		}
	}
	docs, _ := store.DocumentsForOwner(ctx, s.Store.Pool, ownerType, &ownerID)
	var paths []string
	for _, d := range docs {
		// Only what the working copy holds, as committed.
		if _, err := os.Stat(filepath.Join(worktree, d.Path)); err == nil {
			paths = append(paths, d.Path)
		}
	}
	if len(paths) > 0 {
		b.WriteString("\nIts documents, which you can read in your working copy as they were committed:\n\n")
		for _, p := range paths {
			b.WriteString("- " + p + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// lastSpikeFindings is the findings body of the spike this one asks again for,
// or "" when it follows none or they can't be read.
func (s *Server) lastSpikeFindings(ctx context.Context, sp *store.Spike) (string, string) {
	if sp.FollowsID == nil {
		return "", ""
	}
	prev, err := store.GetSpike(ctx, s.Store.Pool, *sp.FollowsID)
	if err != nil {
		return "", ""
	}
	doc, err := store.CurrentDocForOwner(ctx, s.Store.Pool, lifecycle.DocTypeFindings, "spike", prev.ID)
	if err != nil {
		return prev.PublicID, ""
	}
	raw, err := s.readDocFile(doc.Path)
	if err != nil {
		return prev.PublicID, ""
	}
	if parsed, err := content.Parse(string(raw)); err == nil {
		return prev.PublicID, strings.TrimSpace(parsed.Body)
	}
	return prev.PublicID, strings.TrimSpace(string(raw))
}

// planSpike builds a run-spike plan (FR-4). It refuses a spike that isn't
// running: a run for a spike that has ended has nothing to do and must not
// spend a token.
func (s *Server) planSpike(ctx context.Context, d *store.Dispatch) (*dispatch.Plan, error) {
	cfg, err := s.freshConfig()
	if err != nil {
		return nil, err
	}
	sp, err := store.GetSpike(ctx, s.Store.Pool, d.RefID)
	if err != nil {
		return nil, err
	}
	if sp.State != store.SpikeRunning || sp.TokenBudget == nil {
		return nil, fmt.Errorf("%s is %s, not running, so there is nothing for its run to do", sp.PublicID, sp.State)
	}
	role, skillBody, err := s.roleAndSkill(d.Role)
	if err != nil {
		return nil, err
	}
	worktree, err := s.ensureSpikeWorktree(ctx, sp)
	if err != nil {
		return nil, fmt.Errorf("the spike's working copy couldn't be made: %w", err)
	}

	block, err := s.surfacedBlock(ctx, surfaceScope{InitiativeID: &sp.InitiativeID})
	if err != nil {
		return nil, err
	}
	var b strings.Builder
	withSurfaced(&b, s.projectName(), block)
	b.WriteString("\n# The question\n\n" + sp.Question + "\n")
	if from := s.spikeOwnerSection(ctx, sp, worktree); from != "" {
		b.WriteString("\n# Where it came from\n\n" + from + "\n")
	}
	if id, body := s.lastSpikeFindings(ctx, sp); body != "" {
		fmt.Fprintf(&b, "\n# What the last spike found\n\nThis spike asks again after %s. Here is what that one found; start from it.\n\n%s\n", id, body)
	}
	if strings.TrimSpace(sp.Draft) != "" {
		b.WriteString("\n# What you have saved so far\n\n" + strings.TrimSpace(sp.Draft) + "\n\n" + spikeResumeSentence + "\n")
	}
	fmt.Fprintf(&b, "\n# Your budget\n\nYou have %s tokens. %s\n", groupThousands(*sp.TokenBudget), spikeBudgetSentence)
	b.WriteString("\n# Your working copy\n\n" + spikeWorkingCopy + "\n")

	commands := map[string]toolhost.CommandSpec{}
	for name, c := range cfg.Commands {
		commands[name] = toolhost.CommandSpec{Argv: c.Argv, TimeoutSeconds: c.TimeoutSeconds, OutputCapBytes: c.OutputCapBytes}
	}
	profile := map[string]bool{}
	for _, t := range role.Tools {
		profile[t] = true
	}
	id := sp.ID
	return &dispatch.Plan{
		System: roleSystemPrompt(role, skillBody), User: b.String(), TurnCap: turnCapFor(cfg, role),
		Tools:           append(dispatch.ProfileToolDefs(role.Tools), dispatch.FinishSpikeTool()),
		OutcomeTool:     "finish_spike",
		ValidateOutcome: s.validateFinishSpike(sp, s.spikeOwnerPath(ctx, sp)),
		ToolCtx: &toolhost.Context{
			WorktreeRoot: worktree, SpikeID: sp.ID.String(), Commands: commands, Profile: profile,
		},
		Budget: &dispatch.Budget{
			Limit: *sp.TokenBudget,
			Total: func(ctx context.Context) (int64, error) { return store.SpikeTokens(ctx, s.Store.Pool, id) },
			Add: func(ctx context.Context, n int64) (int64, error) {
				return store.AddSpikeTokens(ctx, s.Store.Pool, id, n)
			},
			EarlyTools: []string{"save_findings"},
		},
	}, nil
}

// validateFinishSpike checks finish_spike's findings as writeFindings would
// build them (FR-4.4): a concluding run is sent back to fix them, because it
// can still be told, where a run that stopped can't.
func (s *Server) validateFinishSpike(sp *store.Spike, ownerPath string) func(json.RawMessage) error {
	return func(raw json.RawMessage) error {
		var in struct {
			Findings string `json:"findings"`
		}
		if err := json.Unmarshal(raw, &in); err != nil {
			return err
		}
		if strings.TrimSpace(in.Findings) == "" {
			return errors.New("the findings are empty: give them as sections, starting with Answer and What we found")
		}
		_, secs := splitFindings(in.Findings)
		body := map[string]string{}
		for _, sec := range secs {
			body[strings.ToLower(sec.Heading)] += sec.Body
		}
		for _, h := range []string{headingAnswer, headingFound} {
			if needsFill(body[strings.ToLower(h)]) {
				return fmt.Errorf("the findings need a %s section with something in it", h)
			}
		}
		return s.checkFindings(sp, ownerPath, in.Findings, findingsEnd{How: store.SpikeConcluded})
	}
}
