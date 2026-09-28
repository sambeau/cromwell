-- 0007_send_to_development.sql — Send to development (SPEC-011, DEC-006 with
-- Amendment 1). Forward-only and additive (DESIGN-001 §11).

-- --- The sent mark (SPEC-011 FR-1, SD-1) ---
--
-- A mark, not a lifecycle state: a sent feature is still `idea` until its
-- contract is approved. One row per feature while it is sent; Withdraw deletes
-- the row and the audit trail keeps the history. `hold` is this send's hold
-- for a person after the spec review (FR-5.2), fixed when the send was made.

CREATE TABLE feature_sends (
  feature_id uuid PRIMARY KEY REFERENCES features(id),
  sent_by    text NOT NULL,
  sent_at    timestamptz NOT NULL DEFAULT now(),
  hold       boolean NOT NULL DEFAULT false
);

-- --- Held documents (SPEC-011 FR-5.3, FR-5.4) ---
--
-- A spec in `reviewing` that waits for a person. dispatch_id is the agent
-- review whose approval is being held; NULL when agent review is switched off
-- and no agent has given a verdict, which is why "let the reviewer decide" is
-- then not offered.

CREATE TABLE document_holds (
  document_id uuid PRIMARY KEY REFERENCES documents(id),
  dispatch_id uuid REFERENCES dispatches(id),
  created_at  timestamptz NOT NULL DEFAULT now()
);

-- --- Human issues (SPEC-011 FR-6, SD-6) ---
--
-- An issue is a comment a person raised, always must-address. `via` is the
-- channel it came by and `quote` the person's words when the chat agent
-- relayed it. An issue is addressed when a reviewer answers it (addressed or
-- does_not_apply, with a note) or a person approves the document directly;
-- `resolved` is then set and the addressed_* columns say by whom and why.

ALTER TABLE document_comments
  ADD COLUMN is_issue              boolean NOT NULL DEFAULT false,
  ADD COLUMN via                   text CHECK (via IN ('ui', 'mcp')),
  ADD COLUMN quote                 text,
  ADD COLUMN addressed_by          text,
  ADD COLUMN addressed_dispatch_id uuid REFERENCES dispatches(id),
  ADD COLUMN addressed_note        text,
  ADD COLUMN addressed_at          timestamptz;

CREATE INDEX document_comments_open_issues ON document_comments (document_id)
  WHERE is_issue AND NOT resolved;
