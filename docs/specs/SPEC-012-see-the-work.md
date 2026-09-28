# SPEC-012: See the work

**Status:** **Approved — Sam, 2026-09-28**, with the build, and with all nine
choices in §5 (DoD 6) as recommended. Authored by Claude. An independent
consistency review, [REVIEW-012](../reviews/REVIEW-012-see-the-work.md), found
four material and thirteen smaller problems in the first draft. All of them
are dealt with in this revision, and §7 says how, finding by finding.
**Date:** 2026-09-28
**Roadmap milestone:** M6 in the
[status report and roadmap](../notes/subutai-status-and-roadmap-2026-09-28.md) §11
**Parent design:** [DESIGN-010](../design/DESIGN-010-subutai.md) §8, "Watching
the work" (approved, binding), and
[DESIGN-008](../design/DESIGN-008-the-workflow-surface.md) §5.2 (page
anatomy), D-4 (tokens, never money), D-6 (human prose), and D-10 (lists, not
dense tables)
**Background:** the [discussion response](../notes/subutai-discussion-response-2026-07-31.md)
§5, which argued that both halves are cheap because every message already
passes through the server, and the
[conformance audit](../notes/research-conformance-audit-2026-07-29.md) item
C-7, review metrics

---

## A note on prose

Requirements are written for the person who builds this and the person who
checks it. Everything a person reads on the new pages is in plain words, with
terms explained (D-6). Headings and labels are short phrases, as on the
existing pages; notices, empty states, warnings, and explanations are full
sentences. On those pages a dispatch is called an **agent run**, as the
activity feed already does. The **transcript** is the record of one run:
what the agent was told, what it did, and what it concluded.

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
> told, did, and concluded. A moment an agent caused leads to the run that
> caused it.

Two supporting claims:

> A run that fails, stalls, or is killed part-way still leaves the transcript
> of what it did before it stopped.

> For each reviewer role and model, a person can see how often it approves,
> how much it finds, how many rounds its work takes, and how long it takes to
> reach a verdict. A reviewer that approves nearly everything in seconds, and
> says almost nothing about why, stands out.

"Exactly" has one stated limit: an entry over its size limit is cut in the
middle, and says so where it is shown (SD-4). The limits for prompts are set
high enough that ordinary prompts are never cut.

## 2. Scope

### In scope

1. **Transcripts** (FR-1, FR-2): every agent run's complete conversation,
   written as the run proceeds, with configurable size limits and retention.
2. **The transcript viewer** (FR-3), reachable from task pages, document pages,
   and the timeline (FR-4).
3. **The feature timeline** (FR-5, FR-6): a read model over the audit stream,
   shown on the feature page.
4. **Review health** (FR-7, FR-8): the numbers, on their own page, linked from
   Home.
5. **Real latency** in the `tool_calls` ledger (FR-1.8).
6. **A run given up on while alive stops** (FR-1.9), because transcripts make
   the old behaviour visible and wrong.

### Out of scope (deferred, with destination)

| What | Why not now | Where it goes |
|---|---|---|
| MCP tools to read a transcript or a timeline | The brief keeps M6 off the MCP tool set, which M3 is changing. | Proposed in the handoff: `get_timeline` and `get_agent_run`. |
| A timeline for milestones and roadmaps ("one level up", DESIGN-010 §8) | The per-feature model has to settle first. The milestone view is its roll-up. | A follow-up after M6; noted in the handoff. |
| People and the chat agent in review health | SD-8. | After M15's per-user identity. |
| Writing `dispatch_id` into the audit rows of verdicts | The call sites are beside code M3 is changing. The timeline finds the cause without it (FR-5.5). | Proposed to M3 in the handoff. |
| Executors, claims, and chat-done work | M13. | M13. |
| The rename | M7. | M7. |
| Searching across transcripts | Not asked for. The rows are there to index later. | Unscheduled. |
| Storing a repeated system prompt once | A saving, not a need, at today's volumes. | Noted in the handoff. |

### Scope decisions

- **SD-1 — A transcript is rows, one per entry, not one document per run.**
  Each entry is a row in a new `transcript_entries` table: a prompt, a turn, a
  piece of text, a tool call, a tool result, a nudge, the outcome, or an error.
  Rows are appended as the run goes, so a run that dies leaves everything up
  to its last write, and nothing is ever rewritten. One JSON blob per run
  would have to be rewritten on every turn, or written only at the end, which
  is the failure the brief rules out.

- **SD-2 — A transcript belongs to a run *and its attempt*.** A failed run is
  retried on the same `dispatches` row with `attempt` increased
  (`RequeueDispatch`). Entries carry `(dispatch_id, attempt, seq)`, so each
  attempt's conversation stays separate, and the viewer shows each in turn.

- **SD-3 — Every failure is written into the transcript where the failure is
  recorded.** `store.MarkDispatchFailed` appends an `error` entry, with the
  reason, in the same transaction as the state change. That covers every way
  a run fails: an error in the loop, the stall sweep finding a run that has
  gone quiet, and a run refused at admission. The reason is on the
  `dispatch.failed` audit row too, but `RequeueDispatch` clears
  `dispatches.error`, and the transcript is where a person reading the run
  looks. The write sits in a savepoint, so it can never stop the failure
  itself being recorded (FR-1.9).

- **SD-4 — Big entries are cut, not dropped.** An entry over its limit keeps
  its beginning and its end, with a marker in the middle that says how much was
  cut, and the original size is stored beside it. Once an attempt has used its
  whole budget, later tool calls, results, and text are still written, so the
  shape of the run is kept, but their content is replaced by a marker. What
  the agent was told and what it concluded are never replaced by the budget
  marker: the prompts, the outcome, and errors sit outside the budget, and the
  prompts have their own, higher limit (FR-2).

- **SD-5 — Transcripts expire; what they concluded doesn't.** By default a
  transcript is deleted 180 days after its run finished. The run's outcome,
  tokens, and tool ledger stay for good, because estimates, calibration, and
  review health read them. The run is marked as pruned, so the viewer can say
  why there is nothing to show. Setting the retention to 0 keeps transcripts
  for ever.

- **SD-6 — The timeline is a list of phases, and each moment also knows its
  cause.** A mapping table turns selected audit events into **moments**, and
  everything else stays in the detail. Each moment opens a phase that lasts
  until the next moment. Two things link a moment to runs:
  - **its phase's runs**: a run belongs to the phase that was current when it
    was first queued. That is decided by the run's first `dispatch.queued`
    audit row, in the audit stream's own order (time, then id). So a run queued
    in the same transaction as a moment belongs to that moment, and a retry
    stays where the run began. *Building* holds the implementers and code
    reviewers, and *Code review sent back* holds the rework it asked for;
  - **its cause**: for a moment an agent caused, the run whose verdict or
    answer produced it (FR-5.5). *Spec approved* leads to the review that
    approved it, even though that review ran in the phase before.

  A new event kind needs one entry in the table and nothing else (FR-5.3).

- **SD-7 — On a feature page, the timeline replaces "Recent activity".** Each
  moment's detail lists the audit events in its phase, labelled as the
  activity feed labels them, and every run. The runs' own lifecycle rows
  (`dispatch.queued`, `dispatch.running`, and so on) are left out, because the
  run rows carry the same facts. The detail is a superset of the old list in
  everything else: it covers the feature's documents and tasks, not only the
  feature's own rows, and it isn't cut at ten. Keeping both would show the
  same events twice on one page, which design round 4 set out to stop.
  Project and initiative pages keep "Recent activity" unchanged, because they
  have no timeline.

- **SD-8 — Review health counts agent verdicts only, for now. This narrows
  DESIGN-010 §8, and needs Sam's yes as such.** §8 asks for review health "per
  reviewer" and attribution of every verdict to an agent, the chat AI, or a
  human. A review here is a succeeded run whose purpose is a review
  (`review-*`) or verification (`verify-feature`), and whose outcome carries a
  verdict. Left out, deliberately:
  - **A review of a document type a person approves.** That reviewer only
    comments; it has no verdict to count (SPEC-009 FR-2.2).
  - **People's verdicts.** Their approval rate and findings are well defined,
    but until M15 every person acts as one configured UI actor, so a
    per-person row would be one row for everyone. Time to verdict for a
    person also measures their calendar, not their care.
  - **The chat agent**, which can't give a review verdict today (DEC-006
    Amendment 1 keeps verdicts a human relay).

  The page says in a sentence that people's decisions aren't counted, and
  where they are.

- **SD-9 — The warnings are fixed rules, stated on the page from the same
  values.** No reviewer is flagged before it has given **five** verdicts in
  the window. Then:
  - **"Approves almost everything, quickly, and says almost nothing about
    why"** when at least 90% of its verdicts are approvals, its median time to
    verdict is under 30 seconds, *and* its median answer is under 300 output
    tokens. Speed alone isn't the signal: a document review is a single model
    call, so its time is the model's latency, and a fast model that reasons
    at length is not a rubber stamp.
  - **"Approves almost everything, and finds almost nothing"** when at least
    90% are approvals and its reviews average fewer than one finding, major or
    minor, in five. This rule reads `review-*` verdicts only. A verifier can't
    approve while a criterion is unmet, so its approvals carry no findings by
    construction, and a good verifier would always trip it.
  - **"Hardly ever approves"** when 10% or fewer are approvals.

  The rules are constants in `internal/reviewhealth`; the page's explanation
  is written from those constants, so the two can't drift.

- **SD-10 — The migration runner applies any migration it hasn't applied, and
  tolerates a gap.** Before this spec it applied only versions above the
  highest applied, and refused a gap. M3 owns `0007` and this milestone owns
  `0008`, built in parallel. Under the old rule this branch couldn't load its
  migrations until `0007` existed, and a database that had `0008` would never
  get `0007`. The runner now applies every version not yet in
  `schema_migrations`, in order, and requires versions to be unique rather
  than contiguous. The fix protects only databases migrated by a binary that
  has it; the handoff tells M3 and Sam what that means (§6).

- **SD-11 — The tool ledger gets real latency.** The dispatcher times each tool
  call and writes it to `tool_calls.latency_ms` and to the transcript. The
  ledger's schema doesn't change. It has no attempt column, and its `seq`
  restarts on each attempt, so for a run from before transcripts the viewer
  shows the ledger as one list.

- **SD-12 — No money, anywhere new.** None of the new reads selects `cost_usd`,
  and no new template can print it (D-4). Tokens are shown per turn and per
  run. No audit payload field is rendered as it stands: a *Waiting for a
  person* moment is labelled from the checkpoint's kind, never its question,
  because the budget checkpoint's question is a sentence about dollars. A
  person's answer is labelled from the answer's fields, never its free text.

- **SD-13 — Transcripts keep what tools read, secrets included.** A tool that
  reads a `.env` file, or prints an environment variable, now stores what it
  read in Postgres, and the run's page shows it. For a single local operator
  that is acceptable, and it is the price of "exactly what it did". Redacting
  would need to know what a secret looks like, and would make the record
  inexact. The handoff says so, so it is a known property rather than a
  surprise.

- **SD-14 — The line of moments sits above the page body. This adds to
  DESIGN-008 §5.2, and needs Sam's yes as such.** §5.2 draws the page as
  breadcrumbs, the design document as the body, and the relations across the
  bottom, with activity among them. DESIGN-010 §8's "how is it going, at a
  glance" asks for the journey before the reading, so the timeline is placed
  between the page head and the body: one line of moments, and a closed
  disclosure holding the detail. The design document is still the body, one
  line lower.

- **SD-15 — A run that was given up on stops.** The stall sweep can fail a run
  whose loop is still alive, for example when one model call, with its
  retries, outlasts the stall threshold. Before this spec, the loop carried on
  and could record its outcome over the retry's row. Now the loop checks its
  own attempt is still current before and after each model call, stops if it
  isn't, and success is recorded only for the current attempt (FR-1.9).

## 3. Requirements

### FR-1: Writing transcripts

- **FR-1.1 — The table.** Migration `0008` creates `transcript_entries`:
  `id`, `dispatch_id` (references `dispatches`, cascade on delete), `attempt`,
  `seq`, `turn`, `kind`, `tool_name`, `tool_use_id`, `content`,
  `content_bytes`, `truncated`, `is_error`, `latency_ms`, the four token
  counts, and `created_at`. `kind` is one of `system`, `prompt`, `turn`,
  `text`, `tool_call`, `tool_result`, `nudge`, `outcome`, and `error`. The
  entries of an attempt are ordered by `seq`, then by `id`, which is
  time-ordered; `(dispatch_id, attempt, seq)` is indexed but not unique
  (FR-1.9). The migration also adds `dispatches.transcript_pruned_at`.
  *AC:* `TestMigrateFromEmpty` finds the table.
- **FR-1.2 — The prompts, once.** When a run starts, the dispatcher writes the
  assembled system prompt (`system`) and the user prompt (`prompt`), each once
  per attempt, before the first model call.
- **FR-1.3 — Each turn.** After each model call, one `turn` entry records the
  turn number, the model call's latency, the stop reason, and that turn's
  token counts. It is followed by one `text` entry per text block, and one
  `tool_call` entry per tool-use block with the tool's name, its call id, and
  its input.
- **FR-1.4 — Each tool result.** Each tool result the agent is sent back gets a
  `tool_result` entry with the tool's name, its call id, the result, whether
  it was an error, and how long the tool took. That includes an outcome tool
  whose input failed validation, whose result is the validation message.
- **FR-1.5 — Nudges, the outcome, and errors.** The loop's nudge ("Call a tool
  to proceed…") is a `nudge` entry. A valid outcome is an `outcome` entry with
  the tool's name and its input. Every failure is an `error` entry, written by
  `MarkDispatchFailed` (SD-3).
- **FR-1.6 — Written as it goes.** Each turn's entries are written in one
  batch as soon as the model replies, and each turn's tool results in one
  batch after the tools have run: at most two round trips per turn. A write
  that fails is logged and the run carries on. A transcript is a record, and
  must never be the reason a run fails (NFR-4). A batch is one implicit
  transaction, so a failed write loses that batch, not the run.
  *AC:* `TestTranscriptOfASuccessfulRun`: a mock-provider review that calls a
  tool it isn't offered, then submits an invalid verdict, then a valid one,
  has, in order: `system`, `prompt`, `turn`, `text`, `tool_call`,
  `tool_result`, `turn`, `tool_call`, `tool_result` (the invalid outcome,
  marked as an error), `turn`, `tool_call`, `outcome`. Each turn carries its
  tokens, and the tool result carries a latency.
  *AC:* `TestTranscriptOfAFailedRun`: a run whose second model call fails keeps
  its prompts and its first turn, and ends with an `error` entry giving the
  reason. After a retry that succeeds, attempts 1 and 2 are both there, each
  with its own entries, and attempt 1 is unchanged.
  *AC:* `TestTranscriptOfAStalledRun`: a run left `running` with no heartbeat,
  as if its process died, keeps the entries it wrote, and the stall sweep adds
  an `error` entry saying it stalled.
- **FR-1.7 — Clean text.** Content is made safe for Postgres before it is
  written: invalid UTF-8 is replaced, and NUL bytes are replaced with U+FFFD.
  A tool that reads a binary file can't break the write. `content_bytes` is
  the size after cleaning and before any cut.
  *AC:* `TestCutAndClean` cleans a string with a NUL byte and broken UTF-8,
  and cuts one with a multi-byte character on the cut boundary.
- **FR-1.8 — Real latency in the ledger.** `tool_calls.latency_ms` is the
  measured time of the tool call (SD-11).
  *AC:* `TestToolLatencyIsMeasured`: the tool's ledger row and its transcript
  entry carry the same latency.
- **FR-1.9 — Two writers, and a run given up on.** The running loop numbers
  its own entries; the failure entry takes the next number from the table.
  Because `seq` isn't unique, the two can't collide, and a tie is broken by
  the time-ordered id. The failure entry is written in a savepoint, so it can't
  fail the transaction that records the failure. The loop refreshes its
  heartbeat for its own attempt before and after each model call; when the
  attempt is no longer the run's current, running one, the loop stops, writes
  nothing more, and records no outcome. `MarkAttemptSucceeded` refuses to
  record success for an attempt that isn't current (SD-15).
  *AC:* `TestTranscriptOfARunGivenUpWhileAlive`: the stall sweep fails a run
  while its loop is waiting on the model; when the reply comes, the run stays
  failed, the late approval isn't applied, and the transcript ends with the
  stall.

### FR-2: Size and retention

- **FR-2.1 — Configuration.** `config.yaml` gains an optional `transcripts`
  section. Each field has a default, so an existing project needs no change.
  For the byte limits, 0 or leaving the field out means its default:

  | Field | Default | What it limits |
  |---|---|---|
  | `max_prompt_bytes` | 1,048,576 | The system prompt, and the user prompt. |
  | `max_tool_result_bytes` | 32,768 | One tool result. |
  | `max_entry_bytes` | 262,144 | Any other single entry: a piece of text, a tool's input, an outcome. |
  | `max_attempt_bytes` | 4,194,304 | The tool calls, results, text, and nudges of one attempt. |
  | `retention_days` | 180 | How long a finished run's transcript is kept. 0 keeps it for ever. |

  A negative value is a configuration error naming the field.
  *AC:* `TestTranscriptConfigDefaults`: the defaults apply, explicit values
  are kept, retention 0 is kept as 0, and negative values name their fields.
- **FR-2.2 — Cutting.** An entry over its limit keeps its first three quarters
  and its last quarter of the limit, cut on character boundaries, with the
  marker *"[… 123,456 bytes cut from the middle …]"* between them. `truncated`
  is set, and `content_bytes` keeps the original size (SD-4).
- **FR-2.3 — The attempt budget.** When an attempt's stored bytes would pass
  `max_attempt_bytes`, later entries are still written, with their metadata,
  but their content is replaced by *"[Not stored: this run reached its
  transcript size limit.]"*, and `truncated` is set. The prompts, the outcome,
  and errors are outside the budget.
  *AC:* `TestTranscriptLimits`: a 100 KB tool result is stored cut, with its
  original size. `TestTranscriptBudget`: with a ten-byte budget, the tool call
  past it keeps its name and carries the marker, and the prompts and outcome
  are whole.
- **FR-2.4 — Pruning.** A heartbeat duty, paced to once an hour, deletes the
  transcript entries of runs that finished more than `retention_days` ago, and
  sets their `transcript_pruned_at`. It deletes nothing when `retention_days`
  is 0. It never touches `dispatches`' outcome or tokens, or `tool_calls`. A
  pruned run that is later retried shows its new attempt's transcript.
  *AC:* `TestTranscriptPruning`: a recent run keeps its entries; retention 0
  removes nothing; an old run's entries go, its outcome and tokens stay, and it
  is marked.

### FR-3: The transcript viewer

- **FR-3.1 — The page.** `GET /ui/run/{id}` shows one agent run. Its heading
  says what the run was for, in words: for example *Reviewing the code of
  "Greeting helper"*. Under it: the role and the model, the run's state, when
  it started and how long it took, and its tokens in and out. The breadcrumb
  leads to the feature, and to the task or document the run was about. An
  unknown id is the ordinary not-found page.
- **FR-3.2 — What it concluded, first.** The first panel is the run's
  conclusion:
  - for a review or verification: the verdict in words, the reasoning, and
    the findings with their severity, or each criterion with whether it was
    met and the evidence;
  - for an implementation: its summary and the files it changed;
  - for a document written by an agent: that it wrote the document, and about
    how long it is;
  - for anything else: the outcome, laid out as indented text.

  A failed attempt says why in a plain sentence, chosen by the kind of failure
  (it stalled, the model's service didn't answer, it ran out of turns, its
  model isn't configured), with the engine's own text beneath it in a closed
  disclosure.
- **FR-3.3 — What it was told, once.** The system prompt and the user prompt
  are each shown once, closed by default, with their size, and say so if they
  were cut.
- **FR-3.4 — What it did.** Then the conversation, turn by turn. Each turn
  shows its number and its tokens in and out, where known. The agent's text is
  shown as written. Each tool call is a closed disclosure whose summary names
  the tool, its main argument (a path, or a command's name), how long it took,
  and whether it failed. Opening it shows the input and the result. Nudges are
  a short line, and a cut entry says how much was cut.
- **FR-3.5 — Attempts.** A run with more than one attempt lists them at the
  top, and shows the latest by default. `?attempt=N` shows another. Each
  earlier attempt ends with the failure that stopped it.
- **FR-3.6 — Runs without a transcript.** A run from before this milestone
  says so, and shows its outcome and its tool ledger as one list. A pruned
  run says its transcript was removed after the retention period, and when. A
  queued run says it hasn't started.
- **FR-3.7 — A running run follows itself, without closing anything.** While
  the run is running, a status line at the top polls every five seconds for
  how many turns the run has taken, and offers a link to reload when there are
  more than the page shows. When the run ends, the line says so and stops
  polling. The conversation itself is never swapped, so nothing the reader has
  opened closes.
- **FR-3.8 — Tokens, never money** (D-4, SD-12).
  *AC:* `TestUIAgentRunPage` renders a run whose first attempt failed
  part-way and whose second approved with a finding, and checks the heading,
  the conclusion, the finding, the prompts, the turns and their tokens, both
  attempts, the failure in words and in the engine's text, and the link to the
  document. It then checks a run with no transcript, a pruned run, a running
  run's status line and its fragment, and an unknown id. No rendered page
  shows a currency amount; `TestRenderedTemplatesCarryNoCurrency` still
  passes over the new templates.

### FR-4: Links into transcripts

- **FR-4.1 — The task page** gains an **"Agent runs on this task"** section:
  every run on the task, oldest first. Each row says what the run was for, its
  verdict if it has one or else its state, its tokens, how long it took, and
  its attempt if it was retried, and links to the run.
- **FR-4.2 — The document page** gains the same list for runs on the document,
  which are its reviews: **"Agent reviews of this document"**. It sits below
  the review comments, inside the live region, and outside the document's
  actions, which M3 is changing. It comes from the page's own handler, not from
  `renderDocumentPage` (M3's ground): after a review action the list returns
  with the next live refresh.
- **FR-4.3 — The timeline** links each moment's runs, and its cause (FR-6.3).
  *AC:* `TestUIRunLinks`: after a full loop with a code-review sendback, the
  task page lists its four runs, the spec's page lists its review, and each
  link resolves.

### FR-5: The timeline read model

- **FR-5.1 — What it reads.** For a feature: every audit event filed against
  the feature, against any document the feature owns (superseded revisions
  too), or against any of its tasks, in the stream's order (time, then id);
  and every run on those same things. Each event carries the type of its
  document, or the title of its document or task. Tasks deleted by a later
  decomposition drop out, with their events; they never had runs.
- **FR-5.2 — Moments.** A mapping table in `internal/timeline` turns events
  into moments. The `event` values are the lifecycle's own:

  | Audit kind | Payload `event`, or other condition | Moment |
  |---|---|---|
  | `feature.created` | | Created |
  | `feature.sent` | | Sent to development |
  | `document.registered` | a design, spec, or development plan | Design written, Spec written, Plan written |
  | `document.revision_created` | the same types | Spec revised, and so on |
  | `document.transition` | `request_changes` | Spec sent back (round *n*), and so on |
  | `document.transition` | `approve` | Spec approved, and so on |
  | `devplan.decomposed` | | Broken into *n* tasks |
  | `feature.transition` | `contract_approved` | Ready to build |
  | `feature.transition` | `start` | Building (*x* of *y* tasks done) |
  | `feature.transition` | `tasks_complete` | Verifying |
  | `feature.transition` | `rework` | Verification sent it back |
  | `feature.transition` | `verified` | Done |
  | `feature.transition` | `abandon` | Abandoned |
  | `feature.transition` | `contract_invalidated` | Back to an idea, because its design changed |
  | `feature.spec_stale` | | Spec out of date |
  | `task.transition` | `request_changes` | Code review sent back *task* (round *n*) |
  | `checkpoint.created` | labelled by the checkpoint's kind | Waiting for a person: *what about* |
  | `checkpoint.responded` | labelled by the answer's fields | A person decided: *the answer* |

  Round numbers count the times the same document or task was sent back.
  "Building (*x* of *y*)" counts the feature's tasks as they are now, leaving
  out abandoned ones. Each moment has a **state**, one of the lifecycle marks
  the pages already use, which picks its icon and hue.
- **FR-5.3 — A new event kind is one entry.** Adding a moment for a new kind
  of event means adding one row to the table, and nothing else. M3's "sent"
  event is the first case: `feature.sent` is mapped already, although nothing
  emits it yet, and a feature without one simply has no "Sent to development"
  moment. This holds on one condition, **handed to M3 as a requirement**:
  *"sent" is audited once per feature, against the feature, as
  `feature.sent`, even when the send is made from an initiative.* The other M3
  events the timeline expects, and would map with one line each once M3 names
  them, are: a person raising an issue on a spec, a spec held for a person,
  and "let the reviewer decide".
  *AC:* `TestTimelineMapping`: a table test over synthetic events, with and
  without `feature.sent`. `TestTimelineNewKindIsOneEntry`: a new kind, added as
  one entry, becomes a moment.
- **FR-5.4 — Phases and runs.** Each moment opens a phase that runs until the
  next moment. A run belongs to the phase current at its first
  `dispatch.queued` audit row; a run queued in the same transaction as a
  moment belongs to that moment; runs and events before the first moment
  belong to it (SD-6). A run with no queue row in the stream falls back to its
  `queued_at`.
  *AC:* `TestTimelinePhases` and `TestTimelinePlacesRunsByTheirQueueRow` (the
  tie and the retry). `TestTimelineOfAFullLoop`: after a full mock-provider
  loop with one code-review sendback, the moments are *Created, Spec written,
  Spec approved, Plan written, Plan approved, Broken into 1 task, Ready to
  build, Building (1 of 1 task done), Code review sent back "Greeting helper"
  (round 1), Verifying, Done*, in that order, with a *Sent to development*
  moment allowed after *Created*. The first build and review are in
  *Building*, the rework and its review in the sendback, and the verifier in
  *Verifying*.
- **FR-5.5 — Who caused a moment, and which run.** Each moment records who
  caused it (DESIGN-010 §8, "everything is attributed"):
  - **an agent**, when the audit row's actor is the orchestrator or an agent
    role and a run caused it. The run is the moment's **cause**: the latest
    run of the purposes the table names for that moment, on the same document
    or task or on the feature, that finished at or before the event. Only
    succeeded runs count, except for *Waiting for a person*, whose cause may
    be a failed run;
  - **the chat agent**, when the actor is the configured `mcp_actor`;
  - **a person**, named, for any other actor;
  - **the system**, for the orchestrator's own steps with no run behind them.

  *AC:* `TestTimelineCauses` covers each of the four, a spec sent back and
  then approved (each led to by its own review), a code-review sendback, and a
  failed run behind a checkpoint. In `TestTimelineOfAFullLoop`, *Spec
  approved*, *Plan approved*, the sendback, and *Done* each lead to their
  review or verification, the sendback's cause asked for changes, and *Spec
  written* is by the person who registered it.

### FR-6: The timeline on the feature page

- **FR-6.1 — One line.** Between the feature page's head and its body (SD-14),
  the timeline shows a single line of moments: *Created · Spec written · Spec
  approved · … · Done*. Moments of the same kind in a row are shown once, by
  a short fold label and a count (*"Code review sent back ×3"*); the table
  gives each foldable kind its fold label, since the full labels differ by
  task and round. A lone moment keeps its full label. The last mark is where
  the feature is now, and is outlined and named as such for screen readers.
  Each mark shows its state by icon and word, not colour alone.
- **FR-6.2 — The detail.** Below the line, a closed disclosure, **"What
  happened at each step"**, opens the moments as a list. Each moment gives its
  time and who caused it; then, if an agent did, **"What led to this"** and
  the run; then the runs of its phase; then its events.
- **FR-6.3 — Links down.** Each run links to its transcript, with what it was
  for, its verdict if it has one, and its tokens. A moment about a document or
  task links to that page.
- **FR-6.4 — It stays current, without closing anything.** The line reloads on
  the live signal from `GET /ui/frag/timeline?feature=<id>`, which returns the
  line only. The detail isn't swapped, so a reader's open disclosure stays
  open.
- **FR-6.5 — A partial of its own.** The timeline's templates are in their own
  file, `observe.html`. `entity.html` changes by one include for the timeline,
  one for the task page's runs, and one for the document page's runs, each
  outside the page head and the document's actions. The feature page stops
  loading "Recent activity", so that section's existing `{{if .Activity}}`
  hides it without a template change (SD-7).
  *AC:* `TestUIFeatureTimeline`: the page shows the line and the detail, the
  moments, a cause, a person named, a budget checkpoint labelled by its kind
  with no currency on the page, links to every run that resolve, and no
  "Recent activity"; the fragment is the line only; a bad id is a bad request;
  the initiative page keeps "Recent activity". The demo checks in the browser
  that an open detail stays open across a live refresh (DoD 2).

### FR-7: Review health, the read

- **FR-7.1 — What counts.** A review is a succeeded run whose purpose starts
  with `review-` or is `verify-feature`, and whose outcome has a verdict
  (SD-8). The window is all time, or the last 30 or 90 days, by the run's
  finish time.
- **FR-7.2 — Per reviewer.** Reviews are grouped by role and model. For each
  group:
  - **verdicts:** how many, and how many approved, sent back, and escalated;
  - **approval rate:** approvals over verdicts;
  - **findings per review:** majors and minors, separately. A comment with no
    severity counts as major, as the review loop treats it. For a
    verification, each unmet criterion counts as a major;
  - **rounds:** reviews per document, task, or feature reviewed, as a mean and
    a maximum;
  - **time to verdict:** the median and the slowest, from the run starting to
    it finishing, so time spent queued doesn't count;
  - **tokens per review** and **output per review:** the medians.
- **FR-7.3 — Warnings.** Each group gets the warnings SD-9 defines, or none.
  Flagged reviewers sort first, then the busiest.
  *AC:* `TestSummarise`, by hand: the counts, rates, findings, rounds, and
  medians; a rubber stamp flagged twice; a reviewer that never approves
  flagged; and three healthy reviewers left alone: a careful one, a verifier
  that approves 9 of 10, and a fast model that reasons at length.
  `TestFromOutcome` reads both outcome shapes.

### FR-8: Review health, the page

- **FR-8.1 — The page.** `GET /ui/review-health` shows one card per reviewer,
  not a dense table (D-10): its figures as short blocks, each with a sentence
  beneath ("approved 9 of 10 verdicts", "the median; the slowest took 2
  minutes"). A flagged reviewer carries its warning in a sentence and sorts
  to the top. A paragraph explains what counts, and states the warning rules
  from SD-9's constants. A choice of period switches between all time, 90
  days, and 30 days.
- **FR-8.2 — From Home.** The Home page gains a small card in its side rail,
  **"How the reviewers are doing"**, loaded as a fragment
  (`GET /ui/frag/review-health`): how many verdicts, from how many reviewers,
  and how many need a look, with a link to the page. Without script, the card
  is the link.
- **FR-8.3 — Empty.** With no reviews yet, the page and the card say so in a
  sentence.
  *AC:* `TestUIReviewHealth`: empty, then with seeded verdicts: the rubber
  stamp flagged and sorted first, a comments-only review left out, the window
  excluding an old verdict and so flagging a reviewer it no longer offsets,
  the card counting who needs a look, and no currency.

## 4. Non-functional requirements

- **NFR-1 — No new stack.** Go templates and HTMX, with no new script: the
  live parts use HTMX's own polling and the existing live signal. `go vet
  ./...` and `go test -race -count=1 ./...` stay clean.
- **NFR-2 — Storage per run, reported.** With the defaults, the budgeted part
  of one attempt stores at most 4 MiB, plus its prompts and outcome. Expected:
  a one-turn review stores its two prompts and a few hundred bytes more,
  typically 10 to 60 KB; a thirty-turn implementation stays under about 1 MB,
  because each tool result is capped at 32 KB. Postgres compresses text over
  2 KB in storage, so the size on disk is smaller. The walkthrough reports
  the sizes the demo wrote; this is a report, not a gate.
- **NFR-3 — Write overhead per run, reported.** At most two database round
  trips per turn, one for the prompts, and one heartbeat before and after each
  model call. That is small against a model call, which takes seconds. The
  walkthrough reports the measured write time of the demo's runs; this is a
  report, not a gate.
- **NFR-4 — A transcript never fails a run, and never stops a failure being
  recorded** (FR-1.6, FR-1.9).
- **NFR-5 — Read cost.** The timeline is two queries per feature, on indexed
  columns (`audit_events (ref_type, ref_id, occurred_at)` and
  `dispatches (ref_type, ref_id)`). Review health is one query over succeeded
  review runs in the window, aggregated in Go. A transcript is one query on
  its index.
- **NFR-6 — Tokens, never money** (D-4, SD-12). Plain words everywhere (D-6).
- **NFR-7 — Stays off M3's ground.** Nothing here changes `internal/rules/`,
  `internal/server/authoring.go`, the MCP files, `internal/starter/`, or the
  document page's actions. The migration is `0008`. The handoff names every
  line of `entity.html` this milestone touches.
- **NFR-8 — Tested as before.** Integration tests run against real Postgres
  with the mock provider, including runs that fail part-way, stall, and are
  given up on while alive; the render tests cover the new templates.

## 5. Definition of done

1. Every FR's acceptance criteria pass, and the existing suite still passes.
2. **A demo with no AI provider.** A harness drives a feature through the
   whole loop with the mock provider, including one code-review sendback and a
   failed-then-retried run. Playwright, with the pre-installed Chromium,
   screenshots the feature's timeline, a transcript, and review health, and
   checks that an open timeline detail stays open across a live refresh.
3. The demo and its screenshots are written up in
   `docs/walkthrough-spec-012.md`, with the measured sizes and write times
   (NFR-2, NFR-3).
4. `go vet ./...` and `go test -race -count=1 ./...` are clean.
5. A handoff note records what was built, what was decided, what M3 needs to
   know, a live-smoke checklist for Sam, and the MCP tools proposed for later.
6. **Choices that need Sam's yes:**
   1. the timeline replaces "Recent activity" on feature pages (SD-7);
   2. review health counts agent verdicts only, for now, as a narrowing of
      DESIGN-010 §8 (SD-8);
   3. the warning rules, as reworked after review (SD-9);
   4. the migration runner change (SD-10);
   5. the 180-day default retention (SD-5);
   6. the line of moments above the page body, as an addition to DESIGN-008
      §5.2 (SD-14);
   7. the requirement handed to M3: "sent" is audited once per feature,
      against the feature (FR-5.3);
   8. transcripts keep what tools read, secrets included (SD-13);
   9. a run given up on while alive stops, which changes the engine's
      behaviour (SD-15).

## 6. Open questions carried forward

- **What M3 calls its events.** The timeline maps `feature.sent`, on the
  condition in FR-5.3. The issue, hold, and "let the reviewer decide" events
  need one line each once named.
- **The migration gap on shared databases.** A database migrated with
  `0008` by this branch, and then used from a branch with the old runner,
  will skip `0007` silently, and the status's schema version (the highest
  applied) hides the gap. M3 should take the runner change before running
  against any database this branch has touched, or use a fresh one. Once both
  have merged, the gap closes. Whether `0007` and `0008` really are
  independent can only be checked when the second merges.
- **Writing `dispatch_id` into verdict audit rows.** The cause rule of FR-5.5
  works from times and purposes. Recording the run on the audit row itself,
  at the call sites in `actions.go` and `actions_phase2.go`, would make it
  exact. Those sites are beside M3's changes, so it is proposed, not done.
- **Heartbeats during long tool calls.** The loop refreshes its heartbeat per
  model call, so a single long tool call (a test run) can still outlast the
  stall threshold. FR-1.9 makes that safe; refreshing around long tool calls
  would make it rare.
- **Transcripts for the chat agent.** Chat work runs outside the server
  (DESIGN-010 §8, "unmeasured"). M13 decides what a chat-done task records.
- **Decisions in prompts (M11)** will be checked by reading transcripts. The
  prompts are now kept whole, so that check is possible.
- **A milestone-level timeline**, from DESIGN-010 §8's "one level up".
- **The inbox and Home print a checkpoint's question as it stands**, and the
  budget question is a sentence about dollars. That predates this spec
  (SD-12 keeps it off the new pages) and should be fixed where it lives.

## 7. Changes after review

How each [REVIEW-012](../reviews/REVIEW-012-see-the-work.md) finding was dealt
with.

| Finding | What changed |
|---|---|
| R12-1 (material) | FR-5.5 and SD-6: each moment an agent caused links to its **cause**, the run whose verdict produced it, separately from its phase's runs. The detail shows "What led to this" first. `TestTimelineCauses` and `TestTimelineOfAFullLoop` check it. Writing `dispatch_id` into verdict audit rows is proposed to M3 (§6). |
| R12-2 (material) | FR-1.9 and SD-15: `seq` is no longer unique, the failure entry is written in a savepoint, the loop stops when its attempt was given up on, and success is recorded only for the current attempt. `TestTranscriptOfARunGivenUpWhileAlive` holds a model call open across a stall. |
| R12-3 (material) | SD-9 reworked: five verdicts before any warning; speed counts only with a terse answer; "finds almost nothing" reads reviews only. `TestSummarise` adds a verifier approving 9 of 10 and a fast model that reasons, neither flagged. |
| R12-4 (material) | FR-5.3 hands M3 a requirement: `feature.sent`, once per feature, against the feature, even from an initiative. The issue, hold, and "let the reviewer decide" events are listed as expected one-line entries. The full-loop AC allows a *Sent* moment. |
| R12-5 | SD-14 records the line above the body as an addition to DESIGN-008 §5.2. FR-6.5 names the three one-line includes; the task and document lists are partials in `observe.html`. The handoff names the lines touched. |
| R12-6 | SD-6 and FR-5.4: a run is placed by its first `dispatch.queued` row in (time, id) order, so same-transaction ties and retries are defined. `TestTimelinePlacesRunsByTheirQueueRow` covers both. |
| R12-7 | SD-12: *Waiting for a person* is labelled by kind and *A person decided* by the answer's fields; no payload field is rendered as it stands. `TestUIFeatureTimeline` puts a dollar budget question on the record and checks the page has no currency. The inbox's existing leak is in §6. |
| R12-8 | FR-3.7: a running run's page polls a status line only. FR-6.4: the timeline's live refresh swaps the line only. The demo checks an open detail survives a refresh. |
| R12-9 | FR-2.1: prompts have their own 1 MiB limit; FR-2.3: prompts, the outcome, and errors are outside the budget; 0 means the default for byte limits. The Goal states the one limit on "exactly". |
| R12-10 | SD-8 is now a recorded narrowing of DESIGN-010 §8 for Sam, with the reasons, and says "a document type a person approves". FR-5.5 attributes the chat agent by the configured `mcp_actor`. |
| R12-11 | SD-3's justification corrected (the audit row keeps the reason; the transcript is where it is read). Scope item 5 cites FR-1.8. FR-5.2 uses the real `event` values. SD-11 and FR-3.6 say the old ledger is one list. |
| R12-12 | ACs added for the running page, the live line, a moment's state and "who", sorting, the window, and pruning's pace (hourly). The full-loop AC tolerates M3's moment. NFR-2 and NFR-3 say they are reported, not gated. |
| R12-13 | §6 tells M3 to take the runner change or use a fresh database, and says the schema version hides a gap; the handoff repeats it. |
| R12-14 | Serial commas added. The prose note now promises plain words, not sentence headings. FR-3.2 shows failures as a sentence by kind, with the engine's text beneath. "Sendback" is gone from the prose. FR-8.1 says how D-10 applies: cards with figures, not a table. |
| R12-15 | SD-7 and the build leave `dispatch.*` rows out of a phase's events. |
| R12-16 | FR-6.1: folding compares a fold label the table gives each foldable kind; a lone moment keeps its full label. `TestTimelineLine` checks both. |
| R12-17 | The status line and this section now agree. |
| §4 notes | Secrets: SD-13. Comment-to-run links and a "written by" link on documents: follow-ups in the handoff. Deleted tasks: FR-5.1. Batches: FR-1.6. `content_bytes`: FR-1.7. Pruned then retried: FR-2.4. Heartbeats: §6. |
