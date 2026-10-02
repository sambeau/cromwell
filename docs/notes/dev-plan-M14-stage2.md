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
| T1 | Foundations | | Migration `0016_spike_executors.sql` (FR-11.1, in its stated order); `store.Spike`'s new fields, `store.StartSpike`'s new shape, `store.LockSpike`, `store.RecordAgentExecution` taking the reference type (round 1 for a spike), `store.SpikesPastDeadline`, `store.SetClaimDeadline`, `store.RecordSpikeFindingsActivity`, `EndSpikeState` accepting `time_box`, `spikeEndings`' `time_box` entry; `lifecycle.ClaimEventExpire` and `expired`; the dispatcher writing a `run-spike` run's agent row when it is marked running (FR-12.2); `spikes.default_time_box_hours` (FR-12.3) and the starter config; `Server.StartSpike` taking a `SpikeStartRequest` (FR-12.2, FR-12.5's notices are T5's), making a chat or person spike's worktree after the commit under its lock; the lock helpers of SD-27 and the `EndSpike`/`endSpikeLocked` split (a refactor, no behaviour change); `spikeHadWorkingCopy` replacing `TokensUsed > 0` in `spikes_leak.go` and `ensureSpikeWorktree` (FR-13.3). Store, lifecycle and config tests; existing callers of `StartSpike` updated. |
| T2 | Claiming a spike | T1 | FR-13.1 to FR-13.4, FR-13.6, FR-13.7, FR-14: `spikes_claims.go` (the claimable, `ClaimSpike`, `SaveSpikeFindings`, `SubmitSpike`, `ReleaseSpikeClaim`, the spike's claim rules); in `claims.go` and `claims_task.go` the registry, `lock` and `rules` on the interface, a renewal's refusal for a spike, `claimResult` for a spike, `claimActivityWords("findings")`, `claim_task`'s pointer to `claim_spike`; in `claim_sweep.go` the `deadlineEnder` hook (deadline first), `release_consequence` in the checkpoint context, and `ReleaseClaimAnswered` for a spike. `SubmitSpike` ends through T1's `endSpikeLocked`. Integration tests in `integration_spikes_claims_test.go`. |
| T3 | Ending a spike, and saying who ran it | T1 | FR-15, FR-12.4, FR-16.1's executor line as a function, FR-16.4's audit field: `endSpikeLocked` for `time_box` (refused for an agent spike), the latest claim read under the spike lock (`done` → `concluded`, else `expire`), every ending of a chat or person spike taking the worktree's lock first, reconciliation (no "run was lost" for chat and person, keep their directories, a `done` claim before the deadline), the findings' time-box and unmeasured sentences, `CloseSpike`'s `closer_ran_it`, `spikeExecutor` in `spikes_executor.go`. Integration tests in `integration_spikes_timebox_test.go`. |
| T4 | MCP and the chat skill | T2, T3 | FR-13.5, FR-13.8, FR-16.2, FR-17: `claim_spike`, `save_spike_findings`, `submit_spike`; `get_spike` and `list_spikes`' new fields; the `initialize` sentence; the tool-set test; `chat-skills/run-a-spike/SKILL.md` and its rules test; `TestSpikeIsClaimedOnlyByItsExecutor` replacing `TestSpikeCantBeClaimedYet`. |
| T5 | The web UI | T2, T3 | FR-12.1, FR-16.1, FR-16.3, FR-17.3: the start screen's **Who runs it** and time box; the spike page's executor line, time box, unmeasured line, the person's claim panel and draft editor, the chat claim's panel; the four `POST` routes; the list's executor word; the start notices (FR-12.5); the close dialog's **Ask again** without a budget for chat and person (FR-15.5); "Closed by sam, who also ran it."; the Inbox's release words from the checkpoint context (`ui.go`) and the timeline's spike claim moment (FR-14.2). Page tests. |
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

// SD-27: the worktree's lock, then spikeEndMu. EndSpike takes both and calls
// endSpikeLocked; a caller already holding both (SubmitSpike) calls
// endSpikeLocked directly. An idea or a spike with no worktree path takes no
// working-copy lock.
func (s *Server) withSpikeEndLocks(sp *store.Spike, fn func() error) error
func (s *Server) endSpikeLocked(ctx context.Context, spikeID uuid.UUID, how, note string) error
// spikeHadWorkingCopy: an agent spike that has used tokens, or any started
// chat or person spike.
func spikeHadWorkingCopy(sp *store.Spike) bool
```

**`internal/store`, also**

```go
func RecordAgentExecution(ctx, tx, refType string, refID, dispatchID uuid.UUID, role, model, startHead string) error
// RecordSpikeFindingsActivity: activity `findings` on an open claim, a
// claim.activity audit row and a pending claim-stale withdrawn; reports
// whether the claim was open.
func RecordSpikeFindingsActivity(ctx, tx pgx.Tx, claimID uuid.UUID, actor string) (bool, error)
```

## The Go API that wave 2 fixes

**T2, the interface** (`claims.go`): `claimable` gains

```go
lock(ctx context.Context, tx pgx.Tx, t *claimTarget) error
rules() []string
// releaseConsequence is what the release answer does, for the Inbox.
releaseConsequence() string
```

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
| T1 | done | `6bb71f8` |
| T2 | done | `faa9890`, merged at `8e4ec84` |
| T3 | done | `d94b172` |
| T4 | done | `587a265` |
| T5 | done | `153bb15`, merged at `654d8b4` |
| T6 | done | `0dd4279`, merged at `2296ade` |
| Round 1 fixes | done | `3967c3d` |
| Round 2 fixes | done | `ab981c3`, merged at `e3f5a29` |

Every task was built by an implementer in its own worktree, merged by the
lead, and its worktree removed.
