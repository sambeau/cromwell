# SPEC-012: See the work

**Status:** **Draft — for Sam's approval.** Authored by Claude. Sam has said he
will approve the spec and the build together. An independent consistency
review is recorded in [REVIEW-012](../reviews/REVIEW-012-see-the-work.md), and
§7 says how each of its findings was dealt with.
**Date:** 2026-09-28
**Roadmap milestone:** M6 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §11
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) §8, "Watching
the work" (approved, binding), and
[DESIGN-008](../design/DESIGN-008-the-workflow-surface.md) §5.2 (page
anatomy), D-4 (tokens, never money), D-6 (human prose) and D-10 (two lists)
**Background:** the [discussion response](../notes/subutai-discussion-response-2026-07-31.md)
§5, which argued that both halves are cheap because every message already
passes through the server, and the
[conformance audit](../notes/research-conformance-audit-2026-07-29.md) item
C-7, review metrics

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Every heading, label, empty state and notice a person reads on the
new pages is a full, plain sentence (D-6). In those places a dispatch is called
an **agent run**, as the activity feed already does. The **transcript** is the
record of one run: what the agent was told, what it did, and what it concluded.

## 0. Framing

DESIGN-010 §8 asks Subutai to answer two questions well:

- *"How is it going?"*, at a glance. **The timeline** answers it.
- *"What exactly happened?"*, when you need to dig. **Transcripts** answer it.

It adds a third: **review health**. A reviewer that approves everything in
seconds is broken, and the numbers should say so.

Today none of this is visible, and most of it isn't kept:

- **Conversations are not stored.** `dispatch.runLoop` holds each run's
  messages in memory and drops them when the run ends. What survives is the
  structured outcome (`dispatches.outcome`), the token counts, and a
  `tool_calls` ledger of names and byte counts, whose latency is always 0.
- **The feature page shows "Recent activity"**, the last ten audit rows filed
  against the feature itself. Everything that happens to its documents and
  tasks, which is most of its life, is filed against those, so it doesn't show.
- **There are no review metrics** beyond the code-review round count that
  `max_review_rounds` checks, and estimate calibration.

The data for the timeline and the metrics is already in the audit stream and
the dispatch table. Transcripts need one new table and a write path. Everything
else is read models and pages.

## 1. Goal

**One claim, which the definition of done checks directly:**

> From a feature's page, a person can see its journey as one line of major
> moments, and click from any moment through to exactly what each agent was
> told, did and concluded.

Two supporting claims:

> A run that fails or is killed part-way still leaves the transcript of what it
> did before it stopped.

> For each reviewer role and model, a person can see how often it approves, how
> much it finds, how many rounds its work takes, and how long it takes to reach
> a verdict. A reviewer that approves nearly everything in seconds stands out.

## 2. Scope

### In scope

1. **Transcripts** (FR-1, FR-2): every agent run's complete conversation,
   written as the run proceeds, with configurable size limits and retention.
2. **The transcript viewer** (FR-3), reachable from task pages, document pages
   and the timeline (FR-4).
3. **The feature timeline** (FR-5, FR-6): a read model over the audit stream,
   shown on the feature page.
4. **Review health** (FR-7, FR-8): the numbers, on their own page, linked from
   Home.
5. **Real latency** in the `tool_calls` ledger (FR-1.6).

### Out of scope (deferred, with destination)

| What | Why not now | Where it goes |
|---|---|---|
| MCP tools to read a transcript or a timeline | The brief keeps M6 off the MCP tool set, which M3 is changing. | Proposed in the handoff: `get_timeline` and `get_agent_run`. |
| A timeline for milestones and roadmaps ("one level up", DESIGN-010 §8) | The per-feature model has to settle first. The milestone view is its roll-up. | A follow-up after M6; noted in the handoff. |
| Human reviewers in the review-health table | People's verdicts are audit rows with no run behind them and no "time to verdict" of the same kind (SD-8). | The handoff proposes a second table once per-user identity lands (M15). |
| Executors, claims and chat-done work | M13. | M13. |
| The rename | M7. | M7. |
| Searching across transcripts | Not asked for. The rows are there to index later. | Unscheduled. |

### Scope decisions

- **SD-1 — A transcript is rows, one per entry, not one document per run.**
  Each entry is a row in a new `transcript_entries` table: a prompt, a turn, a
  piece of text, a tool call, a tool result, a nudge, the outcome or an error.
  Rows are appended as the run goes, so a run that dies leaves everything up
  to its last write, and nothing is ever rewritten. One JSON blob per run would
  have to be rewritten on every turn, or written only at the end, which is the
  failure the brief rules out.

- **SD-2 — A transcript belongs to a run *and its attempt*.** A failed run is
  retried on the same `dispatches` row with `attempt` increased
  (`RequeueDispatch`). Keying entries by `(dispatch_id, attempt, seq)` keeps
  each attempt's conversation separate, and the viewer shows each attempt in
  turn.

- **SD-3 — Every failure is written down where the failure is recorded.**
  `store.MarkDispatchFailed` appends an `error` entry, with the reason, in the
  same transaction as the state change. That covers every way a run fails: an
  error in the loop, the stall sweep finding a run whose process died, and a
  run refused at admission. It matters because `RequeueDispatch` clears
  `dispatches.error`: without this, the reason an earlier attempt failed would
  be lost the moment it was retried.

- **SD-4 — Big entries are cut, not dropped.** An entry over its limit keeps
  its beginning and its end, with a marker in the middle that says how much was
  cut. The original size is stored beside it. Once a run's attempt has used its
  whole budget, later entries are still written, so the shape of the run is
  kept, but their content is replaced by a marker. The limits are configuration
  (FR-2).

- **SD-5 — Transcripts expire; what they concluded doesn't.** By default a
  transcript is deleted 180 days after its run finished. The run's outcome,
  tokens and tool ledger stay for good, because estimates, calibration and
  review health read them. The run is marked as pruned, so the viewer can say
  why there is nothing to show. Setting the retention to 0 keeps transcripts
  for ever.

- **SD-6 — The timeline is a list of phases.** A mapping table turns selected
  audit events into **moments**. Everything else stays in the detail. Each
  moment opens a phase that lasts until the next moment, and a run belongs to
  the phase that was current when it was queued. So *Building* holds the
  implementers and code reviewers, and *Verifying* holds the verifier. A new
  event kind needs one entry in the table and nothing else (FR-5.3).

- **SD-7 — On a feature page, the timeline replaces "Recent activity".** Each
  moment's detail lists every audit event in its phase, labelled as the
  activity feed labels them, and every run. That is a superset of the old
  list: it covers the feature's documents and tasks, not only the feature's own
  rows, and it isn't cut at ten. Keeping both would show the same events twice
  on one page, which design round 4 set out to stop. Project and initiative
  pages keep "Recent activity" unchanged, because they have no timeline.

- **SD-8 — Review health counts agent verdicts only.** A review here is a
  succeeded run whose purpose is a review (`review-*`) or verification
  (`verify-feature`), and whose outcome carries a verdict. Two things are left
  out, deliberately:
  - **The design reviewer's comments-only review** has no verdict, and M3
    retires it (DESIGN-010 §17a item 1).
  - **Human verdicts** are audit rows with no run behind them. "Time to
    verdict" for a person means time since submission, which measures their
    calendar, not the quality of their review. The UI actor is also a single
    configured name until M15, so a per-person table would have one row.

- **SD-9 — The warnings are fixed rules, stated on the page.** A reviewer with
  at least three verdicts in the window is flagged:
  - **"Approves almost everything, and quickly"** when at least 90% of its
    verdicts are approvals and its median time to verdict is under 30 seconds;
  - **"Approves almost everything, and finds almost nothing"** when at least
    90% are approvals and it averages fewer than one finding in five reviews;
  - **"Hardly ever approves"** when 10% or fewer are approvals.

  The rules are constants in code, and the page states them in words. Making
  them configuration can wait until someone wants a different threshold.

- **SD-10 — The migration runner applies any migration it hasn't applied, and
  tolerates a gap.** Today it applies only versions above the highest applied,
  and refuses a gap. M3 owns `0007` and this milestone owns `0008`, built in
  parallel. With the old rule this branch couldn't load its migrations until
  `0007` existed, and a database that had `0008` would never get `0007`. The
  runner now applies every version not yet in `schema_migrations`, in order,
  and requires versions to be unique and increasing rather than contiguous.
  `0007` and `0008` touch different tables, so their order doesn't matter.

- **SD-11 — The tool ledger gets real latency.** The dispatcher times each tool
  call and writes it to `tool_calls.latency_ms` and to the transcript. The
  ledger's schema doesn't change.

- **SD-12 — No money, anywhere new.** None of the new reads selects `cost_usd`,
  and no new template can print it (D-4). Tokens are shown per turn and per
  run.

## 3. Requirements

### FR-1: Writing transcripts

- **FR-1.1 — The table.** Migration `0008` creates `transcript_entries`:
  `id`, `dispatch_id` (references `dispatches`, cascade on delete), `attempt`,
  `seq`, `turn`, `kind`, `tool_name`, `tool_use_id`, `content`,
  `content_bytes` (the size before any cut), `truncated`, `is_error`,
  `latency_ms`, the four token counts, and `created_at`. `(dispatch_id,
  attempt, seq)` is unique. `kind` is one of `system`, `prompt`, `turn`,
  `text`, `tool_call`, `tool_result`, `nudge`, `outcome` and `error`. It also
  adds `dispatches.transcript_pruned_at`.
  *AC:* `TestMigrateFromEmpty` finds the table and the column.
- **FR-1.2 — The prompts, once.** When a run starts, the dispatcher writes the
  assembled system prompt (`system`) and the user prompt (`prompt`), each once
  per attempt, before the first model call.
- **FR-1.3 — Each turn.** After each model call, one `turn` entry records the
  turn number, the model call's latency, the stop reason and that turn's token
  counts. It is followed by one `text` entry per text block, and one
  `tool_call` entry per tool-use block with the tool's name, its call id and
  its input.
- **FR-1.4 — Each tool result.** Each tool result the agent is sent back gets a
  `tool_result` entry with the tool's name, its call id, the result, whether it
  was an error, and how long the tool took. That includes an outcome tool whose
  input failed validation, whose result is the validation message.
- **FR-1.5 — Nudges, the outcome and errors.** The loop's nudge ("Call a tool
  to proceed…") is a `nudge` entry. A valid outcome is an `outcome` entry with
  the tool's name and its input. Every failure is an `error` entry, written by
  `MarkDispatchFailed` in its own transaction (SD-3).
- **FR-1.6 — Written as it goes.** Each turn's entries are written in one
  batch as soon as the model replies, and each turn's tool results in one batch
  after the tools have run: at most two round trips per turn. A write that
  fails is logged and the run carries on. A transcript is a record, and must
  never be the reason a run fails.
  *AC:* `TestTranscriptOfASuccessfulRun`: a mock-provider run with a tool call,
  an invalid outcome and a valid one has, in order: `system`, `prompt`, `turn`,
  `tool_call`, `tool_result`, `turn`, `tool_call`, `tool_result` (the invalid
  outcome, marked as an error), `turn`, `tool_call`, `outcome`. Each turn
  carries its tokens, and the tool result carries a latency.
  *AC:* `TestTranscriptOfAFailedRun`: a run whose second model call fails
  permanently keeps its prompts and its first turn, and ends with an `error`
  entry giving the reason. After a retry that succeeds, attempt 1 and attempt
  2 are both there, each with its own entries.
  *AC:* `TestTranscriptOfAStalledRun`: a run left `running` with no heartbeat,
  as if its process died, keeps the entries it wrote, and the stall sweep adds
  an `error` entry saying it stalled.
- **FR-1.7 — Clean text.** Content is made safe for Postgres before it is
  written: invalid UTF-8 is replaced, and NUL bytes are replaced with U+FFFD. A
  tool that reads a binary file can't break the write.
  *AC:* a unit test cuts and cleans a string with NUL bytes, broken UTF-8 and a
  multi-byte rune on the cut boundary.
- **FR-1.8 — Real latency in the ledger.** `tool_calls.latency_ms` is the
  measured time of the tool call (SD-11).
  *AC:* in `TestTranscriptOfASuccessfulRun` the tool's ledger row and its
  transcript entry carry the same latency.

### FR-2: Size and retention

- **FR-2.1 — Configuration.** `config.yaml` gains an optional `transcripts`
  section. Each field has a default, so an existing project needs no change:

  | Field | Default | What it limits |
  |---|---|---|
  | `max_tool_result_bytes` | 32,768 | One tool result. |
  | `max_entry_bytes` | 262,144 | Any other single entry: a prompt, a piece of text, a tool's input, an outcome. |
  | `max_attempt_bytes` | 4,194,304 | Everything one attempt of one run may store. |
  | `retention_days` | 180 | How long a finished run's transcript is kept. 0 keeps it for ever. |

  A negative value is a configuration error naming the field.
  *AC:* `TestTranscriptConfigDefaults` and a negative-value case in the config
  tests.
- **FR-2.2 — Cutting.** An entry over its limit keeps its first three quarters
  and its last quarter of the limit, cut on character boundaries, with the
  marker *"[… 123,456 bytes cut from the middle …]"* between them. `truncated`
  is set and `content_bytes` keeps the original size (SD-4).
- **FR-2.3 — The attempt budget.** When an attempt's stored bytes reach
  `max_attempt_bytes`, later entries are still written, with their metadata,
  but their content is replaced by *"[Not stored: this run reached its
  transcript size limit.]"*, and `truncated` is set.
  *AC:* `TestTranscriptLimits`: a tool result of 100 KB with the defaults is
  stored cut, with its original size; with a tiny attempt budget, later
  entries exist and carry the marker.
- **FR-2.4 — Pruning.** The heartbeat deletes the transcript entries of runs
  that finished more than `retention_days` ago, and sets their
  `transcript_pruned_at`. It deletes nothing when `retention_days` is 0. It
  never touches `dispatches`' outcome, tokens or `tool_calls`.
  *AC:* `TestTranscriptPruning`: an old finished run's entries go and its
  outcome stays; a recent run's entries stay; with retention 0 nothing goes.

### FR-3: The transcript viewer

- **FR-3.1 — The page.** `GET /ui/run/{id}` shows one agent run. Its heading
  says what the run was for, in words: for example "Reviewing the code of
  *Greeting helper*". Under it: the role and the model; the run's state; when
  it started and how long it took; and its tokens in and out. The breadcrumb
  leads to the feature, and to the task or document the run was about. An
  unknown id is the ordinary not-found page.
- **FR-3.2 — What it concluded, first.** The first panel is the run's
  conclusion. For a review or verification: the verdict in words, the
  reasoning, and the findings with their severity, or each criterion with
  whether it was met and the evidence. For an implementation: its summary and
  the files it changed. For a document written by an agent: that it wrote the
  document, and its length. For anything else: the outcome, laid out as
  indented text. A failed run shows the reason it failed here instead.
- **FR-3.3 — What it was told, once.** The system prompt and the user prompt
  are each shown once, closed by default, with their size.
- **FR-3.4 — What it did.** Then the conversation, turn by turn. Each turn
  shows its number and its tokens in and out, where known. The agent's text is
  shown as written. Each tool call is a closed disclosure whose summary names
  the tool, its main argument (a path, or a command's name), how long it took,
  and whether it failed. Opening it shows the input and the result. Nudges are
  shown as a short line. A cut entry says how much was cut.
- **FR-3.5 — Attempts.** A run with more than one attempt lists them at the
  top, and shows the latest by default. `?attempt=N` shows another. Each
  earlier attempt ends with the error that stopped it.
- **FR-3.6 — Runs without a transcript.** A run from before this milestone
  says so, and shows its outcome and its tool ledger. A pruned run says its
  transcript was removed after the retention period, and when. A queued run
  says it hasn't started.
- **FR-3.7 — A running run follows itself.** While a run is running, its page
  refreshes its conversation every five seconds, and stops when the run ends.
- **FR-3.8 — Tokens, never money** (D-4, SD-12).
  *AC:* `TestUIAgentRunPage` renders a finished review run, a failed run with
  two attempts, and a pre-M6 run, and checks each piece of this section;
  `TestRenderedTemplatesCarryNoCurrency` still passes with the new templates.

### FR-4: Links into transcripts

- **FR-4.1 — The task page** gains an **"Agent runs on this task"** section:
  every run on the task, oldest first. Each row says what the run was for, its
  state, its verdict if it has one, when it ran and its tokens, and links to
  the run.
- **FR-4.2 — The document page** gains the same section for runs on the
  document, which are its reviews: **"Agent reviews of this document"**. It
  sits below the review comments, outside the document's actions, which M3 is
  changing.
- **FR-4.3 — The timeline** links each moment's runs (FR-6.3).
  *AC:* `TestUIRunLinks`: after a full mock-provider loop, the task page and
  the spec's page each link to their runs, and each link resolves.

### FR-5: The timeline read model

- **FR-5.1 — What it reads.** For a feature: every audit event filed against
  the feature, against any document the feature owns (including superseded
  revisions), or against any of its tasks; and every run on those same things.
  Each event carries the type of its document or the title of its task.
- **FR-5.2 — Moments.** A mapping table turns events into moments. The table
  in the build is:

  | Event | When | Moment |
  |---|---|---|
  | `feature.created` | | Created |
  | `feature.sent` | | Sent to development |
  | `document.registered` | a design | Design written |
  | `document.registered` | a spec | Spec written |
  | `document.registered` | a development plan | Plan written |
  | `document.revision_created` | any of those | Spec revised, and so on |
  | `document.transition` | request changes | Spec sent back (round *n*), and so on |
  | `document.transition` | approve | Spec approved, and so on |
  | `devplan.decomposed` | | Broken into *n* tasks |
  | `feature.transition` | to ready | Ready to build |
  | `feature.transition` | start | Building (*x* of *y* tasks done) |
  | `feature.transition` | tasks complete | Verifying |
  | `feature.transition` | rework | Verification sent it back |
  | `feature.transition` | to done | Done |
  | `feature.transition` | to abandoned | Abandoned |
  | `feature.transition` | contract invalidated | Back to an idea, because its design changed |
  | `feature.spec_stale` | | Spec out of date |
  | `task.transition` | request changes | Code review sent back *task* (round *n*) |
  | `checkpoint.created` | | Waiting for a person: *what about* |
  | `checkpoint.responded` | | A person decided: *the answer* |

  Round numbers count sendbacks of the same document or task. "Building (*x*
  of *y*)" counts the feature's tasks as they are now. Each moment records who
  caused it: an agent run, the orchestrator, or the person named on the audit
  row (DESIGN-010 §8, "everything is attributed").
- **FR-5.3 — A new event kind is one entry.** Adding a moment for a new kind
  of event means adding one row to the table, and nothing else. M3's "sent"
  event is the first case: `feature.sent` is mapped already, although nothing
  emits it yet. A feature with no such event simply has no "Sent to
  development" moment. (If M3 names the event differently, the entry changes
  to match.)
  *AC:* `TestTimelineMapping`: a table test over synthetic events, including
  one with `feature.sent` and one without; and a test that registers a new
  kind with one entry and sees its moment.
- **FR-5.4 — Phases and runs.** Each moment opens a phase that runs until the
  next moment. Each run belongs to the phase that was current when it was
  queued, and each audit event to the phase it falls in (SD-6). Runs queued
  before the first moment belong to the first moment.
  *AC:* `TestTimelineOfAFullLoop`: after the full mock-provider loop, the
  moments are *Created, Spec written, Spec approved, Plan written, Plan
  approved, Broken into 1 task, Ready to build, Building (1 of 1 tasks done),
  Verifying, Done*, in that order, and the implementer and code reviewer are
  in *Building*, and the verifier in *Verifying*.
  *AC:* a code-review sendback appears as its own moment with round 1, and an
  escalation as a "Waiting for a person" moment.

### FR-6: The timeline on the feature page

- **FR-6.1 — One line.** The feature page shows the timeline as a single line
  of moments under its heading: *Created · Spec written · Spec approved · … ·
  Done*. Moments with the same label in a row are shown once, with a count
  (*"Code review sent back ×3"*). The current phase is marked, and each moment
  shows its state by icon and word, not colour alone.
- **FR-6.2 — The detail.** Below the line, a closed disclosure, **"What
  happened at each step"**, opens the moments as a list. Each gives its time,
  who caused it, and, inside it, the phase's runs and events.
- **FR-6.3 — Links down.** Each run in a phase links to its transcript, with
  what it was for, its verdict if it has one, and its tokens. A moment about a
  document or task links to that page.
- **FR-6.4 — It stays current.** The timeline reloads itself on the live
  signal, as the dashboard's regions do, from `GET /ui/frag/timeline?feature=<id>`.
- **FR-6.5 — A partial of its own.** The timeline is its own template in its
  own file. `entity.html` changes by one include, and the feature page loads
  the timeline instead of "Recent activity" (SD-7).
  *AC:* `TestUIFeatureTimeline` renders the page after the full loop and finds
  each moment, the links to the runs, and no "Recent activity" section; the
  project and initiative pages still have theirs.

### FR-7: Review health, the read

- **FR-7.1 — What counts.** A review is a succeeded run whose purpose starts
  with `review-` or is `verify-feature`, and whose outcome has a verdict
  (SD-8). The window is all time, or the last 30 or 90 days, by the run's
  finish time.
- **FR-7.2 — Per reviewer.** Reviews are grouped by role and model. For each
  group:
  - **verdicts:** how many, and how many approved, sent back and escalated;
  - **approval rate:** approvals over verdicts;
  - **findings per review:** majors and minors, separately. A comment with no
    severity counts as major, as the review loop treats it. For a
    verification, each unmet criterion counts as a major;
  - **rounds:** reviews per document or task reviewed, as a mean and a
    maximum;
  - **time to verdict:** the median and the slowest, from the run starting to
    it finishing;
  - **tokens per review:** the median.
- **FR-7.3 — Warnings.** Each group gets the warnings SD-9 defines, or none.
  *AC:* `TestReviewHealth`: over seeded runs, the counts, rates, findings,
  rounds and medians come out as calculated by hand; a group of fast
  approvals is flagged as approving quickly; a group that always sends back is
  flagged; a design review with no verdict isn't counted; the window excludes
  an old run.

### FR-8: Review health, the page

- **FR-8.1 — The page.** `GET /ui/review-health` shows one row per reviewer,
  with the numbers from FR-7 in words a person follows ("approved 9 of 10",
  "a median of 12 seconds"). A flagged reviewer is marked with an icon and its
  warning in a sentence, and sorts to the top. The page says in a paragraph
  what each warning means and when it is raised. A window choice switches
  between all time, 30 days and 90 days.
- **FR-8.2 — From Home.** The Home page gains a small card in its side rail,
  **"How the reviewers are doing"**, loaded as a fragment
  (`GET /ui/frag/review-health`): how many reviews, how many reviewers, and
  any warnings, with a link to the page.
- **FR-8.3 — Empty.** With no reviews yet, the page and the card say so in a
  sentence.
  *AC:* `TestUIReviewHealth` renders both with and without reviews, and with a
  flagged reviewer.

## 4. Non-functional requirements

- **NFR-1 — No new stack.** Go templates and HTMX. The only script is HTMX's
  own polling and live-signal attributes. `go vet ./...` and
  `go test -race -count=1 ./...` stay clean.
- **NFR-2 — Storage per run.** With the defaults, one attempt stores at most
  4 MiB of content. Expected sizes: a one-turn review stores its two prompts
  and a few hundred bytes more, typically 10 to 60 KB; a thirty-turn
  implementation stays under about 1 MB, because each tool result is capped at
  32 KB. Postgres compresses text over 2 KB in storage, so the size on disk is
  smaller. The walkthrough reports the sizes the demo actually wrote.
- **NFR-3 — Write overhead per run.** At most two database round trips per
  turn, plus one per attempt for the prompts. That is small against a model
  call, which takes seconds. The walkthrough reports the measured write time
  of the demo's runs.
- **NFR-4 — A transcript never fails a run** (FR-1.6).
- **NFR-5 — Read cost.** The timeline is two queries per feature, on indexed
  columns (`audit_events (ref_type, ref_id, occurred_at)` and
  `dispatches (ref_type, ref_id)`). Review health is one query over
  succeeded review runs in the window, aggregated in Go. The transcript is one
  query on its unique index.
- **NFR-6 — Tokens, never money** (D-4, SD-12). Human prose everywhere (D-6).
- **NFR-7 — Stays off M3's ground.** Nothing here changes `internal/rules/`,
  `internal/server/authoring.go`, the MCP files, `internal/starter/` or the
  document page's actions. The migration is `0008`.
- **NFR-8 — Tested as before.** Integration tests run against real Postgres
  with the mock provider, including a run that fails part-way; the render
  tests cover the new templates.

## 5. Definition of done

1. Every FR's acceptance criteria pass, and the existing suite still passes.
2. **A demo with no AI provider.** A harness drives a feature through the
   whole loop with the mock provider, including one code-review sendback and a
   failed-then-retried run. Playwright, with the pre-installed Chromium,
   screenshots the feature's timeline, a transcript, and review health.
3. The demo and its screenshots are written up in
   `docs/walkthrough-spec-012.md`, with the measured sizes and write times
   (NFR-2, NFR-3).
4. `go vet ./...` and `go test -race -count=1 ./...` are clean.
5. A handoff note records what was built, what was decided, what M3 needs to
   know, a live-smoke checklist for Sam, and the MCP tools proposed for later.
6. **Choices that need Sam's yes:**
   1. the timeline replaces "Recent activity" on feature pages (SD-7);
   2. review health counts agent verdicts only, for now (SD-8);
   3. the warning thresholds (SD-9);
   4. the migration runner change (SD-10);
   5. the 180-day default retention (SD-5).

## 6. Open questions carried forward

- **What M3 calls its "sent" event.** The timeline maps `feature.sent`. If M3
  chooses another name, or files the event against an initiative for a
  multi-feature send, the one table entry changes (FR-5.3).
- **Transcripts for the chat agent.** Chat work uses subscription tokens and
  runs outside the server (DESIGN-010 §8, "unmeasured"). M13 decides what a
  chat-done task records.
- **Decisions in prompts (M11)** will be checked by reading transcripts. The
  prompts are now kept, so that check is possible.
- **A milestone-level timeline**, from DESIGN-010 §8's "one level up".

## 7. Changes after review

Filled in after REVIEW-012.
