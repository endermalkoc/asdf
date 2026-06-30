package main

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/enums"
	"github.com/endermalkoc/cusp/internal/ids"
	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/workspace"
)

var changesetCmd = &cobra.Command{
	Use:     "changeset",
	Aliases: []string{"cs"},
	Short:   "Manage changesets — PR-like branches that bundle edits across entities",
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}

// currentHead returns the HEAD commit hash on the pinned connection's branch.
func currentHead(ctx context.Context, conn *sql.Conn) (string, error) {
	var h string
	err := conn.QueryRowContext(ctx, "SELECT commit_hash FROM dolt_log ORDER BY date DESC LIMIT 1").Scan(&h)
	return h, err
}

// resolveChangeset returns the changeset branch from an arg, else the active one.
func resolveChangeset(ws *workspace.Workspace, args []string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}
	if b := ws.ActiveChangeset(); b != "" {
		return b, nil
	}
	return "", fmt.Errorf("no changeset specified and none active (use `cusp changeset start` or pass a name)")
}

var changesetStartCmd = &cobra.Command{
	Use:   "start <title>",
	Short: "Open a changeset (a Dolt branch) and make it the active target for edits",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		conn, err := ws.Pin(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()

		title := args[0]
		branch := "changeset/" + slug(title)
		if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
			return err
		}
		base, err := currentHead(ctx, conn)
		if err != nil {
			return err
		}
		if err := versioncontrolops.CreateBranch(ctx, conn, branch); err != nil {
			return fmt.Errorf("creating branch %q: %w", branch, err)
		}

		actor := workspace.ResolveActor(flagActor)
		authorID, err := store.SeedActor(ctx, conn, actor.Handle, actor.Name)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		id := ids.New()
		if _, err := conn.ExecContext(ctx,
			"INSERT INTO `rev_changeset` (id,title,description,author_id,status,branch,base_commit,created_at,updated_at) VALUES (?,?,?,?,?,?,?,?,?)",
			id, title, "", authorID, enums.ChangesetDraft, branch, base, now, now); err != nil {
			return fmt.Errorf("recording changeset: %w", err)
		}
		if err := versioncontrolops.StageAndCommit(ctx, conn, map[string]bool{"rev_changeset": true},
			"open changeset "+branch, actor.CommitAuthorString()); err != nil {
			return err
		}
		if err := ws.SetActiveChangeset(branch); err != nil {
			return err
		}
		emit(map[string]any{"branch": branch, "status": enums.ChangesetDraft, "base_commit": base},
			fmt.Sprintf("started changeset %s (now active) — edits go here until `cusp changeset submit`/`merge`", branch))
		return nil
	},
}

var changesetDiffCmd = &cobra.Command{
	Use:   "diff [branch]",
	Short: "Show the changeset's combined diff vs its base (PR view)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		branch, err := resolveChangeset(ws, args)
		if err != nil {
			return err
		}
		conn, err := ws.Pin(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		var base string
		if err := conn.QueryRowContext(ctx, "SELECT base_commit FROM `rev_changeset` WHERE branch=?", branch).Scan(&base); err != nil {
			return fmt.Errorf("unknown changeset %q: %w", branch, err)
		}
		diffs, err := workspace.Diff(ctx, conn, base, branch)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(diffs, "")
			return nil
		}
		if len(diffs) == 0 {
			fmt.Printf("%s: no changes vs base\n", branch)
			return nil
		}
		var b strings.Builder
		fmt.Fprintf(&b, "%s vs base:\n", branch)
		for _, d := range diffs {
			fmt.Fprintf(&b, "  %-22s +%d ~%d -%d\n", d.Table, d.Added, d.Modified, d.Deleted)
		}
		fmt.Print(b.String())
		return nil
	},
}

var changesetSubmitCmd = &cobra.Command{
	Use:   "submit [branch]",
	Short: "Mark the changeset open for review (records its head commit)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		branch, err := resolveChangeset(ws, args)
		if err != nil {
			return err
		}
		conn, err := ws.Pin(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		// Read the branch HEAD, then return to main to record metadata.
		if err := versioncontrolops.CheckoutBranch(ctx, conn, branch); err != nil {
			return err
		}
		head, err := currentHead(ctx, conn)
		if err != nil {
			return err
		}
		if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
			return err
		}
		now := time.Now().UTC()
		if _, err := conn.ExecContext(ctx,
			"UPDATE `rev_changeset` SET status=?, head_commit=?, updated_at=? WHERE branch=?",
			enums.ChangesetOpen, head, now, branch); err != nil {
			return err
		}
		actor := workspace.ResolveActor(flagActor)
		if err := versioncontrolops.StageAndCommit(ctx, conn, map[string]bool{"rev_changeset": true},
			"submit changeset "+branch, actor.CommitAuthorString()); err != nil {
			return err
		}
		emit(map[string]any{"branch": branch, "status": enums.ChangesetOpen, "head_commit": head},
			fmt.Sprintf("submitted changeset %s for review (head %s)", branch, head[:min(8, len(head))]))
		return nil
	},
}

var changesetMergeCmd = &cobra.Command{
	Use:   "merge [branch]",
	Short: "Merge the changeset into main",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		branch, err := resolveChangeset(ws, args)
		if err != nil {
			return err
		}
		conn, err := ws.Pin(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		actor := workspace.ResolveActor(flagActor)
		if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
			return err
		}
		conflicts, err := versioncontrolops.Merge(ctx, conn, branch, actor.CommitAuthorString())
		if err != nil {
			return err
		}
		if len(conflicts) > 0 {
			tables := make([]string, 0, len(conflicts))
			for _, c := range conflicts {
				tables = append(tables, c.NodeID)
			}
			return fmt.Errorf("merge of %s has conflicts in: %s (resolve and retry)", branch, strings.Join(tables, ", "))
		}
		mergeCommit, err := currentHead(ctx, conn)
		if err != nil {
			return err
		}
		now := time.Now().UTC()
		if _, err := conn.ExecContext(ctx,
			"UPDATE `rev_changeset` SET status=?, merge_commit=?, updated_at=? WHERE branch=?",
			enums.ChangesetMerged, mergeCommit, now, branch); err != nil {
			return err
		}
		if err := versioncontrolops.StageAndCommit(ctx, conn, map[string]bool{"rev_changeset": true},
			"merge changeset "+branch, actor.CommitAuthorString()); err != nil {
			return err
		}
		if ws.ActiveChangeset() == branch {
			_ = ws.ClearActiveChangeset()
		}
		emit(map[string]any{"branch": branch, "status": enums.ChangesetMerged, "merge_commit": mergeCommit},
			fmt.Sprintf("merged changeset %s into main (%s)", branch, mergeCommit[:min(8, len(mergeCommit))]))
		return nil
	},
}

var changesetAbandonCmd = &cobra.Command{
	Use:   "abandon [branch]",
	Short: "Close a changeset and delete its branch",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		branch, err := resolveChangeset(ws, args)
		if err != nil {
			return err
		}
		conn, err := ws.Pin(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		actor := workspace.ResolveActor(flagActor)
		if err := versioncontrolops.CheckoutBranch(ctx, conn, "main"); err != nil {
			return err
		}
		if _, err := conn.ExecContext(ctx,
			"UPDATE `rev_changeset` SET status=?, updated_at=? WHERE branch=?", enums.ChangesetClosed, time.Now().UTC(), branch); err != nil {
			return err
		}
		if err := versioncontrolops.StageAndCommit(ctx, conn, map[string]bool{"rev_changeset": true},
			"abandon changeset "+branch, actor.CommitAuthorString()); err != nil {
			return err
		}
		if err := versioncontrolops.DeleteBranch(ctx, conn, branch); err != nil {
			return fmt.Errorf("deleting branch %q: %w", branch, err)
		}
		if ws.ActiveChangeset() == branch {
			_ = ws.ClearActiveChangeset()
		}
		emit(map[string]any{"branch": branch, "status": enums.ChangesetClosed},
			fmt.Sprintf("abandoned changeset %s (branch deleted)", branch))
		return nil
	},
}

var changesetLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List changesets",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		rows, err := ws.DB().QueryContext(ctx,
			"SELECT branch,COALESCE(title,''),status FROM `rev_changeset` ORDER BY updated_at DESC")
		if err != nil {
			return err
		}
		defer rows.Close()
		type row struct{ Branch, Title, Status string }
		var out []row
		for rows.Next() {
			var r row
			if err := rows.Scan(&r.Branch, &r.Title, &r.Status); err != nil {
				return err
			}
			out = append(out, r)
		}
		if flagJSON {
			emit(out, "")
			return nil
		}
		active := ws.ActiveChangeset()
		var b strings.Builder
		for _, r := range out {
			marker := " "
			if r.Branch == active {
				marker = "*"
			}
			fmt.Fprintf(&b, "%s %-28s %-9s %s\n", marker, r.Branch, r.Status, r.Title)
		}
		fmt.Print(b.String())
		return nil
	},
}

func init() {
	changesetCmd.AddCommand(changesetStartCmd, changesetDiffCmd, changesetSubmitCmd, changesetMergeCmd, changesetAbandonCmd, changesetLsCmd)
	rootCmd.AddCommand(changesetCmd)
}
