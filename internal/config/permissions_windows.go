//go:build windows

package config

import (
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

// CheckADLGDirPermissions is a no-op on Windows where filesystem
// permissions use ACLs rather than Unix permission bits.
func CheckADLGDirPermissions(path string) {}

// FixADLGDirPermissions is a no-op on Windows where filesystem
// permissions use ACLs rather than Unix permission bits.
func FixADLGDirPermissions(path string) (bool, error) { return false, nil }
