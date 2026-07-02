package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// In-process CLI tests for the "maintenance" command group: export, generate, config (generate
// enable/add/sync/…), dolt (status/remote/gc/compact), flatten, and import (qase/tutor), plus the
// cross-cutting error paths (validation → exit 2, not_found → exit 3, unknown subcommand). Each
// test drives the real rootCmd/Execute against a fresh Dolt workspace via the shared harness in
// cli_harness_test.go; to amortize the ~6s server start, each test sweeps many related commands on
// one workspace. Not parallel (runCLI swaps os.Stdout; newCLIWorkspace chdirs).

// maintenanceParseEnvelope decodes the --json error envelope emitted on stdout by Execute so a
// test can assert the mapped exit code and category together.
func maintenanceParseEnvelope(t *testing.T, out string) (code int, category string) {
	t.Helper()
	var env struct {
		Error struct {
			Code     int    `json:"code"`
			Category string `json:"category"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("error envelope not valid JSON: %v\n%s", err, out)
	}
	return env.Error.Code, env.Error.Category
}

// TestCLI_Maintenance_ExportGenerateConfig sweeps the read-artifact and config commands:
// export (stdout + --out file, human + JSON), generate (md/json/html + bad format), and the
// full config/config-generate surface including get/set and their error paths.
func TestCLI_Maintenance_ExportGenerateConfig(t *testing.T) {
	newCLIWorkspace(t)

	// Seed a domain so export/generate have real content to serialize.
	if _, code := runCLI(t, "domain", "add", "core", "Core"); code != 0 {
		t.Fatalf("domain add exit=%d", code)
	}

	// export → stdout: the JSONL snapshot lands on stdout (summary goes to stderr).
	out, code := runCLI(t, "export")
	if code != 0 {
		t.Fatalf("export exit=%d", code)
	}
	if !strings.Contains(out, "\"table\"") || !strings.Contains(out, "req_domain") {
		t.Fatalf("export stdout missing JSONL rows:\n%s", out)
	}
	// Every emitted line must itself be a valid JSON object.
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !validJSON(line) {
			t.Fatalf("export line is not valid JSON: %q", line)
		}
	}

	// export --out <file>: writes the snapshot to the file; stdout carries the summary.
	snap := filepath.Join(t.TempDir(), "snap.jsonl")
	out, code = runCLI(t, "--json", "export", "--out", snap)
	if code != 0 {
		t.Fatalf("export --out exit=%d", code)
	}
	if !validJSON(out) {
		t.Fatalf("export --out --json summary not JSON:\n%s", out)
	}
	body, err := os.ReadFile(snap)
	if err != nil {
		t.Fatalf("read snapshot: %v", err)
	}
	if !strings.Contains(string(body), "req_domain") {
		t.Fatalf("snapshot file missing domain rows:\n%s", body)
	}

	// generate: default markdown, then json and html renderers, then a bad format.
	out, code = runCLI(t, "generate", "--format", "md")
	if code != 0 || !strings.Contains(out, "generated") {
		t.Fatalf("generate md exit=%d out=%q", code, out)
	}
	out, code = runCLI(t, "--json", "generate", "--format", "json")
	if code != 0 || !validJSON(out) {
		t.Fatalf("generate json exit=%d valid=%v", code, validJSON(out))
	}
	if _, code := runCLI(t, "generate", "--format", "html"); code != 0 {
		t.Fatalf("generate html exit=%d", code)
	}
	if _, code := runCLI(t, "generate", "--format", "bogus"); code == 0 {
		t.Fatalf("generate --format bogus should fail")
	}

	// config: show (human + json), and the effective-config get view.
	if out, code := runCLI(t, "config", "show"); code != 0 || !strings.Contains(out, "generate:") {
		t.Fatalf("config show exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "--json", "config", "show"); code != 0 || !validJSON(out) {
		t.Fatalf("config show --json exit=%d valid=%v", code, validJSON(out))
	}
	if out, code := runCLI(t, "config", "get"); code != 0 || !strings.Contains(out, "user.handle") {
		t.Fatalf("config get (all) exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "--json", "config", "get"); code != 0 || !validJSON(out) {
		t.Fatalf("config get --json exit=%d valid=%v", code, validJSON(out))
	}
	// config get <key>: a known key resolves to a bare value.
	if out, code := runCLI(t, "config", "get", "generate.enabled"); code != 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("config get generate.enabled exit=%d out=%q", code, out)
	}

	// config set: a settable identity key round-trips.
	if out, code := runCLI(t, "config", "set", "user.handle", "maint"); code != 0 || !strings.Contains(out, "maint") {
		t.Fatalf("config set user.handle exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "config", "get", "user.handle"); code != 0 || !strings.Contains(out, "maint") {
		t.Fatalf("config get user.handle after set exit=%d out=%q", code, out)
	}

	// config generate: enable → add a format → sync → show → disable → remove.
	if out, code := runCLI(t, "config", "generate", "enable"); code != 0 || !strings.Contains(out, "enabled") {
		t.Fatalf("config generate enable exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "config", "generate", "add", "md"); code != 0 || !strings.Contains(out, "configured") {
		t.Fatalf("config generate add md exit=%d out=%q", code, out)
	}
	// add re-points the same format (replace, not duplicate); "markdown" canonicalizes to "md".
	if _, code := runCLI(t, "config", "generate", "add", "markdown"); code != 0 {
		t.Fatalf("config generate add markdown exit=%d", code)
	}
	if out, code := runCLI(t, "config", "generate", "sync"); code != 0 || !strings.Contains(out, "synced") {
		t.Fatalf("config generate sync exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "--json", "config", "generate", "sync"); code != 0 || !validJSON(out) {
		t.Fatalf("config generate sync --json exit=%d valid=%v", code, validJSON(out))
	}
	if out, code := runCLI(t, "config", "show"); code != 0 || !strings.Contains(out, "md") {
		t.Fatalf("config show after add exit=%d out=%q", code, out)
	}
	if _, code := runCLI(t, "config", "generate", "disable"); code != 0 {
		t.Fatalf("config generate disable exit=%d", code)
	}
	if _, code := runCLI(t, "config", "generate", "remove", "md"); code != 0 {
		t.Fatalf("config generate remove md exit=%d", code)
	}
	// A bad format token is rejected on add.
	if _, code := runCLI(t, "config", "generate", "add", "bogus"); code == 0 {
		t.Fatalf("config generate add bogus should fail")
	}

	// Error paths that live in the config commands:
	// unsettable key → validation (exit 2).
	out, code = runCLI(t, "--json", "config", "set", "dolt.mode", "external")
	if code != 2 {
		t.Fatalf("config set bad key: expected validation exit 2, got %d\n%s", code, out)
	}
	if ec, cat := maintenanceParseEnvelope(t, out); ec != 2 || cat != "validation" {
		t.Fatalf("config set envelope: code=%d cat=%q", ec, cat)
	}
	// unknown config key → not_found (exit 3).
	out, code = runCLI(t, "--json", "config", "get", "no.such.key")
	if code != 3 {
		t.Fatalf("config get unknown key: expected not_found exit 3, got %d\n%s", code, out)
	}
	if ec, cat := maintenanceParseEnvelope(t, out); ec != 3 || cat != "not_found" {
		t.Fatalf("config get envelope: code=%d cat=%q", ec, cat)
	}
}

// TestCLI_Maintenance_DoltAndFlatten sweeps the Dolt housekeeping surface: status (human + json),
// remote add/ls/remove, gc, compact (dry-run + json + a bad --days), flatten (dry-run + missing
// --force), and an unknown `dolt` subcommand.
func TestCLI_Maintenance_DoltAndFlatten(t *testing.T) {
	newCLIWorkspace(t)

	// status: human line, then the structured JSON view.
	if out, code := runCLI(t, "dolt", "status"); code != 0 || !strings.Contains(out, "mode:") {
		t.Fatalf("dolt status exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "--json", "dolt", "status"); code != 0 || !validJSON(out) {
		t.Fatalf("dolt status --json exit=%d valid=%v", code, validJSON(out))
	}

	// remotes: empty → add → ls (human + json) → remove → empty again.
	if out, code := runCLI(t, "dolt", "remote", "ls"); code != 0 || !strings.Contains(out, "no remotes") {
		t.Fatalf("dolt remote ls (empty) exit=%d out=%q", code, out)
	}
	remoteURL := "file://" + filepath.Join(t.TempDir(), "remote")
	if out, code := runCLI(t, "dolt", "remote", "add", "origin", remoteURL); code != 0 || !strings.Contains(out, "origin") {
		t.Fatalf("dolt remote add exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "dolt", "remote", "ls"); code != 0 || !strings.Contains(out, "origin") {
		t.Fatalf("dolt remote ls exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "--json", "dolt", "remote", "ls"); code != 0 || !validJSON(out) {
		t.Fatalf("dolt remote ls --json exit=%d valid=%v", code, validJSON(out))
	}
	if out, code := runCLI(t, "dolt", "remote", "remove", "origin"); code != 0 || !strings.Contains(out, "origin") {
		t.Fatalf("dolt remote remove exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "dolt", "remote", "ls"); code != 0 || !strings.Contains(out, "no remotes") {
		t.Fatalf("dolt remote ls (after remove) exit=%d out=%q", code, out)
	}

	// gc: reclaims chunks, changes no history.
	if out, code := runCLI(t, "dolt", "gc"); code != 0 || !strings.Contains(out, "gc") {
		t.Fatalf("dolt gc exit=%d out=%q", code, out)
	}

	// compact: dry-run preview (human + json), then a bad --days.
	if _, code := runCLI(t, "dolt", "compact", "--dry-run"); code != 0 {
		t.Fatalf("dolt compact --dry-run exit=%d", code)
	}
	if out, code := runCLI(t, "--json", "dolt", "compact", "--dry-run"); code != 0 || !validJSON(out) {
		t.Fatalf("dolt compact --json exit=%d valid=%v", code, validJSON(out))
	}
	if _, code := runCLI(t, "dolt", "compact", "--days", "-1"); code == 0 {
		t.Fatalf("dolt compact --days -1 should fail")
	}

	// flatten: dry-run previews without squashing; without --force it refuses.
	if _, code := runCLI(t, "flatten", "--dry-run"); code != 0 {
		t.Fatalf("flatten --dry-run exit=%d", code)
	}
	if out, code := runCLI(t, "--json", "flatten", "--dry-run"); code != 0 || !validJSON(out) {
		t.Fatalf("flatten --dry-run --json exit=%d valid=%v", code, validJSON(out))
	}
	// Without --force (and more than one commit) flatten refuses with a generic error (exit 1);
	// if the DB happens to already be flat it reports success instead. Either is acceptable, but
	// a plain run must never silently squash.
	if _, code := runCLI(t, "flatten"); code != 0 && code != 1 {
		t.Fatalf("flatten (no force) unexpected exit=%d", code)
	}

	// Unknown subcommand: cobra only rejects unknown positionals at the root (a mid-tree parent
	// like `dolt` with no Run falls through to help, exit 0), so assert the root-level error path.
	if _, code := runCLI(t, "totally-bogus-command"); code == 0 {
		t.Fatalf("unknown root subcommand should be non-zero")
	}
	// An unknown flag is a reliable parse error on any command → non-zero exit.
	if _, code := runCLI(t, "dolt", "gc", "--no-such-flag"); code == 0 {
		t.Fatalf("unknown flag should be non-zero")
	}
}

// TestCLI_Maintenance_Import covers the read-only import reports and the --apply write path for
// the qase adapter (from saved responses), plus a tutor-corpus report from a minimal fixture.
func TestCLI_Maintenance_Import(t *testing.T) {
	newCLIWorkspace(t)

	// qase: saved-response fixture dir (envelope + bare-array shapes, matching qase.ParseDir).
	qaseDir := t.TempDir()
	maintenanceWriteQaseFixture(t, qaseDir)

	// Read-only report (human + JSON).
	if out, code := runCLI(t, "import", "qase", "--from", qaseDir); code != 0 || !strings.Contains(out, "parsed Qase project") {
		t.Fatalf("import qase report exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "--json", "import", "qase", "--from", qaseDir); code != 0 || !validJSON(out) {
		t.Fatalf("import qase --json exit=%d valid=%v", code, validJSON(out))
	}

	// An empty dir parses to an empty graph (missing files are treated as empty), not an error.
	empty := t.TempDir()
	if out, code := runCLI(t, "import", "qase", "--from", empty); code != 0 {
		t.Fatalf("import qase empty exit=%d out=%q", code, out)
	}

	// tutor: minimal corpus report (positional docs path), human + JSON.
	tutorDir := t.TempDir()
	maintenanceWriteTutorCorpus(t, tutorDir)
	if out, code := runCLI(t, "import", "tutor", tutorDir); code != 0 || !strings.Contains(out, "tutor corpus") {
		t.Fatalf("import tutor report exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "--json", "import", "tutor", tutorDir); code != 0 || !validJSON(out) {
		t.Fatalf("import tutor --json exit=%d valid=%v", code, validJSON(out))
	}
	// A docs path with no specs/ dir is an error.
	if _, code := runCLI(t, "import", "tutor", t.TempDir()); code == 0 {
		t.Fatalf("import tutor on a dir without specs/ should fail")
	}

	// --apply writes the parsed graph through the command contract (one transaction/commit).
	// All --apply runs are kept last so the shared --apply flag var can't leak into the
	// read-only runs above.
	if out, code := runCLI(t, "import", "tutor", tutorDir, "--apply"); code != 0 || !strings.Contains(out, "imported tutor corpus") {
		t.Fatalf("import tutor --apply exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "import", "qase", "--from", qaseDir, "--apply"); code != 0 || !strings.Contains(out, "imported Qase tests") {
		t.Fatalf("import qase --apply exit=%d out=%q", code, out)
	}
	// Re-applying is idempotent (deterministic ids); --json emits the apply stats.
	if out, code := runCLI(t, "--json", "import", "qase", "--from", qaseDir, "--apply"); code != 0 || !validJSON(out) {
		t.Fatalf("import qase --apply --json exit=%d valid=%v", code, validJSON(out))
	}
}

// maintenanceWriteQaseFixture materializes the saved-response fixture that qase.ParseDir reads
// (suites.json / cases.json / configurations.json / runs.json / results.json), covering both the
// {"result":{"entities":[…]}} envelope and the bare-array shape.
func maintenanceWriteQaseFixture(t *testing.T, dir string) {
	t.Helper()
	files := map[string]string{
		"suites.json":         `{"result":{"entities":[{"id":1,"title":"Auth"},{"id":2,"title":"Login","parent_id":1}]}}`,
		"cases.json":          `[{"id":10,"title":"sign in ADDR-FR-001","suite_id":2,"priority":"high","severity":4,"type":"functional","layer":"e2e","automation":1,"status":"active","is_flaky":true,"steps":[{"position":1,"action":"a","expected_result":"b"}]}]`,
		"configurations.json": `{"result":{"entities":[{"title":"Browser","configurations":[{"id":100,"title":"Chrome"}]}]}}`,
		"runs.json":           `{"result":{"entities":[{"id":50,"title":"Nightly","status":"complete","configurations":[100]}]}}`,
		"results.json":        `[{"run_id":50,"case_id":10,"configuration":{"id":100},"status":"passed","time_spent_ms":1200}]`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatalf("write qase fixture %s: %v", name, err)
		}
	}
}

// maintenanceWriteTutorCorpus materializes a minimal tutor docs corpus that tutor.Parse accepts:
// specs/ with an index.md (domains), a prefix-registry.md (specs — required), and one spec file
// carrying an FR line and a user story so the report has non-empty counts.
func maintenanceWriteTutorCorpus(t *testing.T, root string) {
	t.Helper()
	files := map[string]string{
		"specs/index.md": "# Index of Domains\n\n" +
			"| Domain | Description |\n" +
			"|--------|-------------|\n" +
			"| enrollment/ | Student enrollment flows |\n",
		"specs/prefix-registry.md": "# Prefix Registry\n\n" +
			"| Prefix | Path | Domain | Status |\n" +
			"|--------|------|--------|--------|\n" +
			"| ADDS | enrollment/add-student.md | enrollment | Active |\n",
		"specs/enrollment/add-student.md": "---\n" +
			"id: ADDS\n" +
			"title: Add Student\n" +
			"domain: enrollment\n" +
			"status: Active\n" +
			"---\n\n" +
			"# Add Student\n\n" +
			"## Requirements\n\n" +
			"- **ADDS-FR-001**: System MUST require a first name and last name.\n\n" +
			"## User Stories\n\n" +
			"### Enroll a student\n\n" +
			"As a registrar I want to add a student so that they appear on the roster.\n",
	}
	for rel, body := range files {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(p), err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write tutor fixture %s: %v", rel, err)
		}
	}
}
