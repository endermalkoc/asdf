# Cusp Roadmap

What's next. A living document — pairs with [CHANGELOG.md](CHANGELOG.md) (what's already built),
[ARCHITECTURE.md](ARCHITECTURE.md) (how it's put together), [docs/entities/](entities/index.md) (the
data model), and [docs/command-contract.md](command-contract.md) (the workflow every command follows).

> Completed work lives in [CHANGELOG.md](CHANGELOG.md). This file tracks what's left.

## Finish the command contract

- **Broaden CRUD — remaining.** The entity CRUD surface is complete across the core, planning, and
  testing layers plus `edge`/`section`/`ref` (all in [CHANGELOG.md](CHANGELOG.md)). The one leftover —
  per-doc renderers so `deliverable`/`test_case` docs can be diffed in review — is tracked under
  [Review surface → remaining follow-ups](#review-surface--changeset-review-open-decision).
- **Per-kind edge type policy** — which endpoint *types* a given edge kind may link. Left generic for
  now; graph integrity (endpoint resolution, self-loop/cycle rejection) is done.
- **Changeset-preview render** — make `generate` branch-aware if a changeset-preview render is wanted
  (today it reads `main`, the canonical build).

## Core features

| Feature | What | Status / notes |
|---|---|---|
| **Remote sync — remaining** | federation (peers) | push/pull/remote/fetch + `cusp sync` + **`cusp dolt clone`** (bootstrap a workspace from a remote) are **done** (see [CHANGELOG.md](CHANGELOG.md)). **Remaining:** federation/peers. |
| **Batch add** | `cusp <entity> add --file <f>` and/or `cusp batch <f>` — bulk-create entities from a **JSON/CSV** file in Cusp's own shape, in **one changeset/commit** | adapt beads' `bd create --file`/`--graph`; rides the `Mutate` wrapper so the whole batch is one transaction + one Dolt commit. |
| **Generic import** | `cusp import --format json\|csv <f>` — ingest **arbitrary external** JSON/CSV and map columns/fields into the schema via a mapping spec | **TODO.** The staging core (`src/cli/internal/importer`: `Graph`/`Report`/idempotent `Apply`) exists from the tutor work; still needed is the external-shape → field-mapping front end (distinct from batch add). Routes through the contract. |
| **Source adapters — remaining** | `cusp import <source>` — pluggable per-source adapters on the staging core | **`tutor` done** (see [CHANGELOG.md](CHANGELOG.md) — read-only report + `--apply`, Domain→Entity). **Remaining:** `EntityAttribute`/`EntityRelationship` need a non-prose source or an enrichment pass (they live in entity-doc prose, not a structured form); test-management data is covered by the **Qase adapter** below. Future adapters reuse `importer.Apply`. |
| **Qase adapter — follow-ons** | optional extensions to the shipped `cusp import qase` | The adapter is **done** (see [CHANGELOG.md](CHANGELOG.md); verified against a live Qase project). **Optional remaining:** tune the Qase→Cusp integer enum maps (`qase.enumMaps`) against more real data; add a **CSV export** / **JUnit run-report** input path; map Qase milestones. |
| **Export round-trip (restore)** | an import reader for a `cusp export` JSONL snapshot — read the `{table,row}` lines back to rebuild a workspace | `cusp export` (the write half) is **done** (see [CHANGELOG.md](CHANGELOG.md)); the remaining half is the restore path — feed a snapshot back through the staging core / `Mutate` (pairs with **Batch add** and **DB maintenance → backup/restore**). |
| **Open Knowledge Format (OKF)** | interop with Google's [OKF](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md): each concept is one `.md` with a YAML **frontmatter** block (`---`) + markdown body; concept id = file path minus `.md` (`tables/users.md` → `tables/users`); reserved `index.md` (grouped listings / progressive disclosure) + `log.md` (newest-first change history); frontmatter is **required `type`** + recommended `title`/`description`/`resource`/`tags`/`timestamp` (unknown fields preserved); cross-references are **plain markdown links** — bundle-absolute (`/tables/customers.md`) or relative (`./other.md`) — treated as **untyped directed edges** (relationship type lives in prose) | **TODO (requested).** Cusp already aligns closely: generated docs carry YAML frontmatter + H1 + body, we emit `index.md` sitemap pages, and our Dolt history is essentially `log.md` (`cusp log`). **Two options, decision deferred:** (a) an **OKF adapter** — import OKF bundles through the staging core + export an OKF bundle as a `generate` format (`--format okf`, with a generated `log.md` from `dolt_log`); or (b) **make the default Markdown output OKF-shaped** — frontmatter keyed to OKF (`type` ← Cusp layer, plus `title`/`description`/`tags`/`timestamp`) and cross-references rendered as **standard markdown links** (`/path.md`) instead of the current Obsidian wikilinks + `^block` anchors. **Gaps to bridge either way:** wikilink/block-ref → plain markdown link; map Cusp layers (spec/entity/requirement/term) → OKF `type`; add `log.md`; reconcile our `index.md` tree with OKF's listing convention. |
| **Validation & analysis — remaining** | `cusp doctor` (health + auto-fix), drift detection | **`cusp check`** (inline-ref + cycle integrity), **`cusp impact <ref>`** (graph traversal over refs + edges + `ent_relationship`, `--transitive`), and **`cusp coverage`** (requirement→test-case rollups + orphan-FR + delivery-status drift) are **done** (see [CHANGELOG.md](CHANGELOG.md)). **Remaining:** `cusp doctor`/`drift` — a health aggregator (roll up check + coverage + integrity, with auto-fix) and schema/data drift detection (adapt beads patterns; we have `schema.CheckForwardDrift`-style hooks). |
| **Query / inspect — optional extensions** | a higher-level `query` DSL, a standalone `cusp diff <from> <to>` | `cusp sql`/`stats`/`search`/`log` are **done** (see [CHANGELOG.md](CHANGELOG.md)). These extensions are optional, not blocking. |
| **Agent integration — more hosts** | extend `cusp setup` to the remaining hosts: **Gemini** (`.gemini/settings.json` hook), and data-driven rules-file recipes for **Cursor** (`.cursor/rules/cusp.mdc`), Copilot, Windsurf, Aider, opencode | **`cusp prime` + `cusp setup claude` + `cusp setup codex` (skill + instructions + SessionStart hooks) are DONE** (see [CHANGELOG.md](CHANGELOG.md)) — the primary path for shell-capable agents, reusing the single `Mutate` write surface. The marker engine + templates + prime payload are host-agnostic, so new hosts are mostly a recipe: Gemini reuses the JSON SessionStart-hook helper; the rules-file hosts just write the instruction section. |
| **MCP server** (`cusp serve --mcp`) | a thin MCP adapter over `Mutate` + the read/`store` funcs, for shell-less hosts (Claude Desktop) | **secondary** to the CLI-first agent path above (measured at ~1–2k context tokens vs ~10–50k for MCP tool schemas). Not a second source of truth — a surface over the same `Mutate` contract. The review-`store` funcs and `app.GatherPrime`/read helpers are the reusable seam. Also earns its keep as the VS Code extension's transport. |
| **DB maintenance** | `cusp backup`/`restore` | **`cusp flatten` + `cusp dolt compact` (history compaction) + `cusp dolt gc` (standalone Dolt GC) DONE** (see [CHANGELOG.md](CHANGELOG.md)) — flatten/compact GC after squashing; `gc` reclaims chunks without touching history. **Remaining:** `backup`/`restore`. |
| **CLI polish** | **help system** (rich help + examples, `help-all` overview), shell completion | **requested (help).** Cobra gives base help/completion; add per-command examples and a top-level overview. |

## Modules & plugins (separable layers)

**Requested.** Package Cusp's [data-model layers](entities/index.md#layers) as **modules** a workspace
installs independently — `requirements`, `testing`, `planning`, `entity`, `glossary`, `interop` layered
on an always-on **core** (Structure + Review/Changeset + identifiers + the `Mutate` contract + the
generation engine + the Dolt infra). A project that tracks only specs + FRs need not carry the testing or
planning tables, verbs, or generated docs.

The layered model is **already the seam**: each [docs/entities/](entities/index.md) layer is a
self-contained bundle of tables, enums, verbs, renderers, and (where relevant) an import adapter. Modules
formalize that boundary and operationalize the **generic-core invariant** (CLAUDE.md #4) — the core stays
domain-free, every layer becomes opt-in.

**A module owns, end to end:**

- **Schema** — its own migration(s), so the monolithic `0001_init` splits into a core migration + per-module
  migrations applied on install (reuse the `schema` runner / migration tracking).
- **Verbs** — its CLI commands registered into the root only when enabled (`test*` ships with `testing`;
  `capability`/`deliverable`/`view` with `planning`).
- **Generation** — its renderers and `index.md` pages, gated like today's `config generate enable/disable`
  (already a per-layer output toggle — modules generalize it).
- **Seed / enums** — its seed value-sets, per the closed/seed/table [policy](entities/decisions.md).
- **Import adapters** — Qase ships with `testing`; the Notion/`tutor` adapters with `structure`/`requirements`
  (folds the "Source adapters" row above into module packaging).

**Mechanism (sketch — design deferred):**

- `cusp module list` / `install <name>` / `enable|disable <name>` — applies or removes the module's migration,
  registers its verbs, toggles its renderers. The enabled set is recorded in a **`module` registry** (new core
  table or workspace config) so `generate`, `Mutate` validation, and `check` know what's live.
- **First-party "modules" vs third-party "plugins":** a single static Go binary has no portable dynamic-load
  story, so in-tree modules are **build-time-registered** (a `Module` interface + registry that `main` wires;
  runtime enable-state gates them). True out-of-tree **plugins** would ride a process boundary — the **MCP
  server** or subprocess adapters — not Go `plugin`. Open question.

**Cross-module edges (the hard part):** the polymorphic `Edge`/`EntityRef` `*_type` enums and the
`requirement_test_case` coverage junction **span layers**. Decide which module owns a cross-layer junction, and
how `check`/validation degrade when a reference points into a **not-installed** module (reject vs. tolerate as
dangling). This warrants a new entry in [decisions.md](entities/decisions.md) and likely a `module` entity in
the model once the design firms up.

## Plan generation (requirements changeset → plan)

**Requested.** A **planning skill** (agent recipe + MCP/CLI surface, installed via the
[Agent integration](#core-features) `setup`) that reads a **requirements changeset** — the FR/spec delta on a
[changeset](entities/review.md) branch — and produces an ordered **implementation plan** to deliver it. The
agent consumes the changeset diff (`cusp diff` / `dolt_diff_*`, already available) and writes the plan back
through the `Mutate` contract, so the plan is reviewable in the same changeset flow.

**Two altitudes of "planning" — keep them distinct, let them connect.** The existing
[Planning layer](entities/planning.md) (`Capability → Deliverable → View → Spec`) is **strategic** —
long-lived, top-down, milestone-scoped *what to build*. A generated plan is **tactical** — bottom-up from a
specific requirements delta, an ordered task breakdown of *how to deliver this change*. They are **not the same
entity**; forcing the second into `Deliverable` would distort both. But they should **bridge**: a plan's steps
can cite the `Requirement`s they implement and roll up into (or create) `Deliverable`s, so execution plans
ladder back to the strategic layer.

**Do we need a structure to store plans?** Leaning **yes** — a plan is durable knowledge, and invariant #1 says
the DB is the source of truth (a plan can't live only as a generated `.md`). Sketch a new **`Plan` / `PlanStep`**
sub-structure in the `planning` module (see [Modules & plugins](#modules--plugins-separable-layers)):

- **`Plan`** — title, status, the originating **`Changeset`** (which requirements delta it plans), optional
  `Milestone`/`Deliverable` rollup.
- **`PlanStep`** — ordered steps under a plan, each citing the `Requirement`(s) it satisfies (reuse the
  polymorphic `Edge`/coverage-style link), with its own status so progress is trackable.

**Open questions (design deferred):** is a `Plan` 1:1 with a changeset or longer-lived (re-planned as
requirements evolve)?; do steps reuse `Edge` (`plan_step → requirement`) or a dedicated junction?; does a step
map to a `Deliverable` or obviate one at this altitude?; and is plan generation a pure MCP-driven **skill** or
also a deterministic `cusp plan` scaffold the agent fills in. Record the outcome in
[decisions.md](entities/decisions.md) and add the entities to the [model](entities/index.md) once firm.

## Review surface — changeset review (open decision)

Humans (and agents) review the diff a changeset introduces — go section/field by section/field, comment on
requirements/plans, set a verdict — and an agent revises on the branch in response. **The schema already
models this:** [Comment](entities/review.md) carries polymorphic `subject` + `locator` + threading +
`resolved`, and [Review](entities/review.md) carries the `approve|deny|request_changes` verdict. So this is an
**interface gap, not a data-model gap** — what's missing is the surface(s) that drive those tables and the
verbs to write them.

**Two separate axes — do not conflate them:**

- **Agent ↔ Cusp = transport.** Settled: **CLI + skill + hooks** primary, MCP secondary (see
  [Agent integration](#core-features)). The agent half of the loop (read changeset diff → comment → revise)
  needs **no GUI**.
- **Human ↔ Cusp = the review *surface*.** This is the **open decision** below, and it is *independent* of the
  transport choice — MCP is an agent transport, **not** a human review UI, so choosing CLI-over-MCP for agents
  does not decide this.

**Step 1 (near-term, unblocks the whole loop):** wire `Review`/`Comment` into **CLI + MCP** — comment on
subject+locator, list/resolve threads, set verdict. Small, contract-shaped, closes the *agent* side
immediately, and is a prerequisite for every surface below. **CLI slice DONE** — `cusp comment`
(add/ls/show/resolve/reopen/edit/delete), `cusp review` (+`ls`), and per-entity `cusp changeset diff
--entities`, all writing the review rows to `main` (see [CHANGELOG.md](CHANGELOG.md)). **Remaining:** the
**MCP** surface (deferred with the rest of Agent integration — no server yet; the new `store` funcs are the
reusable seam for a thin adapter over `Mutate`).

**Step 2 (human review UX) — decided: a VS Code extension is the primary surface.** *First slice built* —
the changeset tree expands to its per-entity diff, and reviewing a changeset opens VS Code's **native diff
editor** (`vscode.diff`) over the affected spec rendered at base (`main`) vs head, with **native Comments
API** threads anchored to a requirement via `^fr-key` block anchors (add/reply/resolve → `cusp comment`, set
verdict → `cusp review`); spec **and entity** docs are diffed (a section-only edit rolls up to its doc), via
`cusp spec render`/`cusp entity render`; see [CHANGELOG.md](CHANGELOG.md). Remaining polish: base at the exact
`base_commit` (today `main`), `deliverable`/`test_case` docs, and richer thread navigation.

- **VS Code extension (chosen).** Reuse VS Code's native **diff editor** + **Comments API** (the same API the
  GitHub PR extension uses for gutter threads + a comments panel), talking to `cusp` over CLI/MCP. The
  extension holds no state — Dolt stays the source of truth, it's a third front-end over `Mutate`. It gets the
  hard 70% of a PR UI (diff rendering, thread UI, anchoring interaction) **for free**, leaving only Cusp-glue:
  a `TreeDataProvider` over `cusp changeset ls`, a virtual-doc provider rendering each entity's base/head, and
  the **`Comment`-row ↔ thread** mapping. It **preserves semantic anchoring** — a comment binds to a
  requirement row/field, not a drifting `.md` line — which is *easier* here than in GitHub because re-anchoring
  rides our deterministic canonical render order ([`SpecSectionType.position`](structure.md)). It's also a
  natural consumer of the MCP server — **this is where MCP earns its keep**.
  - **Covers the browser case too:** the same extension runs in Cursor/Windsurf and in **vscode.dev /
    code-server** (VS Code in a plain browser tab, no install), so it serves both editor and browser reviewers.
- **CLI / TUI** — `cusp changeset diff` + `cusp review` in the terminal. The Step-1 byproduct and SSH-friendly
  floor; weakest for careful line-by-line review, kept as a complement, not the main surface.
- **Federate to GitHub / DoltHub PRs** — *optional interop*, not the primary loop: project the changeset as a
  real PR and sync `Review`/`Comment` back via interop/federation. Near-zero frontend, but couples out and
  **loses semantic anchoring** (line comments on generated `.md` drift when requirements reorder). Rides the
  existing federation roadmap for GitHub-native teams.
- **Native web app** (`cusp serve --web`) — **deferred, no longer a primary goal.** The VS Code extension
  (incl. vscode.dev/code-server) covers the developer/agent audience *and* the browser case; the only persona
  left unserved is a reviewer who refuses any VS Code-flavored UI. Revisit only if that persona materializes —
  and even then build on a diff library (Monaco/CodeMirror merge, react-diff-view), never reimplement diffing.

**Decision:** Step 1 (wire `Review`/`Comment` into CLI+MCP) → **VS Code extension** as the primary human review
surface, covering editor + browser via vscode.dev/code-server; CLI/TUI as the terminal floor; GitHub federation
as optional interop; the from-scratch web app deferred unless a non-editor reviewer surface is demanded.

**Review surface — remaining follow-ups** (the CLI verbs, native-diff extension slice, exact base/head
rendering, and section/relationship roll-up shipped; see [CHANGELOG.md](CHANGELOG.md)):

- **MCP surface for `Review`/`Comment`.** The CLI slice of Step 1 is done; the MCP half is still pending
  (deferred with [Agent integration](#core-features) — no server yet). The new `store` review funcs are the
  reusable seam for a thin adapter over `Mutate`; no second source of truth.
- **Diff `deliverable` / `test_case` docs.** Only spec and entity docs are diffed today (requirement/spec/
  user_story → owning spec; entity/entity-section/relationship → entity). `deliverable`/`test_case`
  `EntityDiff` rows carry no `docRef` because there's no single-doc renderer for them yet — add
  `deliverable`/`testcase render` (or a planning/testing doc render) and populate `docRef`. Rides the same
  base/head render path (the `Reader` revision-database read handles rendering at any commit).
- **Richer thread anchoring + navigation.** Comments anchor via `^fr-key` block anchors (requirements) or
  whole-doc (entities); deepen to field/section-level locators and add a comments-panel jump list.

## "What am I missing vs beads?" — feature survey

Cross-cutting beads features (not issue-domain), and Cusp's status:

| beads | Cusp status |
|---|---|
| `dolt push/pull/remote`, `sync`, `federation` | push/pull/remote/fetch + `sync` + `clone` **DONE**; federation remaining |
| `dolt start/stop/status` (server lifecycle), `branch` | **DONE** — `cusp dolt start/stop/status` + `cusp branch ls/create/delete/checkout` (see [CHANGELOG.md](CHANGELOG.md)) |
| `export` (JSONL) | **DONE** — `cusp export` (deterministic JSONL snapshot; see [CHANGELOG.md](CHANGELOG.md)) |
| `import` | **roadmap** — Generic import (arbitrary JSON/CSV mapping) remaining; Source adapters `tutor`/`notion`/`qase` **done** |
| `batch` (bulk create) | **roadmap (Batch add — JSON/CSV)** |
| `sql` (raw passthrough) | **DONE** (read-only) — Query/inspect |
| `search` | **DONE** — Query/inspect |
| `backup`/`restore` | **roadmap (DB maintenance)** |
| `doctor`, `drift`, `preflight` | **roadmap (Validation & analysis)** |
| `gc`, `compact`, `flatten` | **DONE** — `cusp dolt gc` + `cusp dolt compact` + `cusp flatten` (see [CHANGELOG.md](CHANGELOG.md)) |
| `setup` (agent install) + `prime`/hooks | **DONE for Claude Code + Codex** — `cusp setup claude\|codex` (skill + instructions + SessionStart hook) + `cusp prime` (see [CHANGELOG.md](CHANGELOG.md)); more hosts (Gemini, Cursor rules, …) remain → **roadmap (Agent integration — more hosts)** |
| MCP server | **roadmap (MCP server)** — **secondary**, shell-less hosts only (Claude Desktop); thin adapter over `Mutate`. CLI+skill+hooks is the shipped primary path |
| `config` (get/set) | **DONE** — `cusp config get/set` (effective config + persisted actor identity; see [CHANGELOG.md](CHANGELOG.md)) |
| `version`/`upgrade` (self-update) | **DONE** — `cusp version` + `cusp upgrade` (download + checksum-verify + replace in place) |
| shell completion | **roadmap (CLI polish)** — cobra-provided |
| `hooks` (`on_create`, …) | **deferred** — `internal/hooks` not lifted (needs a node concept) |
| `worktree` commands | partial — `git.GetMainRepoRoot` is worktree-aware; explicit cmds maybe |
| `metrics`/telemetry | likely **skip / opt-in** |
| `stats`, `count`, `info`, `history`, `where` | `stats` + `history` (`log`) **DONE**; rest **roadmap (Query/inspect)** |

**Generate (Markdown/JSON/HTML), cross-references, and the glossary are Cusp-originals beads has none
of** — beads is an issue tracker that snapshots to JSONL; Cusp is a spec/knowledge store whose
human/agent views are generated from the canonical DB, woven together by inline entity links and a
shared glossary.

## Testing & CI

The **first slice is DONE** (see [CHANGELOG.md](CHANGELOG.md)): the `internal/testutil` harness (an
isolated workspace on a real owned Dolt server via `app.InitWorkspace`), the `internal/integration`
contract suite (init clean+committed, add=one-commit+clean working set+attribution,
validation-rejects-before-write, dry-run rollback, branch-scoped reads, the changeset round-trip with
review rows surviving the merge, idempotent upsert), unit tests for the actor/identity precedence and
prime rendering, and a two-job **CI** workflow (fast `go test -short` always; a dolt-backed full-suite
job). **Remaining:**

- **The coverage push is DONE — every owned package is now ≥70%** (see [CHANGELOG.md](CHANGELOG.md)):
  the logic/pure packages, then the command layer (`cmd/cusp` 10.8%→**74%** via in-process `runCLI`
  tests across all command groups). Owned total **~85%** (from 14.1%), floor at **84%** in CI (headroom
  for run-to-run variance). The push also **found + fixed a real bug** — `comment ls --subject` read
  `rev_comment` on the changeset branch (via a pooled-connection leak in `app.Reader`) instead of
  `main`, so it returned "no comments"; see [CHANGELOG.md](CHANGELOG.md).
  **Raise the floor in `scripts/coverage.sh` as coverage grows.** Remaining, all optional polish:
    - **Intentional gaps** — `app` `remote.go` push/pull/fetch/sync (need a live Dolt remote peer) and
      rare DB-error return branches (need fault injection) are the main uncovered spots; reach them with
      a remote-peer test harness / an injectable store seam if desired.
    - **Speed** — the DB-backed suite is now several minutes (per-test owned Dolt server, ~6–13s each).
      Move to one server per package with a fresh database per test if it becomes a drag.
- **Embedded-driver e2e** — the in-process `dolthub/driver/v2` test was reverted (needs cgo +
  `libicu-dev`); reintroduce behind a build tag (e.g. `-tags dolt_e2e`) once CI guarantees ICU.

## Deliberately NOT carried from beads

- **Issue-tracker verbs** — `create/close/reopen/ready/blocked/dep/assign/priority/label/epic/todo/
  defer/ack/acquire/release`. Cusp has its own entity verbs (`domain/spec/req/edge`, + planned
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
- **Lifted utilities now consumed:** `src/cli/internal/git` (workspace), `src/cli/internal/storage/dberrors`
  (schema runner). Still orphaned: `src/cli/internal/timeparsing` (pull in when a command takes dates).
- **`fr_key`** is an app-maintained column, not a SQL generated column (cross-table generation
  isn't possible in Dolt) — keep the store deriving it on write.
- **Concurrency:** same-branch number allocation is safe (`FOR UPDATE` + retry); cross-branch
  FR-number convergence is the documented merge-renumber policy (identifiers.md).
- **`go mod -C src/cli tidy`** upkeep as imports change (currently 12 direct deps — `goldmark` added for the HTML renderer).
