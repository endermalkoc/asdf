package main

import (
	"strings"
	"testing"

	"github.com/endermalkoc/cusp/internal/app"
)

func TestRenderPrimeState(t *testing.T) {
	md := renderPrimeState(&app.PrimeState{
		ActiveChangeset: "changeset/revise",
		OpenChangesets: []app.PrimeChangeset{
			{Branch: "changeset/revise", Title: "Revise", Status: "draft", Active: true},
			{Branch: "changeset/other", Title: "", Status: "open"},
		},
		UnresolvedComments: 3,
		CheckFindings:      2,
		Stats:              []app.PrimeStat{{Kind: "domains", Count: 1}, {Kind: "requirements", Count: 7}},
	})

	for _, want := range []string{
		"current state",
		"`changeset/revise`",   // active changeset
		"Open changesets:** 2", // count
		"* `changeset/revise`", // active marker
		"(untitled)",           // empty title fallback
		"Unresolved review comments:** 3",
		"2 issue(s)",                 // check findings
		"domains 1 · requirements 7", // counts joined
	} {
		if !strings.Contains(md, want) {
			t.Errorf("prime state missing %q in:\n%s", want, md)
		}
	}
}

func TestRenderPrimeState_NoActiveChangeset_Clean(t *testing.T) {
	md := renderPrimeState(&app.PrimeState{})
	if !strings.Contains(md, "none — edits commit to `main`") {
		t.Errorf("expected no-active-changeset hint:\n%s", md)
	}
	if !strings.Contains(md, "clean") {
		t.Errorf("expected clean integrity for 0 findings:\n%s", md)
	}
}

func TestSlug(t *testing.T) {
	cases := map[string]string{
		"Revise the Contact entity": "revise-the-contact-entity",
		"  Trim  Spaces  ":          "trim-spaces",
		"FR-012: tighten wording!":  "fr-012-tighten-wording",
		"already-slugged":           "already-slugged",
		"UPPER":                     "upper",
	}
	for in, want := range cases {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}
