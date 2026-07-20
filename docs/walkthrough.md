# Phase-1 Live Smoke Test — Session Record

**Status:** Complete — satisfies SPEC-001 Definition of Done items 2 and 4
**Date:** 2026-07-20
**Operator:** Sam (commands run by Claude on Sam's behalf)
**Database:** Supabase-hosted Postgres, project `mxxxleqbisaphlqvhjdg`,
session pooler at `aws-0-eu-central-1.pooler.supabase.com:5432`
**Provider:** DeepSeek via its Anthropic-compatible gateway
(`https://api.deepseek.com/anthropic`), model `deepseek-v4-flash`
**Binary:** commit `881a25a` plus the smoke-test fixes in this commit

This is the real thing: a real Supabase database, a real provider API, real
money (a tenth of a cent). Every command and its actual output is recorded
below, in the order it was run.

## 1. What the test proves

SPEC-001 §1's three load-bearing claims, end to end:

1. **A code orchestrator dispatched an agent-reviewer directly** — no chat
   intermediary anywhere in the path.
2. **The human gate fired only where the design says it should** — the spec
   went `draft → reviewing → approved` and the feature reached `ready` with
   no human involvement; the one checkpoint raised was an integrity
   violation the human caused deliberately.
3. **The ledger is complete by construction** — 14 audit rows covering
   every transition, gate, and dispatch state change, and a cost figure
   that reconciles to the token counts exactly.

## 2. Install

```
$ git init && git commit -m initial          # a fresh project repo
$ cromwell init
initialised .cromwell/ — set ANTHROPIC_API_KEY, then run `cromwell serve`
```

`init` created `.cromwell/` (config.yaml, pack.lock.yaml, the
`spec-reviewer` role, the `review-spec` skill, the spec template and
manifest), installed the git post-commit hook, and applied migration 0001
to the Supabase database — which was empty beforehand (verified) and had
10 tables afterwards.

`config.yaml` was then edited to point at DeepSeek (this is the
user-owned, never-pack-tracked file — DESIGN-004 F-7):

```yaml
providers:
  deepseek:
    api_key_env: DEEPSEEK_API_KEY
    base_url: https://api.deepseek.com/anthropic
    rate:
      requests_per_minute: 30
models:
  deepseek-v4-flash:
    provider: deepseek
    price_per_mtok:      # platform.deepseek pricing, 2026-07-20
      input: 0.14
      output: 0.28
      cache_read: 0.0028
      cache_write: 0.14
```

**The compartment's cross-reference checking earned its place immediately.**
The first `serve` refused to start:

```
error: configuration invalid; refusing to start:
roles/spec-reviewer.yaml: model: unknown model "claude-sonnet-5" (not in config.yaml models)
```

The shipped role referenced a model no longer in the edited registry. The
message named the file, the field, and the reason — DESIGN-004 §9's
contract, working on its first real encounter with a mistake. After
pointing the role at `deepseek-v4-flash`:

```
$ cromwell serve &
$ cromwell status
{
  "config": "ok",
  "pending_checkpoints": 0,
  "queued": null,
  "running": 0,
  "schema_version": 1
}
```

## 3. Planning: initiative, feature, spec

```
$ cromwell initiative add auth --name "Authentication"
initiative auth created

$ cromwell feature add auth/login --name "Login form"
feature auth/login created (idea)

$ cp .cromwell/templates/spec/template.md docs/specs/login.md
$ git add -A && git commit -m "spec from template"
$ cromwell doc add docs/specs/login.md --type spec --owner auth/login
registered docs/specs/login.md as spec (draft)
```

Validating the untouched template — the honest first thing an author does:

```
$ cromwell validate docs/specs/login.md
invalid:
  [placeholders] front-matter field "title" contains an unresolved {{...}} placeholder
  [placeholders] front-matter field "owner" contains an unresolved {{...}} placeholder
  [placeholders] section "Overview" contains an unresolved {{...}} placeholder
  [placeholders] section "Behaviour" contains an unresolved {{...}} placeholder
  [placeholders] section "Acceptance criteria" contains an unresolved {{...}} placeholder
  [placeholders] section "Out of scope" contains an unresolved {{...}} placeholder
```

The validation floor caught every placeholder, including the two in front
matter (a gap found and fixed during CLI testing earlier the same day).

A real spec was then written into the file: a login form with session
behaviour, an identical-error-message requirement, a five-attempt lockout,
and six acceptance criteria. See the "Behaviour" and "Acceptance criteria"
sections quoted in §4 below for what the reviewer actually judged.

```
$ cromwell validate docs/specs/login.md
valid

$ cromwell submit docs/specs/login.md
submitted: reviewing (agent review queued)
```

## 4. The live review

The dispatch ran for **27.8 seconds** against DeepSeek. No human touched
anything during it. The reviewer completed via its `submit_review` outcome
tool; the structured payload recorded in the ledger:

```json
{
  "verdict": "approve",
  "comments": [
    {
      "section_ref": "Behaviour",
      "body": "Minor: network errors (timeout, server 5xx) during submission are not mentioned. If the project has a generic error-handling mechanism this is fine; if not, a short note would prevent guessing. Not a blocker."
    }
  ],
  "reasoning": "Clear, complete within scope, fully testable, and internally consistent. No defects that require changes. The network-error path is a minor gap but is a cross-cutting concern typically handled generically, not a failure of the login-form specification."
}
```

This is worth dwelling on, because it is the behaviour the starter-pack
skill was written to produce and the hardest thing to get right. The
reviewer found a genuine gap (unspecified network-error handling), judged
it a cross-cutting concern rather than a defect in this spec, attached it
as a comment, and **approved anyway** — exactly the "minor wording
preferences are not grounds to withhold approval" discipline in
`review-spec/SKILL.md`. It neither rubber-stamped nor escalated a call it
was equipped to make.

On approval the orchestrator evaluated G1 and advanced the feature with no
further input:

```
$ cromwell log --limit 12
... dispatch.succeeded  document  orchestrator  {"cost_usd":0.0009181200000000001,"input_tokens":1674,"output_tokens":2442,...}
... document.transition document  spec-reviewer {"from":"reviewing","to":"approved","event":"approve"}
... gate.evaluated      feature   orchestrator  {"gate":"G1","pass":true,"reason":"current spec approved"}
... feature.transition  feature   orchestrator  {"from":"idea","to":"ready","event":"contract_approved"}
```

## 5. The complete audit trail

All 14 rows from the Supabase `audit_events` table, in order. There are no
gaps: every state change in the session is here, each written in the same
transaction as the change it records (DESIGN-002 O-3).

| Time | Actor | Kind | Ref |
|---|---|---|---|
| 16:36:57 | sambeau | initiative.created | initiative |
| 16:36:58 | sambeau | feature.created | feature |
| 16:36:59 | sambeau | document.registered | document |
| 16:37:17 | orchestrator | document.indexed | document |
| 16:37:17 | sambeau | document.validated | document |
| 16:37:17 | sambeau | document.transition | document (draft → reviewing) |
| 16:37:18 | orchestrator | dispatch.queued | document |
| 16:37:19 | orchestrator | dispatch.running | document |
| 16:37:47 | orchestrator | dispatch.succeeded | document |
| 16:37:48 | spec-reviewer | document.transition | document (reviewing → approved) |
| 16:37:48 | orchestrator | gate.evaluated | feature (G1, pass) |
| 16:37:48 | orchestrator | feature.transition | feature (idea → ready) |
| 16:38:56 | orchestrator | checkpoint.created | document (integrity, §7) |
| 16:39:23 | sambeau | checkpoint.responded | document |

Note the actor column: the human authored and submitted, the orchestrator
dispatched and gated, and the **reviewer role itself is the actor on the
approval transition**. Who decided what is legible without reading code.

## 6. Cost

```
$ cromwell cost
document   019f8063-4ee2-7683-8bcc-92e902a32a63  dispatches=1  tokens=4116  $0.0009
total: $0.0009
```

The ledger row, with the price snapshot frozen at dispatch claim (O-4):

| Field | Value |
|---|---|
| purpose / role / model | `review-spec` / `spec-reviewer` / `deepseek-v4-flash` |
| attempt | 1 |
| input / output tokens | 1674 / 2442 |
| cache read / write | 0 / 0 |
| price_snapshot | `{"input":0.14,"output":0.28,"cache_read":0.0028,"cache_write":0.14}` |
| cost_usd | 0.000918 |
| wall time | 27.8 s |

Reconciliation, by hand: 1674 × $0.14/M + 2442 × $0.28/M = $0.00023436 +
$0.00068376 = **$0.00091812**, matching the recorded 0.000918 to the
stored precision. FR-7.2's acceptance criterion — cost matching snapshot
prices × reported tokens — holds against a real provider response.

The whole session cost **less than a tenth of a cent**.

## 7. The integrity violation

The last exercise was deliberate dishonesty: editing an approved document's
file directly and committing it, which the design forbids (DESIGN-003 §2,
L-2).

```
$ sed -i '' 's/fifteen minutes/five minutes/g' docs/specs/login.md
$ git commit -am "sneaky edit to an approved spec"

$ cromwell inbox
019f8065-1763-7382-bd85-73956d4eaa43  [document-integrity]
  Approved document docs/specs/login.md was modified on disk. Revert the file or create a revision.
  context: {"commit":"08de06bb1307cad346873c15a6ff2906155c95ba"}
  since: 2026-07-20T17:38:56.613025+01:00
```

The installed post-commit hook, the watcher's diff, the drift check against
`content_hash`, and the rule engine's refusal to re-index drifted approved
content all fired in sequence, against a real commit, with the offending
hash in the checkpoint context (FR-4.3). The honest path was then taken:

```
$ git revert --no-edit HEAD
$ cromwell respond 019f8065-... reverted --reason "tamper reverted in git; content restored to approved hash"
responded; the orchestrator resumes

$ cromwell inbox
inbox empty
```

## 8. Search

```
$ cromwell search "lockout"
[{ "Path": "docs/specs/login.md", "Heading": "Acceptance criteria",
   "Snippet": "<b>lockout</b> window is rejected with", ... }]
```

Postgres FTS over the indexed sections, no embeddings needed for the slice
(SPEC-001 §2 defers those to phase 2).

## 9. Defects found by this session

Three, all fixed in the same commit as this document:

1. **Stale document titles.** A document registered from the template kept
   its `{{feature name}}` placeholder title in the database even after the
   file gained a real one — visible in the search output. Indexing now
   refreshes the title from front matter on every pass.
2. **`dispatch.succeeded` audit rows omitted the outcome.** Answering "what
   did the reviewer actually say?" required a ledger query; the structured
   outcome now rides along in the audit payload, so `cromwell log` answers
   it directly.
3. **Shutdown could race an in-flight action** (found by the test suite
   while preparing this run, fixed in `881a25a`): the orchestrator may be
   running a server-authored git commit when the context is cancelled.
   `Run` now drains its loops before returning.

None of these are design faults — the designs were silent on all three,
which is what a smoke test is for.

## 10. Operational notes for the next operator

- **Supabase direct connections are IPv6-only.** `db.<ref>.supabase.co`
  will fail with "no route to host" on IPv4-only networks. Use the
  **session pooler** (`aws-0-<region>.pooler.supabase.com:5432`) — session
  mode, not transaction mode, because pgx uses prepared statements.
- **Passwords with reserved characters must be URL-encoded** in the
  connection string.
- **Unix socket paths are capped at ~104 bytes on macOS.** A deep project
  path plus `.cromwell/run/cromwell.sock` can exceed it; set
  `server.socket` to a short absolute path if `serve` fails to bind.
- Secrets stayed in the environment throughout (`CROMWELL_DATABASE_URL`,
  `DEEPSEEK_API_KEY`); nothing secret was written to `.cromwell/` or git
  (NFR-4).

## 11. Definition of Done status

| # | Criterion | Status |
|---|---|---|
| 1 | All FR acceptance criteria pass in CI (mock provider) | ✅ `go test -race ./...` green; plain Postgres 16 |
| 1b | …and in the Supabase local stack | ⬜ Outstanding — CI runs against plain Postgres; this session exercised Supabase-hosted Postgres manually |
| 2 | Live smoke test green against a Supabase-hosted project, audit trail and cost inspected by a human | ✅ This document |
| 3 | `go vet ./...` and `go test -race ./...` clean | ✅ |
| 4 | `docs/walkthrough.md` records the session | ✅ This document |
| 5 | Phase-2 entry criteria drafted | ✅ [phase-2-entry-criteria.md](notes/phase-2-entry-criteria.md) |

One item remains: running the automated suite against the Supabase local
stack (`supabase start`) in addition to plain Postgres, per FR-1.3. The
suite is host-agnostic by construction — it takes a connection URL — so
this is a CI configuration task, not code work.
