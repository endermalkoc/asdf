# Changelog

Completed work on **Cusp** (Agentic Delivery Lifecycle Graph) — what's built and verified. The
companion [ROADMAP.md](ROADMAP.md) tracks what's next; this file records what's already done.
Everything here was verified against real Dolt (most of it manually — see
[ROADMAP.md](ROADMAP.md#testing--ci)). The project has no tagged releases yet, so all of the below is
pre-release.

## [Unreleased]

### Foundation

- **Dolt infrastructure** (salvaged from [beads](https://github.com/steveyegge/beads), MIT — see
  [NOTICE](../NOTICE)): owned/external/embedded server management (`src/cli/internal/doltserver`), the warm
  proxy daemon (`dbproxy`), version-control ops over a `DBConn` (`versioncontrolops`: branch/
  commit/merge/clone/gc/flatten/backup), remotes (`remotecache`, `doltremote`), config/git/error
  helpers. Issue-domain dependency severed to a minimal shim; all pure-Go, building clean.
- **Data model** ([docs/entities/](entities/index.md)) — authoritative, consistent, with the
  deterministic relationship-PK rule.
- **Schema** — `0001_init` (26 entities + 6 junctions) + `0002` (`domain.description`) + a migration
  runner (`src/cli/internal/storage/schema`); validated against real Dolt (FK/UNIQUE/deterministic-PK enforced).
- **ID minting** (`src/cli/internal/ids`) — ULID (authored rows) + deterministic `uuidv5` (relationships).
- **`cusp init`** — creates `.cusp/`, starts a managed (owned) `dolt sql-server`, applies the
  schema, seeds the actor, records the initial Dolt commit.

### Command contract & CRUD

- **Command contract** (`src/cli/internal/app.Mutate`) — every mutating command: managed connect →
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
- **External references — `cusp ref add/ls/show/rm`** ([ref.go](../src/cli/cmd/cusp/ref.go),
  [store/externalref.go](../src/cli/internal/store/externalref.go)). CRUD over `pub_external_ref` — a
  link from a Cusp subject (deliverable | requirement | test_result) to its id/key in an outside
  system (jira, github, beads, linear, …), previously written only by importers. `add` resolves the
  subject from a `TYPE:key` reference (a bare value is a requirement fr_key) or an explicit
  `--subject-type`/`--subject-id` escape hatch for tokenless subjects, and is idempotent by
  (subject, system) via the shared `store.UpsertExternalRef`; `ls` lists all or one subject's refs
  (subjects labelled via `app.LabelIndex`); `show`/`rm` operate by id. Rides the `Mutate` contract
  (active changeset, `--dry-run`, exit codes); the table is non-rendering, so no doc regen.
- **`cusp config` (workspace generate config)** — `cusp config show` +
  `cusp config generate enable/disable/add/remove/sync` drive the workspace `.cusp/config.json`
  (the `generate` section powering incremental auto-gen).
- **`cusp config get/set` (effective config + actor identity)**
  ([config.go](../src/cli/cmd/cusp/config.go), [workspace/identity.go](../src/cli/internal/workspace/identity.go)).
  `config get [key]` prints the resolved configuration in one place — actor identity (what writes are
  attributed to), Dolt server settings (mode / database / host / live port / user), and the generate
  config — all resolved without a database connection (and without auto-starting the server). `config
  set user.handle|user.name|user.email` persists a **per-user, git-ignored** identity to
  `.cusp/identity.json`, wired into `workspace.ResolveActor` between the `--actor`/`$CUSP_ACTOR`
  override and git config — so a developer's writes are attributed without passing `--actor` each
  time, and the identity isn't shared through the host repo. Server settings are workspace-managed
  (read-only via `get`); generate settings keep their own verbs. Verified: `set` overrides git in the
  resolved identity and in the real Dolt commit author, `--actor` still wins, unknown/`unsettable`
  keys give not_found/validation exits. (The lifted `internal/config` viper subsystem was found
  orphaned/beads-flavored and deliberately not surfaced.)
- **Reads honor the active changeset** — `ls`/`show` reads (and the `Mutate` validation hook's
  existence/ref checks) run on the resolved target branch (`--changeset` → active changeset →
  `main`) via `app.Reader`/`app.ResolveBranch`, since branch state is connection-scoped. So you see
  edits staged in the active changeset, and validation resolves rows created earlier in the same
  changeset. `changeset ls` reads `main` (changeset metadata lives there); `generate` reads `main`
  (the canonical build).

### Review & collaboration (changeset review — Step 1)

- **`cusp comment` / `cusp review` — the review verbs** ([comment.go](../src/cli/cmd/cusp/comment.go),
  [review.go](../src/cli/cmd/cusp/review.go), [store/review.go](../src/cli/internal/store/review.go)). Wires
  the long-modeled [Review layer](entities/review.md) into the CLI so a reviewer (human via the extension,
  or an agent via the shell) can drive a changeset review. **`cusp comment`** — `add`/`ls`/`show`/`resolve`/
  `reopen`/`edit`/`delete`: a comment attaches to a changeset (positional branch or the active one),
  optionally **anchored** to a `--subject TYPE:key` (resolved against the *changeset branch*, since a
  changeset-new entity isn't on `main` yet) with a free-form `--locator`, and threads via `--reply <id>`;
  `requirement`/`spec`/`entity` resolve from a token, `user_story`/`test_case`/`deliverable` take the explicit
  `--subject-type`/`--subject-id` pair. **`cusp review --verdict approve|deny|request_changes [--summary]`**
  (+`review ls`) upserts one verdict per reviewer per changeset (deterministic id → idempotent, merge-safe)
  **and** moves the changeset `status` (`approve→approved`, `request_changes→changes_requested`,
  `deny→denied`); merge/abandon stay with the `changeset` verbs. **Where they run:** the `Comment`/`Review`
  rows must live on `main` (else they'd merge away with the change), so both verbs route through a
  `runMutateOnMain` wrapper that pins `main` regardless of the active changeset — reusing the whole command
  contract (validate → tx+retry → attributed Dolt commit → `--dry-run`). Verified end-to-end against real
  Dolt: a comment on a changeset-new requirement, threaded replies, resolve/`--unresolved`, a verdict upsert
  (one row) with status sync, and — the invariant — the `rev_comment`/`rev_review` rows **surviving the
  changeset's merge** on `main`, with the anchor uuid still resolving. `verdict`/`subject_type` are closed
  enums (unit-tested).
- **Per-entity changeset diff — `cusp changeset diff --entities`** ([app/diff.go](../src/cli/internal/app/diff.go)).
  Grows the diff from today's table-level row counts (`workspace.Diff`) to an **`EntityDiff` list** — per
  changed requirement/spec/user_story/entity/deliverable/test_case: its subject (`type`+`id`+display ref),
  `changeType` (added/modified/removed), and the per-field base→head values — so a review surface can anchor
  a comment to a concrete row and field. Reuses the auto-gen `dolt_diff` plumbing (`quoteRef`), pairs the
  `to_/from_` columns dynamically (no static column list), and resolves labels on the changeset branch so
  changeset-new rows show their `fr_key`. Each row carries a self-describing `docRef` (`spec:<ref>` /
  `entity:<name>`), and a **doc-coverage pass** rolls section/child changes up to their owning doc so a
  prose-only edit still surfaces its doc — `ent_entity_section` + `ent_relationship` (both endpoints) →
  entity, `req_spec_section`/`req_requirement_group`/`req_acceptance_scenario` → spec. The `--json` form is an
  **envelope** `{base, head, entities}` carrying the exact refs to render each side at: `base` = the merge
  base, `head` = the branch while it exists, else the recorded `head_commit`/`merge_commit` (both survive
  branch deletion). So a **merged** changeset stays reviewable, an **abandoned-before-submit** one fails
  cleanly with a coded `not_found`, and the human (non-JSON) output is unchanged.
- **VS Code extension — the review surface (first slice)** ([reviewView.ts](../src/extension/src/changesets/reviewView.ts),
  [changesetTree.ts](../src/extension/src/changesets/changesetTree.ts)). The changeset tree expands to its
  changed entities (`diff`), and reviewing a changeset opens VS Code's **native diff editor** (`vscode.diff`)
  over two virtual documents — the affected **spec or entity** doc rendered as Markdown at the exact **base
  commit** (the merge base) vs the **head** (the live branch, else its recorded commit), via `cusp spec
  render`/`cusp entity render --format md --changeset <ref>` — so the gutters, side-by-side/inline toggle,
  navigation and coloring are VS Code's, not hand-rolled. Rendering at a commit works because `app.Reader`
  falls back to a **read-only revision database** (`USE db/<commit>`) when the ref isn't a live branch (Dolt
  has no detached HEAD), resetting the pooled connection on release. So the diff is exact even if `main`
  advanced after the branch forked, and a merged/abandoned-after-submit changeset still renders from its
  commits. Review
  comments hang off the head side as **native Comments API** `CommentThread`s, anchored to a requirement via
  the rendered `^fr-key` block anchors (a comment binds to a requirement, not a line number), or to the whole
  entity for an entity doc; reply → `addComment`, resolve/reopen → `resolveComment`, and a **Set Verdict**
  command → `setReview`. To feed the diff, `EntityDiff` carries a server-resolved `docRef` — a self-describing
  `spec:<ref>` / `entity:<name>` token — and the client gains `renderDocMarkdown(docRef, branch)` dispatching
  on it. The diff also **rolls a section/child change up to its owning doc** (`ent_entity_section` → entity,
  `req_spec_section`/`req_requirement_group`/`req_acceptance_scenario` → spec), so a prose-only edit still
  opens its doc's diff even though no subject row changed. Fills the transport contract's pending methods
  (`diff`/`listComments`/`addComment`/`resolveComment`/`setReview`) in the transport-agnostic `client.ts` +
  `cliClient.ts`, keeping the UI free of `cusp`-process knowledge (MCP-swappable later). The extension holds
  no state — Dolt stays the source of truth. **Changeset lifecycle actions** complete the in-editor loop: a
  **New Changeset** button (view title → `cusp changeset start`) and per-changeset **Submit / Merge /
  Abandon** context actions (`cusp changeset submit`/`merge`/`abandon`), Merge/Abandon behind a modal
  confirm. Menu visibility is status-gated via the tree-item contextValue — active changesets get the full
  set (review + verdict + lifecycle), **merged** ones are viewable-only (their diff, no lifecycle/verdict),
  and **abandoned** ones are inert (no twistie, no click, no actions).
- **VS Code extension — Test view** ([tests/testsTree.ts](../src/extension/src/tests/testsTree.ts)). A new
  **Test** activity-bar view over the testing layer, with two roots: **Tests** — the imported catalog as a
  self-nesting **suite tree → cases** (each case showing type · status · priority), and **Test Runs** — the
  runs, each expanding to its **results** (the case title + a pass/fail/blocked/skipped status icon). Built
  client-side from `test suite ls` / `test case ls` / `test run ls` / `test result ls` (`--json`); the catalog
  is fetched once and indexed (parent→children, suite→cases, case-id→title so results show titles), run results
  load lazily on expand. Clicking a **case** (or a run's **result**) opens a Qase-style **detail panel**
  ([testCaseView.ts](../src/extension/src/tests/testCaseView.ts)) — title, metadata chips, preconditions, and
  the ordered **steps with the gherkin** (Given/When/Then keywords highlighted), from `test case show` +
  `test case step ls`. Client gains `testSuites`/`testCases`/`testRuns`/`testResults`/`testCase`/`testCaseSteps`.
  Verified against the live-imported Qase catalog (10 domain suites → features → 1287 cases; 9 runs → results;
  gherkin rendered from a real case).

### Planning & testing layers (CRUD)

- **Planning-layer CRUD** ([milestone.go](../src/cli/cmd/cusp/milestone.go), [capability.go](../src/cli/cmd/cusp/capability.go),
  [deliverable.go](../src/cli/cmd/cusp/deliverable.go), [view.go](../src/cli/cmd/cusp/view.go),
  [store/planning_crud.go](../src/cli/internal/store/planning_crud.go)). Hand-editing verbs for the layer that
  was previously write-only via the Notion import: **`cusp milestone`** (add/ls/show/edit/delete, keyed by
  slug), and **`cusp capability`/`deliverable`/`view`** (add/ls/show/edit/delete, keyed by their ULID id, with
  FKs resolved from human keys — `--domain`/`--milestone`/`--spec`). Junctions are managed by **`link`/`unlink`**
  subcommands (`capability link --milestone|--deliverable`, `deliverable link --view|--blocked-by`), reusing
  the importer's `Upsert*`/`Link*`/`Clear*` store helpers so add and edit share one path (add mints
  `ids.New()`; edit loads-then-upserts). Deletes clear junctions (FK cascade) and polymorphic refs
  (`DeleteNodeRefs`). New closed enums: `CapabilityLevel`, `DeliverableSize`/`Status`/`AIReady`.
- **Testing-layer CRUD** ([test.go](../src/cli/cmd/cusp/test.go), [testrun.go](../src/cli/cmd/cusp/testrun.go),
  [store/testing.go](../src/cli/internal/store/testing.go)) — **net-new** (the Qase-modeled test tables had zero
  app code). **`cusp test`** groups: **`suite`** (self-nesting tree), **`case`** (+ `step add/ls/delete`, +
  `cover`/`uncover` linking a case to the requirements it covers via `req_requirement_test_case`), **`run`**
  (+ `config`/`unconfig` attaching configurations), **`result`** (`add`/`ls` — the id is **deterministic** over
  run+case+config, so a re-report converges via `ON DUPLICATE KEY UPDATE`), and **`config`** (group/value
  pairs). Seven new closed enums (`TestLayer`/`Type`/`Severity`/`Automation`, `TestCaseStatus`, `TestRunStatus`,
  `TestResultStatus`); test-case priority reuses the shared 0–4 scheme; steps auto-assign their ordinal.
  Shared command-shape helpers (`simpleList`/`simpleGet`/`simpleMutate`/`simpleDelete`,
  [cmdhelpers.go](../src/cli/cmd/cusp/cmdhelpers.go)) keep each verb a few lines over the same
  connect/`runMutate`/`emit` primitives. Verified end-to-end against real Dolt (nested suites, coverage,
  deterministic-result idempotency, cascade deletes, enum rejection, `--dry-run`). Enum sets are unit-tested.

### Generation & cross-references

- **Generate** — `cusp generate --format md|json|html`: DB → git-ignored read artifacts.
  **md/json/html. Cusp-original** — beads has no generate; it exports JSONL. Core to the "generated,
  never edited" principle. **Renderer architecture:** `Load` assembles the graph once into a
  format-agnostic view `Model` ([model.go](../src/cli/internal/generate/model.go)); a `Renderer` turns it
  into output files. Two families: **document** (Markdown — Obsidian wikilinks+block-refs; HTML —
  same pipeline transformed to relative `.html` links via goldmark) and **data** (JSON — per-doc
  records + an `index.json` manifest, prose keeps raw `[[TYPE:key]]` tokens). Markdown is
  byte-identical across the refactor; adding a format = one `Renderer`. HTML ships a **default
  static-site theme** ([css.go](../src/cli/internal/generate/css.go), [html.go](../src/cli/internal/generate/html.go)):
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
  ([icons.go](../src/cli/internal/generate/icons.go), inline Feather SVGs): type glyphs in the nav/breadcrumb
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
  `planning.json` (`src/cli/internal/generate/model.go` `loadPlanning` + `src/cli/internal/store/planning_read.go`).
  Capabilities render as a **hierarchy** (domain › epic › capability, grouped by domain) — a
  nested list in Markdown, a guide-railed tree in HTML; deliverables group by milestone (with
  their linked capabilities/views/blocked-by) and views group by domain with their **route** and
  a wikilink to the backing spec. The HTML pages are rendered **straight from the Model**
  (`src/cli/internal/generate/planning_html.go`) so enum values become colored **pills** (level, size,
  deliverable status, AI-ready, milestone) and every row/section carries a **type icon**; the
  section appears in the nav/sitemap/breadcrumbs only when planning data exists. Relationships
  resolve to display titles; `notion_url` stays in `planning.json` as source metadata but is not
  rendered as a link.
- **Incremental generation** — auto-regenerate only the **affected** output docs on **every DB
  change** (CLI *or* MCP), in the user-configured subset of formats. Hooked at `app.Mutate`
  ([autogen.go](../src/cli/internal/app/autogen.go)): after a successful commit **to `main`** (changeset
  drafts update on merge, not in flight), it classifies the commit's dirty tables from the **Dolt
  diff** (`dolt_diff(parent, head, table)` — survives `DeleteNodeRefs`, so it's correct for deletes).
  **Fast path:** a change confined to a spec/entity's content (section, story, scenario, group, or a
  requirement *statement* edit — fr_key unchanged) re-renders **only that document**, via targeted
  loaders `LoadDocs`/`loadSpecDoc`/`loadEntityDoc` ([model.go](../src/cli/internal/generate/model.go)) — no
  full `Load`. **Full path:** a structural change (spec/entity/domain/term/section-type or fr_key
  added/removed/renamed) can move indexes or the inline-ref target set, so it rebuilds the whole
  graph. **Either way** `Sync` ([incremental.go](../src/cli/internal/generate/incremental.go)) writes only
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

- **Import pipeline + `tutor` source adapter** — a source-agnostic staging core (`src/cli/internal/importer`:
  a `Graph` of Cusp entity shapes + a drift/ER-gap `Report` + an idempotent `Apply` writer keyed on
  business identifiers) and the deterministic, **no-LLM** `tutor` adapter (`src/cli/internal/importer/tutor`).
  `cusp import tutor <docs>` parses **Domain/Spec/Requirement/UserStory/AcceptanceScenario/Edge/
  Milestone/Entity** and reports counts + coverage + drift; `--apply` loads the graph
  through `app.Mutate` (one changeset/`main` commit), idempotent on re-run. Validated against the
  real corpus (~2.2k FRs, 123 specs, 23 entities). The five ER refinements it
  surfaced are resolved in [decisions.md](entities/decisions.md). The structured authorization
  layer (`Privilege`/`AccessRule`) was **removed** — a tutor-specific, never-consumed paradigm;
  access rules stay as entity-doc prose (migration `0012`, [decisions.md](entities/decisions.md)).
- **Notion source adapter (planning layer)** — `cusp import notion` ingests the Notion
  Capabilities / Deliverables / Views databases into the [planning layer](entities/planning.md)
  (`src/cli/internal/importer/notion`). Maps each page → `Capability` (3-tier `level`, self-nested via
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
- **Qase source adapter (testing layer)** — `cusp import qase` ingests a Qase test-management project
  into the [testing layer](entities/testing.md) (`src/cli/internal/importer/qase`). Maps Qase **suites →
  `TestSuite`** (self-nesting tree), **cases → `TestCase`** (+ inline **`TestStep`s**, + any
  `PREFIX-FR-NNN` citations found in the case text/tags/custom-fields → **`req_requirement_test_case`**
  coverage against *existing* requirements), **configuration groups → `Configuration`**, **runs →
  `TestRun`** (+ `test_run_configuration`), and **results → `TestResult`**. Because the `test_*` `Add*`
  funcs mint fresh ULIDs, the import path uses new **deterministic `Upsert*` variants** (row id =
  `ids.Rel` over the Qase id) so re-import converges (verified: stable row counts on a second `--apply`).
  Qase encodes case enums as integers; each enum field decodes as **int _or_ string** and normalizes
  string-first, so the offline path uses readable values and the integer maps (`qase.enumMaps`,
  best-effort) are the API fallback. Source is the **Qase API v1** (`--token`/`$CUSP_QASE_TOKEN` /
  `$QASE_API_TOKEN` + `--project`, `Token` header, paginated) or saved API responses (`--from <dir>`:
  `suites.json`/`cases.json`/`configurations.json`/`runs.json`/`results.json`) for an offline/testable
  path. Read-only report by default; `--apply` rides `importer.Apply` + `app.Mutate` (one transaction,
  one Dolt commit). The pure mapping (enum normalization, FR extraction, suite tree) is unit-tested.
  **Verified against a real Qase project via the live API** — a 1291-case / 59-suite / 7-run / 225-result
  project imported in one ~8s commit (7 suite-less cases correctly skipped), with **1427 FR-tag citations
  resolving to real requirements** (18 unmatched skipped) and a re-`--apply` leaving row counts identical
  (idempotent). Real-API shape notes baked in: Qase tags are objects (`{title}`, where the FR ids live),
  results come from the project-level `/result/{code}` list (per-run paths 404), the duration field is
  `time_spent_ms`, and the case enum integers (`type 7`→acceptance, `status 0`→active, `severity 4`→normal,
  `automation 0`→manual) validated against live data. Reuses the staging core alongside the `tutor`/`notion`
  adapters.

### Remote sync

- **Remote sync — push/pull/remote/fetch + sync.** `cusp dolt remote add/remove/ls`,
  `cusp dolt push/pull/fetch [remote] [branch]` (default origin/main, `--force`, `--user` +
  `DOLT_REMOTE_PASSWORD`), and `cusp sync` (pull-then-push the canonical branch). Orchestration in
  [app/remote.go](../src/cli/internal/app/remote.go) pins one connection (branch state is connection-scoped;
  a pull's merge needs one session — runs through `MergeAndSettle`) over the lifted
  `versioncontrolops` primitives; a pull/sync that advanced the branch **refreshes the generated
  artifacts** if auto-gen is enabled (a merge bypasses the `Mutate` hook). Verified end-to-end
  against a `file://` remote: add/ls/push/fetch, a divergent commit pulled back + merged, and a local
  edit round-tripped via `sync`.
- **Clone a workspace from a remote — `cusp dolt clone <url> [dir]`**
  ([clone.go](../src/cli/cmd/cusp/clone.go)). The bootstrap counterpart to `cusp init`: makes `.cusp/`,
  starts the managed server, `DOLT_CLONE`s the remote into the standard database, and writes metadata
  + `.gitignore` (the scaffold shared with `init`). No existing `.cusp` needed — with `[dir]` it
  clones into (and creates) that directory, resolving the workspace to the enclosing git repo's root
  or `git init`-ing a fresh one (shelling out with an explicit dir, since the git package caches its
  context and wouldn't see a just-created repo); `--force` tears down and re-clones. The clone's
  `origin` remote is set up automatically, so `cusp dolt pull`/`push` work immediately. Verified
  end-to-end against a `file://` remote: clone lands 41 tables with data + external refs intact,
  refuse-if-exists / `--force`, `pull` from the auto-wired origin, and nested-dir → repo-root
  resolution.

### Server, branches & DB maintenance

- **Dolt server lifecycle — `cusp dolt start/stop/status`** ([dolt.go](../src/cli/cmd/cusp/dolt.go)).
  `start` starts (or adopts, idempotently) the managed owned server; `stop` gracefully shuts it
  down (flush → SIGTERM → SIGKILL, `--force` to skip the wait); `status` reports mode + pid/port/
  data-dir/logs (text or `--json`). `start`/`stop` refuse when the workspace targets an external
  server (`--dsn`/`$CUSP_DSN` or an explicit port). Closes the gap where owned mode could auto-start
  a server but offered no clean stop — wraps the lifted `doltserver.Start/StopWithForce/IsRunning`.
- **Raw branch escape hatch — `cusp branch ls/create/delete/checkout`**
  ([branch.go](../src/cli/cmd/cusp/branch.go), [app/branch.go](../src/cli/internal/app/branch.go)). The low-level
  view beneath the changeset model: `ls` lists Dolt branches (marks the active target, tags
  `changeset/*`), `create` branches off the active target, `delete` removes one (refuses `main` and
  the active target), `checkout` retargets the ambient read/write branch (reusing the active-changeset
  pointer; `main` clears it). For the tracked PR workflow use `cusp changeset`; these are for
  diagnostics and manual surgery.
- **History maintenance — `cusp flatten` + `cusp dolt compact`**
  ([flatten.go](../src/cli/cmd/cusp/flatten.go), [app/maintenance.go](../src/cli/internal/app/maintenance.go)).
  `flatten` squashes all Dolt history into one snapshot (irreversible; `--force`/`--dry-run`);
  `dolt compact --days N` squashes commits older than the window into one base, cherry-picking the
  recent ones on top (`--force`/`--dry-run`), driving the lifted `versioncontrolops.Compact` from the
  commit log. Both GC afterward (best-effort). Verified end-to-end against real Dolt: a 7-commit
  history collapses to root + 1 snapshot with all `req_domain` data intact.
- **Standalone GC — `cusp dolt gc`** ([dolt.go](../src/cli/cmd/cusp/dolt.go),
  [app/maintenance.go](../src/cli/internal/app/maintenance.go)). Reclaims disk from unreferenced Dolt
  chunks — those orphaned by deleted branches, merged/abandoned changesets, or history compaction —
  without changing any commit history (unlike `flatten`/`compact`, which squash *and* GC). Runs
  `DOLT_GC()` on a pinned non-transactional connection (`app.GarbageCollect` over
  `versioncontrolops.DoltGC`). Verified against real Dolt: GC succeeds, writes continue afterward.

### Query & inspect

- **Query / inspect — `sql`/`stats`/`search`/`log`** ([query.go](../src/cli/cmd/cusp/query.go)).
  **`cusp sql <query>`** — read-only passthrough (SELECT/SHOW/DESCRIBE/EXPLAIN/WITH only; writes
  rejected with exit 2, so the attributed write path is never bypassed — invariant #3),
  dynamic-column table output or `--json` array, honors the active changeset, and reaches the
  `dolt_*` system tables (history/diff/blame) and Dolt `AS OF` time-travel for free. **`cusp stats`**
  — counts per layer. **`cusp search <text>`** — LIKE across domain names / spec titles /
  requirement fr_keys+statements / entity names+descriptions / glossary terms (`store.Search`).
  **`cusp log`** — recent commits from `dolt_log`. A shared dynamic row-printer (`writeRows`) backs
  them.
- **JSONL export — `cusp export`** ([export.go](../src/cli/cmd/cusp/export.go),
  [app/export.go](../src/cli/internal/app/export.go)). Dumps every data table as JSON Lines — one
  `{"table":…,"row":{col:value|null}}` object per line — for backups and interop alongside Dolt
  history. Tables go in name order and rows in a total order over all their columns (deterministic
  even for composite-key junctions), so re-exporting the same data is byte-for-byte identical and
  git-diffable. Values are string-or-null (types recover from the schema); the `schema_migrations`
  bookkeeping cursor is excluded. Reads the active changeset (else `main`); writes to stdout (summary
  to stderr, so pipes stay clean) or `--out <file>`. The core is a reusable `app.Export(ctx, r, w)`.
  Verified against real Dolt: valid JSONL, byte-identical re-exports, nulls preserved.

### Validation & analysis

- **`cusp check` (first slice)** — read-only whole-graph integrity: scans every prose field
  (`store.ListProseFields`) for inline `[[TYPE:key]]` tokens that don't resolve (`app.Check`), audits
  each acyclic edge kind for cycles (`findCycle`), honors the active changeset, exits nonzero on
  findings (gates CI/agents).
- **`cusp impact <ref>`** — graph traversal around an entity (TYPE:key): inbound (what references /
  points an edge at it = what's affected if it changes) and outbound (what it relies on), over
  entity_refs + edges + **entity↔entity relationships** (`ent_relationship`, for entity subjects —
  labelled `relationship (<cardinality>)`), with `--transitive` for the reverse-edge blast radius
  (`app.Impact` + `store.ListAllEdges`/`ListEntityRefsFor`/`ListEntityRelationships`); honors the
  active changeset.
- **`cusp coverage`** ([coverage.go](../src/cli/cmd/cusp/coverage.go),
  [app/coverage.go](../src/cli/internal/app/coverage.go), [store/coverage.go](../src/cli/internal/store/coverage.go)).
  Requirement→test-case traceability: how many requirements have a linked test case (via
  `req_requirement_test_case`, e.g. from `cusp import qase`), rolled up overall and per spec, plus two
  actionable gap lists — **orphan FRs** (no test case) and **delivery-status drift** (a requirement
  whose `delivery_status` counts as covered yet has no test case behind it — claims done, isn't
  tested). Read-only, honors the active changeset; `--orphans` for just the untested FRs, `--json`
  for the structured report.

### Agent integration (CLI-first)

The primary agent path — a skill + instructions + a SessionStart hook that injects live workspace
state — for **Claude Code** and **Codex**. CLI-first (no MCP server required): the whole surface
rides the existing `cusp` commands.

- **`cusp prime`** ([prime.go](../src/cli/cmd/cusp/prime.go),
  [app/prime.go](../src/cli/internal/app/prime.go)). Emits the context an agent needs at session
  start: live state (active/open changesets, unresolved review comments, `cusp check` integrity,
  headline counts — `app.GatherPrime`, best-effort so a partial snapshot never fails the hook) then a
  compact workflow reference (invariants + the changeset PR loop + read commands). `--hook-json` wraps
  it in the SessionStart envelope `{"hookSpecificOutput":{"hookEventName":"SessionStart",
  "additionalContext":…}}`; `--json` emits the structured state. **Silent no-op outside a Cusp
  workspace** (safe as a universal hook); a repo-local `.cusp/PRIME.md` overrides the payload
  verbatim; degrades to the static reference if the DB is unreachable.
- **`cusp setup [claude|codex]`** ([setup.go](../src/cli/cmd/cusp/setup.go),
  [setup_codex.go](../src/cli/cmd/cusp/setup_codex.go), [internal/agentsetup](../src/cli/internal/agentsetup)).
  Installs, idempotently, per host: the **skill** (`SKILL.md` with trigger frontmatter), an
  **instruction section** injected into `CLAUDE.md`/`AGENTS.md` via marker-delimited managed blocks
  (`<!-- BEGIN CUSP INTEGRATION v:N hash:… -->`, content-hash freshness, append-if-absent /
  replace-if-present, symlink guard — unit-tested in `agentsetup`), and the **SessionStart hook**.
  Claude Code: `.claude/settings.json` hook running `cusp prime --hook-json` (check-before-add +
  legacy-variant sweep). Codex: `[features] hooks = true` in `.codex/config.toml` + `.codex/hooks.json`
  routing four events (SessionStart / UserPromptSubmit / PreCompact / PostCompact) through a hidden
  **`cusp codex-hook <event>`** shim ([codex_hook.go](../src/cli/cmd/cusp/codex_hook.go)) that emits
  the Codex envelope and re-primes after a compaction (PostCompact drops a marker, the next
  UserPromptSubmit injects once and clears it). Flags: `--list`, `--print`, `--check`
  (present/stale/missing), `--remove`, `--global` vs project. Verified end-to-end: install into a repo
  with existing `CLAUDE.md` (user content preserved), idempotent re-runs, stale→updated on template
  drift, `--check`/`--remove`, the hook payload envelope, the codex-hook shim across all four events +
  the marker dance, and the no-workspace passthrough.

### Testing & CI (first slice)

- **Integration harness — `internal/testutil`** ([testutil/workspace.go](../src/cli/internal/testutil/workspace.go)).
  `NewWorkspace(t)` spins up an isolated Cusp workspace on a **real owned Dolt server** in a temp dir
  (own ephemeral port), initialized through the same `app.InitWorkspace` path the `cusp init` command
  uses, with `t.Cleanup` stopping the server. `RequireDolt(t)` skips when the `dolt` binary is absent
  or under `-short`. Enabled by two refactors that also improve the code: **`app.InitWorkspace`**
  (init's DB setup extracted from the cobra command — server, schema, seed, initial commit, metadata)
  and **`workspace.ConnectAt(cuspDir)`** (connect to an explicit workspace dir, not just the cwd).
- **Contract integration suite — `internal/integration`**
  ([contract_test.go](../src/cli/internal/integration/contract_test.go)). Exercises the guarantees only
  a real DB can prove, driving the same `app.Mutate` / `store` / `versioncontrolops` primitives the CLI
  uses: `init` leaves a clean working set + one commit; each add is **exactly one commit** with a clean
  working set and correct actor attribution; **validation rejects before any write**; dry-run rolls
  back; **reads honor the changeset branch** (edits don't leak to `main`); the **changeset round-trip**
  (`start → add on branch → merge`) lands on `main` with the **review comment (written to `main`)
  surviving the merge**; and deterministic upsert is idempotent.
- **Unit tests** — actor/identity precedence (`ResolveActor` override + `$CUSP_ACTOR`; identity-file
  round-trip), `prime` state rendering, and changeset-slug generation; alongside the existing
  `ids`/`enums`/`app`/`agentsetup`/`refs` units.
- **CLI golden-path smoke test** ([cli_smoke_test.go](../src/cli/cmd/cusp/cli_smoke_test.go)) — drives
  the real `rootCmd` in-process (init → domain add → changeset start/add/diff `--json`/merge → stats
  `--json` → a not-found error case), asserting exit codes and the `--json` / `{"error":…}` envelope.
  It covers the cobra wiring, flag parsing, `emit`, and exit-code mapping the app/store tests skip.
  Enabled by making **`Execute()` return the exit code** (main does `os.Exit`), so it's callable
  in-process. A `runCLI` helper resets the global flags and captures `os.Stdout`.
- **Coverage ratchet + command tracker** ([scripts/coverage.sh](../scripts/coverage.sh), `make
  cover`/`cover-check`/`cover-commands`/`cover-html`) — coverage over the packages Cusp *authors*
  (excluding the salvaged Dolt infra), measured from the full dolt-backed suite. `make cover-check`
  fails below a floor (raised in a commit as coverage grows — a monotonic ratchet), wired into the CI
  integration job. `make cover-commands` prints statement-weighted coverage per `cmd/cusp` command
  file — the "which commands have tests" view — which the CLI smoke test lifted for the golden-path
  commands (init/changeset/domain/query/root).
- **Coverage push — owned total 14.1% → 85.1%; every owned package ≥70%.** First the logic/pure
  packages: `internal/store` 5.6%→**82%**, `internal/app` 18%→**77%**, `internal/generate` 0%→**91%**,
  `internal/importer` 0.2%→**87%** (+ `qase` **94%**, `notion` **97%**, `tutor` 0%→**96%**),
  `internal/workspace` 59%→**82%**, `internal/refs`→**92%** — via the real-Dolt harness for the
  DB-backed paths and pure unit/fixture tests elsewhere. Then the **command layer**: `cmd/cusp`
  10.8%→**74%** via `cli_*_test.go` groups that drive the real `rootCmd` in-process through the shared
  `newCLIWorkspace`/`runCLI` harness (structure, graph, planning, testing, review, config/setup,
  maintenance/import), asserting exit codes + `--json`. The harness resets the *whole* command tree's
  flags between calls, since cobra retains per-command `Changed()` state and bound vars across
  in-process `Execute()`s. The ratchet floor is raised to **84%** (headroom below the ~85% measured,
  for run-to-run variance).
- **Bug found + fixed by the coverage work — `cusp comment ls --subject <ref>` returned "no
  comments".** Writing the review CLI tests surfaced it: `app.Reader`'s `release` restored the pooled
  connection only after a revision-database read, not after a live-branch checkout — so resolving the
  `--subject` on the changeset branch left the pooled connection checked out there, and the follow-on
  `ws.DB()` read of `rev_comment` landed on the changeset branch instead of `main` (where comments
  live), reporting none. Fixed by always restoring `main` in `release`; regression test added.
- **CI** ([.github/workflows/ci.yml](../.github/workflows/ci.yml)) — a fast **unit** job (gofmt +
  build + vet + `go test -short`, no database) that runs always, and an **integration** job that
  installs the `dolt` binary, runs the full suite, and enforces the coverage ratchet; plus the
  existing release dry-run.

### Distribution & self-update

- **`cusp version`** ([version.go](../src/cli/cmd/cusp/version.go)) — reports version / commit / build date /
  Go toolchain / os·arch (human or `--json`). Build metadata is injected at release time via ldflags
  (GoReleaser / Makefile) and falls back to `runtime/debug.ReadBuildInfo()` for `go install` builds.
  See [build-and-release.md](build-and-release.md).
- **`cusp upgrade [version]`** — self-update in place ([upgrade.go](../src/cli/cmd/cusp/upgrade.go) +
  `src/cli/internal/selfupdate`). Resolves the latest GitHub release (or a pinned tag), downloads the archive
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
  end-to-end: command/binary `cusp`, the `src/cli/cmd/cusp` entrypoint, the `.cusp/` config/workspace dir,
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
