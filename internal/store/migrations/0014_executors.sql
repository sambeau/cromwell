-- 0014_executors.sql — executors: who did a task's work, and the claims by
-- which a person or the chat agent holds it (SPEC-020, DESIGN-010 §5b).
-- Forward-only and additive (DESIGN-001 §11).

-- The audit rows that are about a claim itself (SPEC-020 FR-2.1). Not used in
-- this transaction, so adding the value here is allowed.
ALTER TYPE ref_type ADD VALUE IF NOT EXISTS 'claim';

-- --- Claims (FR-2.1) ---
--
-- A person or the chat agent holding a task instead of an agent. `ref_type`
-- and `ref_id` have no foreign key: M14 claims spikes too. `feature_id` is the
-- feature whose working copy the claim holds, null when it holds none.

CREATE TABLE work_claims (
  id               uuid PRIMARY KEY,
  ref_type         text NOT NULL,
  ref_id           uuid NOT NULL,
  feature_id       uuid REFERENCES features(id),
  kind             text NOT NULL CHECK (kind IN ('chat', 'person')),
  actor            text NOT NULL,
  via              text NOT NULL CHECK (via IN ('mcp', 'ui')),
  state            text NOT NULL DEFAULT 'open'
                   CHECK (state IN ('open', 'submitted', 'returned', 'ended')),
  end_reason       text CHECK (end_reason IN ('done', 'released', 'abandoned')),
  ended_by         text,
  claimed_at       timestamptz NOT NULL DEFAULT now(),
  last_activity_at timestamptz NOT NULL DEFAULT now(),
  last_activity    text NOT NULL DEFAULT 'claimed',
  worktree_seen    text NOT NULL DEFAULT '',
  deadline_at      timestamptz,
  submitted_at     timestamptz,
  ended_at         timestamptz,
  CONSTRAINT work_claims_end_reason CHECK ((state = 'ended') = (end_reason IS NOT NULL))
);
-- One claim that hasn't ended per item.
CREATE UNIQUE INDEX work_claims_current ON work_claims (ref_type, ref_id)
  WHERE state <> 'ended';
-- One open claim per feature: it holds the working copy (SD-4).
CREATE UNIQUE INDEX work_claims_open_feature ON work_claims (feature_id)
  WHERE state = 'open' AND feature_id IS NOT NULL;
CREATE INDEX work_claims_feature ON work_claims (feature_id) WHERE feature_id IS NOT NULL;

-- --- Executions (FR-1.1) ---
--
-- One row per executor's part in a round. Rows go with their item when the
-- service deletes it, since `ref_id` has no foreign key.

CREATE TABLE executions (
  id           uuid PRIMARY KEY,
  ref_type     text NOT NULL,
  ref_id       uuid NOT NULL,
  round        integer NOT NULL CHECK (round >= 1),
  kind         text NOT NULL CHECK (kind IN ('agent', 'chat', 'person')),
  actor        text NOT NULL,
  model        text NOT NULL DEFAULT '',
  dispatch_id  uuid REFERENCES dispatches(id),
  claim_id     uuid REFERENCES work_claims(id),
  via          text NOT NULL CHECK (via IN ('agent', 'mcp', 'ui')),
  measured     boolean NOT NULL,
  start_head   text NOT NULL DEFAULT '',
  inferred     boolean NOT NULL DEFAULT false,
  started_at   timestamptz NOT NULL DEFAULT now(),
  submitted_at timestamptz,
  CONSTRAINT executions_agent CHECK (kind <> 'agent' OR (dispatch_id IS NOT NULL AND measured)),
  CONSTRAINT executions_claimed CHECK (kind = 'agent' OR (claim_id IS NOT NULL AND NOT measured))
);
CREATE INDEX executions_ref ON executions (ref_type, ref_id, started_at, id);
-- Each run and each claim-round is one row; a round may hold any number. A
-- claim resumed after a send-back is the same claim in a later round (FR-1.2),
-- so a claim's uniqueness is per round.
CREATE UNIQUE INDEX executions_dispatch ON executions (dispatch_id)
  WHERE kind = 'agent';
CREATE UNIQUE INDEX executions_claim ON executions (claim_id, round)
  WHERE kind <> 'agent';

-- --- The branch watch (FR-5.6) ---
--
-- `watched_head` is the last branch head the watch accounted for. Null until
-- it starts. `worktree_commits` are the commits Subutai made on a feature's
-- branch, so the watch can tell them from anyone else's.

ALTER TABLE worktrees ADD COLUMN watched_head text;

CREATE TABLE worktree_commits (
  worktree_id uuid NOT NULL REFERENCES worktrees(id) ON DELETE CASCADE,
  hash        text NOT NULL,
  act         text NOT NULL CHECK (act IN ('implementation', 'submit', 'release')),
  at          timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (worktree_id, hash)
);

-- --- Backfill (FR-1.3) ---
--
-- One `agent` row, inferred, for every implement-task dispatch with a
-- `dispatch.running` audit row, started at the first such row. Its round is
-- one more than the task's request_changes transitions before that time. The
-- branch head at the start isn't known, so `start_head` stays empty. No
-- claims existed before this migration.

INSERT INTO executions (id, ref_type, ref_id, round, kind, actor, model, dispatch_id, via,
                        measured, start_head, inferred, started_at)
SELECT gen_random_uuid(), 'task', d.ref_id,
       1 + (SELECT count(*) FROM audit_events rc
            WHERE rc.kind = 'task.transition' AND rc.ref_type = 'task' AND rc.ref_id = d.ref_id
              AND rc.payload->>'event' = 'request_changes'
              AND rc.occurred_at < r.first_running),
       'agent', d.role, d.model, d.id, 'agent', true, '', true, r.first_running
FROM dispatches d
JOIN LATERAL (
  SELECT min(ae.occurred_at) AS first_running
  FROM audit_events ae
  WHERE ae.kind = 'dispatch.running' AND ae.payload->>'dispatch_id' = d.id::text
) r ON r.first_running IS NOT NULL
WHERE d.purpose = 'implement-task' AND d.ref_type = 'task';
