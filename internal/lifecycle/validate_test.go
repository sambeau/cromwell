package lifecycle

import (
	"strings"
	"testing"

	"cromwell/internal/config"
)

func specManifest() *config.Manifest {
	return &config.Manifest{
		Type:         "spec",
		ReviewerRole: "spec-reviewer",
		FrontMatter:  config.FrontMatterReqs{Required: []string{"title", "type", "owner"}},
		Sections: config.SectionReqs{
			Order: "strict",
			Required: []config.SectionRef{
				{Heading: "Overview"},
				{Heading: "Acceptance criteria"},
			},
		},
		Rules: []config.Rule{
			{Kind: "min_list_items", Section: "Acceptance criteria", Min: 1},
		},
	}
}

const goodSpec = `---
title: Login form
type: spec
owner: FEAT-x1
---

# Login form

## Overview

Users sign in with email and password.

## Acceptance criteria

- Valid credentials produce a session
- Invalid credentials show an error
`

func TestValidateGoodSpec(t *testing.T) {
	r := Validate(specManifest(), goodSpec, nil)
	if !r.Valid {
		t.Fatalf("good spec should validate; issues: %+v", r.Issues)
	}
}

// FR-5.2's acceptance criterion: a spec missing its acceptance-criteria
// section is rejected with the section named.
func TestMissingSectionNamed(t *testing.T) {
	bad := strings.Replace(goodSpec, "## Acceptance criteria", "## Something else", 1)
	r := Validate(specManifest(), bad, nil)
	if r.Valid {
		t.Fatal("should fail")
	}
	found := false
	for _, is := range r.Issues {
		if strings.Contains(is.Detail, "Acceptance criteria") {
			found = true
		}
	}
	if !found {
		t.Errorf("issues should name the missing section: %+v", r.Issues)
	}
}

func TestFrontMatterChecks(t *testing.T) {
	noFM := "# Just a heading\n"
	if r := Validate(specManifest(), noFM, nil); r.Valid {
		t.Error("missing front matter should fail")
	}
	wrongType := strings.Replace(goodSpec, "type: spec", "type: note", 1)
	if r := Validate(specManifest(), wrongType, nil); r.Valid {
		t.Error("type mismatch should fail")
	}
	missingOwner := strings.Replace(goodSpec, "owner: FEAT-x1\n", "", 1)
	if r := Validate(specManifest(), missingOwner, nil); r.Valid {
		t.Error("missing required front-matter field should fail")
	}
}

func TestStrictOrder(t *testing.T) {
	reordered := `---
title: X
type: spec
owner: F
---

## Acceptance criteria

- one

## Overview

text
`
	r := Validate(specManifest(), reordered, nil)
	if r.Valid {
		t.Fatal("out-of-order sections should fail under strict order")
	}
}

func TestPlaceholders(t *testing.T) {
	withPlaceholder := strings.Replace(goodSpec, "email and password", "{{fill this in}}", 1)
	if r := Validate(specManifest(), withPlaceholder, nil); r.Valid {
		t.Error("unresolved {{...}} should fail")
	}
	withTodo := strings.Replace(goodSpec, "email and password", "TODO decide", 1)
	if r := Validate(specManifest(), withTodo, nil); r.Valid {
		t.Error("TODO should fail")
	}
	// "TODOS" as part of a longer word must not trip the check.
	withWord := strings.Replace(goodSpec, "email and password", "mastodont TODOSaurus", 1)
	if r := Validate(specManifest(), withWord, nil); !r.Valid {
		t.Errorf("word-boundary check failed: %+v", r.Issues)
	}
}

func TestMinListItems(t *testing.T) {
	empty := strings.Replace(goodSpec,
		"- Valid credentials produce a session\n- Invalid credentials show an error\n",
		"prose without any list\n", 1)
	r := Validate(specManifest(), empty, nil)
	if r.Valid {
		t.Fatal("empty criteria list should fail min_list_items")
	}
}

func TestLinkChecking(t *testing.T) {
	withLink := strings.Replace(goodSpec, "email and password", "see [design](../design/x.md)", 1)
	resolves := func(target string) bool { return false }
	r := Validate(specManifest(), withLink, resolves)
	if r.Valid {
		t.Fatal("unresolvable link should fail")
	}
	if !strings.Contains(r.Issues[len(r.Issues)-1].Detail, "../design/x.md") {
		t.Errorf("issue should name the target: %+v", r.Issues)
	}
	// External and anchor links are not checked.
	external := strings.Replace(goodSpec, "email and password",
		"see [rfc](https://example.com/rfc) and [above](#overview)", 1)
	if r := Validate(specManifest(), external, resolves); !r.Valid {
		t.Errorf("external/anchor links should not be checked: %+v", r.Issues)
	}
}

func TestFrontMatterPlaceholders(t *testing.T) {
	templated := strings.Replace(goodSpec, "title: Login form", `title: "{{feature name}}"`, 1)
	r := Validate(specManifest(), templated, nil)
	if r.Valid {
		t.Fatal("front-matter placeholder should fail validation")
	}
}
