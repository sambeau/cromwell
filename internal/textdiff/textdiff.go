// Package textdiff is a small line diff for showing a person what changed in
// a document (SPEC-016 SD-7). It is a longest-common-subsequence diff over
// lines, in unified form with context, and nothing more: documents are short,
// and a readable diff matters more here than a minimal one.
package textdiff

import "strings"

// Kind marks a line of a diff.
type Kind byte

const (
	Same    Kind = ' '
	Removed Kind = '-'
	Added   Kind = '+'
)

// Line is one line of a hunk. OldNo and NewNo are 1-based line numbers in
// each version, or 0 where the line doesn't exist in that version.
type Line struct {
	Kind  Kind
	Text  string
	OldNo int
	NewNo int
}

// Hunk is a run of changes with the unchanged lines around them.
type Hunk struct {
	Lines []Line
}

// Diff is the difference between two texts.
type Diff struct {
	Hunks []Hunk
	// TooLarge is set when the texts are too big to compare line by line;
	// the caller shows both whole instead.
	TooLarge bool
}

// Empty reports whether the texts are the same.
func (d Diff) Empty() bool { return !d.TooLarge && len(d.Hunks) == 0 }

// Counts returns how many lines were removed and added.
func (d Diff) Counts() (removed, added int) {
	for _, h := range d.Hunks {
		for _, l := range h.Lines {
			switch l.Kind {
			case Removed:
				removed++
			case Added:
				added++
			}
		}
	}
	return removed, added
}

// MaxPairs bounds the comparison table: past four million line pairs a
// document is shown whole rather than diffed.
const MaxPairs = 4_000_000

// Compare diffs a against b, keeping context unchanged lines around each
// change. Line endings are compared without their carriage returns.
func Compare(a, b string, context int) Diff {
	x, y := splitLines(a), splitLines(b)
	// Trim the common head and tail first: an edit usually touches a few
	// lines of a long file, and this keeps the table small.
	pre := 0
	for pre < len(x) && pre < len(y) && x[pre] == y[pre] {
		pre++
	}
	suf := 0
	for suf < len(x)-pre && suf < len(y)-pre && x[len(x)-1-suf] == y[len(y)-1-suf] {
		suf++
	}
	mx, my := x[pre:len(x)-suf], y[pre:len(y)-suf]
	if len(mx) == 0 && len(my) == 0 {
		return Diff{}
	}
	if len(mx)*len(my) > MaxPairs {
		return Diff{TooLarge: true}
	}

	var ops []Line
	for i := 0; i < pre; i++ {
		ops = append(ops, Line{Kind: Same, Text: x[i], OldNo: i + 1, NewNo: i + 1})
	}
	ops = append(ops, lcs(mx, my, pre)...)
	for i := 0; i < suf; i++ {
		oi, ni := len(x)-suf+i, len(y)-suf+i
		ops = append(ops, Line{Kind: Same, Text: x[oi], OldNo: oi + 1, NewNo: ni + 1})
	}
	return Diff{Hunks: hunks(ops, context)}
}

// lcs aligns the middle sections, numbering lines from offset.
func lcs(x, y []string, offset int) []Line {
	n, m := len(x), len(y)
	// table[i][j] is the LCS length of x[i:] and y[j:].
	table := make([][]int32, n+1)
	for i := range table {
		table[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if x[i] == y[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else if table[i+1][j] >= table[i][j+1] {
				table[i][j] = table[i+1][j]
			} else {
				table[i][j] = table[i][j+1]
			}
		}
	}
	var out []Line
	i, j := 0, 0
	for i < n || j < m {
		switch {
		case i < n && j < m && x[i] == y[j]:
			out = append(out, Line{Kind: Same, Text: x[i], OldNo: offset + i + 1, NewNo: offset + j + 1})
			i++
			j++
		case j < m && (i == n || table[i][j+1] > table[i+1][j]):
			out = append(out, Line{Kind: Added, Text: y[j], NewNo: offset + j + 1})
			j++
		default:
			out = append(out, Line{Kind: Removed, Text: x[i], OldNo: offset + i + 1})
			i++
		}
	}
	return out
}

// hunks groups changed lines with up to context unchanged lines either side,
// merging groups whose context would touch.
func hunks(ops []Line, context int) []Hunk {
	var out []Hunk
	var cur []Line
	lastChange := -1
	for i, op := range ops {
		if op.Kind == Same {
			continue
		}
		start := i - context
		if start < 0 {
			start = 0
		}
		if cur != nil && start <= lastChange+context+1 {
			// Close enough to the previous change: extend the hunk.
			cur = append(cur, ops[lastChange+1:i+1]...)
		} else {
			if cur != nil {
				out = append(out, Hunk{Lines: append(cur, trailing(ops, lastChange, context)...)})
			}
			cur = append([]Line(nil), ops[start:i+1]...)
		}
		lastChange = i
	}
	if cur != nil {
		out = append(out, Hunk{Lines: append(cur, trailing(ops, lastChange, context)...)})
	}
	return out
}

func trailing(ops []Line, last, context int) []Line {
	end := last + 1 + context
	if end > len(ops) {
		end = len(ops)
	}
	return ops[last+1 : end]
}

// splitLines splits text into lines without their endings. A final line
// break doesn't make an empty last line.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}
