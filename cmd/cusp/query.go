package main

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/store"
)

var logLimit int

// readVerbs are the statement kinds `cusp sql` permits. Writes are rejected so the database
// is only ever mutated through the entity commands (which create attributed Dolt commits) —
// invariant #3: all writes go through the CLI/MCP, never a raw passthrough.
var readVerbs = map[string]bool{
	"select": true, "show": true, "describe": true, "desc": true, "explain": true, "with": true,
}

var sqlCmd = &cobra.Command{
	Use:   "sql <query>",
	Short: "Run a read-only SQL query against the database",
	Long: "Execute a read-only SQL query against the canonical Dolt database and print the result.\n" +
		"Reads honor the active changeset (--changeset → active changeset → main). Only SELECT/SHOW/\n" +
		"DESCRIBE/EXPLAIN/WITH are allowed — writes must go through the entity commands so every\n" +
		"change is an attributed Dolt commit. Dolt history/diff/blame are available via the dolt_*\n" +
		"system tables (e.g. `cusp sql \"SELECT * FROM dolt_log LIMIT 5\"`). Use `-` to read from stdin.",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := strings.Join(args, " ")
		if strings.TrimSpace(query) == "-" {
			b, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			query = string(b)
		}
		if err := ensureReadOnly(query); err != nil {
			return err
		}
		ctx := cmd.Context()
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		rows, err := rd.QueryContext(ctx, query)
		if err != nil {
			return err
		}
		defer rows.Close()
		return writeRows(rows)
	},
}

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Counts of every entity layer (honors the active changeset)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		// One UNION ALL keeps the order and runs on the resolved branch.
		const q = "SELECT 'domains' AS kind, COUNT(*) AS count FROM `req_domain` " +
			"UNION ALL SELECT 'specs', COUNT(*) FROM `req_spec` " +
			"UNION ALL SELECT 'requirements', COUNT(*) FROM `req_requirement` " +
			"UNION ALL SELECT 'user_stories', COUNT(*) FROM `req_user_story` " +
			"UNION ALL SELECT 'scenarios', COUNT(*) FROM `req_acceptance_scenario` " +
			"UNION ALL SELECT 'entities', COUNT(*) FROM `ent_entity` " +
			"UNION ALL SELECT 'glossary_terms', COUNT(*) FROM `req_glossary_term` " +
			"UNION ALL SELECT 'edges', COUNT(*) FROM `req_edge` " +
			"UNION ALL SELECT 'entity_refs', COUNT(*) FROM `req_entity_ref` " +
			"UNION ALL SELECT 'milestones', COUNT(*) FROM `plan_milestone`"
		rows, err := rd.QueryContext(ctx, q)
		if err != nil {
			return err
		}
		defer rows.Close()
		return writeRows(rows)
	},
}

var searchCmd = &cobra.Command{
	Use:   "search <text>",
	Short: "Full-text search across domains, specs, requirements, entities, and terms",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		pattern := "%" + strings.Join(args, " ") + "%"
		hits, err := store.Search(ctx, rd, pattern)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(hits, "")
			return nil
		}
		if len(hits) == 0 {
			fmt.Fprintln(os.Stderr, "(no matches)")
			return nil
		}
		tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
		for _, h := range hits {
			loc := h.Path
			if loc == "" {
				loc = "-"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", h.Type, h.Key, truncate(h.Label, 70), loc)
		}
		return tw.Flush()
	},
}

var logCmd = &cobra.Command{
	Use:   "log",
	Short: "Recent database commits (Dolt history on the active branch)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		rd, done, err := connectRead(ctx)
		if err != nil {
			return err
		}
		defer done()
		rows, err := rd.QueryContext(ctx,
			"SELECT SUBSTRING(commit_hash,1,10) AS commit, committer, "+
				"DATE_FORMAT(date,'%Y-%m-%d %H:%i') AS date, message FROM dolt_log LIMIT ?", logLimit)
		if err != nil {
			return err
		}
		defer rows.Close()
		return writeRows(rows)
	},
}

func init() {
	logCmd.Flags().IntVar(&logLimit, "limit", 20, "number of commits to show")
	rootCmd.AddCommand(sqlCmd, statsCmd, searchCmd, logCmd)
}

// ensureReadOnly rejects any statement that is not a read, so `cusp sql` can never bypass the
// attributed write path.
func ensureReadOnly(query string) error {
	verb := strings.ToLower(strings.TrimLeft(query, "( \t\r\n"))
	if i := strings.IndexFunc(verb, func(r rune) bool { return r == ' ' || r == '\t' || r == '\n' || r == '\r' || r == '(' }); i >= 0 {
		verb = verb[:i]
	}
	if !readVerbs[verb] {
		return app.ValidationFailed(fmt.Errorf("cusp sql is read-only (got %q) — use the entity commands to write", verb))
	}
	return nil
}

// writeRows prints a *sql.Rows result as an aligned table, or as a JSON array of objects under
// --json. Columns are read dynamically, so it works for any query (including dolt_* tables).
func writeRows(rows *sql.Rows) error {
	cols, err := rows.Columns()
	if err != nil {
		return err
	}
	vals := make([]sql.NullString, len(cols))
	ptrs := make([]any, len(cols))
	for i := range vals {
		ptrs[i] = &vals[i]
	}

	if flagJSON {
		out := []map[string]any{}
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				return err
			}
			m := make(map[string]any, len(cols))
			for i, c := range cols {
				if vals[i].Valid {
					m[c] = vals[i].String
				} else {
					m[c] = nil
				}
			}
			out = append(out, m)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		emit(out, "")
		return nil
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, strings.Join(cols, "\t"))
	n := 0
	for rows.Next() {
		if err := rows.Scan(ptrs...); err != nil {
			return err
		}
		cells := make([]string, len(cols))
		for i := range vals {
			if vals[i].Valid {
				cells[i] = oneLine(vals[i].String)
			} else {
				cells[i] = "NULL"
			}
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
		n++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	if n == 0 {
		fmt.Fprintln(os.Stderr, "(no rows)")
	}
	return nil
}

// oneLine collapses newlines/tabs so a multi-line cell stays on one table row.
func oneLine(s string) string {
	return strings.NewReplacer("\n", " ", "\t", " ", "\r", " ").Replace(s)
}

// truncate shortens s to n runes, adding an ellipsis when cut.
func truncate(s string, n int) string {
	s = oneLine(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
