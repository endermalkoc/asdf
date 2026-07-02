package main

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/endermalkoc/cusp/internal/doltserver"
	"github.com/endermalkoc/cusp/internal/git"
	"github.com/endermalkoc/cusp/internal/testutil"
)

// A golden-path CLI test that drives the real rootCmd/Execute in-process against a real Dolt
// workspace — covering the cobra wiring, flag parsing, emit, and the exit-code / --json error
// envelope that the app/store-level tests skip. It runs each command through Execute() (which now
// returns the code instead of calling os.Exit), capturing stdout. Needs the `dolt` binary; skips
// under -short. Not parallel: it swaps os.Stdout and changes the working directory.

func TestCLI_GoldenPath(t *testing.T) {
	testutil.RequireDolt(t)

	// A workspace lives at a git repo root, so init inside a fresh git repo.
	dir := t.TempDir()
	gitInit(t, dir)
	t.Chdir(dir)
	git.ResetCaches() // re-resolve the repo root at the new cwd
	cuspDir := filepath.Join(dir, ".cusp")
	t.Cleanup(func() {
		_ = doltserver.IgnoreNotRunning(doltserver.Stop(cuspDir))
		git.ResetCaches()
	})

	// init -> clean workspace.
	if _, code := runCLI(t, "init"); code != 0 {
		t.Fatalf("init exit=%d", code)
	}

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

// runCLI drives the real command tree once and returns its stdout and exit code. It resets the
// global flags (cobra does not clear persistent-flag vars between Executes) and captures os.Stdout.
func runCLI(t *testing.T, args ...string) (string, int) {
	t.Helper()
	resetGlobalFlags()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	rootCmd.SetArgs(args)
	code := Execute()
	_ = w.Close()
	os.Stdout = old

	out, _ := io.ReadAll(r)
	_ = r.Close()
	return string(out), code
}

func resetGlobalFlags() {
	flagDSN = ""
	flagJSON = false
	flagActor = ""
	flagChangeset = ""
	flagForce = false
	flagStrict = false
	flagDryRun = false
}

func gitInit(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "smoke@cusp.test"},
		{"config", "user.name", "Smoke"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
}

func validJSON(s string) bool {
	return json.Valid([]byte(s))
}

func statCount(stats []map[string]any, kind string) string {
	for _, s := range stats {
		if s["kind"] == kind {
			return toString(s["count"])
		}
	}
	return ""
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}
