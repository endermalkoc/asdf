package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"

	"github.com/endermalkoc/asdf/internal/configfile"
	"github.com/endermalkoc/asdf/internal/doltserver"
	"github.com/endermalkoc/asdf/internal/git"
	"github.com/endermalkoc/asdf/internal/storage/doltutil"
	"github.com/endermalkoc/asdf/internal/storage/schema"
	"github.com/endermalkoc/asdf/internal/store"
	"github.com/endermalkoc/asdf/internal/workspace"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create an ASDF workspace (.asdf/) and its Dolt database in this repo",
	Long: "Initializes an ASDF workspace: creates .asdf/, starts a managed (owned) dolt\n" +
		"sql-server, applies the schema, and records the initial Dolt commit. Requires the\n" +
		"`dolt` binary on PATH.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		// 1. Locate the project root (init a git repo if needed); create .asdf/.
		root, err := git.GetMainRepoRoot()
		if err != nil {
			if _, e := exec.Command("git", "init").CombinedOutput(); e != nil {
				return fmt.Errorf("git init: %w", e)
			}
			if root, err = git.GetMainRepoRoot(); err != nil {
				return err
			}
		}
		asdfDir := filepath.Join(root, ".asdf")
		if _, statErr := os.Stat(configfile.ConfigPath(asdfDir)); statErr == nil {
			if !initForce {
				return fmt.Errorf("ASDF workspace already exists at %s (pass --force to delete and reinitialize)", asdfDir)
			}
			// --force: tear down the existing workspace so we can rebuild from scratch.
			// Stop the managed server (idempotent — ErrServerNotRunning is fine), then
			// remove .asdf entirely. The DB is reproducible (re-import), so this is the
			// fast reset loop while the schema is still churning.
			if err := doltserver.IgnoreNotRunning(doltserver.Stop(asdfDir)); err != nil {
				return fmt.Errorf("stopping existing dolt server: %w", err)
			}
			if err := os.RemoveAll(asdfDir); err != nil {
				return fmt.Errorf("removing existing workspace %s: %w", asdfDir, err)
			}
		}
		if err := os.MkdirAll(asdfDir, 0o700); err != nil {
			return err
		}

		// 2. Start the owned dolt sql-server (no metadata yet → owned mode).
		state, err := doltserver.Start(asdfDir)
		if err != nil {
			return fmt.Errorf("starting dolt server: %w", err)
		}

		// 3. Connect (server only), create + select the database, on one pinned conn.
		dbName := configfile.DefaultDoltDatabase
		serverDSN := doltutil.ServerDSN{
			Host: configfile.DefaultDoltServerHost,
			Port: state.Port,
			User: configfile.DefaultDoltServerUser,
		}.String()
		db, err := sql.Open("mysql", serverDSN)
		if err != nil {
			return err
		}
		defer db.Close()
		conn, err := db.Conn(ctx)
		if err != nil {
			return err
		}
		defer conn.Close()
		if _, err := conn.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+dbName+"`"); err != nil {
			return fmt.Errorf("create database %q: %w", dbName, err)
		}
		if _, err := conn.ExecContext(ctx, "USE `"+dbName+"`"); err != nil {
			return err
		}

		// 4. Apply the schema.
		n, err := schema.MigrateUp(ctx, conn)
		if err != nil {
			return fmt.Errorf("applying schema: %w", err)
		}

		// 5. Seed the current actor (changeset.author_id references actor).
		actor := workspace.ResolveActor(flagActor)
		if _, err := store.SeedActor(ctx, conn, actor.Handle, actor.Name); err != nil {
			return err
		}

		// 6. Initial Dolt commit — stage everything (fresh DB) on the same conn.
		if _, err := conn.ExecContext(ctx, "CALL DOLT_ADD('-A')"); err != nil {
			return fmt.Errorf("staging schema: %w", err)
		}
		if _, err := conn.ExecContext(ctx, "CALL DOLT_COMMIT('-m', ?, '--author', ?)",
			"asdf init", actor.CommitAuthorString()); err != nil {
			return fmt.Errorf("initial commit: %w", err)
		}

		// 7. Persist metadata (DoltMode empty → owned mode) + .gitignore.
		cfg := &configfile.Config{DoltDatabase: dbName}
		if err := cfg.Save(asdfDir); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(asdfDir, ".gitignore"),
			[]byte("dolt-server.pid\ndolt-server.port\ndolt-server.lock\ndolt-server.log\nactive_changeset\n"), 0o644); err != nil {
			return err
		}

		emit(map[string]any{"asdf_dir": asdfDir, "database": dbName, "migrations": n, "port": state.Port},
			fmt.Sprintf("initialized ASDF workspace at %s\n  database: %s · migrations applied: %d · server port: %d",
				asdfDir, dbName, n, state.Port))
		return nil
	},
}

var initForce bool

func init() {
	initCmd.Flags().BoolVar(&initForce, "force", false,
		"if a workspace already exists, stop its server, delete .asdf, and reinitialize from scratch")
	rootCmd.AddCommand(initCmd)
}
