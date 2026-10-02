-- 0016_spike_executors.sql — who runs a spike, and its time box (SPEC-021 §4,
-- FR-11.1). Forward-only and additive (DESIGN-001 §11). Nothing here names a
-- new enum value: the new words (the executor, `time_box`, `expired`) are
-- text, and checked as text.

-- --- The new columns (FR-11.1) ---
--
-- `executor` is who may run the spike: agent, chat or person. It is null
-- until the spike starts. `time_box_hours` and `deadline_at` are a chat or
-- person spike's limit, set at the start; an agent spike has a token budget
-- instead (SD-18).

ALTER TABLE spikes ADD COLUMN executor text;
ALTER TABLE spikes ADD COLUMN time_box_hours integer;
ALTER TABLE spikes ADD COLUMN deadline_at timestamptz;

-- --- 1. Backfill ---
--
-- Every spike that has started was run by the spike runner, so it is an agent
-- spike. This comes first, so that the checks below validate rows that
-- already satisfy them.

UPDATE spikes SET executor = 'agent' WHERE started_at IS NOT NULL;

-- One `agent` row in `executions`, inferred, for every run-spike dispatch
-- with a `dispatch.running` audit row, started at the first such row, as 0014
-- did for tasks. A spike has one round. The branch head means nothing for a
-- spike's detached worktree, so `start_head` stays empty.

INSERT INTO executions (id, ref_type, ref_id, round, kind, actor, model, dispatch_id, via,
                        measured, start_head, inferred, started_at)
SELECT gen_random_uuid(), 'spike', d.ref_id, 1, 'agent', d.role, d.model, d.id, 'agent',
       true, '', true, r.first_running
FROM dispatches d
JOIN LATERAL (
  SELECT min(ae.occurred_at) AS first_running
  FROM audit_events ae
  WHERE ae.kind = 'dispatch.running' AND ae.payload->>'dispatch_id' = d.id::text
) r ON r.first_running IS NOT NULL
WHERE d.purpose = 'run-spike' AND d.ref_type::text = 'spike'
ON CONFLICT (dispatch_id) WHERE kind = 'agent' DO NOTHING;

-- --- 2. The spikes' checks that assumed a token budget (FR-11.1) ---
--
-- 0015 named every check, so each is dropped by name. The replacements are
-- keyed on started_at rather than on the state: a spike closed after a run
-- has its executor too. Each is written with IS NOT NULL so that a null
-- column can't make a check pass by being unknown.

ALTER TABLE spikes DROP CONSTRAINT spikes_started;
ALTER TABLE spikes ADD CONSTRAINT spikes_started CHECK (
  -- a spike that has started has its executor
  (started_at IS NULL OR executor IS NOT NULL)
  -- a spike that is running or over has started
  AND (state NOT IN ('running', 'ended') OR started_at IS NOT NULL)
  -- an agent spike has its token budget
  AND (NOT (started_at IS NOT NULL AND executor IS NOT NULL AND executor = 'agent')
       OR token_budget IS NOT NULL)
  -- a chat or person spike has its time box and its deadline
  AND (NOT (started_at IS NOT NULL AND executor IS NOT NULL AND executor IN ('chat', 'person'))
       OR (time_box_hours IS NOT NULL AND deadline_at IS NOT NULL)));

ALTER TABLE spikes DROP CONSTRAINT spikes_ended_how_values;
ALTER TABLE spikes ADD CONSTRAINT spikes_ended_how_values
  CHECK (ended_how IN ('concluded', 'budget', 'turn_limit', 'failed', 'time_box'));

-- Only an agent spike stops at a budget or a turn limit, and only a chat or
-- person spike has a time box to reach.
ALTER TABLE spikes ADD CONSTRAINT spikes_ended_how_executor CHECK (
  (ended_how IS NULL OR ended_how NOT IN ('budget', 'turn_limit')
    OR (executor IS NOT NULL AND executor = 'agent'))
  AND (ended_how IS NULL OR ended_how <> 'time_box'
    OR (executor IS NOT NULL AND executor IN ('chat', 'person'))));

ALTER TABLE spikes ADD CONSTRAINT spikes_executor_values
  CHECK (executor IS NULL OR executor IN ('agent', 'chat', 'person'));
ALTER TABLE spikes ADD CONSTRAINT spikes_time_box_positive
  CHECK (time_box_hours IS NULL OR time_box_hours BETWEEN 1 AND 168);

-- A spike has a token budget or a time box, never both: an agent spike has no
-- time box and no deadline, and a chat or person spike has no token budget.
ALTER TABLE spikes ADD CONSTRAINT spikes_one_limit CHECK (
  (executor IS NULL OR executor <> 'agent' OR (time_box_hours IS NULL AND deadline_at IS NULL))
  AND (executor IS NULL OR executor NOT IN ('chat', 'person') OR token_budget IS NULL));

-- --- 3. A claim that ended at its deadline (SD-21) ---
--
-- 0014's end-reason value check was unnamed, so Postgres called it
-- work_claims_end_reason_check. The replacement is named and allows `expired`.
-- The named work_claims_end_reason (ended exactly when there is a reason) is
-- kept as it is.

ALTER TABLE work_claims DROP CONSTRAINT work_claims_end_reason_check;
ALTER TABLE work_claims ADD CONSTRAINT work_claims_end_reason_values
  CHECK (end_reason IN ('done', 'released', 'abandoned', 'expired'));
