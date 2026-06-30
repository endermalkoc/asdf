package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Workspace configuration is project-local, stored as `.cusp/config.json` (alongside
// `active_changeset`). It is a local preference — where and in which formats to materialize
// generated artifacts — not version-controlled source of truth, so it lives in a file rather
// than the database. The Dolt server config (`metadata.json`) is separate and unrelated.

const configFileName = "config.json"

// Config is the on-disk workspace config. Only the generate section exists today; new
// sections are added as optional fields so old files keep parsing.
type Config struct {
	Generate GenerateConfig `json:"generate"`
}

// GenerateConfig controls incremental auto-generation: when Enabled, every database mutation
// on `main` re-materializes the affected documents in each configured Format. Off by default
// (opt-in) — an empty config auto-generates nothing.
type GenerateConfig struct {
	Enabled bool           `json:"enabled"`
	Formats []FormatConfig `json:"formats,omitempty"`
}

// FormatConfig is one auto-generated output: a format and the directory it renders into. An
// empty Out means the default, `.cusp/artifacts/<format>`.
type FormatConfig struct {
	Format string `json:"format"`
	Out    string `json:"out,omitempty"`
}

func (w *Workspace) configPath() string { return filepath.Join(w.CuspDir, configFileName) }

// LoadConfig reads `.cusp/config.json`. A missing file is the zero Config (auto-gen off).
func (w *Workspace) LoadConfig() (*Config, error) {
	return loadConfig(w.CuspDir)
}

// SaveConfig writes `.cusp/config.json`.
func (w *Workspace) SaveConfig(c *Config) error { return saveConfig(w.CuspDir, c) }

// loadConfig / saveConfig operate on a bare `.cusp` dir so callers (e.g. the `config`
// command) can read or edit config without standing up a database connection.
func loadConfig(cuspDir string) (*Config, error) {
	b, err := os.ReadFile(filepath.Join(cuspDir, configFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return &Config{}, nil
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

func saveConfig(cuspDir string, c *Config) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cuspDir, configFileName), append(b, '\n'), 0o644)
}

// LoadConfigDir reads the workspace config given just the `.cusp` directory — for commands
// that operate on config files without a live database (see cmd/cusp/config.go).
func LoadConfigDir(cuspDir string) (*Config, error) { return loadConfig(cuspDir) }

// SaveConfigDir writes the workspace config given just the `.cusp` directory.
func SaveConfigDir(cuspDir string, c *Config) error { return saveConfig(cuspDir, c) }

// OutDir resolves a format's output directory: the configured Out if set (relative paths are
// resolved against the project root, the parent of `.cusp`), else the default
// `.cusp/artifacts/<format>`.
func (w *Workspace) OutDir(f FormatConfig) string {
	return resolveOutDir(w.CuspDir, f)
}

func resolveOutDir(cuspDir string, f FormatConfig) string {
	if strings.TrimSpace(f.Out) == "" {
		return filepath.Join(cuspDir, "artifacts", f.Format)
	}
	if filepath.IsAbs(f.Out) {
		return f.Out
	}
	return filepath.Join(filepath.Dir(cuspDir), f.Out)
}

// DefaultOutDir is the out directory a format gets with no override (`.cusp/artifacts/<fmt>`).
func DefaultOutDir(cuspDir, format string) string {
	return resolveOutDir(cuspDir, FormatConfig{Format: format})
}

// EffectiveOutDir resolves a format target's output directory given just the `.cusp` dir
// (the configured override if set, else the default).
func EffectiveOutDir(cuspDir string, f FormatConfig) string { return resolveOutDir(cuspDir, f) }
