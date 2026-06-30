// Package storage provides shared version-control value types.
package storage

import "time"

// These value types were salvaged from beads. Where beads embedded its concrete
// *types.Issue (the node state at a commit / either side of a diff), Cusp keeps
// the state opaque as a column→value row (map[string]any) so the package stays
// free of any entity model — the concrete store decodes a row into whichever
// entity the NodeID refers to.

// HistoryEntry represents a node's state at a specific point in history.
type HistoryEntry struct {
	CommitHash string         // The commit hash at this point
	Committer  string         // Who made the commit
	CommitDate time.Time      // When the commit was made
	Node       map[string]any // The node state at that commit (column → value)
}

// DiffEntry represents a change between two commits.
type DiffEntry struct {
	NodeID   string         // The ID of the affected node
	DiffType string         // "added", "modified", or "removed"
	OldValue map[string]any // State before (nil for "added")
	NewValue map[string]any // State after (nil for "removed")
}

// Conflict represents a merge conflict.
type Conflict struct {
	NodeID      string      // The ID of the conflicting node
	Field       string      // Which field has the conflict (empty for table-level)
	OursValue   interface{} // Value on current branch
	TheirsValue interface{} // Value on merged branch
}

// RemoteInfo describes a configured remote.
type RemoteInfo struct {
	Name string `json:"name"` // Remote name (e.g., "origin")
	URL  string `json:"url"`  // Remote URL (e.g., "dolthub://org/repo")
}

// SyncStatus describes the synchronization state with a peer.
type SyncStatus struct {
	Peer         string    // Peer name
	LastSync     time.Time // When last synced
	LocalAhead   int       // Commits ahead of peer
	LocalBehind  int       // Commits behind peer
	HasConflicts bool      // Whether there are unresolved conflicts
}

// FederationPeer represents a remote peer with authentication credentials.
// Used for peer-to-peer Dolt remotes between workspaces with SQL user auth.
type FederationPeer struct {
	Name        string     // Unique name for this peer (used as remote name)
	RemoteURL   string     // Dolt remote URL (e.g., http://host:port/org/db)
	Username    string     // SQL username for authentication
	Password    string     // Password (decrypted, not stored directly)
	Sovereignty string     // Sovereignty tier: T1, T2, T3, T4
	LastSync    *time.Time // Last successful sync time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
