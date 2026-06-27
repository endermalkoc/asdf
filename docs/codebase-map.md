# Codebase map — orientation for a fresh session

Where the code lives, where the reference material is, and where to start for each roadmap
item. Read alongside [ARCHITECTURE.md](ARCHITECTURE.md) (how it works) and
[ROADMAP.md](ROADMAP.md) (what to build). The rules every command follows are in
[command-contract.md](command-contract.md).

## Start here (reading order for a new session)

1. [CLAUDE.md](../CLAUDE.md) — invariants, build/run, the hub of links.
2. [ROADMAP.md](ROADMAP.md) — what to pick up next ([CHANGELOG.md](../CHANGELOG.md) records what's already done).
3. [ARCHITECTURE.md](ARCHITECTURE.md) — storage modes, the versioning/changeset model, generation.
4. [command-contract.md](command-contract.md) — the workflow every mutating command MUST follow.
5. [docs/entities/](entities/index.md) — the authoritative data model.
6. This map — where things live + the beads equivalent for each area.

## Reference clones (NOT in this repo — local, read-only)

- **beads** — `/home/ender/repos/misc/beads` (`../../misc/beads` from this repo). The MIT project
  ADLG's Dolt infrastructure was **salvaged and adapted** from; [NOTICE](../NOTICE) lists exactly
  which packages were lifted. When ROADMAP says "adapt beads' X" or "beads has Y", that's where it
  lives. Treat as a read-only reference — ADLG's `internal/` copies have rewritten import paths and
  the issue domain severed; do **not** re-sync from beads.
- **tutor corpus** — `../tutor/docs/specs` and `../tutor/docs/analysis`. The first real import
  target and the source the data model was modeled against (genericize tutor-isms, don't bake them
  in — see [CLAUDE.md](../CLAUDE.md)).

## Where things live

| Path | What it is |
|---|---|
| `cmd/adlg/` | the CLI (cobra). `main.go` → `root.go` (flags, `connect`, `emit`); one file per group: `init`, `domain`, `spec`, `req`, `edge`, `changeset` |
| `internal/app/` | **`Mutate`** — the command-contract wrapper **every** mutating command routes through; `validate.go` |
| `internal/workspace/` | `.adlg` resolution, managed connect (owned/external/embedded), `ResolveActor`, active-changeset state, `WithRetryTx`, `Diff` |
| `internal/store/` | entity CRUD as `Execer`-based free functions (Domain/Spec/Requirement/Edge so far) |
| `internal/ids/` | PK minting — `New()` (ULID) and `Rel()` (deterministic uuidv5) |
| `internal/enums/` | allowed enum value sets (the schema is VARCHAR by design; the app validates) |
| `internal/storage/schema/` | migration runner + `migrations/0001_init.{up,down}.sql` (the DDL) |
| `internal/storage/versioncontrolops/` | Dolt branch/commit/merge/diff over a `DBConn` — the changeset engine |
| `internal/doltserver/` | manages the owned `dolt sql-server` (lifecycle, ports, recovery) |
| `internal/configfile/`, `internal/config/` | config + DSN/server resolution |
| `internal/storage/{dbproxy,doltutil,kvkeys,dberrors}`, `remotecache`, `doltremote`, `git`, `timeparsing`, `lockfile`, `atomicfile`, `debug` | salvaged infra (see [NOTICE](../NOTICE)) |
| `docs/entities/` | the authoritative data model (one file per layer + index/identifiers/enums/decisions) |

## Picking up a ROADMAP item — where to look

| Roadmap item | Start in ADLG | beads reference |
|---|---|---|
| Remote sync (`adlg dolt push/pull`) | `versioncontrolops/remotes.go`, `remotecache` | `cmd/bd/dolt.go`, `federation.go` |
| Generate (Markdown/HTML) | net-new; read via `store` / `versioncontrolops` | *(none — beads exports JSONL, not docs)* |
| Batch add (`--file` JSON/CSV) | extend the `add` commands; loop store funcs inside one `app.Mutate` | `cmd/bd/create.go` (`--file`/`--graph`), `cmd/bd/batch.go` |
| Generic import (JSON/CSV) | net-new `internal/import` core (field-mapping → entities, via the contract) | `cmd/bd/import.go` (pattern) |
| Source adapters (first: tutor) | net-new adapters on the generic core | `cmd/bd/import.go`; tutor source at `../tutor/docs` |
| `check` / `impact` | traverse `edge` + FKs; ARCHITECTURE §Validation/§Impact | beads `check`/`validate`, `internal/tracker` |
| Agent setup (Claude Code, …) | net-new `cmd/adlg` + a `setup` pkg | `cmd/bd/setup/{claude,codex,cursor,…}.go` |
| MCP server | net-new (Go MCP SDK) | `integrations/beads-mcp` (Python — design ref only) |
| New entity verb (`test`, `milestone`, …) | add to `internal/store`, route through `app.Mutate`, satisfy the contract | `cmd/bd/<verb>.go` |
| DB maintenance (gc/compact/backup) | `versioncontrolops` (gc/compact/flatten lifted) | `cmd/bd/{gc,compact,backup}.go` |

## How to add a command (the short version)

1. New file in `cmd/adlg/` with a cobra command.
2. Read path: `connect(ctx)` → `store.List…(ctx, ws.DB())` → `emit`.
3. Write path: `connect(ctx)` → `app.Mutate(ctx, ws, MutateOpts{Summary, Changeset: flagChangeset,
   Actor: flagActor, Validate}, func(ctx, w){ store.Add…(ctx, w.Tx, …); w.MarkDirty("table") })`.
   The wrapper owns connect/target/tx/commit/attribution — don't re-implement them. See
   [command-contract.md](command-contract.md).

## Build / run / test

See [CLAUDE.md](../CLAUDE.md). TL;DR: `go build ./...` · `go vet ./...` · `go test ./...`; needs
the `dolt` binary on PATH; `adlg init` bootstraps a workspace, then the verbs work against the
managed server.
