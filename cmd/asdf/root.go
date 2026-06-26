package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/app"
	"github.com/endermalkoc/asdf/internal/store"
	"github.com/endermalkoc/asdf/internal/workspace"
)

var (
	flagDSN       string // external override; empty = managed (owned) server
	flagJSON      bool
	flagActor     string
	flagChangeset string
	flagForce     bool
	flagStrict    bool
)

var rootCmd = &cobra.Command{
	Use:   "asdf",
	Short: "Version-controlled source of truth for a project's specs, requirements, tests, and plans",
	Long: "ASDF (Agentic Software Development Framework) — a Dolt-backed store for a software\n" +
		"project's domains, specs, requirements, tests, and plans, driven by humans and agents.",
	SilenceUsage: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagDSN, "dsn", os.Getenv("ASDF_DSN"),
		"connect to an external dolt sql-server at this DSN (default: managed owned server; env ASDF_DSN)")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit JSON instead of human-readable text")
	rootCmd.PersistentFlags().StringVar(&flagActor, "actor", "", "actor handle for attribution (default: git user / $USER)")
	rootCmd.PersistentFlags().StringVar(&flagChangeset, "changeset", "", "target this changeset branch (default: the active changeset, else commit to main)")
	rootCmd.PersistentFlags().BoolVar(&flagForce, "force", false, "write even when an inline [[TYPE:key]] cross-reference is unresolved (dangling)")
	rootCmd.PersistentFlags().BoolVar(&flagStrict, "strict", false, "reject (instead of warn) values outside a seed value-set (kind, delivery status, …)")
}

// Execute runs the CLI.
func Execute() {
	if err := rootCmd.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// connect opens the workspace: managed (owned) server by default, or the
// external server at --dsn / $ASDF_DSN when set.
func connect(ctx context.Context) (*workspace.Workspace, error) {
	return workspace.Connect(ctx, flagDSN)
}

// connectRead opens the workspace and returns a read handle pinned to the active
// changeset / --changeset branch (else main), so a read sees edits staged in the
// changeset (command-contract step 2). done releases the read connection and closes
// the workspace. Use this for content reads (`ls`/`show`); reads of rows that always
// live on main (e.g. `changeset ls`) use connect + ws.DB() directly.
func connectRead(ctx context.Context) (r store.Execer, done func(), err error) {
	ws, err := connect(ctx)
	if err != nil {
		return nil, nil, err
	}
	r, release, err := app.Reader(ctx, ws, flagChangeset)
	if err != nil {
		_ = ws.Close()
		return nil, nil, err
	}
	return r, func() { _ = release(); _ = ws.Close() }, nil
}

// emit prints v as JSON when --json is set, otherwise prints the human string.
func emit(v any, human string) {
	if flagJSON {
		b, _ := json.MarshalIndent(v, "", "  ")
		fmt.Println(string(b))
		return
	}
	fmt.Println(human)
}
