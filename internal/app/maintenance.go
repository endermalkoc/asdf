package app

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// Dolt history maintenance: flatten (squash all history into one commit) and
// compact (squash commits older than a retention window, keeping recent ones).
// Both operate on the canonical branch's commit graph via the salvaged
// versioncontrolops primitives and reclaim space with a Dolt GC afterward. GC is
// best-effort: the squash already succeeded, so a GC failure is a warning, not a
// command failure.

// FlattenPreview reports the commit count and the root commit hash without
// changing anything — the basis for `flatten --dry-run`.
func FlattenPreview(ctx context.Context, ws *workspace.Workspace) (commitCount int, initialHash string, err error) {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return 0, "", err
	}
	defer conn.Close()
	return versioncontrolops.FlattenDryRun(ctx, conn)
}

// Flatten squashes all Dolt history on the current branch into a single commit,
// then GCs to reclaim the orphaned history. Irreversible.
func Flatten(ctx context.Context, ws *workspace.Workspace) error {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := versioncontrolops.Flatten(ctx, conn); err != nil {
		return err
	}
	if err := versioncontrolops.DoltGC(ctx, conn); err != nil {
		fmt.Fprintln(os.Stderr, "warning: dolt gc after flatten failed:", err)
	}
	return nil
}

// CompactResult summarizes a dolt-history compaction or its preview.
type CompactResult struct {
	TotalCommits  int    `json:"total_commits"`
	OldCommits    int    `json:"old_commits"`    // older than the cutoff (squashed into 1)
	RecentCommits int    `json:"recent_commits"` // within the window (preserved)
	CutoffDays    int    `json:"cutoff_days"`
	CutoffDate    string `json:"cutoff_date"`
	InitialHash   string `json:"initial_hash"`
	BoundaryHash  string `json:"boundary_hash"`
	Compacted     bool   `json:"compacted"` // false for a preview or a no-op
}

// CompactDolt squashes Dolt commits older than `days` into a single base commit,
// cherry-picking the newer commits on top, then GCs. With dryRun (or when there
// is nothing to compact) it computes the breakdown without touching history.
// `now` anchors the retention cutoff (passed in for determinism/testability).
func CompactDolt(ctx context.Context, ws *workspace.Workspace, days int, dryRun bool, now time.Time) (*CompactResult, error) {
	conn, err := ws.Pin(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	entries, err := versioncontrolops.Log(ctx, conn, 0) // all commits, newest first
	if err != nil {
		return nil, fmt.Errorf("read commit log: %w", err)
	}
	total := len(entries)
	res := &CompactResult{TotalCommits: total, CutoffDays: days}
	if total <= 1 {
		return res, nil // nothing to compact
	}

	cutoff := now.AddDate(0, 0, -days)
	res.CutoffDate = cutoff.Format("2006-01-02")
	res.InitialHash = entries[total-1].Hash // oldest commit (the root)

	var recentHashes []string
	for _, e := range entries { // newest first
		if e.Date.Before(cutoff) {
			res.OldCommits++
			if res.BoundaryHash == "" {
				res.BoundaryHash = e.Hash // newest commit older than the cutoff
			}
		} else {
			recentHashes = append(recentHashes, e.Hash)
		}
	}
	// Reverse to chronological (oldest first) for cherry-picking back on top.
	for i, j := 0, len(recentHashes)-1; i < j; i, j = i+1, j-1 {
		recentHashes[i], recentHashes[j] = recentHashes[j], recentHashes[i]
	}
	res.RecentCommits = len(recentHashes)

	if dryRun || res.OldCommits <= 1 {
		return res, nil
	}
	if res.BoundaryHash == "" {
		return nil, fmt.Errorf("could not find the boundary commit for compaction")
	}
	if err := versioncontrolops.Compact(ctx, conn, res.InitialHash, res.BoundaryHash, res.OldCommits, recentHashes); err != nil {
		return nil, fmt.Errorf("compact: %w", err)
	}
	if err := versioncontrolops.DoltGC(ctx, conn); err != nil {
		fmt.Fprintln(os.Stderr, "warning: dolt gc after compact failed:", err)
	}
	res.Compacted = true
	return res, nil
}
