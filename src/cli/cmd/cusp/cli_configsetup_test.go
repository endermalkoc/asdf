package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// In-process CLI tests for the config / setup / ref command groups. Each test starts a fresh
// workspace via newCLIWorkspace (see cli_harness_test.go) and then sweeps many related commands
// on that one workspace to amortize the ~6s Dolt-server start. Assertions check real exit codes
// (0 success, 2 validation, 3 not_found, 1 generic) and key output substrings / JSON shape.

// runCLI (the shared harness) resets only the global persistent flags between in-process Executes;
// cobra leaves each command's own flag vars at their last-parsed value. These wrappers clear the
// setup / ref command-local flag vars first, so a flag set on one call does not leak into the next.

func configsetupRunSetup(t *testing.T, args ...string) (string, int) {
	setupList, setupPrint, setupCheck, setupRemove, setupGlobal = false, false, false, false, false
	return runCLI(t, args...)
}

func configsetupRunRef(t *testing.T, args ...string) (string, int) {
	refSubject, refSubjectType, refSubjectID, refSystem, refExternalID, refURL = "", "", "", "", "", ""
	return runCLI(t, args...)
}

// configsetupJSONField unmarshals a JSON object and returns the named field as a string.
func configsetupJSONField(t *testing.T, out, key string) string {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(out), &m); err != nil {
		t.Fatalf("parse json for key %q: %v\n%s", key, err, out)
	}
	v, ok := m[key]
	if !ok {
		t.Fatalf("json missing key %q in:\n%s", key, out)
	}
	return toString(v)
}

// TestCLI_ConfigSetup_Config sweeps `cusp config`: show/get/set and the generate sub-verbs
// (enable/disable/add/remove/sync), plus the not_found / validation / unknown-format error paths.
func TestCLI_ConfigSetup_Config(t *testing.T) {
	newCLIWorkspace(t)
	// The --out flag writes to a package-level var that cobra does not reset between Executes;
	// clear it so the no-override `add` cases below are deterministic regardless of test order.
	configOutDir = ""

	// --- show ---------------------------------------------------------------
	if out, code := runCLI(t, "config"); code != 0 || !strings.Contains(out, "generate:") {
		t.Fatalf("config exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "config", "show"); code != 0 || !strings.Contains(out, "formats") {
		t.Fatalf("config show exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "config", "show", "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("config show --json exit=%d valid=%v", code, validJSON(out))
	}

	// --- get ----------------------------------------------------------------
	out, code := runCLI(t, "config", "get")
	if code != 0 || !strings.Contains(out, "user.handle") || !strings.Contains(out, "dolt.mode") {
		t.Fatalf("config get exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "config", "get", "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("config get --json exit=%d valid=%v", code, validJSON(out))
	}
	if out, code := runCLI(t, "config", "get", "dolt.mode"); code != 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("config get dolt.mode exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "config", "get", "user.handle", "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("config get user.handle --json exit=%d valid=%v", code, validJSON(out))
	}
	// Unknown key → not_found (exit 3).
	if _, code := runCLI(t, "config", "get", "no.such.key"); code != 3 {
		t.Fatalf("config get unknown-key expected exit 3, got %d", code)
	}

	// --- set (settable user.* identity keys) --------------------------------
	if out, code := runCLI(t, "config", "set", "user.handle", "emalkoc"); code != 0 || !strings.Contains(out, "user.handle") {
		t.Fatalf("config set user.handle exit=%d out=%q", code, out)
	}
	// The persisted identity is read back by `config get`.
	if out, code := runCLI(t, "config", "get", "user.handle"); code != 0 || strings.TrimSpace(out) != "emalkoc" {
		t.Fatalf("config get user.handle after set: exit=%d out=%q", code, out)
	}
	if _, code := runCLI(t, "config", "set", "user.name", "Ender Malkoc"); code != 0 {
		t.Fatalf("config set user.name exit=%d", code)
	}
	if _, code := runCLI(t, "config", "set", "user.email", "ender@example.com"); code != 0 {
		t.Fatalf("config set user.email exit=%d", code)
	}
	if out, code := runCLI(t, "config", "set", "user.handle", "x", "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("config set --json exit=%d valid=%v", code, validJSON(out))
	}
	// Non-settable key → validation (exit 2).
	if _, code := runCLI(t, "config", "set", "dolt.mode", "external"); code != 2 {
		t.Fatalf("config set non-settable key expected exit 2, got %d", code)
	}

	// --- generate sub-verbs -------------------------------------------------
	// Sync with no formats configured is a no-op that reports so.
	if out, code := runCLI(t, "config", "generate", "sync"); code != 0 || !strings.Contains(out, "no formats") {
		t.Fatalf("config generate sync (no formats) exit=%d out=%q", code, out)
	}
	if _, code := runCLI(t, "config", "generate", "enable"); code != 0 {
		t.Fatalf("config generate enable exit=%d", code)
	}
	if out, code := runCLI(t, "config", "generate", "add", "md"); code != 0 || !strings.Contains(out, "md") {
		t.Fatalf("config generate add md exit=%d out=%q", code, out)
	}
	// "markdown" canonicalizes to "md".
	if _, code := runCLI(t, "config", "generate", "add", "markdown"); code != 0 {
		t.Fatalf("config generate add markdown exit=%d", code)
	}
	if _, code := runCLI(t, "config", "generate", "add", "html"); code != 0 {
		t.Fatalf("config generate add html exit=%d", code)
	}
	// An --out override is surfaced in `config show`.
	configOutDir = ""
	if out, code := runCLI(t, "config", "generate", "add", "json", "--out", "artifacts/json"); code != 0 || !strings.Contains(out, "json") {
		t.Fatalf("config generate add json --out exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "config", "show"); code != 0 || !strings.Contains(out, "md") || !strings.Contains(out, "html") || !strings.Contains(out, "(override)") {
		t.Fatalf("config show after adds exit=%d out=%q", code, out)
	}
	// Sync now materializes the configured formats.
	if out, code := runCLI(t, "config", "generate", "sync"); code != 0 || !strings.Contains(out, "synced") {
		t.Fatalf("config generate sync exit=%d out=%q", code, out)
	}
	if out, code := runCLI(t, "config", "generate", "sync", "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("config generate sync --json exit=%d valid=%v", code, validJSON(out))
	}
	if out, code := runCLI(t, "config", "generate", "remove", "json"); code != 0 || !strings.Contains(out, "removed") {
		t.Fatalf("config generate remove json exit=%d out=%q", code, out)
	}
	// Unknown format → generic error (exit 1) on both add and remove.
	if _, code := runCLI(t, "config", "generate", "add", "pdf"); code != 1 {
		t.Fatalf("config generate add unknown-format expected exit 1, got %d", code)
	}
	if _, code := runCLI(t, "config", "generate", "remove", "pdf"); code != 1 {
		t.Fatalf("config generate remove unknown-format expected exit 1, got %d", code)
	}
	if out, code := runCLI(t, "config", "generate", "disable"); code != 0 || !strings.Contains(out, "disabled") {
		t.Fatalf("config generate disable exit=%d out=%q", code, out)
	}
	// `config generate` with no sub-verb falls back to showing the config.
	if out, code := runCLI(t, "config", "generate"); code != 0 || !strings.Contains(out, "generate:") {
		t.Fatalf("config generate (bare) exit=%d out=%q", code, out)
	}
}

// TestCLI_ConfigSetup_Setup sweeps `cusp setup`: --list / --print, the validation error paths,
// and a full install → check → remove idempotency cycle for both the claude and codex recipes,
// writing into the temp git repo the harness set up (which is fine and expected).
func TestCLI_ConfigSetup_Setup(t *testing.T) {
	newCLIWorkspace(t)

	// --- list / print / errors ---------------------------------------------
	out, code := configsetupRunSetup(t, "setup", "--list")
	if code != 0 || !strings.Contains(out, "claude") || !strings.Contains(out, "codex") {
		t.Fatalf("setup --list exit=%d out=%q", code, out)
	}
	if out, code := configsetupRunSetup(t, "setup", "--list", "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("setup --list --json exit=%d valid=%v", code, validJSON(out))
	}
	if out, code := configsetupRunSetup(t, "setup", "--print"); code != 0 || strings.TrimSpace(out) == "" {
		t.Fatalf("setup --print exit=%d out-len=%d", code, len(out))
	}
	// No recipe and no --print/--list → validation (exit 2).
	if _, code := configsetupRunSetup(t, "setup"); code != 2 {
		t.Fatalf("setup (no args) expected exit 2, got %d", code)
	}
	// Unknown recipe → validation (exit 2).
	if _, code := configsetupRunSetup(t, "setup", "bogus"); code != 2 {
		t.Fatalf("setup bogus expected exit 2, got %d", code)
	}

	// --- claude + codex install/check/remove cycles -------------------------
	for _, recipe := range []string{"claude", "codex"} {
		// Before install nothing is present.
		if out, code := configsetupRunSetup(t, "setup", recipe, "--check"); code != 0 || !strings.Contains(out, "missing") {
			t.Fatalf("setup %s --check (pre-install) exit=%d out=%q", recipe, code, out)
		}
		// Install creates artifacts.
		if out, code := configsetupRunSetup(t, "setup", recipe); code != 0 || !strings.Contains(out, "created") {
			t.Fatalf("setup %s install exit=%d out=%q", recipe, code, out)
		}
		// Re-install is idempotent (everything unchanged).
		if out, code := configsetupRunSetup(t, "setup", recipe); code != 0 || !strings.Contains(out, "unchanged") {
			t.Fatalf("setup %s re-install exit=%d out=%q", recipe, code, out)
		}
		// Check now reports the artifacts present.
		if out, code := configsetupRunSetup(t, "setup", recipe, "--check"); code != 0 || !strings.Contains(out, "present") {
			t.Fatalf("setup %s --check (post-install) exit=%d out=%q", recipe, code, out)
		}
		// JSON install output is a valid array of results.
		if out, code := configsetupRunSetup(t, "setup", recipe, "--json"); code != 0 || !validJSON(out) {
			t.Fatalf("setup %s --json exit=%d valid=%v", recipe, code, validJSON(out))
		}
		// Remove tears the artifacts back down.
		if _, code := configsetupRunSetup(t, "setup", recipe, "--remove"); code != 0 {
			t.Fatalf("setup %s --remove exit=%d", recipe, code)
		}
		// Remove again is idempotent.
		if _, code := configsetupRunSetup(t, "setup", recipe, "--remove"); code != 0 {
			t.Fatalf("setup %s --remove (again) exit=%d", recipe, code)
		}
		// Check after removal reports the managed artifacts gone.
		if out, code := configsetupRunSetup(t, "setup", recipe, "--check"); code != 0 || !strings.Contains(out, "missing") {
			t.Fatalf("setup %s --check (post-remove) exit=%d out=%q", recipe, code, out)
		}
	}
}

// TestCLI_ConfigSetup_Ref sweeps `cusp ref` add/ls/show/rm against a real requirement subject
// (domain → spec+prefix → requirement), covering idempotent re-add, per-subject filtering, and
// the validation / not_found error paths.
func TestCLI_ConfigSetup_Ref(t *testing.T) {
	newCLIWorkspace(t)

	// Prerequisites: a domain, a spec with an FR prefix, and a requirement to reference.
	if _, code := runCLI(t, "domain", "add", "core", "Core"); code != 0 {
		t.Fatalf("domain add exit=%d", code)
	}
	if _, code := runCLI(t, "spec", "add", "attendance.md", "--domain", "core", "--prefix", "ATT", "--title", "Attendance"); code != 0 {
		t.Fatalf("spec add exit=%d", code)
	}
	out, code := runCLI(t, "req", "add", "ATT", "The system tracks attendance.", "--json")
	if code != 0 || !validJSON(out) {
		t.Fatalf("req add exit=%d valid=%v out=%q", code, validJSON(out), out)
	}
	frKey := configsetupJSONField(t, out, "fr_key")
	if frKey == "" {
		t.Fatalf("empty fr_key from req add: %s", out)
	}

	// --- add (bare fr_key subject) ------------------------------------------
	out, code = configsetupRunRef(t, "ref", "add", "--subject", frKey, "--system", "github", "--external-id", "GH-1", "--url", "https://gh/1", "--json")
	if code != 0 || !validJSON(out) {
		t.Fatalf("ref add exit=%d valid=%v out=%q", code, validJSON(out), out)
	}
	refID := configsetupJSONField(t, out, "id")
	if refID == "" {
		t.Fatalf("empty ref id from add: %s", out)
	}

	// Re-add same (subject, system) is idempotent — updates in place (human line says "updated").
	out, code = configsetupRunRef(t, "ref", "add", "--subject", "REQ:"+frKey, "--system", "github", "--external-id", "GH-2")
	if code != 0 || !strings.Contains(out, "updated") {
		t.Fatalf("ref add (idempotent update) exit=%d out=%q", code, out)
	}

	// A second system on the same subject is a new ref.
	if out, code := configsetupRunRef(t, "ref", "add", "--subject", frKey, "--system", "jira", "--external-id", "PROJ-9"); code != 0 || !strings.Contains(out, "added") {
		t.Fatalf("ref add (second system) exit=%d out=%q", code, out)
	}

	// --- ls / show ----------------------------------------------------------
	if out, code := configsetupRunRef(t, "ref", "ls"); code != 0 || !strings.Contains(out, "github") || !strings.Contains(out, "jira") {
		t.Fatalf("ref ls exit=%d out=%q", code, out)
	}
	if out, code := configsetupRunRef(t, "ref", "ls", "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("ref ls --json exit=%d valid=%v", code, validJSON(out))
	}
	// Filter to a single subject.
	if out, code := configsetupRunRef(t, "ref", "ls", "--subject", frKey); code != 0 || !strings.Contains(out, "github") {
		t.Fatalf("ref ls --subject exit=%d out=%q", code, out)
	}
	if out, code := configsetupRunRef(t, "ref", "show", refID); code != 0 || !strings.Contains(out, "github") {
		t.Fatalf("ref show exit=%d out=%q", code, out)
	}
	if out, code := configsetupRunRef(t, "ref", "show", refID, "--json"); code != 0 || !validJSON(out) {
		t.Fatalf("ref show --json exit=%d valid=%v", code, validJSON(out))
	}
	// Unknown id → not_found (exit 3).
	if _, code := configsetupRunRef(t, "ref", "show", "no-such-ref"); code != 3 {
		t.Fatalf("ref show unknown expected exit 3, got %d", code)
	}

	// --- error paths on add -------------------------------------------------
	// Missing --system / --external-id → validation (exit 2).
	if _, code := configsetupRunRef(t, "ref", "add", "--subject", frKey); code != 2 {
		t.Fatalf("ref add missing system/external-id expected exit 2, got %d", code)
	}
	// No subject at all → validation (exit 2).
	if _, code := configsetupRunRef(t, "ref", "add", "--system", "github", "--external-id", "X"); code != 2 {
		t.Fatalf("ref add no subject expected exit 2, got %d", code)
	}
	// --subject-type without --subject-id → validation (exit 2).
	if _, code := configsetupRunRef(t, "ref", "add", "--subject-type", "requirement", "--system", "github", "--external-id", "X"); code != 2 {
		t.Fatalf("ref add half subject-type/id expected exit 2, got %d", code)
	}
	// Unknown subject → not_found (exit 3).
	if _, code := configsetupRunRef(t, "ref", "add", "--subject", "REQ:NOPE-FR-999", "--system", "github", "--external-id", "X"); code != 3 {
		t.Fatalf("ref add unknown subject expected exit 3, got %d", code)
	}

	// --- rm -----------------------------------------------------------------
	if out, code := configsetupRunRef(t, "ref", "rm", refID); code != 0 || !strings.Contains(out, "deleted") {
		t.Fatalf("ref rm exit=%d out=%q", code, out)
	}
	// Removing again → not_found (exit 3).
	if _, code := configsetupRunRef(t, "ref", "rm", refID); code != 3 {
		t.Fatalf("ref rm (again) expected exit 3, got %d", code)
	}
}
