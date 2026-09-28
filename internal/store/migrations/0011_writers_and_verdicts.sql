-- 0011_writers_and_verdicts.sql — who wrote each document, and who gave each
-- verdict (SPEC-017 FR-2, DESIGN-010 §8, DEC-007 decision 1). Forward-only
-- and additive (DESIGN-001 §11). M9 owns 0012.

-- --- Writers (SPEC-017 SD-3) ---
--
-- One row per writing act Subutai sees: an author agent writing or revising a
-- document, a document added by attach or adopt, a starter design, a revision
-- opened by Revise. Edits made to the file afterwards aren't seen, so aren't
-- here. `inferred` marks a row this migration read back from the audit trail
-- (SD-7). Rows go with the document when a draft is detached.

CREATE TABLE document_writers (
  id          uuid PRIMARY KEY,
  document_id uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  act         text NOT NULL CHECK (act IN ('wrote', 'revised', 'added', 'started', 'opened_revision')),
  writer_kind text NOT NULL CHECK (writer_kind IN ('agent', 'chat', 'person', 'system')),
  actor       text NOT NULL,
  model       text,
  dispatch_id uuid REFERENCES dispatches(id),
  via         text,
  inferred    boolean NOT NULL DEFAULT false,
  at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX document_writers_doc ON document_writers (document_id, at, id);

-- --- Verdicts (SPEC-017 SD-5, SD-6) ---
--
-- One row per verdict on a document: an agent's (with the review run behind
-- it) or a person's (with how it came: the web UI, the Inbox, or relayed from
-- chat with the person's words). A relayed verdict is the person's; `actor`
-- is then the MCP actor that carried it, since there is no per-user identity
-- yet (M15). `held` marks an agent approval held for a person; `released_*`
-- says who let the reviewer decide.

CREATE TABLE document_verdicts (
  id             uuid PRIMARY KEY,
  document_id    uuid NOT NULL REFERENCES documents(id) ON DELETE CASCADE,
  verdict        text NOT NULL CHECK (verdict IN ('approve', 'send_back')),
  held           boolean NOT NULL DEFAULT false,
  giver_kind     text NOT NULL CHECK (giver_kind IN ('agent', 'person')),
  actor          text NOT NULL,
  model          text,
  dispatch_id    uuid REFERENCES dispatches(id),
  via            text,
  quote          text,
  released_by    text,
  released_via   text,
  released_quote text,
  inferred       boolean NOT NULL DEFAULT false,
  at             timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT document_verdicts_relay_quoted CHECK (via IS DISTINCT FROM 'mcp' OR quote IS NOT NULL),
  CONSTRAINT document_verdicts_agent_run CHECK (giver_kind <> 'agent' OR dispatch_id IS NOT NULL)
);
CREATE INDEX document_verdicts_doc ON document_verdicts (document_id, at, id);

-- --- Reading back what the trail already says (SD-7, FR-2.8) ---
--
-- Here, only what the trail pins down: an author agent (its run), Subutai's
-- own revisions, an agent's verdict (its run), a held approval, a release, and
-- a person's verdict where a document.human_verdict or an escalation answer
-- sits just before it. Telling the chat agent from a person needs the
-- configured actor names, which SQL can't read, so the server does that part
-- when it starts (BackfillProvenance), for documents older than this
-- migration.

-- The first registration row of each document: who put it in.
CREATE TEMPORARY TABLE m11_first_reg ON COMMIT DROP AS
  SELECT DISTINCT ON (ae.ref_id)
         ae.ref_id AS document_id, ae.actor, ae.kind, ae.occurred_at
  FROM audit_events ae
  WHERE ae.ref_type = 'document'
    AND ae.kind IN ('document.registered', 'document.revision_created')
  ORDER BY ae.ref_id, ae.occurred_at, ae.id;

-- An author agent: the registering actor is the role of an authoring run on
-- the document's feature that finished shortly before.
INSERT INTO document_writers (id, document_id, act, writer_kind, actor, model, dispatch_id, via, inferred, at)
SELECT gen_random_uuid(), r.document_id, 'wrote', 'agent', r.actor, d.model, d.id, 'agent', true, r.occurred_at
FROM m11_first_reg r
JOIN documents doc ON doc.id = r.document_id
JOIN LATERAL (
  SELECT dp.id, dp.model FROM dispatches dp
  WHERE dp.purpose IN ('write-spec', 'write-dev-plan')
    AND dp.state = 'succeeded'
    AND dp.role = r.actor
    AND dp.ref_type = 'feature' AND dp.ref_id = doc.owner_id
    AND dp.finished_at BETWEEN r.occurred_at - interval '10 minutes' AND r.occurred_at + interval '5 seconds'
  ORDER BY dp.finished_at DESC
  LIMIT 1
) d ON true
WHERE r.kind = 'document.registered' AND doc.owner_type = 'feature';

-- Subutai itself: a revision the cascade opened.
INSERT INTO document_writers (id, document_id, act, writer_kind, actor, inferred, at)
SELECT gen_random_uuid(), r.document_id, 'opened_revision', 'system', r.actor, true, r.occurred_at
FROM m11_first_reg r
WHERE r.kind = 'document.revision_created'
  AND r.actor IN ('lifecycle-engine', 'orchestrator', 'subutai')
  AND NOT EXISTS (SELECT 1 FROM document_writers w WHERE w.document_id = r.document_id);

-- Verdicts, from each approval or send-back transition.
CREATE TEMPORARY TABLE m11_verdicts ON COMMIT DROP AS
  SELECT ae.id AS audit_id, ae.ref_id AS document_id, ae.actor, ae.occurred_at,
         CASE ae.payload->>'event' WHEN 'approve' THEN 'approve' ELSE 'send_back' END AS verdict,
         doc.type AS doc_type
  FROM audit_events ae
  JOIN documents doc ON doc.id = ae.ref_id
  WHERE ae.ref_type = 'document' AND ae.kind = 'document.transition'
    AND ae.payload->>'event' IN ('approve', 'request_changes');

-- An agent's verdict: its actor is the role of a review run on the document
-- that finished at or before it. A release (document.hold_released) shortly
-- before names the person who let the reviewer decide.
INSERT INTO document_verdicts (id, document_id, verdict, giver_kind, actor, model, dispatch_id,
                               released_by, released_via, released_quote, inferred, at)
SELECT gen_random_uuid(), v.document_id, v.verdict, 'agent', v.actor, d.model, d.id,
       rel.actor, rel.payload->>'via', rel.payload->>'quote', true, v.occurred_at
FROM m11_verdicts v
JOIN LATERAL (
  SELECT dp.id, dp.model FROM dispatches dp
  WHERE dp.purpose = 'review-' || v.doc_type
    AND dp.state = 'succeeded' AND dp.role = v.actor
    AND dp.ref_type = 'document' AND dp.ref_id = v.document_id
    AND dp.finished_at <= v.occurred_at + interval '5 seconds'
  ORDER BY dp.finished_at DESC
  LIMIT 1
) d ON true
LEFT JOIN LATERAL (
  SELECT h.actor, h.payload FROM audit_events h
  WHERE h.kind = 'document.hold_released' AND h.ref_type = 'document' AND h.ref_id = v.document_id
    AND h.occurred_at BETWEEN v.occurred_at - interval '1 minute' AND v.occurred_at
  ORDER BY h.occurred_at DESC
  LIMIT 1
) rel ON true;

-- A person's verdict: a document.human_verdict row by the same actor just
-- before, which says how it came and carries a relay's words; or an answer
-- to a review escalation.
INSERT INTO document_verdicts (id, document_id, verdict, giver_kind, actor, via, quote, inferred, at)
SELECT gen_random_uuid(), v.document_id, v.verdict, 'person', v.actor,
       COALESCE(hv.payload->>'via', CASE WHEN cr.id IS NOT NULL THEN 'escalation' END),
       hv.payload->>'quote', true, v.occurred_at
FROM m11_verdicts v
LEFT JOIN LATERAL (
  SELECT h.payload FROM audit_events h
  WHERE h.kind = 'document.human_verdict' AND h.ref_type = 'document' AND h.ref_id = v.document_id
    AND h.actor = v.actor
    AND h.occurred_at BETWEEN v.occurred_at - interval '10 minutes' AND v.occurred_at
  ORDER BY h.occurred_at DESC
  LIMIT 1
) hv ON true
LEFT JOIN LATERAL (
  SELECT c.id FROM audit_events c
  WHERE c.kind = 'checkpoint.responded' AND c.ref_type = 'document' AND c.ref_id = v.document_id
    AND c.actor = v.actor AND c.payload->>'kind' = 'review-escalation'
    AND c.occurred_at BETWEEN v.occurred_at - interval '10 minutes' AND v.occurred_at
  ORDER BY c.occurred_at DESC
  LIMIT 1
) cr ON true
WHERE (hv.payload IS NOT NULL OR cr.id IS NOT NULL)
  AND (hv.payload IS NULL OR hv.payload->>'via' IS DISTINCT FROM 'mcp' OR hv.payload->>'quote' IS NOT NULL)
  AND NOT EXISTS (SELECT 1 FROM document_verdicts x
                  WHERE x.document_id = v.document_id AND x.at = v.occurred_at AND x.inferred);

-- An agent's approval held for a person (SPEC-011 FR-5.4): document.held
-- names the review run whose approval waits.
INSERT INTO document_verdicts (id, document_id, verdict, held, giver_kind, actor, model, dispatch_id, inferred, at)
SELECT gen_random_uuid(), ae.ref_id, 'approve', true, 'agent', dp.role, dp.model, dp.id, true, ae.occurred_at
FROM audit_events ae
JOIN dispatches dp ON dp.id = (ae.payload->>'dispatch_id')::uuid
WHERE ae.kind = 'document.held' AND ae.ref_type = 'document'
  AND ae.payload ? 'dispatch_id'
  AND EXISTS (SELECT 1 FROM documents d WHERE d.id = ae.ref_id);

-- A person's approval recorded on adoption, or with "This was already
-- approved" (SPEC-015 FR-5.3, FR-5.8): a document.human_verdict with no
-- transition behind it.
INSERT INTO document_verdicts (id, document_id, verdict, giver_kind, actor, via, inferred, at)
SELECT gen_random_uuid(), ae.ref_id, 'approve', 'person', ae.actor, ae.payload->>'via', true, ae.occurred_at
FROM audit_events ae
WHERE ae.kind = 'document.human_verdict' AND ae.ref_type = 'document'
  AND ae.payload ? 'already_approved'
  AND ae.payload->>'via' IS DISTINCT FROM 'mcp'
  AND EXISTS (SELECT 1 FROM documents d WHERE d.id = ae.ref_id);
