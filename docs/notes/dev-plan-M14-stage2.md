---
title: "Spikes, stage 2 — dev plan"
type: dev_plan
owner: "M14"
---

# Spikes, stage 2 — dev plan

**For:** [SPEC-021](../specs/SPEC-021-spikes.md) §4 (SD-17 to SD-25, FR-11 to
FR-18), revised after [REVIEW-021](../reviews/REVIEW-021-spikes.md) §6. Stage 3
of [How M13 and M14 are built](orchestration-M13-M14.md).

## Approach

The foundations go first and alone: the migration, the store, the claim
machine's new event, the configuration, and the start's new shape. Their Go
API is fixed below, so the later tasks can run in parallel without guessing
at each other's names.

Then three waves:
- **Wave 2**, in parallel: claiming a spike (T2) and ending one (T3). T2 owns
  `claims.go`, `claim_sweep.go` and the new `spikes_claims.go`; T3 owns
  `spikes_end.go`, `spikes.go`'s findings sentences and the new
  `spikes_executor.go`. Neither edits the other's files.
- **Wave 3**, in parallel: MCP and the chat skill (T4), and the web UI (T5).
  T4 owns `mcp_spike_tools.go`, `mcp_claim_tools.go`, `mcp.go` and the
  tool-set test; T5 owns `ui_spikes.go`, `spike.html` and `ui.go`'s routes.
- **Wave 4**: the integration checks and the end-to-end tests (T6), then the
  review cycle.

Each implementer works in its own git worktree, first merges
`claude/subutai-m14-stage2-spikes` into it, runs `go vet ./...`, `gofmt -l`
on the files it touched, and `go test -race -count=1` for the packages it
touched, against its **own** Postgres cluster (`SUBUTAI_TEST_PGDATA` and
`SUBUTAI_TEST_PGPORT` with `scripts/test-db.sh`), or with
`SUBUTAI_TEST_DB_SUFFIX` set. The lead merges each task.

## Tasks

| id | title | depends_on | description |
|----|-------|------------|-------------|
| T1 | Foundations | | Migration `0016_spike_executors.sql` (FR-11.1); `store.Spike`'s new fields, `store.StartSpike`'s new shape, `store.LockSpike`, `store.RecordSpikeAgentExecution`, `store.SpikesPastDeadline`, `store.SetClaimDeadline`, `EndSpikeState` accepting `time_box`; `lifecycle.ClaimEventExpire` and `expired`; `spikes.default_time_box_hours` (FR-12.3) and the starter config; `Server.StartSpike` taking a `SpikeStartRequest` (FR-12.2), making a chat or person spike's worktree after the commit, and writing an agent spike's execution row. Store, lifecycle and config tests; existing callers of `StartSpike` updated. |
| T2 | Claiming a spike | T1 | FR-13.1 to FR-13.4, FR-13.6, FR-13.7, FR-14: `spikes_claims.go` (the claimable, `ClaimSpike`, `SaveSpikeFindings`, `SubmitSpike`, `ReleaseSpikeClaim`, the spike's claim rules); in `claims.go` the registry, the no-feature lock, `claimResult` for a spike, `claimActivityWords("findings")`, `claim_task`'s pointer to `claim_spike` (`resolveClaimRef`); in `claim_sweep.go` the `deadlineEnder` hook and `ReleaseClaimAnswered` for a spike. Integration tests in `integration_spikes_claims_test.go`. |
| T3 | Ending a spike, and saying who ran it | T1 | FR-15, FR-12.4, FR-16.1's executor line as a function: `EndSpike(time_box)`, the claim's `expire` in the end's transaction, reconciliation (no "run was lost" for chat and person, keep their directories, the deadline, a `done` claim), the leak check before a remake for a claimed spike, the findings' time-box and unmeasured sentences, `spikeExecutor` in `spikes_executor.go`. Integration tests in `integration_spikes_timebox_test.go`. |
| T4 | MCP and the chat skill | T2, T3 | FR-13.5, FR-13.8, FR-16.2, FR-17: `claim_spike`, `save_spike_findings`, `submit_spike`; `get_spike` and `list_spikes`' new fields; the `initialize` sentence; the tool-set test; `chat-skills/run-a-spike/SKILL.md` and its rules test; `TestSpikeIsClaimedOnlyByItsExecutor` replacing `TestSpikeCantBeClaimedYet`. |
| T5 | The web UI | T2, T3 | FR-12.1, FR-16.1, FR-16.3, FR-17.3: the start screen's **Who runs it** and time box; the spike page's executor line, time box, unmeasured line, the person's claim panel and draft editor, the chat claim's panel; the four `POST` routes; the list's executor word; the Inbox's `claim-stale` on a spike. Page tests. |
| T6 | Integration checks and end-to-end | T4, T5 | FR-18: `TestChatSpikeIsHeldByItsClaimAndDeadline`, `TestSpikeExecutorIsRecorded`, `TestChatSpikeTokensAreUnmeasured`, `TestChatSpikeEndToEnd`, `TestPersonSpikeEndToEnd`, and FR-17.3's route test. |

## The Go API that T1 fixes

**`internal/lifecycle/claim.go`**

```go
const ClaimEventExpire ClaimEvent = "expire" // open, submitted, returned → ended
```

**`internal/store`**

```go
// spikes.go
const SpikeTimeBox = "time_box" // a fifth ended_how
const (
    ExecutorAgent  = "agent"
    ExecutorChat   = "chat"   // == WriterChat
    ExecutorPerson = "person" // == WriterPerson
)
type Spike struct { /* … */ Executor string; TimeBoxHours *int; DeadlineAt *time.Time }
type SpikeStart struct {
    Executor     string // agent | chat | person
    Budget       int64  // agent only
    BudgetSource string // agent only
    TimeBoxHours int    // chat and person only
    BaseCommit, WorktreePath string
    RefsAtStart  map[string]string
    Actor        string
}
func StartSpike(ctx, tx, id, SpikeStart) (*Spike, error)        // sets deadline_at = now() + hours
func LockSpike(ctx, tx pgx.Tx, id uuid.UUID) (*Spike, error)    // SELECT … FOR UPDATE
func RecordSpikeAgentExecution(ctx, tx, spikeID, dispatchID uuid.UUID, role, model string) error
func SpikesPastDeadline(ctx, q Querier) ([]Spike, error)         // running, chat or person, deadline_at <= now()
// EndSpikeState accepts SpikeTimeBox.

// claims.go
func SetClaimDeadline(ctx, tx pgx.Tx, claimID uuid.UUID, at time.Time) error
// claimEndReason gains ClaimEventExpire: "expired"; TransitionClaim's ended_by
// is empty for expire.
```

**`internal/config`**

```go
type SpikeConfig struct { DefaultTokenBudget *int64; DefaultTimeBoxHours *int `yaml:"default_time_box_hours"` }
const DefaultSpikeTimeBoxHours = 4
const MaxSpikeTimeBoxHours = 168
func (c *Config) SpikeDefaultTimeBoxHours() int
```

**`internal/server`**

```go
type SpikeStartRequest struct {
    Executor     string // store.ExecutorAgent, ExecutorChat, ExecutorPerson
    Budget       int64  // agent: 0 means the override, else the default
    TimeBoxHours int    // chat and person: 0 means the default
}
func (s *Server) StartSpike(ctx, spikeID uuid.UUID, req SpikeStartRequest, actor string) (*store.Spike, error)
var ErrSpikeTimeBox = errors.New("A spike's time box is a whole number of hours, from 1 to 168.")
```

## The Go API that wave 2 fixes

**T2, `internal/server/spikes_claims.go`**

```go
type spikeClaims struct{ s *Server } // registered as claimables()["spike"]
// deadlineEnder is the optional hook the claim sweep's step 3 uses.
type deadlineEnder interface { endAtDeadline(ctx context.Context, t *claimTarget) error }

type SpikeClaimResult struct {
    Spike       *store.Spike
    Claim       *store.Claim
    WorkingCopy WorkingCopy        // Path, BaseCommit; Branch empty
    Contract    map[string]any     // question, where_it_came_from, decisions, earlier_findings, draft, findings_template
    Rules       []string
    TimeLeft    string             // FR-13.4's sentence
    Renewed     bool
}
func (s *Server) ClaimSpike(ctx, ref string, who Claimant) (*SpikeClaimResult, error)
func (s *Server) SaveSpikeFindings(ctx, ref string, who Claimant, text string) (*store.Spike, error)
func (s *Server) SubmitSpike(ctx, ref string, who Claimant, findings string) (*store.Spike, error) // returns the ended spike
func (s *Server) ReleaseSpikeClaim(ctx, ref string, by string) error
var spikeClaimRuleSentences []string // FR-13.4's rules, said once
func (s *Server) spikeClaimRules() []string
func spikeTimeLeft(sp *store.Spike, now time.Time) string
```

Refusals are `*ClaimRefusal`, as SPEC-020's.

**T3, `internal/server/spikes_executor.go`**

```go
type SpikeExecutor struct {
    Sentence string // FR-16.1's line
    Kind     string // agent | chat | person; "" before the start
    Who, Model, RunID string
    Measured bool
}
func (s *Server) spikeExecutor(ctx context.Context, q store.Querier, sp *store.Spike) (SpikeExecutor, error)
func spikeTimeBoxLine(sp *store.Spike, now time.Time) string // FR-16.1's limit line
func spikeUnmeasuredLine(sp *store.Spike) string            // "" for an agent spike
```

`EndSpike(ctx, id, store.SpikeTimeBox, "")` is refused for an agent spike.

## Status

| id | state | merged at |
|----|-------|-----------|
| T1 | pending | |
| T2 | pending | |
| T3 | pending | |
| T4 | pending | |
| T5 | pending | |
| T6 | pending | |
