package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
)

var flattenForce bool

var flattenCmd = &cobra.Command{
	Use:   "flatten",
	Short: "Squash ALL Dolt history into a single commit (irreversible)",
	Long: "Nuclear option: squash the entire Dolt commit history on the current branch into one\n" +
		"commit, then GC to reclaim space. All commit-level history (time travel) is lost; the\n" +
		"resulting database has exactly one commit holding the current data. Use it when the Dolt\n" +
		"store has grown large and you don't need history. For a softer squash that keeps recent\n" +
		"commits, use `cusp dolt compact`.\n\n" +
		"  cusp flatten --dry-run    # preview the commit count\n" +
		"  cusp flatten --force      # actually squash",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()

		count, initial, err := app.FlattenPreview(ctx, ws)
		if err != nil {
			return err
		}
		if count <= 1 {
			emit(map[string]any{"commit_count": count, "message": "already flat"},
				"Already flat (1 commit). Nothing to do.")
			return nil
		}
		if flagDryRun {
			emit(map[string]any{"dry_run": true, "commit_count": count, "initial_hash": initial, "would_flatten": true},
				fmt.Sprintf("DRY RUN — flatten preview\n  commits:        %d\n  initial commit: %s\n  would squash %d commits into 1; run with --force to proceed.",
					count, initial, count))
			return nil
		}
		if !flattenForce {
			return fmt.Errorf("would squash %d commits into 1 (irreversible) — use --force to confirm or --dry-run to preview", count)
		}
		if err := app.Flatten(ctx, ws); err != nil {
			return err
		}
		emit(map[string]any{"commits_before": count, "commits_after": 1},
			fmt.Sprintf("✓ flattened %d commits → 1", count))
		return nil
	},
}

func init() {
	flattenCmd.Flags().BoolVarP(&flattenForce, "force", "f", false, "confirm the irreversible history squash")
	rootCmd.AddCommand(flattenCmd)
}
