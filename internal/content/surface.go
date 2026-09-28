package content

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// The surfaced block (SPEC-018 FR-6): the project's conventions and the
// accepted decisions on a dispatch's branch of the tree, pushed into its
// prompt under a hard cap. Rendering is a pure function of its input, so the
// same accepted rows always give the same bytes and the provider's prompt
// cache keeps working (NFR-1).

// SurfaceDecision is one accepted decision as the block sees it.
type SurfaceDecision struct {
	ID         string // DEC-008
	Number     int    // 8, for ordering
	Title      string
	Ruling     string
	Reason     string
	FromTitle  bool // no ruling recorded: the title is surfaced (SD-1)
	ApprovedAt time.Time
}

// SurfaceOwner is the project or one initiative on the branch, with its
// decisions. Label is "" for the project and "INIT-004 Auth" for an
// initiative.
type SurfaceOwner struct {
	Label     string
	Decisions []SurfaceDecision
}

// SurfaceInput is everything the block is made from. Owners run from the
// project (first) to the dispatch's own initiative (last).
type SurfaceInput struct {
	Conventions  string
	Owners       []SurfaceOwner
	MaxTokens    int
	MaxDecisions int // 0: no count cap
}

// SurfaceResult is the rendered block, what it holds and what the cap left
// out.
type SurfaceResult struct {
	Block    string
	Included []string // decision IDs, in rendering order
	LeftOut  []string // decision IDs the cap left out, in admission order
	Tokens   int      // the block's estimated size
}

// EstimateTokens is a text's size in tokens, estimated as its characters
// (runes) divided by 3.5, rounded up (SD-8). Subutai has no tokeniser. The
// figure is nominal: current tokenisers give three and a half to four
// characters of English per token, and fewer for identifiers and paths.
func EstimateTokens(s string) int {
	return (utf8.RuneCountInString(s)*2 + 6) / 7
}

// maxLeftOutNamed bounds the left-out line, so a long tail of decisions can't
// crowd out the ones admitted.
const maxLeftOutNamed = 10

// SurfacedBlock renders the block (FR-6.3, FR-6.4, SD-9, SD-10).
//
// Admission is nearest owner first and, within an owner, newest accepted
// first. The first decision that doesn't fit ends admission for its owner and
// every owner further out, so what is left out is always the far end of the
// tree. Rendering is widest-shared first: the conventions, then the project's
// decisions, then each initiative's from the outermost in, each by number, so
// dispatches on sibling branches share the longest possible prefix.
func SurfacedBlock(in SurfaceInput) SurfaceResult {
	conventions := demoteHeadings(strings.TrimSpace(in.Conventions))
	total := 0
	for _, o := range in.Owners {
		total += len(o.Decisions)
	}
	if conventions == "" && total == 0 {
		return SurfaceResult{}
	}

	// Admission order: nearest owner first, newest first within it.
	type cand struct {
		owner int
		d     SurfaceDecision
	}
	var order []cand
	for oi := len(in.Owners) - 1; oi >= 0; oi-- {
		ds := append([]SurfaceDecision(nil), in.Owners[oi].Decisions...)
		sort.SliceStable(ds, func(i, j int) bool {
			if !ds[i].ApprovedAt.Equal(ds[j].ApprovedAt) {
				return ds[i].ApprovedAt.After(ds[j].ApprovedAt)
			}
			return ds[i].Number > ds[j].Number
		})
		for _, d := range ds {
			order = append(order, cand{owner: oi, d: d})
		}
	}

	admitted := map[string]bool{}
	render := func() (string, []string, []string) {
		var inc, out []string
		for _, c := range order {
			if !admitted[c.d.ID] {
				out = append(out, c.d.ID)
			}
		}
		var b strings.Builder
		b.WriteString("# Project decisions and conventions\n\n")
		b.WriteString("These are binding: people decided them. If the task, or a document you are given, contradicts one, follow the decision and say so in your outcome; don't work around it.\n")
		if conventions != "" {
			b.WriteString("\n## Conventions\n\n" + conventions + "\n")
		}
		wroteHeading := false
		for _, o := range in.Owners {
			ds := append([]SurfaceDecision(nil), o.Decisions...)
			sort.SliceStable(ds, func(i, j int) bool { return ds[i].Number < ds[j].Number })
			for _, d := range ds {
				if !admitted[d.ID] {
					continue
				}
				if !wroteHeading {
					b.WriteString("\n## Decisions\n\n")
					wroteHeading = true
				}
				b.WriteString(decisionLine(o.Label, d) + "\n")
				inc = append(inc, d.ID)
			}
		}
		if len(out) > 0 {
			titles := map[string]string{}
			for _, c := range order {
				titles[c.d.ID] = c.d.Title
			}
			var names []string
			for i, id := range out {
				if i == maxLeftOutNamed {
					names = append(names, fmt.Sprintf("and %d more", len(out)-maxLeftOutNamed))
					break
				}
				names = append(names, fmt.Sprintf("%s (%s)", id, stripIDPrefix(id, titles[id])))
			}
			b.WriteString("\nLeft out for space: " + strings.Join(names, ", ") + ".\n")
		}
		return b.String(), inc, out
	}

	for _, c := range order {
		if in.MaxDecisions > 0 && len(admitted) >= in.MaxDecisions {
			break
		}
		admitted[c.d.ID] = true
		block, _, _ := render()
		if in.MaxTokens > 0 && EstimateTokens(block) > in.MaxTokens {
			delete(admitted, c.d.ID)
			break
		}
	}
	block, inc, out := render()
	return SurfaceResult{Block: block, Included: inc, LeftOut: out, Tokens: EstimateTokens(block)}
}

// decisionLine is one decision as agents are told it (FR-6.2).
func decisionLine(ownerLabel string, d SurfaceDecision) string {
	head := d.ID
	if ownerLabel != "" {
		head += " (" + ownerLabel + ")"
	}
	if d.FromTitle || strings.TrimSpace(d.Ruling) == "" {
		return "- " + head + ": " + stripIDPrefix(d.ID, d.Title)
	}
	line := "- " + head + ": " + oneLine(d.Ruling)
	if r := oneLine(d.Reason); r != "" {
		line += " Why: " + r
	}
	return line
}

// SurfacedLine is how one decision reads in a prompt, for the pages that
// show it (FR-4.2).
func SurfacedLine(ownerLabel string, d SurfaceDecision) string {
	return strings.TrimPrefix(decisionLine(ownerLabel, d), "- ")
}

// demoteHeadings moves the conventions' own headings one level down, so they
// sit under the block's "## Conventions" (R18-17).
func demoteHeadings(s string) string {
	lines := strings.Split(s, "\n")
	inFence := false
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimSpace(l), "```") {
			inFence = !inFence
			continue
		}
		if !inFence && strings.HasPrefix(l, "#") {
			trimmed := strings.TrimLeft(l, "#")
			if n := len(l) - len(trimmed); n < 6 && strings.HasPrefix(trimmed, " ") {
				lines[i] = "#" + l
			}
		}
	}
	return strings.Join(lines, "\n")
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// stripIDPrefix drops a leading "DEC-001:" from a title that already carries
// its ID, as this repository's decisions do, so it isn't said twice.
func stripIDPrefix(id, title string) string {
	t := strings.TrimSpace(title)
	if rest, ok := strings.CutPrefix(t, id); ok {
		rest = strings.TrimLeft(rest, " :—–-")
		if rest != "" {
			return rest
		}
	}
	return t
}
