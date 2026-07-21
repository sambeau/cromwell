# SPEC-003: Phase 3 — Sizing, Calibration, and Milestones

**Status:** Approved and binding — 2026-07-21 ([REVIEW-003](../reviews/REVIEW-003-phase-3-planning-package.md))
**Date:** 2026-07-21
**Parent design:** DESIGN-001 §5–7 (estimates, milestones, calibration schema), DESIGN-003 §8 (gate G4); builds on DESIGN-002 (dispatch), DESIGN-004 (compartment)
**Vision:** [vision-v1](../vision/vision-v1.md) §4 (milestones, roadmaps), §5 (sizing and cost)
**Precondition:** [phase-3 entry criteria](../notes/phase-3-entry-criteria.md); phases 1 and 2 complete

## 0. Framing: the command centre, in slices

Phase 3 in the roadmap is "the command centre": web UI, MCP facet, milestones,
roadmaps, estimates and calibration, defects, checklists (README phase plan).
That is a large surface spanning a new front-end project, a new inbound
protocol, a planning-and-tracking engine, and two more work entities. A
veteran team would not build it as one undifferentiated push; it would slice
it so each increment is a complete, load-bearing capability (the discipline
phases 1 and 2 followed).

**This spec is the first phase-3 slice: the planning-and-tracking engine** —
sizing in tokens, confidence tiers, cost roll-ups, calibration from actuals,
milestones, and roadmaps. It is chosen first because:

- It is the vision's most distinctive intellectual claim (§5: tokens as the
  honest unit; §4: milestones that reflect reality without fudging) and the
  one part of the command centre not yet proven.
- It builds entirely on the **complete dispatch/cost ledger** phases 1–2
  already produce and on schema DESIGN-001 already defines (§6 milestones,
  §7 estimates) — high leverage, in the server's wheelhouse, no new stack.
- It is CLI-exposable and fully testable now; the surfaces that *render* it
  (web UI, MCP facet) then have real data to show and can each get the design
  attention they deserve in their own specs.

The **remaining command-centre pieces are deferred to their own specs**, each
named in §2. This slicing is itself the central scope decision for review — if
the web UI (the visible "command centre") should come first instead, that is a
one-line redirect here, and this spec becomes SPEC-004.

## 1. Goal

Prove that Cromwell **measures and plans honestly**. Concretely, the three
claims this slice must demonstrate:

1. **Sizing is honest** — work is sized in tokens (the real unit of AI
   computation, vision §5); every estimate carries a confidence tier assigned
   from its evidence; roll-ups propagate the *worst* tier in the tree, and
   unestimated work shows as `?`, listed explicitly. No estimate ever looks
   more confident than its weakest input.
2. **Actuals feed calibration** — completed work records actual token
   consumption from the ledger beside its estimate; the growing corpus of
   (description, estimate, actual) anchors future estimates, so rough numbers
   sharpen over time.
3. **Milestones reflect reality without fudging** — a milestone references its
   members live; locking snapshots the membership to a flat leaf set; progress
   against that frozen set keeps computing; and the only way to ship without
   something is to remove it before locking (vision §4). A locked milestone is
   the true record of what shipped, including what was descoped.

Phase 3 (this slice) is complete when a user can estimate features and tasks,
see honest tier-propagated roll-ups and cost at every level, group
deliverables into milestones that track live and lock honestly, order
milestones into roadmaps, and watch estimates calibrate against actuals as
work completes.

## 2. Scope

### In scope

- **Estimates** on features and tasks (DESIGN-001 §7): record a token
  estimate with a confidence tier, keep re-estimation history, the latest is
  current.
- **Confidence tiers** (vision §5) assigned from evidence: `decomposed` (sum
  of children — arithmetic), `considered` (single-unit estimate informed by
  the calibration corpus), `rough` (pure judgement).
- **Roll-ups**: parent totals over the entity tree, worst-tier propagation,
  `?` for unestimated work listed explicitly. CLI `cromwell estimate`.
- **AI-assisted estimation**: an `estimate` dispatch purpose and an
  `estimator` role that receives corpus reference points and returns a
  tokens number with rationale (vision §5), reusing the dispatch machinery.
- **Actuals and calibration**: actuals summed from the dispatch ledger by
  owning entity; the calibration corpus as the join (description, latest
  estimate, summed actuals); nearest-neighbour retrieval to inform the
  `considered` tier (retrieval mechanism per SD-1).
- **Cost roll-ups extended** (vision §5): cost per task/feature already
  exists (phase 1); extend to per-initiative (transitive), per-milestone,
  per-roadmap, and per-month.
- **Milestones** (DESIGN-001 §6, vision §4): create; live membership over
  initiatives (transitive), features, and nested milestones; computed
  progress; lock with snapshot to a flat leaf set; gate **G4**
  (milestone-lockable); descope-before-lock.
- **Roadmaps**: ordered sequences of milestones (the order is the planner's
  to mean; the system preserves it).
- **Migration `0004`**: the `estimates`, `milestones`, `milestone_members`,
  `milestone_snapshots`, `roadmaps`, `roadmap_entries` tables (DESIGN-001
  defines them; phase 1–2 shipped none).
- **CLI**: `estimate`, `milestone`, `roadmap`, `cost` (extended).

### Out of scope (deferred, with destination)

| Deferred | To |
|---|---|
| **Web UI** (dashboards, inbox, document review, planning surfaces) | SPEC-004 — its own front-end project consuming the HTTP API (DEC-001) |
| **MCP facet** (read/poke for editor assistants) | SPEC-005 |
| **Defects** (triage pipeline reusing task machinery) | A later spec (DESIGN-003 §9 sketch; vision §14) |
| **Jobs and checklists** (human task lists) | A later spec; until then milestones do not take checklist members (SD-2) |
| **Embeddings / semantic retrieval; section role classification** | Still deferred (SD-4 from phase 2); the `considered` tier uses non-embedding retrieval here (SD-1) |
| Human-hours tracking (the optional orthogonal tag, vision §5) | Later; sizing here measures AI execution only |

### Scope decisions

- **SD-1: The `considered` tier retrieves without embeddings.** Vision §5's
  calibration retrieves nearest neighbours by description-embedding
  similarity, but embeddings are deferred (phase-2 SD-4). In this slice the
  corpus is retrieved by Postgres full-text search over stored entity
  descriptions (features/tasks carry a `description`; they have no tags, so
  tag overlap is not part of entity retrieval — it is a documents concern) —
  no new embedding infrastructure. The corpus (description, estimate, actual)
  is recorded in full regardless, so embedding-based retrieval is a later
  *retrieval-mechanism* swap, not a data migration. All three tiers are real
  in this slice; embeddings sharpen the `considered` tier later.
- **SD-2: Milestone members are initiatives, features, and nested milestones
  only.** Checklists (a valid member type per vision §4) are deferred with
  jobs/checklists; the schema's `milestone_members.member_type` already
  admits them, so adding checklist membership later is additive.
- **SD-3: The command centre's surfaces are separate specs.** The web UI
  (SPEC-004) and MCP facet (SPEC-005) are not in this slice; this slice
  delivers the engine they render, exposed via the CLI and HTTP API.
- **SD-4: One provider, one more role.** Anthropic-compatible endpoints only
  (inherits SPEC-001 D-2). The `estimator` role is a new dispatch role bound
  via `assignments:` (DESIGN-004 §5).

## 3. Requirements

### FR-1: Estimates and confidence tiers

- **FR-1.1** An estimate records a token count, a confidence tier
  (`decomposed` | `considered` | `rough`), a rationale, and (when AI-made)
  the estimating dispatch; multiple estimates per entity are allowed and the
  latest is current (DESIGN-001 §7).
  *AC:* estimating a feature twice keeps both rows; `cromwell estimate <ref>`
  shows the latest as current and the prior as history.
- **FR-1.2** Tier assignment follows the evidence, not the caller's say-so:
  an estimate that is the sum of a feature's task estimates is `decomposed`;
  a single-unit estimate that consulted the corpus is `considered`; a
  single-unit estimate with no corpus evidence is `rough`.
  *AC:* a feature whose tasks are all estimated rolls up to a `decomposed`
  feature estimate by arithmetic; a feature estimated as a unit with corpus
  neighbours is `considered`; with none, `rough`.
- **FR-1.3** `cromwell estimate set <ref> <tokens> [--rationale]` records a
  human estimate (tier `rough`, or `considered` if it cites corpus rows);
  `cromwell estimate ai <ref>` dispatches the estimator (FR-4).
  *AC:* both paths produce an estimate row with the correct tier.

### FR-2: Roll-ups with worst-tier propagation

- **FR-2.1** A parent's estimate is the sum of its descendants' current
  estimates; its tier is the **worst tier in the subtree** (`rough` beats
  `considered` beats `decomposed`); a single unestimated descendant makes the
  parent's total a `?` for the unknown part, and the unestimated leaves are
  listed explicitly (vision §5).
  *AC:* an initiative with two estimated features (one `considered`, one
  `rough`) and one unestimated feature rolls up to a `rough` total plus a `?`,
  and names the unestimated feature.
- **FR-2.2** `cromwell estimate [--ref <entity>]` renders the roll-up: total
  tokens, tier, and the list of unestimated descendants.
  *AC:* the command output matches the computed roll-up for a seeded tree.

### FR-3: Actuals and calibration

- **FR-3.1** Actual token consumption per entity is summed from the dispatch
  ledger by owner and by descendant ownership (a feature's actual includes
  its tasks' dispatches); on completion the (description, latest estimate,
  summed actuals) tuple is available as the calibration corpus (DESIGN-001
  §7 — a view, not a stored table).
  *AC:* after a feature completes, `cromwell estimate <feature>` shows
  estimate vs actual, and the delta.
- **FR-3.2** The corpus is retrievable by similarity for the `considered`
  tier: given a description, return the nearest prior (description, estimate,
  actual) rows by full-text rank over entity descriptions (SD-1).
  *AC:* estimating a new feature whose description shares terms with a
  completed one retrieves that completed one as a reference point.

### FR-4: AI-assisted estimation

- **FR-4.1** An `estimate` dispatch purpose runs the `estimator` role
  (`assignments:` bound) with the entity's description and the retrieved
  corpus reference points; its outcome tool `submit_estimate(tokens,
  rationale)` records a `considered` estimate (or `rough` when the corpus was
  empty). Cost is ledgered like any dispatch.
  *AC:* `cromwell estimate ai <feature>` produces a `considered` estimate
  citing the corpus neighbours in its rationale, with a costed dispatch row.
- **FR-4.2** The estimator starter-pack role and `estimate-work` skill ship
  in the pack (authored and reviewed like code, as prior roles were).
  *AC:* a fresh `init` yields the role, skill, and `assignments: estimate:
  estimator`.

### FR-5: Milestones

- **FR-5.1** `cromwell milestone create <name> [--target-date]`; membership
  is added/removed live over initiatives (transitive to descendant features),
  features, and nested milestones (SD-2); progress is computed from the
  resolved leaf set.
  *AC:* adding an initiative includes its descendant features in the
  milestone automatically; adding a feature later under that initiative
  appears in the milestone without re-adding.
- **FR-5.2** Gate **G4** (milestone-lockable, DESIGN-003 §8): a milestone can
  lock only when at least one resolved member is `done`. Locking snapshots the
  live membership to a flat leaf set (`milestone_snapshots`); progress against
  a locked milestone reads the snapshot and each leaf's current state.
  *AC:* locking a milestone with no done member is refused with the G4 reason;
  after a member finishes, locking snapshots the leaves; a member finishing
  *after* lock still advances the historical progress but the promised set is
  fixed.
- **FR-5.3** No fudging: the only way to lock without a member is to remove it
  first; the removal (with its reason) is on the milestone's audit trail
  (vision §4).
  *AC:* removing a member before lock and locking records the descope in the
  audit log; the locked snapshot excludes it.

### FR-6: Roadmaps

- **FR-6.1** `cromwell roadmap create <name>`; `roadmap add <roadmap>
  <milestone> [--position]` orders milestones; the system preserves the order
  and makes no claim about its meaning (vision §4).
  *AC:* milestones appear in the roadmap in the given order; reordering
  persists.

### FR-7: Cost roll-ups extended

- **FR-7.1** `cromwell cost [--ref]` rolls cost up per initiative
  (transitive over descendant features/tasks), per milestone (over its
  resolved members), per roadmap, and per calendar month, from the existing
  dispatch ledger with its frozen price snapshots (phase 1 O-4).
  *AC:* a seeded tree of dispatches produces correct per-initiative and
  per-month totals; a milestone's cost is the sum over its resolved members.

## 4. Non-functional requirements

- **NFR-1** Roll-up, tier-propagation, milestone-resolution, and locking
  logic is pure and unit-tested without a provider or Postgres where
  practical (as the lifecycle/gate logic is); the estimator dispatch is
  integration-tested with the mock provider.
- **NFR-2** `go vet ./...` and `go test -race ./...` clean; CI runs the suite
  against plain Postgres and the Supabase local stack (inherited).
- **NFR-3** Migration `0004` creates only the phase-3 tables; forward-only
  (DESIGN-001 §11); no changes to phase 1–2 tables.
- **NFR-4** No secrets in git, `.cromwell/`, the audit log, or error messages
  (inherited). Estimates and milestones contain no secret material.
- **NFR-5** Locking a milestone is atomic: the snapshot is written and the
  state flips to `locked` in one transaction (DESIGN-001 §2.3 append-only
  snapshot; O-3 audit-in-transaction).

## 5. Definition of Done

1. All FR acceptance criteria pass in CI (mock provider) on plain Postgres
   and the Supabase local stack.
2. A live smoke test extends the record: estimate a feature (AI-assisted,
   citing the corpus), complete work, see the actual land beside the estimate
   and the tier roll up honestly; group deliverables into a milestone, lock it
   after one ships, and read the honest snapshot — against a live provider,
   with the ledger and calibration inspected by a human. Recorded in a
   walkthrough.
3. `go vet ./...` and `go test -race ./...` clean.
4. The walkthrough records the session.
5. Entry criteria for the next command-centre slice (web UI, SPEC-004)
   drafted.

## 6. Open questions carried into implementation

- **Estimator skill quality** drives whether `considered` estimates are worth
  more than `rough` ones; authored during implementation and reviewed like
  code (as prior roles were).
- **Corpus retrieval ranking** (FT-rank vs tag-overlap weighting, SD-1) is an
  implementation detail to tune against real data; the outcome is
  reference-point quality, not a contract.
- **Whether a companion DESIGN-007** (planning-layer design: roll-up
  algorithm, milestone resolution, calibration) should precede
  implementation, matching phases 1–2's design-then-spec hierarchy. The
  schema already exists in DESIGN-001 §6–7 and G4 in DESIGN-003 §8, so this
  spec is self-sufficient to review; a DESIGN-007 would deepen the algorithmic
  detail if the reviewer wants it before build.
