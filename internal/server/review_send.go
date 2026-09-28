package server

// Send to development and spec review under DEC-006 Amendment 1 (SPEC-011).
//
// Every act here is a service method the web UI and the MCP relay tools share
// (NFR-1): each runs in one transaction with its audit row (O-3), and the
// surfaces only carry forms and arguments to it. A relay passes via "mcp" and
// the human's quoted words, which go on the audit row beside the act; the web
// UI passes via "ui" and no quote.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/bus"
	"subutai/internal/config"
	"subutai/internal/lifecycle"
	"subutai/internal/rules"
	"subutai/internal/store"
)

// relayAct is who acted and how, carried onto every audit row this file writes.
type relayAct struct {
	Actor string
	Via   string // "ui" | "mcp"
	Quote string // the human's words, for a relay
}

func (a relayAct) payload(extra map[string]any) map[string]any {
	p := map[string]any{"via": a.Via}
	if a.Quote != "" {
		p["quote"] = a.Quote
	}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

// ---- Snapshot inputs ----

// specHeld reports whether an agent approval of this document would be held
// for a person (FR-5.2). Only specs are held (FR-5.7). With agent review off
// the hold is forced on, so there is always at least one reviewer.
func (s *Server) specHeld(ctx context.Context, d *store.Document) bool {
	// A bug's report is its spec, and is held as one (SPEC-019 SD-8).
	if !lifecycle.IsSpecType(d.Type) {
		return false
	}
	cfg, err := s.freshConfig()
	if err != nil {
		return true // fail closed: a person looks rather than nobody
	}
	if !cfg.AgentSpecReview() {
		return true
	}
	if d.OwnerType == "feature" && d.OwnerID != nil {
		if send, err := store.GetFeatureSend(ctx, s.Store.Pool, *d.OwnerID); err == nil {
			return send.Hold
		}
	}
	return cfg.HoldSpecs()
}

func (s *Server) openIssueIDs(ctx context.Context, docID uuid.UUID) ([]uuid.UUID, error) {
	issues, err := store.OpenIssues(ctx, s.Store.Pool, docID)
	if err != nil {
		return nil, err
	}
	ids := make([]uuid.UUID, 0, len(issues))
	for _, c := range issues {
		ids = append(ids, c.ID)
	}
	return ids, nil
}

// ---- Rule-engine actions ----

// reviseAuthoredDocument answers a spec or plan's return to draft, and a
// person's "allow another round" (Force): the invariants decide whether the
// author is owed a revision (SPEC-011 FR-3.1), and apply the round cap.
func (s *Server) reviseAuthoredDocument(ctx context.Context, a rules.ReviseAuthoredDocument) error {
	return s.restoreAuthoring(ctx, a.FeatureID, a.Force)
}

func authorPurpose(docType string) string {
	// A bug's report, sent back, is revised by the spec author (SPEC-019 SD-7).
	if docType == "dev_plan" {
		return "write-dev-plan"
	}
	return "write-spec"
}

func docTypeWords(docType string) string {
	switch docType {
	case "dev_plan":
		return "dev-plan"
	case "spec":
		return "specification"
	case "bug_report":
		return "bug report"
	}
	return docType
}

// holdDocument records a held agent approval (FR-5.4): the reviewer's
// reasoning and minor findings join the thread, and the spec waits in
// reviewing for a person.
func (s *Server) holdDocument(ctx context.Context, a rules.HoldDocument) error {
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if a.Reasoning != "" {
			if err := store.InsertComment(ctx, tx, a.DocID, a.DispatchID, a.Actor, "", a.Reasoning, ""); err != nil {
				return err
			}
		}
		for _, c := range a.Comments {
			if err := store.InsertComment(ctx, tx, a.DocID, a.DispatchID, a.Actor, c.SectionRef, c.Body, c.Severity); err != nil {
				return err
			}
		}
		if a.DispatchID != nil {
			// The reviewer's approval is a verdict, held (SPEC-017 SD-6).
			v := s.agentVerdict(ctx, a.DocID, store.VerdictApprove, a.Actor, *a.DispatchID)
			v.Held = true
			if err := store.RecordVerdict(ctx, tx, v); err != nil {
				return err
			}
		}
		return store.SetDocumentHold(ctx, tx, a.DocID, a.DispatchID, a.Actor,
			"the spec reviewer approved it, and this spec is held for a person")
	})
	if err == nil {
		s.notifyEntityChanged("document", a.DocID)
	}
	return err
}

// answerIssues stores a reviewer's answers to human issues (SD-6).
func (s *Server) answerIssues(ctx context.Context, a rules.AnswerIssues) error {
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		for _, ans := range a.Answers {
			id, err := uuid.Parse(ans.IssueID)
			if err != nil {
				continue
			}
			if err := store.AddressIssue(ctx, tx, id, a.Actor, a.DispatchID, ans.Status, ans.Note); err != nil {
				return err
			}
		}
		return nil
	})
}

// estimateFeature re-checks the invariants after a plan is approved: the
// closing estimate is one of them (FR-2.2a, FR-7.1).
func (s *Server) estimateFeature(ctx context.Context, featureID uuid.UUID) error {
	return s.reconcileFeatureAuthoring(ctx, featureID)
}

// reviewKey is the idempotency key for a review. A fresh review — a person's
// request, or a re-review after an issue overtook a verdict — is keyed by the
// count of reviews so far, so it is not mistaken for a replay of the last.
func (s *Server) reviewKey(ctx context.Context, a rules.QueueReview) (string, error) {
	doc, err := store.GetDocument(ctx, s.Store.Pool, a.DocID)
	if err != nil {
		return "", err
	}
	if !a.Fresh && a.IdempotencyKey != "" {
		// Each submission is its own review, even of unchanged content — an
		// author that sends back the same text must still be reviewed, or
		// the loop stalls (SPEC-011 FR-3). A replay of the same submission
		// carries the same submitted_at, so it stays a no-op.
		if doc.SubmittedAt != nil {
			return fmt.Sprintf("%s:s%d", a.IdempotencyKey, doc.SubmittedAt.UnixNano()), nil
		}
		return a.IdempotencyKey, nil
	}
	n, err := store.CountDispatchesForRef(ctx, s.Store.Pool, "document", a.DocID, "review-"+a.DocType)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s:r%d", rules.ReviewIdempotencyKey(doc.ID, doc.ContentHash), n), nil
}

// supersedePlanWithSpec runs inside a successor spec's approval (FR-6.6,
// SD-7): the old spec's approved plan decomposed a contract that no longer
// stands, so it goes too, and a ready feature is an idea again so the plan
// invariant can write a new one. A feature being built keeps its plan: that
// is the revision-in-flight path's business (SPEC-009 FR-9.6).
func (s *Server) supersedePlanWithSpec(ctx context.Context, tx pgx.Tx, spec *store.Document) ([]archivedMove, error) {
	if !lifecycle.IsSpecType(spec.Type) || spec.OwnerType != "feature" || spec.OwnerID == nil {
		return nil, nil
	}
	f, err := store.GetFeature(ctx, tx, *spec.OwnerID)
	if err != nil {
		return nil, ignoreNotFound(err)
	}
	if f.State != lifecycle.FeatIdea && f.State != lifecycle.FeatReady {
		return nil, nil
	}
	plan, err := store.CurrentDocForOwner(ctx, tx, "dev_plan", "feature", f.ID)
	if err == store.ErrNotFound || (err == nil && plan.State != lifecycle.DocApproved) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := store.TransitionDocument(ctx, tx, plan, lifecycle.DocSupersede, "lifecycle-engine",
		map[string]any{"cause": "spec-revision", "spec": spec.ID.String()}); err != nil {
		return nil, err
	}
	to := s.archiveTarget(plan)
	if err := store.UpdateDocumentPath(ctx, tx, plan.ID, to); err != nil {
		return nil, err
	}
	if f.State == lifecycle.FeatReady {
		if err := store.TransitionFeature(ctx, tx, f, lifecycle.FeatContractInvalidated, "lifecycle-engine",
			map[string]any{"cause": "spec-revision", "spec": spec.ID.String()}); err != nil {
			return nil, err
		}
	}
	return []archivedMove{{plan.Path, to}}, nil
}

// ---- Send to development (FR-4) ----

// sendReadiness says whether a feature can be sent, and if not, why, in a
// sentence a person can act on (FR-4.1). G0's words are used as they stand.
func (s *Server) sendReadiness(ctx context.Context, f *store.Feature) (bool, string) {
	if _, err := store.GetFeatureSend(ctx, s.Store.Pool, f.ID); err == nil {
		return false, "This feature has already been sent to development."
	}
	// A bug's gate is acceptance, and its report is its contract
	// (DESIGN-010 §9, §17a item 6; SPEC-019 FR-4.1). Triage speaks first, so
	// a rejected bug is told it was rejected, not that it is abandoned.
	if f.IsBug() {
		if ok, why := s.bugSendReadiness(ctx, f); !ok {
			return false, why
		}
	}
	if f.State != lifecycle.FeatIdea && f.State != lifecycle.FeatReady {
		return false, "Only a feature that hasn't started building can be sent to development; this one is " + string(f.State) + "."
	}
	if f.IsBug() {
		return true, ""
	}
	if g := s.specReady(ctx, f); !g.Pass {
		return false, g.Reason
	}
	if strings.TrimSpace(f.Description) == "" {
		return false, "This feature has no description yet. Describe what it should do before sending it, because the spec is written from that description."
	}
	return true, ""
}

// SendToDevelopment writes the sent mark (FR-1.3, FR-4.4). It is called only
// from the web UI's send form (FR-4.5, NFR-3). hold is the send screen's
// checkbox; with agent review off it is forced on.
func (s *Server) SendToDevelopment(ctx context.Context, featureID uuid.UUID, hold bool, actor string) error {
	f, err := store.GetFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return err
	}
	if ok, why := s.sendReadiness(ctx, f); !ok {
		return errors.New(why)
	}
	cfg, err := s.freshConfig()
	if err != nil {
		return err
	}
	if !cfg.AgentSpecReview() {
		hold = true
	}
	// A bug's draft report is checked before the mark and submitted after
	// it: for a bug, sending is "review this and go on" (SPEC-019 SD-6).
	var report *store.Document
	if f.IsBug() {
		if report, err = s.checkReportSubmittable(ctx, f); err != nil {
			return err
		}
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := store.RecordFeatureSend(ctx, tx, f.ID, actor, hold)
		return e
	})
	if errors.Is(err, store.ErrAlreadySent) {
		return errors.New("This feature has already been sent to development.")
	}
	if err != nil {
		return err
	}
	if report != nil {
		if r, _, err := s.SubmitDoc(ctx, report.Path, actor); err != nil {
			s.Log.Warn("could not submit a sent bug's report", "bug", f.PublicID, "err", err)
		} else if !r.Valid {
			s.Log.Warn("a sent bug's report failed validation after the send", "bug", f.PublicID)
		}
	}
	s.notifyEntityChanged("feature", f.ID)
	s.Bus.Publish(bus.FeatureSent{FeatureID: f.ID})
	return nil
}

// authoringPurposes are the dispatches a send sets off, which Withdraw may
// cancel while they wait.
var authoringPurposes = []string{"write-spec", "write-dev-plan"}

// withdrawable reports whether a send can still be withdrawn (FR-4.6, SD-10):
// every authoring dispatch since the send is still queued, and none has
// written a document.
func (s *Server) withdrawable(ctx context.Context, f *store.Feature) (bool, []store.Dispatch, error) {
	send, err := store.GetFeatureSend(ctx, s.Store.Pool, f.ID)
	if err != nil {
		return false, nil, ignoreNotFound(err)
	}
	if f.State != lifecycle.FeatIdea && f.State != lifecycle.FeatReady {
		return false, nil, nil
	}
	ds, err := store.DispatchesForRefSince(ctx, s.Store.Pool, "feature", f.ID, authoringPurposes, send.SentAt)
	if err != nil {
		return false, nil, err
	}
	// A bug's send submits its report, so the report's review is the work
	// the send set off, and it too must still be waiting (SPEC-019 R19-12).
	if f.IsBug() {
		if rep, err := store.CurrentDocForOwner(ctx, s.Store.Pool, lifecycle.DocTypeBugReport, "feature", f.ID); err == nil {
			rs, err := store.DispatchesForRefSince(ctx, s.Store.Pool, "document", rep.ID,
				[]string{"review-" + lifecycle.DocTypeBugReport}, send.SentAt)
			if err != nil {
				return false, nil, err
			}
			ds = append(ds, rs...)
		}
	}
	for _, d := range ds {
		if d.State != "queued" {
			return false, nil, nil
		}
	}
	return true, ds, nil
}

// WithdrawSend removes the mark while nothing has started (FR-4.6).
func (s *Server) WithdrawSend(ctx context.Context, featureID uuid.UUID, actor string) error {
	f, err := store.GetFeature(ctx, s.Store.Pool, featureID)
	if err != nil {
		return err
	}
	if _, err := store.GetFeatureSend(ctx, s.Store.Pool, f.ID); err == store.ErrNotFound {
		return errors.New("This feature hasn't been sent to development, so there is nothing to withdraw.")
	}
	ok, queued, err := s.withdrawable(ctx, f)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("Work on this feature has already started, so the send can't be withdrawn. Abandon the feature if it should stop.")
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		cancelled := 0
		for _, d := range queued {
			done, err := store.CancelQueuedDispatch(ctx, tx, d.ID, actor, "send withdrawn")
			if err != nil {
				return err
			}
			if !done {
				// The dispatcher claimed it between the check and now.
				return errors.New("Work on this feature started just now, so the send can't be withdrawn. Abandon the feature if it should stop.")
			}
			cancelled++
		}
		// A bug's report goes back to draft with its review cancelled, as it
		// was before the send (SPEC-019 R19-12).
		if f.IsBug() {
			if rep, err := store.CurrentDocForOwner(ctx, tx, lifecycle.DocTypeBugReport, "feature", f.ID); err == nil &&
				rep.State == lifecycle.DocReviewing {
				if err := store.ClearDocumentHold(ctx, tx, rep.ID); err != nil {
					return err
				}
				if err := store.TransitionDocument(ctx, tx, rep, lifecycle.DocWithdraw, actor,
					map[string]any{"cause": "send withdrawn"}); err != nil {
					return err
				}
			}
		}
		return store.DeleteFeatureSend(ctx, tx, f.ID, actor, cancelled)
	})
	if err == nil {
		s.notifyEntityChanged("feature", f.ID)
	}
	return err
}

// ---- A person's review acts (FR-5.5, FR-6, FR-8) ----

// refuseIfPending refuses a person's act on a document while a question about
// it waits in the Inbox (SD-11): an escalated review or an authoring deadlock
// that refs the document, or a design-revision question that lists its
// feature. Acting here would answer that question by the back door.
func (s *Server) refuseIfPending(ctx context.Context, doc *store.Document) error {
	pending, err := s.Store.PendingCheckpoints(ctx)
	if err != nil {
		return err
	}
	for _, cp := range pending {
		if cp.RefType == "document" && cp.RefID == doc.ID &&
			(cp.Kind == "review-escalation" || cp.Kind == "authoring-deadlock") {
			return errors.New("A question about this document is waiting in the Inbox, so the decision is made there, in the web UI. Answer it first.")
		}
	}
	if doc.OwnerType == "feature" && doc.OwnerID != nil {
		if blocked, err := s.pendingDesignRevisionFor(ctx, *doc.OwnerID); err != nil {
			return err
		} else if blocked {
			return errors.New("This feature's design was revised, and a question in the Inbox asks whether its specification still stands. Answer it there first.")
		}
	}
	return nil
}

// agentReviewedType reports whether a document type is decided by an agent
// reviewer (spec, dev_plan and the like), as opposed to by a person.
func (s *Server) agentReviewedType(docType string) bool {
	m, err := config.LoadManifest(s.CompartmentRoot, docType)
	return err == nil && !m.HumanApproval()
}

// DirectApprove is a person's approval of a document in reviewing (FR-5.5).
// For a spec or plan it settles the open issues by that decision (SD-6) and
// clears any hold; for a design it is SPEC-009's human approval.
func (s *Server) DirectApprove(ctx context.Context, docID uuid.UUID, act relayAct) error {
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return err
	}
	if doc.State != lifecycle.DocReviewing {
		return fmt.Errorf("Only a document in review can be approved; this one is %s.%s", doc.State, submitHint(doc.State))
	}
	if err := s.refuseIfPending(ctx, doc); err != nil {
		return err
	}
	if err := s.auditAct(ctx, doc.ID, act, "document.human_verdict", map[string]any{"verdict": "approve"}); err != nil {
		return err
	}
	return s.approveDocumentAs(ctx, rules.ApproveDocument{DocID: doc.ID, Actor: act.Actor},
		personVerdict(doc.ID, store.VerdictApprove, act))
}

func submitHint(state lifecycle.DocumentState) string {
	if state == lifecycle.DocDraft {
		return " Submit it for review first."
	}
	return ""
}

// SendBack is a person's send-back verdict (FR-8.1). For a design it is
// SPEC-009's request for changes. For a spec or plan the reason is recorded
// as a human issue, because a person's objection must be addressed (SD-6),
// and the document returns to draft — and to its author, for a sent feature.
func (s *Server) SendBack(ctx context.Context, docID uuid.UUID, reason string, act relayAct) error {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return errors.New("Sending a document back needs a reason its author can act on.")
	}
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return err
	}
	if doc.State != lifecycle.DocReviewing {
		return fmt.Errorf("Only a document in review can be sent back; this one is %s.", doc.State)
	}
	if err := s.refuseIfPending(ctx, doc); err != nil {
		return err
	}
	if s.humanApprovalType(doc.Type) {
		if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			return store.Audit(ctx, tx, act.Actor, "document.human_verdict", "document", &doc.ID,
				act.payload(map[string]any{"verdict": "send_back"}))
		}); err != nil {
			return err
		}
		return s.humanReturnDocument(ctx, doc.ID, reason, act)
	}
	if err := s.checkIssueAllowed(ctx, doc); err != nil {
		return err
	}
	return s.returnWithIssue(ctx, doc, reason, "", act, true)
}

// returnWithIssue records a human issue and sends the document back to draft
// in one transaction, cancelling a queued review and any hold.
//
// verdict is set for a person's send-back (SD-15), which is a verdict; an
// issue raised on a document in review sends it back too, but records none
// (SPEC-017 FR-2.3).
func (s *Server) returnWithIssue(ctx context.Context, doc *store.Document, body, section string, act relayAct, verdict bool) error {
	from := doc.State
	err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := store.InsertIssue(ctx, tx, doc.ID, act.Actor, section, body, act.Via, act.Quote); err != nil {
			return err
		}
		if err := s.cancelQueuedReviews(ctx, tx, doc, act.Actor); err != nil {
			return err
		}
		if err := store.ClearDocumentHold(ctx, tx, doc.ID); err != nil {
			return err
		}
		if verdict {
			v := personVerdict(doc.ID, store.VerdictSendBack, act)
			if err := store.RecordVerdict(ctx, tx, v); err != nil {
				return err
			}
			return store.TransitionDocument(ctx, tx, doc, lifecycle.DocRequestChanges, act.Actor,
				act.payload(map[string]any{"cause": "human issue", "verdict_by": store.GiverPerson}))
		}
		return store.TransitionDocument(ctx, tx, doc, lifecycle.DocRequestChanges, act.Actor,
			act.payload(map[string]any{"cause": "human issue"}))
	})
	if err != nil {
		return err
	}
	s.notifyEntityChanged("document", doc.ID)
	s.Bus.Publish(bus.DocumentTransitioned{
		DocID: doc.ID, From: from, To: lifecycle.DocDraft,
		Event: lifecycle.DocRequestChanges, Actor: act.Actor,
	})
	return nil
}

func (s *Server) cancelQueuedReviews(ctx context.Context, tx pgx.Tx, doc *store.Document, actor string) error {
	rows, err := tx.Query(ctx, `SELECT id FROM dispatches
		WHERE ref_type = 'document' AND ref_id = $1 AND purpose = $2 AND state = 'queued'`,
		doc.ID, "review-"+doc.Type)
	if err != nil {
		return err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		if _, err := store.CancelQueuedDispatch(ctx, tx, id, actor, "document sent back by a person"); err != nil {
			return err
		}
	}
	return nil
}

// checkIssueAllowed refuses an issue once building has started (FR-6.1):
// then the answer is a revision.
func (s *Server) checkIssueAllowed(ctx context.Context, doc *store.Document) error {
	if doc.OwnerType != "feature" || doc.OwnerID == nil {
		return nil
	}
	f, err := store.GetFeature(ctx, s.Store.Pool, *doc.OwnerID)
	if err != nil {
		return err
	}
	if f.State != lifecycle.FeatIdea && f.State != lifecycle.FeatReady {
		return fmt.Errorf("Building has started on %s, so issues can no longer be raised on its %s. Revise the document instead.",
			f.Name, docTypeWords(doc.Type))
	}
	return nil
}

// RaiseIssue records a human issue on a document and routes it by the
// document's state and whether its feature is sent (FR-6.2).
func (s *Server) RaiseIssue(ctx context.Context, docID uuid.UUID, body, section string, act relayAct) (*store.Document, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return nil, errors.New("An issue needs words: say what is wrong, so the author can deal with it.")
	}
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return nil, err
	}
	if doc.State == lifecycle.DocSuperseded {
		return nil, errors.New("This document has been superseded. Raise the issue on its current version.")
	}
	if s.humanApprovalType(doc.Type) {
		// A design carries an issue as a note for its human approver (SD-14).
		if doc.State == lifecycle.DocApproved {
			switch doc.Type {
			case "decision":
				return nil, errors.New("This decision is accepted, and an accepted decision is never edited. Append a dated amendment, or supersede it with a new decision, from its page.")
			case "design":
				return nil, errors.New("This design is approved. A design is changed by revising it, so start a revision and make the change there.")
			}
			return nil, fmt.Errorf("This %s is approved. It is changed by revising it, so start a revision and make the change there.", docTypeWords(doc.Type))
		}
		return doc, s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := store.InsertIssue(ctx, tx, doc.ID, act.Actor, section, body, act.Via, act.Quote)
			return e
		})
	}
	if !lifecycle.IsContractType(doc.Type) {
		return nil, fmt.Errorf("Issues can be raised on specifications, bug reports, dev-plans and designs, not on a %s.", doc.Type)
	}
	if err := s.checkIssueAllowed(ctx, doc); err != nil {
		return nil, err
	}
	if err := s.refuseIfPending(ctx, doc); err != nil {
		return nil, err
	}
	sent := false
	if doc.OwnerType == "feature" && doc.OwnerID != nil {
		if f, err := store.GetFeature(ctx, s.Store.Pool, *doc.OwnerID); err == nil {
			sent, _ = s.featureSent(ctx, f)
		}
	}

	switch doc.State {
	case lifecycle.DocReviewing:
		if sent {
			return doc, s.returnWithIssue(ctx, doc, body, section, act, false)
		}
		// Unsent: recorded only. A review in flight will meet it, and a
		// verdict that didn't answer it is not applied (FR-6.4).
		err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := store.InsertIssue(ctx, tx, doc.ID, act.Actor, section, body, act.Via, act.Quote)
			return e
		})
		if err == nil {
			s.notifyEntityChanged("document", doc.ID)
		}
		return doc, err

	case lifecycle.DocDraft:
		if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := store.InsertIssue(ctx, tx, doc.ID, act.Actor, section, body, act.Via, act.Quote)
			return e
		}); err != nil {
			return nil, err
		}
		s.notifyEntityChanged("document", doc.ID)
		if sent {
			// The draft now waits for its author, who revises it (FR-6.2)
			// unless already at work — the invariant decides.
			return doc, s.reconcileFeatureAuthoring(ctx, *doc.OwnerID)
		}
		return doc, nil

	case lifecycle.DocApproved:
		// An approved document reopens as a successor that carries the issue
		// (SD-7). The approved one stays current until the successor is
		// approved, and building can't start in between (FR-6.7).
		// A revision already under way carries the issue instead.
		if live, err := s.liveSuccessor(ctx, doc.ID); err != nil {
			return nil, err
		} else if live != nil {
			return s.RaiseIssue(ctx, live.ID, body, section, act)
		}
		// The person who raised the issue opened the revision, relayed or
		// not (SPEC-017 FR-2.2, R17-7).
		succ, err := s.reviseDocBy(ctx, doc.Path, act.Actor,
			writerAct{Act: store.ActOpenedRevision, Kind: store.WriterPerson, Actor: act.Actor, Via: act.Via})
		if err != nil {
			return nil, err
		}
		if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
			_, e := store.InsertIssue(ctx, tx, succ.ID, act.Actor, section, body, act.Via, act.Quote)
			return e
		}); err != nil {
			return nil, err
		}
		s.notifyEntityChanged("document", doc.ID)
		if sent {
			// The successor carries the issue, so it waits for its author.
			return succ, s.reconcileFeatureAuthoring(ctx, *doc.OwnerID)
		}
		return succ, nil
	}
	return doc, nil
}

// liveSuccessor returns the live revision of a document, or nil.
func (s *Server) liveSuccessor(ctx context.Context, docID uuid.UUID) (*store.Document, error) {
	var id uuid.UUID
	err := s.Store.Pool.QueryRow(ctx, `SELECT id FROM documents
		WHERE supersedes_id = $1 AND state <> 'superseded' LIMIT 1`, docID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return store.GetDocument(ctx, s.Store.Pool, id)
}

// RequestReview is a person's request for an agent review (FR-5.5). A draft
// is submitted, which queues its review as usual. A document in review with
// no review queued or running gets a fresh one, even with agent review off —
// a one-off, whose approval of a held spec is held again.
func (s *Server) RequestReview(ctx context.Context, docID uuid.UUID, act relayAct) (string, error) {
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return "", err
	}
	m, err := config.LoadManifest(s.CompartmentRoot, doc.Type)
	if err != nil {
		return "", fmt.Errorf("Documents of type %s have no template, so there is no reviewer for them.", doc.Type)
	}
	if m.ReviewerRole == "" {
		return "", errors.New("Designs have no agent reviewer: a design is reviewed in the conversation that writes it. For a cold read, use the review-design chat skill.")
	}
	if err := s.refuseIfPending(ctx, doc); err != nil {
		return "", err
	}
	switch doc.State {
	case lifecycle.DocDraft:
		if err := s.refuseIfAuthorAtWork(ctx, doc); err != nil {
			return "", err
		}
		if err := s.auditAct(ctx, doc.ID, act, "document.review_requested", nil); err != nil {
			return "", err
		}
		report, _, err := s.SubmitDoc(ctx, doc.Path, act.Actor)
		if err != nil {
			return "", err
		}
		if !report.Valid {
			return "", fmt.Errorf("The document doesn't pass validation yet, so it can't be reviewed: %s", reportSentence(report))
		}
		return "The document was submitted, and its review is queued.", nil
	case lifecycle.DocReviewing:
		live, err := store.LiveDispatchForRef(ctx, s.Store.Pool, "document", doc.ID, "review-"+doc.Type)
		if err != nil {
			return "", err
		}
		if live {
			return "", errors.New("A review of this document is already queued or running.")
		}
		if err := s.auditAct(ctx, doc.ID, act, "document.review_requested", nil); err != nil {
			return "", err
		}
		return "A fresh agent review is queued.", s.queueReview(ctx, rules.QueueReview{DocID: doc.ID, DocType: doc.Type, Fresh: true})
	}
	return "", fmt.Errorf("Only a draft or a document in review can be reviewed; this one is %s.", doc.State)
}

// SubmitFromChat submits a draft the chat agent wrote, as the chat agent,
// with no quote (SPEC-017 FR-1, SD-1): submitting is asking the orchestrator
// to do what it would do anyway (DEC-005). It says what happens next, in the
// project's own terms, so the chat agent can tell the person.
func (s *Server) SubmitFromChat(ctx context.Context, doc *store.Document) (string, error) {
	switch doc.State {
	case lifecycle.DocDraft:
	case lifecycle.DocReviewing:
		return "", errors.New("This document is already in review. A fresh review is a person's call: relay it with relay_review_request and their words.")
	case lifecycle.DocApproved:
		return "", errors.New("This document is approved. To change it, a person revises it on its page, or raises an issue on it, which you can relay.")
	default:
		return "", fmt.Errorf("This revision of the document is %s; name the current one.", doc.State)
	}
	// A revision of a feature being built pauses its building until a person
	// answers (SPEC-011 FR-6.7, revision-in-flight), so submitting one is the
	// person's call, not a planning act (SPEC-017 SD-1, R17-1).
	if doc.SupersedesID != nil && doc.OwnerType == "feature" && doc.OwnerID != nil {
		if f, err := store.GetFeature(ctx, s.Store.Pool, *doc.OwnerID); err == nil &&
			(f.State == lifecycle.FeatActive || f.State == lifecycle.FeatReview) {
			return "", errors.New("This is a revision of a feature that is being built, and submitting it pauses the building until a person decides. A person submits it on its page.")
		}
	}
	if !s.hasTemplate(doc.Type) {
		return "", fmt.Errorf("Documents of type %s have no template to check them against, so they can't be submitted for review yet.", doc.Type)
	}
	if err := s.refuseIfPending(ctx, doc); err != nil {
		return "", err
	}
	if err := s.refuseIfAuthorAtWork(ctx, doc); err != nil {
		return "", err
	}
	// The submission's own audit row says it came from chat (NFR-1: one
	// transaction).
	report, _, err := s.submitDocWith(ctx, doc.Path, s.mcpActor(),
		map[string]any{"via": "mcp", "submitted_from_chat": true}, nil)
	if err != nil {
		return "", err
	}
	if !report.Valid {
		return "", fmt.Errorf("The document doesn't pass its template's checks yet, so it stays a draft. Fix these and submit it again: %s", reportSentence(report))
	}
	s.notifyEntityChanged("document", doc.ID)
	return s.afterSubmission(ctx, doc), nil
}

// afterSubmission says what happens to a document once it is submitted
// (SPEC-017 FR-1.3).
func (s *Server) afterSubmission(ctx context.Context, doc *store.Document) string {
	words := docTypeWords(doc.Type)
	if s.humanApprovalType(doc.Type) {
		return "It is in review, and waits for a person to approve it, on its page or by telling you."
	}
	cfg, err := s.freshConfig()
	if err != nil {
		return "It is in review."
	}
	if lifecycle.IsSpecType(doc.Type) && !cfg.AgentSpecReview() {
		return "Agent review is switched off for this project, so the " + words + " waits for a person to approve it."
	}
	role := s.reviewerRole(doc.Type)
	if role == "" {
		return "It is in review, and waits for a person."
	}
	who := s.roleWho(cfg, "review-"+doc.Type, role)
	reviewer := "The " + roleWords(role) + " (" + strings.TrimPrefix(who, role+" on ") + ")"
	if doc.Type == "dev_plan" && doc.OwnerType == "feature" && doc.OwnerID != nil {
		// An approved plan is broken into tasks at once, and with an approved
		// spec the feature is ready to build, sent or not (SPEC-011 SD-2).
		return "Its review is queued. " + reviewer + " decides whether the dev-plan is approved; you don't. " +
			"Once it is approved, it is broken into tasks, and with an approved specification the feature is ready " +
			"to build. Building starts only when a person presses Start building."
	}
	if s.specHeld(ctx, doc) {
		return "Its review is queued. " + reviewer + " reviews the " + words + ", and then it is held for a person, who approves it or lets the reviewer decide. Neither verdict is yours."
	}
	return "Its review is queued. " + reviewer + " decides whether the " + words + " is approved; you don't."
}

func reportSentence(r *lifecycle.Report) string {
	var parts []string
	for _, is := range r.Issues {
		parts = append(parts, is.Detail)
	}
	return strings.Join(parts, "; ") + "."
}

func (s *Server) auditAct(ctx context.Context, docID uuid.UUID, act relayAct, kind string, extra map[string]any) error {
	return s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.Audit(ctx, tx, act.Actor, kind, "document", &docID, act.payload(extra))
	})
}

// releaseState says whether a held spec can be released with "let the
// reviewer decide", and if not, why (FR-5.5).
func (s *Server) releaseState(ctx context.Context, doc *store.Document) (*store.DocumentHold, error) {
	if doc.State != lifecycle.DocReviewing {
		return nil, errors.New("Only a spec waiting in review can be released to its reviewer.")
	}
	hold, err := store.GetDocumentHold(ctx, s.Store.Pool, doc.ID)
	if err == store.ErrNotFound {
		return nil, errors.New("This spec isn't held for anyone, so there is nothing to release.")
	}
	if err != nil {
		return nil, err
	}
	cfg, err := s.freshConfig()
	if err != nil {
		return nil, err
	}
	if !cfg.AgentSpecReview() {
		return nil, errors.New("Agent review is switched off for this project, so there is no reviewer to let decide. Approve the spec, or raise an issue on it.")
	}
	if hold.DispatchID == nil {
		return nil, errors.New("No agent reviewer has approved this spec yet. Ask for an agent review, or approve it yourself.")
	}
	open, err := store.OpenIssues(ctx, s.Store.Pool, doc.ID)
	if err != nil {
		return nil, err
	}
	if len(open) > 0 {
		return nil, errors.New("An issue was raised after the reviewer approved this spec, so its approval doesn't cover it. Ask for a fresh review, or approve the spec yourself.")
	}
	return hold, nil
}

// ReleaseHold is "let the reviewer decide" (FR-5.5): the held agent approval
// stands. The approval is the reviewer's; the release is the person's.
func (s *Server) ReleaseHold(ctx context.Context, docID uuid.UUID, act relayAct) error {
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return err
	}
	hold, err := s.releaseState(ctx, doc)
	if err != nil {
		return err
	}
	d, err := store.GetDispatch(ctx, s.Store.Pool, *hold.DispatchID)
	if err != nil {
		return err
	}
	if err := s.auditAct(ctx, doc.ID, act, "document.hold_released", map[string]any{"dispatch_id": d.ID.String()}); err != nil {
		return err
	}
	v := s.agentVerdict(ctx, doc.ID, store.VerdictApprove, d.Role, d.ID)
	v.ReleasedBy, v.ReleasedVia, v.ReleasedQuote = act.Actor, act.Via, act.Quote
	return s.approveDocumentAs(ctx, rules.ApproveDocument{DocID: doc.ID, Actor: d.Role, DispatchID: &d.ID}, v)
}

// refuseIfAuthorAtWork refuses a person's submission of a draft while its
// author agent is revising it, so the two can't race (FR-3.6).
func (s *Server) refuseIfAuthorAtWork(ctx context.Context, doc *store.Document) error {
	if doc.OwnerType != "feature" || doc.OwnerID == nil || !lifecycle.IsContractType(doc.Type) {
		return nil
	}
	live, err := store.LiveDispatchForRef(ctx, s.Store.Pool, "feature", *doc.OwnerID, authorPurpose(doc.Type))
	if err != nil {
		return err
	}
	if live {
		return errors.New("Its author is revising this document now. It will be submitted for review when the revision is done.")
	}
	return nil
}

// DetachDocument removes a draft's registration, leaving the file (FR-9.3).
func (s *Server) DetachDocument(ctx context.Context, docID uuid.UUID, actor string) (*store.Document, error) {
	doc, _, err := s.detachDocument(ctx, docID, actor)
	return doc, err
}

// detachDocument is DetachDocument, also saying what happened to the ID in
// the file (SPEC-015 SD-13): "committed", "uncommitted", or "" for none.
func (s *Server) detachDocument(ctx context.Context, docID uuid.UUID, actor string) (*store.Document, string, error) {
	doc, err := store.GetDocument(ctx, s.Store.Pool, docID)
	if err != nil {
		return nil, "", err
	}
	if doc.State != lifecycle.DocDraft {
		return nil, "", fmt.Errorf("Only a draft can be detached; this document is %s. An approved document is changed by revising it.", doc.State)
	}
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.DetachDocument(ctx, tx, doc, actor)
	}); err != nil {
		return nil, "", err
	}
	idOutcome := s.stripDetachedIdentity(doc)
	if doc.OwnerType == "feature" && doc.OwnerID != nil {
		s.notifyEntityChanged("feature", *doc.OwnerID)
		if err := s.reconcileFeatureAuthoring(ctx, *doc.OwnerID); err != nil {
			s.Log.Warn("reconcile after detach", "feature", *doc.OwnerID, "err", err)
		}
	}
	return doc, idOutcome, nil
}
