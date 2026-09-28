-- 0010_identity.sql — documents with identity (SPEC-015, DESIGN-010 §7).
-- Forward-only and additive (DESIGN-001 §11). Version 0009 belongs to M5
-- (checklists and jobs), which merges first; nothing here depends on it, and
-- the checklist block below runs only if its table exists.
--
-- Every entity gets a short ID minted by the database, one sequence per
-- prefix (SPEC-015 SD-1 to SD-3). The ID is a column default, so every insert
-- gets one without asking, whatever code path made it. The existing row ids
-- stay the primary keys; nothing that references them changes.

-- --- Minting (FR-1.1) ---
--
-- One sequence per prefix in internal/ident's registry, including those whose
-- entity doesn't exist yet (BUG, SPK) and decisions, which are documents
-- (DESIGN-010 §7). Adding a bug table later is a column default, not a
-- sequence.

CREATE SEQUENCE ident_init_seq;
CREATE SEQUENCE ident_feat_seq;
CREATE SEQUENCE ident_bug_seq;
CREATE SEQUENCE ident_spk_seq;
CREATE SEQUENCE ident_dec_seq;
CREATE SEQUENCE ident_ms_seq;
CREATE SEQUENCE ident_rm_seq;
CREATE SEQUENCE ident_cl_seq;

-- The number, zero-padded to width and never truncated: 7 is "007", 1234 is
-- "1234" (SD-3).
CREATE FUNCTION ident_number(n bigint, width integer) RETURNS text
LANGUAGE sql IMMUTABLE AS $$
  SELECT CASE WHEN length(n::text) >= width THEN n::text ELSE lpad(n::text, width, '0') END
$$;

CREATE FUNCTION mint_ident(prefix text) RETURNS text
LANGUAGE sql VOLATILE AS $$
  SELECT prefix || '-' || ident_number(nextval(('ident_' || lower(prefix) || '_seq')::regclass), 3)
$$;

-- Moves a prefix's sequence so the next number is above n; never backwards.
-- Adopting DEC-005 calls this with 5 (FR-5.5); the backfill calls it with the
-- count it numbered.
CREATE FUNCTION ident_advance(prefix text, n bigint) RETURNS void
LANGUAGE plpgsql AS $$
DECLARE
  seq  regclass := ('ident_' || lower(prefix) || '_seq')::regclass;
  used bigint;
BEGIN
  EXECUTE format('SELECT CASE WHEN is_called THEN last_value ELSE last_value - 1 END FROM %s', seq)
    INTO used;
  IF n > used THEN
    PERFORM setval(seq, n, true);
  END IF;
END $$;

-- --- Entities, numbered in creation order (FR-1.2, FR-2.1) ---
--
-- Each table: add the column, number the existing rows by created_at then row
-- id, move the sequence past them, then make the column the default, NOT NULL
-- and unique. The updated_at triggers are paused so numbering isn't recorded
-- as an edit.

ALTER TABLE initiatives ADD COLUMN public_id text;
ALTER TABLE initiatives DISABLE TRIGGER initiatives_updated_at;
UPDATE initiatives i SET public_id = 'INIT-' || ident_number(o.rn, 3)
  FROM (SELECT id, row_number() OVER (ORDER BY created_at, id) AS rn FROM initiatives) o
  WHERE i.id = o.id;
ALTER TABLE initiatives ENABLE TRIGGER initiatives_updated_at;
SELECT ident_advance('INIT', (SELECT count(*) FROM initiatives));
ALTER TABLE initiatives
  ALTER COLUMN public_id SET DEFAULT mint_ident('INIT'),
  ALTER COLUMN public_id SET NOT NULL,
  ADD CONSTRAINT initiatives_public_id UNIQUE (public_id);

-- Features also gain their task counter (SD-4) and the mark that keeps an
-- existing feature's authored documents at SPEC-009's paths (SD-16).
ALTER TABLE features
  ADD COLUMN public_id text,
  ADD COLUMN task_seq integer NOT NULL DEFAULT 0,
  ADD COLUMN legacy_doc_paths boolean NOT NULL DEFAULT false;
ALTER TABLE features DISABLE TRIGGER features_updated_at;
UPDATE features f SET public_id = 'FEAT-' || ident_number(o.rn, 3), legacy_doc_paths = true
  FROM (SELECT id, row_number() OVER (ORDER BY created_at, id) AS rn FROM features) o
  WHERE f.id = o.id;
SELECT ident_advance('FEAT', (SELECT count(*) FROM features));
ALTER TABLE features
  ALTER COLUMN public_id SET DEFAULT mint_ident('FEAT'),
  ALTER COLUMN public_id SET NOT NULL,
  ADD CONSTRAINT features_public_id UNIQUE (public_id);

ALTER TABLE milestones ADD COLUMN public_id text;
UPDATE milestones m SET public_id = 'MS-' || ident_number(o.rn, 3)
  FROM (SELECT id, row_number() OVER (ORDER BY created_at, id) AS rn FROM milestones) o
  WHERE m.id = o.id;
SELECT ident_advance('MS', (SELECT count(*) FROM milestones));
ALTER TABLE milestones
  ALTER COLUMN public_id SET DEFAULT mint_ident('MS'),
  ALTER COLUMN public_id SET NOT NULL,
  ADD CONSTRAINT milestones_public_id UNIQUE (public_id);

-- Roadmaps have no created_at; their row ids are UUIDv7, ordered by creation
-- time (SD-5).
ALTER TABLE roadmaps ADD COLUMN public_id text;
UPDATE roadmaps r SET public_id = 'RM-' || ident_number(o.rn, 3)
  FROM (SELECT id, row_number() OVER (ORDER BY id) AS rn FROM roadmaps) o
  WHERE r.id = o.id;
SELECT ident_advance('RM', (SELECT count(*) FROM roadmaps));
ALTER TABLE roadmaps
  ALTER COLUMN public_id SET DEFAULT mint_ident('RM'),
  ALTER COLUMN public_id SET NOT NULL,
  ADD CONSTRAINT roadmaps_public_id UNIQUE (public_id);

-- --- Tasks, numbered by their feature (FR-1.3, FR-2.2) ---
--
-- FEAT-023-T03: the feature's ID and the next number from its own counter.
-- There is no default, because a default can't read the feature's row;
-- CreateTask writes the ID and bumps task_seq in one statement.

ALTER TABLE tasks ADD COLUMN public_id text;
ALTER TABLE tasks DISABLE TRIGGER tasks_updated_at;
UPDATE tasks t SET public_id = f.public_id || '-T' || ident_number(o.rn, 2)
  FROM (SELECT id, row_number() OVER (PARTITION BY feature_id ORDER BY created_at, position, id) AS rn
        FROM tasks) o, features f
  WHERE t.id = o.id AND f.id = t.feature_id;
ALTER TABLE tasks ENABLE TRIGGER tasks_updated_at;
UPDATE features f SET task_seq = c.n
  FROM (SELECT feature_id, count(*) AS n FROM tasks GROUP BY feature_id) c
  WHERE f.id = c.feature_id;
ALTER TABLE features ENABLE TRIGGER features_updated_at;
ALTER TABLE tasks
  ALTER COLUMN public_id SET NOT NULL,
  ADD CONSTRAINT tasks_public_id UNIQUE (public_id);

-- --- Checklists, if M5's table is here (FR-2.3) ---
--
-- M5 creates checklists in 0009 and merges first, so on the merged line this
-- runs after it. On a database that has 0010 but not yet 0009 it does
-- nothing, and those checklists will have no ID: recreate such a database.
-- Dynamic SQL, so nothing here is planned against a table that doesn't exist.

DO $$
BEGIN
  IF to_regclass('checklists') IS NOT NULL THEN
    EXECUTE 'ALTER TABLE checklists ADD COLUMN IF NOT EXISTS public_id text';
    EXECUTE $q$
      UPDATE checklists c SET public_id = 'CL-' || ident_number(o.rn, 3)
        FROM (SELECT id, row_number() OVER (ORDER BY id) AS rn FROM checklists) o
        WHERE c.id = o.id AND c.public_id IS NULL
    $q$;
    EXECUTE $q$ SELECT ident_advance('CL', (SELECT count(*) FROM checklists)) $q$;
    EXECUTE $q$ ALTER TABLE checklists
      ALTER COLUMN public_id SET DEFAULT mint_ident('CL'),
      ALTER COLUMN public_id SET NOT NULL $q$;
    EXECUTE 'ALTER TABLE checklists ADD CONSTRAINT checklists_public_id UNIQUE (public_id)';
  END IF;
END $$;

-- --- Documents: an ID and a revision (FR-3.1) ---
--
-- Both nullable: a document registered before this migration, or attached
-- without adoption, has none and keeps its path identity (SD-7, SD-12). The
-- ID alone isn't unique, because an approved document and its successor
-- draft share it while a revision is open.

ALTER TABLE documents
  ADD COLUMN public_id text,
  ADD COLUMN revision  integer,
  ADD CONSTRAINT documents_identity_pair CHECK ((public_id IS NULL) = (revision IS NULL)),
  ADD CONSTRAINT documents_revision_positive CHECK (revision IS NULL OR revision > 0);
CREATE UNIQUE INDEX documents_public_revision ON documents (public_id, revision)
  WHERE public_id IS NOT NULL;

-- Decisions are documents with their own number (DESIGN-010 §11, SD-6). Not
-- used in this transaction, so adding the value here is allowed.
ALTER TYPE document_type ADD VALUE IF NOT EXISTS 'decision';
