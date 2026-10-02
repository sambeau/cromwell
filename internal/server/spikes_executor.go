package server

// Who ran a spike, and what limit it had (SPEC-021 FR-16.1, FR-16.4): the
// executor line, the time box line and the unmeasured line, as functions the
// spike's page and get_spike share, and the sentence for a spike closed by the
// person who ran it.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// SpikeExecutor says who runs, or ran, a spike and carries the fields of
// FR-16.2's executor entry.
type SpikeExecutor struct {
	Sentence string // FR-16.1's line
	Kind     string // agent | chat | person; "" before the start
	Who      string
	Model    string
	RunID    string
	// Measured is false for a chat or person spike: its tokens aren't counted
	// (SD-24).
	Measured bool
}

// spikeExecutor reads a spike's executions and latest claim and says who runs
// it.
func (s *Server) spikeExecutor(ctx context.Context, q store.Querier, sp *store.Spike) (SpikeExecutor, error) {
	execs, err := store.ExecutionsFor(ctx, q, "spike", sp.ID)
	if err != nil {
		return SpikeExecutor{}, err
	}
	claim, err := store.LatestClaimFor(ctx, q, "spike", sp.ID)
	if errors.Is(err, store.ErrNotFound) {
		claim = nil
	} else if err != nil {
		return SpikeExecutor{}, err
	}
	return spikeExecutorSentence(sp, execs, claim, time.Now()), nil
}

// spikeExecutorSentence is spikeExecutor without the reads. now is the time
// "ago" counts from.
func spikeExecutorSentence(sp *store.Spike, execs []store.Execution, claim *store.Claim, now time.Time) SpikeExecutor {
	out := SpikeExecutor{Kind: sp.Executor, Measured: !unmeasuredExecutor(sp.Executor)}
	running := sp.State == store.SpikeRunning
	switch sp.Executor {
	case store.ExecutorAgent:
		var last *store.Execution
		for i := range execs {
			if execs[i].Kind == store.ExecutorAgent {
				last = &execs[i]
			}
		}
		if last == nil {
			out.Sentence = "To be run by the spike runner."
			return out
		}
		out.Who = whoWords(store.ExecutorAgent, last.Actor, last.Model)
		out.Model = last.Model
		if last.DispatchID != nil {
			out.RunID = last.DispatchID.String()
		}
		out.Sentence = "Run by " + out.Who + "."
	case store.ExecutorChat, store.ExecutorPerson:
		kind := sp.Executor
		held := claim != nil && claim.State != lifecycle.ClaimEnded
		// Who ran it: the latest execution of its kind, else the latest claim.
		actor := ""
		for _, e := range execs {
			if e.Kind == kind {
				actor = e.Actor
			}
		}
		if actor == "" && claim != nil {
			actor = claim.Actor
		}
		out.Who = whoWords(kind, actor, "")
		inChat := kind == store.ExecutorChat
		switch {
		case held && running:
			if inChat {
				out.Sentence = "Being run in chat by " + out.Who + ", who claimed it " + agoWords(claim.ClaimedAt, now) + "."
			} else {
				out.Sentence = "Being run by hand by " + out.Who + "."
			}
		case running:
			out.Who = ""
			if inChat {
				out.Sentence = "To be run in chat. Waiting for the chat agent to claim it."
			} else {
				out.Sentence = "To be run by hand. Waiting for a person to claim it."
			}
		case actor == "" && !inChat:
			out.Who = ""
			out.Sentence = "It was to be run by hand, but nobody claimed it."
		case actor == "":
			out.Who = ""
			out.Sentence = "It was to be run in chat, but nobody claimed it."
		case inChat:
			out.Sentence = "Run in chat by " + out.Who + "."
		default:
			out.Sentence = "Run by hand by " + out.Who + "."
		}
	default:
		out.Sentence = "Not started yet."
		out.Measured = true
	}
	return out
}

// spikeTimeBoxLine is FR-16.1's limit for a chat or person spike: the time box
// and, while it runs, when it ends and what is left. It is "" for an agent
// spike, whose limit is a token budget.
func spikeTimeBoxLine(sp *store.Spike, now time.Time) string {
	if !unmeasuredExecutor(sp.Executor) || sp.TimeBoxHours == nil {
		return ""
	}
	h := *sp.TimeBoxHours
	line := fmt.Sprintf("Time box: %d %s", h, plural(h, "hour", "hours"))
	if sp.State != store.SpikeRunning || sp.DeadlineAt == nil {
		return line + "."
	}
	end := sp.DeadlineAt.Local()
	return fmt.Sprintf("%s, ending at %s on %d %s (%s).", line, end.Format("15:04"), end.Day(), end.Month(), spikeLeftWords(sp.DeadlineAt.Sub(now)))
}

// spikeLeftWords says the time that is left, to the minute: "3 hours 12
// minutes left", "12 minutes left", "less than a minute left", and "no time
// left" once the deadline has passed.
func spikeLeftWords(d time.Duration) string {
	if d <= 0 {
		return "no time left"
	}
	mins := int(d / time.Minute)
	hours, mins := mins/60, mins%60
	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", hours, plural(hours, "hour", "hours")))
	}
	if mins > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", mins, plural(mins, "minute", "minutes")))
	}
	if len(parts) == 0 {
		return "less than a minute left"
	}
	return strings.Join(parts, " ") + " left"
}

// spikeUnmeasuredLine is what stands in place of the token bar for a chat or
// person spike (FR-16.1, SD-24); "" for an agent spike.
func spikeUnmeasuredLine(sp *store.Spike) string {
	switch sp.Executor {
	case store.ExecutorChat:
		return "This spike ran in chat, so its tokens weren't measured."
	case store.ExecutorPerson:
		return "This spike was run by hand, so its tokens weren't measured."
	}
	return ""
}

// spikeRunBy says whether who is the person who ran the spike by hand: the
// latest claim on it is a person's, and was theirs (SD-26).
func spikeRunBy(ctx context.Context, q store.Querier, sp *store.Spike, who string) (bool, error) {
	if sp.Executor != store.ExecutorPerson || who == "" {
		return false, nil
	}
	claim, err := store.LatestClaimFor(ctx, q, "spike", sp.ID)
	if errors.Is(err, store.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return claim.Kind == store.ExecutorPerson && claim.Actor == who, nil
}

// spikeClosedBySentence is the closed spike's "Closed by sam, who also ran
// it." (FR-16.4). It is "" for a spike not closed, or closed by someone who
// didn't run it: the page says who closed it in its own way.
func (s *Server) spikeClosedBySentence(ctx context.Context, sp *store.Spike) string {
	if sp.State != store.SpikeClosed || sp.ClosedBy == "" {
		return ""
	}
	ran, err := spikeRunBy(ctx, s.Store.Pool, sp, sp.ClosedBy)
	if err != nil || !ran {
		return ""
	}
	return "Closed by " + sp.ClosedBy + ", who also ran it."
}
