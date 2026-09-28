-- 0008_transcripts.sql — agent-run transcripts (SPEC-012 FR-1, FR-2.4).
-- Forward-only and additive (DESIGN-001 §11). Version 0007 belongs to a
-- parallel line of work (M3); the runner fills that gap when it lands
-- (SPEC-012 SD-10), and nothing here depends on it.
--
-- One row per entry of a run's conversation, appended as the run proceeds, so
-- a run that dies part-way keeps everything up to its last write (SD-1). A
-- failed run is retried on the same dispatches row with attempt + 1, so the
-- attempt is part of the key (SD-2).

CREATE TABLE transcript_entries (
  id                 uuid PRIMARY KEY,
  dispatch_id        uuid NOT NULL REFERENCES dispatches(id) ON DELETE CASCADE,
  attempt            integer NOT NULL,
  seq                integer NOT NULL,
  turn               integer NOT NULL DEFAULT 0,
  kind               text NOT NULL,
  tool_name          text,
  tool_use_id        text,
  content            text NOT NULL DEFAULT '',
  -- the size before any cut (SD-4)
  content_bytes      integer NOT NULL DEFAULT 0,
  truncated          boolean NOT NULL DEFAULT false,
  is_error           boolean NOT NULL DEFAULT false,
  latency_ms         integer,
  input_tokens       bigint,
  output_tokens      bigint,
  cache_read_tokens  bigint,
  cache_write_tokens bigint,
  created_at         timestamptz NOT NULL DEFAULT now(),
  CHECK (kind IN ('system', 'prompt', 'turn', 'text', 'tool_call',
                  'tool_result', 'nudge', 'outcome', 'error'))
);
-- Deliberately not unique: the running loop numbers its own entries, and the
-- failure that gives up on it appends one more from the database side. If the
-- two ever pick the same number, both rows are kept, and the time-ordered id
-- breaks the tie (SPEC-012 FR-1.9).
CREATE INDEX transcript_entries_run ON transcript_entries (dispatch_id, attempt, seq, id);

-- When retention removed a finished run's transcript (SD-5). The run's
-- outcome, tokens and tool ledger are kept regardless.
ALTER TABLE dispatches ADD COLUMN transcript_pruned_at timestamptz;

-- The retention sweep looks for finished runs by age.
CREATE INDEX dispatches_finished ON dispatches (finished_at) WHERE finished_at IS NOT NULL;
