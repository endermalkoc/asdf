# Changelog

Completed work on **Cusp** (Agentic Delivery Lifecycle Graph) — what's built and verified. The
companion [ROADMAP.md](ROADMAP.md) tracks what's next; this file records what's already done.
Everything here was verified against real Dolt (most of it manually — see
[ROADMAP.md](ROADMAP.md#testing--ci)). The project has no tagged releases yet, so all of the below is
pre-release.

## [Unreleased]

### Foundation

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
- **`cusp init`** — creates `.cusp/`, starts a managed (owned) `dolt sql-server`, applies the
  schema, seeds the actor, records the initial Dolt commit.

### Command contract & CRUD

- **Command contract** (`internal/app.Mutate`) — every mutating command: managed connect →
  resolve changeset/`main` target → validate → transaction → mint → attribute/timestamp → real
  Dolt commit with actor+message (clean working set). Bad input fails before any write.
- **Entity CRUD** — `add`/`ls`/`show`/`edit`/`delete` across `domain`/`spec`/`req`/`entity`/`term`
  (+ `edge add`/`ls`/`delete`, `section add`/`ls`/`delete`); `edit` re-runs the shared
  canonicalize→validate→reconcile-refs layer; `delete` cleans polymorphic refs + FK cascades. All
  honor `--dry-run` / exit-codes / the active changeset (see **Broaden CRUD** for the detail).
- **Changesets (the PR model)** — `cusp changeset start/diff/submit/merge/abandon/ls`: a changeset
  is a Dolt branch; edits route to the active changeset; `diff` is the combined PR view; edits stay
  isolated from `main` until merge; `Changeset`/`Review` rows live on `main`.
- **Graph integrity** — `edge add` takes `TYPE:key` endpoints (any keyed entity —
  domain/spec/requirement/entity/milestone/glossary_term; a bare value stays a requirement fr_key),
  resolves each through the shared resolver (so a non-existent endpoint is rejected and its real type
  is recorded), and rejects self-loops and cycles for the acyclic kinds (refines/depends_on/
  supersedes/defers_to) while permitting them for references/relates (`app.ResolveRef` +
  `app.CheckEdgeAcyclic` + `store.ListEdgesOfKind`).
- **`--dry-run` flag** — global `--dry-run` injected by the `runMutate` CLI wrapper over
  `Mutate`'s `DryRun`: validates + previews, then rolls back; prints a `[dry-run] … no changes were
  committed` note.
- **Structured errors → exit codes** — `app.CodedError` tags failures at the source
  (validation=2, not_found=3, dangling_ref=4, generic=1); `Execute` maps the exit code and, under
  `--json`, emits an `{"error":{code,category,message}}` envelope on stdout. See
  [command-contract.md](command-contract.md).
- **Broaden CRUD — core surface** — `show`/`edit`/`delete` for **req, spec, domain, entity,
  glossary-term**; plus **edge `ls`/`delete`** and **section `delete`**. `edit` re-runs the shared
  canonicalize→validate→reconcile-refs layer (req statement, term definition); `delete` removes the
  row and every polymorphic reference touching it (`store.DeleteNodeRefs` over
  entity_refs/edges/external_refs), relying on FK cascade for structured children (spec→requirements/
  sections/stories; domain→specs via explicit `deleteSpecByID`; entity→sections) and cleaning non-FK
  junctions (entity relationships, glossary aliases). All honor `--dry-run` + exit codes + the active
  changeset.
- **`cusp config` (workspace generate config)** — `cusp config show` +
  `cusp config generate enable/disable/add/remove/sync` drive the workspace `.cusp/config.json`
  (the `generate` section powering incremental auto-gen).
- **Reads honor the active changeset** — `ls`/`show` reads (and the `Mutate` validation hook's
  existence/ref checks) run on the resolved target branch (`--changeset` → active changeset →
  `main`) via `app.Reader`/`app.ResolveBranch`, since branch state is connection-scoped. So you see
  edits staged in the active changeset, and validation resolves rows created earlier in the same
  changeset. `changeset ls` reads `main` (changeset metadata lives there); `generate` reads `main`
  (the canonical build).

### Generation & cross-references

- **Generate** — `cusp generate --format md|json|html`: DB → git-ignored read artifacts.
  **md/json/html. Cusp-original** — beads has no generate; it exports JSONL. Core to the "generated,
  never edited" principle. **Renderer architecture:** `Load` assembles the graph once into a
  format-agnostic view `Model` ([model.go](../internal/generate/model.go)); a `Renderer` turns it
  into output files. Two families: **document** (Markdown — Obsidian wikilinks+block-refs; HTML —
  same pipeline transformed to relative `.html` links via goldmark) and **data** (JSON — per-doc
  records + an `index.json` manifest, prose keeps raw `[[TYPE:key]]` tokens). Markdown is
  byte-identical across the refactor; adding a format = one `Renderer`. HTML ships a **default
  static-site theme** ([css.go](../internal/generate/css.go), [html.go](../internal/generate/html.go)):
  a clean StrictDoc-like stylesheet (written once to `assets/style.css`), an **always-visible sidebar
  tree** and **breadcrumbs** organized under top-level **sections** — **Specifications** (all domains
  → nested sub-directory groups → spec docs) and **Entities** (with Glossary and more to come) — that
  mirror each document's real **path** (e.g. `enrollment/student-detail/overview.md` nests under
  Specifications → Enrollment → a collapsible "Student Detail" group, and breadcrumbs read
  Specifications / Enrollment / Student Detail / Overview). The tree is built from a lightweight `Nav`
  header tree loaded on both the full and fast paths (so a single re-rendered page still carries the
  complete, byte-identical navigation), with directory groups collapsible (`<details>`), only the
  current page's branch auto-expanded (siblings stay collapsed, so navigating never re-expands the
  whole tree — there is no script), and the current page highlighted. **Iconography**
  ([icons.go](../internal/generate/icons.go), inline Feather SVGs): type glyphs in the nav/breadcrumb
  bar (domain, folder, spec, entity, glossary) and **color-graded priority badges** on user stories
  (0 Critical → 4 Backlog), decorated onto the rendered HTML so the Markdown view stays clean.
  **Spec metadata bar:** id, a colored status badge, the source created date, and the linked domain
  render as chips under the H1 — sourced from the DB columns (frontmatter in Markdown, fields in
  JSON), never duplicated as prose. The **root index page is a browsable tree** (sitemap: sections →
  domains → specs, sub-dirs collapsible). **Internal cross-reference links** are cleaned and styled:
  a source link whose text was a path (`shared/contacts-notifications.md`) renders as the target's
  **title** ("Contacts & Notifications", any `… — suffix` kept) — driven by a `Label` on the ref
  target (spec title / entity·domain name / fr_key) used in `refs.renderLink`, so Markdown and HTML
  both get clean text — and in HTML they carry an `xref` class for distinct styling. The importer
  strips the boilerplate preamble (`Feature Branch`/`Created`/`Status`/`Updated`) and captures
  `Created` into `spec.created_at`, so that metadata lives only in structured columns.
  **Planning layer:** all renderers also emit a **Planning** section — `planning/index` plus
  `capabilities` / `deliverables` / `views` roll-up pages (Markdown + HTML) and a single
  `planning.json` (`internal/generate/model.go` `loadPlanning` + `internal/store/planning_read.go`).
  Capabilities render as a **hierarchy** (domain › epic › capability, grouped by domain) — a
  nested list in Markdown, a guide-railed tree in HTML; deliverables group by milestone (with
  their linked capabilities/views/blocked-by) and views group by domain with their **route** and
  a wikilink to the backing spec. The HTML pages are rendered **straight from the Model**
  (`internal/generate/planning_html.go`) so enum values become colored **pills** (level, size,
  deliverable status, AI-ready, milestone) and every row/section carries a **type icon**; the
  section appears in the nav/sitemap/breadcrumbs only when planning data exists. Relationships
  resolve to display titles; `notion_url` stays in `planning.json` as source metadata but is not
  rendered as a link.
- **Incremental generation** — auto-regenerate only the **affected** output docs on **every DB
  change** (CLI *or* MCP), in the user-configured subset of formats. Hooked at `app.Mutate`
  ([autogen.go](../internal/app/autogen.go)): after a successful commit **to `main`** (changeset
  drafts update on merge, not in flight), it classifies the commit's dirty tables from the **Dolt
  diff** (`dolt_diff(parent, head, table)` — survives `DeleteNodeRefs`, so it's correct for deletes).
  **Fast path:** a change confined to a spec/entity's content (section, story, scenario, group, or a
  requirement *statement* edit — fr_key unchanged) re-renders **only that document**, via targeted
  loaders `LoadDocs`/`loadSpecDoc`/`loadEntityDoc` ([model.go](../internal/generate/model.go)) — no
  full `Load`. **Full path:** a structural change (spec/entity/domain/term/section-type or fr_key
  added/removed/renamed) can move indexes or the inline-ref target set, so it rebuilds the whole
  graph. **Either way** `Sync` ([incremental.go](../internal/generate/incremental.go)) writes only
  files whose content **hash** changed against a per-out-dir `.cusp-manifest.json` (full rebuild also
  deletes orphans). Pure non-render commits (`edge add`, ref index) generate nothing. **Config:**
  `.cusp/config.json` `generate.enabled` + `formats:[{format, out}]`, out defaults to
  `.cusp/artifacts/<format>`; driven by `cusp config generate enable/disable/add/remove/sync`.
  **Correctness gate (verified live):** a full `config generate sync` immediately after any
  incremental edit writes **0 files** — i.e. incremental output ≡ a full rebuild.
- **Cross-references** — inline **entity links** inside Markdown / description text fields, stored
  as canonical refs (e.g. `[[REQ:ATT-FR-012]]`), rendered by `generate` as **Obsidian wikilinks +
  block references** (`[[path#^fr-key|label]]`) for Markdown and as **relative `<a href>` links**
  for HTML (the data formats leave the raw `[[TYPE:key]]` tokens). **md/html/json. Cusp-original.**
  Design ratified in [decisions.md](entities/decisions.md); the queryable form is
  [`EntityRef`](entities/requirements.md#entityref). Targets any keyed entity
  (Domain/Spec/Requirement/Milestone/Entity/Glossary term). A dangling ref blocks an interactive
  write / warns on import / is a `check` finding. **Distinct from `Edge`**: `Edge` is the
  hand-authored structured graph; `EntityRef` holds all prose-derived references — **agents should
  prefer edges where a real relationship exists**; inline links are prose-readability sugar. Both the
  HTML render (relative `<a href>`) and `[[TERM:…]]` resolution (the resolver indexes glossary terms;
  `generate` emits the glossary page as the link target) are done.
- **Glossary / terms** — a `GlossaryTerm` store (slug, term, definition, aliases, optional domain
  scope) — shared vocabulary so humans & agents define a concept **once** and reference it
  everywhere. **Cusp-original.** Data model ([glossary.md](entities/glossary.md): `GlossaryTerm`
  + `GlossaryAlias`); `cusp term add/ls/show/edit/delete` (with aliases); `[[TERM:slug]]` resolves
  (slug + aliases) and `generate` emits the `glossary.md` page (block-anchor per term) as the link
  target. Distinct from the business **Entity** layer (domain *documents*) — a term is project
  *vocabulary*. **Note:** the tutor corpus seeds no terms, so the page is empty until terms are
  authored.

### Import

- **Import pipeline + `tutor` source adapter** — a source-agnostic staging core (`internal/importer`:
  a `Graph` of Cusp entity shapes + a drift/ER-gap `Report` + an idempotent `Apply` writer keyed on
  business identifiers) and the deterministic, **no-LLM** `tutor` adapter (`internal/importer/tutor`).
  `cusp import tutor <docs>` parses **Domain/Spec/Requirement/UserStory/AcceptanceScenario/Edge/
  Milestone/Entity** and reports counts + coverage + drift; `--apply` loads the graph
  through `app.Mutate` (one changeset/`main` commit), idempotent on re-run. Validated against the
  real corpus (~2.2k FRs, 123 specs, 23 entities). The five ER refinements it
  surfaced are resolved in [decisions.md](entities/decisions.md). The structured authorization
  layer (`Privilege`/`AccessRule`) was **removed** — a tutor-specific, never-consumed paradigm;
  access rules stay as entity-doc prose (migration `0012`, [decisions.md](entities/decisions.md)).
- **Notion source adapter (planning layer)** — `cusp import notion` ingests the Notion
  Capabilities / Deliverables / Views databases into the [planning layer](entities/planning.md)
  (`internal/importer/notion`). Maps each page → `Capability` (3-tier `level`, self-nested via
  `parent_id`), `Deliverable` (`size`/`status`/`ai_ready`, milestone-scoped — Notion's `—`
  placeholder → `proposed`, `Yes/No/N/A` → `yes/no/na`), and `View` (route; soft `spec_id` resolved
  by unique spec slug, best-effort); Notion **relation** properties → the planning junctions
  (`capability_milestone`, `capability_deliverable`, `deliverable_view`, `deliverable_dependency`),
  reconciled per owner so a relation removed at the source drops its row on re-import; Notion `Domain`
  / `Milestone` values seed `Domain`/`Milestone` rows (a page with no Domain → an `unassigned`
  domain). Every page is traced back to its origin via a `pub_external_ref` (`system=notion`, with the
  page URL), and a deliverable's `Bead IDs` → a `system=beads` ref. **Idempotency:** planning rows
  have no natural unique key, so each row id is **deterministic** (`ids.Rel` over the stable Notion
  page id) — re-import converges (0 inserts on the second run) instead of duplicating. Source is the
  **Notion API** (`--token`, or `$CUSP_NOTION_TOKEN` / `$NOTION_API_KEY`, paginated) or saved query
  responses (`--from <dir>`) for an offline/testable path. Read-only report by default; `--apply`
  rides `app.Mutate` (one transaction, one Dolt commit). Verified end-to-end against the live
  workspace (392 capabilities, 252 deliverables, 20 views; idempotent re-run). The pure mapping is
  unit-tested. This widened `ExternalRef.subject_type` to add `capability` / `view` (and `notion` as a
  `system`) — see [interop.md](entities/interop.md).

### Remote sync

- **Remote sync — push/pull/remote/fetch + sync.** `cusp dolt remote add/remove/ls`,
  `cusp dolt push/pull/fetch [remote] [branch]` (default origin/main, `--force`, `--user` +
  `DOLT_REMOTE_PASSWORD`), and `cusp sync` (pull-then-push the canonical branch). Orchestration in
  [app/remote.go](../internal/app/remote.go) pins one connection (branch state is connection-scoped;
  a pull's merge needs one session — runs through `MergeAndSettle`) over the lifted
  `versioncontrolops` primitives; a pull/sync that advanced the branch **refreshes the generated
  artifacts** if auto-gen is enabled (a merge bypasses the `Mutate` hook). Verified end-to-end
  against a `file://` remote: add/ls/push/fetch, a divergent commit pulled back + merged, and a local
  edit round-tripped via `sync`.

### Server, branches & DB maintenance

- **Dolt server lifecycle — `cusp dolt start/stop/status`** ([dolt.go](../cmd/cusp/dolt.go)).
  `start` starts (or adopts, idempotently) the managed owned server; `stop` gracefully shuts it
  down (flush → SIGTERM → SIGKILL, `--force` to skip the wait); `status` reports mode + pid/port/
  data-dir/logs (text or `--json`). `start`/`stop` refuse when the workspace targets an external
  server (`--dsn`/`$CUSP_DSN` or an explicit port). Closes the gap where owned mode could auto-start
  a server but offered no clean stop — wraps the lifted `doltserver.Start/StopWithForce/IsRunning`.
- **Raw branch escape hatch — `cusp branch ls/create/delete/checkout`**
  ([branch.go](../cmd/cusp/branch.go), [app/branch.go](../internal/app/branch.go)). The low-level
  view beneath the changeset model: `ls` lists Dolt branches (marks the active target, tags
  `changeset/*`), `create` branches off the active target, `delete` removes one (refuses `main` and
  the active target), `checkout` retargets the ambient read/write branch (reusing the active-changeset
  pointer; `main` clears it). For the tracked PR workflow use `cusp changeset`; these are for
  diagnostics and manual surgery.
- **History maintenance — `cusp flatten` + `cusp dolt compact`**
  ([flatten.go](../cmd/cusp/flatten.go), [app/maintenance.go](../internal/app/maintenance.go)).
  `flatten` squashes all Dolt history into one snapshot (irreversible; `--force`/`--dry-run`);
  `dolt compact --days N` squashes commits older than the window into one base, cherry-picking the
  recent ones on top (`--force`/`--dry-run`), driving the lifted `versioncontrolops.Compact` from the
  commit log. Both GC afterward (best-effort). Verified end-to-end against real Dolt: a 7-commit
  history collapses to root + 1 snapshot with all `req_domain` data intact.

### Query & inspect

- **Query / inspect — `sql`/`stats`/`search`/`log`** ([query.go](../cmd/cusp/query.go)).
  **`cusp sql <query>`** — read-only passthrough (SELECT/SHOW/DESCRIBE/EXPLAIN/WITH only; writes
  rejected with exit 2, so the attributed write path is never bypassed — invariant #3),
  dynamic-column table output or `--json` array, honors the active changeset, and reaches the
  `dolt_*` system tables (history/diff/blame) and Dolt `AS OF` time-travel for free. **`cusp stats`**
  — counts per layer. **`cusp search <text>`** — LIKE across domain names / spec titles /
  requirement fr_keys+statements / entity names+descriptions / glossary terms (`store.Search`).
  **`cusp log`** — recent commits from `dolt_log`. A shared dynamic row-printer (`writeRows`) backs
  them.

### Validation & analysis

- **`cusp check` (first slice)** — read-only whole-graph integrity: scans every prose field
  (`store.ListProseFields`) for inline `[[TYPE:key]]` tokens that don't resolve (`app.Check`), audits
  each acyclic edge kind for cycles (`findCycle`), honors the active changeset, exits nonzero on
  findings (gates CI/agents).
- **`cusp impact <ref>`** — graph traversal around an entity (TYPE:key): inbound (what references /
  points an edge at it = what's affected if it changes) and outbound (what it relies on), over
  entity_refs + edges, with `--transitive` for the reverse-edge blast radius (`app.Impact` +
  `store.ListAllEdges`/`ListEntityRefsFor`); honors the active changeset.

### Distribution & self-update

- **`cusp version`** ([version.go](../cmd/cusp/version.go)) — reports version / commit / build date /
  Go toolchain / os·arch (human or `--json`). Build metadata is injected at release time via ldflags
  (GoReleaser / Makefile) and falls back to `runtime/debug.ReadBuildInfo()` for `go install` builds.
  See [build-and-release.md](build-and-release.md).
- **`cusp upgrade [version]`** — self-update in place ([upgrade.go](../cmd/cusp/upgrade.go) +
  `internal/selfupdate`). Resolves the latest GitHub release (or a pinned tag), downloads the archive
  for the running OS/arch, **verifies its SHA-256 against the release's `checksums.txt`**, then
  atomically replaces the running binary (rename on Unix; move-aside on Windows). `--check` reports
  availability without installing; `--json` for scripts; fails fast with a clear message when the
  binary's directory isn't writable. Mirrors the exact artifact conventions of
  [install.sh](../install.sh) / [.goreleaser.yaml](../.goreleaser.yaml), so it updates from the same
  release assets the installer consumes. Pure-stdlib (archive/tar·zip, crypto/sha256, net/http — no
  new deps); the asset-name / checksum-parse / extract / replace helpers are unit-tested.
- **CLI error output** — `Execute` is now the sole error reporter (cobra's own error print is
  silenced via `SilenceErrors`), so a failure emits **once**: a single `error: …` line, or the
  structured `{"error":{code,category,message}}` envelope under `--json` (previously cobra also
  printed a duplicate `Error: …` line, which leaked plain text alongside the JSON envelope).

### Project & resolved decisions

- **Rename / rebrand — adlg → cusp (full).** The `adlg` acronym (Agentic Delivery Lifecycle
  Graph) was hard to remember and gave the CLI no memorable handle. Renamed to **Cusp** — a short,
  brandable name (a *cusp* is the point of transition where one state becomes the next: spec →
  requirement → test → shipped code), verified collision-free in the dev-tool/agentic space. Applied
  end-to-end: command/binary `cusp`, the `cmd/cusp` entrypoint, the `.cusp/` config/workspace dir,
  the `CUSP_*` env prefix, **the Go module path (`github.com/endermalkoc/cusp`), all internal
  identifiers** (`cuspDir`, `EnsureCuspDir`, …), the shared-server global DB name (`cusp_global`), and
  all docs/help/output; *Agentic Delivery Lifecycle Graph* is retained as the tagline. The GitHub repo
  and local folder renames are manual follow-ups.
- **Rename / rebrand — asdf → adlg (full).** The old `asdf` binary/command name collided with the
  [asdf version manager](https://asdf-vm.com/), a widely-installed CLI of the same name. Renamed to
  **`adlg` (Agentic Delivery Lifecycle Graph)** end-to-end: published binary/command, the `cmd/adlg`
  entrypoint, the `.adlg/` config/workspace dir, the `ADLG_*` env prefix, **the Go module path and
  GitHub repo (`github.com/endermalkoc/adlg`), all internal identifiers** (`adlgDir`, `EnsureAdlgDir`,
  …), and the shared-server global DB name (`adlg_global`); docs/help/output all updated. Kept as
  `asdf` only: references to the *other* tool (asdf-vm) and the developer's personal `~/asdf-tutor`
  sandbox.
- **Cross-reference syntax — resolved** ([decisions.md](entities/decisions.md)): token form is
  `[[TYPE:key]]` (optional `|display`); the Markdown render is an Obsidian wikilink with a `^block`
  reference anchor, and a relative HTML `<a href>` for the HTML renderer; the **edge-vs-inline-link**
  policy is settled — prose-derived references go to
  [`EntityRef`](entities/requirements.md#entityref), `Edge` is hand-authored/structured. Data
  model + implementation **done** (canonicalize-on-write through the shared ingestion layer,
  md/html/json render — see **Cross-references** above).
- **Glossary schema — resolved** ([glossary.md](entities/glossary.md),
  [decisions.md](entities/decisions.md)): `GlossaryTerm`(slug, term, definition, optional
  `domain_id`, status) + `GlossaryAlias`(`UNIQUE(alias)`); the `[[TERM:slug]]` link key is the slug,
  aliases resolve too. Data model + implementation **done** (see **Glossary / terms** above).
