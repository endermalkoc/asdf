# ADLG Roadmap

What's next. A living document — pairs with [CHANGELOG.md](../CHANGELOG.md) (what's already built),
[ARCHITECTURE.md](ARCHITECTURE.md) (how it's put together), [docs/entities/](entities/index.md) (the
data model), and [docs/command-contract.md](command-contract.md) (the workflow every command follows).

> Completed work lives in [CHANGELOG.md](../CHANGELOG.md). This file tracks what's left.

## Finish the command contract

- **Broaden CRUD — remaining entities.** CRUD for the entities not yet modeled as CRUD: Milestone,
  Test*, Capability, Deliverable, View, ExternalRef. (The core surface —
  req/spec/domain/entity/glossary-term + `edge ls`/`delete` + `section delete` — is done; see
  [CHANGELOG.md](../CHANGELOG.md).)
- **`adlg config get/set`** — a general typed `config get/set` over the lifted
  `internal/config`/`configfile` (Dolt server settings). (The workspace `generate` config —
  `config show` + `config generate enable/disable/add/remove/sync` — is done.)
- **Per-kind edge type policy** — which endpoint *types* a given edge kind may link. Left generic for
  now; graph integrity (endpoint resolution, self-loop/cycle rejection) is done.
- **Changeset-preview render** — make `generate` branch-aware if a changeset-preview render is wanted
  (today it reads `main`, the canonical build).

## Core features

| Feature | What | Status / notes |
|---|---|---|
| **Remote sync — remaining** | `adlg dolt clone`, federation (peers) | push/pull/remote/fetch + `adlg sync` are **done** (see [CHANGELOG.md](../CHANGELOG.md)). **Remaining:** `adlg dolt clone` (bootstrap a workspace from a remote — distinct flow, no existing `.adlg`) and federation/peers. |
| **Batch add** | `adlg <entity> add --file <f>` and/or `adlg batch <f>` — bulk-create entities from a **JSON/CSV** file in ADLG's own shape, in **one changeset/commit** | adapt beads' `bd create --file`/`--graph`; rides the `Mutate` wrapper so the whole batch is one transaction + one Dolt commit. |
| **Generic import** | `adlg import --format json\|csv <f>` — ingest **arbitrary external** JSON/CSV and map columns/fields into the schema via a mapping spec | **TODO.** The staging core (`internal/importer`: `Graph`/`Report`/idempotent `Apply`) exists from the tutor work; still needed is the external-shape → field-mapping front end (distinct from batch add). Routes through the contract. |
| **Source adapters — remaining** | `adlg import <source>` — pluggable per-source adapters on the staging core | **`tutor` done** (see [CHANGELOG.md](../CHANGELOG.md) — read-only report + `--apply`, Domain→Entity). **Remaining:** `Test*` needs a Qase export (absent in the corpus); `EntityAttribute`/`EntityRelationship` need a non-prose source or an enrichment pass (they live in entity-doc prose, not a structured form). Future adapters reuse `importer.Apply`. |
| **Export** | `adlg export` — JSONL snapshot (git-friendly, diffable) | beads' model; useful for backups/interop alongside Dolt history. |
| **Open Knowledge Format (OKF)** | interop with Google's [OKF](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md): each concept is one `.md` with a YAML **frontmatter** block (`---`) + markdown body; concept id = file path minus `.md` (`tables/users.md` → `tables/users`); reserved `index.md` (grouped listings / progressive disclosure) + `log.md` (newest-first change history); frontmatter is **required `type`** + recommended `title`/`description`/`resource`/`tags`/`timestamp` (unknown fields preserved); cross-references are **plain markdown links** — bundle-absolute (`/tables/customers.md`) or relative (`./other.md`) — treated as **untyped directed edges** (relationship type lives in prose) | **TODO (requested).** ADLG already aligns closely: generated docs carry YAML frontmatter + H1 + body, we emit `index.md` sitemap pages, and our Dolt history is essentially `log.md` (`adlg log`). **Two options, decision deferred:** (a) an **OKF adapter** — import OKF bundles through the staging core + export an OKF bundle as a `generate` format (`--format okf`, with a generated `log.md` from `dolt_log`); or (b) **make the default Markdown output OKF-shaped** — frontmatter keyed to OKF (`type` ← ADLG layer, plus `title`/`description`/`tags`/`timestamp`) and cross-references rendered as **standard markdown links** (`/path.md`) instead of the current Obsidian wikilinks + `^block` anchors. **Gaps to bridge either way:** wikilink/block-ref → plain markdown link; map ADLG layers (spec/entity/requirement/term) → OKF `type`; add `log.md`; reconcile our `index.md` tree with OKF's listing convention. |
| **Validation & analysis — remaining** | richer traceability (orphan-FR/coverage rollups), `impact` over `ent_relationship`, `adlg doctor` (health + auto-fix), drift detection | **`adlg check`** (inline-ref + cycle integrity) and **`adlg impact <ref>`** (graph traversal, `--transitive`) first slices are **done** (see [CHANGELOG.md](../CHANGELOG.md)). **Remaining:** richer traceability (orphan-FR/coverage rollups), `impact` over `ent_relationship` too, `doctor`/`drift` (adapt beads patterns; we have `schema.CheckForwardDrift`-style hooks). |
| **Query / inspect — optional extensions** | a higher-level `query` DSL, a standalone `adlg diff <from> <to>` | `adlg sql`/`stats`/`search`/`log` are **done** (see [CHANGELOG.md](../CHANGELOG.md)). These extensions are optional, not blocking. |
| **Agent integration** | `adlg setup` → install into **Claude Code**, Codex, Cursor, Gemini, Aider, opencode; the **MCP server** (`adlg serve --mcp`) | **requested ("initialize into Claude Code").** Mirror beads' `cmd/bd/setup` (same agent targets). MCP is in the README roadmap. |
| **DB maintenance** | `adlg backup`/`restore`, `gc` (Dolt GC), `compact`/`flatten` (history compaction) | Infra lifted in `versioncontrolops` (gc/compact/flatten); wire the CLI. |
| **Self-update** | **`adlg upgrade`** — download the latest release binary, verify its checksum, and replace in place; **`adlg version`** reports the build | **requested.** Mirror beads' `cmd/bd/upgrade.go`; pairs with the GoReleaser/GitHub-Releases distribution in the README. |
| **CLI polish** | **help system** (rich help + examples, `help-all` overview), shell completion | **requested (help).** Cobra gives base help/completion; add per-command examples and a top-level overview. |

## "What am I missing vs beads?" — feature survey

Cross-cutting beads features (not issue-domain), and ADLG's status:

| beads | ADLG status |
|---|---|
| `dolt push/pull/remote`, `sync`, `federation` | push/pull/remote/fetch + `sync` **DONE**; `clone` + federation remaining |
| `export` (JSONL) | **roadmap (Export)** |
| `import` | **roadmap (Generic import + Source adapters: tutor)** |
| `batch` (bulk create) | **roadmap (Batch add — JSON/CSV)** |
| `sql` (raw passthrough) | **DONE** (read-only) — Query/inspect |
| `search` | **DONE** — Query/inspect |
| `backup`/`restore` | **roadmap (DB maintenance)** |
| `doctor`, `drift`, `preflight` | **roadmap (Validation & analysis)** |
| `gc`, `compact`, `flatten` | infra lifted → **roadmap (DB maintenance)** |
| `setup` (agent install) | **roadmap (Agent integration)** |
| MCP server | **roadmap (Agent integration)** |
| `config` (get/set) | **Finish the command contract** |
| `version`/`upgrade` (self-update) | **roadmap (Self-update)** |
| shell completion | **roadmap (CLI polish)** — cobra-provided |
| `hooks` (`on_create`, …) | **deferred** — `internal/hooks` not lifted (needs a node concept) |
| `worktree` commands | partial — `git.GetMainRepoRoot` is worktree-aware; explicit cmds maybe |
| `metrics`/telemetry | likely **skip / opt-in** |
| `stats`, `count`, `info`, `history`, `where` | `stats` + `history` (`log`) **DONE**; rest **roadmap (Query/inspect)** |

**Generate (Markdown/JSON/HTML), cross-references, and the glossary are ADLG-originals beads has none
of** — beads is an issue tracker that snapshots to JSONL; ADLG is a spec/knowledge store whose
human/agent views are generated from the canonical DB, woven together by inline entity links and a
shared glossary.

## Testing & CI

Today only `internal/ids` is unit-tested; everything else was verified **manually** against real
Dolt (`init` → `add` → commit → changeset round-trip). Codify that:

- **Unit tests** (fast, no DB) — pure logic: `ids` (✅), `enums` + `app` validation, `store` SQL +
  field mapping (`fr_key` derivation, `nullIfEmpty`), and `workspace` helpers (actor resolution,
  changeset slug, retry/serialization classification, `dolt_diff_stat` parsing).
- **Integration tests** (real Dolt, **server mode — no cgo**) — a harness that starts a managed
  `dolt sql-server` (or `testcontainers-go/modules/dolt`), applies the schema, and exercises the
  command contract end-to-end: `adlg init`; each `add` produces one Dolt commit with a **clean
  working set**; validation rejects bad input; and the full **changeset round-trip** (`start → add
  on branch → diff → submit → merge`, with `Changeset`/`Review` rows staying on `main`). Reference:
  beads' `internal/testutil/integration`.
- **Embedded-driver e2e** — the in-process `dolthub/driver/v2` test was reverted (needs cgo +
  `libicu-dev`); reintroduce behind a build tag (e.g. `-tags dolt_e2e`) once CI guarantees ICU.
- **CI** — run `go build ./...` · `go vet ./...` · `go test ./...` on every push; gate the
  integration suite on `dolt` availability (PATH or testcontainers).

## Deliberately NOT carried from beads

- **Issue-tracker verbs** — `create/close/reopen/ready/blocked/dep/assign/priority/label/epic/todo/
  defer/ack/acquire/release`. ADLG has its own entity verbs (`domain/spec/req/edge`, + planned
  Test/Milestone/Capability/…).
- **Agent-orchestration machinery** — `swarm/wisp/mol/gate/bond/pour/cook/prime/kv-memories`. Beads-specific.
- **Divergences (by design):** ULID + deterministic ids (not content-hash); **Dolt-native history +
  the Changeset/Review layer** (not an `events` audit table); the **changeset PR model** (not
  auto-commit-per-op); no `is_blocked` denormalization (compute via `impact`).

## Deferred work & open decisions

- **Merge coordination** (was beads `merge_slot.go`) — not lifted (a lock implemented *as* an
  issue). When needed, build a generic Dolt-native lock (dedicated table or reserved branch), not a
  lift.
- **Embedded mode needs cgo + ICU** (`libicu-dev`) to build. Owned/external (pure-Go) is the
  default; embedded is recognized but deferred. Revisit shipping embedded-by-default vs.
  requiring the `dolt` binary.
- **Lifted utilities now consumed:** `internal/git` (workspace), `internal/storage/dberrors`
  (schema runner). Still orphaned: `internal/timeparsing` (pull in when a command takes dates).
- **`fr_key`** is an app-maintained column, not a SQL generated column (cross-table generation
  isn't possible in Dolt) — keep the store deriving it on write.
- **Concurrency:** same-branch number allocation is safe (`FOR UPDATE` + retry); cross-branch
  FR-number convergence is the documented merge-renumber policy (identifiers.md).
- **`go mod tidy`** upkeep as imports change (currently 12 direct deps — `goldmark` added for the HTML renderer).
