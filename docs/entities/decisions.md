# Decisions

[← index](index.md)

## Open questions

_None — all resolved (see below)._

## Resolved decisions

- Directory tree derives from `Spec.path` (no `Folder` table).
- `Test` is first-class, expanded into a Qase-style layer.
- Acceptance scenarios stay as structured Given/When/Then.
- **IDs**: ULID surrogate PKs + `UNIQUE` business keys + tiered display IDs — see
  [Identifiers & keys](identifiers.md).
- **Pure-relationship PKs are deterministic, not ULID**: junctions use a composite PK, and
  `Edge` / `TestResult` derive their surrogate `id` (`uuidv5`) from their `UNIQUE` identity —
  so a relationship two branches create independently converges on merge instead of tripping
  a unique-key violation. (Adopts the beads dependency-table technique; see
  [Identifiers & keys](identifiers.md).)
- **Testing**: runs/results are CLI-authored or CLI-imported; `Configuration` is included;
  Qase `Plan` / `SharedStep` are omitted.
- **Entities** are authored business-domain documents — `EntityAttribute` is a domain
  property (meaning), not a schema-column mirror; no sync from a DB schema.
- **`ExternalRef` subjects**: `Deliverable`, `Requirement`, `TestResult`. `system` stays a
  free string with a documented set (`jira`/`rally`/`beads`/`linear`/`github`/…).
- **Review & collaboration layer added** — `Changeset` + `Review` + `Comment` +
  `Actor`; the changeset carries Dolt branch/commit coordinates as the bridge. See
  [review.md](review.md).
- **History & diff stay Dolt-native** — no revision/audit tables. Spec/requirement history
  and agent-change diffs come from `dolt_history_*` / `dolt_diff_*` / `dolt_log` /
  `dolt_blame_*`.
- **Document sections are captured so a regenerate is information-complete** (migration `0003`,
  simplified by `0010`). Goal: `asdf generate` must reproduce the *information* of the source docs
  (format may differ). **All prose sections live in [`DocSection`](structure.md#docsection)** — a
  recognized template section (feature-spec `overview`/`edge_cases`/`success_criteria`/`platform_scope`/
  `assumptions`/`clarifications`/`preamble`/`more_info`; entity `purpose`/`key_concepts`/…) carries a
  normalized `section_key`; a bespoke one carries `section_key = NULL`. The per-story
  `why_priority`/`independent_test` and the FR group are still **promoted** from prose. The FR group is a
  first-class [`RequirementGroup`](requirements.md#requirementgroup) (migration `0004`, replacing the
  earlier denormalized `Requirement.section`): it carries the group's `position`, `header`, and the
  interspersed `note`, and requirements link to it by document `position` — so the FR list regenerates in
  **source order**, with grouping and notes intact. **Key Entities** lists become `spec → entity`
  references. `Spec.heading` (the H1 identity line) and `Entity.description` (the glossary one-liner)
  stay columns — they are identity, not sections. Generation emits a fixed canonical section order —
  order/format may differ from the source, information does not. `EntityAttribute`/`EntityRelationship`
  stay the optional finer structured extraction over the same prose, not a second source of truth
  (`row_level_access` has no structured table — see the authz-removal decision below).
- **Migration `0010` collapses the per-section typed columns into `DocSection`** — the original `0003`
  modeled recurring sections as ~17 typed `TEXT` columns on `Spec`/`Entity`, which **duplicated** the
  generic `DocSection` model and baked one corpus's doc template into the **core** schema (against the
  [CLAUDE.md](../../CLAUDE.md) "keep the core generic" invariant). `0010` drops those columns and adds a
  nullable `DocSection.section_key` so every section is one model (keyed or bespoke). Output is **unchanged**
  (the generator looks recognized sections up by key at the same canonical positions); it's a storage
  simplification. Like the `0004` `Requirement.section`→`RequirementGroup` move, it is DDL-only — section
  content is import-only (`AddSpec` writes none), so a re-import repopulates.
- **ER refinements from the tutor import** (validated by `asdf import tutor`, the read-only
  parse-and-report adapter — `internal/importer/tutor`). Five pieces of source data had no clean
  home; the resolutions keep the core generic:
  - **`Domain.description`** — added (migration `0002`). The domain index gives every domain a
    one-line summary; generic, so it belongs in core.
  - **`Spec.status` gains `reviewed`** — the corpus uses `Draft | Reviewed | Active`; `reviewed`
    (under review, before active) had no home in `draft|active|obsolete`. Spec-only — a
    requirement's `content_status` stays `draft|active|obsolete`. VARCHAR enum, app-enforced; no DDL.
  - **`e2e_ref` is test linkage, not a Requirement column** — a registry FR may cite an e2e test
    file. Adding `Requirement.e2e_ref` would bake in a tutor-/tooling-ism. Proper home is
    `TestCase.path` + the `Requirement`↔`TestCase` junction, once a test source is imported; until
    then the import carries it in `Requirement.notes`. No schema change.
  - **A milestone that is a beads issue id maps to `ExternalRef`, not `Milestone`** — a few
    registry entries put a `tut-xxxx` beads id in the `milestone` field. That is an interop
    reference: `ExternalRef{subject_type: requirement, system: beads, external_id: tut-xxxx}`. No
    schema change (`ExternalRef` already covers it).
  - **`backlog` is a valid milestone value** — `Milestone.abbreviation` is an open string with
    seed examples (`M0`–`M7`, `Future`); `backlog` joins them as seed/policy data, not a new column.
- **Changeset is the unit of batched, reviewable change** (the `Changeset` entity, formerly
  `ChangeProposal`). A changeset is a **Dolt branch** that bundles edits across many entities
  so they are diffed and reviewed together (a PR over the knowledge graph), not committed
  per-edit. CLI: `asdf changeset start|diff|submit|merge|abandon`; mutating commands take an
  optional `--changeset <name>` (and honor an ambient active changeset set by `start`). With
  **no changeset, a write commits straight to `main`** (auto-commit default). Lifecycle via
  `status`: `draft` (building) → `open` (submitted for review) → `approved`/`merged`. The diff
  is computed from Dolt (`base_commit`→`head_commit`), never duplicated into tables; the
  `Changeset`/`Review`/`Comment` rows live on `main`, not inside the changeset branch. This
  deliberately differs from beads (which auto-commits each operation).
- **Cross-references are inline `[[TYPE:key]]` links, dual-formed into [`EntityRef`](requirements.md#entityref)**
  (the Cross-references feature; see [ROADMAP.md](../ROADMAP.md)). An author/agent writes a
  canonical token in any text field — `[[REQ:ATT-FR-012]]`, optional display
  `[[REQ:ATT-FR-012|the attendance rule]]` — where `TYPE:key` resolves over the existing business
  keys: `DOMAIN`(abbreviation) · `SPEC`(prefix, else path) · `REQ`(fr_key) · `ENTITY`(name) ·
  `MILESTONE`(abbreviation) · `TERM`(glossary — deferred until `GlossaryTerm` lands). At ingestion the
  token is **validated, kept verbatim in the text** (for rendering), **and projected into a queryable
  `entity_ref` row** (owner→target, `kind=references`). The token stays the source of truth; `EntityRef`
  is its reconciled projection (re-derived per owner on each write — so removing a link removes its row).
  - **`EntityRef` vs `Edge`**: `EntityRef` is the home for **all prose-derived references** — the
    inline `[[…]]` links *and* the importer's auto-discovered mentions (inline FR citations, Key
    Entities lists). `Edge` becomes **hand-authored / structured-only** (`refines`/`depends_on`/… asserted
    via `edge add`). `impact`/`check` read **both**. (This reclassifies the importer's former ~600
    prose-derived `references` edges into `entity_ref`.)
  - **Source attribution** is the nearest first-class node owning the text: a link inside a scenario,
    `DocSection`, or `RequirementGroup.note` attributes to its owning user_story / spec. Self-references
    are ignored (no self-ref row).
  - **Dangling refs** (token that resolves to nothing): **block** an interactive CLI/MCP write
    (`--force` overrides); **record a non-blocking finding** on bulk import; a `check` finding once
    `asdf check` exists.
  - **Render is per-format**: `generate` rewrites each token to an **Obsidian-compatible relative
    Markdown link** `[label](relpath#anchor)` (and an **HTML `<a>`** once an HTML generate path exists —
    deferred). The generator emits a stable anchor per FR/heading so `[[REQ:…]]` targets the FR inside its
    spec file. A target with no generated page (e.g. a milestone) renders label-only but still records the
    `entity_ref`. The importer **converts** the corpus's `[label](./other.md)` cross-spec links to canonical
    tokens so they round-trip as links.
- **Glossary is shared project vocabulary, separate from the Entity layer** (the
  [`GlossaryTerm`](glossary.md) + [`GlossaryAlias`](glossary.md) entities). A `GlossaryTerm` defines a
  *concept* once (`make-up-credit` → "minutes of lesson time owed…"); an `Entity` models a *document*
  (Student, Invoice). A term carries a `slug` (the stable `[[TERM:slug]]` link key), a display `term`,
  a `definition` (prose that may itself contain `[[…]]` links — so a term is also an `EntityRef` owner),
  an optional `domain_id` (scoping; null = global, **not** a studio tenant concept), and `status`.
  **Aliases are a child table** (`GlossaryAlias`, `UNIQUE(alias)`) so several surface forms resolve to one
  term; the resolver tries the slug first, then aliases. A term is a first-class
  [cross-reference](requirements.md#entityref) target (`[[TERM:slug]]`, deferred until now) and a generated
  artifact (the `glossary.md` page, one anchor per term). Authored via the CLI (`asdf term …`), not imported
  from the tutor corpus (no structured glossary source there).
- **Physical table names are prefixed by layer (migration `0011`).** MySQL/Dolt has no
  schema-within-database namespace (`CREATE SCHEMA` == `CREATE DATABASE`, a separate version-control unit),
  and ASDF must keep all tables in one database so a changeset is one atomic branch over the whole graph.
  So tables are grouped by **name prefix** instead: `req_*` (structure + requirements + glossary),
  `ent_*` (entity layer), `test_*` (testing), `plan_*` (planning, incl. `plan_milestone` /
  `plan_delivery_status`), `pub_*` (interop / external refs), `rev_*` (review). Tables already named
  `test_*` keep their name; `schema_migrations` (the migration runner's cursor) is left unprefixed. This is
  the **logical entity name** (`Spec`, `Requirement`, …) staying as-is in this data model — only the
  physical `CREATE TABLE` names carry the prefix. Pure rename; no data or generated-output change.
- **The structured authorization layer was removed (migration `0012`).** Earlier revisions modeled
  row-level access as `Privilege` (`(resource, scope, action)` triple) + `AccessRule` (entity↔privilege
  binding with a `condition`). This baked **one corpus's CASL-style authorization paradigm** — row-level,
  triple-based, "never role names" — into the core schema (against the [CLAUDE.md](../../CLAUDE.md)
  "keep the core generic" invariant), and it was **never consumed**: nothing read `AccessRule`, and
  `Privilege` was write-only (the tutor importer filled it; no command or generator read it). The access
  content humans see is rendered from the `row_level_access` [`DocSection`](structure.md#docsection) prose,
  so dropping both tables leaves **generated output byte-identical**. The tutor importer's privilege parser
  and the `Privilege.scope`/`action` seed enums were removed with it. A *generic*, consumer-driven
  authorization concept can return when there is a real reader for it — see [ROADMAP](../ROADMAP.md).
- **Enum policy: three buckets — closed / seed / table** (the schema stores every enum as `VARCHAR`, so this is
  a *validation-policy* decision, not a storage one; see [enums.md](enums.md)). The import-derived growth had
  baked one corpus's value sets into the core; this draws the line:
  - **closed** — fixed lifecycle/workflow + structural discriminators (`*.status`, `Edge.kind`, the polymorphic
    `*_type`, `cardinality`, `verdict`, …). Validated hard; unknown rejected (`enums.Valid`).
  - **seed** — open value-sets with documented defaults that are project-/tenant-/tooling-specific:
    `Domain.kind`, `Spec.kind`, `UserStory.priority` (widened to P1–P5; the corpus already holds P4),
    `Requirement.optout_marker` (corpus carries none; the FR-marker parser is forward-looking),
    and the Qase `TestCase.*` taxonomies. Validated **leniently** — `app.ValidateEnumSoft` accepts an unknown
    value with a warning; `--strict` restores hard rejection. Import never rejects (it warns), matching the
    existing `unknown-delivery-status` finding.
  - **table** — `Requirement.delivery_status`, the one value-set that carries policy, graduates to a
    [`delivery_status`](requirements.md#deliverystatus) lookup table (seeded in migration `0009`). It is keyed
    by its **business value** (not a ULID — a deliberate exception, like a classic reference table) and
    referenced by `requirement.delivery_status` **with no FK**, on purpose: the lookup is *soft* so drift is
    tolerated and surfaced by `check`, never rejected at write (record this rationale — a reviewer who sees the
    schema's ~43 FKs should not "fix" it by adding the constraint). The coverage policy (e2e/shared/milestone
    requirements) moves from prose into the table's boolean columns, enforced by the future `asdf check`.
