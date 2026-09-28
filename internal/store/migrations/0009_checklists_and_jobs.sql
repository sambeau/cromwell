-- 0009_checklists_and_jobs.sql — checklists and jobs (SPEC-014 FR-1.1,
-- DESIGN-010 §4 and §6). Forward-only and additive (DESIGN-001 §11). Version
-- 0010 belongs to a parallel line of work (M8); nothing here depends on it.
--
-- A job is one thing only a person can do, ticked by hand. A checklist is a
-- list of jobs, done when it has at least one job and every job is ticked
-- (SD-9). The words were reserved in ref_type by 0001; SPEC-003 SD-2 deferred
-- the tables until now.

-- --- Checklists ---
--
-- Owned like milestones and roadmaps (0005, DESIGN-010 §17a item 4): the
-- project with a null owner_id, or an initiative. The same pair of CHECKs, so
-- the invariant reads identically across the schema.

CREATE TABLE checklists (
  id          uuid PRIMARY KEY,
  name        text NOT NULL,
  description text NOT NULL DEFAULT '',
  owner_type  ref_type NOT NULL DEFAULT 'project',
  owner_id    uuid REFERENCES initiatives(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT checklists_name     CHECK (btrim(name) <> ''),
  CONSTRAINT checklists_owner_kind CHECK (owner_type IN ('project', 'initiative')),
  CONSTRAINT checklists_owner_id   CHECK ((owner_type = 'project') = (owner_id IS NULL))
);
CREATE INDEX checklists_owner ON checklists (owner_type, owner_id);

-- --- Jobs ---
--
-- A title, an optional note, a place in the checklist's order (dense, 1…n),
-- and who ticked it and when (roadmap decision 8). ticked_via says which
-- surface the tick came through, and ticked_quote holds the person's words
-- when the chat agent relayed it (DEC-006 Amendment 1, SPEC-014 SD-8). All
-- four tick columns are null while the job is unticked.

CREATE TABLE jobs (
  id           uuid PRIMARY KEY,
  checklist_id uuid NOT NULL REFERENCES checklists(id),
  title        text NOT NULL,
  note         text NOT NULL DEFAULT '',
  position     integer NOT NULL,
  ticked_by    text,
  ticked_at    timestamptz,
  ticked_via   text,
  ticked_quote text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT jobs_title  CHECK (btrim(title) <> ''),
  CONSTRAINT jobs_ticked CHECK ((ticked_by IS NULL) = (ticked_at IS NULL)),
  CONSTRAINT jobs_via    CHECK (ticked_via IS NULL OR ticked_via IN ('ui', 'mcp')),
  -- how and in whose words only exist for a tick
  CONSTRAINT jobs_tick_detail CHECK (ticked_at IS NOT NULL OR (ticked_via IS NULL AND ticked_quote IS NULL)),
  -- a tick through the chat agent always carries the person's words (DEC-006 Amendment 1)
  CONSTRAINT jobs_relay_quote CHECK (ticked_via IS DISTINCT FROM 'mcp' OR ticked_quote IS NOT NULL)
);
CREATE INDEX jobs_order ON jobs (checklist_id, position);

-- --- A checklist as a milestone deliverable (DESIGN-010 §6) ---
--
-- It joins milestone_members as a fourth member type, and milestone_snapshots
-- as a second leaf type, so marking a milestone as shipped records its
-- checklists beside its features (SPEC-014 FR-2.5).

ALTER TABLE milestone_members DROP CONSTRAINT milestone_members_member_type_check;
ALTER TABLE milestone_members ADD CONSTRAINT milestone_members_member_type_check
  CHECK (member_type IN ('initiative', 'feature', 'milestone', 'checklist'));

ALTER TABLE milestone_snapshots DROP CONSTRAINT milestone_snapshots_leaf_type_check;
ALTER TABLE milestone_snapshots ADD CONSTRAINT milestone_snapshots_leaf_type_check
  CHECK (leaf_type IN ('feature', 'checklist'));

-- A feature's done is terminal, so a shipped milestone's record over features
-- can only move forward. A checklist's isn't: a job can be unticked or added
-- later. So the snapshot records whether each checklist was done when the
-- milestone was marked as shipped, and the shipped record counts it done if it
-- was done then or is done now (SPEC-014 SD-4, REVIEW-014 R14-1). Features
-- leave it null.
ALTER TABLE milestone_snapshots ADD COLUMN done_at_lock boolean;
ALTER TABLE milestone_snapshots ADD CONSTRAINT milestone_snapshots_done_at_lock
  CHECK ((leaf_type = 'checklist') = (done_at_lock IS NOT NULL));
