package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// In-process CLI tests for the testing layer verbs (`cusp test suite|case|step|config|run|result`).
// They drive the real rootCmd/Execute against a live Dolt workspace via the shared harness
// (newCLIWorkspace / runCLI in cli_harness_test.go) and assert exit codes, human lines, and --json.
//
// The whole group runs on ONE workspace in ONE function: the command-local flag vars (tcaseSuite,
// trunStatus, …) are package globals that cobra does not reset between Executes, so a single DB
// keeps every captured id valid and lets ordering guard the unconditionally-validated flags — the
// `run add` status/milestone error cases are therefore driven last, after every happy `run add`.

// testingRowID parses the "id" field from an add command's --json output (every testing-layer row
// carries an "id" json tag).
func testingRowID(t *testing.T, out string) string {
	t.Helper()
	var row struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(out), &row); err != nil {
		t.Fatalf("parse id from JSON %q: %v", out, err)
	}
	if row.ID == "" {
		t.Fatalf("empty id in JSON %q", out)
	}
	return row.ID
}

func TestCLI_Testing_Sweep(t *testing.T) {
	newCLIWorkspace(t)

	// ---- test suite: add (root + nested), ls, show ------------------------
	// --parent "" guards the first add against any stale parent left in the global by another test.
	out, code := runCLI(t, "test", "suite", "add", "Auth Suite", "--parent", "", "--description", "auth flows", "--json")
	if code != 0 || !validJSON(out) {
		t.Fatalf("suite add exit=%d valid=%v out=%s", code, validJSON(out), out)
	}
	rootSuite := testingRowID(t, out)

	// nested suite exercises validateSuiteParent's success branch + position.
	out, code = runCLI(t, "test", "suite", "add", "Login", "--parent", rootSuite, "--position", "1", "--json")
	if code != 0 {
		t.Fatalf("nested suite add exit=%d out=%s", code, out)
	}
	childSuite := testingRowID(t, out)

	if out, code = runCLI(t, "test", "suite", "ls"); code != 0 || !strings.Contains(out, rootSuite) || !strings.Contains(out, "Auth Suite") {
		t.Fatalf("suite ls exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "suite", "show", rootSuite); code != 0 || !strings.Contains(out, "Auth Suite") {
		t.Fatalf("suite show exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "suite", "show", childSuite, "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("suite show --json exit=%d out=%s", code, out)
	}

	// ---- test case: add, ls (all + by suite), show ------------------------
	out, code = runCLI(t, "test", "case", "add", "Login works",
		"--suite", childSuite, "--description", "user can log in", "--preconditions", "account exists",
		"--layer", "e2e", "--type", "functional", "--priority", "2", "--severity", "normal",
		"--automation", "automated", "--status", "active", "--path", "tests/login.spec.ts", "--json")
	if code != 0 || !validJSON(out) {
		t.Fatalf("case add exit=%d valid=%v out=%s", code, validJSON(out), out)
	}
	caseID := testingRowID(t, out)

	out, code = runCLI(t, "test", "case", "add", "Logout works", "--suite", childSuite, "--layer", "e2e", "--json")
	if code != 0 {
		t.Fatalf("case add 2 exit=%d out=%s", code, out)
	}
	case2 := testingRowID(t, out)

	if out, code = runCLI(t, "test", "case", "ls"); code != 0 || !strings.Contains(out, "Login works") {
		t.Fatalf("case ls exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "case", "ls", childSuite, "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("case ls suite --json exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "case", "show", caseID); code != 0 || !strings.Contains(out, "Login works") {
		t.Fatalf("case show exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "case", "show", caseID, "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("case show --json exit=%d out=%s", code, out)
	}

	// ---- test step: add (two, one with explicit position), ls -------------
	// steps live under `test case step` (testStepCmd is a subcommand of testCaseCmd).
	if out, code = runCLI(t, "test", "case", "step", "add", caseID, "--action", "enter credentials", "--expected", "form accepts", "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("step add exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "case", "step", "add", caseID, "--action", "click submit", "--expected", "dashboard shows", "--position", "2"); code != 0 || !strings.Contains(out, "added step") {
		t.Fatalf("step add 2 exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "case", "step", "ls", caseID); code != 0 || !strings.Contains(out, "enter credentials") {
		t.Fatalf("step ls exit=%d out=%s", code, out)
	}

	// ---- test config: add, ls, show ---------------------------------------
	out, code = runCLI(t, "test", "config", "add", "browser", "chrome", "--description", "Chrome latest", "--json")
	if code != 0 || !validJSON(out) {
		t.Fatalf("config add exit=%d out=%s", code, out)
	}
	cfgID := testingRowID(t, out)
	if out, code = runCLI(t, "test", "config", "ls"); code != 0 || !strings.Contains(out, "browser") {
		t.Fatalf("config ls exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "config", "show", cfgID, "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("config show --json exit=%d out=%s", code, out)
	}

	// ---- test run: add, ls, show ------------------------------------------
	out, code = runCLI(t, "test", "run", "add", "Sprint 1", "--status", "active", "--description", "first cycle", "--json")
	if code != 0 || !validJSON(out) {
		t.Fatalf("run add exit=%d out=%s", code, out)
	}
	runID := testingRowID(t, out)
	if out, code = runCLI(t, "test", "run", "ls"); code != 0 || !strings.Contains(out, runID) {
		t.Fatalf("run ls exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "run", "show", runID); code != 0 || !strings.Contains(out, "Sprint 1") {
		t.Fatalf("run show exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "run", "show", runID, "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("run show --json exit=%d out=%s", code, out)
	}

	// ---- test result: add (idempotent upsert), ls -------------------------
	out, code = runCLI(t, "test", "result", "add", "--run", runID, "--case", caseID, "--status", "passed", "--comment", "green", "--duration", "150", "--config", "", "--json")
	if code != 0 || !validJSON(out) {
		t.Fatalf("result add exit=%d out=%s", code, out)
	}
	resID := testingRowID(t, out)
	// re-report the same (run, case, config) identity: the deterministic id converges.
	out, code = runCLI(t, "test", "result", "add", "--run", runID, "--case", caseID, "--status", "failed", "--comment", "flip", "--config", "", "--json")
	if code != 0 {
		t.Fatalf("result re-add exit=%d out=%s", code, out)
	}
	if got := testingRowID(t, out); got != resID {
		t.Fatalf("result upsert should converge: first=%s second=%s", resID, got)
	}
	// a distinct result (different case) and one bound to a configuration (distinct identity).
	if out, code = runCLI(t, "test", "result", "add", "--run", runID, "--case", case2, "--status", "skipped", "--config", ""); code != 0 || !strings.Contains(out, "recorded skipped") {
		t.Fatalf("result add 2 exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "result", "add", "--run", runID, "--case", caseID, "--status", "blocked", "--config", cfgID); code != 0 || !strings.Contains(out, "recorded blocked") {
		t.Fatalf("result add config exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "result", "ls", runID); code != 0 || !strings.Contains(out, "case="+caseID) {
		t.Fatalf("result ls exit=%d out=%s", code, out)
	}
	if out, code = runCLI(t, "test", "result", "ls", runID, "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("result ls --json exit=%d out=%s", code, out)
	}

	// ================= error paths =================
	// not-found (exit 3) on show/get across the layer.
	if _, code = runCLI(t, "test", "suite", "show", "no-such-suite"); code != 3 {
		t.Fatalf("suite show missing want 3 got %d", code)
	}
	if _, code = runCLI(t, "test", "config", "show", "no-such-cfg"); code != 3 {
		t.Fatalf("config show missing want 3 got %d", code)
	}
	if _, code = runCLI(t, "test", "run", "show", "no-such-run"); code != 3 {
		t.Fatalf("run show missing want 3 got %d", code)
	}
	// not-found under --json emits the structured error envelope.
	out, code = runCLI(t, "--json", "test", "case", "show", "no-such-case")
	if code != 3 {
		t.Fatalf("case show missing want 3 got %d out=%s", code, out)
	}
	var env struct {
		Error struct {
			Code     int    `json:"code"`
			Category string `json:"category"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil || env.Error.Code != 3 || env.Error.Category != "not_found" {
		t.Fatalf("bad error envelope: err=%v code=%d cat=%q out=%s", err, env.Error.Code, env.Error.Category, out)
	}

	// suite add with a missing parent -> not found (3).
	if _, code = runCLI(t, "test", "suite", "add", "Orphan", "--parent", "no-such-parent"); code != 3 {
		t.Fatalf("suite add bad parent want 3 got %d", code)
	}
	// case add into a missing suite -> not found (3).
	if _, code = runCLI(t, "test", "case", "add", "Ghost", "--suite", "no-such-suite", "--layer", "unit"); code != 3 {
		t.Fatalf("case add missing suite want 3 got %d", code)
	}
	// case add with an invalid enum -> validation (2).
	if _, code = runCLI(t, "test", "case", "add", "BadLayer", "--suite", childSuite, "--layer", "teleport"); code != 2 {
		t.Fatalf("case add bad layer want 2 got %d", code)
	}
	// case add with an out-of-range priority -> validation (2); --layer valid so the priority
	// branch is the one that trips.
	if _, code = runCLI(t, "test", "case", "add", "BadPri", "--suite", childSuite, "--layer", "unit", "--priority", "9"); code != 2 {
		t.Fatalf("case add bad priority want 2 got %d", code)
	}
	// step add to a missing case -> not found (3).
	if _, code = runCLI(t, "test", "case", "step", "add", "no-such-case", "--action", "noop"); code != 3 {
		t.Fatalf("step add missing case want 3 got %d", code)
	}
	// step ls of a missing case is not an error — it just lists nothing (exit 0).
	if out, code = runCLI(t, "test", "case", "step", "ls", "no-such-case"); code != 0 {
		t.Fatalf("step ls missing case want 0 got %d out=%s", code, out)
	}

	// result add error surface: missing run / missing case / invalid status / missing config.
	if _, code = runCLI(t, "test", "result", "add", "--run", "no-such-run", "--case", caseID, "--status", "passed", "--config", ""); code != 3 {
		t.Fatalf("result add missing run want 3 got %d", code)
	}
	if _, code = runCLI(t, "test", "result", "add", "--run", runID, "--case", "no-such-case", "--status", "passed", "--config", ""); code != 3 {
		t.Fatalf("result add missing case want 3 got %d", code)
	}
	if _, code = runCLI(t, "test", "result", "add", "--run", runID, "--case", caseID, "--status", "exploded", "--config", ""); code != 2 {
		t.Fatalf("result add bad status want 2 got %d", code)
	}
	if _, code = runCLI(t, "test", "result", "add", "--run", runID, "--case", caseID, "--status", "passed", "--config", "no-such-cfg"); code != 3 {
		t.Fatalf("result add missing config want 3 got %d", code)
	}

	// run add error surface — these must be the LAST run adds: the status/milestone globals are
	// validated unconditionally, so each explicitly passes a valid --status to override any prior
	// leftover, and the milestone case runs after the status case.
	if _, code = runCLI(t, "test", "run", "add", "BadRun", "--status", "spinning"); code != 2 {
		t.Fatalf("run add bad status want 2 got %d", code)
	}
	if _, code = runCLI(t, "test", "run", "add", "MilestoneRun", "--status", "active", "--milestone", "no-such-ms"); code != 3 {
		t.Fatalf("run add bad milestone want 3 got %d", code)
	}
}
