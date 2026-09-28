package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/store"
)

// The chat agent's checklist tools (SPEC-014 FR-7). Creating a checklist and
// adding, renaming, moving and removing its jobs are planning authoring, under
// DEC-004 and DESIGN-010 §5c and §17a item 4, so they need no quoted words.
// None of them ticks a job: that is a person's act, which the chat agent can
// only relay with the person's words (relay_tick_job, in mcp_relay.go;
// DEC-006 Amendment 1 decision 8).
//
// Checklists are named by id, or by exact name where no other checklist shares
// it; a job by id, or by its exact title within the named checklist. Each
// write calls the same audited store method the web UI calls, in one
// transaction, as the MCP actor, and signals the SSE hub.

// --- Resolving arguments ---

func (s *Server) checklistByRef(ctx context.Context, ref string) (*store.Checklist, error) {
	if id, err := uuid.Parse(ref); err == nil {
		return store.GetChecklist(ctx, s.Store.Pool, id)
	}
	return store.ChecklistByName(ctx, s.Store.Pool, ref)
}

func (s *Server) mcpChecklistArg(ctx context.Context, args map[string]any, key string) (*store.Checklist, error) {
	ref, ok := argString(args, key)
	if !ok {
		return nil, fmt.Errorf("say which checklist, by its id or its exact name, in %q", key)
	}
	c, err := s.checklistByRef(ctx, ref)
	if errors.Is(err, store.ErrNotFound) {
		return nil, fmt.Errorf("there is no checklist called or numbered %q; call list_checklists to see what exists", ref)
	}
	return c, err
}

// mcpJobArg resolves the checklist and one of its jobs. A job id from another
// checklist is refused, so a job is never changed through the wrong list.
func (s *Server) mcpJobArg(ctx context.Context, args map[string]any) (*store.Checklist, *store.Job, error) {
	c, err := s.mcpChecklistArg(ctx, args, "checklist")
	if err != nil {
		return nil, nil, err
	}
	ref, ok := argString(args, "job")
	if !ok {
		return nil, nil, errors.New("say which job, by its id or its exact title, in \"job\"; get_checklist lists them")
	}
	var j *store.Job
	if id, perr := uuid.Parse(ref); perr == nil {
		j, err = store.GetJob(ctx, s.Store.Pool, id)
		if err == nil && j.ChecklistID != c.ID {
			err = store.ErrNotFound
		}
	} else {
		j, err = store.JobByTitle(ctx, s.Store.Pool, c.ID, ref)
	}
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil, fmt.Errorf("the checklist %s has no job called or numbered %q; call get_checklist to see its jobs", c.Name, ref)
	}
	if err != nil {
		return nil, nil, err
	}
	return c, j, nil
}

// errWouldFinish refuses removing a checklist's last unticked job over MCP.
var errWouldFinish = errors.New("removing that job would leave the checklist done without anyone ticking it, " +
	"so you can't do it from here. If the person has done it, relay that with relay_tick_job and their words. " +
	"If it isn't needed after all, the person can remove it on the checklist's page")

// mcpChecklistError explains a store refusal in the words the agent relays.
func mcpChecklistError(err error) error {
	switch {
	case errors.Is(err, store.ErrJobTickedRename):
		return errors.New("that job is ticked, so its title can't change: the tick is for the job as it was. " +
			"You can still change its note")
	case errors.Is(err, store.ErrJobAlreadyTicked):
		return errors.New("that job is already ticked, so there is nothing to tick")
	case errors.Is(err, store.ErrJobNotTicked):
		return errors.New("that job isn't ticked, so there is nothing to untick")
	case errors.Is(err, store.ErrJobTitleBlank):
		return errors.New("a job needs a title, such as \"Get the API key\"")
	}
	return err
}

// --- What the tools return ---

func (s *Server) mcpJobSummary(j store.Job) map[string]any {
	out := map[string]any{
		"id": j.ID.String(), "position": j.Position, "title": j.Title, "note": j.Note, "ticked": j.Ticked(),
	}
	if j.Ticked() {
		if j.TickedBy != nil {
			out["ticked_by"] = *j.TickedBy
		}
		out["ticked_at"] = j.TickedAt.Format(time.RFC3339)
		if j.TickedVia != nil {
			out["ticked_via"] = *j.TickedVia
		}
		if j.TickedQuote != nil {
			out["ticked_quote"] = *j.TickedQuote
		}
	}
	return out
}

func (s *Server) mcpChecklistSummary(ctx context.Context, c *store.Checklist) (map[string]any, error) {
	st, err := store.GetChecklistStatus(ctx, s.Store.Pool, c.ID)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id": c.ID.String(), "name": c.Name, "description": c.Description,
		"owner":       s.mcpOwnerOf(ctx, c.OwnerType, c.OwnerID),
		"jobs_ticked": st.Ticked, "jobs_total": st.Jobs, "done": st.Done(),
		"url": checklistURL(c.ID),
	}, nil
}

// mcpChecklistDetail is get_checklist's answer: the summary, every job, and
// the milestones it is directly in.
func (s *Server) mcpChecklistDetail(ctx context.Context, c *store.Checklist) (map[string]any, error) {
	out, err := s.mcpChecklistSummary(ctx, c)
	if err != nil {
		return nil, err
	}
	jobs, err := store.Jobs(ctx, s.Store.Pool, c.ID)
	if err != nil {
		return nil, err
	}
	js := make([]any, 0, len(jobs))
	for _, j := range jobs {
		js = append(js, s.mcpJobSummary(j))
	}
	out["jobs"] = js
	ms, err := store.MilestonesForMember(ctx, s.Store.Pool, "checklist", c.ID)
	if err != nil {
		return nil, err
	}
	in := make([]any, 0, len(ms))
	for _, m := range ms {
		in = append(in, map[string]any{"id": m.ID.String(), "name": m.Name})
	}
	out["milestones"] = in
	return out, nil
}

// --- Writes (FR-7.1 to FR-7.5) ---

func (s *Server) mcpCreateChecklist(r *http.Request, args map[string]any) (any, error) {
	name, ok := argString(args, "name")
	if !ok {
		return nil, errors.New("a name is required, such as \"Launch paperwork\"")
	}
	description, _ := argString(args, "description")
	titles, err := argStrings(args, "jobs")
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	ownerType, ownerID, err := s.mcpPlanOwner(ctx, args)
	if err != nil {
		return nil, err
	}
	var c *store.Checklist
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		var e error
		if c, e = store.CreateChecklist(ctx, tx, ownerType, ownerID, name, description, s.mcpActor()); e != nil {
			return e
		}
		for _, t := range titles {
			if _, e := store.AddJob(ctx, tx, c.ID, t, "", s.mcpActor()); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		return nil, mcpChecklistError(err)
	}
	s.notifyEntityChanged("checklist", c.ID)
	return s.mcpChecklistDetail(ctx, c)
}

func (s *Server) mcpAddJob(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	c, err := s.mcpChecklistArg(ctx, args, "checklist")
	if err != nil {
		return nil, err
	}
	title, _ := argString(args, "title")
	note, _ := argString(args, "note")
	position, hasPosition, err := argInt(args, "position")
	if err != nil {
		return nil, err
	}
	err = s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		j, e := store.AddJob(ctx, tx, c.ID, title, note, s.mcpActor())
		if e != nil || !hasPosition {
			return e
		}
		_, e = store.PlaceJob(ctx, tx, j.ID, position, s.mcpActor())
		return e
	})
	if err != nil {
		return nil, mcpChecklistError(err)
	}
	s.notifyEntityChanged("checklist", c.ID)
	return s.mcpChecklistDetail(ctx, c)
}

func (s *Server) mcpRenameJob(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	c, j, err := s.mcpJobArg(ctx, args)
	if err != nil {
		return nil, err
	}
	title, _ := argString(args, "title")
	note := j.Note
	if n, ok := args["note"].(string); ok {
		note = n // given, even as empty, it replaces the note
	}
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := store.EditJob(ctx, tx, j.ID, title, note, s.mcpActor())
		return e
	}); err != nil {
		return nil, mcpChecklistError(err)
	}
	s.notifyEntityChanged("checklist", c.ID)
	return s.mcpChecklistDetail(ctx, c)
}

func (s *Server) mcpMoveJob(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	c, j, err := s.mcpJobArg(ctx, args)
	if err != nil {
		return nil, err
	}
	position, ok, err := argInt(args, "position")
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errors.New("say where the job goes in \"position\", counting from 1")
	}
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		_, e := store.PlaceJob(ctx, tx, j.ID, position, s.mcpActor())
		return e
	}); err != nil {
		return nil, mcpChecklistError(err)
	}
	s.notifyEntityChanged("checklist", c.ID)
	return s.mcpChecklistDetail(ctx, c)
}

func (s *Server) mcpRemoveJob(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	c, j, err := s.mcpJobArg(ctx, args)
	if err != nil {
		return nil, err
	}
	reason, _ := argString(args, "reason")
	if err := s.Store.WithTx(ctx, func(tx pgx.Tx) error {
		// Removing the last unticked job would make the checklist done with
		// nobody having ticked anything, which is exactly what the relay rule
		// protects. Only a person does that, on the checklist's page (SD-7).
		fin, e := store.WouldFinishChecklist(ctx, tx, j)
		if e != nil {
			return e
		}
		if fin {
			return errWouldFinish
		}
		_, e = store.RemoveJob(ctx, tx, j.ID, reason, s.mcpActor())
		return e
	}); err != nil {
		return nil, mcpChecklistError(err)
	}
	s.notifyEntityChanged("checklist", c.ID)
	return s.mcpChecklistDetail(ctx, c)
}

// --- Reads (FR-7.6, FR-7.7) ---

func (s *Server) mcpListChecklists(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	ownerType, ownerID, all, err := s.mcpListOwner(ctx, args)
	if err != nil {
		return nil, err
	}
	var cls []store.Checklist
	if all {
		cls, err = store.ListChecklists(ctx, s.Store.Pool)
	} else {
		cls, err = store.ChecklistsOwnedBy(ctx, s.Store.Pool, ownerType, ownerID)
	}
	if err != nil {
		return nil, err
	}
	out := make([]any, 0, len(cls))
	for i := range cls {
		sum, err := s.mcpChecklistSummary(ctx, &cls[i])
		if err != nil {
			return nil, err
		}
		out = append(out, sum)
	}
	return map[string]any{"checklists": out}, nil
}

func (s *Server) mcpGetChecklist(r *http.Request, args map[string]any) (any, error) {
	ctx := r.Context()
	c, err := s.mcpChecklistArg(ctx, args, "checklist")
	if err != nil {
		return nil, err
	}
	return s.mcpChecklistDetail(ctx, c)
}

// argStrings reads an optional list of strings. ok-less: absent is empty.
func argStrings(args map[string]any, key string) ([]string, error) {
	v, present := args[key]
	if !present || v == nil {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s must be a list of job titles", key)
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		t, ok := item.(string)
		if !ok || strings.TrimSpace(t) == "" {
			return nil, fmt.Errorf("every entry in %s must be a job title in words", key)
		}
		out = append(out, strings.TrimSpace(t))
	}
	return out, nil
}

// mcpChecklistTools is the SPEC-014 authoring part of the registry, kept
// beside its handlers. The one tool that ticks, relay_tick_job, is a relay and
// lives with the other relays.
func (s *Server) mcpChecklistTools() []mcpTool {
	checklistRef := stringProp("The checklist's id, or its exact name if no other checklist shares it.")
	jobRef := stringProp("The job's id, or its exact title if no other job on this checklist shares it.")
	return []mcpTool{
		{
			Name: "create_checklist",
			Description: "Create a checklist: a list of jobs only a person can do, such as getting an API " +
				"key, signing a contract or choosing an icon. It is done when every job is ticked, and it " +
				"can be added to a milestone with add_milestone_member so the release isn't complete " +
				"while a job is open. It belongs to the plan of an initiative (pass owner_path) or of the " +
				"whole project (leave owner_path out). You can give its first jobs now.",
			Schema: objectSchema(map[string]any{
				"name":        stringProp("The checklist's name, for example \"Launch paperwork\"."),
				"description": stringProp("Optional. What the jobs are for, in a sentence of plain prose."),
				"owner_path":  stringProp("Optional. The path of the initiative whose plan this is, for example \"auth\". Leave it out for a project-level checklist."),
				"jobs": map[string]any{"type": "array", "items": map[string]any{"type": "string"},
					"description": "Optional. The titles of its first jobs, in order, each a short instruction such as \"Get the API key\"."},
			}, "name"),
			Handler: s.mcpCreateChecklist,
		},
		{
			Name: "add_job",
			Description: "Add a job to a checklist: one thing only a person can do. It goes at the end " +
				"unless you give a position. A new job is unticked; only the person can say it is done, " +
				"and you pass that on with relay_tick_job.",
			Schema: objectSchema(map[string]any{
				"checklist": checklistRef,
				"title":     stringProp("What has to be done, as a short instruction, for example \"Get the API key\"."),
				"note":      stringProp("Optional. Anything worth knowing about it."),
				"position":  map[string]any{"type": "integer", "description": "Optional. Where it goes, counting from 1. Leave it out for the end."},
			}, "checklist", "title"),
			Handler: s.mcpAddJob,
		},
		{
			Name: "rename_job",
			Description: "Change a job's title, and optionally its note. This doesn't tick or untick it, and " +
				"a ticked job's title can't change, because the tick is for the job as it was. Pass note to " +
				"replace the note, or an empty note to clear it; leave note out to keep it.",
			Schema: objectSchema(map[string]any{
				"checklist": checklistRef,
				"job":       jobRef,
				"title":     stringProp("The new title."),
				"note":      stringProp("Optional. The new note, which replaces the old one."),
			}, "checklist", "job", "title"),
			Handler: s.mcpRenameJob,
		},
		{
			Name: "move_job",
			Description: "Move a job to a position on its checklist, counting from 1. The other jobs " +
				"keep their order around it.",
			Schema: objectSchema(map[string]any{
				"checklist": checklistRef,
				"job":       jobRef,
				"position":  map[string]any{"type": "integer", "description": "Where it goes, counting from 1."},
			}, "checklist", "job", "position"),
			Handler: s.mcpMoveJob,
		},
		{
			Name: "remove_job",
			Description: "Remove a job from a checklist, for example when it turns out not to be needed. " +
				"The reason, if given, is kept on the audit trail with the job. You can't remove the last " +
				"unticked job, because that would make the checklist done without anyone ticking it: if " +
				"the person has done it, relay that with relay_tick_job.",
			Schema: objectSchema(map[string]any{
				"checklist": checklistRef,
				"job":       jobRef,
				"reason":    stringProp("Optional. Why it is being removed, in a sentence."),
			}, "checklist", "job"),
			Handler: s.mcpRemoveJob,
		},
		{
			Name: "list_checklists",
			Description: "List checklists with their owner, how many of their jobs are ticked, and " +
				"whether each is done. Give owner_type (and owner_path for an initiative) to see only one " +
				"plan's checklists; leave both out to see every checklist in the project.",
			Schema: objectSchema(map[string]any{
				"owner_type": stringProp("Optional. \"project\" or \"initiative\"."),
				"owner_path": stringProp("The initiative's path, when owner_type is \"initiative\"."),
			}),
			Handler: s.mcpListChecklists,
		},
		{
			Name: "get_checklist",
			Description: "Return one checklist in detail: its owner, whether it is done, its jobs in order " +
				"with who ticked each one and when (and the person's words, when a tick was relayed), and " +
				"the milestones it is part of.",
			Schema: objectSchema(map[string]any{
				"checklist": checklistRef,
			}, "checklist"),
			Handler: s.mcpGetChecklist,
		},
	}
}
