package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/app"
	"github.com/endermalkoc/cusp/internal/configfile"
	"github.com/endermalkoc/cusp/internal/doltserver"
	"github.com/endermalkoc/cusp/internal/git"
	"github.com/endermalkoc/cusp/internal/workspace"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create a Cusp workspace (.cusp/) and its Dolt database in this repo",
	Long: "Initializes a Cusp workspace: creates .cusp/, starts a managed (owned) dolt\n" +
		"sql-server, applies the schema, and records the initial Dolt commit. Requires the\n" +
		"`dolt` binary on PATH.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		// 1. Locate the project root (init a git repo if needed); create .cusp/.
		root, err := git.GetMainRepoRoot()
		if err != nil {
			if _, e := exec.Command("git", "init").CombinedOutput(); e != nil {
				return fmt.Errorf("git init: %w", e)
			}
			if root, err = git.GetMainRepoRoot(); err != nil {
				return err
			}
		}
		cuspDir := filepath.Join(root, ".cusp")
		if _, statErr := os.Stat(configfile.ConfigPath(cuspDir)); statErr == nil {
			if !initForce {
				return fmt.Errorf("Cusp workspace already exists at %s (pass --force to delete and reinitialize)", cuspDir)
			}
			// --force: tear down the existing workspace so we can rebuild from scratch.
			// Stop the managed server (idempotent — ErrServerNotRunning is fine), then
			// remove .cusp entirely. The DB is reproducible (re-import), so this is the
			// fast reset loop while the schema is still churning.
			if err := doltserver.IgnoreNotRunning(doltserver.Stop(cuspDir)); err != nil {
				return fmt.Errorf("stopping existing dolt server: %w", err)
			}
			if err := os.RemoveAll(cuspDir); err != nil {
				return fmt.Errorf("removing existing workspace %s: %w", cuspDir, err)
			}
		}
		// 2. Build the workspace database (server, schema, seed, initial commit, metadata).
		res, err := app.InitWorkspace(ctx, cuspDir, workspace.ResolveActor(flagActor))
		if err != nil {
			return err
		}

		// 3. Write the host `.gitignore` (what the surrounding git repo may track).
		if err := writeWorkspaceGitignore(cuspDir); err != nil {
			return err
		}

		emit(map[string]any{"cusp_dir": cuspDir, "database": res.Database, "migrations": res.Migrations, "port": res.Port},
			fmt.Sprintf("initialized Cusp workspace at %s\n  database: %s · migrations applied: %d · server port: %d",
				cuspDir, res.Database, res.Migrations, res.Port))
		return nil
	},
}

var initForce bool

// writeWorkspaceGitignore writes .cusp/.gitignore, which controls what the HOST repo's git
// tracks: only metadata.json (and this file) are committable. The Dolt store is its own
// versioned repo (sync via `cusp dolt push`/`pull`), the generated Markdown/HTML are
// regenerable build artifacts (invariant #2: never hand-edited, never committed), and the
// server runtime files are machine-local — none belong in host git. Shared by `init` and
// `dolt clone`, which both scaffold a workspace.
func writeWorkspaceGitignore(cuspDir string) error {
	const workspaceGitignore = `# Cusp workspace — the host repo's git tracks only metadata.json (and this file).
# Everything else is regenerable, per-user, or machine-local, and never hand-edited:
#   dolt/          the Dolt store — its own versioned repo ('cusp dolt push'/'pull')
#   generated/     generated Markdown/HTML build artifacts ('cusp generate' output)
#   dolt-server.*  the managed dolt server's runtime files
#   identity.json  per-user actor identity ('cusp config set user.*') — local, not shared
dolt/
dolt.corrupt.*
generated/
dolt-server.pid
dolt-server.port
dolt-server.lock
dolt-server.log
active_changeset
identity.json
`
	return os.WriteFile(filepath.Join(cuspDir, ".gitignore"), []byte(workspaceGitignore), 0o644)
}

func init() {
	initCmd.Flags().BoolVar(&initForce, "force", false,
		"if a workspace already exists, stop its server, delete .cusp, and reinitialize from scratch")
	rootCmd.AddCommand(initCmd)
}
