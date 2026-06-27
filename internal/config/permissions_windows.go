//go:build windows

package config

import (
	"io/fs"
	"os"
)

const (
	// ASDFDirPerm is the permission mode for .adlg/ directories (owner-only).
	ASDFDirPerm fs.FileMode = 0700
	// ASDFFilePerm is the permission mode for state files inside .adlg/ (owner-only).
	ASDFFilePerm fs.FileMode = 0600
)

// EnsureASDFDir creates the .adlg directory with secure permissions.
func EnsureASDFDir(path string) error {
	return os.MkdirAll(path, ASDFDirPerm)
}

// CheckASDFDirPermissions is a no-op on Windows where filesystem
// permissions use ACLs rather than Unix permission bits.
func CheckASDFDirPermissions(path string) {}

// FixASDFDirPermissions is a no-op on Windows where filesystem
// permissions use ACLs rather than Unix permission bits.
func FixASDFDirPermissions(path string) (bool, error) { return false, nil }
