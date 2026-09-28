package server

// The chat relay tools (SPEC-011 FR-8, DEC-006 Amendment 1 decision 8). The
// chat agent carries a person's decision here and never makes one of its own:
// every tool takes the person's words, quoted, and records them with the act
// on the audit trail, `via: mcp`. Each calls the same service method the
// document page calls, as the MCP actor.
//
// The fifth relay, relay_tick_job (SPEC-014 FR-7.9), carries a person's word
// that a job on a checklist is done, or not done after all. It is the only way
// the chat agent can tick or untick a job.
//
// submit_for_review (SPEC-017 FR-1) lives here too, beside the relays, but is
// not one: it hands the chat agent's own work to the reviewer, which DEC-005
// counts as asking the orchestrator to do what it would do anyway. It takes no
// quote, and it gives no verdict; the reviewer decides (SD-1, Sam's choice).
//
// What is NOT here is the boundary (DEC-006 Amendment 1, DESIGN-010 §5c): no
// tool sends to development, withdraws a send, starts building, overrides a
// gate or answers a checkpoint, and none holds a verdict of the agent's own.
// (Marking a milestone as shipped is planning, not a relay, under DEC-004
// Amendment 1; it lives with the milestone tools.) Adding a relay tool that
// isn't on Amendment 1's list needs a decision, and
// TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet fails on anything unnamed.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"

	"subutai/internal/ident"
	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// docProp is the document argument every relay and submit_for_review share.
var docProp = stringProp("The document's path in the repository, as list_documents gives it, or its ID, such as \"FEAT-003-spec\" or \"DEC-005\".")

// quoteProp is the one argument every relay shares.
var quoteProp = stringProp("Required. The person's own words that asked for this, quoted exactly as they said them. " +
	"It is stored with the act, so anyone reading the record can see what was actually said.")

func (s *Server) mcpRelayTools() []mcpTool {
	return []mcpTool{
		{
			Name: "submit_for_review",
			Description: "Submit a draft you have written — a specification, a dev-plan or a design — for review, " +
				"once the person and you think it's ready. It is checked against its template first; if it fails, " +
				"you are told what to fix. A specification or dev-plan then goes to its independent agent reviewer, " +
				"whose verdict is its own, never yours; a design waits for a person to approve it. Submitting " +
				"doesn't send anything to development: a person does that from the command centre. For a document " +
				"already in review, a fresh review is the person's call: relay it with relay_review_request.",
			Schema: objectSchema(map[string]any{
				"document": docProp,
			}, "document"),
			Handler: s.mcpSubmitForReview,
		},
		{
			Name: "relay_verdict",
			Description: "Carry a person's verdict on a document in review: approve it, or send it back. Use this only " +
				"when the person has told you their decision, and quote their words. You have no verdict of your own " +
				"and must never approve or send back on your own judgement. Sending back a specification or dev-plan " +
				"records the reason as an issue its reviewer must see answered. A document whose review is waiting in " +
				"the Inbox is decided there, not here.",
			Schema: objectSchema(map[string]any{
				"path":    docProp,
				"verdict": stringProp("\"approve\" or \"send_back\"."),
				"reason":  stringProp("For a send-back: what must change, in the person's terms."),
				"quote":   quoteProp,
			}, "path", "verdict", "quote"),
			Handler: s.mcpRelayVerdict,
		},
		{
			Name: "relay_issue",
			Description: "Carry an issue a person raised on a document: something they say is wrong or missing. " +
				"An issue must be dealt with — the reviewer can't approve until it has said how the issue was " +
				"addressed, or why it doesn't apply. On a feature that has been sent to development it goes to the " +
				"author at once; on an approved specification it opens a revision. Quote the person's words.",
			Schema: objectSchema(map[string]any{
				"path":    docProp,
				"issue":   stringProp("The issue, written clearly for the author and the reviewer."),
				"section": stringProp("Optional. The heading of the section it is about."),
				"quote":   quoteProp,
			}, "path", "issue", "quote"),
			Handler: s.mcpRelayIssue,
		},
		{
			Name: "relay_review_request",
			Description: "Carry a person's request for a fresh agent review of a specification or dev-plan already " +
				"in review — for example when agent review is switched off for the project, or a review failed. " +
				"To hand in a draft you wrote, use submit_for_review instead; this relay still submits a draft " +
				"when the person asks for it in their own words. The independent reviewer decides, not you. " +
				"Designs have no agent reviewer: for a cold read of a design, use the review-design chat skill. " +
				"Quote the person's words.",
			Schema: objectSchema(map[string]any{
				"path":  docProp,
				"quote": quoteProp,
			}, "path", "quote"),
			Handler: s.mcpRelayReviewRequest,
		},
		{
			Name: "relay_release_hold",
			Description: "Carry a person's \"let the reviewer decide\" on a specification held for them: the agent " +
				"reviewer's approval then stands. This is only possible once the reviewer has approved it and no " +
				"issue is open, and not when agent review is switched off for the project. Quote the person's words.",
			Schema: objectSchema(map[string]any{
				"path":  docProp,
				"quote": quoteProp,
			}, "path", "quote"),
			Handler: s.mcpRelayRelease,
		},
		{
			Name: "relay_tick_job",
			Description: "Carry a person's word that a job on a checklist is done, which ticks it, or that it " +
				"isn't done after all, which unticks it. Use this only when the person has told you, and quote " +
				"their words: the tick is recorded as theirs, relayed by you, with the quote beside it. Never " +
				"tick a job on your own judgement. A note, if given, replaces the job's note.",
			Schema: objectSchema(map[string]any{
				"checklist": stringProp("The checklist's ID, such as \"CL-002\", or its exact name if no other checklist shares it."),
				"job":       stringProp("The job's id, or its exact title if no other job on this checklist shares it."),
				"ticked":    map[string]any{"type": "boolean", "description": "true to tick the job (the person says it is done), false to untick it (they say it isn't)."},
				"note":      stringProp("Optional. Something the person said about it, such as where the key is kept."),
				"quote":     quoteProp,
			}, "checklist", "job", "ticked", "quote"),
			Handler: s.mcpRelayTickJob,
		},
	}
}

// mcpRelayTickJob ticks or unticks a job on a person's word, as the MCP actor,
// with via: mcp and the quote stored on the job and on the audit row (SPEC-014
// FR-7.9). It calls the same TickJob the checklist page calls.
func (s *Server) mcpRelayTickJob(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	quote, ok := argString(args, "quote")
	if !ok {
		return nil, errors.New("the person's words are required in quote: a relay carries what they said, " +
			"and without it there is nothing to relay. If they haven't told you to do this, don't")
	}
	tick, ok := args["ticked"].(bool)
	if !ok {
		return nil, errors.New("say whether the job is done in \"ticked\": true to tick it, false to untick it")
	}
	c, j, err := s.mcpJobArg(ctx, args)
	if err != nil {
		return nil, err
	}
	note, _ := argString(args, "note")
	var done *store.Job
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		done, e = store.TickJob(ctx, tx, j.ID, tick, note, "mcp", quote, s.mcpActor())
		return e
	}); err != nil {
		return nil, mcpChecklistError(err)
	}
	s.notifyEntityChanged("checklist", c.ID)
	st, err := store.GetChecklistStatus(ctx, s.Store.Pool, c.ID)
	if err != nil {
		return nil, err
	}
	verb := "ticked"
	if !tick {
		verb = "unticked"
	}
	return map[string]any{
		"done": fmt.Sprintf("recorded the person's word: %s is %s", done.Title, verb),
		"job":  s.mcpJobSummary(*done),
		"checklist": map[string]any{"id": c.ID.String(), "name": c.Name,
			"jobs_ticked": st.Ticked, "jobs_total": st.Jobs, "done": st.Done()},
		"url": checklistURL(c.ID),
	}, nil
}

// relayTarget resolves the document and the person's quote every relay needs.
func (s *Server) relayTarget(r *http.Request, args map[string]any) (*store.Document, relayAct, error) {
	ref, ok := argString(args, "path")
	if !ok {
		return nil, relayAct{}, errors.New("the document's path or ID is required; call list_documents to find it")
	}
	quote, ok := argString(args, "quote")
	if !ok {
		return nil, relayAct{}, errors.New("the person's words are required in quote: a relay carries what they said, " +
			"and without it there is nothing to relay. If they haven't told you to do this, don't")
	}
	doc, err := s.documentByRef(r.Context(), ref)
	if err != nil {
		return nil, relayAct{}, err
	}
	return doc, relayAct{Actor: s.mcpActor(), Via: "mcp", Quote: quote}, nil
}

// documentByRef finds the document a relay or submit_for_review names: by
// its path, or by its ID (SPEC-017 FR-3.1). An ID names its newest revision
// that isn't superseded — the one in review while a revision is open —
// because a person's verdict or issue is about the text in front of them.
// "FEAT-003-spec.r2" names that revision exactly.
func (s *Server) documentByRef(ctx context.Context, ref string) (*store.Document, error) {
	if doc, err := store.LiveDocumentByPath(ctx, s.Store.Pool, ref); err == nil {
		return doc, nil
	} else if err != store.ErrNotFound {
		return nil, err
	}
	if id, ok := ident.Parse(ref); ok && (id.Shape == ident.ShapeDocument || id.Shape == ident.ShapeDecision) {
		var doc *store.Document
		var err error
		if id.Revision > 0 {
			doc, err = store.DocumentByIdentity(ctx, s.Store.Pool, id.ID, id.Revision)
		} else {
			doc, err = store.LiveDocumentByPublicID(ctx, s.Store.Pool, id.ID)
		}
		if err == nil {
			if doc.State == lifecycle.DocSuperseded {
				return nil, fmt.Errorf("%s revision %d has been superseded; name the current one, %s", id.ID, doc.Revision, id.ID)
			}
			return doc, nil
		}
		if err != store.ErrNotFound {
			return nil, err
		}
	}
	return nil, fmt.Errorf("there is no current document at or called %q; call list_documents to see what exists", ref)
}

// mcpSubmitForReview is submit_for_review (SPEC-017 FR-1): SubmitDoc, the
// document page's Submit, as the chat agent, with no quote (SD-1). The
// refusals are the page's, in the page's words.
func (s *Server) mcpSubmitForReview(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	ref, ok := argString(args, "document")
	if !ok {
		return nil, errors.New("say which document to submit, by its path or its ID, in \"document\"")
	}
	doc, err := s.documentByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	next, err := s.SubmitFromChat(ctx, doc)
	if err != nil {
		return nil, err
	}
	return s.relayResult(r, doc, next)
}

func (s *Server) mcpRelayVerdict(r *http.Request, args map[string]any) (any, error) {
	doc, act, err := s.relayTarget(r, args)
	if err != nil {
		return nil, err
	}
	verdict, _ := argString(args, "verdict")
	switch verdict {
	case "approve":
		if err := s.DirectApprove(r.Context(), doc.ID, act); err != nil {
			return nil, err
		}
	case "send_back":
		reason, _ := argString(args, "reason")
		if err := s.SendBack(r.Context(), doc.ID, reason, act); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("the verdict must be \"approve\" or \"send_back\"")
	}
	s.notifyEntityChanged("document", doc.ID)
	return s.relayResult(r, doc, "recorded the person's verdict: "+verdict)
}

func (s *Server) mcpRelayIssue(r *http.Request, args map[string]any) (any, error) {
	doc, act, err := s.relayTarget(r, args)
	if err != nil {
		return nil, err
	}
	issue, _ := argString(args, "issue")
	section, _ := argString(args, "section")
	on, err := s.RaiseIssue(r.Context(), doc.ID, issue, section, act)
	if err != nil {
		return nil, err
	}
	if on != nil && on.ID != doc.ID {
		return s.relayResult(r, on, "the document was approved, so the issue opened this revision of it and is recorded there")
	}
	if s.humanApprovalType(doc.Type) {
		return s.relayResult(r, doc, "recorded the issue as a note for the person who approves this document")
	}
	return s.relayResult(r, doc, "recorded the issue; the reviewer can't approve until it is answered")
}

func (s *Server) mcpRelayReviewRequest(r *http.Request, args map[string]any) (any, error) {
	doc, act, err := s.relayTarget(r, args)
	if err != nil {
		return nil, err
	}
	msg, err := s.RequestReview(r.Context(), doc.ID, act)
	if err != nil {
		return nil, err
	}
	return s.relayResult(r, doc, msg)
}

func (s *Server) mcpRelayRelease(r *http.Request, args map[string]any) (any, error) {
	doc, act, err := s.relayTarget(r, args)
	if err != nil {
		return nil, err
	}
	if err := s.ReleaseHold(r.Context(), doc.ID, act); err != nil {
		return nil, err
	}
	return s.relayResult(r, doc, "the reviewer's approval stands")
}

// relayResult reports the document as it now is, so the agent can tell the
// person what happened.
func (s *Server) relayResult(r *http.Request, doc *store.Document, done string) (any, error) {
	fresh, err := store.GetDocument(r.Context(), s.Store.Pool, doc.ID)
	if err != nil {
		fresh = doc
	}
	out := s.mcpDoc(r.Context(), *fresh)
	out["done"] = done
	return out, nil
}
