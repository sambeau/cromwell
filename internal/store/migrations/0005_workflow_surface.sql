-- 0005_workflow_surface.sql — the workflow surface, Stage A (SPEC-007 FR-10.1).
-- Forward-only and additive (DESIGN-001 §11): no phase 1–3 column changes
-- meaning. This migration gives milestones and roadmaps an owner (so an
-- initiative can own its own local planning, DESIGN-008 D-9) and marks one
-- document per owner as its main design document (DESIGN-008 §5.2, Q-B).
--
-- Existing rows default to project-owned, matching the documents ownership
-- pattern from 0001 (owner_type + nullable owner_id; project = null id).

-- --- Milestone and roadmap ownership (DESIGN-008 §5.1a, D-9) ---
--
-- Owner is the project (owner_id NULL) or an initiative. The same two-column
-- shape and the same pair of CHECKs the documents table uses, so the invariant
-- reads identically across the schema. owner_id references initiatives because,
-- unlike documents, the only non-project owner here is an initiative — a real
-- foreign key rather than a polymorphic id. Existing milestones and roadmaps
-- become project-owned via the column default.

ALTER TABLE milestones
  ADD COLUMN owner_type ref_type NOT NULL DEFAULT 'project',
  ADD COLUMN owner_id   uuid REFERENCES initiatives(id),
  ADD CONSTRAINT milestones_owner_kind CHECK (owner_type IN ('project', 'initiative')),
  ADD CONSTRAINT milestones_owner_id   CHECK ((owner_type = 'project') = (owner_id IS NULL));

ALTER TABLE roadmaps
  ADD COLUMN owner_type ref_type NOT NULL DEFAULT 'project',
  ADD COLUMN owner_id   uuid REFERENCES initiatives(id),
  ADD CONSTRAINT roadmaps_owner_kind CHECK (owner_type IN ('project', 'initiative')),
  ADD CONSTRAINT roadmaps_owner_id   CHECK ((owner_type = 'project') = (owner_id IS NULL));

-- --- One entity per readable path (SPEC-007 FR-1.1) ---
--
-- The workflow surface addresses every entity by its readable slug path, so a
-- path must resolve to exactly one thing. The 0001 constraint UNIQUE (parent_id,
-- slug) does not give that at the root: SQL treats NULLs as distinct, so any
-- number of top-level initiatives could share a slug, leaving /ui/i/auth
-- ambiguous and InitiativeBySlugPath returning whichever row came first.
--
-- This partial unique index closes the gap for parent-less initiatives; nested
-- ones are already covered by the original constraint. If a project has somehow
-- accumulated duplicate top-level slugs, this migration fails and names them —
-- which is the honest outcome, since those rows cannot both be addressable and
-- one must be renamed before the surface can work.

CREATE UNIQUE INDEX initiatives_root_slug ON initiatives (slug)
  WHERE parent_id IS NULL;

-- --- The main-document mark (SPEC-007 FR-3.1, DESIGN-008 Q-B) ---
--
-- An entity's page body is its main design document: the one marked primary,
-- or — when none is marked — the first attached design document (resolved in
-- the read layer, not here). At most one live document per owner may carry the
-- mark; the partial unique index enforces it, coalescing the nullable
-- project-level owner_id to a fixed sentinel so two project-level primaries
-- collide too. Existing rows start unmarked; selection falls back to the first
-- design document until an operator or the chat agent marks one.

ALTER TABLE documents
  ADD COLUMN is_primary boolean NOT NULL DEFAULT false;

CREATE UNIQUE INDEX documents_primary_per_owner
  ON documents (owner_type, COALESCE(owner_id, '00000000-0000-0000-0000-000000000000'::uuid))
  WHERE is_primary AND state <> 'superseded';
