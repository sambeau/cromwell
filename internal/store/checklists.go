package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Checklists and jobs (SPEC-014, DESIGN-010 §4 and §6). A job is one thing
// only a person can do, ticked by hand; a checklist is a list of jobs, done
// when it has at least one job and every job is ticked. Checklists are owned
// like milestones and roadmaps (the project or an initiative), and a checklist
// can be a milestone deliverable.
//
// Every job event is audited against its checklist (ref_type 'checklist'), with
// the job's id and title in the payload, so a checklist's history is one query
// and survives the job being removed.

type Checklist struct {
	ID          uuid.UUID
	Name        string
	Description string
	OwnerType   string
	OwnerID     *uuid.UUID
	CreatedAt   time.Time
	// PublicID is the minted ID, "CL-002" (SPEC-015 FR-1.2, SPEC-017 FR-3).
	PublicID string
}

const checklistCols = `id, name, description, owner_type, owner_id, created_at, COALESCE(public_id, '')`

func scanChecklist(row pgx.Row) (*Checklist, error) {
	var c Checklist
	err := row.Scan(&c.ID, &c.Name, &c.Description, &c.OwnerType, &c.OwnerID, &c.CreatedAt, &c.PublicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func scanChecklists(rows pgx.Rows) ([]Checklist, error) {
	defer rows.Close()
	var out []Checklist
	for rows.Next() {
		c, err := scanChecklist(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// Job is one entry on a checklist. TickedBy, TickedAt, TickedVia and
// TickedQuote are all nil while it is unticked (SPEC-014 SD-8).
type Job struct {
	ID          uuid.UUID
	ChecklistID uuid.UUID
	Title       string
	Note        string
	Position    int
	TickedBy    *string
	TickedAt    *time.Time
	TickedVia   *string
	TickedQuote *string
	CreatedAt   time.Time
}

// Ticked reports whether the job is ticked.
func (j Job) Ticked() bool { return j.TickedAt != nil }

const jobCols = `id, checklist_id, title, note, position, ticked_by, ticked_at, ticked_via, ticked_quote, created_at`

func scanJob(row pgx.Row) (*Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.ChecklistID, &j.Title, &j.Note, &j.Position,
		&j.TickedBy, &j.TickedAt, &j.TickedVia, &j.TickedQuote, &j.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &j, err
}

var (
	// ErrJobAlreadyTicked and ErrJobNotTicked refuse a tick that changes
	// nothing, so a relayed tick of a job someone already ticked says so
	// rather than overwriting who ticked it.
	ErrJobAlreadyTicked = errors.New("that job is already ticked")
	ErrJobNotTicked     = errors.New("that job isn't ticked")
	// ErrJobTitleBlank refuses a job with nothing to do.
	ErrJobTitleBlank = errors.New("a job needs a title")
	// ErrJobTickedRename refuses changing what a ticked job says: the tick
	// vouches for the job as it was titled, so renaming it would make the tick
	// vouch for something nobody ticked (REVIEW-014 R14-2).
	ErrJobTickedRename = errors.New("a ticked job's title can't change; untick it first, or add a new job")
	// ErrRelayNeedsQuote refuses a tick through the chat agent without the
	// person's words (DEC-006 Amendment 1).
	ErrRelayNeedsQuote = errors.New("a tick through the chat agent needs the person's words")
)

// --- Checklists ---

// CreateChecklist creates a checklist owned by the project (ownerType
// "project", ownerID nil) or an initiative (SPEC-014 FR-1.2). Ownership says
// whose plan it belongs to; it never constrains which milestones it can be in
// (D-12).
func CreateChecklist(ctx context.Context, tx pgx.Tx, ownerType string, ownerID *uuid.UUID, name, description, actor string) (*Checklist, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("a checklist needs a name")
	}
	if err := checkPlanOwner(ownerType, ownerID); err != nil {
		return nil, err
	}
	description = strings.TrimSpace(description)
	c := &Checklist{ID: NewID(), Name: name, Description: description, OwnerType: ownerType, OwnerID: ownerID}
	if err := tx.QueryRow(ctx, `
		INSERT INTO checklists (id, name, description, owner_type, owner_id)
		VALUES ($1, $2, $3, $4, $5) RETURNING created_at, COALESCE(public_id, '')`,
		c.ID, name, description, ownerType, ownerID).Scan(&c.CreatedAt, &c.PublicID); err != nil {
		return nil, err
	}
	if err := Audit(ctx, tx, actor, "checklist.created", "checklist", &c.ID,
		ownerPayload(map[string]any{"name": name}, ownerType, ownerID)); err != nil {
		return nil, err
	}
	return c, nil
}

// ChecklistByPublicID finds a checklist by its ID, "CL-002".
func ChecklistByPublicID(ctx context.Context, q Querier, publicID string) (*Checklist, error) {
	return scanChecklist(q.QueryRow(ctx, `SELECT `+checklistCols+` FROM checklists WHERE public_id = $1`, publicID))
}

func GetChecklist(ctx context.Context, q Querier, id uuid.UUID) (*Checklist, error) {
	return scanChecklist(q.QueryRow(ctx, `SELECT `+checklistCols+` FROM checklists WHERE id = $1`, id))
}

// ChecklistByName resolves a checklist by its exact name; it errors if the
// name is absent or shared, so the caller can ask for the id.
func ChecklistByName(ctx context.Context, q Querier, name string) (*Checklist, error) {
	rows, err := q.Query(ctx, `SELECT `+checklistCols+` FROM checklists WHERE name = $1`, name)
	if err != nil {
		return nil, err
	}
	found, err := scanChecklists(rows)
	if err != nil {
		return nil, err
	}
	switch len(found) {
	case 0:
		return nil, ErrNotFound
	case 1:
		return &found[0], nil
	default:
		return nil, fmt.Errorf("%d checklists are called %q; use its id", len(found), name)
	}
}

// ListChecklists returns every checklist, oldest first.
func ListChecklists(ctx context.Context, q Querier) ([]Checklist, error) {
	rows, err := q.Query(ctx, `SELECT `+checklistCols+` FROM checklists ORDER BY created_at, id`)
	if err != nil {
		return nil, err
	}
	return scanChecklists(rows)
}

// ChecklistsOwnedBy lists the checklists planned directly in an entity's plan,
// oldest first. ownerID is nil for the project.
func ChecklistsOwnedBy(ctx context.Context, q Querier, ownerType string, ownerID *uuid.UUID) ([]Checklist, error) {
	rows, err := q.Query(ctx, `
		SELECT `+checklistCols+` FROM checklists
		WHERE owner_type = $1 AND owner_id IS NOT DISTINCT FROM $2
		ORDER BY created_at, id`, ownerType, ownerID)
	if err != nil {
		return nil, err
	}
	return scanChecklists(rows)
}

// ChecklistStatus is a checklist's counts: its jobs, how many are ticked, and
// whether it is done — at least one job, every one ticked (SPEC-014 SD-9).
type ChecklistStatus struct {
	Jobs   int
	Ticked int
}

// Done reports whether the checklist is done.
func (s ChecklistStatus) Done() bool { return s.Jobs > 0 && s.Ticked == s.Jobs }

func GetChecklistStatus(ctx context.Context, q Querier, checklistID uuid.UUID) (ChecklistStatus, error) {
	var s ChecklistStatus
	err := q.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE ticked_at IS NOT NULL)
		FROM jobs WHERE checklist_id = $1`, checklistID).Scan(&s.Jobs, &s.Ticked)
	return s, err
}

// --- Jobs ---

func GetJob(ctx context.Context, q Querier, id uuid.UUID) (*Job, error) {
	return scanJob(q.QueryRow(ctx, `SELECT `+jobCols+` FROM jobs WHERE id = $1`, id))
}

// Jobs returns a checklist's jobs in order.
func Jobs(ctx context.Context, q Querier, checklistID uuid.UUID) ([]Job, error) {
	rows, err := q.Query(ctx, `
		SELECT `+jobCols+` FROM jobs WHERE checklist_id = $1
		ORDER BY position, created_at, id`, checklistID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *j)
	}
	return out, rows.Err()
}

// JobByTitle resolves a job by its exact title within one checklist. Two jobs
// with the same title can't be told apart this way, so that is an error
// asking for the id.
func JobByTitle(ctx context.Context, q Querier, checklistID uuid.UUID, title string) (*Job, error) {
	jobs, err := Jobs(ctx, q, checklistID)
	if err != nil {
		return nil, err
	}
	var found []Job
	for _, j := range jobs {
		if j.Title == title {
			found = append(found, j)
		}
	}
	switch len(found) {
	case 0:
		return nil, ErrNotFound
	case 1:
		return &found[0], nil
	default:
		return nil, fmt.Errorf("%d jobs on this checklist are called %q; use the job's id", len(found), title)
	}
}

// jobAudit writes a job event against its checklist, with the job named.
func jobAudit(ctx context.Context, tx pgx.Tx, actor, kind string, j *Job, extra map[string]any) error {
	p := map[string]any{"job_id": j.ID.String(), "title": j.Title}
	for k, v := range extra {
		p[k] = v
	}
	return Audit(ctx, tx, actor, kind, "checklist", &j.ChecklistID, p)
}

// lockChecklist takes a row lock on the checklist, so two writers renumbering
// its jobs at once can't interleave.
func lockChecklist(ctx context.Context, tx pgx.Tx, checklistID uuid.UUID) error {
	var id uuid.UUID
	err := tx.QueryRow(ctx, `SELECT id FROM checklists WHERE id = $1 FOR UPDATE`, checklistID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// AddJob puts a new job at the end of a checklist (SPEC-014 FR-1.3).
func AddJob(ctx context.Context, tx pgx.Tx, checklistID uuid.UUID, title, note, actor string) (*Job, error) {
	title, note = strings.TrimSpace(title), strings.TrimSpace(note)
	if title == "" {
		return nil, ErrJobTitleBlank
	}
	if err := lockChecklist(ctx, tx, checklistID); err != nil {
		return nil, err
	}
	j := &Job{ID: NewID(), ChecklistID: checklistID, Title: title, Note: note}
	if err := tx.QueryRow(ctx, `
		INSERT INTO jobs (id, checklist_id, title, note, position)
		VALUES ($1, $2, $3, $4, (SELECT COALESCE(MAX(position), 0) + 1 FROM jobs WHERE checklist_id = $2))
		RETURNING position, created_at`,
		j.ID, checklistID, title, note).Scan(&j.Position, &j.CreatedAt); err != nil {
		return nil, err
	}
	extra := map[string]any{"position": j.Position}
	if note != "" {
		extra["note"] = note
	}
	if err := jobAudit(ctx, tx, actor, "job.added", j, extra); err != nil {
		return nil, err
	}
	return j, nil
}

// EditJob changes a job's title and note (SPEC-014 FR-1.3). The audit row
// keeps the old and new values of both. A ticked job's note can change, but
// not its title (ErrJobTickedRename).
func EditJob(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, title, note, actor string) (*Job, error) {
	title, note = strings.TrimSpace(title), strings.TrimSpace(note)
	if title == "" {
		return nil, ErrJobTitleBlank
	}
	old, err := scanJob(tx.QueryRow(ctx, `SELECT `+jobCols+` FROM jobs WHERE id = $1 FOR UPDATE`, jobID))
	if err != nil {
		return nil, err
	}
	if old.Ticked() && title != old.Title {
		return nil, ErrJobTickedRename
	}
	if _, err := tx.Exec(ctx, `UPDATE jobs SET title = $2, note = $3 WHERE id = $1`, jobID, title, note); err != nil {
		return nil, err
	}
	j := *old
	j.Title, j.Note = title, note
	if err := jobAudit(ctx, tx, actor, "job.edited", &j, map[string]any{
		"old_title": old.Title, "old_note": old.Note, "note": note}); err != nil {
		return nil, err
	}
	return &j, nil
}

// PlaceJob moves a job to a 1-based place in its checklist and renumbers every
// job 1…n (SPEC-014 FR-1.3). A place outside 1…n means the end, as
// PlaceRoadmapEntry does. It returns the final place.
func PlaceJob(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, place int, actor string) (int, error) {
	j, err := GetJob(ctx, tx, jobID)
	if err != nil {
		return 0, err
	}
	if err := lockChecklist(ctx, tx, j.ChecklistID); err != nil {
		return 0, err
	}
	jobs, err := Jobs(ctx, tx, j.ChecklistID)
	if err != nil {
		return 0, err
	}
	order := make([]uuid.UUID, 0, len(jobs))
	for _, o := range jobs {
		if o.ID != jobID {
			order = append(order, o.ID)
		}
	}
	idx := place - 1
	if place <= 0 || idx > len(order) {
		idx = len(order)
	}
	order = append(order, uuid.Nil)
	copy(order[idx+1:], order[idx:])
	order[idx] = jobID
	if err := writeJobOrder(ctx, tx, order); err != nil {
		return 0, err
	}
	final := idx + 1
	if err := jobAudit(ctx, tx, actor, "job.moved", j, map[string]any{
		"from": j.Position, "position": final}); err != nil {
		return 0, err
	}
	return final, nil
}

// RemoveJob deletes a job and renumbers the rest (SPEC-014 FR-1.3). The audit
// row keeps its title, note, whether it was ticked, and the reason.
func RemoveJob(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, reason, actor string) (*Job, error) {
	j, err := GetJob(ctx, tx, jobID)
	if err != nil {
		return nil, err
	}
	if err := lockChecklist(ctx, tx, j.ChecklistID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM jobs WHERE id = $1`, jobID); err != nil {
		return nil, err
	}
	rest, err := Jobs(ctx, tx, j.ChecklistID)
	if err != nil {
		return nil, err
	}
	order := make([]uuid.UUID, len(rest))
	for i, o := range rest {
		order[i] = o.ID
	}
	if err := writeJobOrder(ctx, tx, order); err != nil {
		return nil, err
	}
	if err := jobAudit(ctx, tx, actor, "job.removed", j, map[string]any{
		"note": j.Note, "was_ticked": j.Ticked(), "reason": strings.TrimSpace(reason)}); err != nil {
		return nil, err
	}
	return j, nil
}

// writeJobOrder stores an order as dense positions 1…n.
func writeJobOrder(ctx context.Context, tx pgx.Tx, order []uuid.UUID) error {
	for i, id := range order {
		if _, err := tx.Exec(ctx, `UPDATE jobs SET position = $2 WHERE id = $1`, id, i+1); err != nil {
			return err
		}
	}
	return nil
}

// TickJob ticks a job (tick true) or unticks it (tick false), in the caller's
// transaction, with an audit row (SPEC-014 FR-1.3). via is "ui" or "mcp";
// quote is the person's words when the chat agent relays the act (DEC-006
// Amendment 1), and empty otherwise. A non-blank note replaces the job's note;
// a blank one leaves it (SD-6). Ticking a ticked job or unticking an unticked
// one is refused and writes nothing.
func TickJob(ctx context.Context, tx pgx.Tx, jobID uuid.UUID, tick bool, note, via, quote, actor string) (*Job, error) {
	note, quote = strings.TrimSpace(note), strings.TrimSpace(quote)
	if via != "ui" && via != "mcp" {
		return nil, fmt.Errorf("a tick comes through the web UI or the chat agent, not %q", via)
	}
	if via == "mcp" && quote == "" {
		return nil, ErrRelayNeedsQuote
	}
	// Lock the row so two ticks of the same job can't both succeed.
	j, err := scanJob(tx.QueryRow(ctx, `SELECT `+jobCols+` FROM jobs WHERE id = $1 FOR UPDATE`, jobID))
	if err != nil {
		return nil, err
	}
	switch {
	case tick && j.Ticked():
		return nil, ErrJobAlreadyTicked
	case !tick && !j.Ticked():
		return nil, ErrJobNotTicked
	}
	newNote := j.Note
	if note != "" {
		newNote = note
	}
	var quoteVal *string
	if quote != "" {
		quoteVal = &quote
	}
	if tick {
		err = tx.QueryRow(ctx, `
			UPDATE jobs SET ticked_by = $2, ticked_at = now(), ticked_via = $3, ticked_quote = $4, note = $5
			WHERE id = $1 RETURNING `+jobCols, jobID, actor, via, quoteVal, newNote).Scan(
			&j.ID, &j.ChecklistID, &j.Title, &j.Note, &j.Position,
			&j.TickedBy, &j.TickedAt, &j.TickedVia, &j.TickedQuote, &j.CreatedAt)
	} else {
		err = tx.QueryRow(ctx, `
			UPDATE jobs SET ticked_by = NULL, ticked_at = NULL, ticked_via = NULL, ticked_quote = NULL, note = $2
			WHERE id = $1 RETURNING `+jobCols, jobID, newNote).Scan(
			&j.ID, &j.ChecklistID, &j.Title, &j.Note, &j.Position,
			&j.TickedBy, &j.TickedAt, &j.TickedVia, &j.TickedQuote, &j.CreatedAt)
	}
	if err != nil {
		return nil, err
	}
	kind := "job.ticked"
	if !tick {
		kind = "job.unticked"
	}
	extra := map[string]any{"via": via}
	if quote != "" {
		extra["quote"] = quote
	}
	if note != "" {
		extra["note"] = note
	}
	if err := jobAudit(ctx, tx, actor, kind, j, extra); err != nil {
		return nil, err
	}
	return j, nil
}

// checklistsDone returns the subset of the given checklists that are done.
func checklistsDone(ctx context.Context, q Querier, ids []uuid.UUID) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	var n int
	err := q.QueryRow(ctx, `
		SELECT count(*) FROM checklists c
		WHERE c.id = ANY($1)
		  AND EXISTS (SELECT 1 FROM jobs j WHERE j.checklist_id = c.id)
		  AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.checklist_id = c.id AND j.ticked_at IS NULL)`,
		ids).Scan(&n)
	return n, err
}

// WouldFinishChecklist reports whether removing this job would leave its
// checklist done without anyone ticking anything: the job is unticked, and
// every other job on the checklist is ticked. The chat agent's remove_job
// refuses that (SPEC-014 SD-7, REVIEW-014 R14-3).
func WouldFinishChecklist(ctx context.Context, q Querier, j *Job) (bool, error) {
	if j.Ticked() {
		return false, nil
	}
	var others, unticked int
	err := q.QueryRow(ctx, `
		SELECT count(*), count(*) FILTER (WHERE ticked_at IS NULL)
		FROM jobs WHERE checklist_id = $1 AND id <> $2`, j.ChecklistID, j.ID).Scan(&others, &unticked)
	return others > 0 && unticked == 0, err
}
