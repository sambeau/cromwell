-- 0013_bugs.sql — bugs (SPEC-019, DESIGN-010 §9). Forward-only and additive
-- (DESIGN-001 §11). M11 owns 0012 and merges first; nothing here depends on
-- it.

-- --- A bug is a feature row with a kind (SPEC-019 SD-1) ---
--
-- Everything after acceptance keys off a features row — the sent mark, the
-- documents, tasks, the worktree, dispatches, estimates, gates, the timeline
-- and milestone leaves — so a bug is one, marked. Its ID comes from the BUG
-- sequence: the column default stays mint_ident('FEAT'), and a bug's insert
-- names mint_ident('BUG') itself (FR-1.3). The check keeps the two honest.

ALTER TABLE features
  ADD COLUMN kind text NOT NULL DEFAULT 'feature',
  ADD CONSTRAINT features_kind CHECK (kind IN ('feature', 'bug')),
  ADD CONSTRAINT features_kind_prefix CHECK ((kind = 'bug') = (public_id LIKE 'BUG-%'));

-- A bug's report, which serves as its spec (DESIGN-010 §9, §17a item 6). Not
-- used in this transaction, so adding the value here is allowed.
ALTER TYPE document_type ADD VALUE IF NOT EXISTS 'bug_report';

-- --- What only a bug has (FR-1.1) ---
--
-- One row per bug row: who reported it and how, the feature it came from,
-- and its triage (SD-2). Triage is a column here, not a feature state: a
-- reported or accepted bug is an `idea`, and rejecting it or marking it a
-- duplicate abandons the feature row in the same transaction.

CREATE TABLE bugs (
  feature_id           uuid PRIMARY KEY REFERENCES features(id),
  origin_feature_id    uuid REFERENCES features(id),
  triage               text NOT NULL DEFAULT 'reported'
                       CHECK (triage IN ('reported', 'accepted', 'rejected', 'duplicate')),
  reporter_kind        text NOT NULL CHECK (reporter_kind IN ('person', 'chat', 'agent', 'review')),
  reported_by          text NOT NULL,
  reported_dispatch_id uuid REFERENCES dispatches(id),
  source_task_id       uuid REFERENCES tasks(id),
  reported_at          timestamptz NOT NULL DEFAULT now(),
  duplicate_of         uuid REFERENCES features(id),
  decided_by           text,
  decided_via          text CHECK (decided_via IN ('ui', 'mcp')),
  decided_quote        text,
  decided_reason       text,
  decided_at           timestamptz,
  CONSTRAINT bugs_duplicate_pointer CHECK ((triage = 'duplicate') = (duplicate_of IS NOT NULL)),
  CONSTRAINT bugs_not_own_duplicate CHECK (duplicate_of IS DISTINCT FROM feature_id),
  CONSTRAINT bugs_decided CHECK (
    (triage = 'reported') = (decided_by IS NULL AND decided_via IS NULL AND decided_at IS NULL)),
  CONSTRAINT bugs_relay_quoted CHECK (decided_via IS DISTINCT FROM 'mcp' OR decided_quote IS NOT NULL),
  CONSTRAINT bugs_rejection_reason CHECK (triage <> 'rejected' OR coalesce(btrim(decided_reason), '') <> ''),
  CONSTRAINT bugs_agent_run CHECK (reporter_kind NOT IN ('agent', 'review') OR reported_dispatch_id IS NOT NULL)
);
CREATE INDEX bugs_queue ON bugs (triage, reported_at);
CREATE INDEX bugs_origin ON bugs (origin_feature_id) WHERE origin_feature_id IS NOT NULL;
