package storage

import (
	"context"
	"time"
)

// CompactionCandidate is a node eligible for content compaction/tiering.
//
// Genericized from beads' issue-specific type during the salvage: IssueID became
// the generic NodeID; the remaining fields were already domain-agnostic.
type CompactionCandidate struct {
	NodeID         string
	ClosedAt       time.Time
	OriginalSize   int
	EstimatedSize  int
	DependentCount int
}

// NodeSnapshot is a pre-compaction copy of a node's mutable content, archived
// before compaction destructively overwrites it.
//
// Genericized from beads' IssueSnapshot: where beads hard-coded issue fields
// (title/description/design/notes/acceptance_criteria), Cusp keeps the archived
// content opaque so each entity type (Spec, Requirement, TestCase, …) decides
// its own snapshot shape.
type NodeSnapshot struct {
	// CompactionLevel is the tier the node was being compacted *to* when this
	// snapshot was taken.
	CompactionLevel int
	// Content is the archived field set, keyed by field name.
	Content map[string]string
}

// CompactionStore provides node content compaction and tiering operations.
type CompactionStore interface {
	CheckEligibility(ctx context.Context, nodeID string, tier int) (bool, string, error)
	ApplyCompaction(ctx context.Context, nodeID string, tier int, originalSize int, compactedSize int, commitHash string) error
	GetTier1Candidates(ctx context.Context) ([]*CompactionCandidate, error)
	GetTier2Candidates(ctx context.Context) ([]*CompactionCandidate, error)

	// SnapshotNode archives a node's current content before a destructive
	// compaction overwrites it. tier is the level being compacted to. Must be
	// called before the overwrite.
	SnapshotNode(ctx context.Context, nodeID string, tier int) error
	// GetCompactionSnapshot returns the most recent archived snapshot for a
	// node, or (nil, nil) when none exists.
	GetCompactionSnapshot(ctx context.Context, nodeID string) (*NodeSnapshot, error)
	// RestoreFromSnapshot restores a node's content from its most recent
	// snapshot and steps its compaction level back down. Returns the applied
	// snapshot, or (nil, nil) when none exists.
	RestoreFromSnapshot(ctx context.Context, nodeID string) (*NodeSnapshot, error)
}
