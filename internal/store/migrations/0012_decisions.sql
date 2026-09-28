-- 0012_decisions.sql — decisions and conventions (SPEC-018 SD-14). Forward-only
-- and additive (DESIGN-001 §11). M12 owns 0013.

-- The project conventions document is its own type (SD-13).
ALTER TYPE document_type ADD VALUE IF NOT EXISTS 'conventions';

-- --- What is pushed into prompts (SD-14) ---
--
-- For each accepted decision and conventions revision, the text that is
-- surfaced, recorded when it was accepted. Surfacing reads this, never the
-- file, so a hand edit to an accepted decision can't reach a prompt. For a
-- decision, `ruling` and `reason` are its front-matter fields; `from_title`
-- marks one with no ruling, surfaced by its title (SD-1). For the conventions
-- document, `ruling` holds its body and `reason` is empty.

CREATE TABLE surfaced_texts (
  document_id uuid PRIMARY KEY REFERENCES documents(id) ON DELETE CASCADE,
  ruling      text NOT NULL,
  reason      text NOT NULL DEFAULT '',
  from_title  boolean NOT NULL DEFAULT false,
  recorded_at timestamptz NOT NULL DEFAULT now()
);

-- --- Which decision superseded which (SD-6) ---
--
-- Recorded when the superseding decision is accepted. Rows name document
-- rows: the accepted revision that was superseded, and the revision of the
-- new decision that superseded it.

CREATE TABLE decision_supersessions (
  superseded_id  uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  superseding_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  created_at     timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (superseded_id, superseding_id)
);
CREATE INDEX decision_supersessions_by ON decision_supersessions (superseding_id);
