package workspace

import (
	"os"
	"path/filepath"
	"strings"
)

// The active changeset is the ambient changeset branch mutating commands target
// when no --changeset is given — analogous to git's HEAD. It is recorded in
// `.cusp/active_changeset` and is project-local (shared across worktrees, which
// share one `.cusp`).

func (w *Workspace) activeChangesetPath() string {
	return filepath.Join(w.CuspDir, "active_changeset")
}

// ActiveChangeset returns the ambient active changeset branch, or "" if none.
func (w *Workspace) ActiveChangeset() string {
	b, err := os.ReadFile(w.activeChangesetPath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// SetActiveChangeset records the ambient active changeset branch.
func (w *Workspace) SetActiveChangeset(branch string) error {
	return os.WriteFile(w.activeChangesetPath(), []byte(branch+"\n"), 0o644)
}

// ClearActiveChangeset removes the ambient active changeset (no-op if unset).
func (w *Workspace) ClearActiveChangeset() error {
	if err := os.Remove(w.activeChangesetPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
