package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"cromwell/internal/lifecycle"
)

// Milestones and roadmaps (DESIGN-001 §6, vision §4). A milestone references
// its members live; locking resolves the live membership once into a flat leaf
// set (milestone_snapshots) and flips the state, atomically (NFR-5). No
// fudging: the only way to lock without a member is to remove it first, and
// the removal is on the audit trail (FR-5.3).

type Milestone struct {
	ID          uuid.UUID
	Name        string
	Description string
	TargetDate  *time.Time
	State       lifecycle.MilestoneState
	LockedAt    *time.Time
	CreatedAt   time.Time
}

const milestoneCols = `id, name, description, target_date, state, locked_at, created_at`

func scanMilestone(row pgx.Row) (*Milestone, error) {
	var m Milestone
	var state string
	err := row.Scan(&m.ID, &m.Name, &m.Description, &m.TargetDate, &state, &m.LockedAt, &m.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	m.State = lifecycle.MilestoneState(state)
	return &m, err
}

func CreateMilestone(ctx context.Context, tx pgx.Tx, name, description string, targetDate *time.Time, actor string) (*Milestone, error) {
	m := &Milestone{ID: NewID(), Name: name, Description: description, TargetDate: targetDate, State: lifecycle.MilestoneOpen}
	_, err := tx.Exec(ctx, `
		INSERT INTO milestones (id, name, description, target_date)
		VALUES ($1, $2, $3, $4)`, m.ID, name, description, targetDate)
	if err != nil {
		return nil, err
	}
	if err := Audit(ctx, tx, actor, "milestone.created", "milestone", &m.ID,
		map[string]any{"name": name}); err != nil {
		return nil, err
	}
	return m, nil
}

func GetMilestone(ctx context.Context, q Querier, id uuid.UUID) (*Milestone, error) {
	return scanMilestone(q.QueryRow(ctx, `SELECT `+milestoneCols+` FROM milestones WHERE id = $1`, id))
}

// MilestoneByName resolves a milestone by its (assumed unique) name; it errors
// if the name is absent or ambiguous, so the CLI can ask for disambiguation.
func MilestoneByName(ctx context.Context, q Querier, name string) (*Milestone, error) {
	rows, err := q.Query(ctx, `SELECT `+milestoneCols+` FROM milestones WHERE name = $1`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []*Milestone
	for rows.Next() {
		m, err := scanMilestone(rows)
		if err != nil {
			return nil, err
		}
		found = append(found, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	switch len(found) {
	case 0:
		return nil, ErrNotFound
	case 1:
		return found[0], nil
	default:
		return nil, fmt.Errorf("milestone name %q is ambiguous (%d matches); use its id", name, len(found))
	}
}

// ListMilestones returns all milestones, newest first.
func ListMilestones(ctx context.Context, q Querier) ([]Milestone, error) {
	rows, err := q.Query(ctx, `SELECT `+milestoneCols+` FROM milestones ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Milestone
	for rows.Next() {
		m, err := scanMilestone(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *m)
	}
	return out, rows.Err()
}

type MilestoneMember struct {
	MemberType string
	MemberID   uuid.UUID
	AddedAt    time.Time
}

// AddMember adds a live member. Membership is idempotent on (milestone,
// member_type, member_id) — re-adding is a no-op (ON CONFLICT). Locked
// milestones are frozen: their promised set is the snapshot, not live members.
func AddMember(ctx context.Context, tx pgx.Tx, milestoneID uuid.UUID, memberType string, memberID uuid.UUID, actor string) error {
	if err := assertOpen(ctx, tx, milestoneID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO milestone_members (milestone_id, member_type, member_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (milestone_id, member_type, member_id) DO NOTHING`,
		milestoneID, memberType, memberID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil // already a member
	}
	return Audit(ctx, tx, actor, "milestone.member_added", "milestone", &milestoneID,
		map[string]any{"member_type": memberType, "member_id": memberID.String()})
}

// RemoveMember drops a live member before lock; the reason is recorded on the
// milestone's audit trail — this is the honest descope path (FR-5.3).
func RemoveMember(ctx context.Context, tx pgx.Tx, milestoneID uuid.UUID, memberType string, memberID uuid.UUID, reason, actor string) error {
	if err := assertOpen(ctx, tx, milestoneID); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM milestone_members
		WHERE milestone_id = $1 AND member_type = $2 AND member_id = $3`,
		milestoneID, memberType, memberID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return Audit(ctx, tx, actor, "milestone.member_removed", "milestone", &milestoneID,
		map[string]any{"member_type": memberType, "member_id": memberID.String(), "reason": reason})
}

func assertOpen(ctx context.Context, q Querier, milestoneID uuid.UUID) error {
	m, err := GetMilestone(ctx, q, milestoneID)
	if err != nil {
		return err
	}
	if m.State != lifecycle.MilestoneOpen {
		return fmt.Errorf("milestone is %s; membership is frozen at lock (FR-5.2)", m.State)
	}
	return nil
}

// Members returns the milestone's live members.
func Members(ctx context.Context, q Querier, milestoneID uuid.UUID) ([]MilestoneMember, error) {
	rows, err := q.Query(ctx, `
		SELECT member_type, member_id, added_at FROM milestone_members
		WHERE milestone_id = $1 ORDER BY added_at`, milestoneID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MilestoneMember
	for rows.Next() {
		var m MilestoneMember
		if err := rows.Scan(&m.MemberType, &m.MemberID, &m.AddedAt); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// ResolveMembers expands a milestone's live membership to its flat leaf set of
// feature ids (FR-5.1): a feature member resolves to itself, an initiative to
// its descendant features transitively, and a nested milestone to its own
// resolved features (recursively). The result is deduplicated. Checklists are
// not members in this slice (SD-2).
func ResolveMembers(ctx context.Context, q Querier, milestoneID uuid.UUID) ([]uuid.UUID, error) {
	seen := map[uuid.UUID]bool{}    // features
	visited := map[uuid.UUID]bool{} // milestones, to break nesting cycles
	var order []uuid.UUID
	var walk func(mID uuid.UUID) error
	walk = func(mID uuid.UUID) error {
		if visited[mID] {
			return nil
		}
		visited[mID] = true
		members, err := Members(ctx, q, mID)
		if err != nil {
			return err
		}
		for _, mem := range members {
			switch mem.MemberType {
			case "feature":
				if !seen[mem.MemberID] {
					seen[mem.MemberID] = true
					order = append(order, mem.MemberID)
				}
			case "initiative":
				feats, err := descendantFeatureIDs(ctx, q, mem.MemberID)
				if err != nil {
					return err
				}
				for _, f := range feats {
					if !seen[f] {
						seen[f] = true
						order = append(order, f)
					}
				}
			case "milestone":
				if err := walk(mem.MemberID); err != nil {
					return err
				}
			}
		}
		return nil
	}
	if err := walk(milestoneID); err != nil {
		return nil, err
	}
	return order, nil
}

// descendantFeatureIDs returns every feature in an initiative's subtree
// (transitive over nested initiatives).
func descendantFeatureIDs(ctx context.Context, q Querier, initiativeID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := q.Query(ctx, `
		WITH RECURSIVE subtree AS (
			SELECT id FROM initiatives WHERE id = $1
			UNION ALL
			SELECT i.id FROM initiatives i JOIN subtree s ON i.parent_id = s.id
		)
		SELECT f.id FROM features f JOIN subtree s ON f.initiative_id = s.id
		ORDER BY f.id`, initiativeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIDs(rows)
}

// Progress is a milestone's computed completion over its resolved (open) or
// snapshotted (locked) leaf features.
type Progress struct {
	Total  int
	Done   int
	Leaves []uuid.UUID
}

// LiveProgress computes progress over the live-resolved membership (open
// milestones, FR-5.1).
func LiveProgress(ctx context.Context, q Querier, milestoneID uuid.UUID) (Progress, error) {
	leaves, err := ResolveMembers(ctx, q, milestoneID)
	if err != nil {
		return Progress{}, err
	}
	return featureProgress(ctx, q, leaves)
}

// SnapshotProgress computes progress over the frozen leaf set of a locked
// milestone: the promised set is fixed, but each leaf's current state still
// advances the historical progress (FR-5.2).
func SnapshotProgress(ctx context.Context, q Querier, milestoneID uuid.UUID) (Progress, error) {
	rows, err := q.Query(ctx, `
		SELECT leaf_id FROM milestone_snapshots
		WHERE milestone_id = $1 AND leaf_type = 'feature' ORDER BY leaf_id`, milestoneID)
	if err != nil {
		return Progress{}, err
	}
	defer func() { rows.Close() }()
	leaves, err := scanIDs(rows)
	rows.Close()
	if err != nil {
		return Progress{}, err
	}
	return featureProgress(ctx, q, leaves)
}

func featureProgress(ctx context.Context, q Querier, leaves []uuid.UUID) (Progress, error) {
	p := Progress{Total: len(leaves), Leaves: leaves}
	if len(leaves) == 0 {
		return p, nil
	}
	err := q.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE state = 'done')
		FROM features WHERE id = ANY($1)`, leaves).Scan(&p.Done)
	return p, err
}

// LockMilestone evaluates gate G4 over the live membership and, if it passes,
// snapshots the resolved leaves and flips the state to locked — all in the
// caller's transaction (NFR-5, atomic). On G4 failure it returns an error
// carrying the gate reason and writes no snapshot. There is no force path
// (L-6): to lock without a member, remove it first (FR-5.3).
func LockMilestone(ctx context.Context, tx pgx.Tx, milestoneID uuid.UUID, actor string) (lifecycle.GateResult, error) {
	m, err := GetMilestone(ctx, tx, milestoneID)
	if err != nil {
		return lifecycle.GateResult{}, err
	}
	if m.State != lifecycle.MilestoneOpen {
		return lifecycle.GateResult{}, fmt.Errorf("milestone already %s", m.State)
	}

	p, err := LiveProgress(ctx, tx, milestoneID)
	if err != nil {
		return lifecycle.GateResult{}, err
	}
	g := lifecycle.G4(p.Total, p.Done)
	if err := Audit(ctx, tx, actor, "gate.evaluated", "milestone", &milestoneID,
		map[string]any{"gate": string(g.Gate), "pass": g.Pass, "reason": g.Reason}); err != nil {
		return g, err
	}
	if !g.Pass {
		return g, fmt.Errorf("G4: %s", g.Reason)
	}

	// Snapshot the resolved leaves, then flip the state — append-only snapshot
	// (DESIGN-001 §2.3), one transaction.
	for _, leaf := range p.Leaves {
		if _, err := tx.Exec(ctx, `
			INSERT INTO milestone_snapshots (milestone_id, leaf_type, leaf_id)
			VALUES ($1, 'feature', $2)`, milestoneID, leaf); err != nil {
			return g, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE milestones SET state = 'locked', locked_at = now() WHERE id = $1`, milestoneID); err != nil {
		return g, err
	}
	if err := Audit(ctx, tx, actor, "milestone.locked", "milestone", &milestoneID,
		map[string]any{"leaves": len(p.Leaves), "done_at_lock": p.Done}); err != nil {
		return g, err
	}
	return g, nil
}

// --- Roadmaps (FR-6) ---

type Roadmap struct {
	ID   uuid.UUID
	Name string
}

func CreateRoadmap(ctx context.Context, tx pgx.Tx, name, actor string) (*Roadmap, error) {
	r := &Roadmap{ID: NewID(), Name: name}
	if _, err := tx.Exec(ctx, `INSERT INTO roadmaps (id, name) VALUES ($1, $2)`, r.ID, name); err != nil {
		return nil, err
	}
	// Roadmaps are not a ref_type (DESIGN-001 §3); a roadmap-scoped event is
	// filed at project level with the roadmap id in the payload.
	if err := Audit(ctx, tx, actor, "roadmap.created", "project", nil,
		map[string]any{"roadmap_id": r.ID.String(), "name": name}); err != nil {
		return nil, err
	}
	return r, nil
}

func GetRoadmap(ctx context.Context, q Querier, id uuid.UUID) (*Roadmap, error) {
	var r Roadmap
	err := q.QueryRow(ctx, `SELECT id, name FROM roadmaps WHERE id = $1`, id).Scan(&r.ID, &r.Name)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &r, err
}

// RoadmapByName resolves a roadmap by its (assumed unique) name.
func RoadmapByName(ctx context.Context, q Querier, name string) (*Roadmap, error) {
	rows, err := q.Query(ctx, `SELECT id, name FROM roadmaps WHERE name = $1`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []Roadmap
	for rows.Next() {
		var r Roadmap
		if err := rows.Scan(&r.ID, &r.Name); err != nil {
			return nil, err
		}
		found = append(found, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	switch len(found) {
	case 0:
		return nil, ErrNotFound
	case 1:
		return &found[0], nil
	default:
		return nil, fmt.Errorf("roadmap name %q is ambiguous (%d matches); use its id", name, len(found))
	}
}

// SetRoadmapEntry places a milestone at a position in the roadmap; re-adding
// the same milestone updates its position (the order is the planner's to mean,
// the system only preserves it — FR-6.1).
func SetRoadmapEntry(ctx context.Context, tx pgx.Tx, roadmapID, milestoneID uuid.UUID, position int, actor string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO roadmap_entries (roadmap_id, milestone_id, position)
		VALUES ($1, $2, $3)
		ON CONFLICT (roadmap_id, milestone_id) DO UPDATE SET position = EXCLUDED.position`,
		roadmapID, milestoneID, position)
	if err != nil {
		return err
	}
	return Audit(ctx, tx, actor, "roadmap.entry_set", "milestone", &milestoneID,
		map[string]any{"roadmap_id": roadmapID.String(), "position": position})
}

type RoadmapEntry struct {
	MilestoneID uuid.UUID
	Position    int
}

// RoadmapEntries returns a roadmap's milestones in position order (ties broken
// by milestone id for a stable ordering).
func RoadmapEntries(ctx context.Context, q Querier, roadmapID uuid.UUID) ([]RoadmapEntry, error) {
	rows, err := q.Query(ctx, `
		SELECT milestone_id, position FROM roadmap_entries
		WHERE roadmap_id = $1 ORDER BY position, milestone_id`, roadmapID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RoadmapEntry
	for rows.Next() {
		var e RoadmapEntry
		if err := rows.Scan(&e.MilestoneID, &e.Position); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
