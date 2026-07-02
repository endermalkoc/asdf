package workspace_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/testutil"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// These run against a real owned Dolt server via the testutil harness (RequireDolt-gated).
// NewWorkspace already drives ConnectAt → managedDSN → dsnForPort → open on the success path.

func TestWorkspace_DBPinClose(t *testing.T) {
	ws := testutil.NewWorkspace(t)

	if ws.DB() == nil {
		t.Fatal("DB() returned nil pool")
	}
	// A pinned connection is usable and independent of the pool.
	conn, err := ws.Pin(context.Background())
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	var one int
	if err := conn.QueryRowContext(context.Background(), "SELECT 1").Scan(&one); err != nil {
		t.Fatalf("query on pinned conn: %v", err)
	}
	if one != 1 {
		t.Fatalf("SELECT 1 = %d", one)
	}
	if err := conn.Close(); err != nil {
		t.Fatalf("close pinned conn: %v", err)
	}
	// Close releases the pool without error (harness cleanup also calls it — Close is idempotent-safe here since we don't reuse ws after).
	if err := ws.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestDiff_EmptyAndAfterMutation(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	ctx := context.Background()

	conn, err := ws.Pin(ctx)
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	defer conn.Close()
	if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
		t.Fatalf("checkout main: %v", err)
	}

	// A ref against itself has no changes.
	empty, err := workspace.Diff(ctx, conn, "main", "main")
	if err != nil {
		t.Fatalf("Diff(main,main): %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("expected empty diff for main..main, got %+v", empty)
	}

	// Add a domain on main → one new commit whose diff touches req_domain.
	if err := app.Mutate(ctx, ws, app.MutateOpts{
		Summary:   "add diff-domain",
		Changeset: "main",
		Actor:     "diff-tester",
	}, func(ctx context.Context, w *app.Write) error {
		_, e := store.AddDomain(ctx, w.Tx, store.Domain{Slug: "diffd", Name: "Diff Domain"})
		w.MarkDirty("req_domain")
		return e
	}); err != nil {
		t.Fatalf("Mutate add domain: %v", err)
	}

	// Re-pin's branch state is per-connection; re-checkout main on this same conn.
	if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
		t.Fatalf("re-checkout main: %v", err)
	}
	changed, err := workspace.Diff(ctx, conn, "HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("Diff(HEAD~1,HEAD): %v", err)
	}
	if len(changed) == 0 {
		t.Fatal("expected a non-empty diff after adding a domain")
	}
	var sawDomain bool
	for _, d := range changed {
		if d.Table == "req_domain" {
			sawDomain = true
			if d.Added < 1 {
				t.Fatalf("expected >=1 row added to req_domain, got %+v", d)
			}
		}
	}
	if !sawDomain {
		t.Fatalf("req_domain not in diff: %+v", changed)
	}
}

func TestWithRetryTx_CommitsAndPropagatesErrors(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	ctx := context.Background()

	conn, err := ws.Pin(ctx)
	if err != nil {
		t.Fatalf("Pin: %v", err)
	}
	defer conn.Close()

	// Success: a no-op read-only transaction commits.
	var ran bool
	if err := workspace.WithRetryTx(ctx, conn, func(tx *sql.Tx) error {
		ran = true
		var n int
		return tx.QueryRowContext(ctx, "SELECT 1").Scan(&n)
	}); err != nil {
		t.Fatalf("WithRetryTx success: %v", err)
	}
	if !ran {
		t.Fatal("body was not invoked")
	}

	// A non-retryable body error is returned as-is (no retry loop).
	sentinel := errors.New("body failed")
	calls := 0
	err = workspace.WithRetryTx(ctx, conn, func(tx *sql.Tx) error {
		calls++
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if calls != 1 {
		t.Fatalf("non-retryable error should not retry, body ran %d times", calls)
	}
}
