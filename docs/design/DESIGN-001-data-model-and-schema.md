# DESIGN-001: Data Model and Postgres Schema

**Status:** Draft for review
**Date:** 2026-07-02
**Parent:** [vision-v1](../vision/vision-v1.md) §3 (Vocabulary), §5 (Sizing), §6 (Documents), §11 (Observability)
**Depends on:** DEC-001 (Go), DEC-002 (Supabase-hosted Postgres)

## 1. Purpose

Defines the Postgres schema for Cromwell's canonical state: work entities,
tracking entities, the document index, the audit log, the dispatch/cost ledger,
checkpoints, and the calibration corpus. This is the contract every other
component builds against.

Out of scope: the lifecycle *rules* (which transitions are legal, what gates
compute) — those are DESIGN-003. This document defines what is stored.

## 2. Principles

1. **Postgres holds state, never content.** Document bodies live in git as
   Markdown (vision §7). Postgres holds entity records, document *index* data
   (sections, metadata, embeddings), and ledgers. The one deliberate nuance:
   indexed section *text* is stored to power FTS and retrieval — it is a derived
   cache of the file, rebuilt on every document event, never authoritative.
2. **One table per entity type.** No generic `entities` table. Each concept in the
   vocabulary gets a typed table with real columns and enum states. Polymorphic
   references (a document's owner, an audit event's subject) use an
   `entity_ref` convention: `(ref_type, ref_id)` column pairs with a CHECK on
   `ref_type`.
3. **Append-only ledgers.** `audit_events`, `dispatches`, `tool_calls`, and
   `checkpoint` responses are insert-only. No UPDATE, no DELETE (enforced by
   revoking privileges from the app role where practical).
4. **State transitions happen in Go, recorded in SQL.** No triggers encode
   business rules. Triggers exist only for `NOTIFY` fan-out and `updated_at`
   maintenance. The lifecycle engine (DESIGN-003) is the single writer of state
   columns.
5. **IDs are UUIDv7.** Time-ordered, native `uuid` type, generated in Go.
   Human-facing short IDs (`FEAT-x7k2`) are a display encoding of the UUID's
   tail, not a separate key.

## 3. Enum types

```sql
CREATE TYPE feature_state   AS ENUM ('idea', 'ready', 'active', 'review', 'done', 'abandoned');
CREATE TYPE task_state      AS ENUM ('pending', 'ready', 'active', 'review', 'done', 'abandoned');
CREATE TYPE defect_state    AS ENUM ('reported', 'triaged', 'accepted', 'active', 'review', 'done', 'rejected');
CREATE TYPE document_state  AS ENUM ('draft', 'reviewing', 'approved', 'superseded');
CREATE TYPE document_type   AS ENUM ('spec', 'dev_plan', 'design', 'research', 'report', 'note', 'policy');
CREATE TYPE milestone_state AS ENUM ('open', 'locked');
CREATE TYPE job_state       AS ENUM ('pending', 'done');
CREATE TYPE estimate_tier   AS ENUM ('decomposed', 'considered', 'rough');
CREATE TYPE dispatch_state  AS ENUM ('queued', 'running', 'succeeded', 'failed', 'cancelled');
CREATE TYPE checkpoint_state AS ENUM ('pending', 'answered', 'expired', 'withdrawn');
CREATE TYPE ref_type        AS ENUM ('project', 'initiative', 'feature', 'task', 'defect', 'milestone', 'document', 'checklist', 'job');
```

The document lifecycle is the canonical four states from vision §3/§6. The
`submitted`/`reviewed` wording in vision §8 is an inconsistency; DESIGN-003
fixes the vocabulary as: the *event* is `submit`, the resulting *state* is
`reviewing`.

`defect_state` is a first sketch honouring the vision's placeholder status; the
triage vocabulary is revisable without structural change.

## 4. Work entities

```sql
CREATE TABLE initiatives (
  id          uuid PRIMARY KEY,
  parent_id   uuid REFERENCES initiatives(id),
  slug        text NOT NULL,            -- path segment, e.g. 'auth'
  name        text NOT NULL,
  description text NOT NULL DEFAULT '',
  archived    boolean NOT NULL DEFAULT false,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (parent_id, slug)
);
```

Initiatives have no lifecycle — only the `archived` bit (vision §4). Nesting is
by `parent_id`; a NULL parent is a top-level initiative. Cycle prevention is
enforced in the service layer (single writer), not by constraint.

```sql
CREATE TABLE features (
  id            uuid PRIMARY KEY,
  initiative_id uuid NOT NULL REFERENCES initiatives(id),
  slug          text NOT NULL,
  name          text NOT NULL,
  description   text NOT NULL DEFAULT '',
  state         feature_state NOT NULL DEFAULT 'idea',
  branch        text,                   -- git branch once active
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (initiative_id, slug)
);

CREATE TABLE tasks (
  id          uuid PRIMARY KEY,
  feature_id  uuid NOT NULL REFERENCES features(id),
  position    integer NOT NULL,         -- order within the dev-plan
  title       text NOT NULL,
  description text NOT NULL DEFAULT '',
  state       task_state NOT NULL DEFAULT 'pending',
  depends_on  uuid[] NOT NULL DEFAULT '{}',  -- task ids within the same feature
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE defects (
  id            uuid PRIMARY KEY,
  initiative_id uuid REFERENCES initiatives(id),   -- optional filing location
  feature_id    uuid REFERENCES features(id),      -- optional: defect against a feature
  title         text NOT NULL,
  description   text NOT NULL DEFAULT '',
  state         defect_state NOT NULL DEFAULT 'reported',
  severity      text,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE checklists (
  id         uuid PRIMARY KEY,
  name       text NOT NULL,
  owner_type ref_type NOT NULL,
  owner_id   uuid,                      -- NULL when owner_type = 'project'
  created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE jobs (
  id           uuid PRIMARY KEY,
  checklist_id uuid NOT NULL REFERENCES checklists(id),
  position     integer NOT NULL,
  title        text NOT NULL,
  state        job_state NOT NULL DEFAULT 'pending',
  done_at      timestamptz,
  done_by      text
);
```

A checklist's completion state is computed from its jobs (vision §3) — no stored
aggregate.

## 5. Documents

```sql
CREATE TABLE documents (
  id            uuid PRIMARY KEY,
  type          document_type NOT NULL,
  state         document_state NOT NULL DEFAULT 'draft',
  owner_type    ref_type NOT NULL,      -- 'project' | 'initiative' | 'feature'
  owner_id      uuid,                   -- NULL when owner_type = 'project'
  path          text NOT NULL,          -- repo-relative path to the .md file
  title         text NOT NULL,
  tags          text[] NOT NULL DEFAULT '{}',
  supersedes_id uuid REFERENCES documents(id),  -- the doc this one replaced
  content_hash  text NOT NULL,          -- sha256 of file at last index
  indexed_at    timestamptz,
  submitted_at  timestamptz,
  approved_at   timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (path) WHERE state <> 'superseded'
);
```

Revision model: amending an approved document creates a **new document row**
whose `supersedes_id` points at the old one; on the successor's approval the
predecessor transitions to `superseded` atomically (DESIGN-003 §5). History is
therefore a chain of rows, and "the current spec of feature X" is the
non-superseded spec owned by X.

```sql
CREATE TABLE document_sections (
  id           uuid PRIMARY KEY,
  document_id  uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  position     integer NOT NULL,
  heading      text NOT NULL,
  level        integer NOT NULL,        -- markdown heading level
  content      text NOT NULL,           -- derived cache of the file (principle 1)
  role_class   text,                    -- 'requirement' | 'decision' | 'rationale' | ... (agent-classified, nullable until classified)
  fts          tsvector GENERATED ALWAYS AS (to_tsvector('english', heading || ' ' || content)) STORED,
  embedding    vector(1536)             -- pgvector; NULL until embedded
);

CREATE INDEX ON document_sections USING gin (fts);
CREATE INDEX ON document_sections USING hnsw (embedding vector_cosine_ops);
```

Section rows are fully rebuilt on each index pass (delete + insert per document);
they carry no state of their own.

```sql
CREATE TABLE document_comments (
  id          uuid PRIMARY KEY,
  document_id uuid NOT NULL REFERENCES documents(id),
  dispatch_id uuid REFERENCES dispatches(id),  -- set when authored by an agent-reviewer
  author      text NOT NULL,                   -- human name or role name
  section_ref text,                            -- heading anchor, nullable for whole-doc comments
  body        text NOT NULL,
  resolved    boolean NOT NULL DEFAULT false,
  created_at  timestamptz NOT NULL DEFAULT now()
);
```

Comments are how `request_changes` feedback travels (vision §9); they persist
across resubmission so authors and reviewers see the thread.

## 6. Tracking entities

```sql
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
  member_type  ref_type NOT NULL,       -- 'initiative' (transitive) | 'feature' | 'checklist' | 'milestone'
  member_id    uuid NOT NULL,
  added_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (milestone_id, member_type, member_id)
);

CREATE TABLE milestone_snapshots (       -- written once, at lock time
  milestone_id uuid NOT NULL REFERENCES milestones(id),
  leaf_type    ref_type NOT NULL,        -- resolved leaves: 'feature' | 'job'
  leaf_id      uuid NOT NULL,
  PRIMARY KEY (milestone_id, leaf_type, leaf_id)
);

CREATE TABLE roadmaps (
  id   uuid PRIMARY KEY,
  name text NOT NULL
);

CREATE TABLE roadmap_entries (
  roadmap_id   uuid NOT NULL REFERENCES roadmaps(id),
  milestone_id uuid NOT NULL REFERENCES milestones(id),
  position     integer NOT NULL,
  PRIMARY KEY (roadmap_id, milestone_id)
);
```

Open milestones resolve membership live by expanding `milestone_members`
(initiatives expand to their descendant features transitively). Locking
(vision §4) resolves the expansion once into `milestone_snapshots`; progress
against a locked milestone reads the snapshot, current state of each leaf.

## 7. Sizing, estimates, calibration

```sql
CREATE TABLE estimates (
  id          uuid PRIMARY KEY,
  ref_type    ref_type NOT NULL,        -- 'feature' | 'task'
  ref_id      uuid NOT NULL,
  tokens      bigint NOT NULL,
  tier        estimate_tier NOT NULL,
  rationale   text NOT NULL DEFAULT '', -- corpus neighbours cited, if any
  dispatch_id uuid REFERENCES dispatches(id),  -- the estimating dispatch, if AI-made
  created_at  timestamptz NOT NULL DEFAULT now()
);
```

Multiple estimate rows per entity are allowed; the latest is current, prior rows
are the re-estimation history. Roll-ups (parent totals, worst-tier propagation,
`?` for unestimated — vision §5) are computed in queries, never stored.
Actuals are computed from the dispatch ledger (§8) by owner; the calibration
corpus is the join `(entity description, latest estimate, summed actuals)` —
a view, not a table.

## 8. Observability ledgers

```sql
CREATE TABLE audit_events (
  id           uuid PRIMARY KEY,
  occurred_at  timestamptz NOT NULL DEFAULT now(),
  actor        text NOT NULL,           -- 'orchestrator' | role name | human identifier
  kind         text NOT NULL,           -- 'document.transition' | 'gate.evaluated' | 'dispatch.started' | ...
  ref_type     ref_type NOT NULL,
  ref_id       uuid,
  payload      jsonb NOT NULL DEFAULT '{}'
);
CREATE INDEX ON audit_events (ref_type, ref_id, occurred_at);
CREATE INDEX ON audit_events (kind, occurred_at);
```

Every lifecycle transition, gate evaluation, dispatch, checkpoint, and budget
event writes a row *in the same transaction* as the state change it records —
completeness by construction (vision §11), not by discipline.

```sql
CREATE TABLE dispatches (
  id                 uuid PRIMARY KEY,
  state              dispatch_state NOT NULL DEFAULT 'queued',
  purpose            text NOT NULL,     -- 'review-spec' | 'implement-task' | 'estimate' | ...
  role               text NOT NULL,     -- role name from .cromwell/roles/
  model              text NOT NULL,
  ref_type           ref_type NOT NULL, -- owning entity for cost attribution
  ref_id             uuid NOT NULL,
  attempt            integer NOT NULL DEFAULT 1,
  input_tokens       bigint,
  output_tokens      bigint,
  cache_read_tokens  bigint,
  cache_write_tokens bigint,
  cost_usd           numeric(12,6),     -- computed at completion from price snapshot
  price_snapshot     jsonb,             -- per-token prices in effect at dispatch time
  outcome            jsonb,             -- structured result (review verdict, etc.)
  error              text,
  queued_at          timestamptz NOT NULL DEFAULT now(),
  started_at         timestamptz,
  finished_at        timestamptz,
  heartbeat_at       timestamptz        -- liveness for stall detection
);
CREATE INDEX ON dispatches (ref_type, ref_id);
CREATE INDEX ON dispatches (state) WHERE state IN ('queued', 'running');

CREATE TABLE tool_calls (
  id          uuid PRIMARY KEY,
  dispatch_id uuid NOT NULL REFERENCES dispatches(id),
  seq         integer NOT NULL,
  tool        text NOT NULL,
  arg_bytes   integer NOT NULL,
  result_bytes integer NOT NULL,
  latency_ms  integer NOT NULL,
  status      text NOT NULL             -- 'ok' | 'error' | 'retried'
);
```

`price_snapshot` resolves the vision's open question on historical pricing:
prices are frozen per dispatch, so retrospective cost is exact regardless of
later price-table edits. The price table itself lives in `.cromwell/config.yaml`
(config compartment), not the database.

Cost rollups (per task/feature/initiative/milestone/month — vision §5) are
queries over `dispatches` grouped by owner and joined through the entity tree.

## 9. Checkpoints and worktrees

```sql
CREATE TABLE checkpoints (
  id          uuid PRIMARY KEY,
  state       checkpoint_state NOT NULL DEFAULT 'pending',
  kind        text NOT NULL,            -- 'review-escalation' | 'dispatch-failure' | 'budget' | 'gate-override' | ...
  ref_type    ref_type NOT NULL,
  ref_id      uuid NOT NULL,
  question    text NOT NULL,
  context     jsonb NOT NULL DEFAULT '{}',   -- reviewer reasoning, error details
  options     jsonb,                    -- suggested responses, if enumerable
  response    jsonb,
  responded_by text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  answered_at timestamptz
);

CREATE TABLE worktrees (
  id            uuid PRIMARY KEY,
  feature_id    uuid NOT NULL REFERENCES features(id),
  path          text NOT NULL,
  branch        text NOT NULL,
  graph_project text,                   -- codebase-memory-mcp project name
  created_at    timestamptz NOT NULL DEFAULT now(),
  removed_at    timestamptz
);
```

## 10. Eventing

A single trigger-based channel:

```sql
NOTIFY cromwell_events, '{"kind": "document.transition", "ref_type": "document", "ref_id": "..."}'
```

`AFTER UPDATE` triggers on state columns of `documents`, `features`, `tasks`,
`defects`, and `checkpoints` emit NOTIFY. In the single-server deployment the
in-process event bus is primary and NOTIFY is the safety net / future
multi-process channel (DESIGN-002 §3); the trigger exists from day one so the
contract never changes.

## 11. Migration policy

- Forward-only numbered SQL files: `migrations/0001_init.sql`, ...
- Embedded in the server binary via `embed.FS`; applied by `cromwell init` and
  `cromwell upgrade` inside a transaction each, recorded in `schema_migrations
  (version, applied_at)`.
- No down migrations. A mistake is corrected by a new forward migration.
- Phase 1 ships only the tables phase 1 uses (see SPEC-001 §4); later phases add
  tables by migration. Enum values are only ever appended.

## 12. Open questions carried forward

- Embedding dimensionality (1536 above) is provisional pending an embedding-model
  decision; the column is created in the migration that first uses it, so the
  choice is deferred to phase 2 without schema debt.
- Defect triage states are a sketch; revisit when the Defect design (vision §14)
  is written.
- Whether `document_sections.role_class` classification runs on every index pass
  or lazily is a phase-2 orchestrator policy question, not a schema question.
