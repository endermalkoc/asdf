package workspace

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// ConnectAt refuses a directory that isn't a workspace, pointing at `cusp init`. This path
// needs no Dolt server.
func TestConnectAt_NoWorkspace(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope", ".cusp")
	_, err := ConnectAt(context.Background(), missing, "")
	if err == nil {
		t.Fatal("expected error connecting to a missing workspace")
	}
	if !strings.Contains(err.Error(), "cusp init") {
		t.Fatalf("error should mention `cusp init`, got: %v", err)
	}
}

// Connect resolves the project's `.cusp` from the git root; the repo has no committed `.cusp`,
// so it surfaces ConnectAt's no-workspace error. This exercises Connect + ResolveCuspDir without
// standing up a server.
func TestConnect_ResolvesAndReportsMissingWorkspace(t *testing.T) {
	_, err := Connect(context.Background(), "")
	if err == nil {
		t.Fatal("expected error: repo has no .cusp workspace")
	}
	if !strings.Contains(err.Error(), "cusp init") {
		t.Fatalf("expected a no-workspace error, got: %v", err)
	}
}

// open fails cleanly on an unreachable DSN (bad host/port): sql.Open succeeds lazily, but the
// ping must fail and the error is wrapped.
func TestOpen_PingFailure(t *testing.T) {
	// Port 1 is not a MySQL server; the ping must fail fast.
	_, err := open(context.Background(), t.TempDir(), "root@tcp(127.0.0.1:1)/cusp")
	if err == nil {
		t.Fatal("expected ping failure against a dead DSN")
	}
	if !strings.Contains(err.Error(), "connecting to dolt") {
		t.Fatalf("expected wrapped connect error, got: %v", err)
	}
}
