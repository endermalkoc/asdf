package app

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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

// Reader pins a connection, points it at the resolved read target (ResolveBranch), and
// returns it as a store.Execer for read commands, plus a release func to return the
// connection. A read on a changeset branch sees that changeset's staged edits; on main
// it sees main. Branch state is connection-scoped, so a read that must honor the active
// changeset cannot use the shared pool (ws.DB(), which sits on main) — it needs this
// pinned, checked-out connection.
//
// The target may also be a commit hash (e.g. a changeset's base_commit, for an exact
// base/head diff render). Dolt has no detached HEAD, so a commit is read through its
// read-only revision database (`USE db/commit`); the connection is reset before it returns
// to the pool so the next borrower doesn't inherit that view.
//
// Read commands whose rows live on main regardless of the active changeset (e.g.
// `changeset ls`, which reads rev_changeset) should keep using ws.DB() directly.
func Reader(ctx context.Context, ws *workspace.Workspace, changeset string) (store.Execer, func() error, error) {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return nil, nil, err
	}
	target := ResolveBranch(ws, changeset)
	resetDB, err := selectReadRef(ctx, conn, target)
	if err != nil {
		_ = conn.Close()
		return nil, nil, fmt.Errorf("selecting read target %q: %w", target, err)
	}
	release := func() error {
		// If we borrowed a read-only revision database (a commit ref), drop the revision view by
		// switching the pooled connection back to the plain database first.
		if resetDB != "" {
			_, _ = conn.ExecContext(ctx, "USE `"+resetDB+"`")
		}
		// Always restore `main` before returning the connection to the pool, so a later ws.DB()
		// read — which does NOT check out a branch — doesn't inherit this reader's branch. Without
		// this, a --changeset read poisons the pool: e.g. `comment ls --subject <ref>` resolves the
		// subject on the changeset branch, then reads rev_comment on that reused connection and
		// misses the comments (which live on main), reporting "no comments".
		_ = versioncontrolops.CheckoutBranch(ctx, conn, "main")
		return conn.Close()
	}
	return conn, release, nil
}

// selectReadRef points conn at ref for reading. A live branch is checked out normally; a commit
// (Dolt has no detached HEAD) is read via its read-only revision database (`USE db/ref`). It
// returns the plain database name when a revision db was selected — so the caller can reset the
// pooled connection on release — and "" for a normal branch checkout.
func selectReadRef(ctx context.Context, conn *sql.Conn, ref string) (resetDB string, err error) {
	if e := versioncontrolops.CheckoutBranch(ctx, conn, ref); e == nil {
		return "", nil
	} else if !validRef(ref) {
		return "", e // not a live branch and not a safe ref to interpolate — surface the checkout error
	}
	var current string
	if e := conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&current); e != nil {
		return "", fmt.Errorf("no such branch or commit %q", ref)
	}
	db := baseDB(current)
	if db == "" {
		return "", fmt.Errorf("no such branch or commit %q", ref)
	}
	if _, e := conn.ExecContext(ctx, "USE `"+db+"/"+ref+"`"); e != nil {
		return "", fmt.Errorf("no such branch or commit %q", ref)
	}
	return db, nil
}

// baseDB strips any `/revision` suffix from a current-database name, leaving the plain db.
func baseDB(current string) string {
	return strings.SplitN(current, "/", 2)[0]
}

// validRef reports whether ref is safe to interpolate into a `USE db/ref` statement — the
// character set of Dolt branch names and commit hashes (alphanumerics plus / - _). Guards the
// revision-database path against injection (dolt_diff/USE reject bind vars).
func validRef(ref string) bool {
	if ref == "" {
		return false
	}
	for _, r := range ref {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '/' || r == '-' || r == '_'
		if !ok {
			return false
		}
	}
	return true
}
