//go:build !windows

package config

import (
	"fmt"
	"io/fs"
	"os"
)

const (
	// ADLGDirPerm is the permission mode for .adlg/ directories (owner-only).
	ADLGDirPerm fs.FileMode = 0700
	// ADLGFilePerm is the permission mode for state files inside .adlg/ (owner-only).
	ADLGFilePerm fs.FileMode = 0600
)

// EnsureADLGDir creates the .adlg directory with secure permissions.
func EnsureADLGDir(path string) error {
	return os.MkdirAll(path, ADLGDirPerm)
}

// CheckADLGDirPermissions warns to stderr if the .adlg directory has
// group or world-accessible permissions. The check is non-fatal.
func CheckADLGDirPermissions(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return // directory doesn't exist yet
	}
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		fmt.Fprintf(os.Stderr, "Warning: %s has permissions %04o (recommended: 0700). Run: chmod 700 %s\n", path, perm, path)
	}
}

// FixADLGDirPermissions sets the .adlg directory to ADLGDirPerm when it
// has group or world-accessible bits. Returns true if permissions changed.
func FixADLGDirPermissions(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, nil // directory doesn't exist yet
	}
	perm := info.Mode().Perm()
	if perm&0077 == 0 {
		return false, nil // no group or world-accessible bits
	}
	if err := os.Chmod(path, ADLGDirPerm); err != nil {
		return false, fmt.Errorf("failed to chmod %s to %04o: %w", path, ADLGDirPerm, err)
	}
	return true, nil
}
