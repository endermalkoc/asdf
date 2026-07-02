package app

import (
	"context"

	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// PrimeState is the live workspace snapshot `cusp prime` injects into an agent's
// context at session start: the open work (active + open changesets), review
// signals (unresolved comments), integrity (check findings), and headline counts.
// Gathered best-effort — a partial snapshot is better than a failed session hook.
type PrimeState struct {
	ActiveChangeset    string           `json:"activeChangeset,omitempty"`
	OpenChangesets     []PrimeChangeset `json:"openChangesets,omitempty"`
	UnresolvedComments int              `json:"unresolvedComments"`
	CheckFindings      int              `json:"checkFindings"`
	Stats              []PrimeStat      `json:"stats,omitempty"`
}

// PrimeChangeset is one in-flight changeset (draft | open | changes_requested).
type PrimeChangeset struct {
	Branch string `json:"branch"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status"`
	Active bool   `json:"active,omitempty"`
}

// PrimeStat is one entity-layer count.
type PrimeStat struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// GatherPrime collects the workspace state for `cusp prime`. Changeset/comment
// rows always live on `main` (read via ws.DB()); stats and integrity run on the
// resolved read branch (the active changeset, so they describe the current work).
// Each piece is independent and best-effort: a failure in one leaves the rest.
func GatherPrime(ctx context.Context, ws *workspace.Workspace) *PrimeState {
	st := &PrimeState{ActiveChangeset: ws.ActiveChangeset()}
	db := ws.DB()

	// Open (in-flight) changesets — on main.
	if rows, err := db.QueryContext(ctx,
		"SELECT branch, COALESCE(title,''), status FROM `rev_changeset` "+
			"WHERE status IN ('draft','open','changes_requested') ORDER BY updated_at DESC"); err == nil {
		for rows.Next() {
			var c PrimeChangeset
			if err := rows.Scan(&c.Branch, &c.Title, &c.Status); err != nil {
				break
			}
			c.Active = c.Branch == st.ActiveChangeset
			st.OpenChangesets = append(st.OpenChangesets, c)
		}
		rows.Close()
	}

	// Unresolved review comments across all changesets — on main.
	_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `rev_comment` WHERE resolved = FALSE").Scan(&st.UnresolvedComments)

	// Stats + integrity on the resolved read branch (current work).
	if r, release, err := Reader(ctx, ws, ""); err == nil {
		st.Stats = gatherPrimeStats(ctx, r)
		if findings, e := Check(ctx, r); e == nil {
			st.CheckFindings = len(findings)
		}
		_ = release()
	}
	return st
}

// gatherPrimeStats returns a compact headline count per layer.
func gatherPrimeStats(ctx context.Context, r store.Execer) []PrimeStat {
	const q = "SELECT 'domains' AS kind, COUNT(*) AS n FROM `req_domain` " +
		"UNION ALL SELECT 'specs', COUNT(*) FROM `req_spec` " +
		"UNION ALL SELECT 'requirements', COUNT(*) FROM `req_requirement` " +
		"UNION ALL SELECT 'entities', COUNT(*) FROM `ent_entity` " +
		"UNION ALL SELECT 'test_cases', COUNT(*) FROM `test_case`"
	rows, err := r.QueryContext(ctx, q)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []PrimeStat
	for rows.Next() {
		var s PrimeStat
		if err := rows.Scan(&s.Kind, &s.Count); err != nil {
			return out
		}
		out = append(out, s)
	}
	return out
}
