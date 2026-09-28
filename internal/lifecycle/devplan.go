package lifecycle

// Dev-plan task-table parsing and decomposition planning (DESIGN-005 §2, §4).
// Pure: the parser turns a dev-plan's `## Tasks` Markdown table into an
// ordered, validated list of task rows; the planner reconciles that list
// against a feature's existing tasks non-destructively (DP-4). No I/O.

import (
	"fmt"
	"sort"
	"strings"

	"subutai/internal/content"
)

// TaskRow is one parsed row of the dev-plan task table. LocalID is the
// plan-local identifier (e.g. "T1") that keeps re-decomposition stable
// (DP-3). DependsOn holds other rows' LocalIDs.
type TaskRow struct {
	Position  int
	LocalID   string
	Title     string
	DependsOn []string
	Body      string
}

// taskTableColumns is the fixed column set enforced by the table_parses rule
// (DESIGN-005 §2). Order in the table is free; presence is not.
var taskTableColumns = []string{"id", "title", "depends_on", "description"}

// ParseTaskTable extracts and validates the task table from a dev-plan's
// Tasks section. It returns a typed error naming the first structural defect
// (missing table, wrong columns, duplicate/dangling id, cycle) so the
// table_parses validation rule can report it (FR-1.2). A returned nil error
// guarantees the dependency graph is acyclic.
func ParseTaskTable(tasksSection string) ([]TaskRow, error) {
	header, rows, err := parseMarkdownTable(tasksSection)
	if err != nil {
		return nil, err
	}
	colIndex := map[string]int{}
	for i, h := range header {
		colIndex[strings.ToLower(strings.TrimSpace(h))] = i
	}
	for _, want := range taskTableColumns {
		if _, ok := colIndex[want]; !ok {
			return nil, fmt.Errorf("task table is missing the %q column", want)
		}
	}
	if len(header) != len(taskTableColumns) {
		return nil, fmt.Errorf("task table has %d columns, expected exactly %v", len(header), taskTableColumns)
	}

	var out []TaskRow
	seen := map[string]bool{}
	for i, cells := range rows {
		get := func(col string) string { return strings.TrimSpace(cells[colIndex[col]]) }
		id := get("id")
		if id == "" {
			return nil, fmt.Errorf("task table row %d has an empty id", i+1)
		}
		if seen[id] {
			return nil, fmt.Errorf("task table has a duplicate id %q", id)
		}
		seen[id] = true
		var deps []string
		if d := get("depends_on"); d != "" {
			for _, part := range strings.Split(d, ",") {
				p := strings.TrimSpace(part)
				if p != "" {
					deps = append(deps, p)
				}
			}
		}
		out = append(out, TaskRow{
			Position: i, LocalID: id, Title: get("title"),
			DependsOn: deps, Body: get("description"),
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("task table has no rows")
	}

	// Every dependency must reference a declared id.
	for _, r := range out {
		for _, dep := range r.DependsOn {
			if !seen[dep] {
				return nil, fmt.Errorf("task %q depends on unknown id %q", r.LocalID, dep)
			}
			if dep == r.LocalID {
				return nil, fmt.Errorf("task %q depends on itself", r.LocalID)
			}
		}
	}
	if cycle := findCycle(out); cycle != nil {
		return nil, fmt.Errorf("task dependencies form a cycle: %s", strings.Join(cycle, " → "))
	}
	return out, nil
}

// parseMarkdownTable parses the first GitHub-style pipe table in the section:
// a header row, a delimiter row (---), then body rows. Returns the header
// cells and the body rows' cells.
func parseMarkdownTable(section string) (header []string, rows [][]string, err error) {
	var tableLines []string
	for _, line := range strings.Split(section, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "|") {
			tableLines = append(tableLines, t)
		} else if len(tableLines) > 0 {
			break // table ended
		}
	}
	if len(tableLines) < 2 {
		return nil, nil, fmt.Errorf("no task table found (expected a Markdown table under the Tasks heading)")
	}
	header = splitRow(tableLines[0])
	if !isDelimiterRow(tableLines[1]) {
		return nil, nil, fmt.Errorf("task table is missing its header delimiter row (|---|---|)")
	}
	for _, line := range tableLines[2:] {
		cells := splitRow(line)
		if len(cells) != len(header) {
			return nil, nil, fmt.Errorf("task table row has %d cells, header has %d", len(cells), len(header))
		}
		rows = append(rows, cells)
	}
	return header, rows, nil
}

func splitRow(line string) []string {
	line = strings.TrimSpace(line)
	line = strings.TrimPrefix(line, "|")
	line = strings.TrimSuffix(line, "|")
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func isDelimiterRow(line string) bool {
	for _, cell := range splitRow(line) {
		c := strings.TrimSpace(cell)
		if c == "" || strings.Trim(c, "-: ") != "" {
			return false
		}
	}
	return true
}

// findCycle returns a cycle of LocalIDs if the dependency graph has one, else
// nil. Iterative DFS with a stable node order for deterministic messages.
func findCycle(rows []TaskRow) []string {
	deps := map[string][]string{}
	var order []string
	for _, r := range rows {
		deps[r.LocalID] = r.DependsOn
		order = append(order, r.LocalID)
	}
	sort.Strings(order)

	const (
		white = 0
		grey  = 1
		black = 2
	)
	color := map[string]int{}
	var stack []string
	var dfs func(node string) []string
	dfs = func(node string) []string {
		color[node] = grey
		stack = append(stack, node)
		for _, dep := range deps[node] {
			switch color[dep] {
			case grey:
				// Found a back-edge; extract the cycle from the stack.
				for i, n := range stack {
					if n == dep {
						return append(append([]string{}, stack[i:]...), dep)
					}
				}
			case white:
				if c := dfs(dep); c != nil {
					return c
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[node] = black
		return nil
	}
	for _, n := range order {
		if color[n] == white {
			if c := dfs(n); c != nil {
				return c
			}
		}
	}
	return nil
}

// ParseDevPlanTasks is the convenience entry the validation rule and
// decomposition both use: parse a whole dev-plan document and extract its
// task rows. It locates the `Tasks` section by heading.
func ParseDevPlanTasks(raw string) ([]TaskRow, error) {
	doc, err := content.Parse(raw)
	if err != nil {
		return nil, err
	}
	sec := doc.SectionByHeading("Tasks")
	if sec == nil {
		return nil, fmt.Errorf("dev-plan has no Tasks section")
	}
	return ParseTaskTable(sec.Content)
}

// ---- Decomposition planning (DESIGN-005 §4) ----

// ExistingTask is the rule engine's view of a task already owned by the
// feature, for reconciliation.
type ExistingTask struct {
	LocalID string
	State   TaskState
}

// DecompositionPlan is the non-destructive reconciliation of a new task
// table against the feature's existing tasks (DP-4). Creates and Updates
// carry rows to apply; DeleteLocalIDs are not-yet-started tasks the new
// table dropped; KeptMismatches are started/done tasks the new table dropped
// — never deleted, surfaced to the human instead.
type DecompositionPlan struct {
	Creates        []TaskRow
	Updates        []TaskRow
	DeleteLocalIDs []string
	KeptMismatches []string
}

// PlanDecomposition reconciles rows against existing. With no existing tasks
// (first decomposition) every row is a create.
func PlanDecomposition(rows []TaskRow, existing []ExistingTask) DecompositionPlan {
	byID := map[string]ExistingTask{}
	for _, e := range existing {
		byID[e.LocalID] = e
	}
	newIDs := map[string]bool{}
	var plan DecompositionPlan
	for _, r := range rows {
		newIDs[r.LocalID] = true
		if _, ok := byID[r.LocalID]; ok {
			plan.Updates = append(plan.Updates, r)
		} else {
			plan.Creates = append(plan.Creates, r)
		}
	}
	for _, e := range existing {
		if newIDs[e.LocalID] {
			continue
		}
		if e.State == TaskPending || e.State == TaskReady {
			plan.DeleteLocalIDs = append(plan.DeleteLocalIDs, e.LocalID)
		} else {
			// active / review / done: work happened; never erase it silently.
			plan.KeptMismatches = append(plan.KeptMismatches, e.LocalID)
		}
	}
	return plan
}
