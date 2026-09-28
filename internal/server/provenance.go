package server

// Who wrote each document and who gave each verdict (SPEC-017 FR-2,
// DESIGN-010 §8 "Everything is attributed", DEC-007 decision 1). The rows are
// written where the acts happen, in the same transaction; this file holds the
// small helpers those places share, and the one place each record becomes a
// sentence, so the document page, the MCP results and the send screen all say
// it the same way (FR-2.4).

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/store"
)

// writerAct is who did a writing act, before it is stored.
type writerAct struct {
	Act        string
	Kind       string
	Actor      string
	Model      string
	DispatchID *uuid.UUID
	Via        string
}

// systemActors are the names Subutai itself acts under.
func systemActor(actor string) bool {
	switch actor {
	case "orchestrator", "lifecycle-engine", "subutai":
		return true
	}
	return false
}

// writerFor says who an actor is, for a writing act whose surface isn't
// otherwise known: the chat agent by its configured name, Subutai by its own
// names, and anyone else a person.
func (s *Server) writerFor(act, actor, via string) writerAct {
	w := writerAct{Act: act, Actor: actor, Via: via}
	switch {
	case actor == s.mcpActor():
		w.Kind, w.Via = store.WriterChat, "mcp"
	case systemActor(actor):
		w.Kind, w.Via = store.WriterSystem, ""
	default:
		w.Kind = store.WriterPerson
	}
	return w
}

// recordWriter stores a writing act in the caller's transaction. A zero act
// records nothing.
func recordWriter(ctx context.Context, tx pgx.Tx, docID uuid.UUID, w writerAct) error {
	if w.Kind == "" {
		return nil
	}
	return store.RecordWriter(ctx, tx, store.Writer{
		DocumentID: docID, Act: w.Act, Kind: w.Kind, Actor: w.Actor,
		Model: w.Model, DispatchID: w.DispatchID, Via: w.Via,
	})
}

// agentVerdict is the verdict record for an agent's review run.
func (s *Server) agentVerdict(ctx context.Context, docID uuid.UUID, verdict, role string, dispatchID uuid.UUID) store.Verdict {
	v := store.Verdict{DocumentID: docID, Verdict: verdict, Kind: store.GiverAgent, Actor: role, DispatchID: &dispatchID}
	if d, err := store.GetDispatch(ctx, s.Store.Pool, dispatchID); err == nil {
		v.Model = d.Model
		if v.Actor == "" {
			v.Actor = d.Role
		}
	}
	return v
}

// personVerdict is the verdict record for a person's act.
func personVerdict(docID uuid.UUID, verdict string, act relayAct) store.Verdict {
	return store.Verdict{DocumentID: docID, Verdict: verdict, Kind: store.GiverPerson,
		Actor: act.Actor, Via: act.Via, Quote: act.Quote}
}

// ---- Saying it ----

// roleWords says a role the way a person would: "spec-author" is "spec
// author".
func roleWords(role string) string {
	return strings.ReplaceAll(strings.ReplaceAll(role, "-", " "), "_", " ")
}

// whoWords names a writer or a verdict's giver.
func whoWords(kind, actor, model string) string {
	switch kind {
	case store.WriterAgent:
		w := "the " + roleWords(actor)
		if model != "" {
			w += " (" + model + ")"
		}
		return w
	case store.WriterChat:
		return "the chat agent"
	case store.WriterSystem:
		return "Subutai"
	}
	return actor
}

const notRecorded = "Who wrote this wasn't recorded."

// writerWho names a writer. A person's act the chat agent carried is the
// person's, and they are unnamed until per-user identity (SPEC-017 SD-5).
func writerWho(w store.Writer) string {
	if w.Kind == store.WriterPerson && w.Via == "mcp" {
		return "a person, relayed by the chat agent"
	}
	return whoWords(w.Kind, w.Actor, w.Model)
}

// writerLead says one writing act.
func writerLead(w store.Writer) string {
	who := writerWho(w)
	switch w.Act {
	case store.ActWrote:
		return "Written by " + who
	case store.ActRevised:
		return "Revised by " + who
	case store.ActAdded:
		// What the chat agent adds, it wrote: its tools tell it to write a
		// file and then add it. A person often adds a file written long
		// before (SPEC-017 SD-4).
		if w.Kind == store.WriterChat {
			return "Written by " + who
		}
		return "Added by " + who
	case store.ActStarted:
		return "Started from the template by " + who
	case store.ActOpenedRevision:
		return "Revision opened by " + who
	}
	return "Written by " + who
}

func sameWriter(a, b store.Writer) bool { return a.Kind == b.Kind && a.Actor == b.Actor }

// writerSentence says who wrote a document, across its revisions (FR-2.4):
// the first act; the latest revision opened after it; and the last writer
// after it who was someone else. ws is oldest first, the document's earlier
// revisions included (writerHistory).
func writerSentence(ws []store.Writer) string {
	if len(ws) == 0 {
		return notRecorded
	}
	lead := ws[0]
	out := writerLead(lead)
	if lead.Inferred {
		out += " (from the audit trail)"
	}
	out += "."
	opened, revised := -1, -1
	for i := 1; i < len(ws); i++ {
		switch ws[i].Act {
		case store.ActOpenedRevision:
			opened = i
		case store.ActWrote, store.ActRevised:
			if !sameWriter(ws[i], lead) {
				revised = i
			}
		}
	}
	if revised >= 0 && revised < opened {
		revised = -1 // a revision since has its own story
	}
	if opened >= 0 {
		out += " " + writerLead(ws[opened]) + "."
	}
	if revised >= 0 {
		out += " Revised by " + writerWho(ws[revised]) + "."
	}
	return out
}

// writerHistory is a document's writing acts with its earlier revisions',
// oldest first, so a revision still says who wrote the first one.
func (s *Server) writerHistory(ctx context.Context, doc store.Document) ([]store.Writer, error) {
	var chain []uuid.UUID
	cur := &doc
	for i := 0; cur != nil && i < 50; i++ {
		chain = append([]uuid.UUID{cur.ID}, chain...)
		if cur.SupersedesID == nil {
			break
		}
		prev, err := store.GetDocument(ctx, s.Store.Pool, *cur.SupersedesID)
		if err != nil {
			break
		}
		cur = prev
	}
	var out []store.Writer
	for _, id := range chain {
		ws, err := store.Writers(ctx, s.Store.Pool, id)
		if err != nil {
			return nil, err
		}
		out = append(out, ws...)
	}
	return out, nil
}

// lastWriter is the writing act the line ends on, for its time.
func lastWriter(ws []store.Writer) *store.Writer {
	if len(ws) == 0 {
		return nil
	}
	return &ws[len(ws)-1]
}

func quotedWords(q string) string { return "“" + q + "”" }

// verdictSentence says who gave a verdict and how (FR-2.4, SD-5).
func verdictSentence(v store.Verdict) string {
	verb := "Approved"
	if v.Verdict == store.VerdictSendBack {
		verb = "Sent back"
	}
	var out string
	switch {
	case v.Kind == store.GiverAgent:
		out = verb + " by " + whoWords(store.WriterAgent, v.Actor, v.Model)
		if v.Held {
			out += ", and held for a person"
		}
		if v.ReleasedBy != "" || v.ReleasedVia != "" {
			switch {
			case v.ReleasedVia == "mcp":
				out += "; a person let the reviewer decide, relayed by the chat agent"
				if v.ReleasedQuote != "" {
					out += ": " + quotedWords(v.ReleasedQuote)
				}
			default:
				out += "; " + v.ReleasedBy + " let the reviewer decide"
			}
		}
	case v.Via == "mcp":
		// A relay is the person's verdict, carried by the chat agent. There
		// is no name for the person until per-user identity (M15).
		out = verb + " by a person, relayed by the chat agent"
		if v.Quote != "" {
			out += ": " + quotedWords(v.Quote)
		}
	case v.Via == "escalation":
		out = verb + " by " + v.Actor + ", answering the reviewer's escalation"
	default:
		out = verb + " by " + v.Actor
	}
	if v.Inferred {
		out += " (from the audit trail)"
	}
	return out + "."
}

// ---- The views ----

// provenanceView is the document page's "written by" include (FR-2.5).
type provenanceView struct {
	Written    string
	WrittenAt  time.Time
	WrittenRun string
	Verdict    string
	VerdictAt  time.Time
	VerdictRun string
}

func runURL(id *uuid.UUID) string {
	if id == nil {
		return ""
	}
	return "/ui/run/" + id.String()
}

// documentProvenance reads a document's writers and latest verdict.
func (s *Server) documentProvenance(ctx context.Context, doc store.Document) (provenanceView, []store.Writer, *store.Verdict, error) {
	ws, err := s.writerHistory(ctx, doc)
	if err != nil {
		return provenanceView{}, nil, nil, err
	}
	v, err := store.LastVerdict(ctx, s.Store.Pool, doc.ID)
	if err != nil && err != store.ErrNotFound {
		return provenanceView{}, nil, nil, err
	}
	p := provenanceView{Written: writerSentence(ws)}
	if lw := lastWriter(ws); lw != nil {
		p.WrittenAt = lw.At
		// The run that wrote it: the latest agent act.
		for i := len(ws) - 1; i >= 0; i-- {
			if ws[i].DispatchID != nil {
				p.WrittenRun = runURL(ws[i].DispatchID)
				break
			}
		}
	}
	if v != nil {
		p.Verdict, p.VerdictAt, p.VerdictRun = verdictSentence(*v), v.At, runURL(v.DispatchID)
	}
	return p, ws, v, nil
}

// mcpWrittenBy is a document entry's written_by (FR-2.6).
func mcpWrittenBy(ws []store.Writer) map[string]any {
	if len(ws) == 0 {
		return map[string]any{"sentence": notRecorded, "recorded": false}
	}
	lead := ws[0]
	out := map[string]any{
		"sentence": writerSentence(ws), "recorded": true,
		"kind": lead.Kind, "who": writerWho(lead), "act": lead.Act,
	}
	if lead.Model != "" {
		out["model"] = lead.Model
	}
	for i := len(ws) - 1; i >= 0; i-- {
		if ws[i].DispatchID != nil {
			out["run_id"] = ws[i].DispatchID.String()
			break
		}
	}
	if lead.Inferred {
		out["inferred"] = true
	}
	return out
}

// mcpLastVerdict is a document entry's last_verdict (FR-2.6).
func mcpLastVerdict(v store.Verdict) map[string]any {
	who := v.Actor
	if v.Kind == store.GiverAgent {
		who = whoWords(store.WriterAgent, v.Actor, v.Model)
	} else if v.Via == "mcp" {
		who = "a person, relayed by the chat agent"
	}
	out := map[string]any{
		"sentence": verdictSentence(v), "verdict": v.Verdict, "kind": v.Kind, "who": who,
		"at": v.At.Format(time.RFC3339),
	}
	if v.Via != "" {
		out["via"] = v.Via
	}
	if v.Quote != "" {
		out["quote"] = v.Quote
	}
	if v.Held {
		out["held"] = true
	}
	if v.DispatchID != nil {
		out["run_id"] = v.DispatchID.String()
	}
	if v.Inferred {
		out["inferred"] = true
	}
	return out
}

// mcpDoc is a document entry with who wrote it and its latest verdict. A
// failure to read them leaves the entry as it was: the document is still
// worth describing.
func (s *Server) mcpDoc(ctx context.Context, d store.Document) map[string]any {
	out := mcpDocEntry(d)
	ws, err := s.writerHistory(ctx, d)
	if err != nil {
		s.Log.Warn("read writers", "doc", d.ID, "err", err)
		return out
	}
	out["written_by"] = mcpWrittenBy(ws)
	if v, err := store.LastVerdict(ctx, s.Store.Pool, d.ID); err == nil {
		out["last_verdict"] = mcpLastVerdict(*v)
	}
	return out
}

// mcpDocs is mcpDoc over a list.
func (s *Server) mcpDocs(ctx context.Context, docs []store.Document) []any {
	out := make([]any, 0, len(docs))
	for _, d := range docs {
		out = append(out, s.mcpDoc(ctx, d))
	}
	return out
}

// doneBy says who did an already-done send step (FR-5.1): "by the chat agent"
// or "".
func doneBy(ws []store.Writer) string {
	if len(ws) == 0 {
		return ""
	}
	lead := ws[0]
	if lead.Act == store.ActAdded && lead.Kind != store.WriterChat {
		return fmt.Sprintf(" (added by %s)", writerWho(lead))
	}
	return " by " + writerWho(lead)
}

// ---- Documents older than migration 0011 (SPEC-017 SD-7, FR-2.8) ----

// BackfillProvenance reads back, from the audit trail, who added each
// document registered before migration 0011 and who gave each person's
// verdict on one, where the migration's SQL couldn't: telling the chat agent
// from a person needs the configured actor names. It runs when the server
// starts, touches only documents older than 0011 with nothing recorded, and
// marks every row it writes as inferred. A role's act it can't tie to a run is
// left unrecorded rather than guessed.
func (s *Server) BackfillProvenance(ctx context.Context) error {
	var cutoff *time.Time
	if err := s.Store.Pool.QueryRow(ctx,
		`SELECT applied_at FROM schema_migrations WHERE version = 11`).Scan(&cutoff); err != nil || cutoff == nil {
		return err
	}
	roles := map[string]bool{}
	rows, err := s.Store.Pool.Query(ctx, `SELECT DISTINCT role FROM dispatches`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var r string
		if err := rows.Scan(&r); err != nil {
			rows.Close()
			return err
		}
		roles[r] = true
	}
	rows.Close()
	chat := s.mcpActor()
	classify := func(actor string) string {
		switch {
		case actor == chat:
			return store.WriterChat
		case systemActor(actor):
			return store.WriterSystem
		case roles[actor], actor == "", actor == "unknown":
			return ""
		}
		return store.WriterPerson
	}

	// Writers: the first registration of each older document with none.
	type reg struct {
		doc     uuid.UUID
		actor   string
		kind    string
		at      time.Time
		started bool
	}
	var regs []reg
	rows, err = s.Store.Pool.Query(ctx, `
		SELECT DISTINCT ON (ae.ref_id) ae.ref_id, ae.actor, ae.kind, ae.occurred_at,
		       EXISTS (SELECT 1 FROM audit_events st WHERE st.kind = 'document.started'
		               AND st.ref_type = 'document' AND st.ref_id = ae.ref_id)
		FROM audit_events ae
		JOIN documents d ON d.id = ae.ref_id
		WHERE ae.ref_type = 'document'
		  AND ae.kind IN ('document.registered', 'document.revision_created')
		  AND d.created_at < $1
		  AND NOT EXISTS (SELECT 1 FROM document_writers w WHERE w.document_id = ae.ref_id)
		ORDER BY ae.ref_id, ae.occurred_at, ae.id`, *cutoff)
	if err != nil {
		return err
	}
	for rows.Next() {
		var g reg
		if err := rows.Scan(&g.doc, &g.actor, &g.kind, &g.at, &g.started); err != nil {
			rows.Close()
			return err
		}
		regs = append(regs, g)
	}
	rows.Close()

	// Verdicts: an approval or send-back before 0011 by someone who isn't a
	// role, with nothing recorded at that moment.
	type tr struct {
		doc     uuid.UUID
		actor   string
		event   string
		via     *string
		quote   *string
		at      time.Time
		escalID *string
	}
	var trs []tr
	rows, err = s.Store.Pool.Query(ctx, `
		SELECT ae.ref_id, ae.actor, ae.payload->>'event', ae.payload->>'via', ae.payload->>'quote', ae.occurred_at,
		       (SELECT c.id::text FROM audit_events c
		         WHERE c.kind = 'checkpoint.responded' AND c.ref_type = 'document' AND c.ref_id = ae.ref_id
		           AND c.actor = ae.actor AND c.payload->>'kind' = 'review-escalation'
		           AND c.occurred_at BETWEEN ae.occurred_at - interval '10 minutes' AND ae.occurred_at
		         ORDER BY c.occurred_at DESC LIMIT 1)
		FROM audit_events ae
		JOIN documents d ON d.id = ae.ref_id
		WHERE ae.ref_type = 'document' AND ae.kind = 'document.transition'
		  AND ae.payload->>'event' IN ('approve', 'request_changes')
		  AND ae.occurred_at < $1
		  AND NOT EXISTS (SELECT 1 FROM document_verdicts v WHERE v.document_id = ae.ref_id AND v.at = ae.occurred_at)
		ORDER BY ae.occurred_at, ae.id`, *cutoff)
	if err != nil {
		return err
	}
	for rows.Next() {
		var t tr
		if err := rows.Scan(&t.doc, &t.actor, &t.event, &t.via, &t.quote, &t.at, &t.escalID); err != nil {
			rows.Close()
			return err
		}
		trs = append(trs, t)
	}
	rows.Close()

	if len(regs) == 0 && len(trs) == 0 {
		return nil
	}
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		for _, g := range regs {
			kind := classify(g.actor)
			if kind == "" {
				continue
			}
			act := store.ActAdded
			switch {
			case g.kind == "document.revision_created":
				act = store.ActOpenedRevision
			case g.started:
				act = store.ActStarted
			}
			via := ""
			if kind == store.WriterChat {
				via = "mcp"
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO document_writers (id, document_id, act, writer_kind, actor, via, inferred, at)
				VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), true, $7)`,
				store.NewID(), g.doc, act, kind, g.actor, via, g.at); err != nil {
				return err
			}
		}
		for _, t := range trs {
			kind := classify(t.actor)
			via, quote := "", ""
			if t.via != nil {
				via = *t.via
			}
			if t.quote != nil {
				quote = *t.quote
			}
			switch {
			case kind == store.WriterChat && via == "mcp" && quote != "":
				// A relayed send-back of a spec or plan: the person's.
			case kind == store.WriterPerson:
				if via == "" && t.escalID != nil {
					via = "escalation"
				}
			default:
				continue
			}
			verdict := store.VerdictApprove
			if t.event == "request_changes" {
				verdict = store.VerdictSendBack
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO document_verdicts (id, document_id, verdict, giver_kind, actor, via, quote, inferred, at)
				VALUES ($1, $2, $3, 'person', $4, NULLIF($5, ''), NULLIF($6, ''), true, $7)`,
				store.NewID(), t.doc, verdict, t.actor, via, quote, t.at); err != nil {
				return err
			}
		}
		return nil
	})
}
