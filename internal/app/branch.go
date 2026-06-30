package app

import (
	"context"
	"slices"

	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// Raw Dolt branch operations. Cusp's tracked, PR-like workflow is the changeset
// (a branch named changeset/<slug>, recorded in rev_changeset and made the active
// target). These wrappers are the low-level escape hatch beneath that model: list
// the raw branch graph, or hand-create/delete/retarget a branch. Branch state is
// connection-scoped, so create/delete pin a dedicated connection.

// BranchList is the set of Dolt branches plus the branch Cusp reads/writes by
// default (the active changeset, else main — see ResolveBranch).
type BranchList struct {
	Branches []string `json:"branches"`
	Active   string   `json:"active"`
}

// Branches returns all Dolt branches and the active read/write target.
func Branches(ctx context.Context, ws *workspace.Workspace) (BranchList, error) {
	names, err := versioncontrolops.ListBranches(ctx, ws.DB())
	if err != nil {
		return BranchList{}, err
	}
	return BranchList{Branches: names, Active: ResolveBranch(ws, "")}, nil
}

// CreateBranch creates a new Dolt branch off the active target branch.
func CreateBranch(ctx context.Context, ws *workspace.Workspace, name string) error {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	from := ResolveBranch(ws, "")
	if err := versioncontrolops.CheckoutBranch(ctx, conn, from); err != nil {
		return NotFound("branch", from)
	}
	return versioncontrolops.CreateBranch(ctx, conn, name)
}

// DeleteBranch deletes a Dolt branch. It runs from main so the deletion never
// targets the connection's own branch; callers guard against deleting main and
// the active target.
func DeleteBranch(ctx context.Context, ws *workspace.Workspace, name string) error {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
		return err
	}
	return versioncontrolops.DeleteBranch(ctx, conn, name)
}

// Checkout sets name as the active read/write target branch (what ResolveBranch
// returns when no --changeset is given). "main" clears the pointer, since main is
// the default. The branch must already exist.
func Checkout(ctx context.Context, ws *workspace.Workspace, name string) error {
	names, err := versioncontrolops.ListBranches(ctx, ws.DB())
	if err != nil {
		return err
	}
	if !slices.Contains(names, name) {
		return NotFound("branch", name)
	}
	if name == "main" {
		return ws.ClearActiveChangeset()
	}
	return ws.SetActiveChangeset(name)
}
