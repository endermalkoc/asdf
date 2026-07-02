package agentsetup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallSection_CreateAppendUpdateUnchanged(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	// Create: file absent → scaffold + block.
	act, err := InstallSection(path, "hello world")
	if err != nil || act != ActionCreated {
		t.Fatalf("create: act=%q err=%v", act, err)
	}
	got := read(t, path)
	if !strings.Contains(got, scaffoldHeader) || !strings.Contains(got, "hello world") {
		t.Fatalf("create content missing scaffold/body:\n%s", got)
	}
	if !strings.Contains(got, "<!-- BEGIN CUSP INTEGRATION") || !strings.Contains(got, endMarker) {
		t.Fatalf("create missing markers:\n%s", got)
	}

	// Unchanged: same body → no rewrite reported.
	act, err = InstallSection(path, "hello world")
	if err != nil || act != ActionUnchanged {
		t.Fatalf("unchanged: act=%q err=%v", act, err)
	}

	// Update: new body → block replaced in place, exactly one block remains.
	act, err = InstallSection(path, "hello CUSP")
	if err != nil || act != ActionUpdated {
		t.Fatalf("update: act=%q err=%v", act, err)
	}
	got = read(t, path)
	if strings.Contains(got, "hello world") || !strings.Contains(got, "hello CUSP") {
		t.Fatalf("update did not replace body:\n%s", got)
	}
	if n := strings.Count(got, "<!-- BEGIN CUSP INTEGRATION"); n != 1 {
		t.Fatalf("expected exactly 1 block after update, got %d:\n%s", n, got)
	}
}

func TestInstallSection_AppendPreservesUserContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "CLAUDE.md")
	const user = "# My project\n\nSome existing guidance.\n"
	if err := os.WriteFile(path, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	act, err := InstallSection(path, "cusp section")
	if err != nil || act != ActionAppended {
		t.Fatalf("append: act=%q err=%v", act, err)
	}
	got := read(t, path)
	if !strings.HasPrefix(got, user) {
		t.Fatalf("append clobbered user content:\n%s", got)
	}
	if !strings.Contains(got, "cusp section") {
		t.Fatalf("append missing block:\n%s", got)
	}

	// Removing restores the user content (block + its separating blank line gone).
	removed, err := RemoveSection(path)
	if err != nil || !removed {
		t.Fatalf("remove: removed=%v err=%v", removed, err)
	}
	if got = read(t, path); strings.TrimRight(got, "\n") != strings.TrimRight(user, "\n") {
		t.Fatalf("remove did not restore user content:\n%q", got)
	}
}

func TestSectionStatus(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")

	// Absent file → not present.
	if present, _, err := SectionStatus(path, "body"); err != nil || present {
		t.Fatalf("absent: present=%v err=%v", present, err)
	}
	if _, err := InstallSection(path, "body v1"); err != nil {
		t.Fatal(err)
	}
	// Present + current for the same body.
	present, current, err := SectionStatus(path, "body v1")
	if err != nil || !present || !current {
		t.Fatalf("current: present=%v current=%v err=%v", present, current, err)
	}
	// Present but stale for a different body.
	present, current, err = SectionStatus(path, "body v2")
	if err != nil || !present || current {
		t.Fatalf("stale: present=%v current=%v err=%v", present, current, err)
	}
}

func TestInstallSection_RefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.md")
	if err := os.WriteFile(target, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "AGENTS.md")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if _, err := InstallSection(link, "body"); err == nil {
		t.Fatal("expected symlink to be refused")
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
