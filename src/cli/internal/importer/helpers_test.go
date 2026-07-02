package importer

import "testing"

func TestIsIssueID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"tut-123", true},
		{"TUT-9", true},  // case-insensitive
		{"Tut-1a", true}, // mixed case
		{"M1", false},    // milestone label
		{"backlog", false},
		{"", false},
		{"tutor", false}, // "tut" without the dash is not an issue id
		{"tut", false},   // needs the trailing dash
	}
	for _, c := range cases {
		if got := isIssueID(c.in); got != c.want {
			t.Errorf("isIssueID(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestFoldE2E(t *testing.T) {
	cases := []struct {
		name, notes, e2e, want string
	}{
		{"no e2e keeps notes", "some notes", "", "some notes"},
		{"blank e2e (whitespace) keeps notes", "some notes", "   ", "some notes"},
		{"empty notes yields bare tag", "", "e2e/add.spec.ts", "e2e: e2e/add.spec.ts"},
		{"whitespace notes yields bare tag", "  ", "add.spec.ts", "e2e: add.spec.ts"},
		{"notes plus e2e are bracketed", "keep me", "add.spec.ts", "keep me [e2e: add.spec.ts]"},
		{"e2e is trimmed", "keep me", "  add.spec.ts  ", "keep me [e2e: add.spec.ts]"},
	}
	for _, c := range cases {
		if got := foldE2E(c.notes, c.e2e); got != c.want {
			t.Errorf("%s: foldE2E(%q,%q) = %q, want %q", c.name, c.notes, c.e2e, got, c.want)
		}
	}
}

func TestSlugFromPath(t *testing.T) {
	cases := map[string]string{
		"enrollment/add-student.md": "add-student",
		"add-student.md":            "add-student",
		"add-student":               "add-student",
		"a/b/c/deep.md":             "deep",
		"":                          ".", // filepath.Base("") == "."
	}
	for in, want := range cases {
		if got := slugFromPath(in); got != want {
			t.Errorf("slugFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDomainRelPath(t *testing.T) {
	cases := map[string]string{
		"enrollment/add-student.md": "add-student.md",
		"enrollment/sub/add.md":     "sub/add.md",
		"top-level.md":              "top-level.md", // no slash → unchanged
	}
	for in, want := range cases {
		if got := domainRelPath(in); got != want {
			t.Errorf("domainRelPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDomainRelDir(t *testing.T) {
	cases := map[string]string{
		"enrollment/add-student.md": "",    // directory of "add-student.md" is "."
		"enrollment/sub/add.md":     "sub", // one nested dir
		"enrollment/a/b/c.md":       "a/b", // deeper nesting
		"top-level.md":              "",    // top-level doc, no domain segment
	}
	for in, want := range cases {
		if got := domainRelDir(in); got != want {
			t.Errorf("domainRelDir(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStoryKey(t *testing.T) {
	if got := storyKey("ADDS", 3); got != "ADDS#3" {
		t.Errorf("storyKey = %q, want ADDS#3", got)
	}
	if storyKey("A", 1) == storyKey("A", 2) {
		t.Error("distinct positions must yield distinct keys")
	}
	if storyKey("A", 1) == storyKey("B", 1) {
		t.Error("distinct prefixes must yield distinct keys")
	}
}

func TestNewStatsAndBump(t *testing.T) {
	st := newStats()
	if st.Inserted == nil || st.Updated == nil || st.Skipped == nil {
		t.Fatal("newStats must initialize all three maps")
	}
	st.bump("domains", true)
	st.bump("domains", true)
	st.bump("domains", false)
	if st.Inserted["domains"] != 2 {
		t.Errorf("Inserted[domains] = %d, want 2", st.Inserted["domains"])
	}
	if st.Updated["domains"] != 1 {
		t.Errorf("Updated[domains] = %d, want 1", st.Updated["domains"])
	}
	if st.Skipped["domains"] != 0 {
		t.Errorf("Skipped[domains] = %d, want 0", st.Skipped["domains"])
	}
}

func TestReportAdd(t *testing.T) {
	r := &Report{}
	r.Add(SevWarn, "spec-status", "status drifted", "enrollment/add.md")
	r.Add(SevGap, "orphan-fr", "no home in ER", "")
	if len(r.Findings) != 2 {
		t.Fatalf("expected 2 findings, got %d", len(r.Findings))
	}
	f := r.Findings[0]
	if f.Severity != SevWarn || f.Category != "spec-status" || f.Message != "status drifted" || f.Ref != "enrollment/add.md" {
		t.Errorf("first finding not recorded verbatim: %+v", f)
	}
	if r.Findings[1].Ref != "" {
		t.Errorf("second finding ref = %q, want empty", r.Findings[1].Ref)
	}
}

func TestTouchedTablesCoversLayers(t *testing.T) {
	seen := map[string]bool{}
	for _, tbl := range TouchedTables {
		if tbl == "" {
			t.Fatal("TouchedTables contains an empty entry")
		}
		if seen[tbl] {
			t.Errorf("TouchedTables lists %q more than once", tbl)
		}
		seen[tbl] = true
	}
	// Spot-check that a table from each layer is staged, since a missing table
	// would silently drop that layer's writes from the Dolt commit.
	for _, want := range []string{
		"req_domain", "req_requirement", // requirements layer
		"plan_capability", "plan_deliverable", "plan_view", // planning layer
		"test_suite", "test_case", "test_result", // testing layer
	} {
		if !seen[want] {
			t.Errorf("TouchedTables is missing %q", want)
		}
	}
}
