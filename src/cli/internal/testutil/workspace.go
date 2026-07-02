// Package testutil provides an integration-test harness: an isolated Cusp workspace backed by a
// real (owned) Dolt server in a temp directory, initialized via the same app.InitWorkspace path
// the `cusp init` command uses. Tests that use it need the `dolt` binary on PATH; they skip
// cleanly when it (or -short) says otherwise.
package testutil

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/doltserver"
	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// TestActor is the fixed identity the harness initializes and tests attribute writes to.
var TestActor = workspace.Actor{Handle: "tester", Name: "Tester", Email: "tester@cusp.test"}

// RequireDolt skips the test unless a real Dolt server can be started: the `dolt` binary must be
// on PATH and -short must be off. Integration tests call this first (NewWorkspace does).
func RequireDolt(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping integration test in -short mode")
	}
	if _, err := exec.LookPath("dolt"); err != nil {
		t.Skip("skipping integration test: dolt binary not on PATH")
	}
}

// NewWorkspace spins up an isolated Cusp workspace backed by an owned Dolt server in a fresh temp
// directory (own ephemeral port), initialized via app.InitWorkspace. The server is stopped and
// the workspace closed on test cleanup.
func NewWorkspace(t *testing.T) *workspace.Workspace {
	t.Helper()
	RequireDolt(t)
	ctx := context.Background()
	cuspDir := filepath.Join(t.TempDir(), ".cusp")
	if _, err := app.InitWorkspace(ctx, cuspDir, TestActor); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	ws, err := workspace.ConnectAt(ctx, cuspDir, "")
	if err != nil {
		t.Fatalf("ConnectAt: %v", err)
	}
	t.Cleanup(func() {
		_ = ws.Close()
		_ = doltserver.IgnoreNotRunning(doltserver.Stop(cuspDir))
	})
	return ws
}

// CommitCount returns the number of Dolt commits on `branch` (a pooled connection's branch state
// is nondeterministic, so this checks out the branch first).
func CommitCount(t *testing.T, ws *workspace.Workspace, branch string) int {
	t.Helper()
	ctx := context.Background()
	conn, err := ws.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := versioncontrolops.CheckoutBranch(ctx, conn, branch); err != nil {
		t.Fatalf("checkout %s: %v", branch, err)
	}
	entries, err := versioncontrolops.Log(ctx, conn, 0)
	if err != nil {
		t.Fatalf("log %s: %v", branch, err)
	}
	return len(entries)
}

// DirtyTables returns the working-set changes on `branch` (empty = clean).
func DirtyTables(t *testing.T, ws *workspace.Workspace, branch string) []string {
	t.Helper()
	ctx := context.Background()
	conn, err := ws.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := versioncontrolops.CheckoutBranch(ctx, conn, branch); err != nil {
		t.Fatalf("checkout %s: %v", branch, err)
	}
	dirty, err := versioncontrolops.DirtyWorkingSet(ctx, conn)
	if err != nil {
		t.Fatalf("dirty working set %s: %v", branch, err)
	}
	return dirty
}

// LatestCommitAuthor returns the author string ("Name <email>") of the most recent commit on branch.
func LatestCommitAuthor(t *testing.T, ws *workspace.Workspace, branch string) string {
	t.Helper()
	ctx := context.Background()
	conn, err := ws.Pin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := versioncontrolops.CheckoutBranch(ctx, conn, branch); err != nil {
		t.Fatalf("checkout %s: %v", branch, err)
	}
	entries, err := versioncontrolops.Log(ctx, conn, 1)
	if err != nil || len(entries) == 0 {
		t.Fatalf("log %s: %v", branch, err)
	}
	return entries[0].Author + " <" + entries[0].Email + ">"
}
