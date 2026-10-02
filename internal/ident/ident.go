// Package ident is the registry of Subutai's IDs (SPEC-015, DESIGN-010 §7):
// the prefixes, their format, and a parser for every shape an ID takes.
//
// IDs are minted by the database — each prefix has a sequence, and each
// entity table a column default that draws from it (migration 0010) — so
// numbers never clash whoever creates what. This package never mints; it
// knows the prefixes, formats numbers the way the database does, and reads an
// ID back into its parts.
package ident

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// Kind is one prefix in the registry.
type Kind struct {
	Name   string // "initiative", "feature", …
	Prefix string // "INIT", "FEAT", …
	// Table is the entity table whose rows carry this prefix — for most, the
	// one whose public_id column defaults to it (a bug names its own) — or
	// "" when it isn't an entity (decisions are documents).
	Table string
}

// Kinds is the registry. Migration 0010 creates a sequence for every entry,
// ident_<prefix>_seq; a test checks the two agree. Adding an entity is one
// line here plus a column default in its migration.
var Kinds = []Kind{
	{Name: "initiative", Prefix: "INIT", Table: "initiatives"},
	{Name: "feature", Prefix: "FEAT", Table: "features"},
	// A bug is a features row with kind 'bug' (SPEC-019 SD-1); its insert
	// names mint_ident('BUG') rather than taking the column default.
	{Name: "bug", Prefix: "BUG", Table: "features"},
	{Name: "spike", Prefix: "SPK", Table: "spikes"},
	{Name: "decision", Prefix: "DEC"},
	{Name: "milestone", Prefix: "MS", Table: "milestones"},
	{Name: "roadmap", Prefix: "RM", Table: "roadmaps"},
	{Name: "checklist", Prefix: "CL", Table: "checklists"},
}

// DocTypes are the document types an ID can name, in database form
// (migration 0001's document_type, plus 0010's decision, 0012's conventions and
// 0013's bug_report and 0015's findings).
var DocTypes = []string{"spec", "dev_plan", "design", "research", "report", "note", "policy", "decision", "conventions", "bug_report", "findings"}

// IsDocType reports whether t is a document type.
func IsDocType(t string) bool {
	for _, d := range DocTypes {
		if d == t {
			return true
		}
	}
	return false
}

// ProjectOwner is the owner part of a project-level document's ID,
// "PROJECT-design": the project has no ID of its own (SPEC-015 SD-6).
const ProjectOwner = "PROJECT"

// KindByPrefix finds a registry entry.
func KindByPrefix(prefix string) (Kind, bool) {
	for _, k := range Kinds {
		if k.Prefix == prefix {
			return k, true
		}
	}
	return Kind{}, false
}

// KindByName finds a registry entry by its entity name.
func KindByName(name string) (Kind, bool) {
	for _, k := range Kinds {
		if k.Name == name {
			return k, true
		}
	}
	return Kind{}, false
}

// Number formats n zero-padded to width, never truncated, as the database's
// ident_number does.
func Number(n int64, width int) string {
	s := strconv.FormatInt(n, 10)
	if len(s) >= width {
		return s
	}
	return strings.Repeat("0", width-len(s)) + s
}

// Format is an entity ID: "INIT-014".
func Format(prefix string, n int64) string { return prefix + "-" + Number(n, 3) }

// TaskID is a task's ID: its feature's ID and the task's number, "FEAT-023-T03".
func TaskID(featureID string, n int64) string { return featureID + "-T" + Number(n, 2) }

// TypeSlug is a document type as it appears in an ID or a file name:
// dev_plan becomes dev-plan.
func TypeSlug(docType string) string { return strings.ReplaceAll(docType, "_", "-") }

// TypeFromSlug reverses TypeSlug.
func TypeFromSlug(slug string) string { return strings.ReplaceAll(slug, "-", "_") }

// DocumentID is a document's ID: its owner's ID and its type, with -n for the
// second and later live documents of the same type (SD-6). A decision has no
// owner part; use DecisionID for it.
func DocumentID(ownerID, docType string, n int) string {
	id := ownerID + "-" + TypeSlug(docType)
	if n > 1 {
		id += "-" + strconv.Itoa(n)
	}
	return id
}

// ArchiveName is the file name of a superseded revision: "FEAT-023-spec.r1.md"
// (SD-9).
func ArchiveName(id string, revision int) string {
	return fmt.Sprintf("%s.r%d.md", id, revision)
}

// Shape is what an ID names.
type Shape int

const (
	ShapeNone     Shape = iota
	ShapeEntity         // INIT-014, FEAT-023, MS-004 …
	ShapeTask           // FEAT-023-T03
	ShapeDecision       // DEC-005
	ShapeDocument       // FEAT-023-spec, INIT-014-design-2, PROJECT-note
)

// Ref is a parsed ID.
type Ref struct {
	Shape Shape
	// ID is the canonical form without any revision suffix.
	ID string
	// Kind is the entity's registry entry (ShapeEntity), or the owner's for a
	// document (zero for a project-level document).
	Kind Kind
	// Entity is the entity ID: the entity itself, the task's feature, or the
	// document's owner ("PROJECT" for the project).
	Entity string
	// Number is the entity's, task's or decision's number.
	Number int64
	// DocType is a document's type in database form (dev_plan), and DocN its
	// -n counter (1 when absent).
	DocType string
	DocN    int
	// Revision is a ".r<n>" suffix, 0 when absent.
	Revision int
}

var (
	prefixAlt = prefixAlternation()
	entityRe  = regexp.MustCompile(`^(` + prefixAlt + `)-(\d{3,})$`)
	// A bug's tasks are numbered from its ID too, "BUG-007-T01" (SPEC-019
	// FR-1.2).
	taskRe     = regexp.MustCompile(`^((?:FEAT|BUG)-\d{3,})-T(\d{2,})$`)
	decisionRe = regexp.MustCompile(`^DEC-(\d{3,})$`)
	documentRe = regexp.MustCompile(`^((?:` + prefixAlt + `)-\d{3,}|` + ProjectOwner + `)-([a-z]+(?:-[a-z]+)*?)(?:-(\d+))?$`)
	revisionRe = regexp.MustCompile(`^(.+)\.r(\d+)$`)
	// DecisionFileRe reads a decision number from the start of a file name,
	// "DEC-005-the-orchestration-boundary.md" (FR-5.4).
	decisionFileRe = regexp.MustCompile(`^(DEC-\d{3,})(?:[-_.]|$)`)
)

func prefixAlternation() string {
	var ps []string
	for _, k := range Kinds {
		ps = append(ps, k.Prefix)
	}
	return strings.Join(ps, "|")
}

// Parse reads an ID. Case is ignored in the prefix and type; the result is
// canonical. A trailing ".r<n>" names a revision of a document or decision.
func Parse(s string) (Ref, bool) {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, "/ ") {
		return Ref{}, false
	}
	rev := 0
	if m := revisionRe.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[2])
		if err != nil || n < 1 {
			return Ref{}, false
		}
		s, rev = m[1], n
	}
	up := strings.ToUpper(s)

	if rev == 0 {
		if m := entityRe.FindStringSubmatch(up); m != nil && m[1] != "DEC" {
			k, _ := KindByPrefix(m[1])
			n, _ := strconv.ParseInt(m[2], 10, 64)
			return Ref{Shape: ShapeEntity, ID: up, Kind: k, Entity: up, Number: n}, true
		}
		if m := taskRe.FindStringSubmatch(up); m != nil {
			k, _ := KindByPrefix(m[1][:strings.IndexByte(m[1], '-')])
			n, _ := strconv.ParseInt(m[2], 10, 64)
			return Ref{Shape: ShapeTask, ID: up, Kind: k, Entity: m[1], Number: n}, true
		}
	}
	if m := decisionRe.FindStringSubmatch(up); m != nil {
		n, _ := strconv.ParseInt(m[1], 10, 64)
		k, _ := KindByPrefix("DEC")
		return Ref{Shape: ShapeDecision, ID: up, Kind: k, Number: n, DocType: "decision", DocN: 1, Revision: rev}, true
	}
	// A document ID: the owner part upper-case, the type lower-case.
	if i := ownerEnd(up); i > 0 {
		norm := up[:i] + strings.ToLower(s[i:])
		if m := documentRe.FindStringSubmatch(norm); m != nil {
			r := Ref{Shape: ShapeDocument, Entity: m[1], DocType: TypeFromSlug(m[2]), DocN: 1, Revision: rev}
			// A decision is DEC-nnn on its own, never an owner, and never a
			// type under one.
			if !IsDocType(r.DocType) || r.DocType == "decision" || strings.HasPrefix(m[1], "DEC-") {
				return Ref{}, false
			}
			if m[3] != "" {
				n, err := strconv.Atoi(m[3])
				if err != nil || n < 2 {
					return Ref{}, false
				}
				r.DocN = n
			}
			if m[1] != ProjectOwner {
				em := entityRe.FindStringSubmatch(m[1])
				k, _ := KindByPrefix(em[1])
				r.Kind = k
				r.Number, _ = strconv.ParseInt(em[2], 10, 64)
			}
			r.ID = DocumentID(r.Entity, r.DocType, r.DocN)
			return r, true
		}
	}
	return Ref{}, false
}

// ownerEnd returns the length of a document ID's owner part in an upper-cased
// string, or 0.
func ownerEnd(up string) int {
	if strings.HasPrefix(up, ProjectOwner+"-") {
		return len(ProjectOwner)
	}
	i := strings.IndexByte(up, '-')
	if i < 0 {
		return 0
	}
	j := i + 1
	for j < len(up) && up[j] >= '0' && up[j] <= '9' {
		j++
	}
	if j == i+1 {
		return 0
	}
	return j
}

// DecisionFromFileName reads a well-formed decision number from the start of a
// file's base name, or "".
func DecisionFromFileName(base string) string {
	if m := decisionFileRe.FindStringSubmatch(strings.ToUpper(base)); m != nil {
		return m[1]
	}
	return ""
}
