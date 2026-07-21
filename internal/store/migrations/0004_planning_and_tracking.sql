-- 0004_planning_and_tracking.sql — phase-3 schema (DESIGN-001 §6–7, SPEC-003 §2).
-- Adds only what phase 3 uses; forward-only (DESIGN-001 §11). No changes to
-- phase 1–2 tables (SPEC-003 NFR-3): corpus full-text search runs over
-- features.description / tasks.description at query time (SD-1, REVIEW-003 §4),
-- so features and tasks are left untouched here.
--
-- estimate_tier and milestone_state are defined in DESIGN-001 §3 but were not
-- shipped by 0001–0003 (only the enums in use). They are created here, exactly
-- as 0002 created task_state; enum values are only ever appended.

CREATE TYPE estimate_tier   AS ENUM ('decomposed', 'considered', 'rough');
CREATE TYPE milestone_state AS ENUM ('open', 'locked');

-- Estimates (DESIGN-001 §7). Multiple rows per entity are allowed; the latest
-- (by created_at) is current, prior rows are the re-estimation history
-- (FR-1.1). Tier follows the evidence, assigned by the writer, never the
-- caller's say-so (FR-1.2). ref_type is constrained to the two entities that
-- carry estimates.
CREATE TABLE estimates (
  id          uuid PRIMARY KEY,
  ref_type    ref_type NOT NULL,        -- 'feature' | 'task'
  ref_id      uuid NOT NULL,
  tokens      bigint NOT NULL,
  tier        estimate_tier NOT NULL,
  rationale   text NOT NULL DEFAULT '', -- corpus neighbours cited, if any
  dispatch_id uuid REFERENCES dispatches(id),  -- the estimating dispatch, if AI-made
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK (ref_type IN ('feature', 'task'))
);
-- "latest is current" reads the newest row per entity; the id (UUIDv7) is the
-- within-transaction tiebreaker for rows sharing a created_at.
CREATE INDEX estimates_ref ON estimates (ref_type, ref_id, created_at DESC, id DESC);

-- Milestones (DESIGN-001 §6, vision §4). Membership is resolved live from
-- milestone_members; locking snapshots the resolved leaves once into
-- milestone_snapshots (FR-5.2, NFR-5).
CREATE TABLE milestones (
  id          uuid PRIMARY KEY,
  name        text NOT NULL,
  description text NOT NULL DEFAULT '',
  target_date date,
  state       milestone_state NOT NULL DEFAULT 'open',
  locked_at   timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE milestone_members (
  milestone_id uuid NOT NULL REFERENCES milestones(id),
  member_type  ref_type NOT NULL,       -- 'initiative' (transitive) | 'feature' | 'milestone' (SD-2: no checklists yet)
  member_id    uuid NOT NULL,
  added_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (milestone_id, member_type, member_id),
  CHECK (member_type IN ('initiative', 'feature', 'milestone'))
);

CREATE TABLE milestone_snapshots (       -- written once, at lock time (NFR-5)
  milestone_id uuid NOT NULL REFERENCES milestones(id),
  leaf_type    ref_type NOT NULL,        -- resolved leaves: 'feature' (SD-2: jobs deferred)
  leaf_id      uuid NOT NULL,
  PRIMARY KEY (milestone_id, leaf_type, leaf_id),
  CHECK (leaf_type IN ('feature'))
);

CREATE TABLE roadmaps (
  id   uuid PRIMARY KEY,
  name text NOT NULL
);

-- Ordered sequence of milestones; the order is the planner's to mean, the
-- system only preserves it (FR-6.1).
CREATE TABLE roadmap_entries (
  roadmap_id   uuid NOT NULL REFERENCES roadmaps(id),
  milestone_id uuid NOT NULL REFERENCES milestones(id),
  position     integer NOT NULL,
  PRIMARY KEY (roadmap_id, milestone_id)
);
CREATE INDEX roadmap_entries_order ON roadmap_entries (roadmap_id, position);
