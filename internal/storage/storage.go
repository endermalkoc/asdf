// Package storage holds Cusp's storage contracts.
//
// The Dolt version-control / remote / sync / federation interfaces and their
// value types were salvaged from beads (see NOTICE) and are pure infrastructure.
// The issue-domain `Storage` interface that beads composed alongside them was
// NOT lifted — Cusp's own entity store (Spec / Requirement / TestCase / Edge / …,
// per ER.md) will grow here instead, keeping the core generic (CLAUDE.md #4).
//
// `RemoteInfo`, `Conflict`, `SyncStatus`, `FederationPeer`, etc. live in
// versioned.go; the interfaces live in their per-concern files
// (version_control.go, remote.go, sync.go, federation.go, history_viewer.go,
// compaction.go). The Dolt operations that implement them are free functions in
// internal/storage/versioncontrolops over a DBConn.
package storage

// DoltStorage is an opaque handle to a Dolt-backed store, composed of the
// salvaged version-control infrastructure contracts. The salvaged
// remotecache/doltutil packages only pass it through (open and return); they
// never invoke its methods. Cusp's concrete store will implement this and extend
// it with the real entity operations.
type DoltStorage interface {
	VersionControl
	HistoryViewer
	RemoteStore
	SyncStore
	FederationStore
	CompactionStore

	// Close releases the store's resources.
	Close() error
}
