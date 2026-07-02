package workspace

import (
	"os"
	"path/filepath"
	"testing"
)

// Config load/save is a pure filesystem round-trip (no Dolt), reachable via both the
// dir-only helpers (LoadConfigDir/SaveConfigDir) and the *Workspace methods.

func TestConfig_LoadMissingIsZero(t *testing.T) {
	dir := t.TempDir()
	c, err := LoadConfigDir(dir)
	if err != nil {
		t.Fatalf("LoadConfigDir on empty dir: %v", err)
	}
	if c == nil {
		t.Fatal("expected non-nil zero Config")
	}
	if c.Generate.Enabled || len(c.Generate.Formats) != 0 {
		t.Fatalf("missing config should be zero, got %+v", *c)
	}
}

func TestConfig_SaveLoadRoundTripDir(t *testing.T) {
	dir := t.TempDir()
	want := &Config{Generate: GenerateConfig{
		Enabled: true,
		Formats: []FormatConfig{
			{Format: "md"},
			{Format: "html", Out: "site/html"},
		},
	}}
	if err := SaveConfigDir(dir, want); err != nil {
		t.Fatalf("SaveConfigDir: %v", err)
	}
	// File exists on disk and ends with a trailing newline.
	b, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	if len(b) == 0 || b[len(b)-1] != '\n' {
		t.Fatalf("expected trailing newline in serialized config")
	}

	got, err := LoadConfigDir(dir)
	if err != nil {
		t.Fatalf("LoadConfigDir: %v", err)
	}
	if !got.Generate.Enabled || len(got.Generate.Formats) != 2 {
		t.Fatalf("round-trip mismatch: got %+v", got.Generate)
	}
	if got.Generate.Formats[1].Format != "html" || got.Generate.Formats[1].Out != "site/html" {
		t.Fatalf("format not preserved: %+v", got.Generate.Formats[1])
	}
}

func TestConfig_WorkspaceMethodsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	w := &Workspace{CuspDir: dir}

	// Missing → zero.
	c, err := w.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if c.Generate.Enabled {
		t.Fatalf("expected disabled generate by default, got %+v", c.Generate)
	}

	c.Generate.Enabled = true
	c.Generate.Formats = []FormatConfig{{Format: "md"}}
	if err := w.SaveConfig(c); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := w.LoadConfig()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !got.Generate.Enabled || len(got.Generate.Formats) != 1 || got.Generate.Formats[0].Format != "md" {
		t.Fatalf("workspace round-trip mismatch: %+v", got.Generate)
	}
}

func TestConfig_LoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadConfigDir(dir); err == nil {
		t.Fatal("expected error for malformed config.json")
	}
}

func TestResolveOutDir(t *testing.T) {
	cuspDir := filepath.Join("proj", ".cusp")

	// Empty Out → default `.cusp/artifacts/<format>`.
	def := resolveOutDir(cuspDir, FormatConfig{Format: "md"})
	if want := filepath.Join(cuspDir, "artifacts", "md"); def != want {
		t.Fatalf("default out: got %q want %q", def, want)
	}
	// Whitespace-only Out is treated as empty.
	if got := resolveOutDir(cuspDir, FormatConfig{Format: "md", Out: "   "}); got != def {
		t.Fatalf("whitespace out should default: got %q want %q", got, def)
	}

	// Absolute Out → returned unchanged.
	abs := filepath.Join(string(filepath.Separator)+"abs", "html")
	if got := resolveOutDir(cuspDir, FormatConfig{Format: "html", Out: abs}); got != abs {
		t.Fatalf("absolute out: got %q want %q", got, abs)
	}

	// Relative Out → resolved against the project root (parent of .cusp).
	rel := resolveOutDir(cuspDir, FormatConfig{Format: "html", Out: "out/html"})
	if want := filepath.Join("proj", "out", "html"); rel != want {
		t.Fatalf("relative out: got %q want %q", rel, want)
	}
}

func TestOutDir_Helpers(t *testing.T) {
	cuspDir := filepath.Join("root", ".cusp")

	if got, want := DefaultOutDir(cuspDir, "md"), filepath.Join(cuspDir, "artifacts", "md"); got != want {
		t.Fatalf("DefaultOutDir: got %q want %q", got, want)
	}
	if got, want := EffectiveOutDir(cuspDir, FormatConfig{Format: "md"}), filepath.Join(cuspDir, "artifacts", "md"); got != want {
		t.Fatalf("EffectiveOutDir default: got %q want %q", got, want)
	}

	w := &Workspace{CuspDir: cuspDir}
	if got, want := w.OutDir(FormatConfig{Format: "html", Out: "gen"}), filepath.Join("root", "gen"); got != want {
		t.Fatalf("Workspace.OutDir relative: got %q want %q", got, want)
	}
	if got, want := w.configPath(), filepath.Join(cuspDir, "config.json"); got != want {
		t.Fatalf("configPath: got %q want %q", got, want)
	}
}
