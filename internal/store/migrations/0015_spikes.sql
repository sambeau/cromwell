-- 0015_spikes.sql — spikes (SPEC-021, DESIGN-010 §10). Forward-only and
-- additive (DESIGN-001 §11). M13 owns 0014 and nothing here depends on it.

-- --- The new enum values (FR-1.1) ---
--
-- `ref_type` gains `spike`, for dispatches, audit rows, checkpoints and
-- document owners; `document_type` gains `findings`. Postgres refuses to use
-- a value added by ALTER TYPE in the transaction that added it, and a
-- migration runs in one transaction. So nothing below names either value as
-- an enum literal: the checks compare text, and no row is written. No other
-- table constrains ref_type to a list (checked against 0001 to 0013), so
-- nothing else needs widening.

ALTER TYPE ref_type ADD VALUE IF NOT EXISTS 'spike';
ALTER TYPE document_type ADD VALUE IF NOT EXISTS 'findings';

-- --- A spike's findings can be a document's owner (FR-1.1, Appendix A) ---
--
-- 0001's owner check was unnamed, so Postgres called it
-- documents_owner_type_check. The replacement is named, and compares
-- owner_type::text so that it holds no enum literal of the new value. The
-- second check (a project has no owner id, anything else has one) is kept
-- as it is.

ALTER TABLE documents DROP CONSTRAINT documents_owner_type_check;
ALTER TABLE documents ADD CONSTRAINT documents_owner_kind
  CHECK (owner_type::text IN ('project', 'initiative', 'feature', 'spike'));

-- --- A run that stopped at its limit says so in its transcript (FR-5.1) ---
--
-- 0008's kind check was unnamed (transcript_entries_kind_check). The
-- dispatcher's final entry for a run that stopped at its budget or turn
-- limit has the kind 'stop'.

ALTER TABLE transcript_entries DROP CONSTRAINT transcript_entries_kind_check;
ALTER TABLE transcript_entries ADD CONSTRAINT transcript_entries_kind
  CHECK (kind IN ('system', 'prompt', 'turn', 'text', 'tool_call',
                  'tool_result', 'nudge', 'outcome', 'error', 'stop'));

-- --- The spikes table (SD-1, FR-1.1) ---
--
-- A spike is not a feature row: it has no spec, plan, tasks, gates or merge,
-- so none of the feature code can reach it by accident. Its ID comes from
-- the SPK sequence that 0010 made. Every check is named, so stage 2 can drop
-- the ones that assume a token budget by name.

CREATE TABLE spikes (
  id                  uuid PRIMARY KEY,
  public_id           text NOT NULL UNIQUE DEFAULT mint_ident('SPK'),
  initiative_id       uuid NOT NULL REFERENCES initiatives(id),
  feature_id          uuid REFERENCES features(id),
  question            text NOT NULL,
  state               text NOT NULL DEFAULT 'idea',
  budget_override     bigint,
  token_budget        bigint,
  tokens_used         bigint NOT NULL DEFAULT 0,
  draft               text,
  draft_saved_at      timestamptz,
  ended_how           text,
  end_note            text,
  closed_as           text,
  follows_id          uuid REFERENCES spikes(id),
  base_commit         text,
  refs_at_start       jsonb,
  worktree_path       text,
  worktree_removed_at timestamptz,
  created_by          text NOT NULL,
  created_via         text NOT NULL,
  started_by          text,
  started_at          timestamptz,
  ended_at            timestamptz,
  closed_by           text,
  closed_at           timestamptz,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT spikes_state CHECK (state IN ('idea', 'running', 'ended', 'closed')),
  CONSTRAINT spikes_ended_how_values CHECK (ended_how IN ('concluded', 'budget', 'turn_limit', 'failed')),
  CONSTRAINT spikes_closed_as_values CHECK (closed_as IN ('answered', 'unanswered')),
  CONSTRAINT spikes_created_via CHECK (created_via IN ('ui', 'mcp')),
  -- closed_as is set exactly when the spike is closed
  CONSTRAINT spikes_closed_as CHECK ((state = 'closed') = (closed_as IS NOT NULL)),
  -- ended_how is set when the run is over: ended, or closed after a run
  CONSTRAINT spikes_ended_how CHECK (
    (state = 'ended' OR (state = 'closed' AND started_at IS NOT NULL)) = (ended_how IS NOT NULL)),
  -- a run in progress or over has its budget and its start
  CONSTRAINT spikes_started CHECK (
    state NOT IN ('running', 'ended') OR (token_budget IS NOT NULL AND started_at IS NOT NULL)),
  -- closed without a run can only be unanswered
  CONSTRAINT spikes_closed_unrun_unanswered CHECK (
    state <> 'closed' OR started_at IS NOT NULL OR closed_as = 'unanswered'),
  CONSTRAINT spikes_budget_positive CHECK (
    (budget_override IS NULL OR budget_override > 0) AND (token_budget IS NULL OR token_budget > 0)),
  CONSTRAINT spikes_tokens_nonneg CHECK (tokens_used >= 0),
  CONSTRAINT spikes_question CHECK (btrim(question) <> '' AND char_length(question) <= 500),
  CONSTRAINT spikes_not_self_follow CHECK (follows_id IS DISTINCT FROM id)
);
CREATE INDEX spikes_initiative ON spikes (initiative_id);
CREATE INDEX spikes_feature ON spikes (feature_id) WHERE feature_id IS NOT NULL;
CREATE INDEX spikes_state ON spikes (state);
CREATE TRIGGER spikes_updated_at BEFORE UPDATE ON spikes
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
