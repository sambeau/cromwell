package server

// The decision tools (SPEC-018 FR-2.3, FR-7.3, FR-7.4, SD-15). create_decision
// is planning authoring under DEC-004: it writes a draft from the template,
// which a person accepts; the chat agent hands it in with submit_for_review
// and can carry the person's verdict with relay_verdict, as for any document.
// list_decisions and get_decision are reads. None accepts, supersedes or
// amends anything: acceptance is a person's.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"subutai/internal/ident"
)

func (s *Server) mcpDecisionTools() []mcpTool {
	return []mcpTool{
		{
			Name: "create_decision",
			Description: "Start a decision the person has made, as a draft with the next DEC- number, from the decision " +
				"template. Fill in its ruling (at most 75 words, the rule itself, which is what agents are told), its " +
				"reason (one line), its context and the alternatives, then hand it in with submit_for_review. A person " +
				"accepts it; you can relay their verdict with relay_verdict. Name any decisions it replaces in " +
				"supersedes: they stay in force until a person accepts this one.",
			Schema: objectSchema(map[string]any{
				"title": stringProp("A short title saying what was decided."),
				"owner": stringProp("Who it binds: \"project\", or an initiative's path or ID, such as \"INIT-004\". An initiative's decision binds everything under it."),
				"supersedes": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
					"description": "Optional. The IDs of accepted decisions this one replaces, such as [\"DEC-003\"]."},
			}, "title", "owner"),
			Handler: s.mcpCreateDecision,
		},
		{
			Name: "list_decisions",
			Description: "List the project's decisions: each one's ID, title, owner, state, the ruling and reason agents " +
				"are told, what it supersedes and what superseded it. Give an owner to see what dispatches there are " +
				"told, including the conventions and any decisions the size cap leaves out.",
			Schema: objectSchema(map[string]any{
				"owner": stringProp("Optional. \"project\", or an initiative's path or ID: that initiative and everything above it, which is what its agents are told."),
				"state": stringProp("Optional. \"accepted\" (the default), \"open\" (drafts and in review), \"superseded\" or \"all\"."),
			}),
			Handler: s.mcpListDecisions,
		},
		{
			Name:        "get_decision",
			Description: "Read one decision by its ID, such as DEC-005: its state, ruling and reason, what it supersedes and what superseded it, its amendments, who wrote it and who accepted it, and its full text.",
			Schema: objectSchema(map[string]any{
				"id": stringProp("The decision's ID, such as \"DEC-005\"."),
			}, "id"),
			Handler: s.mcpGetDecision,
		},
	}
}

func (s *Server) mcpCreateDecision(r *http.Request, args map[string]any) (any, error) {
	title, _ := argString(args, "title")
	owner, ok := argString(args, "owner")
	if !ok || strings.TrimSpace(owner) == "" {
		return nil, errors.New("say who the decision binds: \"project\", or an initiative's path or ID")
	}
	supersedes, err := argStrings(args, "supersedes")
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	req := NewDecision{Title: title, OwnerType: "project", Supersedes: supersedes, Actor: s.mcpActor(), Via: "mcp"}
	if owner != "project" {
		in, err := s.initiativeFromRef(ctx, owner)
		if err != nil {
			return nil, fmt.Errorf("there is no initiative %q; call get_tree to see what exists, or use \"project\"", owner)
		}
		req.OwnerType, req.OwnerID = "initiative", &in.ID
	}
	doc, err := s.CreateDecision(ctx, req)
	if err != nil {
		return nil, err
	}
	out := s.mcpDoc(ctx, *doc)
	out["next"] = fmt.Sprintf("%s is a draft at %s. Fill in its ruling, reason, context and alternatives, then hand it in with submit_for_review; a person accepts it.", doc.PublicID, doc.Path)
	return out, nil
}

func (s *Server) mcpListDecisions(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	owner, _ := argString(args, "owner")
	state, _ := argString(args, "state")
	if owner != "" && owner != "project" {
		in, err := s.initiativeFromRef(ctx, owner)
		if err != nil {
			return nil, fmt.Errorf("there is no initiative %q; call get_tree to see what exists", owner)
		}
		owner = in.PublicID
	}
	page, err := s.decisionsView(ctx, owner, state)
	if err != nil {
		return nil, err
	}
	entries := make([]map[string]any, 0, len(page.Rows))
	for _, row := range page.Rows {
		entries = append(entries, decisionEntry(row))
	}
	out := map[string]any{"state": page.State, "decisions": entries}
	if page.Branch != nil {
		out["for"] = page.OwnerName
		out["left_out"] = page.Branch.LeftOut
		out["told_tokens"] = page.Branch.Tokens
		out["max_tokens"] = page.Branch.MaxTokens
	}
	if page.Conventions != nil {
		out["conventions"] = map[string]any{"id": page.Conventions.PublicID, "path": page.Conventions.Path, "state": decisionState(page.Conventions.State)}
	}
	return out, nil
}

// decisionEntry is one decision in an MCP result (FR-7.3).
func decisionEntry(row decisionRow) map[string]any {
	e := map[string]any{
		"id": row.Doc.PublicID, "title": row.Doc.Title, "owner": row.OwnerLabel, "state": row.State,
		"revision": row.Doc.Revision, "path": row.Doc.Path,
		"supersedes": nonNil(row.Supersedes), "superseded_by": nonNil(row.SupersededBy),
		"told_as": row.Line,
	}
	if row.FromTitle {
		e["ruling"] = ""
		e["note"] = "No ruling is recorded, so agents are told its title."
	} else {
		e["ruling"], e["reason"] = row.Ruling, row.Reason
	}
	if len(row.Amendments) > 0 {
		e["amendments"] = row.Amendments
	}
	if row.LeftOut {
		e["left_out_somewhere"] = true
	}
	return e
}

func nonNil(xs []string) []string {
	if xs == nil {
		return []string{}
	}
	return xs
}

func (s *Server) mcpGetDecision(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	raw, _ := argString(args, "id")
	ref, ok := ident.Parse(strings.ToUpper(strings.TrimSpace(raw)))
	if !ok || ref.Shape != ident.ShapeDecision {
		return nil, fmt.Errorf("%q isn't a decision ID; give one such as DEC-005", raw)
	}
	row, err := s.decisionRowByID(ctx, ref.ID)
	if err != nil {
		return nil, err
	}
	out := decisionEntry(*row)
	for k, v := range s.mcpDoc(ctx, row.Doc) {
		if _, taken := out[k]; !taken {
			out[k] = v
		}
	}
	if text, err := s.readDocFile(row.Doc.Path); err == nil {
		out["text"] = string(text)
	}
	return out, nil
}

func (s *Server) decisionRowByID(ctx context.Context, id string) (*decisionRow, error) {
	rows, err := s.decisionRows(ctx)
	if err != nil {
		return nil, err
	}
	left, _ := s.leftOutAnywhere(ctx)
	for _, row := range rows {
		if row.Doc.PublicID == id {
			row.LeftOut = len(left[id]) > 0
			return &row, nil
		}
	}
	return nil, fmt.Errorf("there is no decision %s; call list_decisions with state \"all\" to see them", id)
}
