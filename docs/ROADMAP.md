# ASDF Roadmap

What's built and what's next. A living document — pairs with [ARCHITECTURE.md](ARCHITECTURE.md)
(how it's put together), [docs/entities/](entities/index.md) (the data model), and
[docs/command-contract.md](command-contract.md) (the workflow every command follows).

## Done

- **Dolt infrastructure** (salvaged from [beads](https://github.com/steveyegge/beads), MIT — see
  [NOTICE](../NOTICE)): owned/external/embedded server management (`internal/doltserver`), the warm
  proxy daemon (`dbproxy`), version-control ops over a `DBConn` (`versioncontrolops`: branch/
  commit/merge/clone/gc/flatten/backup), remotes (`remotecache`, `doltremote`), config/git/error
  helpers. Issue-domain dependency severed to a minimal shim; all pure-Go, building clean.
- **Data model** ([docs/entities/](entities/index.md)) — authoritative, consistent, with the
  deterministic relationship-PK rule.
- **Schema** — `0001_init` (26 entities + 6 junctions) + `0002` (`domain.description`) + a migration
  runner (`internal/storage/schema`); validated against real Dolt (FK/UNIQUE/deterministic-PK enforced).
- **ID minting** (`internal/ids`) — ULID (authored rows) + deterministic `uuidv5` (relationships).
- **`asdf init`** — creates `.asdf/`, starts a managed (owned) `dolt sql-server`, applies the
  schema, seeds the actor, records the initial Dolt commit.
- **Command contract** (`internal/app.Mutate`) — every mutating command: managed connect →
  resolve changeset/`main` target → validate → transaction → mint → attribute/timestamp → real
  Dolt commit with actor+message (clean working set). Bad input fails before any write.
- **Entity CRUD** — `add`/`ls`/`show`/`edit`/`delete` across `domain`/`spec`/`req`/`entity`/`term`
  (+ `edge add`/`ls`/`delete`, `section add`/`ls`/`delete`); `edit` re-runs the shared
  canonicalize→validate→reconcile-refs layer; `delete` cleans polymorphic refs + FK cascades. All
  honor `--dry-run` / exit-codes / the active changeset (see "Broaden CRUD" below for the detail).
- **Changesets (the PR model)** — `asdf changeset start/diff/submit/merge/abandon/ls`: a changeset
  is a Dolt branch; edits route to the active changeset; `diff` is the combined PR view; edits stay
  isolated from `main` until merge; `Changeset`/`Review` rows live on `main`.
- **Import pipeline + `tutor` source adapter** — a source-agnostic staging core (`internal/importer`:
  a `Graph` of ASDF entity shapes + a drift/ER-gap `Report` + an idempotent `Apply` writer keyed on
  business identifiers) and the deterministic, **no-LLM** `tutor` adapter (`internal/importer/tutor`).
  `asdf import tutor <docs>` parses **Domain/Spec/Requirement/UserStory/AcceptanceScenario/Edge/
  Milestone/Entity** and reports counts + coverage + drift; `--apply` loads the graph
  through `app.Mutate` (one changeset/`main` commit), idempotent on re-run. Validated against the
  real corpus (~2.2k FRs, 123 specs, 23 entities). The five ER refinements it
  surfaced are resolved in [decisions.md](entities/decisions.md). Deferred: `EntityAttribute`/
  `EntityRelationship` (they live in entity-doc prose, not a structured form) and `Test*` (no
  test-management export in the corpus). The structured authorization layer (`Privilege`/`AccessRule`)
  was **removed** — a tutor-specific, never-consumed paradigm; access rules stay as entity-doc prose
  (migration `0012`, [decisions.md](entities/decisions.md)).

## Next — finish the command contract

- **Graph integrity — DONE.** `edge add` now takes `TYPE:key` endpoints (any keyed entity —
  domain/spec/requirement/entity/milestone/glossary_term; a bare value stays a requirement fr_key),
  resolves each through the shared resolver (so a non-existent endpoint is rejected and its real type
  is recorded), and rejects self-loops and cycles for the acyclic kinds (refines/depends_on/
  supersedes/defers_to) while permitting them for references/relates (`app.ResolveRef` +
  `app.CheckEdgeAcyclic` + `store.ListEdgesOfKind`). **Deferred:** per-kind *type* policy (which
  endpoint types a given kind may link) — left generic for now.
- **`--dry-run` flag — DONE.** Global `--dry-run` injected by the `runMutate` CLI wrapper over
  `Mutate`'s `DryRun`: validates + previews, then rolls back; prints a `[dry-run] … no changes were
  committed` note.
- **Structured errors → exit codes — DONE.** `app.CodedError` tags failures at the source
  (validation=2, not_found=3, dangling_ref=4, generic=1); `Execute` maps the exit code and, under
  `--json`, emits an `{"error":{code,category,message}}` envelope on stdout. See
  [command-contract.md](command-contract.md).
- **Broaden CRUD — core surface DONE.** `show`/`edit`/`delete` for **req, spec, domain, entity,
  glossary-term**; plus **edge `ls`/`delete`** and **section `delete`**. `edit` re-runs the shared
  canonicalize→validate→reconcile-refs layer (req statement, term definition); `delete` removes the
  row and every polymorphic reference touching it (`store.DeleteNodeRefs` over
  entity_refs/edges/external_refs), relying on FK cascade for structured children (spec→requirements/
  sections/stories; domain→specs via explicit `deleteSpecByID`; entity→sections) and cleaning non-FK
  junctions (entity relationships, glossary aliases). All honor `--dry-run` + exit codes + the active
  changeset. **Remaining:** the not-yet-modeled-as-CRUD entities (Milestone, Test*, Capability,
  Deliverable, View, ExternalRef).
- **`asdf config`** — **partly done:** `asdf config show` + `asdf config generate enable/disable/add/remove/sync` drive the workspace `.asdf/config.json` (the `generate` section powering incremental auto-gen). A general typed `config get/set` over the lifted `internal/config`/`configfile` (Dolt server settings) is still open.
- **Reads honor the active changeset — DONE.** `ls`/`show` reads (and the `Mutate` validation hook's
  existence/ref checks) now run on the resolved target branch (`--changeset` → active changeset →
  `main`) via `app.Reader`/`app.ResolveBranch`, since branch state is connection-scoped. So you see
  edits staged in the active changeset, and validation resolves rows created earlier in the same
  changeset. `changeset ls` still reads `main` (changeset metadata lives there). `generate` still
  reads `main` (the canonical build) — make it branch-aware if a changeset-preview render is wanted.

## Core features

| Feature | What | Status / notes |
|---|---|---|
| **Remote sync** | `asdf dolt push/pull/remote/clone`, `asdf sync`, federation (peers) | **DONE (push/pull/remote/fetch + sync).** `asdf dolt remote add/remove/ls`, `asdf dolt push/pull/fetch [remote] [branch]` (default origin/main, `--force`, `--user` + `DOLT_REMOTE_PASSWORD`), and `asdf sync` (pull-then-push the canonical branch). Orchestration in [app/remote.go](../internal/app/remote.go) pins one connection (branch state is connection-scoped; a pull's merge needs one session — runs through `MergeAndSettle`) over the lifted `versioncontrolops` primitives; a pull/sync that advanced the branch **refreshes the generated artifacts** if auto-gen is enabled (a merge bypasses the `Mutate` hook). Verified end-to-end against a `file://` remote: add/ls/push/fetch, a divergent commit pulled back + merged, and a local edit round-tripped via `sync`. **Remaining:** `asdf dolt clone` (bootstrap a workspace from a remote — distinct flow, no existing `.asdf`) and federation/peers. |
| **Generate** | `asdf generate --format md\|json\|html`: DB → git-ignored read artifacts | **DONE (md/json/html). ASDF-original** — beads has no generate; it exports JSONL. Core to the "generated, never edited" principle. **Renderer architecture:** `Load` assembles the graph once into a format-agnostic view `Model` ([model.go](../internal/generate/model.go)); a `Renderer` turns it into output files. Two families: **document** (Markdown — Obsidian wikilinks+block-refs; HTML — same pipeline transformed to relative `.html` links via goldmark) and **data** (JSON — per-doc records + an `index.json` manifest, prose keeps raw `[[TYPE:key]]` tokens). Markdown is byte-identical across the refactor; adding a format = one `Renderer`. HTML ships a **default static-site theme** ([css.go](../internal/generate/css.go), [html.go](../internal/generate/html.go)): a clean StrictDoc-like stylesheet (written once to `assets/style.css`), an **always-visible sidebar tree** and **breadcrumbs** organized under top-level **sections** — **Specifications** (all domains → nested sub-directory groups → spec docs) and **Entities** (with Glossary and more to come) — that mirror each document's real **path** (e.g. `enrollment/student-detail/overview.md` nests under Specifications → Enrollment → a collapsible "Student Detail" group, and breadcrumbs read Specifications / Enrollment / Student Detail / Overview). The tree is built from a lightweight `Nav` header tree loaded on both the full and fast paths (so a single re-rendered page still carries the complete, byte-identical navigation), with directory groups collapsible (`<details>`), only the current page's branch auto-expanded (siblings stay collapsed, so navigating never re-expands the whole tree — there is no script), and the current page highlighted. **Iconography** ([icons.go](../internal/generate/icons.go), inline Feather SVGs): type glyphs in the nav/breadcrumb bar (domain, folder, spec, entity, glossary) and **color-graded priority badges** on user stories (0 Critical → 4 Backlog), decorated onto the rendered HTML so the Markdown view stays clean. **Spec metadata bar:** id, a colored status badge, the source created date, and the linked domain render as chips under the H1 — sourced from the DB columns (frontmatter in Markdown, fields in JSON), never duplicated as prose. The **root index page is a browsable tree** (sitemap: sections → domains → specs, sub-dirs collapsible). **Internal cross-reference links** are cleaned and styled: a source link whose text was a path (`shared/contacts-notifications.md`) renders as the target's **title** ("Contacts & Notifications", any `… — suffix` kept) — driven by a `Label` on the ref target (spec title / entity·domain name / fr_key) used in `refs.renderLink`, so Markdown and HTML both get clean text — and in HTML they carry an `xref` class for distinct styling. The importer strips the boilerplate preamble (`Feature Branch`/`Created`/`Status`/`Updated`) and captures `Created` into `spec.created_at`, so that metadata lives only in structured columns. Both styling and incremental regeneration (next row) are **done**. |
| **Incremental generation** | auto-regenerate only the **affected** output docs on **every DB change** (CLI *or* MCP), in the user-configured subset of formats; **performance-critical** | **DONE.** Hooked at `app.Mutate` ([autogen.go](../internal/app/autogen.go)): after a successful commit **to `main`** (changeset drafts update on merge, not in flight), it classifies the commit's dirty tables from the **Dolt diff** (`dolt_diff(parent, head, table)` — survives `DeleteNodeRefs`, so it's correct for deletes). **Fast path:** a change confined to a spec/entity's content (section, story, scenario, group, or a requirement *statement* edit — fr_key unchanged) re-renders **only that document**, via targeted loaders `LoadDocs`/`loadSpecDoc`/`loadEntityDoc` ([model.go](../internal/generate/model.go)) — no full `Load`. **Full path:** a structural change (spec/entity/domain/term/section-type or fr_key added/removed/renamed) can move indexes or the inline-ref target set, so it rebuilds the whole graph. **Either way** `Sync` ([incremental.go](../internal/generate/incremental.go)) writes only files whose content **hash** changed against a per-out-dir `.asdf-manifest.json` (full rebuild also deletes orphans). Pure non-render commits (`edge add`, ref index) generate nothing. **Config:** `.asdf/config.json` `generate.enabled` + `formats:[{format, out}]`, out defaults to `.asdf/artifacts/<format>`; driven by `asdf config generate enable/disable/add/remove/sync`. **Correctness gate (verified live):** a full `config generate sync` immediately after any incremental edit writes **0 files** — i.e. incremental output ≡ a full rebuild. |
| **Cross-references** | inline **entity links** inside Markdown / description text fields — stored as canonical refs (e.g. `[[REQ:ATT-FR-012]]`), rendered by `generate` as **Obsidian wikilinks + block references** (`[[path#^fr-key|label]]`) for Markdown and as **relative `<a href>` links** for HTML (the data formats leave the raw `[[TYPE:key]]` tokens) | **DONE (md/html/json). ASDF-original.** Design ratified in [decisions.md](entities/decisions.md); the queryable form is [`EntityRef`](entities/requirements.md#entityref). Targets any keyed entity (Domain/Spec/Requirement/Milestone/Entity; Glossary deferred). Dangling ref → blocks an interactive write / warns on import / a `check` finding later. **Distinct from `Edge`**: `Edge` is the hand-authored structured graph; `EntityRef` holds all prose-derived references — **agents should prefer edges where a real relationship exists**; inline links are prose-readability sugar. Both formerly-deferred pieces are now **done**: the **HTML render** (relative `<a href>` via the HTML renderer) and **`[[TERM:…]]`** resolution (the resolver indexes glossary terms; `generate` emits the glossary page as the link target). |
| **Glossary / terms** | a `GlossaryTerm` store (slug, term, definition, aliases, optional domain scope) — shared vocabulary so humans & agents define a concept **once** and reference it everywhere | **DONE. ASDF-original.** Data model ([glossary.md](entities/glossary.md): `GlossaryTerm` + `GlossaryAlias`); `asdf term add/ls/show/edit/delete` (with aliases); `[[TERM:slug]]` resolves (slug + aliases) and `generate` emits the `glossary.md` page (block-anchor per term) as the link target. Distinct from the business **Entity** layer (domain *documents*) — a term is project *vocabulary*. **Note:** the tutor corpus seeds no terms, so the page is empty until terms are authored. |
| **Batch add** | `asdf <entity> add --file <f>` and/or `asdf batch <f>` — bulk-create entities from a **JSON/CSV** file in ASDF's own shape, in **one changeset/commit** | adapt beads' `bd create --file`/`--graph`; rides the `Mutate` wrapper so the whole batch is one transaction + one Dolt commit. |
| **Generic import** | `asdf import --format json\|csv <f>` — ingest **arbitrary external** JSON/CSV and map columns/fields into the schema via a mapping spec | **TODO.** The staging core (`internal/importer`: `Graph`/`Report`/idempotent `Apply`) exists from the tutor work; still needed is the external-shape → field-mapping front end (distinct from batch add). Routes through the contract. |
| **Source adapters** | `asdf import <source>` — pluggable per-source adapters on the staging core | **`tutor` done** (see Done — read-only report + `--apply`, Domain→Entity). Remaining: `Test*` needs a Qase export (absent in the corpus); `EntityAttribute`/`EntityRelationship` need a non-prose source or an enrichment pass. Future adapters reuse `importer.Apply`. |
| **Export** | `asdf export` — JSONL snapshot (git-friendly, diffable) | beads' model; useful for backups/interop alongside Dolt history. |
| **Validation & analysis** | `asdf check` (traceability), `asdf impact <id>` (graph traversal), `asdf doctor` (health + auto-fix), drift detection | **`asdf check` first slice DONE:** read-only whole-graph integrity — scans every prose field (`store.ListProseFields`) for inline `[[TYPE:key]]` tokens that don't resolve (`app.Check`), audits each acyclic edge kind for cycles (`findCycle`), honors the active changeset, exits nonzero on findings (gates CI/agents). **`asdf impact <ref>` DONE:** graph traversal around an entity (TYPE:key) — inbound (what references / points an edge at it = what's affected if it changes) and outbound (what it relies on), over entity_refs + edges, with `--transitive` for the reverse-edge blast radius (`app.Impact` + `store.ListAllEdges`/`ListEntityRefsFor`); honors the active changeset. **Remaining:** richer traceability (orphan-FR/coverage rollups), `impact` over `ent_relationship` too, `doctor`/`drift` (adapt beads patterns; we have `schema.CheckForwardDrift`-style hooks). |
| **Query / inspect** | `asdf sql` (raw passthrough), `query`, `search`, `stats`, `history`/`diff` (Dolt-native), `show` | **DONE** ([query.go](../cmd/asdf/query.go)). **`asdf sql <query>`** — read-only passthrough (SELECT/SHOW/DESCRIBE/EXPLAIN/WITH only; writes rejected with exit 2, so the attributed write path is never bypassed — invariant #3), dynamic-column table output or `--json` array, honors the active changeset, and reaches the `dolt_*` system tables (history/diff/blame) and Dolt `AS OF` time-travel for free. **`asdf stats`** — counts per layer. **`asdf search <text>`** — LIKE across domain names / spec titles / requirement fr_keys+statements / entity names+descriptions / glossary terms (`store.Search`). **`asdf log`** — recent commits from `dolt_log`. A shared dynamic row-printer (`writeRows`) backs them. **Optional extensions (not blocking):** a higher-level `query` DSL and a standalone `asdf diff <from> <to>`. |
| **Agent integration** | `asdf setup` → install into **Claude Code**, Codex, Cursor, Gemini, Aider, opencode; the **MCP server** (`asdf serve --mcp`) | **requested ("initialize into Claude Code").** Mirror beads' `cmd/bd/setup` (same agent targets). MCP is in the README roadmap. |
| **DB maintenance** | `asdf backup`/`restore`, `gc` (Dolt GC), `compact`/`flatten` (history compaction) | Infra lifted in `versioncontrolops` (gc/compact/flatten); wire the CLI. |
| **Self-update** | **`asdf upgrade`** — download the latest release binary, verify its checksum, and replace in place; **`asdf version`** reports the build | **requested.** Mirror beads' `cmd/bd/upgrade.go`; pairs with the GoReleaser/GitHub-Releases distribution in the README. |
| **CLI polish** | **help system** (rich help + examples, `help-all` overview), shell completion | **requested (help).** Cobra gives base help/completion; add per-command examples and a top-level overview. |

## "What am I missing vs beads?" — feature survey

Cross-cutting beads features (not issue-domain), and ASDF's status:

| beads | ASDF status |
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
| `config` (get/set) | **Next** |
| `version`/`upgrade` (self-update) | **roadmap (Self-update)** |
| shell completion | **roadmap (CLI polish)** — cobra-provided |
| `hooks` (`on_create`, …) | **deferred** — `internal/hooks` not lifted (needs a node concept) |
| `worktree` commands | partial — `git.GetMainRepoRoot` is worktree-aware; explicit cmds maybe |
| `metrics`/telemetry | likely **skip / opt-in** |
| `stats`, `count`, `info`, `history`, `where` | `stats` + `history` (`log`) **DONE**; rest **roadmap (Query/inspect)** |

**Generate (Markdown/JSON/HTML), cross-references, and the glossary are ASDF-originals beads has none
of** — beads is an issue tracker that snapshots to JSONL; ASDF is a spec/knowledge store whose
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
  command contract end-to-end: `asdf init`; each `add` produces one Dolt commit with a **clean
  working set**; validation rejects bad input; and the full **changeset round-trip** (`start → add
  on branch → diff → submit → merge`, with `Changeset`/`Review` rows staying on `main`). Reference:
  beads' `internal/testutil/integration`.
- **Embedded-driver e2e** — the in-process `dolthub/driver/v2` test was reverted (needs cgo +
  `libicu-dev`); reintroduce behind a build tag (e.g. `-tags dolt_e2e`) once CI guarantees ICU.
- **CI** — run `go build ./...` · `go vet ./...` · `go test ./...` on every push; gate the
  integration suite on `dolt` availability (PATH or testcontainers).

## Deliberately NOT carried from beads

- **Issue-tracker verbs** — `create/close/reopen/ready/blocked/dep/assign/priority/label/epic/todo/
  defer/ack/acquire/release`. ASDF has its own entity verbs (`domain/spec/req/edge`, + planned
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
- **Rename / rebrand (blocker before release)** — the **`asdf` binary/command name collides with the
  [asdf version manager](https://asdf-vm.com/)**, a widely-installed CLI of the same name; a user who has
  both on `PATH` gets whichever shadows the other, so `asdf <cmd>` is ambiguous on most dev machines. The
  `.asdf/` dir + `ASDF_*` env vars collide for the same reason. Pick a new published binary name + config
  dir + env prefix before release (branding flows from a few constants — `cmd/asdf` use string, the
  `.asdf` path, the `ASDF_*` lookups). The module path and internal package names can stay.
- **Concurrency:** same-branch number allocation is safe (`FOR UPDATE` + retry); cross-branch
  FR-number convergence is the documented merge-renumber policy (identifiers.md).
- **Cross-reference syntax — RESOLVED** ([decisions.md](entities/decisions.md)): token form is
  `[[TYPE:key]]` (optional `|display`); the Markdown render is an Obsidian wikilink with a `^block`
  reference anchor for Markdown, and a relative HTML `<a href>` for the HTML renderer; the **edge-vs-inline-link** policy
  is settled — prose-derived references go to [`EntityRef`](entities/requirements.md#entityref), `Edge`
  is hand-authored/structured. Data model + implementation **done** (canonicalize-on-write through the
  shared ingestion layer, md/html/json render — see the Cross-references core-feature row).
- **Glossary schema — RESOLVED** ([glossary.md](entities/glossary.md), [decisions.md](entities/decisions.md)):
  `GlossaryTerm`(slug, term, definition, optional `domain_id`, status) + `GlossaryAlias`(`UNIQUE(alias)`); the
  `[[TERM:slug]]` link key is the slug, aliases resolve too. Data model + implementation **done** (see the
  Glossary core-feature row).
- **`go mod tidy`** upkeep as imports change (currently 12 direct deps — `goldmark` added for the HTML renderer).
