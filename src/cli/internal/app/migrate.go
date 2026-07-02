package app

import (
	"context"
	"fmt"

	"github.com/endermalkoc/cusp/internal/storage/schema"
	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// Drift repairs used by `cusp doctor --fix`.

// MigrateWorkspace applies any pending schema migrations to the canonical branch and commits them.
// Forward-only: it cannot downgrade a database that is AHEAD of this binary (a newer cusp migrated
// it). Returns the versions applied (nil when already up to date). Fills the gap that `connect`
// does not migrate and there is no standalone migrate command.
func MigrateWorkspace(ctx context.Context, ws *workspace.Workspace, actor workspace.Actor) ([]int, error) {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
		return nil, err
	}
	pending, err := schema.PendingVersions(ctx, conn)
	if err != nil {
		return nil, err
	}
	if len(pending) == 0 {
		return nil, nil
	}
	if _, err := schema.MigrateUp(ctx, conn); err != nil {
		return nil, fmt.Errorf("applying migrations: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "CALL DOLT_ADD('-A')"); err != nil {
		return nil, fmt.Errorf("staging migration: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "CALL DOLT_COMMIT('-m', ?, '--author', ?)",
		"migrate schema", actor.CommitAuthorString()); err != nil {
		return nil, fmt.Errorf("committing migration: %w", err)
	}
	return pending, nil
}

// FixFRKeyDrift recomputes any drifted fr_keys on the mutation target, returning the count fixed.
// It detects first (a clean read) so a drift-free workspace opens no empty mutation.
func FixFRKeyDrift(ctx context.Context, ws *workspace.Workspace, o MutateOpts) (int, error) {
	r, release, err := Reader(ctx, ws, o.Changeset)
	if err != nil {
		return 0, err
	}
	drift, err := store.DriftedFRKeys(ctx, r)
	_ = release()
	if err != nil {
		return 0, err
	}
	if len(drift) == 0 {
		return 0, nil
	}
	o.Summary = "recompute drifted fr_keys"
	err = Mutate(ctx, ws, o, func(ctx context.Context, w *Write) error {
		for _, d := range drift {
			if e := store.RecomputeFRKey(ctx, w.Tx, d.RequirementID, d.Derived); e != nil {
				return e
			}
		}
		w.MarkDirty("req_requirement")
		return nil
	})
	return len(drift), err
}
