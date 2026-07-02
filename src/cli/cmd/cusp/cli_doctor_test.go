package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// In-process CLI tests for `cusp doctor` (aggregated health: integrity + coverage + hygiene, with
// a gating exit code). Reuses coverageJSONID from cli_coverage_test.go.

func TestCLI_Doctor_Unhealthy(t *testing.T) {
	newCLIWorkspace(t)
	must := func(args ...string) {
		t.Helper()
		if _, code := runCLI(t, args...); code != 0 {
			t.Fatalf("%v: exit=%d", args, code)
		}
	}
	must("domain", "add", "att", "Attendance")
	must("domain", "add", "empty", "Empty") // empty-domain hygiene
	must("spec", "add", "ADDS", "--domain", "att", "--prefix", "ATT", "--title", "Attendance")
	must("spec", "add", "EMPTY", "--domain", "att", "--prefix", "EMP", "--title", "Empty") // empty-spec hygiene
	must("req", "add", "ATT", "record attendance")
	// A dangling inline ref (forced past the write-time guard) → an integrity finding.
	if _, code := runCLI(t, "--force", "req", "add", "ATT", "see [[REQ:NOPE-FR-9]]"); code != 0 {
		t.Fatalf("req add --force exit=%d", code)
	}

	out, code := runCLI(t, "doctor")
	if code == 0 {
		t.Fatalf("doctor should exit nonzero on integrity problems:\n%s", out)
	}
	for _, want := range []string{"integrity: 1 problem", "empty-domain", "empty-spec", "orphan FRs", "✗"} {
		if !strings.Contains(out, want) {
			t.Errorf("doctor output missing %q:\n%s", want, out)
		}
	}

	// --json is a single valid object (no trailing error envelope) and still exits nonzero.
	jout, jcode := runCLI(t, "--json", "doctor")
	if jcode == 0 {
		t.Fatalf("doctor --json should exit nonzero")
	}
	var rep struct {
		Integrity []map[string]any `json:"integrity"`
		Hygiene   []map[string]any `json:"hygiene"`
		Coverage  map[string]any   `json:"coverage"`
	}
	if err := json.Unmarshal([]byte(jout), &rep); err != nil {
		t.Fatalf("doctor --json not a single valid object: %v\n%s", err, jout)
	}
	if len(rep.Integrity) != 1 || len(rep.Hygiene) != 2 {
		t.Fatalf("integrity=%d hygiene=%d (want 1, 2)", len(rep.Integrity), len(rep.Hygiene))
	}
}

func TestCLI_Doctor_Healthy(t *testing.T) {
	newCLIWorkspace(t)
	must := func(args ...string) {
		t.Helper()
		if _, code := runCLI(t, args...); code != 0 {
			t.Fatalf("%v: exit=%d", args, code)
		}
	}
	must("domain", "add", "att", "Attendance")
	must("spec", "add", "ADDS", "--domain", "att", "--prefix", "ATT", "--title", "Attendance")
	must("req", "add", "ATT", "record attendance")
	suiteID := coverageJSONID(t, "test", "suite", "add", "Core")
	caseID := coverageJSONID(t, "test", "case", "add", "records", "--suite", suiteID)
	must("test", "case", "cover", caseID, "--req", "ATT-FR-001")

	out, code := runCLI(t, "doctor")
	if code != 0 {
		t.Fatalf("healthy doctor exit=%d:\n%s", code, out)
	}
	if !strings.Contains(out, "✓ healthy") || !strings.Contains(out, "integrity: clean") || !strings.Contains(out, "100%") {
		t.Fatalf("expected a clean, healthy report:\n%s", out)
	}
}
