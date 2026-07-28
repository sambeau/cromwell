# Entry criteria — the workflow surface, Stage B

**Written:** 2026-07-26, on finishing Stage A (SPEC-007).
**Purpose:** what a Stage B planning package has to settle before any code, and
what it can safely assume is already there.

Stage B is the *editing* half of the workflow surface: composing milestones and
roadmaps, the size roll-up tree, the filterable work list, and task detail
(DESIGN-008 §9). Stage A deliberately owns and displays those objects without
letting anyone change them (SD-1) — this note is the handover.

## What Stage A already gives you

- **The owner columns exist and are populated.** Migration 0005 put
  `owner_type` + nullable `owner_id` on `milestones` and `roadmaps`, with every
  pre-existing row defaulting to project-owned. `MilestonesOwnedBy`,
  `RoadmapsOwnedBy` and `MilestonesForMember` read them.
- **The two-list display is built.** A milestone renders as an unordered
  checklist with both the ticked count and a token bar; a roadmap as an ordered
  list (`milestone-card` and the milestone/roadmap pages). Stage B changes how
  those lists are *edited*, not how they look (D-10).
- **The page anatomy and the rail.** Adding an action to an entity is now a
  matter of adding one form to `entity-rail` and one handler in
  `ui_entity_actions.go` that calls an existing service method; the entity is
  carried by its id, never typed.
- **The service methods for every Stage B mutation already exist and are
  audited**: `CreateMilestone`, `AddMember`, `RemoveMember`, `LockMilestone`
  (gate G4, no force path), `CreateRoadmap`, `SetRoadmapEntry`. SPEC-006 built
  and tested them; Stage A merely stopped exposing them. Nothing new is needed
  in the store except owner-scoped creation (below).

## What Stage B has to decide before it writes code

1. **Owner-scoped creation.** `CreateMilestone` and `CreateRoadmap` currently
   hard-code project ownership, because Stage A never creates one from an
   initiative page. Stage B must extend both to take an owner — a small,
   additive signature change — and decide whether the CLI's `milestone create`
   keeps defaulting to the project (it should, until DEC-003 retires it).
2. **The modal.** DESIGN-008 §5.1a specifies a native `<dialog>` loaded by HTMX
   from the parent entity's page. Settle the fragment routes (one per editable
   object?), how the dialog closes on success, and what it swaps back — the
   existing pattern re-renders the whole entity page, which is simple and may be
   enough.
3. **The member picker's two ends** (D-12). Membership is unconstrained by
   ownership, so members are added from *both* sides: an "Add to a milestone…"
   action on any entity (a short list of milestones), and a milestone-side picker
   defaulting to the owner's subtree with a project-wide search box. The
   project-wide search needs a read that does not exist yet — decide whether it
   reuses the FTS index or is a plain slug/name query.
4. **Checklists and jobs.** SD-2 in the schema left them unbuilt; Sam's round-5
   call was that they arrive in Stage B as human-ticked checkbox deliverables.
   That needs a table and a migration, and it is the largest single piece of
   Stage B — consider whether it is its own slice.
5. **The roll-up tree and the work list.** Both are read surfaces over reads that
   already exist (`InitiativeSizingNode`, `sizing.RollUp`). The open question is
   presentation: DESIGN-008 D-10 warns explicitly against dressing structure up
   into dense tables, which is what the first build got wrong.

## What must stay true

- **No typed entity path** (SD-6, NFR-8). The picker is a picker, not a text box.
  `TestUIBrowseAndDrive` enforces this and Stage B must keep it passing.
- **No money on the surface** (D-4). `TestRenderedTemplatesCarryNoCurrency`
  enforces it; there is no currency helper left to reach for.
- **No new authority** (NFR-3). G4 stays a blocking inline error with no
  checkpoint; G5 keeps raising one. No force flags (L-6).
- **Human-facing prose** (D-6) in every label, empty state and gate reason.

## Precondition

Stage A merged and green, which it is. Stage B does not depend on the MCP facet,
though the two remain peers on the planning half.
