package main

import (
	"encoding/json"
	"testing"
)

// A golden-path CLI test that drives the real rootCmd/Execute in-process against a real Dolt
// workspace — covering the cobra wiring, flag parsing, emit, and the exit-code / --json error
// envelope that the app/store-level tests skip. Shared setup + runCLI live in cli_harness_test.go.

func TestCLI_GoldenPath(t *testing.T) {
	newCLIWorkspace(t)

	// add on main.
	if _, code := runCLI(t, "domain", "add", "core", "Core"); code != 0 {
		t.Fatalf("domain add exit=%d", code)
	}

	// changeset round-trip: start -> edit on the branch -> diff -> merge.
	if _, code := runCLI(t, "changeset", "start", "add auth"); code != 0 {
		t.Fatalf("changeset start exit=%d", code)
	}
	if _, code := runCLI(t, "domain", "add", "auth", "Auth"); code != 0 {
		t.Fatalf("domain add on branch exit=%d", code)
	}
	if out, code := runCLI(t, "changeset", "diff", "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("changeset diff --json exit=%d valid=%v", code, validJSON(out))
	}
	if _, code := runCLI(t, "changeset", "merge"); code != 0 {
		t.Fatalf("changeset merge exit=%d", code)
	}

	// stats --json reflects both merged domains.
	out, code := runCLI(t, "stats", "--json")
	if code != 0 {
		t.Fatalf("stats exit=%d", code)
	}
	var stats []map[string]any
	if err := json.Unmarshal([]byte(out), &stats); err != nil {
		t.Fatalf("stats --json not valid JSON: %v\n%s", err, out)
	}
	if got := statCount(stats, "domains"); got != "2" {
		t.Fatalf("expected 2 domains after merge, stats says %q", got)
	}

	// Error path: a missing entity exits not_found (3), and under --json emits the error envelope.
	out, code = runCLI(t, "--json", "domain", "show", "does-not-exist")
	if code != 3 {
		t.Fatalf("expected not_found exit 3, got %d", code)
	}
	var env struct {
		Error struct {
			Code     int    `json:"code"`
			Category string `json:"category"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Error.Code != 3 || env.Error.Category != "not_found" {
		t.Fatalf("bad error envelope: err=%v code=%d cat=%q\n%s", err, env.Error.Code, env.Error.Category, out)
	}
}
