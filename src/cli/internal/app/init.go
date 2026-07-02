package app

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	_ "github.com/go-sql-driver/mysql"

	"github.com/endermalkoc/cusp/internal/configfile"
	"github.com/endermalkoc/cusp/internal/doltserver"
	"github.com/endermalkoc/cusp/internal/storage/doltutil"
	"github.com/endermalkoc/cusp/internal/storage/schema"
	"github.com/endermalkoc/cusp/internal/store"
	"github.com/endermalkoc/cusp/internal/workspace"
)

// InitResult summarizes a freshly initialized workspace.
type InitResult struct {
	Database   string `json:"database"`
	Migrations int    `json:"migrations"`
	Port       int    `json:"port"`
}

// InitWorkspace builds a Cusp workspace's Dolt database at cuspDir: it creates the directory,
// starts the managed (owned) dolt server, creates + selects the database, applies the schema,
// seeds the actor, records the initial Dolt commit, and writes metadata.json. It does NOT write
// the host `.gitignore` (that governs host-git tracking, a command concern) or handle an existing
// workspace (the caller decides create vs --force). Requires the `dolt` binary on PATH. Shared by
// the `cusp init` command and the test harness so both exercise the same setup path.
func InitWorkspace(ctx context.Context, cuspDir string, actor workspace.Actor) (*InitResult, error) {
	if err := os.MkdirAll(cuspDir, 0o700); err != nil {
		return nil, err
	}

	state, err := doltserver.Start(cuspDir)
	if err != nil {
		return nil, fmt.Errorf("starting dolt server: %w", err)
	}

	dbName := configfile.DefaultDoltDatabase
	serverDSN := doltutil.ServerDSN{
		Host: configfile.DefaultDoltServerHost,
		Port: state.Port,
		User: configfile.DefaultDoltServerUser,
	}.String()
	db, err := sql.Open("mysql", serverDSN)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := conn.ExecContext(ctx, "CREATE DATABASE IF NOT EXISTS `"+dbName+"`"); err != nil {
		return nil, fmt.Errorf("create database %q: %w", dbName, err)
	}
	if _, err := conn.ExecContext(ctx, "USE `"+dbName+"`"); err != nil {
		return nil, err
	}

	n, err := schema.MigrateUp(ctx, conn)
	if err != nil {
		return nil, fmt.Errorf("applying schema: %w", err)
	}

	if _, err := store.SeedActor(ctx, conn, actor.Handle, actor.Name); err != nil {
		return nil, err
	}

	// Initial Dolt commit — stage everything on the fresh DB, on the same conn.
	if _, err := conn.ExecContext(ctx, "CALL DOLT_ADD('-A')"); err != nil {
		return nil, fmt.Errorf("staging schema: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "CALL DOLT_COMMIT('-m', ?, '--author', ?)",
		"cusp init", actor.CommitAuthorString()); err != nil {
		return nil, fmt.Errorf("initial commit: %w", err)
	}

	// Persist metadata (DoltMode empty → owned mode).
	if err := (&configfile.Config{DoltDatabase: dbName}).Save(cuspDir); err != nil {
		return nil, err
	}
	return &InitResult{Database: dbName, Migrations: n, Port: state.Port}, nil
}
