-- 0002_tasks_worktrees.sql — phase-2 schema (DESIGN-005 §7, SPEC-002 NFR-3).
-- Adds only what phase 2 uses; forward-only (DESIGN-001 §11). Phase 1 shipped
-- only the enums it used, so task_state is created here (its values match
-- DESIGN-001 §3 exactly; enum values are only ever appended).

CREATE TYPE task_state AS ENUM ('pending', 'ready', 'active', 'review', 'done', 'abandoned');

ALTER TABLE features ADD COLUMN spec_stale boolean NOT NULL DEFAULT false;

CREATE TABLE tasks (
  id          uuid PRIMARY KEY,
  feature_id  uuid NOT NULL REFERENCES features(id),
  position    integer NOT NULL,
  local_id    text,                     -- dev-plan-local id (e.g. 'T1'); stable re-decomposition key (DP-3)
  title       text NOT NULL,
  description text NOT NULL DEFAULT '',
  state       task_state NOT NULL DEFAULT 'pending',
  depends_on  uuid[] NOT NULL DEFAULT '{}',
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_feature ON tasks (feature_id, position);
-- local_id is unique per feature among non-abandoned tasks (DESIGN-005 §7)
CREATE UNIQUE INDEX tasks_local_id ON tasks (feature_id, local_id)
  WHERE local_id IS NOT NULL AND state <> 'abandoned';
CREATE TRIGGER tasks_updated_at BEFORE UPDATE ON tasks
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER tasks_notify AFTER UPDATE ON tasks
  FOR EACH ROW EXECUTE FUNCTION notify_state_change('task');

CREATE TABLE worktrees (
  id            uuid PRIMARY KEY,
  feature_id    uuid NOT NULL REFERENCES features(id),
  path          text NOT NULL,
  branch        text NOT NULL,
  graph_project text,                   -- codebase-memory-mcp project; NULL in phase 2 (SD-1)
  created_at    timestamptz NOT NULL DEFAULT now(),
  removed_at    timestamptz
);
-- at most one live worktree per feature
CREATE UNIQUE INDEX worktrees_live_feature ON worktrees (feature_id)
  WHERE removed_at IS NULL;
