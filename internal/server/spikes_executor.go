package server

// Who runs a spike, and what limit it has (SPEC-021 FR-16.1, FR-16.2): the
// executor line, the time box line and the unmeasured line, as functions the
// spike's page and get_spike share; who ran it, read once and said the same
// everywhere (FR-15.3, FR-16.4); the sentence for a spike closed by the person
// who ran it; and the words for a time box's deadline and what is left of it.
// All times are said in UTC, with the zone named, so a page and a chat agent
// agree on the hour.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// unmeasuredExecutor says a spike is run by the chat agent or a person: it has
// a time box and a claim, and no token budget (SPEC-021 SD-18, SD-24).
func unmeasuredExecutor(executor string) bool {
	return executor == store.ExecutorChat || executor == store.ExecutorPerson
}

// executorPhrase is how a chat or person spike was run, as a phrase that
// follows "run": "in chat" or "by hand". It is "" for any other executor.
func executorPhrase(executor string) string {
	switch executor {
	case store.ExecutorChat:
		return "in chat"
	case store.ExecutorPerson:
		return "by hand"
	}
	return ""
}

// tokensNotMeasured is the sentence that says a chat or person spike's tokens
// weren't counted (FR-16.1, FR-15.3, SD-24), about subject: "This spike" on
// the page, "It" in the findings. It is "" for an agent spike.
func tokensNotMeasured(executor, subject string) string {
	switch executor {
	case store.ExecutorChat:
		return subject + " ran " + executorPhrase(executor) + ", so its tokens weren't measured."
	case store.ExecutorPerson:
		return subject + " was run " + executorPhrase(executor) + ", so its tokens weren't measured."
	}
	return ""
}

// ---- Who ran it: read once, said once ----

// spikeRunFacts is what is read of a spike's runs, once per page or per list
// entry: its executions, and, for a chat or person spike, its latest claim.
// Every sentence about who ran the spike is made from it.
type spikeRunFacts struct {
	Execs []store.Execution
	Claim *store.Claim // the latest claim, in any state; nil when there is none
}

// readSpikeRunFacts reads the facts for sp.
func readSpikeRunFacts(ctx context.Context, q store.Querier, sp *store.Spike) (spikeRunFacts, error) {
	var f spikeRunFacts
	var err error
	if f.Execs, err = store.ExecutionsFor(ctx, q, "spike", sp.ID); err != nil {
		return spikeRunFacts{}, err
	}
	if !unmeasuredExecutor(sp.Executor) {
		return f, nil
	}
	f.Claim, err = store.LatestClaimFor(ctx, q, "spike", sp.ID)
	if errors.Is(err, store.ErrNotFound) {
		return f, nil
	}
	if err != nil {
		return spikeRunFacts{}, err
	}
	return f, nil
}

// held is true when the latest claim hasn't ended.
func (f spikeRunFacts) held() bool {
	return f.Claim != nil && f.Claim.State != lifecycle.ClaimEnded
}

// spikeRanBy says who ran a spike, as its kind (the spike's executor) and
// actor: the latest execution of that kind, else the latest claim's actor. The
// actor is "" when nobody did. The sentence, the findings and the closer's
// test all use it, so they can't disagree after a release and a second claim.
// Each claim records one execution, so the actor is the latest claimant of
// that kind.
func spikeRanBy(sp *store.Spike, execs []store.Execution, claim *store.Claim) (kind, actor string) {
	kind = sp.Executor
	if kind == "" {
		return "", ""
	}
	for _, e := range execs {
		if e.Kind == kind {
			actor = e.Actor
		}
	}
	if actor == "" && claim != nil {
		actor = claim.Actor
	}
	return kind, actor
}

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

// spikeExecutorSentence says who runs a spike from the facts read. now is the
// time "ago" counts from.
func spikeExecutorSentence(sp *store.Spike, f spikeRunFacts, now time.Time) SpikeExecutor {
	out := SpikeExecutor{Kind: sp.Executor, Measured: !unmeasuredExecutor(sp.Executor)}
	switch sp.Executor {
	case store.ExecutorAgent:
		var last *store.Execution
		for i := range f.Execs {
			if f.Execs[i].Kind == store.ExecutorAgent {
				last = &f.Execs[i]
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
		kind, actor := spikeRanBy(sp, f.Execs, f.Claim)
		how := executorPhrase(kind)
		out.Who = whoWords(kind, actor, "")
		running := sp.State == store.SpikeRunning
		switch {
		case f.held() && running:
			out.Sentence = "Being run " + how + " by " + out.Who
			if kind == store.ExecutorChat {
				out.Sentence += ", who claimed it " + agoWords(f.Claim.ClaimedAt, now)
			}
			out.Sentence += "."
		case running && kind == store.ExecutorChat:
			out.Who = ""
			out.Sentence = "To be run in chat. Waiting for the chat agent to claim it."
		case running:
			out.Who = ""
			out.Sentence = "To be run by hand. Waiting for a person to claim it."
		case actor == "":
			out.Who = ""
			out.Sentence = "It was to be run " + how + ", but nobody claimed it."
		default:
			out.Sentence = "Run " + how + " by " + out.Who + "."
		}
	default:
		out.Sentence = "Not started yet."
		out.Measured = true
	}
	return out
}

// closerRanIt says whether who, closing the spike, is the person who ran it by
// hand (SD-26): the spike is a person's, and spikeRanBy names them.
func closerRanIt(sp *store.Spike, f spikeRunFacts, who string) bool {
	if sp.Executor != store.ExecutorPerson || who == "" {
		return false
	}
	_, actor := spikeRanBy(sp, f.Execs, f.Claim)
	return actor == who
}

// ---- The limit ----

// spikeClock is a moment's hour and minute, in UTC: "17:04 UTC".
func spikeClock(t time.Time) string { return t.UTC().Format("15:04") + " UTC" }

// spikeDeadlineWords is "17:04 UTC on 3 October".
func spikeDeadlineWords(t time.Time) string {
	return spikeClock(t) + " on " + t.UTC().Format("2 January")
}

// spikeSpanWords says a span of time to the minute: "3 hours 12 minutes",
// "12 minutes", "less than a minute". It is the one place a time left is
// formatted.
func spikeSpanWords(d time.Duration) string {
	mins := int(d / time.Minute)
	if mins <= 0 {
		return "less than a minute"
	}
	hours, mins := mins/60, mins%60
	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", hours, plural(hours, "hour", "hours")))
	}
	if mins > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", mins, plural(mins, "minute", "minutes")))
	}
	return strings.Join(parts, " ")
}

// spikeLeftWords says the time that is left: "3 hours 12 minutes left", "less
// than a minute left", and "no time left" once the deadline has passed. A
// sentence about a deadline is built by spikeBoxSentence.
func spikeLeftWords(d time.Duration) string {
	if d <= 0 {
		return "no time left"
	}
	return spikeSpanWords(d) + " left"
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
	if left := sp.DeadlineAt.Sub(now); left > 0 {
		return fmt.Sprintf("%s, ending at %s (%s).", line, spikeDeadlineWords(*sp.DeadlineAt), spikeLeftWords(left))
	}
	return fmt.Sprintf("%s, ended at %s.", line, spikeDeadlineWords(*sp.DeadlineAt))
}

// spikeUnmeasuredLine is what stands in place of the token bar for a chat or
// person spike (FR-16.1, SD-24); "" for an agent spike.
func spikeUnmeasuredLine(sp *store.Spike) string {
	return tokensNotMeasured(sp.Executor, "This spike")
}

// ---- Closed by the person who ran it (FR-16.4) ----

// spikeClosedBySentence is the closed spike's "Closed by sam, who also ran
// it." (FR-16.4). It is "" for a spike not closed, or closed by someone who
// didn't run it: the page says who closed it in its own way.
func spikeClosedBySentence(sp *store.Spike, f spikeRunFacts) string {
	if sp.State != store.SpikeClosed || sp.ClosedBy == "" || !closerRanIt(sp, f, sp.ClosedBy) {
		return ""
	}
	return "Closed by " + sp.ClosedBy + ", who also ran it."
}
