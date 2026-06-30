// Package schema applies Cusp's numbered SQL migrations to a Dolt database.
//
// The design is lifted from beads' internal/storage/schema (numbered
// migrations/NNNN_*.up.sql files embedded with go:embed, applied in order past a
// schema_migrations cursor table). Only the generic core — beads' inner
// `migrationSource` — is reproduced here; beads' public MigrateUp also runs
// issue-domain orchestration (ID rekeys, status backfills, a second "ignored"
// migration stream, Dolt dirty-table staging guards) that is not part of Cusp.
//
// To add a migration: drop a NNNN_name.up.sql (and .down.sql) into migrations/.
// The next sequential 4-digit version is auto-discovered via go:embed.
package schema

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/endermalkoc/cusp/internal/storage/dberrors"
)

// DBConn is the minimal connection interface the runner needs; satisfied by
// *sql.DB, *sql.Conn, and *sql.Tx.
type DBConn interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

//go:embed migrations/*.up.sql
var upMigrations embed.FS

const (
	migrationsDir = "migrations"
	cursorTable   = "schema_migrations"
)

type migrationFile struct {
	version int
	name    string
}

var (
	latestOnce sync.Once
	latestVer  int
)

// parseVersion reads the leading NNNN_ version number from a migration filename.
func parseVersion(name string) (int, error) {
	parts := strings.SplitN(name, "_", 2)
	if len(parts) == 0 {
		return 0, fmt.Errorf("no version prefix")
	}
	return strconv.Atoi(parts[0])
}

// list returns the embedded up-migrations sorted ascending by version. It panics
// on a malformed filename or a duplicate version — both are authoring bugs that
// must fail loudly at startup, never silently skip a migration.
func list() []migrationFile {
	entries, err := fs.ReadDir(upMigrations, migrationsDir)
	if err != nil {
		panic(fmt.Sprintf("schema: failed to read embedded %s: %v", migrationsDir, err))
	}
	var files []migrationFile
	seen := map[int]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		v, err := parseVersion(e.Name())
		if err != nil {
			panic(fmt.Sprintf("schema: invalid migration filename %q: %v", e.Name(), err))
		}
		if prev, dup := seen[v]; dup {
			panic(fmt.Sprintf("schema: duplicate migration version %d: %q and %q", v, prev, e.Name()))
		}
		seen[v] = e.Name()
		files = append(files, migrationFile{version: v, name: e.Name()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].version < files[j].version })
	return files
}

// bootstrapSQL creates the migration cursor table.
func bootstrapSQL() string {
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
	version INT PRIMARY KEY,
	applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
	content_hash CHAR(64)
)`, cursorTable)
}

// LatestVersion is the highest embedded migration version (0 if none).
func LatestVersion() int {
	latestOnce.Do(func() {
		files := list()
		if len(files) > 0 {
			latestVer = files[len(files)-1].version
		}
	})
	return latestVer
}

// CurrentVersion is the highest version recorded as applied in the database.
// A not-yet-created cursor table reports 0.
func CurrentVersion(ctx context.Context, db DBConn) (int, error) {
	var current int
	err := db.QueryRowContext(ctx, "SELECT COALESCE(MAX(version), 0) FROM "+cursorTable).Scan(&current)
	if err == nil || err == sql.ErrNoRows {
		return current, nil
	}
	if dberrors.IsTableNotExist(err) {
		return 0, nil
	}
	return 0, fmt.Errorf("reading %s version: %w", cursorTable, err)
}

// PendingVersions lists embedded versions newer than the database's current one.
func PendingVersions(ctx context.Context, db DBConn) ([]int, error) {
	current, err := CurrentVersion(ctx, db)
	if err != nil {
		return nil, err
	}
	pending := make([]int, 0)
	for _, mf := range list() {
		if mf.version > current {
			pending = append(pending, mf.version)
		}
	}
	return pending, nil
}

// MigrateUp applies every pending migration in order. It returns the number
// applied. Safe to call on every startup: a database already at the latest
// version is a no-op.
func MigrateUp(ctx context.Context, db DBConn) (int, error) {
	return MigrateUpTo(ctx, db, 0)
}

// MigrateUpTo applies pending migrations up to and including maxVersion (0 means
// "all"). Each migration's SQL runs, then its version + content hash is recorded
// in the cursor table.
func MigrateUpTo(ctx context.Context, db DBConn, maxVersion int) (int, error) {
	if _, err := db.ExecContext(ctx, bootstrapSQL()); err != nil {
		return 0, fmt.Errorf("creating %s: %w", cursorTable, err)
	}

	target := LatestVersion()
	if maxVersion > 0 && maxVersion < target {
		target = maxVersion
	}

	current, err := CurrentVersion(ctx, db)
	if err != nil {
		return 0, err
	}
	if current >= target {
		return 0, nil
	}

	count := 0
	for _, mf := range list() {
		if mf.version <= current || mf.version > target {
			continue
		}
		data, err := upMigrations.ReadFile(migrationsDir + "/" + mf.name)
		if err != nil {
			return count, fmt.Errorf("reading migration %s: %w", mf.name, err)
		}
		if _, err := db.ExecContext(ctx, string(data)); err != nil {
			return count, fmt.Errorf("migration %s: %w", mf.name, err)
		}
		sum := sha256.Sum256(data)
		if _, err := db.ExecContext(ctx,
			"INSERT IGNORE INTO "+cursorTable+" (version, content_hash) VALUES (?, ?)",
			mf.version, hex.EncodeToString(sum[:]),
		); err != nil {
			return count, fmt.Errorf("recording %s in %s: %w", mf.name, cursorTable, err)
		}
		count++
	}
	return count, nil
}
