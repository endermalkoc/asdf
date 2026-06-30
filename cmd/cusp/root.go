package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/workspace"
)

var (
	flagDSN       string // external override; empty = managed (owned) server
	flagJSON      bool
	flagActor     string
	flagChangeset string
	flagForce     bool
	flagStrict    bool
	flagDryRun    bool
)

var rootCmd = &cobra.Command{
	Use:   "cusp",
	Short: "Version-controlled source of truth for a project's specs, requirements, tests, and plans",
	Long: "Cusp (Agentic Delivery Lifecycle Graph) — a Dolt-backed store for a software\n" +
		"project's domains, specs, requirements, tests, and plans, driven by humans and agents.",
	SilenceUsage: true,
	// Execute() is the sole error reporter (exit-code mapping + the --json error
	// envelope); silence cobra's own print so errors aren't emitted twice.
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&flagDSN, "dsn", os.Getenv("CUSP_DSN"),
		"connect to an external dolt sql-server at this DSN (default: managed owned server; env CUSP_DSN)")
	rootCmd.PersistentFlags().BoolVar(&flagJSON, "json", false, "emit JSON instead of human-readable text")
	rootCmd.PersistentFlags().StringVar(&flagActor, "actor", "", "actor handle for attribution (default: git user / $USER)")
	rootCmd.PersistentFlags().StringVar(&flagChangeset, "changeset", "", "target this changeset branch (default: the active changeset, else commit to main)")
	rootCmd.PersistentFlags().BoolVar(&flagForce, "force", false, "write even when an inline [[TYPE:key]] cross-reference is unresolved (dangling)")
	rootCmd.PersistentFlags().BoolVar(&flagStrict, "strict", false, "reject (instead of warn) values outside a seed value-set (kind, delivery status, …)")
	rootCmd.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "validate and preview a mutation, then roll back without committing")
	rootCmd.PersistentPostRun = func(cmd *cobra.Command, args []string) {
		if flagDryRun {
			fmt.Fprintln(os.Stderr, "[dry-run] preview only — no changes were committed")
		}
	}
}

// runMutate routes a mutating command through app.Mutate, injecting the global flags
// (--changeset / --actor / --dry-run) so no command repeats them and every command honors
// them by construction. Under --dry-run the body runs and validates, then rolls back —
// nothing is committed.
func runMutate(cmd *cobra.Command, ws *workspace.Workspace, o app.MutateOpts, body func(context.Context, *app.Write) error) error {
	o.Changeset = flagChangeset
	o.Actor = flagActor
	o.DryRun = flagDryRun
	return app.Mutate(cmd.Context(), ws, o, body)
}

// Execute runs the CLI. On failure it maps the error to a documented exit code (see
// docs/command-contract.md) and, under --json, emits a structured error envelope on
// stdout; otherwise a plain "error: …" line on stderr.
func Execute() {
	err := rootCmd.ExecuteContext(context.Background())
	if err == nil {
		return
	}
	code, category := app.ExitGeneric, "error"
	var ce *app.CodedError
	if errors.As(err, &ce) {
		code, category = ce.Code, ce.Category
	}
	if flagJSON {
		b, _ := json.MarshalIndent(map[string]any{
			"error": map[string]any{"code": code, "category": category, "message": err.Error()},
		}, "", "  ")
		fmt.Fprintln(os.Stdout, string(b))
	} else {
		fmt.Fprintln(os.Stderr, "error:", err.Error())
	}
	os.Exit(code)
}

// connect opens the workspace: managed (owned) server by default, or the
// external server at --dsn / $CUSP_DSN when set.
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
