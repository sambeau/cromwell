package server

import (
	"strings"
	"testing"

	"subutai/internal/store"
)

func TestFileFromTextKeepsTheFilesConventions(t *testing.T) {
	cases := []struct{ name, text, like, want string }{
		{"lf file", "a\r\nb\r\n", "x\ny\n", "a\nb\n"},
		{"crlf file", "a\r\nb\r\n", "x\r\ny\r\n", "a\r\nb\r\n"},
		{"crlf file, lf text", "a\nb\n", "x\r\ny\r\n", "a\r\nb\r\n"},
		{"bom kept", "a\r\n", bom + "x\n", bom + "a\n"},
		{"no final newline", "a\r\nb", "x\n", "a\nb"},
	}
	for _, c := range cases {
		if got := string(fileFromText(c.text, []byte(c.like))); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
	if editText([]byte(bom+"a")) != "a" {
		t.Error("the editor shows a file without its byte-order mark")
	}
}

func TestCommitAuthor(t *testing.T) {
	for in, want := range map[string]string{
		"operator":              "operator <operator@localhost>",
		"Sam Phillips":          "Sam Phillips <Sam-Phillips@localhost>",
		"Sam <sam@example.com>": "Sam <sam@example.com>",
		"":                      "operator <operator@localhost>",
		"<>":                    "operator <operator@localhost>",
	} {
		if got := commitAuthor(in); got != want {
			t.Errorf("%q: got %q want %q", in, got, want)
		}
	}
}

func TestCheckIdentityKept(t *testing.T) {
	doc := &store.Document{PublicID: "FEAT-003-spec", Revision: 2}
	good := "---\nid: FEAT-003-spec\nrevision: 2\ntitle: T\n---\n# T\n"
	if err := checkIdentityKept(doc, []byte(good), []byte(good+"more\n")); err != nil {
		t.Fatalf("kept: %v", err)
	}
	// Reordering the front matter is fine.
	moved := "---\ntitle: T\nrevision: 2\nid: FEAT-003-spec\n---\n# T\n"
	if err := checkIdentityKept(doc, []byte(good), []byte(moved)); err != nil {
		t.Fatalf("reordered: %v", err)
	}
	for _, bad := range []string{
		"---\ntitle: T\n---\n# T\n",
		"---\nid: FEAT-004-spec\nrevision: 2\n---\n",
		"---\nid: FEAT-003-spec\nrevision: 3\n---\n",
		"# no front matter\n",
		"---\nid: FEAT-003-spec\nrevision: 2\ntitle: [unclosed\n---\n",
	} {
		err := checkIdentityKept(doc, []byte(good), []byte(bad))
		if err == nil || !strings.Contains(err.Error(), `Put back the lines "id: FEAT-003-spec" and "revision: 2"`) {
			t.Errorf("%q: %v", bad, err)
		}
	}

	none := &store.Document{}
	plain := "---\ntitle: T\n---\n"
	if err := checkIdentityKept(none, []byte(plain), []byte(plain+"x\n")); err != nil {
		t.Fatalf("no ID, plain edit: %v", err)
	}
	if err := checkIdentityKept(none, []byte(plain), []byte("---\nid: X-1\ntitle: T\n---\n")); err == nil {
		t.Fatal("adding an ID was allowed")
	}
	declared := "---\nid: DEC-005\ntitle: T\n---\n"
	if err := checkIdentityKept(none, []byte(declared), []byte(declared+"x\n")); err != nil {
		t.Fatalf("keeping a declared id: %v", err)
	}
	if err := checkIdentityKept(none, []byte(declared), []byte(plain)); err == nil {
		t.Fatal("removing a declared id was allowed")
	}
}
