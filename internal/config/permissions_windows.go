//go:build windows

package config

import (
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

// CheckCuspDirPermissions is a no-op on Windows where filesystem
// permissions use ACLs rather than Unix permission bits.
func CheckCuspDirPermissions(path string) {}

// FixCuspDirPermissions is a no-op on Windows where filesystem
// permissions use ACLs rather than Unix permission bits.
func FixCuspDirPermissions(path string) (bool, error) { return false, nil }
