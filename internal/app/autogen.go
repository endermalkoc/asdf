package app

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/endermalkoc/cusp/internal/generate"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// Incremental auto-generation. When enabled in the workspace config, a successful mutation on
// `main` re-materializes only the documents it affected, in each configured format. The work
// is driven by the Dolt diff of the commit just made: most edits (a story, scenario, section,
// group, or requirement-statement change) touch a single spec/entity and re-render just that
// document; structural changes (a spec/entity/domain/term/section-type added, removed, or
// renamed) can alter index pages or the inline-reference target universe, so they trigger a
// full rebuild. Either way the renderer writes only files whose content actually changed.

// nonRenderTables never appear in any generated document, so a commit touching only these
// (e.g. an `edge add`, or the resolved-ref index maintained by ingestion) generates nothing.
var nonRenderTables = map[string]bool{
	"req_edge":         true,
	"pub_external_ref": true,
	"req_entity_ref":   true,
	"rev_actor":        true,
}

// localSpecTables hold a spec's content keyed directly by spec_id; a change confined to them
// re-renders only the owning spec's document (no index or cross-reference fan-out).
var localSpecTables = map[string]bool{
	"req_spec_section":      true,
	"req_user_story":        true,
	"req_requirement_group": true,
}

// AutoGenerate runs incremental generation for the commit just recorded on branch. It is a
// best-effort post-commit step: callers treat its error as a warning, since the change is
// already durably committed. It no-ops unless auto-gen is enabled, the target is `main`, and
// the commit actually advanced HEAD (parentHash → a new head).
func AutoGenerate(ctx context.Context, ws *workspace.Workspace, conn *sql.Conn, branch, parentHash string, dirty map[string]bool) error {
	// Auto-gen reflects the canonical `main` view; changeset drafts update their artifacts on
	// merge, not while in flight (the artifacts directory is shared across worktrees).
	if branch != "main" {
		return nil
	}
	cfg, err := ws.LoadConfig()
	if err != nil {
		return err
	}
	targets := configuredTargets(ws, cfg)
	if len(targets) == 0 {
		return nil
	}
	headHash, err := headCommitHash(ctx, conn)
	if err != nil {
		return err
	}
	if headHash == "" || headHash == parentHash {
		return nil // nothing was committed (e.g. all writes were no-ops)
	}
	ds, err := classifyDirty(ctx, conn, parentHash, headHash, dirty)
	if err != nil {
		return err
	}
	if _, err := generate.Sync(ctx, conn, targets, ds); err != nil {
		return err
	}
	return nil
}

// SyncConfiguredFull rebuilds every configured format from `main` in full (orphans removed),
// regardless of the Enabled flag — the explicit `config generate sync` action. Returns nil
// stats when no formats are configured.
func SyncConfiguredFull(ctx context.Context, ws *workspace.Workspace) (*generate.SyncStats, error) {
	cfg, err := ws.LoadConfig()
	if err != nil {
		return nil, err
	}
	var targets []generate.Target
	for _, f := range cfg.Generate.Formats {
		targets = append(targets, generate.Target{Format: f.Format, OutDir: ws.OutDir(f)})
	}
	if len(targets) == 0 {
		return &generate.SyncStats{}, nil
	}
	return generate.Sync(ctx, ws.DB(), targets, generate.DirtySet{Full: true})
}

// configuredTargets turns the enabled generate config into resolved render targets.
func configuredTargets(ws *workspace.Workspace, cfg *workspace.Config) []generate.Target {
	if cfg == nil || !cfg.Generate.Enabled {
		return nil
	}
	var out []generate.Target
	for _, f := range cfg.Generate.Formats {
		out = append(out, generate.Target{Format: f.Format, OutDir: ws.OutDir(f)})
	}
	return out
}

// classifyDirty maps the commit's dirty tables to a generate.DirtySet. A table outside the
// local/non-render classification, or a structural change to a requirement (add/remove/key
// rename), escalates to a full rebuild.
func classifyDirty(ctx context.Context, conn *sql.Conn, from, to string, dirty map[string]bool) (generate.DirtySet, error) {
	specSet := map[string]bool{}
	entSet := map[string]bool{}
	for table := range dirty {
		switch {
		case nonRenderTables[table]:
			continue
		case localSpecTables[table]:
			ids, err := diffOwnerIDs(ctx, conn, from, to, table, "to_spec_id", "from_spec_id")
			if err != nil {
				return generate.DirtySet{}, err
			}
			addAll(specSet, ids)
		case table == "req_acceptance_scenario":
			ids, err := scenarioSpecIDs(ctx, conn, from, to)
			if err != nil {
				return generate.DirtySet{}, err
			}
			addAll(specSet, ids)
		case table == "req_requirement":
			ids, structural, err := requirementDirty(ctx, conn, from, to)
			if err != nil {
				return generate.DirtySet{}, err
			}
			if structural {
				return generate.DirtySet{Full: true}, nil
			}
			addAll(specSet, ids)
		case table == "ent_entity_section":
			ids, err := diffOwnerIDs(ctx, conn, from, to, table, "to_entity_id", "from_entity_id")
			if err != nil {
				return generate.DirtySet{}, err
			}
			addAll(entSet, ids)
		default:
			// Anything else (spec/entity/domain/glossary/section-type rows, lookups, …) can
			// move indexes or the ref-target set — only a full rebuild is provably correct.
			return generate.DirtySet{Full: true}, nil
		}
	}
	return generate.DirtySet{SpecIDs: keys(specSet), EntityIDs: keys(entSet)}, nil
}

// diffOwnerIDs returns the distinct owning ids (to_ or from_, covering inserts/updates/
// deletes) of rows changed in table between from and to. The dolt_diff table function
// rejects bind-var arguments, so the refs are validated and inlined (the table name and
// columns are trusted constants).
func diffOwnerIDs(ctx context.Context, conn *sql.Conn, from, to, table, toCol, fromCol string) ([]string, error) {
	q := fmt.Sprintf("SELECT DISTINCT COALESCE(%s, %s) FROM dolt_diff(%s, %s, '%s')",
		toCol, fromCol, quoteRef(from), quoteRef(to), table)
	return queryStrings(ctx, conn, q)
}

// quoteRef validates a Dolt commit ref (hashes are alphanumeric; HEAD~N etc. add a few
// punctuation chars) and returns it as a SQL string literal. An unexpected ref degrades to
// the empty-string literal, which makes dolt_diff a no-op rather than risking injection.
func quoteRef(ref string) string {
	for _, r := range ref {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
			r == '/' || r == '-' || r == '_' || r == '~' || r == '^'
		if !ok {
			return "''"
		}
	}
	return "'" + ref + "'"
}

// scenarioSpecIDs maps changed acceptance scenarios up to their owning specs via the story
// table. A scenario whose story was also deleted is covered by req_user_story's own diff.
func scenarioSpecIDs(ctx context.Context, conn *sql.Conn, from, to string) ([]string, error) {
	storyIDs, err := diffOwnerIDs(ctx, conn, from, to, "req_acceptance_scenario", "to_user_story_id", "from_user_story_id")
	if err != nil {
		return nil, err
	}
	if len(storyIDs) == 0 {
		return nil, nil
	}
	ph := make([]string, len(storyIDs))
	args := make([]any, len(storyIDs))
	for i, id := range storyIDs {
		ph[i] = "?"
		args[i] = id
	}
	q := "SELECT DISTINCT spec_id FROM `req_user_story` WHERE id IN (" + strings.Join(ph, ",") + ")"
	return queryStrings(ctx, conn, q, args...)
}

// requirementDirty inspects the requirement diff: a pure modification with an unchanged
// fr_key is local to its spec; an insert, delete, or fr_key rename changes the ref-target
// universe and forces a full rebuild.
func requirementDirty(ctx context.Context, conn *sql.Conn, from, to string) (specIDs []string, structural bool, err error) {
	q := fmt.Sprintf("SELECT diff_type, COALESCE(to_fr_key,''), COALESCE(from_fr_key,''), "+
		"COALESCE(to_spec_id, from_spec_id) FROM dolt_diff(%s, %s, 'req_requirement')",
		quoteRef(from), quoteRef(to))
	rows, err := conn.QueryContext(ctx, q)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	seen := map[string]bool{}
	for rows.Next() {
		var diffType, toKey, fromKey, specID string
		if err := rows.Scan(&diffType, &toKey, &fromKey, &specID); err != nil {
			return nil, false, err
		}
		if diffType != "modified" || toKey != fromKey {
			return nil, true, nil
		}
		if specID != "" && !seen[specID] {
			seen[specID] = true
			specIDs = append(specIDs, specID)
		}
	}
	return specIDs, false, rows.Err()
}

// queryStrings runs a single-column string query and collects the non-empty rows.
func queryStrings(ctx context.Context, conn *sql.Conn, q string, args ...any) ([]string, error) {
	rows, err := conn.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		if s != "" {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

// headCommitHash returns the latest commit hash on the connection's current branch.
func headCommitHash(ctx context.Context, conn *sql.Conn) (string, error) {
	var h string
	err := conn.QueryRowContext(ctx, "SELECT commit_hash FROM dolt_log LIMIT 1").Scan(&h)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return h, err
}

func addAll(set map[string]bool, ids []string) {
	for _, id := range ids {
		set[id] = true
	}
}

func keys(set map[string]bool) []string {
	if len(set) == 0 {
		return nil
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	return out
}
