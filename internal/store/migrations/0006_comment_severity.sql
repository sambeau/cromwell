-- C-1a (audit §3.3a): severity is enforced on both review paths but was
-- recorded on only one. A document comment now carries the severity its
-- finding had — major sent the document back, minor rode along — and NULL
-- for comments that are not classified findings at all: a design reviewer's
-- comments (which gate nothing) and a human's reasons.
ALTER TABLE document_comments ADD COLUMN severity text
  CHECK (severity IN ('major', 'minor'));
