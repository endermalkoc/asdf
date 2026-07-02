package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// In-process CLI test for `cusp coverage` (requirement→test-case coverage: rollups, orphan FRs,
// delivery-status drift), driven through the shared harness.

func TestCLI_Coverage(t *testing.T) {
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
	must("req", "add", "ATT", "export attendance", "--delivery", "covered")

	// Before any test case: 0/2 covered; FR-002 (delivery=covered) is drift.
	out, code := runCLI(t, "coverage")
	if code != 0 {
		t.Fatalf("coverage exit=%d", code)
	}
	if !strings.Contains(out, "0/2") {
		t.Fatalf("expected 0/2 covered:\n%s", out)
	}
	if !strings.Contains(out, "drift") || !strings.Contains(out, "ATT-FR-002") {
		t.Fatalf("expected FR-002 flagged as drift:\n%s", out)
	}
	if jout, code := runCLI(t, "--json", "coverage"); code != 0 || !validJSON(jout) {
		t.Fatalf("coverage --json exit=%d valid=%v", code, validJSON(jout))
	}

	// Link a test case to FR-001.
	suiteID := coverageJSONID(t, "test", "suite", "add", "Core")
	caseID := coverageJSONID(t, "test", "case", "add", "records attendance", "--suite", suiteID)
	must("test", "case", "cover", caseID, "--req", "ATT-FR-001")

	// After linking: 1/2 covered; FR-001 no longer orphan, FR-002 still orphan.
	out, _ = runCLI(t, "coverage")
	if !strings.Contains(out, "1/2") {
		t.Fatalf("expected 1/2 covered after linking:\n%s", out)
	}
	oout, code := runCLI(t, "coverage", "--orphans")
	if code != 0 {
		t.Fatalf("coverage --orphans exit=%d", code)
	}
	if !strings.Contains(oout, "ATT-FR-002") || strings.Contains(oout, "ATT-FR-001") {
		t.Fatalf("--orphans should list FR-002 but not FR-001:\n%s", oout)
	}

	// Cover FR-002 too → 2/2, no orphans, no drift.
	case2 := coverageJSONID(t, "test", "case", "add", "exports attendance", "--suite", suiteID)
	must("test", "case", "cover", case2, "--req", "ATT-FR-002")
	out, _ = runCLI(t, "coverage")
	if !strings.Contains(out, "2/2") || !strings.Contains(out, "no orphan FRs") || strings.Contains(out, "drift") {
		t.Fatalf("expected 2/2, no orphans, no drift:\n%s", out)
	}
}

// TestCLI_Coverage_Empty exercises the zero-requirements path.
func TestCLI_Coverage_Empty(t *testing.T) {
	newCLIWorkspace(t)
	out, code := runCLI(t, "coverage")
	if code != 0 {
		t.Fatalf("coverage (empty) exit=%d", code)
	}
	if !strings.Contains(out, "0/0") || !strings.Contains(out, "no orphan FRs") {
		t.Fatalf("empty coverage should report 0/0 and no orphans:\n%s", out)
	}
}

// coverageJSONID runs a --json add command and returns its "id" field.
func coverageJSONID(t *testing.T, args ...string) string {
	t.Helper()
	out, code := runCLI(t, append([]string{"--json"}, args...)...)
	if code != 0 {
		t.Fatalf("%v: exit=%d\n%s", args, code, out)
	}
	var v struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &v); err != nil || v.ID == "" {
		t.Fatalf("%v: no id in %q (err=%v)", args, out, err)
	}
	return v.ID
}
