# SPEC-006 Entry Criteria — The command centre's mutation slice (planning & review actions)

**Status:** Draft — satisfies SPEC-004 DoD item 5
**Date:** 2026-07-22
**Parent:** [SPEC-004](../specs/SPEC-004-phase-4-web-command-centre.md) §2 (SD-1:
the first slice is read-everywhere + respond; the planning and document-review
**mutations** were deferred with a destination) and
[DESIGN-007](../design/DESIGN-007-web-command-centre.md) §8 (the operation
catalogue — the mutations already exist as gated, audited service methods).

> **Naming note (settled).** DESIGN-007 §9 already reserved SPEC-005 for the
> **MCP facet** (the read/poke surface for editor assistants). Rather than
> renumber an approved assignment, this mutation slice takes the next free
> number, **SPEC-006** ([SPEC-006](../specs/SPEC-006-command-centre-mutations.md)).
> Spec numbers are identifiers; build order is priority-driven, not numeric.

## Where this sits

SPEC-004 shipped the **operational** command centre: read every surface live and
answer escalations from the browser, with no CLI. That is the command centre's
defining loop (watch → get-pulled-in → answer → resume), fully delivered. What it
deliberately left out is every **planning and document-review mutation** — the
actions the engine and CLI already expose but the UI does not yet drive. This
slice adds those forms. It is a coherent second increment, not a capability gap:
all of it is CLI-driveable today.

## The deferred surface this slice covers (SPEC-004 §2, DESIGN-007 §8)

- **Planning mutations:** create initiative / feature; `estimate set` /
  `estimate ai`; milestone create / add member / remove member (descope) / lock
  (G4); roadmap create / add entry; feature start / abandon; initiative archive
  (G5 → checkpoint).
- **Document-review actions:** on a human-gated (escalated) review, approve /
  request changes — the same checkpoint-response path the inbox already uses,
  surfaced from the document view.

Everything above is an **existing gated, audited service operation** (each
already audit-in-transaction, O-3). The increment is the UI forms and their
`/ui/*` POST handlers, which call the same transactional service methods the
`/api/*` handlers do — no new authority, no gate override except through an
answered checkpoint (CC-4, L-6).

## This slice may start when

1. **SPEC-004 is done** — its FR acceptance criteria green in CI (mock provider)
   on plain Postgres and the Supabase local stack, and its live smoke confirmed
   by a human (SPEC-004 DoD 1–4). The operational slice is what these mutations
   hang off; it must be solid first.

2. **A spec exists and is approved**, scoped to the mutation surface above as a
   second rendering over the *existing* service methods (no new engine
   operation). It should decide, at least:
   - **Form design and validation** — each mutation's form, the fields it
     collects, and how a service-layer rejection (a failed gate, a validation
     error, a conflict) is surfaced inline. The engine already returns these;
     the UI renders them.
   - **Optimistic vs. confirmed feedback** — a mutation posts to `/ui/*`,
     returns the updated fragment, and the SSE hub refreshes affected regions;
     the spec should decide how much the acting page shows before the event
     round-trips (a tuning within SD-5's presentation-only realtime).
   - **The G4/G5 checkpoint paths from the UI** — milestone lock and initiative
     archive can raise checkpoints; the UI must route the block to the inbox (as
     the CLI does) rather than inventing an override, and reflect the raised
     checkpoint live.
   - **Document-review approve / request-changes** from the document view —
     which reuses the checkpoint-response path but is reached from a different
     surface than the inbox; the spec should fix that interaction.

3. **No new store reads are needed** — SPEC-004 added the aggregate reads a
   browser wants (CC-7). The mutation slice should confirm it needs none beyond
   those, or enumerate any additions (additive, no schema change) rather than
   discover them mid-build.

## Carry-overs to fold in or defer (from SPEC-004)

- **Realtime granularity** (SPEC-004 §6) — the operational slice refreshes live
  regions on a coarse `changed` signal. A mutation-heavy slice, where the acting
  user expects immediate targeted feedback, is the natural place to add per-kind
  fragment swaps if the coarse refresh proves too broad. A tuning, not a
  contract change.
- **Multi-user auth** (CC-6) — still a later spec. The mutation slice keeps
  acting as the single configured operator (`server.ui_actor`); the audit actor
  string makes real identities additive when auth lands. The mutation surface
  makes "who did this" matter more, which may raise auth's priority.
- **Config editing** (CC-5) — remains out of scope, possibly permanently; the UI
  shows config read-only. A mutation slice must not cross into writing
  `.cromwell/`.
- **The MCP facet** — the read/poke surface for editor assistants stays its own
  spec (a different rendering with narrower authority); the web UI drives
  mutations, the MCP facet does not.

## Not blocking

The engine and CLI drive every one of these mutations today; this slice is a
rendering increment, not a dependency of anything shipped. It starts when the
team chooses the UI mutation surface as the next increment and writes the spec.
