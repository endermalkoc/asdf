package qase

import "testing"

func iptr(n int) *int { return &n }

func TestBuildGraph_mappingAndCoverage(t *testing.T) {
	suites := []suiteWire{
		{ID: 1, Title: "Auth", ParentID: nil, Position: iptr(1)},
		{ID: 2, Title: "Login", ParentID: iptr(1)},
	}
	cases := []caseWire{{
		ID: 10, Title: "sign in", Description: "covers ADDR-FR-001 and ADDR-FR-001 dup", SuiteID: iptr(2),
		Priority:   flexEnum{Str: "high"},
		Severity:   flexEnum{Int: 4, IsInt: true}, // → normal
		Type:       flexEnum{Str: "functional"},
		Layer:      flexEnum{Str: "e2e"},
		Automation: flexEnum{Int: 1, IsInt: true}, // → automated
		Status:     flexEnum{Str: "active"},
		IsFlaky:    true,
		Steps:      []stepWire{{Position: iptr(1), Action: "a", ExpectedResult: "b"}},
		Tags:       []tagWire{{Title: "BILL-FR-007"}},
	}}
	runs := []runWire{{ID: 50, Title: "Nightly", Status: flexEnum{Str: "complete"}, Configs: []int{100}}}
	groups := []configGroupWire{{Title: "Browser", Configurations: []struct {
		ID    int    `json:"id"`
		Title string `json:"title"`
	}{{ID: 100, Title: "Chrome"}}}}
	results := []resultWire{{RunID: 50, CaseID: 10, Config: &configRef{ID: 100}, Status: "passed", TimeMs: iptr(1200)}}

	g, rep := BuildGraph(suites, cases, groups, runs, results)

	if len(g.TestSuites) != 2 || g.TestSuites[1].ParentSourceID != "1" {
		t.Fatalf("suite tree: %+v", g.TestSuites)
	}
	c := g.TestCases[0]
	if c.SuiteSourceID != "2" || c.Layer != "e2e" || c.Type != "functional" || c.Severity != "normal" ||
		c.Automation != "automated" || c.Status != "active" || !c.IsFlaky {
		t.Fatalf("case enums: %+v", c)
	}
	if c.Priority == nil || *c.Priority != 1 {
		t.Fatalf("priority high → 1, got %v", c.Priority)
	}
	// FR keys deduped across description + tags.
	want := map[string]bool{"ADDR-FR-001": true, "BILL-FR-007": true}
	if len(c.FRKeys) != 2 {
		t.Fatalf("fr keys: %v", c.FRKeys)
	}
	for _, k := range c.FRKeys {
		if !want[k] {
			t.Errorf("unexpected fr key %q", k)
		}
	}
	if g.TestRuns[0].Status != "complete" || len(g.TestRuns[0].ConfigSourceIDs) != 1 {
		t.Fatalf("run: %+v", g.TestRuns[0])
	}
	if len(g.Configurations) != 1 || g.Configurations[0].Group != "Browser" || g.Configurations[0].Name != "Chrome" {
		t.Fatalf("config: %+v", g.Configurations)
	}
	if g.TestResults[0].Status != "passed" || g.TestResults[0].ConfigSourceID != "100" {
		t.Fatalf("result: %+v", g.TestResults[0])
	}
	if rep.Counts["fr_citations"] != 2 {
		t.Errorf("fr_citations count: %d", rep.Counts["fr_citations"])
	}
}
