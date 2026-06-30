package app

import (
	"context"
	"fmt"

	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// ResolveBranch picks the branch a command targets: an explicit --changeset, else the
// ambient active changeset, else main. Shared by Mutate (the write target) and Reader
// (the read target) so a command reads from the same branch it would write to —
// command-contract step 2.
func ResolveBranch(ws *workspace.Workspace, changeset string) string {
	if changeset != "" {
		return changeset
	}
	if a := ws.ActiveChangeset(); a != "" {
		return a
	}
	return "main"
}

// Reader pins a connection, checks out the resolved read target (ResolveBranch), and
// returns it as a store.Execer for read commands, plus a release func to return the
// connection. A read on a changeset branch sees that changeset's staged edits; on main
// it sees main. Branch state is connection-scoped, so a read that must honor the active
// changeset cannot use the shared pool (ws.DB(), which sits on main) — it needs this
// pinned, checked-out connection.
//
// Read commands whose rows live on main regardless of the active changeset (e.g.
// `changeset ls`, which reads rev_changeset) should keep using ws.DB() directly.
func Reader(ctx context.Context, ws *workspace.Workspace, changeset string) (store.Execer, func() error, error) {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return nil, nil, err
	}
	branch := ResolveBranch(ws, changeset)
	if err := versioncontrolops.CheckoutBranch(ctx, conn, branch); err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("selecting read branch %q: %w", branch, err)
	}
	return conn, conn.Close, nil
}
