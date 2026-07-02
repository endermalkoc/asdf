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

// Shared harness for the in-process CLI tests: it drives the real rootCmd/Execute (which returns
// the exit code) against a real Dolt workspace, so tests can assert exit codes and stdout/JSON
// without spawning a subprocess. Each test group gets its own initialized workspace via
// newCLIWorkspace. RequireDolt-gated; not parallel (runCLI swaps os.Stdout and t.Chdir changes cwd).

// newCLIWorkspace initializes a fresh Cusp workspace in a temp git repo, changes into it, and
// runs `cusp init`. The managed Dolt server is stopped on cleanup. Every CLI test starts here.
func newCLIWorkspace(t *testing.T) {
	t.Helper()
	testutil.RequireDolt(t)
	dir := t.TempDir()
	gitInit(t, dir)
	t.Chdir(dir)
	git.ResetCaches() // re-resolve the repo root at the new cwd
	cuspDir := filepath.Join(dir, ".cusp")
	t.Cleanup(func() {
		_ = doltserver.IgnoreNotRunning(doltserver.Stop(cuspDir))
		git.ResetCaches()
	})
	if _, code := runCLI(t, "init"); code != 0 {
		t.Fatalf("init exit=%d", code)
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
