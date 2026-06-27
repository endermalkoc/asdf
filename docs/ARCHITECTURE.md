# ADLG Architecture

How ADLG is put together and the principles that constrain it. For the data model see
the [data model](entities/index.md); for the overview see [README.md](../README.md).

> **Status: early implementation.** The Dolt infrastructure (salvaged from
> [beads](https://github.com/steveyegge/beads), MIT), the schema, `adlg init`, the command
> contract, the `domain`/`spec`/`req`/`edge` verbs, and the changeset (PR) flow are built and
> verified against real Dolt. Generation, `check`/`impact`, remote sync, MCP, and import are
> next — see [ROADMAP.md](ROADMAP.md), [Status / next step](#status--next-step), and
> [Repository layout](#repository-layout).

## Principles

1. **The Dolt database is the single source of truth — always.**
2. **Generated artifacts are read-only.** Markdown and HTML are rendered *from* the DB,
   git-ignored, and never edited by humans or agents. Regenerate; don't edit.
3. **Knowledge is version-controlled** with the same model as code — branch, merge, diff,
   blame — via Dolt.
4. **One store, two interfaces.** CLI and MCP are the only write paths; generated Markdown
   is an optional fast *read* path.
5. **Generic core.** No project-, tenant-, or domain-specific assumptions in the core.

## Source of truth & the generation pipeline

The Dolt database holds the canonical graph. Human- and agent-facing views are **derived**:

```
Dolt DB ──(renderers)──▶ generated/   ├── *.md    (agent + human fast reading)
   ▲                                   └── *.html  (human browsing)
   │ writes only via CLI / MCP
```

- Generation runs on demand (`adlg generate`) and/or after mutations.
- The output directory (default `generated/`) is **git-ignored** — see [.gitignore](../.gitignore).
- Markdown/HTML are **never inputs.** Nothing reads them back into the DB. This is what
  lets us treat them as disposable and always-regenerable.

## Versioning model

Dolt makes the knowledge graph branch/merge/diff-able like a git repo:

- Agents work on **branches** and propose changes; humans review and **merge**.
- Primary keys are **ULIDs** (time-ordered, minted client-side) for authored rows, so rows
  created offline on divergent branches never collide on merge, while index locality stays
  reasonable.
- **Pure-relationship rows** (`Edge`, `TestResult`, junctions) are the exception: their PK is
  **derived deterministically from the row's identity**, not a random ULID — so the *same*
  relationship created independently on two branches converges on merge instead of tripping a
  unique-key violation. See [Identifiers & keys](entities/identifiers.md).
- Human-readable **business keys** (`UNIQUE`, e.g. `ATT-FR-012`, `M1`, a View's route) make
  diffs and merges legible.
- One semantic merge concern remains: `Requirement.number` is sequential, so two branches
  adding FRs to the same spec can collide on the *human* number. That's a merge **policy**
  (renumber-on-merge, or per-agent reserved ranges), not a DB-integrity problem. See
  [Identifiers & keys](entities/identifiers.md).

## Storage engine & server modes

Dolt is reached through a salvaged-from-beads infrastructure layer that supports **three
ways of connecting to the same database**, selected by configuration:

- **Embedded** (`ServerModeEmbedded`) — Dolt linked in-process via `dolthub/driver/v2`; no
  separate binary or server. This is the path that delivers the "single static binary, no
  separate runtime" promise and is the intended default.
- **Owned** (`ServerModeOwned`) — ADLG spawns and supervises its own `dolt sql-server`
  subprocess (requires the `dolt` binary on `PATH`), talking to it over the MySQL wire
  protocol. Implemented by [`internal/doltserver`](../internal/doltserver): cross-platform port
  detection, PID/lock files, log rotation, and manifest/corruption recovery.
- **External** (`ServerModeExternal`) — connect to a `dolt sql-server` someone else runs.

**Warm proxy daemon.** Because Dolt cold-start is expensive per invocation, a long-running
proxy ([`internal/storage/dbproxy`](../internal/storage/dbproxy)) keeps the database hot and
proxies CLI commands to it — the mechanism behind a future `*_proxied_server` command path.

These packages are domain-agnostic: the dependency on beads' issue-domain `storage` package
was **severed to a minimal shim** ([`internal/storage/storage.go`](../internal/storage/storage.go))
holding only `RemoteInfo` and an opaque `DoltStorage` handle, preserving the generic-core
invariant. See [NOTICE](../NOTICE) for attribution and the full salvaged-package list.

## Interfaces

- **CLI** — the primary surface; scriptable, with a JSON output mode for agents.
- **MCP server** — the same operations exposed to agents as MCP tools (`adlg serve --mcp`).
- **Read fast-path** — agents may read the generated Markdown directly for speed.
- **Write path** — CLI/MCP only; all mutations land in the DB, which then regenerates views.

Every command — CLI and MCP — implements one uniform workflow (connect → resolve the
changeset/`main` write target → validate → transaction → mint ids → commit with actor+message →
text/JSON output → structured errors). That workflow lives in a single shared mutation wrapper,
not per command. See **[command-contract.md](command-contract.md)**.

## Validation (`check`)

Consistency and traceability rules evaluated over the graph, runnable as a CI gate:

- every active `Requirement` covered by ≥1 `TestCase` (honoring its `delivery_status` and
  any opt-out marker);
- no dangling `Edge`s or orphaned nodes;
- planning integrity (deliverables linked to capabilities/views; milestone references valid);
- spec/prefix uniqueness and FR-numbering policy.

## Impact analysis (`impact`)

Given a node, traverse the `Edge` graph plus structural foreign keys to report what a change
or breakage affects. For a `Requirement`: dependent FRs (`refines` / `depends_on` /
`references`), covering `TestCase`s, the `Deliverable`s and `View`s that realize it, and the
`Milestone` it targets.

## Import / migration

A **generic, source-agnostic importer** with pluggable adapters that map an external source
into the ADLG schema. It is explicitly **not** tutor-tailored.

- First real migration: the `tutor/docs` corpus — markdown specs + the FR-traceability
  registry + the former planning databases + Qase test management — normalized into
  `Domain` / `Spec` / `Requirement` / `Test*` / `Capability` / `Deliverable` / `View` / …
- Adapters are the place where source-specific quirks live, keeping the core clean.

## Data model

See [docs/entities/](entities/index.md) for the authoritative schema (entities, relationships, enums,
identifiers, and resolved decisions). Layers: Structure · Requirements · Testing (Qase-style)
· Planning · Authorization & entities · Interop.

Note that the **entity layer holds authored business-domain documents**, not a
mirror of any database schema — a documented property need not correspond to a stored column.

## Genericity & tutor-isms to extract

The schema was modeled against the `tutor` corpus, so a few things are corpus-shaped and must
be generalized (configurable policy or seed data, never hardcoded in the core):

- The structured authorization model (`Privilege`/`AccessRule`) — **removed** (migration `0012`): a
  tutor-specific, never-consumed CASL paradigm. Access rules stay as entity-doc prose; a generic,
  consumer-driven authorization concept can return later. See [decisions.md](entities/decisions.md).
- FR conventions — prefix rules, decade-block numbering, opt-out markers, tombstones.
- `Domain` values, milestone labels, Qase-specific enums — seed/import data.

## Tech stack

**Go — now locked** by the adoption of beads' Go/Dolt infrastructure (see [Storage engine &
server modes](#storage-engine--server-modes)). Rationale:

- single static binary, trivial cross-platform distribution (the gold standard for a CLI
  that humans and agents install);
- Dolt is written in Go — drive it natively (embed libraries or manage the `dolt` binary),
  no separate runtime;
- [beads](https://github.com/steveyegge/beads) is an architectural sibling (MIT) whose Dolt
  plumbing we salvaged directly — the precedent became the foundation;
- mature CLI tooling and an MCP Go SDK.

(The former TypeScript/Bun alternative is moot now that Go infrastructure has landed.)

Module path: `github.com/endermalkoc/adlg` (Go 1.26.2). `go.mod`/`go.sum` were brought over
from beads and still need a `go mod tidy` to prune dependencies down to the salvaged subset.

## Repository layout

Source layout mirrors beads (the salvage came from there). The Dolt infrastructure plus a
first domain-layer vertical slice (`ids` → `store` → `cmd/adlg`) are in place:

```
cmd/adlg/            CLI (cobra): init · domain/spec/req/edge add|ls · changeset start/diff/submit/merge
internal/
  app/               the mutation wrapper (Mutate) every command routes through (the command contract)
  workspace/         .adlg resolution + managed connect (owned/external/embedded) + actor + active-changeset + retry + diff
  enums/             allowed enum value sets (schema is VARCHAR by design; app validates)
  ids/               PK minting: ULID (authored) + deterministic uuidv5 (relationships)
  store/             ADLG entity store — executor-based (runs on the wrapper's *sql.Tx / a pinned conn)
  doltserver/        owned-mode dolt sql-server supervision (lifecycle, recovery, logs)
  storage/
    storage.go       severance shim: the DoltStorage handle (composed of the contracts below)
    version_control.go versioned.go remote.go sync.go federation.go
    compaction.go history_viewer.go   VC / remote / sync / federation contracts + value types
    schema/          migration runner + migrations/0001_init.{up,down}.sql (the ADLG DDL)
    versioncontrolops/ Dolt branch/commit/merge/clone/gc/flatten/backup over a DBConn
    kvkeys/          config-key prefixes (merge auto-resolution policy)
    dberrors/        Dolt/SQL error classification
    dbproxy/         warm proxy daemon (pidfile, proxy, server, util)
    doltutil/        DSN building, remote listing, connection close
  remotecache/       clone/pull/push cache for Dolt remotes
  doltremote/        remote-URL normalization
  git/               git plumbing (repo root, worktree detection)
  timeparsing/       CLI date parsing (+6h, tomorrow, RFC3339)
  config/ configfile/ debug/ lockfile/ atomicfile/   leaf helpers
```

The version-control tier was lifted **with surgery**: `versioncontrolops` was decoupled
from beads' issue helpers, and the `versioned`/`compaction`/`history_viewer` contracts were
genericized — beads' `*types.Issue` became opaque rows (`map[string]any`) and generic
`Node*` types, so the package carries no entity model. One file was deliberately **not**
lifted: beads' `merge_slot.go` (a distributed merge lock implemented *as* an issue, driving
the full issue store) — it is a domain feature, not Dolt infrastructure.

## Status / next step

Working end-to-end (verified against real Dolt):

- **Schema** — `0001_init` (26 entities + 6 junctions) applies on real Dolt; FK/UNIQUE/
  deterministic-PK enforcement verified.
- **`adlg init`** — creates `.adlg/`, starts a managed (owned) `dolt sql-server`, runs
  `MigrateUp`, seeds an actor, and records the initial Dolt commit.
- **Command contract** — every mutating command routes through `internal/app.Mutate`:
  managed connect → resolve changeset/`main` target → validate → transaction → mint → commit
  as a real Dolt commit with actor + message (working set left clean). Bad input fails before
  any write.
- **Changesets** — `adlg changeset start/diff/submit/merge/abandon/ls` give the PR flow:
  a changeset is a Dolt branch, edits route to the active changeset, `diff` is the combined
  PR view (`dolt_diff_stat`), edits stay isolated from `main` until `merge`; the `changeset`
  metadata rows live on `main`.

Connection follows beads' model — owned (default) and external (`--dsn`) are wired; embedded
is recognized but deferred (cgo/ICU). Next: broaden the store toward the full `DoltStorage`,
graph integrity (edge cycle detection), `check`/`impact`, the generation pipeline, the MCP
server, and the generic import. See [ROADMAP.md](ROADMAP.md) and [docs/command-contract.md](command-contract.md).

The schema is validated end-to-end against real Dolt (32 tables / 43 FKs / 17 UNIQUEs, with
FK/UNIQUE/deterministic-PK enforcement verified), and a first vertical slice is live: the
`adlg` CLI creates Domain/Spec/Requirement/Edge through `internal/store` into a running
`dolt sql-server` (so writes appear immediately in a connected UI like Dolt Workbench). ULID
minting and the deterministic relationship-PK both work (re-adding an edge converges to one row).

Next: broaden the store toward the full `DoltStorage`, **wire writes to Dolt commits** (tying
into the Actor / ChangeProposal review layer), and make number allocation concurrency-safe;
then the generation pipeline, `check`/`impact`, the MCP server, and the generic import.
