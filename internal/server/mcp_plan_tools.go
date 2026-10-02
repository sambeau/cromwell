package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
	"subutai/internal/store"
)

// The chat agent's milestone and roadmap tools (SPEC-010 FR-7). They are
// planning authoring under DEC-004: each calls the same audited store method
// the web UI calls, in one transaction, as the MCP actor, and signals the SSE
// hub like the SPEC-008 tools. That includes marking a milestone as shipped
// (locking it) and reopening it: DEC-004 Amendment 1 permits both, because G4
// is a record-keeping check rather than a development gate, and both can be
// undone (SPEC-010 SD-4, SD-11). G4 still applies to the agent exactly as it
// does to a person.
//
// Milestones and roadmaps are named by id, or by exact name where that name is
// unique; initiatives and features by path, as the other tools do. Errors are
// sentences the agent can relay (SPEC-008 NFR-5).

// --- Resolving arguments ---

// mcpPlanOwner reads the owner of a new milestone or roadmap: an initiative by
// owner_path, or the project when owner_path is left out.
func (s *Server) mcpPlanOwner(ctx context.Context, args map[string]any) (string, *uuid.UUID, error) {
	path, ok := argString(args, "owner_path")
	if !ok {
		return "project", nil, nil
	}
	in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(path, "/"))
	if err != nil {
		return "", nil, fmt.Errorf("there is no initiative at %q to plan in; call get_tree to see what exists, "+
			"or leave owner_path out to plan at the project level", path)
	}
	return "initiative", &in.ID, nil
}

func (s *Server) mcpMilestoneArg(ctx context.Context, args map[string]any, key string) (*store.Milestone, error) {
	ref, ok := argString(args, key)
	if !ok {
		return nil, fmt.Errorf("say which milestone, by its id or its exact name, in %q", key)
	}
	m, err := s.milestoneByRef(ctx, ref)
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("there is no milestone called or numbered %q; call list_milestones to see what exists", ref)
	}
	return m, err
}

func (s *Server) mcpRoadmapArg(ctx context.Context, args map[string]any) (*store.Roadmap, error) {
	ref, ok := argString(args, "roadmap")
	if !ok {
		return nil, errors.New("say which roadmap, by its id or its exact name")
	}
	rm, err := s.roadmapByRef(ctx, ref)
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("there is no roadmap called or numbered %q; call list_roadmaps to see what exists", ref)
	}
	return rm, err
}

// mcpMemberArg resolves member_type and member to the member's id and a name
// to report. The type is explicit so an initiative and a feature whose paths
// look alike are never confused.
func (s *Server) mcpMemberArg(ctx context.Context, args map[string]any) (string, uuid.UUID, string, error) {
	memberType, _ := argString(args, "member_type")
	ref, ok := argString(args, "member")
	if !ok {
		return "", uuid.Nil, "", errors.New("say what to add or take out in \"member\": a feature or initiative path, or a checklist's or milestone's id or name")
	}
	if spikeRefRe.MatchString(ref) || memberType == "spike" {
		return "", uuid.Nil, "", errors.New(spikeNotDeliverableRefusal)
	}
	switch memberType {
	case "feature":
		f, _, err := s.featureByRef(ctx, ref)
		if err != nil {
			return "", uuid.Nil, "", fmt.Errorf("there is no feature at %q; call get_tree to see what exists", ref)
		}
		return memberType, f.ID, f.Name, nil
	case "bug":
		// A bug is a feature row and is stored as one; "bug" is only the
		// name the tool takes (SPEC-019 SD-13).
		b, err := s.bugByRef(ctx, ref)
		if err != nil {
			return "", uuid.Nil, "", err
		}
		return "feature", b.Feature.ID, b.Feature.PublicID + " " + b.Feature.Name, nil
	case "initiative":
		in, err := s.Store.InitiativeBySlugPath(ctx, strings.Split(ref, "/"))
		if err != nil {
			return "", uuid.Nil, "", fmt.Errorf("there is no initiative at %q; call get_tree to see what exists", ref)
		}
		return memberType, in.ID, in.Name, nil
	case "milestone":
		m, err := s.mcpMilestoneArg(ctx, args, "member")
		if err != nil {
			return "", uuid.Nil, "", err
		}
		return memberType, m.ID, m.Name, nil
	case "checklist":
		c, err := s.mcpChecklistArg(ctx, args, "member")
		if err != nil {
			return "", uuid.Nil, "", err
		}
		return memberType, c.ID, c.Name, nil
	}
	return "", uuid.Nil, "", fmt.Errorf("member_type must be \"feature\", \"bug\", \"initiative\", \"checklist\" or \"milestone\", not %q", memberType)
}

// argInt reads a whole-number argument. JSON numbers arrive as float64; a
// numeric string is accepted too. ok is false when absent.
func argInt(args map[string]any, key string) (int, bool, error) {
	v, present := args[key]
	if !present || v == nil {
		return 0, false, nil
	}
	switch n := v.(type) {
	case float64:
		if n != float64(int(n)) {
			return 0, true, fmt.Errorf("%s must be a whole number", key)
		}
		return int(n), true, nil
	case string:
		if strings.TrimSpace(n) == "" {
			return 0, false, nil
		}
		i, err := strconv.Atoi(strings.TrimSpace(n))
		if err != nil {
			return 0, true, fmt.Errorf("%s must be a whole number", key)
		}
		return i, true, nil
	}
	return 0, true, fmt.Errorf("%s must be a whole number", key)
}

// mcpPlanError explains a store refusal in the words the agent relays.
func mcpPlanError(err error) error {
	switch {
	case errors.Is(err, store.ErrMilestoneCycle):
		return errors.New("a milestone can't contain itself, directly or through another milestone inside it")
	case errors.Is(err, store.ErrBugNotAccepted):
		return errors.New("that bug hasn't been accepted in triage, so it isn't committed work yet; a person accepts it first, which you can relay with relay_triage")
	case strings.Contains(err.Error(), "membership is frozen"):
		return errors.New("that milestone is marked as shipped, so what it contains can't change; " +
			"if the person wants to change it, reopen it first with reopen_milestone")
	case strings.Contains(err.Error(), "already locked"):
		return errors.New("that milestone is already marked as shipped")
	case errors.Is(err, store.ErrMilestoneNotLocked):
		return errors.New("that milestone isn't marked as shipped, so there is nothing to reopen")
	}
	return err
}

// --- What the tools return ---

func (s *Server) mcpOwnerOf(ctx context.Context, ownerType string, ownerID *uuid.UUID) map[string]any {
	if ownerType != "initiative" || ownerID == nil {
		return map[string]any{"type": "project"}
	}
	out := map[string]any{"type": "initiative"}
	if path, err := s.initiativePath(ctx, *ownerID); err == nil {
		out["path"] = path
	}
	if in, err := store.GetInitiative(ctx, s.Store.Pool, *ownerID); err == nil {
		out["name"] = in.Name
	}
	return out
}

// publicOrRow is what a result calls a thing: its ID, or its row id if it
// has none (SPEC-017 SD-9). Lookups accept either.
func publicOrRow(publicID string, id uuid.UUID) string {
	if publicID != "" {
		return publicID
	}
	return id.String()
}

func (s *Server) mcpMilestoneSummary(ctx context.Context, m *store.Milestone) (map[string]any, error) {
	card, err := s.milestoneCardFor(ctx, *m)
	if err != nil {
		return nil, err
	}
	// The stored state "locked" is what people call shipped (SPEC-010 SD-11).
	state := string(m.State)
	if m.State == lifecycle.MilestoneLocked {
		state = "shipped"
	}
	out := map[string]any{
		"id": publicOrRow(m.PublicID, m.ID), "row_id": m.ID.String(), "name": m.Name, "description": m.Description,
		"state": state, "owner": s.mcpOwnerOf(ctx, m.OwnerType, m.OwnerID),
		"items_done": card.Done, "items_total": card.Total,
		"url": "/ui/m/" + m.ID.String(),
	}
	if m.TargetDate != nil {
		out["target_date"] = m.TargetDate.Format("2006-01-02")
	}
	return out, nil
}

func (s *Server) mcpRoadmapSummary(ctx context.Context, rm *store.Roadmap) (map[string]any, error) {
	entries, err := store.RoadmapEntries(ctx, s.Store.Pool, rm.ID)
	if err != nil {
		return nil, err
	}
	ms := make([]any, 0, len(entries))
	for i, e := range entries {
		m, err := store.GetMilestone(ctx, s.Store.Pool, e.MilestoneID)
		if err != nil {
			return nil, err
		}
		sum, err := s.mcpMilestoneSummary(ctx, m)
		if err != nil {
			return nil, err
		}
		sum["position"] = i + 1
		ms = append(ms, sum)
	}
	return map[string]any{
		"id": publicOrRow(rm.PublicID, rm.ID), "row_id": rm.ID.String(), "name": rm.Name, "owner": s.mcpOwnerOf(ctx, rm.OwnerType, rm.OwnerID),
		"milestones": ms, "url": "/ui/r/" + rm.ID.String(),
	}, nil
}

// --- Writes (FR-7.1 to FR-7.6) ---

func (s *Server) mcpCreateMilestone(r *http.Request, args map[string]any) (any, error) {
	name, ok := argString(args, "name")
	if !ok {
		return nil, errors.New("a name is required, such as \"Beta\" or \"First public release\"")
	}
	description, _ := argString(args, "description")
	var target *time.Time
	if td, ok := argString(args, "target_date"); ok {
		t, err := time.Parse("2006-01-02", td)
		if err != nil {
			return nil, fmt.Errorf("the target date %q isn't a date in the form YYYY-MM-DD, such as 2026-12-31", td)
		}
		target = &t
	}
	ctx := r.Context()
	ownerType, ownerID, err := s.mcpPlanOwner(ctx, args)
	if err != nil {
		return nil, err
	}
	var m *store.Milestone
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		m, e = store.CreateMilestone(ctx, tx, ownerType, ownerID, name, description, target, s.mcpActor())
		return e
	}); err != nil {
		return nil, err
	}
	s.notifyEntityChanged("milestone", m.ID)
	return s.mcpMilestoneSummary(ctx, m)
}

func (s *Server) mcpCreateRoadmap(r *http.Request, args map[string]any) (any, error) {
	name, ok := argString(args, "name")
	if !ok {
		return nil, errors.New("a name is required, such as \"The road to version one\"")
	}
	ctx := r.Context()
	ownerType, ownerID, err := s.mcpPlanOwner(ctx, args)
	if err != nil {
		return nil, err
	}
	var rm *store.Roadmap
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		rm, e = store.CreateRoadmap(ctx, tx, ownerType, ownerID, name, s.mcpActor())
		return e
	}); err != nil {
		return nil, err
	}
	s.notifyEntityChanged("roadmap", rm.ID)
	return s.mcpRoadmapSummary(ctx, rm)
}

func (s *Server) mcpAddMilestoneMember(r *http.Request, args map[string]any) (any, error) {
	return s.mcpMilestoneMemberChange(r, args, true)
}

func (s *Server) mcpRemoveMilestoneMember(r *http.Request, args map[string]any) (any, error) {
	return s.mcpMilestoneMemberChange(r, args, false)
}

func (s *Server) mcpMilestoneMemberChange(r *http.Request, args map[string]any, add bool) (any, error) {
	ctx := r.Context()
	m, err := s.mcpMilestoneArg(ctx, args, "milestone")
	if err != nil {
		return nil, err
	}
	memberType, memberID, memberName, err := s.mcpMemberArg(ctx, args)
	if err != nil {
		return nil, err
	}
	reason, _ := argString(args, "reason")
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		if add {
			return store.AddMember(ctx, tx, m.ID, memberType, memberID, s.mcpActor())
		}
		return store.RemoveMember(ctx, tx, m.ID, memberType, memberID, reason, s.mcpActor())
	})
	if errors.Is(err, store.ErrNotFound) && !add {
		return nil, fmt.Errorf("%s isn't directly in the milestone %s, so there is nothing to take out", memberName, m.Name)
	}
	if err != nil {
		return nil, mcpPlanError(err)
	}
	s.notifyEntityChanged("milestone", m.ID)
	out, err := s.mcpMilestoneSummary(ctx, m)
	if err != nil {
		return nil, err
	}
	verb := "added"
	if !add {
		verb = "taken out"
	}
	out["change"] = fmt.Sprintf("%s %s was %s.", memberType, memberName, verb)
	return out, nil
}

func (s *Server) mcpPlaceRoadmapEntry(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	rm, err := s.mcpRoadmapArg(ctx, args)
	if err != nil {
		return nil, err
	}
	m, err := s.mcpMilestoneArg(ctx, args, "milestone")
	if err != nil {
		return nil, err
	}
	position, _, err := argInt(args, "position")
	if err != nil {
		return nil, err
	}
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := store.PlaceRoadmapEntry(ctx, tx, rm.ID, m.ID, position, s.mcpActor())
		return e
	}); err != nil {
		return nil, err
	}
	s.notifyEntityChanged("roadmap", rm.ID)
	return s.mcpRoadmapSummary(ctx, rm)
}

func (s *Server) mcpRemoveRoadmapEntry(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	rm, err := s.mcpRoadmapArg(ctx, args)
	if err != nil {
		return nil, err
	}
	m, err := s.mcpMilestoneArg(ctx, args, "milestone")
	if err != nil {
		return nil, err
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.RemoveRoadmapEntry(ctx, tx, rm.ID, m.ID, s.mcpActor())
	})
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("the milestone %s isn't on the roadmap %s", m.Name, rm.Name)
	}
	if err != nil {
		return nil, err
	}
	s.notifyEntityChanged("roadmap", rm.ID)
	return s.mcpRoadmapSummary(ctx, rm)
}

// --- Reads (FR-7.7, FR-7.8) ---

// mcpListOwner reads the optional owner filter of the list tools, with the
// same owner_type and owner_path pair list_documents uses (SPEC-008). all is
// true when no owner was given.
func (s *Server) mcpListOwner(ctx context.Context, args map[string]any) (ownerType string, ownerID *uuid.UUID, all bool, err error) {
	ownerType, ok := argString(args, "owner_type")
	if !ok {
		return "", nil, true, nil
	}
	switch ownerType {
	case "project":
		return "project", nil, false, nil
	case "initiative":
		path, _ := argString(args, "owner_path")
		id, err := s.mcpResolveOwner(ctx, "initiative", path)
		return "initiative", id, false, err
	}
	return "", nil, false, fmt.Errorf("owner_type must be \"project\" or \"initiative\", not %q; milestones and roadmaps aren't planned in a feature", ownerType)
}

func (s *Server) mcpListMilestones(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	ownerType, ownerID, all, err := s.mcpListOwner(ctx, args)
	if err != nil {
		return nil, err
	}
	var ms []store.Milestone
	if all {
		ms, err = store.ListMilestones(ctx, s.Store.Pool)
	} else {
		ms, err = store.MilestonesOwnedBy(ctx, s.Store.Pool, ownerType, ownerID)
	}
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(ms))
	for i := range ms {
		sum, err := s.mcpMilestoneSummary(ctx, &ms[i])
		if err != nil {
			return nil, err
		}
		out = append(out, sum)
	}
	return map[string]any{"milestones": out}, nil
}

func (s *Server) mcpListRoadmaps(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	ownerType, ownerID, all, err := s.mcpListOwner(ctx, args)
	if err != nil {
		return nil, err
	}
	var rms []store.Roadmap
	if all {
		rms, err = store.ListRoadmaps(ctx, s.Store.Pool)
	} else {
		rms, err = store.RoadmapsOwnedBy(ctx, s.Store.Pool, ownerType, ownerID)
	}
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(rms))
	for i := range rms {
		sum, err := s.mcpRoadmapSummary(ctx, &rms[i])
		if err != nil {
			return nil, err
		}
		out = append(out, sum)
	}
	return map[string]any{"roadmaps": out}, nil
}

func (s *Server) mcpGetMilestone(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	m, err := s.mcpMilestoneArg(ctx, args, "milestone")
	if err != nil {
		return nil, err
	}
	out, err := s.mcpMilestoneSummary(ctx, m)
	if err != nil {
		return nil, err
	}
	card, err := s.milestoneCardFor(ctx, *m)
	if err != nil {
		return nil, err
	}
	out["tokens_done"], out["tokens_estimated"] = card.DoneTokens, card.TotalTokens
	rows, err := s.milestoneMemberRows(ctx, m.ID)
	if err != nil {
		return nil, err
	}
	members := make([]any, 0, len(rows))
	for _, row := range rows {
		mem := map[string]any{"type": row.Kind, "name": row.Label, "done": row.Done}
		if row.PublicID != "" {
			mem["id"] = row.PublicID
		}
		if row.Kind == "milestone" || row.Kind == "checklist" {
			mem["id"] = publicOrRow(row.PublicID, row.ID)
			mem["row_id"] = row.ID.String()
		} else {
			mem["path"] = strings.TrimPrefix(strings.TrimPrefix(row.URL, "/ui/f/"), "/ui/i/")
		}
		members = append(members, mem)
	}
	out["members"] = members
	if m.State == lifecycle.MilestoneOpen {
		prog, err := store.LiveProgress(ctx, s.Store.Pool, m.ID)
		if err != nil {
			return nil, err
		}
		g := lifecycle.G4(prog.Total, prog.Done)
		shipping := map[string]any{"could_mark_shipped_now": g.Pass,
			"how": "Mark it as shipped with mark_milestone_shipped when the person says the release has gone out. It can be undone with reopen_milestone."}
		if !g.Pass {
			shipping["why_not"] = g.Reason
		}
		out["shipping"] = shipping
	} else if m.LockedAt != nil {
		out["shipped_at"] = m.LockedAt.Format(time.RFC3339)
	}
	return out, nil
}

func (s *Server) mcpGetRoadmap(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	rm, err := s.mcpRoadmapArg(ctx, args)
	if err != nil {
		return nil, err
	}
	return s.mcpRoadmapSummary(ctx, rm)
}

// --- Shipping (DEC-004 Amendment 1, SPEC-010 FR-7.10) ---

// mcpMarkMilestoneShipped calls the same LockMilestone the web UI's "Mark as
// shipped" calls. G4 applies exactly as it does to a person: when it refuses,
// its own sentence goes back to the agent and nothing changes.
func (s *Server) mcpMarkMilestoneShipped(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	m, err := s.mcpMilestoneArg(ctx, args, "milestone")
	if err != nil {
		return nil, err
	}
	var g lifecycle.GateResult
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		g, e = store.LockMilestone(ctx, tx, m.ID, s.mcpActor())
		return e
	})
	if err != nil {
		if g.Gate == lifecycle.GateG4 && !g.Pass {
			return nil, errors.New(g.Reason)
		}
		return nil, mcpPlanError(err)
	}
	s.notifyEntityChanged("milestone", m.ID)
	shipped, err := store.GetMilestone(ctx, s.Store.Pool, m.ID)
	if err != nil {
		return nil, err
	}
	out, err := s.mcpMilestoneSummary(ctx, shipped)
	if err != nil {
		return nil, err
	}
	if shipped.LockedAt != nil {
		out["shipped_at"] = shipped.LockedAt.Format(time.RFC3339)
	}
	out["change"] = "The milestone is marked as shipped. What it contains is now fixed; " +
		"reopen_milestone undoes this if it was a mistake."
	return out, nil
}

// mcpReopenMilestone calls UnlockMilestone, as the web UI's "Reopen" does. The
// audit row keeps the record it replaces and the reason.
func (s *Server) mcpReopenMilestone(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	m, err := s.mcpMilestoneArg(ctx, args, "milestone")
	if err != nil {
		return nil, err
	}
	reason, _ := argString(args, "reason")
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		return store.UnlockMilestone(ctx, tx, m.ID, reason, s.mcpActor())
	}); err != nil {
		return nil, mcpPlanError(err)
	}
	s.notifyEntityChanged("milestone", m.ID)
	reopened, err := store.GetMilestone(ctx, s.Store.Pool, m.ID)
	if err != nil {
		return nil, err
	}
	out, err := s.mcpMilestoneSummary(ctx, reopened)
	if err != nil {
		return nil, err
	}
	out["change"] = "The milestone is open again, and what it contains is live. " +
		"The history still shows when it was marked as shipped."
	return out, nil
}

// mcpPlanTools is the SPEC-010 part of the registry, kept beside its handlers.
// The last two, shipping and reopening, came with DEC-004 Amendment 1 (SD-4).
func (s *Server) mcpPlanTools() []mcpTool {
	milestoneRef := stringProp("The milestone's id, or its exact name if no other milestone shares it.")
	roadmapRef := stringProp("The roadmap's id, or its exact name if no other roadmap shares it.")
	return []mcpTool{
		{
			Name: "create_milestone",
			Description: "Create a milestone: a set of work that ships together, such as a release. " +
				"It belongs to the plan of an initiative (pass owner_path) or of the whole project " +
				"(leave owner_path out). What it contains can come from anywhere in the project; add " +
				"that with add_milestone_member.",
			Schema: objectSchema(map[string]any{
				"name":        stringProp("The milestone's name, for example \"Beta\"."),
				"description": stringProp("Optional. What reaching it means, in a sentence of plain prose."),
				"target_date": stringProp("Optional. The date it is aiming for, as YYYY-MM-DD."),
				"owner_path":  stringProp("Optional. The path of the initiative whose plan this is, for example \"auth\". Leave it out for a project-level milestone."),
			}, "name"),
			Handler: s.mcpCreateMilestone,
		},
		{
			Name: "create_roadmap",
			Description: "Create a roadmap: an ordered list of milestones, where the order means " +
				"whatever the planner intends — sequence, priority or schedule. It belongs to the plan " +
				"of an initiative (pass owner_path) or of the whole project. Put milestones on it with " +
				"place_roadmap_entry.",
			Schema: objectSchema(map[string]any{
				"name":       stringProp("The roadmap's name, for example \"The road to version one\"."),
				"owner_path": stringProp("Optional. The path of the initiative whose plan this is. Leave it out for a project-level roadmap."),
			}, "name"),
			Handler: s.mcpCreateRoadmap,
		},
		{
			Name: "add_milestone_member",
			Description: "Add a feature, an initiative, a checklist or another milestone to a milestone. An " +
				"initiative brings in every feature under it, including ones created later, but not the " +
				"checklists planned in it; add a checklist itself. A milestone isn't complete while one of " +
				"its checklists has an unticked job. A milestone can't contain itself, and one marked as " +
				"shipped can't change.",
			Schema: objectSchema(map[string]any{
				"milestone":   milestoneRef,
				"member_type": stringProp("What is being added: \"feature\", \"bug\" (an accepted bug, by its ID), \"initiative\", \"checklist\" or \"milestone\"."),
				"member":      stringProp("For a feature or initiative, its path, such as \"auth/login\". For a checklist or a milestone, its id or exact name."),
			}, "milestone", "member_type", "member"),
			Handler: s.mcpAddMilestoneMember,
		},
		{
			Name: "remove_milestone_member",
			Description: "Take a feature, an initiative, a checklist or a milestone out of a milestone, " +
				"for example when it is descoped from a release. The reason, if given, is kept on the " +
				"audit trail. A milestone marked as shipped can't change.",
			Schema: objectSchema(map[string]any{
				"milestone":   milestoneRef,
				"member_type": stringProp("What is being taken out: \"feature\", \"bug\", \"initiative\", \"checklist\" or \"milestone\"."),
				"member":      stringProp("For a feature or initiative, its path. For a checklist or a milestone, its id or exact name."),
				"reason":      stringProp("Optional. Why it is coming out, in a sentence."),
			}, "milestone", "member_type", "member"),
			Handler: s.mcpRemoveMilestoneMember,
		},
		{
			Name: "place_roadmap_entry",
			Description: "Put a milestone on a roadmap at a position, or move it if it is already " +
				"there. Positions count from 1; leave position out to put it at the end. The other " +
				"milestones keep their order around it. This is also how to reorder a roadmap.",
			Schema: objectSchema(map[string]any{
				"roadmap":   roadmapRef,
				"milestone": milestoneRef,
				"position":  map[string]any{"type": "integer", "description": "Optional. Where it goes, counting from 1. Leave it out for the end."},
			}, "roadmap", "milestone"),
			Handler: s.mcpPlaceRoadmapEntry,
		},
		{
			Name: "remove_roadmap_entry",
			Description: "Take a milestone off a roadmap. The milestone itself is not changed, and " +
				"the rest of the roadmap keeps its order.",
			Schema: objectSchema(map[string]any{
				"roadmap":   roadmapRef,
				"milestone": milestoneRef,
			}, "roadmap", "milestone"),
			Handler: s.mcpRemoveRoadmapEntry,
		},
		{
			Name: "list_milestones",
			Description: "List milestones with their owner, state and how many of their items are " +
				"done. Give owner_type (and owner_path for an initiative) to see only one plan's " +
				"milestones; leave both out to see every milestone in the project.",
			Schema: objectSchema(map[string]any{
				"owner_type": stringProp("Optional. \"project\" or \"initiative\"."),
				"owner_path": stringProp("The initiative's path, when owner_type is \"initiative\"."),
			}),
			Handler: s.mcpListMilestones,
		},
		{
			Name: "list_roadmaps",
			Description: "List roadmaps with their owner and their milestones in order. Give " +
				"owner_type (and owner_path for an initiative) to see only one plan's roadmaps; leave " +
				"both out to see every roadmap in the project.",
			Schema: objectSchema(map[string]any{
				"owner_type": stringProp("Optional. \"project\" or \"initiative\"."),
				"owner_path": stringProp("The initiative's path, when owner_type is \"initiative\"."),
			}),
			Handler: s.mcpListRoadmaps,
		},
		{
			Name: "get_milestone",
			Description: "Return one milestone in detail: its owner, state and target date, what it " +
				"contains and which of those are done, its progress both as items and as estimated " +
				"tokens, and whether it could be marked as shipped now — with the reason if not, so " +
				"you can tell the person.",
			Schema: objectSchema(map[string]any{
				"milestone": milestoneRef,
			}, "milestone"),
			Handler: s.mcpGetMilestone,
		},
		{
			Name:        "get_roadmap",
			Description: "Return one roadmap in detail: its owner and its milestones in order, each with its state and progress.",
			Schema: objectSchema(map[string]any{
				"roadmap": roadmapRef,
			}, "roadmap"),
			Handler: s.mcpGetRoadmap,
		},
		{
			Name: "mark_milestone_shipped",
			Description: "Mark a milestone as shipped, when the person tells you the release has gone " +
				"out. This records exactly which items the milestone covers now — its features and " +
				"checklists — and stops what it contains from changing, so work added later under its " +
				"initiatives doesn't rewrite the record. Items not done yet are recorded as not " +
				"shipped. It is refused, with the reason, until at least one item is done. It can be " +
				"undone with reopen_milestone.",
			Schema: objectSchema(map[string]any{
				"milestone": milestoneRef,
			}, "milestone"),
			Handler: s.mcpMarkMilestoneShipped,
		},
		{
			Name: "reopen_milestone",
			Description: "Reopen a milestone that was marked as shipped, for example because it was " +
				"marked by mistake or something needs to be added to the release after all. This " +
				"throws away the shipped record and makes what it contains live again. The history " +
				"keeps the record it replaced and the reason you give.",
			Schema: objectSchema(map[string]any{
				"milestone": milestoneRef,
				"reason":    stringProp("Optional. Why it is being reopened, in a sentence."),
			}, "milestone"),
			Handler: s.mcpReopenMilestone,
		},
	}
}
