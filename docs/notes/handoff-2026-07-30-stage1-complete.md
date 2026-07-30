# Handoff — SPEC-009 Stage 1 is code-complete

**Date:** 2026-07-30 (second session)
**Supersedes for orientation:** `handoff-2026-07-30.md` (its "not built" list is
now empty; its traps and decisions still hold)

---

## Where the plan stands

Every functional requirement of SPEC-009 Stage 1 is built, tested and
committed. Five commits this session, on top of the two the previous session
left unpushed — **seven commits total are local-only and need pushing**:

| Commit | What it is |
|---|---|
| `ae60b2f`, `03d983f` | The previous session's two (schema, G0, roles, chain wiring) |
| `36f779b` | FR-2 — per-type approval authority; a human decides a design |
| `85bd2f5` | FR-9 — the revision cascade, the per-spec checkpoint, `contract_invalidated` |
| `4f2f15f` | FR-7 + FR-8 — the live document page, the fidelity bar |
| `0718edc` | C-1a — severity recorded on the document path |
| `1a25ef2` | FR-4.4/4.7 — FeatureCreated/FeatureDescribed and the heartbeat sweep |

`go vet ./...` and `CROMWELL_TEST_DATABASE_URL=… go test -race ./...` are
clean, run without cache on every package.

## What remains of the SPEC-009 DoD

Only the two live smokes, both needing Sam:

1. **DoD 2, the load-bearing claim:** approve a design and watch specs,
   dev-plans and tasks appear with no further human act, stopping at gate 2.
2. **DoD 3, the cascade:** revise the design, answer the design-revision
   checkpoint, watch a replacement spec written against the revision.

The smoke project is ready for both: `/tmp/cromwell-smoke` now has
`spec-author`, `dev-plan-author` and `design-reviewer` on `deepseek-chat`, the
`write-spec`/`write-dev-plan` assignments enabled, and the design template
installed. **The server on 127.0.0.1:8801 is still running a pre-FR-2 binary
and must be rebuilt and restarted first.** Also still open from before: the
SPEC-007 and SPEC-008 DoD live smokes await Sam's confirmation.

## Decisions taken in implementation that a reader should not undo

- **The two open questions in SPEC-009 §6 are resolved.** (1) Invalidation
  supersedes first and archives the files, then the reconciler authors fresh —
  for forming features. (2) A spec still in draft or reviewing is *not* in the
  cascade: it has no downstream derivations, and its pending review reads
  against the revised design, which is FR-8's correction mechanism.
- **`contract_invalidated` exists, ready → idea, engine-only.** A ready
  feature whose contract the cascade superseded is an idea again; without the
  transition it would sit in ready with no spec and nothing would ever
  re-author it. Its tasks are kept — decomposition reconciles by local id.
- **An in-flight feature's invalidation goes through the successor path**:
  `ReviseDoc` opens the draft, `write-spec` fills it (`fileAuthoredDocument`
  fills an existing current draft rather than registering a duplicate), and
  submitting fires the existing `MarkRevisionInFlight` unchanged.
- **The heartbeat sweep only touches never-attempted work.** A failed
  authoring dispatch has a dispatch-failure checkpoint governing its retry; a
  sweep that re-enqueued it every thirty seconds would override the human's
  answer.
- **A design reviewer's comments carry no severity, a human's reasons carry
  none either**; an agent *finding* stored without one is recorded at its
  effective reading, major — the same fail-closed rule enforcement uses.

## Defects found and fixed in passing, all latent before this session

- `queueAuthoring` never kicked the dispatcher: authored work sat queued until
  an unrelated event happened along.
- Supersession archived at the bare basename, so a *second* revision of any
  document failed its `git mv`; and short ids taken from a UUIDv7's head are
  timestamps, shared by rows created in the same instant. Archives now carry
  the id's tail (`ShortID`'s reason, rediscovered).
- Plain decomposition assumed no existing tasks and would have duplicated
  them on a re-formed feature; it now always reconciles.
- The review prompt attached ancestor-initiative documents only, so a design
  attached to the feature itself never reached the spec reviewer asked to
  check fidelity against it.
- `gitIn` discarded stderr; "exit status 128" alone diagnoses nothing.

## Stage 2 entry criteria (DoD 6)

Stage 2 — the surfaces (DESIGN-009 §11) — should start only when:

1. **Both Stage 1 live smokes are recorded** in a walkthrough with token
   costs. Stage 2 builds second routes to acts the first routes must first be
   seen performing.
2. **The DEC-005 amendment for approval relay is drafted and accepted** before
   the MCP approval tool is built (DESIGN-009 §5.1 names why: relaying a
   human's approval is a third permission category, not adjudicated planning).
   The amendment should record the `via: mcp` audit convention and the
   single-operator limitation until per-user identity lands.
3. **The opt-in default is revisited** with smoke evidence in hand: whether
   `write-spec`/`write-dev-plan` join the starter assignments uncommented,
   and what `init` says about them either way.
4. Scope, unchanged from DESIGN-009 §11: document-page Submit and Revise, MCP
   approve and submit tools, and the DEC-003 verbs those cover (`doc submit`,
   `doc revise` for human-written documents). The DEC-003 addendum (2026-07-30)
   records exactly what Stage 1 did and did not cover.

## Traps, inherited and new

- All of the previous handoff's traps stand, especially the test-database one:

      CROMWELL_TEST_DATABASE_URL="postgres://postgres:cromwell@localhost:54329/postgres" go test -race ./...

- The mock provider's steps are a global FIFO consumed by whatever dispatch
  runs next. When a test deliberately lets a dispatch fail (no scripted step),
  wait for the queue to quiet before scripting the next step, or the failing
  dispatch steals it. `TestDesignRevisionCascade` shows the pattern.
- The dispatcher's retry sweeps run on the heartbeat, which the integration
  harness does not start: a failed dispatch rests at `failed` after one
  attempt in tests, but retries in production.
