package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// The active changeset is a plain `.cusp/active_changeset` file — no Dolt needed, so a
// Workspace with just CuspDir set exercises the whole read/write/clear cycle.

func TestActiveChangeset_MissingIsEmpty(t *testing.T) {
	w := &Workspace{CuspDir: t.TempDir()}
	if got := w.ActiveChangeset(); got != "" {
		t.Fatalf("expected empty active changeset, got %q", got)
	}
}

func TestActiveChangeset_SetReadTrims(t *testing.T) {
	dir := t.TempDir()
	w := &Workspace{CuspDir: dir}
	if err := w.SetActiveChangeset("cs-feature"); err != nil {
		t.Fatalf("SetActiveChangeset: %v", err)
	}
	if got := w.ActiveChangeset(); got != "cs-feature" {
		t.Fatalf("expected %q, got %q", "cs-feature", got)
	}
	// The on-disk file carries a trailing newline that ActiveChangeset trims off.
	b, err := os.ReadFile(filepath.Join(dir, "active_changeset"))
	if err != nil {
		t.Fatalf("read active_changeset: %v", err)
	}
	if string(b) != "cs-feature\n" {
		t.Fatalf("unexpected on-disk content: %q", string(b))
	}
}

func TestActiveChangeset_Clear(t *testing.T) {
	dir := t.TempDir()
	w := &Workspace{CuspDir: dir}

	// Clearing when unset is a no-op (no error).
	if err := w.ClearActiveChangeset(); err != nil {
		t.Fatalf("clear on missing should be no-op, got %v", err)
	}

	if err := w.SetActiveChangeset("cs-x"); err != nil {
		t.Fatal(err)
	}
	if err := w.ClearActiveChangeset(); err != nil {
		t.Fatalf("ClearActiveChangeset: %v", err)
	}
	if got := w.ActiveChangeset(); got != "" {
		t.Fatalf("expected empty after clear, got %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "active_changeset")); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, stat err=%v", err)
	}
}
