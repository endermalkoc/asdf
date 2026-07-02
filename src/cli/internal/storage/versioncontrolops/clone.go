package versioncontrolops

import (
	"context"
	"fmt"
	"net/url"
)

// DoltClone clones a Dolt database from a remote URL.
// conn must be a non-transactional database connection.
// The database parameter specifies the local database name for the clone.
func DoltClone(ctx context.Context, conn DBConn, remoteURL, database string) error {
	if _, err := conn.ExecContext(ctx, "CALL DOLT_CLONE(?, ?)", remoteURL, database); err != nil {
		return fmt.Errorf("dolt clone %s: %w", sanitizeURL(remoteURL), err)
	}
	return nil
}

// SanitizeURLForDisplay strips credentials from a remote URL so it is safe to
// print (in command output, logs, etc.).
func SanitizeURLForDisplay(raw string) string { return sanitizeURL(raw) }

// sanitizeURL removes credentials from a URL for safe error reporting.
func sanitizeURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	parsed.User = nil
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}
