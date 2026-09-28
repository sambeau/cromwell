package server

// The chat relay tools (SPEC-011 FR-8, DEC-006 Amendment 1 decision 8). The
// chat agent carries a person's decision here and never makes one of its own:
// every tool takes the person's words, quoted, and records them with the act
// on the audit trail, `via: mcp`. Each calls the same service method the
// document page calls, as the MCP actor.
//
// What is NOT here is the boundary (DEC-006 Amendment 1, DESIGN-010 §5c): no
// tool sends to development, withdraws a send, starts building, locks a
// milestone, overrides a gate or answers a checkpoint, and none holds a
// verdict of the agent's own. Adding a relay tool that isn't on Amendment 1's
// list needs a decision, and TestMCPAdvertisedToolSetIsExactlyTheAuthoringSet
// fails on anything unnamed.

import (
	"errors"
	"fmt"
	"net/http"

	"cromwell/internal/store"
)

// quoteProp is the one argument every relay shares.
var quoteProp = stringProp("Required. The person's own words that asked for this, quoted exactly as they said them. " +
	"It is stored with the act, so anyone reading the record can see what was actually said.")

func (s *Server) mcpRelayTools() []mcpTool {
	return []mcpTool{
		{
			Name: "relay_verdict",
			Description: "Carry a person's verdict on a document in review: approve it, or send it back. Use this only " +
				"when the person has told you their decision, and quote their words. You have no verdict of your own " +
				"and must never approve or send back on your own judgement. Sending back a specification or dev-plan " +
				"records the reason as an issue its reviewer must see answered. A document whose review is waiting in " +
				"the Inbox is decided there, not here.",
			Schema: objectSchema(map[string]any{
				"path":    stringProp("The document's path in the repository, as list_documents gives it."),
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
				"path":    stringProp("The document's path in the repository."),
				"issue":   stringProp("The issue, written clearly for the author and the reviewer."),
				"section": stringProp("Optional. The heading of the section it is about."),
				"quote":   quoteProp,
			}, "path", "issue", "quote"),
			Handler: s.mcpRelayIssue,
		},
		{
			Name: "relay_review_request",
			Description: "Carry a person's request for an agent review of a specification or dev-plan. A draft is " +
				"submitted, which queues its review; a document already in review gets a fresh review. The " +
				"independent reviewer decides, not you. Designs have no agent reviewer: for a cold read of a design, " +
				"use the review-design chat skill. Quote the person's words.",
			Schema: objectSchema(map[string]any{
				"path":  stringProp("The document's path in the repository."),
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
				"path":  stringProp("The specification's path in the repository."),
				"quote": quoteProp,
			}, "path", "quote"),
			Handler: s.mcpRelayRelease,
		},
	}
}

// relayTarget resolves the document and the person's quote every relay needs.
func (s *Server) relayTarget(r *http.Request, args map[string]any) (*store.Document, relayAct, error) {
	path, ok := argString(args, "path")
	if !ok {
		return nil, relayAct{}, errors.New("the document's path is required; call list_documents to find it")
	}
	quote, ok := argString(args, "quote")
	if !ok {
		return nil, relayAct{}, errors.New("the person's words are required in quote: a relay carries what they said, " +
			"and without it there is nothing to relay. If they haven't told you to do this, don't")
	}
	doc, err := store.LiveDocumentByPath(r.Context(), s.Store.Pool, path)
	if err != nil {
		return nil, relayAct{}, fmt.Errorf("there is no current document at %q; call list_documents to see what exists", path)
	}
	return doc, relayAct{Actor: s.mcpActor(), Via: "mcp", Quote: quote}, nil
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
	return map[string]any{
		"done": done, "path": fresh.Path, "state": string(fresh.State),
		"url": "/ui/d/" + fresh.Path,
	}, nil
}
