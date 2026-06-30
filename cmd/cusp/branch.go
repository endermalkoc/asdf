package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
)

var branchCmd = &cobra.Command{
	Use:   "branch",
	Short: "Inspect and hand-manage raw Dolt branches (low-level; prefer `cusp changeset`)",
	Long: "Cusp's tracked, PR-like workflow is `cusp changeset` — branches named changeset/<slug>,\n" +
		"recorded in rev_changeset and made the active target. `branch` is the low-level escape\n" +
		"hatch over the raw Dolt branch graph: list branches, or create/delete/checkout one by hand.\n" +
		"A branch created here is untracked (no rev_changeset row), so `changeset submit`/`merge`\n" +
		"won't apply to it — use it for diagnostics and manual surgery, not the everyday flow.",
}

var branchLsCmd = &cobra.Command{
	Use:   "ls",
	Short: "List Dolt branches (* marks the active target; changeset branches are tagged)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		bl, err := app.Branches(ctx, ws)
		if err != nil {
			return err
		}
		if flagJSON {
			emit(bl, "")
			return nil
		}
		for _, b := range bl.Branches {
			marker := "  "
			if b == bl.Active {
				marker = "* "
			}
			tag := ""
			if strings.HasPrefix(b, "changeset/") {
				tag = "  (changeset)"
			}
			fmt.Printf("%s%s%s\n", marker, b, tag)
		}
		return nil
	},
}

var branchCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a branch off the active target",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if err := app.CreateBranch(ctx, ws, args[0]); err != nil {
			return err
		}
		emit(map[string]string{"created": args[0]}, "created branch "+args[0])
		return nil
	},
}

var branchDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a branch (refuses main and the active target)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		name := args[0]
		if name == "main" {
			return fmt.Errorf("refusing to delete the canonical branch %q", name)
		}
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if name == app.ResolveBranch(ws, "") {
			return fmt.Errorf("refusing to delete the active target branch %q — switch away first (`cusp branch checkout main`)", name)
		}
		if err := app.DeleteBranch(ctx, ws, name); err != nil {
			return err
		}
		emit(map[string]string{"deleted": name}, "deleted branch "+name)
		return nil
	},
}

var branchCheckoutCmd = &cobra.Command{
	Use:   "checkout <name>",
	Short: "Set a branch as the active read/write target (main clears the pointer)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		ws, err := connect(ctx)
		if err != nil {
			return err
		}
		defer ws.Close()
		if err := app.Checkout(ctx, ws, args[0]); err != nil {
			return err
		}
		emit(map[string]string{"active": args[0]}, "active target branch is now "+args[0])
		return nil
	},
}

func init() {
	branchCmd.AddCommand(branchLsCmd, branchCreateCmd, branchDeleteCmd, branchCheckoutCmd)
	rootCmd.AddCommand(branchCmd)
}
