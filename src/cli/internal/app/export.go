package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/endermalkoc/cusp/internal/store"
)

// JSON Lines export of the whole graph — a git-friendly, diffable snapshot for
// backups and interop. One object per row; tables in name order and rows in a
// total order over all their columns, so the same data always produces byte-for-
// byte identical output. Every value is a string or null (types recover from the
// schema). Read-only; runs on whatever branch the caller's reader is pinned to.

// ExportStats summarizes a completed export.
type ExportStats struct {
	Tables int `json:"tables"`
	Rows   int `json:"rows"`
}

// exportSkip lists tables excluded from a data export: the migration cursor is
// internal bookkeeping (recreated by the schema runner), not project data.
var exportSkip = map[string]bool{"schema_migrations": true}

// exportLine is one JSONL record: which table a row came from, and the row.
type exportLine struct {
	Table string         `json:"table"`
	Row   map[string]any `json:"row"`
}

// Export writes the graph as JSON Lines to out and returns the counts.
func Export(ctx context.Context, r store.Execer, out io.Writer) (ExportStats, error) {
	tables, err := listDataTables(ctx, r)
	if err != nil {
		return ExportStats{}, err
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false) // keep URLs/statements literal, not <-escaped
	var stats ExportStats
	for _, t := range tables {
		n, err := exportTable(ctx, r, enc, t)
		if err != nil {
			return stats, fmt.Errorf("export %s: %w", t, err)
		}
		stats.Tables++
		stats.Rows += n
	}
	return stats, nil
}

// listDataTables returns the exportable table names (all real tables minus the
// bookkeeping deny-set), sorted for deterministic output.
func listDataTables(ctx context.Context, r store.Execer) ([]string, error) {
	rows, err := r.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if !exportSkip[name] {
			out = append(out, name)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Strings(out)
	return out, nil
}

// exportTable dumps one table's rows (fully ordered) as JSONL lines.
func exportTable(ctx context.Context, r store.Execer, enc *json.Encoder, table string) (int, error) {
	// Table names come from SHOW TABLES (trusted); an identifier can't be a bind param.
	rows, err := r.QueryContext(ctx, "SELECT * FROM `"+table+"`")
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return 0, err
	}
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	type keyed struct {
		key string
		row map[string]any
	}
	var collected []keyed
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return 0, err
		}
		row := make(map[string]any, len(cols))
		cells := make([]string, len(cols))
		for i, c := range cols {
			if vals[i].Valid {
				row[c] = vals[i].String
				cells[i] = "s" + vals[i].String // tag present vs NULL so ""≠NULL in the sort key
			} else {
				row[c] = nil
				cells[i] = "\x00"
			}
		}
		collected = append(collected, keyed{key: strings.Join(cells, "\x01"), row: row})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	// Total order over every column → deterministic even for composite-key junctions.
	sort.SliceStable(collected, func(i, j int) bool { return collected[i].key < collected[j].key })
	for _, k := range collected {
		if err := enc.Encode(exportLine{Table: table, Row: k.row}); err != nil {
			return 0, err
		}
	}
	return len(collected), nil
}
