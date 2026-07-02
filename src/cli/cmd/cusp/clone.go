package main

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/spf13/cobra"

	"github.com/endermalkoc/cusp/internal/configfile"
	"github.com/endermalkoc/cusp/internal/doltserver"
	"github.com/endermalkoc/cusp/internal/storage/doltutil"
	"github.com/endermalkoc/cusp/internal/storage/versioncontrolops"
)

var cloneForce bool

var doltCloneCmd = &cobra.Command{
	Use:   "clone <remote-url> [dir]",
	Short: "Bootstrap a new workspace by cloning a Dolt remote (no existing .cusp)",
	Long: "Creates a fresh Cusp workspace whose Dolt store is cloned from a remote — the\n" +
		"counterpart to `cusp init` (which starts empty). Unlike `cusp dolt pull`, which\n" +
		"updates an existing workspace, `clone` is the no-`.cusp` bootstrap: it makes .cusp/,\n" +
		"starts the managed server, DOLT_CLONEs the remote into it, and records metadata. The\n" +
		"clone's `origin` remote is set up automatically, so `cusp dolt pull`/`push` work next.\n\n" +
		"  cusp dolt clone file:///srv/cusp-store        # into the current repo\n" +
		"  cusp dolt clone dolthub://org/repo myproject  # into ./myproject\n\n" +
		"Authenticated remotes read credentials from the server environment\n" +
		"(DOLT_REMOTE_PASSWORD); public and file:// remotes need none.",
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		remoteURL := args[0]

		// 1. Resolve the workspace root: the named [dir] (created if needed), else the current
		//    directory. The workspace lives at a git-repo root so metadata.json can be tracked —
		//    if the target is inside a repo we use that repo's root, otherwise we `git init` it.
		//    (This shells out with an explicit dir rather than using the git package, whose
		//    context is cached on first use and wouldn't see a just-created repo.)
		target := "."
		if len(args) == 2 {
			target = args[1]
		}
		root, err := resolveCloneRoot(target)
		if err != nil {
			return err
		}
		cuspDir := filepath.Join(root, ".cusp")
		if _, statErr := os.Stat(configfile.ConfigPath(cuspDir)); statErr == nil {
			if !cloneForce {
				return fmt.Errorf("Cusp workspace already exists at %s (pass --force to delete and re-clone)", cuspDir)
			}
			if err := doltserver.IgnoreNotRunning(doltserver.Stop(cuspDir)); err != nil {
				return fmt.Errorf("stopping existing dolt server: %w", err)
			}
			if err := os.RemoveAll(cuspDir); err != nil {
				return fmt.Errorf("removing existing workspace %s: %w", cuspDir, err)
			}
		}
		if err := os.MkdirAll(cuspDir, 0o700); err != nil {
			return err
		}

		// 2. Start the owned dolt server (empty — no databases yet).
		state, err := doltserver.Start(cuspDir)
		if err != nil {
			return fmt.Errorf("starting dolt server: %w", err)
		}

		// 3. Connect (server only) and clone the remote into the standard database name.
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
		if err := versioncontrolops.DoltClone(ctx, conn, remoteURL, dbName); err != nil {
			// Leave no half-built workspace behind on a failed clone.
			_ = doltserver.IgnoreNotRunning(doltserver.Stop(cuspDir))
			_ = os.RemoveAll(cuspDir)
			return err
		}

		// 4. Sanity-check the clone landed a populated database.
		var tables int
		if _, err := conn.ExecContext(ctx, "USE `"+dbName+"`"); err != nil {
			return fmt.Errorf("selecting cloned database %q: %w", dbName, err)
		}
		if err := conn.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ?", dbName).Scan(&tables); err != nil {
			return fmt.Errorf("inspecting cloned database: %w", err)
		}

		// 5. Persist metadata (owned mode) + .gitignore, same as init.
		cfg := &configfile.Config{DoltDatabase: dbName}
		if err := cfg.Save(cuspDir); err != nil {
			return err
		}
		if err := writeWorkspaceGitignore(cuspDir); err != nil {
			return err
		}

		emit(map[string]any{"cusp_dir": cuspDir, "database": dbName, "tables": tables, "port": state.Port, "remote": versioncontrolops.SanitizeURLForDisplay(remoteURL)},
			fmt.Sprintf("cloned into %s\n  database: %s · tables: %d · server port: %d\n  next: `cusp generate` to materialize docs, `cusp dolt pull` to update",
				cuspDir, dbName, tables, state.Port))
		return nil
	},
}

// resolveCloneRoot resolves (and, for a named dir, creates) the workspace root for a clone.
// If the directory is already inside a git repo, that repo's top level is the root; otherwise
// the directory is initialized as a fresh git repo. Uses `git` with an explicit working dir so
// it sees a repo created in the same command (the git package caches its context on first call).
func resolveCloneRoot(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return "", err
	}
	if top := gitToplevel(abs); top != "" {
		return top, nil
	}
	c := exec.Command("git", "init")
	c.Dir = abs
	if out, err := c.CombinedOutput(); err != nil {
		return "", fmt.Errorf("git init %s: %v: %s", abs, err, strings.TrimSpace(string(out)))
	}
	return abs, nil
}

// gitToplevel returns the git repo root containing dir, or "" if dir is not in a repo.
func gitToplevel(dir string) string {
	c := exec.Command("git", "rev-parse", "--show-toplevel")
	c.Dir = dir
	out, err := c.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func init() {
	doltCloneCmd.Flags().BoolVar(&cloneForce, "force", false,
		"if a workspace already exists here, stop its server, delete .cusp, and re-clone")
	doltCmd.AddCommand(doltCloneCmd)
}
