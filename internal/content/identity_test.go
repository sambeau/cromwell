package content

import "testing"

func TestSetIdentityChangesOnlyTheTwoLines(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{
			name: "no front matter",
			in:   "# DEC-005: The boundary\n\nText.\n",
			want: "---\nid: DEC-005\nrevision: 1\n---\n\n# DEC-005: The boundary\n\nText.\n",
		},
		{
			name: "front matter without identity keeps everything else",
			in:   "---\n# a comment\ntitle: \"Login\"   # trailing\ntype: design\n---\n\n## Body\n---\nnot a fence\n",
			want: "---\nid: FEAT-001-design\nrevision: 1\n# a comment\ntitle: \"Login\"   # trailing\ntype: design\n---\n\n## Body\n---\nnot a fence\n",
		},
		{
			name: "existing lines replaced in place",
			in:   "---\ntitle: X\nid: OLD\nnested:\n  id: keep-me\nrevision: 7\n---\nbody\n",
			want: "---\ntitle: X\nid: FEAT-001-design\nnested:\n  id: keep-me\nrevision: 1\n---\nbody\n",
		},
		{
			name: "CRLF preserved",
			in:   "---\r\ntitle: X\r\n---\r\nbody\r\n",
			want: "---\r\nid: FEAT-001-design\r\nrevision: 1\r\ntitle: X\r\n---\r\nbody\r\n",
		},
		{
			name: "closing fence at end of file",
			in:   "---\ntitle: X\n---",
			want: "---\nid: FEAT-001-design\nrevision: 1\ntitle: X\n---",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id := "FEAT-001-design"
			if c.name == "no front matter" {
				id = "DEC-005"
			}
			got, err := SetIdentity(c.in, id, 1)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got\n%q\nwant\n%q", got, c.want)
			}
			gotID, rev, ok := ReadIdentity(got)
			if !ok || gotID != id || rev != 1 {
				t.Fatalf("read back %q %d %v", gotID, rev, ok)
			}
		})
	}
}

func TestStripIdentityRestoresTheFile(t *testing.T) {
	for _, in := range []string{
		"# DEC-005: The boundary\n\nText.\n",
		"---\ntitle: X\n---\nbody\n",
		"---\r\ntitle: X\r\n---\r\nbody\r\n",
	} {
		with, err := SetIdentity(in, "DEC-005", 3)
		if err != nil {
			t.Fatal(err)
		}
		back, err := StripIdentity(with)
		if err != nil {
			t.Fatal(err)
		}
		if back != in {
			t.Errorf("strip didn't restore:\n%q\nwant\n%q", back, in)
		}
	}
}

func TestUnclosedFrontMatterIsRefused(t *testing.T) {
	if _, err := SetIdentity("---\ntitle: X\nbody with no fence\n", "X", 1); err == nil {
		t.Fatal("an unclosed front matter should be refused, not guessed at")
	}
}

func TestReadIdentityNeedsBoth(t *testing.T) {
	if _, _, ok := ReadIdentity("---\nid: FEAT-001-spec\n---\n"); ok {
		t.Error("an id without a revision is not an identity")
	}
	if _, _, ok := ReadIdentity("no front matter"); ok {
		t.Error("no front matter, no identity")
	}
	if id := ReadDeclaredID("---\nid: FEAT-001-spec\n---\n"); id != "FEAT-001-spec" {
		t.Errorf("declared id = %q", id)
	}
}
