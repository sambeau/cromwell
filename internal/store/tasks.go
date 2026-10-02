package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"subutai/internal/lifecycle"
)

type Task struct {
	ID          uuid.UUID
	FeatureID   uuid.UUID
	Position    int
	LocalID     string
	Title       string
	Description string
	State       lifecycle.TaskState
	DependsOn   []uuid.UUID
	BaseCommit  string // branch HEAD when the task's work began (code-review diff base)
	CreatedAt   time.Time
	// PublicID is the task's ID, its feature's ID and its number from the
	// feature's own counter: "FEAT-023-T03" (SPEC-015 SD-4).
	PublicID string
}

const taskCols = `id, feature_id, position, COALESCE(local_id, ''), title, description, state, depends_on, COALESCE(base_commit, ''), created_at, public_id`

// taskColsT is the same column list qualified to the `t` alias, for queries
// that join another table (avoids ambiguous `id`).
const taskColsT = `t.id, t.feature_id, t.position, COALESCE(t.local_id, ''), t.title, t.description, t.state, t.depends_on, COALESCE(t.base_commit, ''), t.created_at, t.public_id`

func scanTask(row pgx.Row) (*Task, error) {
	var t Task
	err := row.Scan(&t.ID, &t.FeatureID, &t.Position, &t.LocalID, &t.Title,
		&t.Description, &t.State, &t.DependsOn, &t.BaseCommit, &t.CreatedAt, &t.PublicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &t, err
}

// SetTaskBaseCommit records the branch HEAD at which a task's work begins,
// once (the first implement dispatch); rework keeps the same base so review
// sees the task's whole contribution (DESIGN-006 §5).
func SetTaskBaseCommit(ctx context.Context, tx pgx.Tx, taskID uuid.UUID, commit string) error {
	_, err := tx.Exec(ctx, `UPDATE tasks SET base_commit = $2 WHERE id = $1 AND base_commit IS NULL`, taskID, commit)
	return err
}

// CreateTask inserts a task (used by decomposition). depends_on is set in a
// second pass once all sibling ids are known (DESIGN-005 §4).
func CreateTask(ctx context.Context, tx pgx.Tx, featureID uuid.UUID, position int, localID, title, description string, actor string) (*Task, error) {
	t := &Task{ID: NewID(), FeatureID: featureID, Position: position, LocalID: localID,
		Title: title, Description: description, State: lifecycle.TaskPending, DependsOn: []uuid.UUID{}}
	// The task's ID comes from its feature's own counter, bumped in the same
	// statement, so two tasks created at once can't share a number (SD-4).
	err := tx.QueryRow(ctx, `
		WITH n AS (
			UPDATE features SET task_seq = task_seq + 1 WHERE id = $2
			RETURNING public_id, task_seq
		)
		INSERT INTO tasks (id, feature_id, position, local_id, title, description, public_id)
		SELECT $1, $2, $3, $4, $5, $6, n.public_id || '-T' || ident_number(n.task_seq, 2) FROM n
		RETURNING public_id`,
		t.ID, featureID, position, nullable(localID), title, description).Scan(&t.PublicID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := Audit(ctx, tx, actor, "task.created", "task", &t.ID,
		map[string]any{"feature_id": featureID.String(), "local_id": localID, "title": title}); err != nil {
		return nil, err
	}
	return t, nil
}

// SetTaskDependencies sets the resolved depends_on UUIDs for a task.
func SetTaskDependencies(ctx context.Context, tx pgx.Tx, taskID uuid.UUID, deps []uuid.UUID) error {
	if deps == nil {
		deps = []uuid.UUID{}
	}
	_, err := tx.Exec(ctx, `UPDATE tasks SET depends_on = $2 WHERE id = $1`, taskID, deps)
	return err
}

func GetTask(ctx context.Context, q Querier, id uuid.UUID) (*Task, error) {
	return scanTask(q.QueryRow(ctx, `SELECT `+taskCols+` FROM tasks WHERE id = $1`, id))
}

// TaskByLocalID resolves a dev-plan-local id (e.g. "T1") within a feature,
// ignoring abandoned tasks (the live id, matching the unique index).
func TaskByLocalID(ctx context.Context, q Querier, featureID uuid.UUID, localID string) (*Task, error) {
	return scanTask(q.QueryRow(ctx, `SELECT `+taskCols+` FROM tasks
		WHERE feature_id = $1 AND local_id = $2 AND state <> 'abandoned'`, featureID, localID))
}

// TasksForFeature returns the feature's tasks in position order.
func TasksForFeature(ctx context.Context, q Querier, featureID uuid.UUID) ([]Task, error) {
	rows, err := q.Query(ctx, `SELECT `+taskCols+` FROM tasks WHERE feature_id = $1 ORDER BY position`, featureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// ErrStaleState refuses an update whose row has moved on since the caller
// read it: a lost race refuses rather than overwrites (SPEC-020 FR-2.7).
var ErrStaleState = errors.New("the state changed while this was being done; look again and retry")

// TransitionTask applies a task lifecycle event, audit row in the same
// transaction. The lifecycle engine is the sole authority on legality, and
// the update is guarded on the state the caller read (SPEC-020 FR-2.7): if the
// task has moved, nothing changes and ErrStaleState is returned.
func TransitionTask(ctx context.Context, tx pgx.Tx, t *Task, event lifecycle.TaskEvent, actor string, payload map[string]any) error {
	next, err := lifecycle.TaskTransition(t.State, event)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE tasks SET state = $2 WHERE id = $1 AND state = $3`, t.ID, next, t.State)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrStaleState
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["from"] = string(t.State)
	payload["to"] = string(next)
	payload["event"] = string(event)
	if err := Audit(ctx, tx, actor, "task.transition", "task", &t.ID, payload); err != nil {
		return err
	}
	t.State = next
	return nil
}

// DeleteTask removes a not-yet-started task during re-decomposition
// (DESIGN-005 §4); callers guarantee the task is pending/ready.
func DeleteTask(ctx context.Context, tx pgx.Tx, id uuid.UUID, actor string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, id); err != nil {
		return err
	}
	return Audit(ctx, tx, actor, "task.deleted", "task", &id, map[string]any{})
}

// UpdateTaskFields refreshes title/description/position during
// re-decomposition of a still-pending task.
func UpdateTaskFields(ctx context.Context, tx pgx.Tx, id uuid.UUID, position int, title, description string) error {
	_, err := tx.Exec(ctx, `UPDATE tasks SET position = $2, title = $3, description = $4 WHERE id = $1`,
		id, position, title, description)
	return err
}

// ReadyDependents returns pending tasks in the feature that list justDone in
// their depends_on and whose every dependency is now done — the tasks that
// become ready when justDone completes (DESIGN-005 §5).
func ReadyDependents(ctx context.Context, q Querier, featureID, justDone uuid.UUID) ([]Task, error) {
	rows, err := q.Query(ctx, `
		SELECT `+taskCols+` FROM tasks t
		WHERE t.feature_id = $1 AND t.state = 'pending' AND $2 = ANY(t.depends_on)
		  AND NOT EXISTS (
			SELECT 1 FROM unnest(t.depends_on) dep
			JOIN tasks d ON d.id = dep
			WHERE d.state <> 'done'
		  )`, featureID, justDone)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// DispatchableTasks returns ready tasks of an active, non-stale feature
// eligible for implementer dispatch (DESIGN-005 §5).
func (s *Store) DispatchableTasks(ctx context.Context, featureID uuid.UUID) ([]Task, error) {
	rows, err := s.Pool.Query(ctx, `
		SELECT `+taskColsT+` FROM tasks t
		JOIN features f ON f.id = t.feature_id
		WHERE t.feature_id = $1 AND t.state = 'ready'
		  AND f.state = 'active' AND NOT f.spec_stale
		ORDER BY t.position`, featureID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// TaskCounts returns (total, done, terminal) for a feature — the inputs to
// gate G2.
func TaskCounts(ctx context.Context, q Querier, featureID uuid.UUID) (total, done, terminal int, err error) {
	err = q.QueryRow(ctx, `
		SELECT count(*),
		       count(*) FILTER (WHERE state = 'done'),
		       count(*) FILTER (WHERE state IN ('done','abandoned'))
		FROM tasks WHERE feature_id = $1`, featureID).Scan(&total, &done, &terminal)
	return
}

// ExistingTasksForPlan returns the feature's tasks as decomposition inputs
// (local_id + state), for reconciliation.
func ExistingTasksForPlan(ctx context.Context, q Querier, featureID uuid.UUID) ([]lifecycle.ExistingTask, error) {
	tasks, err := TasksForFeature(ctx, q, featureID)
	if err != nil {
		return nil, err
	}
	var out []lifecycle.ExistingTask
	for _, t := range tasks {
		if t.LocalID == "" {
			continue
		}
		out = append(out, lifecycle.ExistingTask{LocalID: t.LocalID, State: t.State})
	}
	return out, nil
}
