package textdiff

import (
	"strings"
	"testing"
)

func render(d Diff) string {
	var b strings.Builder
	for i, h := range d.Hunks {
		if i > 0 {
			b.WriteString("@@\n")
		}
		for _, l := range h.Lines {
			b.WriteByte(byte(l.Kind))
			b.WriteString(l.Text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func TestSameTextsHaveNoDiff(t *testing.T) {
	if d := Compare("a\nb\n", "a\nb\n", 3); !d.Empty() {
		t.Fatalf("want empty, got %+v", d)
	}
	if d := Compare("a\r\nb\r\n", "a\nb\n", 3); !d.Empty() {
		t.Fatal("line endings alone are not a change")
	}
	if d := Compare("", "", 3); !d.Empty() {
		t.Fatal("two empty texts are the same")
	}
}

func TestOneChangedLineWithContext(t *testing.T) {
	a := "1\n2\n3\n4\n5\n6\n7\n8\n9\n"
	b := "1\n2\n3\n4\nfive\n6\n7\n8\n9\n"
	got := render(Compare(a, b, 2))
	want := " 3\n 4\n-5\n+five\n 6\n 7\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
}

func TestSeparateChangesMakeSeparateHunks(t *testing.T) {
	var a, b []string
	for i := 0; i < 30; i++ {
		a = append(a, "line")
		b = append(b, "line")
	}
	a[2], b[2] = "old top", "new top"
	a[25], b[25] = "old bottom", "new bottom"
	d := Compare(strings.Join(a, "\n"), strings.Join(b, "\n"), 3)
	if len(d.Hunks) != 2 {
		t.Fatalf("want two hunks, got %d:\n%s", len(d.Hunks), render(d))
	}
	if r, ad := d.Counts(); r != 2 || ad != 2 {
		t.Fatalf("counts %d/%d", r, ad)
	}
}

func TestNearbyChangesMerge(t *testing.T) {
	a := "a\nb\nc\nd\ne\nf\n"
	b := "A\nb\nc\nd\ne\nF\n"
	d := Compare(a, b, 3)
	if len(d.Hunks) != 1 {
		t.Fatalf("want one hunk, got %d", len(d.Hunks))
	}
}

func TestAddedAndRemovedLinesAreNumbered(t *testing.T) {
	d := Compare("a\nb\nc\n", "a\nc\nd\n", 0)
	var got []string
	for _, h := range d.Hunks {
		for _, l := range h.Lines {
			got = append(got, string(l.Kind)+l.Text)
			if l.Kind == Removed && (l.OldNo == 0 || l.NewNo != 0) {
				t.Fatalf("removed line numbers: %+v", l)
			}
			if l.Kind == Added && (l.NewNo == 0 || l.OldNo != 0) {
				t.Fatalf("added line numbers: %+v", l)
			}
		}
	}
	if strings.Join(got, ",") != "-b,+d" {
		t.Fatalf("got %v", got)
	}
}

func TestFromAndToNothing(t *testing.T) {
	if r, a := Compare("", "x\ny\n", 3).Counts(); r != 0 || a != 2 {
		t.Fatalf("from nothing: %d/%d", r, a)
	}
	if r, a := Compare("x\ny\n", "", 3).Counts(); r != 2 || a != 0 {
		t.Fatalf("to nothing: %d/%d", r, a)
	}
}

func TestTooLargeIsSaid(t *testing.T) {
	var a, b strings.Builder
	for i := 0; i < 2100; i++ {
		a.WriteString("a\n")
		b.WriteString("b\n")
	}
	if !Compare(a.String(), b.String(), 3).TooLarge {
		t.Fatal("want TooLarge")
	}
}
