//go:build !windows

package config

import (
	"fmt"
	"io/fs"
	"os"
)

const (
	// CuspDirPerm is the permission mode for .cusp/ directories (owner-only).
	CuspDirPerm fs.FileMode = 0700
	// CuspFilePerm is the permission mode for state files inside .cusp/ (owner-only).
	CuspFilePerm fs.FileMode = 0600
)

// EnsureCuspDir creates the .cusp directory with secure permissions.
func EnsureCuspDir(path string) error {
	return os.MkdirAll(path, CuspDirPerm)
}

// CheckCuspDirPermissions warns to stderr if the .cusp directory has
// group or world-accessible permissions. The check is non-fatal.
func CheckCuspDirPermissions(path string) {
	info, err := os.Stat(path)
	if err != nil {
		return // directory doesn't exist yet
	}
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		fmt.Fprintf(os.Stderr, "Warning: %s has permissions %04o (recommended: 0700). Run: chmod 700 %s\n", path, perm, path)
	}
}

// FixCuspDirPermissions sets the .cusp directory to CuspDirPerm when it
// has group or world-accessible bits. Returns true if permissions changed.
func FixCuspDirPermissions(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, nil // directory doesn't exist yet
	}
	perm := info.Mode().Perm()
	if perm&0077 == 0 {
		return false, nil // no group or world-accessible bits
	}
	if err := os.Chmod(path, CuspDirPerm); err != nil {
		return false, fmt.Errorf("failed to chmod %s to %04o: %w", path, CuspDirPerm, err)
	}
	return true, nil
}
