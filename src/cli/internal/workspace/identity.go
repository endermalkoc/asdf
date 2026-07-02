package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Actor identity is a per-user, workspace-local preference stored in
// `.cusp/identity.json` — git-ignored, so each person's identity stays local and
// isn't shared through the host repo. It seeds ResolveActor between the explicit
// --actor/$CUSP_ACTOR override and git config, so writes are attributed without
// passing --actor every time. Distinct from `config.json` (the shared generate
// config) and `metadata.json` (the Dolt server config).

const identityFileName = "identity.json"

// Identity is the persisted actor identity. Empty fields fall back to git/$USER.
type Identity struct {
	Handle string `json:"handle,omitempty"`
	Name   string `json:"name,omitempty"`
	Email  string `json:"email,omitempty"`
}

// LoadIdentity reads `.cusp/identity.json`; a missing file is the zero Identity.
func LoadIdentity(cuspDir string) (Identity, error) {
	b, err := os.ReadFile(filepath.Join(cuspDir, identityFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return Identity{}, nil
		}
		return Identity{}, err
	}
	var id Identity
	if err := json.Unmarshal(b, &id); err != nil {
		return Identity{}, err
	}
	return id, nil
}

// SaveIdentity writes `.cusp/identity.json`.
func SaveIdentity(cuspDir string, id Identity) error {
	b, err := json.MarshalIndent(id, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cuspDir, identityFileName), append(b, '\n'), 0o644)
}

// currentIdentity resolves the workspace `.cusp` dir from the cwd and loads its
// identity, best-effort: any failure (no workspace, unreadable file) yields the
// zero Identity, so ResolveActor falls through to git/$USER.
func currentIdentity() Identity {
	cuspDir, err := ResolveCuspDir()
	if err != nil {
		return Identity{}
	}
	id, err := LoadIdentity(cuspDir)
	if err != nil {
		return Identity{}
	}
	return id
}
