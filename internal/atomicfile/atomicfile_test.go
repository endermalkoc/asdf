package atomicfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteFileContentAndPerm(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	if err := WriteFile(path, []byte("hello"), 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "hello" {
		t.Fatalf("content = %q, want %q", got, "hello")
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := fi.Mode().Perm(); perm != 0o640 {
		t.Fatalf("perm = %o, want %o", perm, 0o640)
	}
}

// TestCloseIdempotent guards the single-shot contract: a second Close after a
// successful one is a no-op returning nil, not a spurious error.
func TestCloseIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	w, err := Create(path, 0o644)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write([]byte("data")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("second Close should be a no-op, got: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "data" {
		t.Fatalf("content = %q, want %q", got, "data")
	}
}

// TestAbortAfterCloseIsSafe guards Abort's documented "safe to call after Close"
// contract — the classic `defer w.Abort()` cleanup idiom must not error or touch
// the committed target.
func TestAbortAfterCloseIsSafe(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	w, err := Create(path, 0o644)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write([]byte("committed")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := w.Abort(); err != nil {
		t.Fatalf("Abort after Close should be safe, got: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "committed" {
		t.Fatalf("target damaged after Abort: content = %q, want %q", got, "committed")
	}
}

// TestAbortDiscards verifies Abort removes the temp file without creating the
// target, and is idempotent.
func TestAbortDiscards(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	w, err := Create(path, 0o644)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := w.Write([]byte("discard me")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := w.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if err := w.Abort(); err != nil {
		t.Fatalf("second Abort should be a no-op, got: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("target should not exist after Abort, stat err = %v", err)
	}
	// No leftover temp files in the directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected no leftover files, found %d", len(entries))
	}
}

// TestCloseAfterAbort verifies the reverse order is also a safe no-op.
func TestCloseAfterAbort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "f.txt")
	w, err := Create(path, 0o644)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := w.Abort(); err != nil {
		t.Fatalf("Abort: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close after Abort should be a no-op, got: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("target should not exist, stat err = %v", err)
	}
}
