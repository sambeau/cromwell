package server

// The chat agent's bug tools (SPEC-019 FR-6). Reporting a bug is planning
// authoring under DEC-004: it creates an idea and commits nothing, so
// report_bug takes no quote. Triage is a person's decision (DESIGN-010 §9),
// so the chat agent only carries it, with their words, through relay_triage —
// the relay DESIGN-010 §17a item 6 allowed. No tool here sends a bug, starts
// building it, or reopens a decision.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

func (s *Server) mcpBugTools() []mcpTool {
	return []mcpTool{
		{
			Name: "report_bug",
			Description: "Report a bug: something in the software that is broken. Anyone may report one, on a person's " +
				"behalf or your own, and it needs no one's permission. It is filed with a report — the steps to reproduce " +
				"it, what should happen and what happens instead — and waits in the triage queue. Whether it is fixed " +
				"is a person's decision, made in triage; you can carry that decision with relay_triage, never make it.",
			Schema: objectSchema(map[string]any{
				"on":       stringProp("Where the bug was found: an initiative or feature by path, such as \"auth/login\", or by ID, such as \"FEAT-012\", \"INIT-003\" or \"BUG-004\"."),
				"title":    stringProp("A short name for the defect, as a person would say it, such as \"Login accepts an empty password\"."),
				"summary":  stringProp("Optional. One or two sentences saying what is wrong."),
				"steps":    stringProp("How to make it happen, as numbered steps someone else could follow."),
				"expected": stringProp("What should happen."),
				"actual":   stringProp("What happens instead."),
				"notes":    stringProp("Optional. Anything else: a log line, where in the code, a guess at the cause."),
			}, "on", "title", "steps", "expected", "actual"),
			Handler: s.mcpReportBug,
		},
		{
			Name: "list_bugs",
			Description: "List bugs, with each one's ID, title, triage state (reported, accepted, rejected or duplicate), " +
				"lifecycle state and where it hangs. Filter by triage state, or \"open\" for bugs still to be dealt " +
				"with, and by the initiative or feature they were reported on.",
			Schema: objectSchema(map[string]any{
				"triage": stringProp("Optional. \"reported\", \"accepted\", \"rejected\", \"duplicate\", or \"open\" for reported or accepted bugs not yet finished."),
				"on":     stringProp("Optional. An initiative or feature, by path or ID, to list only the bugs reported on it."),
			}),
			Handler: s.mcpListBugs,
		},
		{
			Name: "get_bug",
			Description: "Read one bug by its ID, such as \"BUG-004\": its report's path and review state, who reported " +
				"it and how, and its triage decision, with who made it, the reason and any quoted words.",
			Schema: objectSchema(map[string]any{
				"bug": stringProp("The bug's ID, such as \"BUG-004\"."),
			}, "bug"),
			Handler: s.mcpGetBug,
		},
		{
			Name: "relay_triage",
			Description: "Carry a person's triage decision on a reported bug: accept it, so it can be fixed; reject it, " +
				"with their reason; or mark it as a duplicate of another bug. Use this only when the person has told " +
				"you their decision, and quote their words. You have no triage decision of your own and must never " +
				"accept or reject a bug on your own judgement. Accepting doesn't start any work: a person sends an " +
				"accepted bug to development from the command centre.",
			Schema: objectSchema(map[string]any{
				"bug":          stringProp("The bug's ID, such as \"BUG-004\"."),
				"decision":     stringProp("\"accept\", \"reject\" or \"duplicate\"."),
				"reason":       stringProp("Required to reject: why, in the person's terms."),
				"duplicate_of": stringProp("Required for a duplicate: the ID of the bug this one repeats, such as \"BUG-002\"."),
				"quote":        quoteProp,
			}, "bug", "decision", "quote"),
			Handler: s.mcpRelayTriage,
		},
	}
}

// mcpOn resolves report_bug's and list_bugs' "on": an initiative, a feature
// or a bug, by path or ID.
func (s *Server) mcpOn(ctx context.Context, ref string) (ownerType string, id uuid.UUID, err error) {
	ref = strings.TrimSpace(ref)
	if f, _, ferr := s.featureByRef(ctx, ref); ferr == nil {
		return "feature", f.ID, nil
	}
	if in, _, ierr := s.initiativeByRef(ctx, ref); ierr == nil {
		return "initiative", in.ID, nil
	}
	return "", id, fmt.Errorf("there is no initiative or feature at %q; call get_tree to see what exists", ref)
}

func (s *Server) mcpReportBug(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	on, ok := argString(args, "on")
	if !ok {
		return nil, errors.New("say where the bug was found, with \"on\": an initiative or feature, by path or ID")
	}
	ownerType, ownerID, err := s.mcpOn(ctx, on)
	if err != nil {
		return nil, err
	}
	initiativeID, origin, err := s.reportOn(ctx, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	get := func(k string) string { v, _ := argString(args, k); return v }
	f, doc, err := s.ReportBug(ctx, BugReport{
		InitiativeID: initiativeID, OriginFeatureID: origin,
		Title: get("title"), Summary: get("summary"), Steps: get("steps"),
		Expected: get("expected"), Actual: get("actual"), Notes: get("notes"),
		ReporterKind: store.ReporterChat, Actor: s.mcpActor(), Via: "mcp",
	})
	if err != nil {
		return nil, err
	}
	b, err := store.GetBug(ctx, s.Store.Pool, f.ID)
	if err != nil {
		return nil, err
	}
	out := s.bugResult(ctx, b)
	out["report"] = doc.Path
	out["next"] = "It waits in the triage queue for a person to accept or reject it."
	return out, nil
}

func (s *Server) mcpListBugs(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	var bf store.BugFilter
	if t, ok := argString(args, "triage"); ok {
		switch t {
		case "open", store.TriageReported, store.TriageAccepted, store.TriageRejected, store.TriageDuplicate:
			bf.Triage = t
		default:
			return nil, fmt.Errorf("triage is reported, accepted, rejected, duplicate or open, not %q", t)
		}
	}
	if on, ok := argString(args, "on"); ok {
		ownerType, id, err := s.mcpOn(ctx, on)
		if err != nil {
			return nil, err
		}
		if ownerType == "feature" {
			bf.FeatureID = &id
		} else {
			bf.InitiativeID = &id
		}
	}
	bugs, err := store.ListBugs(ctx, s.Store.Pool, bf)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(bugs))
	for i := range bugs {
		out = append(out, s.bugResult(ctx, &bugs[i]))
	}
	return map[string]any{"bugs": out}, nil
}

func (s *Server) mcpGetBug(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	ref, _ := argString(args, "bug")
	b, err := s.bugByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	out := s.bugResult(ctx, b)
	out["reported_by"] = s.reporterWords(ctx, b)
	if d, err := store.CurrentDocForOwner(ctx, s.Store.Pool, lifecycle.DocTypeBugReport, "feature", b.Feature.ID); err == nil {
		out["report"] = map[string]any{"id": d.PublicID, "path": d.Path, "state": string(d.State)}
	}
	if b.Triage != store.TriageReported {
		dec := map[string]any{"by": b.DecidedBy, "via": b.DecidedVia}
		if b.DecidedAt != nil {
			dec["at"] = b.DecidedAt
		}
		if b.DecidedReason != "" {
			dec["reason"] = b.DecidedReason
		}
		if b.DecidedQuote != "" {
			dec["quote"] = b.DecidedQuote
		}
		out["decision"] = dec
	}
	return out, nil
}

func (s *Server) mcpRelayTriage(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	quote, _ := argString(args, "quote")
	if strings.TrimSpace(quote) == "" {
		return nil, errors.New("a relayed triage decision needs the person's own words, quoted, in \"quote\"")
	}
	ref, _ := argString(args, "bug")
	b, err := s.bugByRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	decision, _ := argString(args, "decision")
	reason, _ := argString(args, "reason")
	dup, _ := argString(args, "duplicate_of")
	b, err = s.DecideTriage(ctx, b.Feature.ID, decision, reason, dup,
		relayAct{Actor: s.mcpActor(), Via: "mcp", Quote: quote})
	if err != nil {
		return nil, err
	}
	out := s.bugResult(ctx, b)
	out["relayed"] = "recorded the person's triage decision: " + b.Triage
	if b.Triage == store.TriageAccepted {
		out["next"] = "It can now be sent to development, which a person does from the command centre."
	}
	return out, nil
}

// bugResult is a bug as the MCP tools show it.
func (s *Server) bugResult(ctx context.Context, b *store.Bug) map[string]any {
	out := map[string]any{
		"id": b.Feature.PublicID, "title": b.Feature.Name, "triage": b.Triage,
		"state": string(b.Feature.State),
	}
	if path, err := s.featurePath(ctx, &b.Feature); err == nil {
		out["path"], out["url"] = path, "/ui/f/"+path
	}
	if in, err := store.GetInitiative(ctx, s.Store.Pool, b.Feature.InitiativeID); err == nil {
		out["initiative"] = in.PublicID
	}
	if b.OriginFeatureID != nil {
		if o, err := store.GetFeature(ctx, s.Store.Pool, *b.OriginFeatureID); err == nil {
			out["reported_on"] = o.PublicID
		}
	}
	if b.DuplicateOf != nil {
		if o, err := store.GetFeature(ctx, s.Store.Pool, *b.DuplicateOf); err == nil {
			out["duplicate_of"] = o.PublicID
		}
	}
	return out
}
