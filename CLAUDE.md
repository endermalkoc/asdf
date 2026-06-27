# CLAUDE.md

Guidance for Claude Code (and other agents) working in this repository.

## What this project is

**ADLG (Agentic Delivery Lifecycle Graph)** — a [Dolt](https://www.dolthub.com/)-backed
CLI + MCP server that is the version-controlled source of truth for a software project's
specs, requirements, tests, plans, and the relationships between them, used by humans and
agents alike. See [README.md](README.md) and [ARCHITECTURE.md](docs/ARCHITECTURE.md).

**Status:** early implementation. The Dolt infrastructure (salvaged from
[beads](https://github.com/steveyegge/beads), MIT), the **schema** (`0001_init`), **`adlg init`**,
the **command contract** (`internal/app.Mutate`), the `domain`/`spec`/`req`/`edge` verbs, and the
**changeset (PR) flow** are built and verified against real Dolt. Still to come: generation,
`check`/`impact`, remote sync, the MCP server, import — see [ROADMAP.md](docs/ROADMAP.md). The
authoritative artifacts remain the [data model](docs/entities/index.md) and
[ARCHITECTURE.md](docs/ARCHITECTURE.md).

> **New session? Start with [docs/codebase-map.md](docs/codebase-map.md)** — the folder map, the
> beads reference location, the reading order, and where to pick up each [ROADMAP](docs/ROADMAP.md) item.

## Non-negotiable invariants

These are the project's load-bearing rules. Do not violate them, and flag any request that
would.

1. **The Dolt database is the single source of truth. Always.**
2. **Markdown and HTML are generated build artifacts.** They are git-ignored and **must
   never be hand-edited or agent-edited.** To change content, change the DB through the
   CLI/MCP and regenerate. If you find yourself editing a generated `.md`/`.html`, stop —
   that edit will be destroyed on the next `generate`.
3. **All reads, writes, and validation go through the CLI or MCP.** Agents may *read*
   generated Markdown for speed, but it is never a write path.
4. **Keep the core generic.** No project-, tenant-, or domain-specific assumptions belong
   in the core. The schema was *derived from* the `tutor` corpus; tutor-shaped behavior is
   to be generalized, not baked in.

> Note: invariant #2 is about a project's *generated knowledge artifacts*. Ordinary source
> files in this repo (code, the docs below) are edited normally.

## The data model is authoritative — keep it in sync

The data model lives in **[docs/entities/](docs/entities/index.md)**, split by layer
(one file per layer + an index, identifiers, enums, decisions). When the model changes,
update it there **first**, and keep its parts consistent — they currently are, so preserve
that:

- the **master `erDiagram`** in [index.md](docs/entities/index.md) and the per-entity
  attribute tables in the layer files must agree;
- the consolidated [enums.md](docs/entities/enums.md) must match the inline enum values;
- Mermaid attribute syntax is `type name [key] ["comment"]` (type first — `bigint id PK`,
  not `id PK`), and enums render as quoted comments;
- record settled choices in [decisions.md](docs/entities/decisions.md) rather than leaving
  them implicit;
- the root [ER.md](docs/ER.md) is just a pointer stub — don't put content there.

## Known tutor-isms to genericize

When implementing, generalize (don't hardcode) these corpus-specific bits — they are seed
data or configurable policy, not core. The **enum policy buckets** (closed / seed / table) in
[docs/entities/decisions.md](docs/entities/decisions.md) are how this is operationalized:

- `Privilege.scope = owned | studio` — **resolved**: `scope`/`action` are **seed** value-sets
  (`studio` is a tenant value), validated leniently, not baked into core.
- FR conventions (prefix rules, decade-block numbering, tombstones) — configurable policy, not
  fixed. (Opt-out markers — `optout_marker`/`optout_reason` — and `Requirement.owner` were modeled
  from the corpus but **dropped**: the corpus carried no data for them and nothing consumed them.)
- The `Domain` value set / `Domain.kind`, `Spec.kind`, milestone labels (`M0`–`M7`, `Future`), and
  Qase `TestCase.*` enums are **seed**; `Requirement.delivery_status` graduated to a **lookup table**.

## Repository layout

- [docs/entities/](docs/entities/index.md) — data model: entities, relationships, identifiers, decisions (root [ER.md](docs/ER.md) is a pointer stub)
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — architecture, generation pipeline, interfaces, import
- [README.md](README.md) — project overview
- [ROADMAP.md](docs/ROADMAP.md) — what's done, what's next, and the beads-feature survey
- [docs/codebase-map.md](docs/codebase-map.md) — **fresh-session orientation**: folder map, the
  beads reference, reading order, and where to pick up each roadmap item
- [docs/command-contract.md](docs/command-contract.md) — the workflow every command follows
- [NOTICE](NOTICE) — attribution for the Dolt infrastructure salvaged from beads
- `internal/` — the salvaged Dolt infrastructure (see
  [ARCHITECTURE.md](docs/ARCHITECTURE.md#repository-layout) for the package map)
- **Reference clones (not in this repo, read-only):** **beads** at `/home/ender/repos/misc/beads`
  (`../../misc/beads`) — the salvage source for `internal/`'s Dolt infra ([NOTICE](NOTICE) lists
  what was lifted); the **tutor corpus** at `../tutor/docs/specs` / `analysis` — the first import
  target and the source the schema was modeled against.

## Tech stack (Go — locked)

Go single static binary + an MCP server. Module `github.com/endermalkoc/adlg`, Go 1.26.2.
Storage is Dolt, reached three ways (embedded / owned / external) — see
[ARCHITECTURE.md](docs/ARCHITECTURE.md#storage-engine--server-modes).

## Working with the salvaged `internal/` code

The `internal/` packages were lifted from beads with import paths rewritten and beads'
issue-domain dependency severed to a minimal shim
([`internal/storage/storage.go`](internal/storage/storage.go)). When extending them:

- **Keep the core generic.** Do not reintroduce a dependency on a domain (issue/spec/etc.)
  model inside `doltserver`, `dbproxy`, `doltutil`, `remotecache`, or `doltremote`. Widen
  `DoltStorage` in the shim instead; that's where ADLG's real store contract grows.
- `go.mod`/`go.sum` came over from beads wholesale — run **`go mod tidy`** to prune to the
  salvaged subset before relying on the dependency list.
## Command contract — every CLI command follows it

**Every command must satisfy [docs/command-contract.md](docs/command-contract.md).** The
cross-cutting workflow (connect → resolve the changeset/`main` write target → validate →
transaction → mint ids → attribute/timestamp → Dolt commit with actor+message → uniform
text/JSON output → structured errors/exit codes) lives in **one shared mutation wrapper**, not
in each command. When adding or reviewing a command, do not re-implement these per command —
route the write through the wrapper and use the contract as the review checklist. The prioritized
list of what's still missing is in [ROADMAP.md](docs/ROADMAP.md) ("Domain-layer gaps"); the changeset model
is a **Resolved decision** in [docs/entities/decisions.md](docs/entities/decisions.md).

## Build / run

- Build: `go build ./...` · vet: `go vet ./...` · test: `go test ./...` (all green).
- CLI: `go run ./cmd/adlg <cmd>` (or `go build -o adlg ./cmd/adlg`). `adlg init` creates `.adlg/`
  and auto-starts a managed `dolt sql-server` (needs the **`dolt` binary on PATH**); `--dsn` /
  `ADLG_DSN` connects to an external server instead.
- Package layout: see [ARCHITECTURE.md](docs/ARCHITECTURE.md#repository-layout).
