-- 0001_init.sql — phase-1 schema (DESIGN-001; SPEC-001 NFR-3).
-- Forward-only: a mistake here is corrected by a later migration, never by
-- editing this file (DESIGN-001 §11). Only the enums and tables phase 1 uses
-- are created; later phases add theirs by migration.

CREATE TYPE feature_state    AS ENUM ('idea', 'ready', 'active', 'review', 'done', 'abandoned');
CREATE TYPE document_state   AS ENUM ('draft', 'reviewing', 'approved', 'superseded');
CREATE TYPE document_type    AS ENUM ('spec', 'dev_plan', 'design', 'research', 'report', 'note', 'policy');
CREATE TYPE dispatch_state   AS ENUM ('queued', 'running', 'succeeded', 'failed', 'cancelled');
CREATE TYPE checkpoint_state AS ENUM ('pending', 'answered', 'expired', 'withdrawn');
CREATE TYPE ref_type         AS ENUM ('project', 'initiative', 'feature', 'task', 'defect', 'milestone', 'document', 'checklist', 'job');

-- updated_at maintenance; triggers encode no business rules (DESIGN-001 §2.4)
CREATE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END $$ LANGUAGE plpgsql;

-- NOTIFY fan-out on state changes (DESIGN-001 §10); in the single-server
-- deployment the in-process bus is primary and this is the safety net.
CREATE FUNCTION notify_state_change() RETURNS trigger AS $$
BEGIN
  IF NEW.state IS DISTINCT FROM OLD.state THEN
    PERFORM pg_notify('cromwell_events', json_build_object(
      'kind', TG_ARGV[0] || '.transition',
      'ref_type', TG_ARGV[0],
      'ref_id', NEW.id
    )::text);
  END IF;
  RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE TABLE initiatives (
  id          uuid PRIMARY KEY,
  parent_id   uuid REFERENCES initiatives(id),
  slug        text NOT NULL,
  name        text NOT NULL,
  description text NOT NULL DEFAULT '',
  archived    boolean NOT NULL DEFAULT false,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (parent_id, slug)
);
CREATE TRIGGER initiatives_updated_at BEFORE UPDATE ON initiatives
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE features (
  id            uuid PRIMARY KEY,
  initiative_id uuid NOT NULL REFERENCES initiatives(id),
  slug          text NOT NULL,
  name          text NOT NULL,
  description   text NOT NULL DEFAULT '',
  state         feature_state NOT NULL DEFAULT 'idea',
  branch        text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (initiative_id, slug)
);
CREATE TRIGGER features_updated_at BEFORE UPDATE ON features
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER features_notify AFTER UPDATE ON features
  FOR EACH ROW EXECUTE FUNCTION notify_state_change('feature');

CREATE TABLE documents (
  id            uuid PRIMARY KEY,
  type          document_type NOT NULL,
  state         document_state NOT NULL DEFAULT 'draft',
  owner_type    ref_type NOT NULL,
  owner_id      uuid,
  path          text NOT NULL,
  title         text NOT NULL,
  tags          text[] NOT NULL DEFAULT '{}',
  supersedes_id uuid REFERENCES documents(id),
  content_hash  text NOT NULL,
  indexed_at    timestamptz,
  submitted_at  timestamptz,
  approved_at   timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  CHECK (owner_type IN ('project', 'initiative', 'feature')),
  CHECK ((owner_type = 'project') = (owner_id IS NULL))
);
-- one live document per path (DESIGN-001 §5)
CREATE UNIQUE INDEX documents_live_path ON documents (path)
  WHERE state <> 'superseded';
CREATE TRIGGER documents_updated_at BEFORE UPDATE ON documents
  FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER documents_notify AFTER UPDATE ON documents
  FOR EACH ROW EXECUTE FUNCTION notify_state_change('document');

CREATE TABLE document_sections (
  id          uuid PRIMARY KEY,
  document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  position    integer NOT NULL,
  heading     text NOT NULL,
  level       integer NOT NULL,
  content     text NOT NULL,
  role_class  text,
  fts         tsvector GENERATED ALWAYS AS (to_tsvector('english', heading || ' ' || content)) STORED
  -- embedding vector(...) arrives with its phase-2 migration (DESIGN-001 §12)
);
CREATE INDEX document_sections_fts ON document_sections USING gin (fts);
CREATE INDEX document_sections_doc ON document_sections (document_id, position);

CREATE TABLE dispatches (
  id                 uuid PRIMARY KEY,
  state              dispatch_state NOT NULL DEFAULT 'queued',
  purpose            text NOT NULL,
  role               text NOT NULL,
  model              text NOT NULL,
  ref_type           ref_type NOT NULL,
  ref_id             uuid NOT NULL,
  attempt            integer NOT NULL DEFAULT 1,
  idempotency_key    text NOT NULL,
  queue_reason       text,
  input_tokens       bigint,
  output_tokens      bigint,
  cache_read_tokens  bigint,
  cache_write_tokens bigint,
  cost_usd           numeric(12,6),
  price_snapshot     jsonb,
  outcome            jsonb,
  error              text,
  queued_at          timestamptz NOT NULL DEFAULT now(),
  started_at         timestamptz,
  finished_at        timestamptz,
  heartbeat_at       timestamptz
);
CREATE INDEX dispatches_ref ON dispatches (ref_type, ref_id);
CREATE INDEX dispatches_open ON dispatches (state) WHERE state IN ('queued', 'running');
-- idempotency: one live dispatch per key (DESIGN-002 §8); terminal retries
-- reuse the key with a new attempt number
CREATE UNIQUE INDEX dispatches_idem ON dispatches (idempotency_key)
  WHERE state IN ('queued', 'running', 'succeeded');

CREATE TABLE tool_calls (
  id           uuid PRIMARY KEY,
  dispatch_id  uuid NOT NULL REFERENCES dispatches(id),
  seq          integer NOT NULL,
  tool         text NOT NULL,
  arg_bytes    integer NOT NULL,
  result_bytes integer NOT NULL,
  latency_ms   integer NOT NULL,
  status       text NOT NULL
);
CREATE INDEX tool_calls_dispatch ON tool_calls (dispatch_id, seq);

CREATE TABLE document_comments (
  id          uuid PRIMARY KEY,
  document_id uuid NOT NULL REFERENCES documents(id),
  dispatch_id uuid REFERENCES dispatches(id),
  author      text NOT NULL,
  section_ref text,
  body        text NOT NULL,
  resolved    boolean NOT NULL DEFAULT false,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX document_comments_doc ON document_comments (document_id, created_at);

CREATE TABLE audit_events (
  id          uuid PRIMARY KEY,
  occurred_at timestamptz NOT NULL DEFAULT now(),
  actor       text NOT NULL,
  kind        text NOT NULL,
  ref_type    ref_type NOT NULL,
  ref_id      uuid,
  payload     jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX audit_events_ref ON audit_events (ref_type, ref_id, occurred_at);
CREATE INDEX audit_events_kind ON audit_events (kind, occurred_at);

CREATE TABLE checkpoints (
  id           uuid PRIMARY KEY,
  state        checkpoint_state NOT NULL DEFAULT 'pending',
  kind         text NOT NULL,
  ref_type     ref_type NOT NULL,
  ref_id       uuid NOT NULL,
  question     text NOT NULL,
  context      jsonb NOT NULL DEFAULT '{}',
  options      jsonb,
  response     jsonb,
  responded_by text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  answered_at  timestamptz
);
-- idempotent per (kind, ref) while pending (DESIGN-002 §7, FR-6.2)
CREATE UNIQUE INDEX checkpoints_pending ON checkpoints (kind, ref_type, ref_id)
  WHERE state = 'pending';
CREATE TRIGGER checkpoints_notify AFTER UPDATE ON checkpoints
  FOR EACH ROW EXECUTE FUNCTION notify_state_change('checkpoint');
