package storage

import "context"

// HistoryViewer provides time-travel queries and diffs over the Dolt history.
//
// Genericized from beads during the salvage: issueID became the generic nodeID,
// and AsOf — which beads typed as *types.Issue — now returns an opaque row
// (column name → value) so it stays domain-agnostic. Cusp's concrete store can
// decode that row into whichever entity the nodeID refers to.
type HistoryViewer interface {
	History(ctx context.Context, nodeID string) ([]*HistoryEntry, error)
	AsOf(ctx context.Context, nodeID string, ref string) (map[string]any, error)
	Diff(ctx context.Context, fromRef, toRef string) ([]*DiffEntry, error)
}
