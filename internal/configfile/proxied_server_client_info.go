package configfile

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const ProxiedServerClientInfoFileName = "proxied_server_client_info.json"

type ProxiedServerClientInfo struct {
	RootPath   string              `json:"root_path,omitempty"`
	ConfigPath string              `json:"config_path,omitempty"`
	LogPath    string              `json:"log_path,omitempty"`
	External   *ExternalDoltConfig `json:"external,omitempty"`
}

func ProxiedServerClientInfoPath(cuspDir string) string {
	return filepath.Join(cuspDir, ProxiedServerClientInfoFileName)
}

func LoadProxiedServerClientInfo(cuspDir string) (*ProxiedServerClientInfo, error) {
	path := ProxiedServerClientInfoPath(cuspDir)
	data, err := os.ReadFile(path) // #nosec G304 - controlled path
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", ProxiedServerClientInfoFileName, err)
	}
	var info ProxiedServerClientInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", ProxiedServerClientInfoFileName, err)
	}
	return &info, nil
}

func SaveProxiedServerClientInfo(cuspDir string, info *ProxiedServerClientInfo) error {
	if info == nil {
		info = &ProxiedServerClientInfo{}
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling %s: %w", ProxiedServerClientInfoFileName, err)
	}
	path := ProxiedServerClientInfoPath(cuspDir)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", ProxiedServerClientInfoFileName, err)
	}
	return nil
}

func resolveSidecarPath(cuspDir, p string) string {
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(cuspDir, p)
}

func (i *ProxiedServerClientInfo) ResolvedRootPath(cuspDir string) string {
	if i == nil {
		return ""
	}
	return resolveSidecarPath(cuspDir, i.RootPath)
}

func (i *ProxiedServerClientInfo) ResolvedConfigPath(cuspDir string) string {
	if i == nil {
		return ""
	}
	return resolveSidecarPath(cuspDir, i.ConfigPath)
}

func (i *ProxiedServerClientInfo) ResolvedLogPath(cuspDir string) string {
	if i == nil {
		return ""
	}
	return resolveSidecarPath(cuspDir, i.LogPath)
}
