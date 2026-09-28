package store

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FR-1.7 and FR-2.2: text is made storable, and a cut keeps the head and the
// tail on character boundaries with a marker saying how much went.
func TestCutAndClean(t *testing.T) {
	dirty := "a\x00b\xffc"
	clean := CleanText(dirty)
	if strings.IndexByte(clean, 0) >= 0 || !utf8.ValidString(clean) {
		t.Errorf("CleanText(%q) = %q", dirty, clean)
	}

	if s, cut := Cut("short", 100); cut || s != "short" {
		t.Errorf("a short string must not be cut: %q %v", s, cut)
	}
	if s, cut := Cut("anything", 0); cut || s != "anything" {
		t.Error("limit 0 leaves the string whole")
	}

	long := strings.Repeat("é", 1000) // two bytes a rune, so byte cuts can split one
	s, cut := Cut(long, 101)
	if !cut || !utf8.ValidString(s) {
		t.Fatalf("cut = %v, valid = %v", cut, utf8.ValidString(s))
	}
	if !strings.HasPrefix(s, "éé") || !strings.HasSuffix(s, "éé") {
		t.Error("the cut should keep the start and the end")
	}
	if !strings.Contains(s, "bytes cut from the middle") {
		t.Errorf("no marker in %q", s)
	}
	if !strings.Contains(CutMarker(123456), "123,456") {
		t.Error("the marker groups thousands")
	}
}
