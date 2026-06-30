package app

import (
	"context"
	"fmt"
	"os"

	"github.com/endermalkoc/cusp/internal/storage"
	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// Remote sync. Cusp's store is a Dolt database, so syncing a version-controlled knowledge
// graph is just `dolt push`/`pull` over the canonical branch. These wrappers pin a single
// connection (branch state is connection-scoped; a pull's merge needs one session) and route
// auth (user + DOLT_REMOTE_PASSWORD in the server env) through to the lifted versioncontrolops
// primitives. After a pull that advanced the branch, generated artifacts are refreshed if
// auto-generation is enabled, so the read views track the pulled state.

// RemoteList returns the configured Dolt remotes.
func RemoteList(ctx context.Context, ws *workspace.Workspace) ([]storage.RemoteInfo, error) {
	return versioncontrolops.ListRemotes(ctx, ws.DB())
}

// RemoteAdd configures a named remote.
func RemoteAdd(ctx context.Context, ws *workspace.Workspace, name, url string) error {
	return versioncontrolops.AddRemote(ctx, ws.DB(), name, url)
}

// RemoteRemove drops a configured remote.
func RemoteRemove(ctx context.Context, ws *workspace.Workspace, name string) error {
	return versioncontrolops.RemoveRemote(ctx, ws.DB(), name)
}

// Fetch downloads a remote's refs without merging.
func Fetch(ctx context.Context, ws *workspace.Workspace, remote, user string) error {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	return versioncontrolops.Fetch(ctx, conn, remote, user)
}

// Push uploads branch to remote (force-push when force is set).
func Push(ctx context.Context, ws *workspace.Workspace, remote, branch, user string, force bool) error {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if force {
		return versioncontrolops.ForcePush(ctx, conn, remote, branch, user)
	}
	return versioncontrolops.Push(ctx, conn, remote, branch, user)
}

// Pull fetches branch from remote and merges it into the local branch, then refreshes the
// generated artifacts (if auto-gen is configured) so the read views match the pulled state.
func Pull(ctx context.Context, ws *workspace.Workspace, remote, branch, user string) error {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := versioncontrolops.CheckoutBranch(ctx, conn, branch); err != nil {
		return fmt.Errorf("selecting branch %q: %w", branch, err)
	}
	if err := versioncontrolops.Pull(ctx, conn, remote, branch, user); err != nil {
		return err
	}
	regenAfterSync(ctx, ws)
	return nil
}

// Sync is the round trip: pull remote/branch into the local branch, then push the merged
// result back — the everyday "share my changes and get others'" command. Runs on one pinned
// connection so the merge and push see the same branch state.
func Sync(ctx context.Context, ws *workspace.Workspace, remote, branch, user string) error {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := versioncontrolops.CheckoutBranch(ctx, conn, branch); err != nil {
		return fmt.Errorf("selecting branch %q: %w", branch, err)
	}
	if err := versioncontrolops.Pull(ctx, conn, remote, branch, user); err != nil {
		return fmt.Errorf("pull: %w", err)
	}
	if err := versioncontrolops.Push(ctx, conn, remote, branch, user); err != nil {
		return fmt.Errorf("push: %w", err)
	}
	regenAfterSync(ctx, ws)
	return nil
}

// regenAfterSync best-effort refreshes the configured artifacts after a pull/sync advanced the
// branch (a merge bypasses the app.Mutate auto-gen hook). No-op unless auto-gen is enabled.
func regenAfterSync(ctx context.Context, ws *workspace.Workspace) {
	cfg, err := ws.LoadConfig()
	if err != nil || cfg == nil || !cfg.Generate.Enabled {
		return
	}
	if _, err := SyncConfiguredFull(ctx, ws); err != nil {
		fmt.Fprintln(os.Stderr, "warning: regenerate after pull failed:", err)
	}
}
