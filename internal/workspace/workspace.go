// Package workspace resolves an ADLG project's `.adlg/` directory and connects to
// its Dolt database. Connection mode follows beads' model (owned / external /
// embedded) via the lifted internal/doltserver + internal/configfile:
//
//   - owned (default): auto-start/adopt a managed `dolt sql-server` (needs `dolt`
//     on PATH) and connect over the MySQL wire.
//   - external: connect to a server the caller runs (via --dsn override or config).
//   - embedded: recognized by doltserver but its in-process driver is not wired
//     here (needs cgo + libicu-dev); owned is used instead.
//
// Because Dolt branch/working-set state is connection-scoped and DOLT_COMMIT
// cannot run inside a *sql.Tx, mutating work pins a single *sql.Conn (see Pin).
package workspace

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/go-sql-driver/mysql"

	"github.com/endermalkoc/adlg/internal/configfile"
	"github.com/endermalkoc/adlg/internal/doltserver"
	"github.com/endermalkoc/adlg/internal/git"
	"github.com/endermalkoc/adlg/internal/storage/doltutil"
)

// Workspace is a connection to an ADLG project's Dolt database.
type Workspace struct {
	ADLGDir string
	db      *sql.DB
}

// ResolveADLGDir returns the `.adlg` directory for the current project — at the
// git repo root (worktree-aware). It does not require `.adlg` to exist.
func ResolveADLGDir() (string, error) {
	root, err := git.GetMainRepoRoot()
	if err != nil {
		return "", fmt.Errorf("locating project root (run inside a git repo): %w", err)
	}
	return filepath.Join(root, ".adlg"), nil
}

// Connect resolves the workspace and returns a connected *Workspace. With an
// empty dsnOverride it ensures the managed (owned) server is running and builds
// the DSN from config; a non-empty dsnOverride (e.g. --dsn) connects to an
// external server directly without managing one.
func Connect(ctx context.Context, dsnOverride string) (*Workspace, error) {
	adlgDir, err := ResolveADLGDir()
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(adlgDir); statErr != nil {
		return nil, fmt.Errorf("no ADLG workspace at %s — run `adlg init` first", adlgDir)
	}

	dsn := dsnOverride
	if dsn == "" {
		dsn, err = managedDSN(adlgDir)
		if err != nil {
			return nil, err
		}
	}
	return open(ctx, adlgDir, dsn)
}

// open dials the DSN and verifies connectivity.
func open(ctx context.Context, adlgDir, dsn string) (*Workspace, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("connecting to dolt: %w", err)
	}
	return &Workspace{ADLGDir: adlgDir, db: db}, nil
}

// managedDSN ensures the owned server is running and builds a DSN for it. The
// live port comes from doltserver (NOT configfile.GetDoltServerPort, which
// defaults to 3307); host/user/db/password come from config (or defaults).
func managedDSN(adlgDir string) (string, error) {
	port, err := doltserver.EnsureRunning(adlgDir)
	if err != nil {
		return "", fmt.Errorf("starting dolt server: %w", err)
	}
	return dsnForPort(adlgDir, port), nil
}

// dsnForPort builds a MySQL DSN for the given live port using config (or defaults).
func dsnForPort(adlgDir string, port int) string {
	cfg, _ := configfile.Load(adlgDir) // (nil,nil) for a minimal workspace → defaults
	d := doltutil.ServerDSN{Port: port}
	if cfg != nil {
		d.Host = cfg.GetDoltServerHost()
		d.User = cfg.GetDoltServerUser()
		d.Database = cfg.GetDoltDatabase()
		d.Password = cfg.GetDoltServerPasswordForPort(port)
	} else {
		d.Host = configfile.DefaultDoltServerHost
		d.User = configfile.DefaultDoltServerUser
		d.Database = configfile.DefaultDoltDatabase
	}
	return d.String()
}

// DB returns the underlying pool — for reads, which don't need branch pinning.
func (w *Workspace) DB() *sql.DB { return w.db }

// Pin returns a single dedicated connection. Branch-scoped work (checkout + the
// SQL transaction + the Dolt commit) must all run on the SAME connection.
func (w *Workspace) Pin(ctx context.Context) (*sql.Conn, error) { return w.db.Conn(ctx) }

// Close releases the connection pool. It does NOT stop the managed server (each
// command adopts the running server; use `adlg dolt stop` to stop it).
func (w *Workspace) Close() error { return w.db.Close() }
