package app_test

import (
	"context"
	"strings"
	"testing"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/storage/schema"
	"github.com/endermalkoc/cusp/internal/testutil"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// Drift detection + repair (doctor --fix). Drift can't be produced through the CLI (the spec prefix
// is immutable, `sql` is read-only), so these inject it via the harness.

func TestDoctor_FRKeyDrift_DetectAndFix(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	req1ID, _ := seedGraph(t, ws) // ATT-FR-001 / ATT-FR-002

	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		_, e := w.Tx.ExecContext(ctx, "UPDATE `req_requirement` SET fr_key='WRONG-KEY' WHERE id=?", req1ID)
		w.MarkDirty("req_requirement")
		return e
	})

	if rep := doctorOn(t, ws); len(rep.FRKeyDrift) != 1 ||
		rep.FRKeyDrift[0].Stored != "WRONG-KEY" || rep.FRKeyDrift[0].Derived != "ATT-FR-001" {
		t.Fatalf("expected fr_key drift WRONG-KEY->ATT-FR-001, got %+v", rep.FRKeyDrift)
	}

	n, err := app.FixFRKeyDrift(context.Background(), ws, app.MutateOpts{Changeset: "main"})
	if err != nil || n != 1 {
		t.Fatalf("FixFRKeyDrift n=%d err=%v", n, err)
	}
	if rep := doctorOn(t, ws); len(rep.FRKeyDrift) != 0 {
		t.Fatalf("drift not fixed: %+v", rep.FRKeyDrift)
	}
}

func TestDoctor_SchemaBehind_Detect(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	// Simulate the cursor of a behind database: drop the newest applied-migration row so the
	// recorded version reads lower than this binary's. (We only assert detection here — actually
	// re-applying the top migration is not safe against an already-migrated schema, since some
	// migrations are non-idempotent DROPs; a genuinely-behind DB never applied them, so real
	// `--fix` runs them once on the older schema.)
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		_, e := w.Tx.ExecContext(ctx,
			"DELETE FROM `schema_migrations` WHERE version = (SELECT v FROM (SELECT MAX(version) v FROM `schema_migrations`) t)")
		w.MarkDirty("schema_migrations")
		return e
	})
	rep := doctorOn(t, ws)
	if rep.Schema.Status != "behind" || rep.Schema.Current >= rep.Schema.Latest || rep.Schema.Pending < 1 {
		t.Fatalf("expected schema behind with pending>=1, got %+v", rep.Schema)
	}
	if rep.Healthy() {
		t.Fatalf("a behind schema should make the report unhealthy")
	}
}

func TestDoctor_SchemaAhead_CannotFix(t *testing.T) {
	ws := testutil.NewWorkspace(t)
	ahead := schema.LatestVersion() + 1
	mutate(t, ws, "main", func(ctx context.Context, w *app.Write) error {
		_, e := w.Tx.ExecContext(ctx,
			"INSERT INTO `schema_migrations` (version, content_hash) VALUES (?, ?)", ahead, strings.Repeat("0", 64))
		w.MarkDirty("schema_migrations")
		return e
	})
	if rep := doctorOn(t, ws); rep.Schema.Status != "ahead" {
		t.Fatalf("expected schema ahead, got %+v", rep.Schema)
	}
	// Migrate is a no-op (nothing pending); it cannot downgrade — still ahead.
	applied, err := app.MigrateWorkspace(context.Background(), ws, testutil.TestActor)
	if err != nil || len(applied) != 0 {
		t.Fatalf("MigrateWorkspace on ahead: applied=%v err=%v", applied, err)
	}
	if rep := doctorOn(t, ws); rep.Schema.Status != "ahead" {
		t.Fatalf("still expected ahead: %+v", rep.Schema)
	}
}

func doctorOn(t *testing.T, ws *workspace.Workspace) app.DoctorReport {
	t.Helper()
	ctx := context.Background()
	r, release, err := app.Reader(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	rep, err := app.Doctor(ctx, r)
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	return rep
}
