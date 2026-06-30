// Package app holds the shared command pipeline. Mutate is the one wrapper every
// mutating command routes through, so the cross-cutting workflow (connect →
// target branch → validate → transaction → Dolt commit with actor+message) lives
// in one place and no command can drift. See docs/command-contract.md.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// Write is the handle a mutation body receives: it writes rows via Tx and records
// which tables it touched via MarkDirty (so only those are staged).
type Write struct {
	Tx     *sql.Tx
	Actor  workspace.Actor
	Branch string
	dirty  versioncontrolops.DirtyTableTracker
}

// MarkDirty records that the body modified a table.
func (w *Write) MarkDirty(table string) { w.dirty.MarkDirty(table) }

// MutateOpts configures a mutation.
type MutateOpts struct {
	Summary   string // Dolt commit message
	Changeset string // explicit target; "" → active changeset, else main
	Actor     string // actor handle override (--actor)
	DryRun    bool   // validate + run + roll back, no commit
	// Validate runs before any write, on the resolved target branch. Its store.Execer
	// reads that branch — so existence/ref checks see rows staged in the active changeset
	// (e.g. a section added to a spec created in the same changeset), not stale main.
	Validate func(ctx context.Context, r store.Execer) error
}

// Mutate runs body as one atomic, attributed, committed change.
//
// Because DOLT_COMMIT cannot run inside a *sql.Tx and branch state is
// connection-scoped, it pins one connection, checks out the target branch on it,
// runs the row writes in a transaction (with retry), and — after the SQL tx
// commits — records the Dolt commit on that same connection.
func Mutate(ctx context.Context, ws *workspace.Workspace, o MutateOpts, body func(ctx context.Context, w *Write) error) error {
	// 1. Resolve the target branch: --changeset → active changeset → main.
	branch := ResolveBranch(ws, o.Changeset)
	actor := workspace.ResolveActor(o.Actor)

	// 2. Pin one connection (branch state is connection-scoped).
	conn, err := ws.Pin(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	// 3. Select the target branch on this connection — so validation and the writes both
	//    see the target branch, not main.
	if err := versioncontrolops.CheckoutBranch(ctx, conn, branch); err != nil {
		return fmt.Errorf("selecting branch %q: %w", branch, err)
	}

	// 4. Validate before touching anything, reading the now-current target branch.
	if o.Validate != nil {
		if err := o.Validate(ctx, conn); err != nil {
			return err
		}
	}

	// Dry run: execute in a transaction and roll back — no persistence, no commit.
	if o.DryRun {
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		w := &Write{Tx: tx, Actor: actor, Branch: branch}
		if err := body(ctx, w); err != nil {
			_ = tx.Rollback()
			return err
		}
		return tx.Rollback()
	}

	// 5. Run the writes in a transaction (retry on serialization failures).
	w := &Write{Actor: actor, Branch: branch}
	if err := workspace.WithRetryTx(ctx, conn, func(tx *sql.Tx) error {
		w.Tx = tx
		w.dirty = versioncontrolops.DirtyTableTracker{} // reset per attempt
		return body(ctx, w)
	}); err != nil {
		return err
	}

	// 6. Record the Dolt commit on the pinned conn, outside the tx. Capture the pre-commit
	//    HEAD so auto-generation can diff exactly the commit we are about to make.
	parentHash, _ := headCommitHash(ctx, conn)
	dirtyTables := w.dirty.DirtyTables()
	if err := versioncontrolops.StageAndCommit(ctx, conn, dirtyTables, o.Summary, actor.CommitAuthorString()); err != nil {
		return fmt.Errorf("committing change: %w", err)
	}

	// 7. Incrementally regenerate the affected artifacts (best-effort: the change is already
	//    committed, so a generation failure is a warning, not a rolled-back mutation).
	if err := AutoGenerate(ctx, ws, conn, branch, parentHash, dirtyTables); err != nil {
		fmt.Fprintln(os.Stderr, "warning: auto-generate failed:", err)
	}
	return nil
}
