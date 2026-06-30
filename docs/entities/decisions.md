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
  simplified by `0010`, restructured by `0013`). Goal: `cusp generate` must reproduce the *information*
  of the source docs (format may differ). A spec's prose sections are
  [`SpecSection`](structure.md#specsection) rows, each addressed by a curated
  [`SpecSectionType`](structure.md#specsectiontype) (title/level/canonical position live on the type);
  entity docs mirror this with [`EntitySection`](authorization.md#entitysection) /
  [`EntitySectionType`](authorization.md#entitysectiontype). There are **no free-form sections** — a
  heading outside the curated vocabulary folds into the `notes` type on import. The per-story
  `why_priority`/`independent_test` and the FR group are still **promoted** from prose. The FR group is a
  first-class [`RequirementGroup`](requirements.md#requirementgroup) (migration `0004`, replacing the
  earlier denormalized `Requirement.section`): it carries the group's `position`, `title`, and the
  interspersed `notes`, and requirements link to it by document `position` — so the FR list regenerates in
  **source order**, with grouping and notes intact. **Key Entities** lists become `spec → entity`
  references. The spec's H1 renders from `Spec.title`; `Entity.description` (the glossary one-liner)
  stays a column — these are identity, not sections. Generation emits a fixed canonical section order
  (`SpecSectionType.position`) — order/format may differ from the source, information does not.
  `EntityAttribute`/`EntityRelationship` stay the optional finer structured extraction over the same
  prose, not a second source of truth (`access_control` has no structured table — see the authz-removal
  decision below).
- **Migration `0013` makes the section vocabulary curated and typed** (superseding the `0003`→`0010`
  arc). `0003` modeled recurring sections as ~17 typed `TEXT` columns; `0010` collapsed them into one
  polymorphic `DocSection` with a nullable `section_key` (recognized) / `NULL` (bespoke). Both let a
  document carry **unbounded one-off sections**, and `0010` still baked the corpus template into the
  generator (every heading/level/order hardcoded). `0013` replaces `DocSection` with **four typed
  tables** — `req_spec_section_type` / `req_spec_section` and the `ent_*` mirror: a curated
  **section-type lookup** (the vocabulary agents *select from* — a built-in `origin = builtin` seed,
  extensible only at a deliberate cost: a separate `section-type add` CLI call, never an inline flag)
  plus a **section instance** that must reference a type (`NOT NULL`; no bespoke). Title, level, and
  canonical `position` live on the type, so render order is **uniform across all docs**. **The
  byte-identical tutor round-trip is retired as the acceptance gate** — documents now *conform to* the
  curated vocabulary (unrecognized headings fold into `notes`); the new gate is "every imported section
  resolves to a seeded type." Two structural blocks (User Scenarios & Testing, Requirements) stay in the
  generator and own the `edge_cases`/`more_info` types — the only schema↔renderer coupling. Like
  `0004`/`0010` it is DDL-only + re-import (section content is import-only; the down-migration restores
  table shape, not git-ignored content). This intentionally re-splits what `0010` unified: `DocSection`
  has exactly two stable, first-class owners (spec, entity), so typed FK tables buy referential
  integrity that the polymorphic table could not, at the cost of mirrored structure.
- **`Spec.path` is domain-relative** (migration `0017`). The domain previously appeared twice — as
  `domain_id` and as the leading segment of `path` — which could drift (rename a domain and the prefix
  goes stale). `path` now stores the location **relative to its domain**; the full docs path is
  reconstructed as `domain.slug + "/" + path` by the generator and the ref resolver, and the top-level
  output directory therefore *always* equals the domain. The UNIQUE key moves from `(path)` to
  `(domain_id, path)` (a domain-relative path like `index.md` isn't globally unique). A spec whose source
  file sat under a directory other than its tagged domain (the corpus had 4) now renders under its
  domain — the classification wins. DDL + re-import (paths repopulate in relative form).
- **The doc path is reconstructed, never stored whole** — neither the domain (above) nor the filename
  is duplicated in `path`. `req_spec.path` holds only the **directory**; the file is `slug.md`, so the
  full location is `domain.slug + "/" + [path + "/"] + slug + ".md"` and the identity is
  `(domain_id, path, slug)`. `ent_entity` mirrors this but is domain-less and derives its filename from
  `name`: the doc lives at `entities/ + [path + "/"] + kebab(name) + ".md"`, where `kebab` lower-cases and
  dashes CamelCase/space boundaries (`EventAttendance` → `event-attendance`); `ent_entity.path` is just an
  optional sub-directory (NULL = directly under `entities/`, which is every entity in the corpus). The
  CamelCase→kebab step can't be expressed in SQL, so entity paths are reconstructed in Go (`store.EntityDocPath`).
- **`position` and `title` are the standard column names** (migration `0019`). Two columns meant
  *document order* under different names — `ordinal` on `UserStory`/`AcceptanceScenario`/`TestStep`,
  `position` everywhere else; standardized on `position`. Likewise a section *type*'s heading text is
  `title` (matching `UserStory.title` etc.), not `heading`. `Spec` previously carried **both** `title`
  (the frontmatter label) and `heading` (the verbatim H1, e.g. `Feature Specification: Add New
  Student`); they were **not** redundant, but the H1's kind-prefix is derivable from `Spec.kind` (a
  corpus-ism), so `heading` is **dropped** and the H1 renders as `# {title}` — `title` is the single
  spec label. DDL rename/drop; re-import repopulates.
- **Entities are first-class documents, not specs.** Each entity doc previously produced **two**
  rows — a `req_spec` with `kind='entity'` (the document) and an `ent_entity` (the concept), linked
  1:1 by `spec_id` — duplicating `domain`/`title`/`status` and putting non-specs in the `spec` table.
  But entities had already diverged into their own `ent_*` namespace (their prose is `ent_entity_section`
  with its own type vocabulary, structure is `ent_attribute`/`ent_relationship`, and cross-refs use a
  distinct `target_type='entity'`), so the spec row was a thin, leaky shell. `ent_entity` now carries its
  own `path` (domain-relative, like `Spec.path`); the `spec[kind=entity]` rows, the `spec_id` FK, and the
  `entity` value of `Spec.kind` are **removed**. `generate` renders entities directly from `ent_entity` +
  `ent_entity_section` to their own doc path; ref targets get their page path from `ent_entity.path`. No
  cross-reference targeted entity docs *as specs* (all entity links resolve via `[[ENTITY:name]]`), so the
  change is ref-safe. Follow-ups (applied): **entities are domain-less and never specs** —
  `ent_entity.domain_id` is dropped and `path` stores the **full** doc location; the importer treats any
  doc under `entities/` as an entity (never a spec) and **does not create an `entities` domain** at all.
  This resolved the one hybrid, `Attachable` (`entities/attachable.md`): it was in the prefix-registry but
  had **zero** FRs/stories and was referenced only as an entity, so it is now a plain entity.
- **`Spec.kind` and `Domain.kind` are dropped.** `Spec.kind`'s only behavioural consumer was the
  `kind='entity'` discriminator (gone now that entities aren't specs); the rest were never rendered or
  branched on, and every imported spec was `feature`. `Domain.kind` (`service`/`shared`/…) was only ever
  *displayed* in `domain ls` — no consumer, not rendered. Both are constant/unread → removed (with their
  CLI flags, the `SpecKind`/`DomainKind` seed enums, and the spec/domain path classifiers). Re-add if a
  real consumer (e.g. a kind-aware `check` or index grouping) ever needs them.
- **`EntityRelationship` is imported from the tutor app's Drizzle schema** — the entity docs describe
  relationships only in prose, so the structured `ent_relationship` rows come from the actual DB schema
  (the foreign-key ground truth, not the duplicative `relations()` DSL). `import tutor` parses
  `src/packages/database/src/schema/*.ts` (auto-detected relative to the docs root, or `--drizzle <dir>`):
  a FK on a table's `id` → **one-to-one**, on any other column → **one-to-many** (parent → child, since the
  cardinality enum has no `many_to_one`), and a non-entity table whose composite PK is two FK columns →
  **many-to-many** (`junction_table` set). Only relationships between two known entities are emitted;
  N-ary junctions (e.g. `tutor_students`) are skipped and reported. Idempotent (deterministic ids).
- **ER refinements from the tutor import** (validated by `cusp import tutor`, the read-only
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
  - **`backlog` is a valid milestone value** — `Milestone.slug` is an open string with
    seed examples (`M0`–`M7`, `Future`); `backlog` joins them as seed/policy data, not a new column.
- **Changeset is the unit of batched, reviewable change** (the `Changeset` entity, formerly
  `ChangeProposal`). A changeset is a **Dolt branch** that bundles edits across many entities
  so they are diffed and reviewed together (a PR over the knowledge graph), not committed
  per-edit. CLI: `cusp changeset start|diff|submit|merge|abandon`; mutating commands take an
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
  keys: `DOMAIN`(slug) · `SPEC`(prefix, else path) · `REQ`(fr_key) · `ENTITY`(name) ·
  `MILESTONE`(slug) · `TERM`(glossary — deferred until `GlossaryTerm` lands). At ingestion the
  token is **validated, kept verbatim in the text** (for rendering), **and projected into a queryable
  `entity_ref` row** (owner→target; it carries no `kind` — it is always a "references" projection,
  whereas typed kinds live on `Edge`). The token stays the source of truth; `EntityRef`
  is its reconciled projection (re-derived per owner on each write — so removing a link removes its row).
  - **`EntityRef` vs `Edge`**: `EntityRef` is the home for **all prose-derived references** — the
    inline `[[…]]` links *and* the importer's auto-discovered mentions (inline FR citations, Key
    Entities lists). `Edge` becomes **hand-authored / structured-only** (`refines`/`depends_on`/… asserted
    via `edge add`). `impact`/`check` read **both**. (This reclassifies the importer's former ~600
    prose-derived `references` edges into `entity_ref`.)
  - **Source attribution** is the nearest first-class node owning the text: a link inside a scenario,
    `SpecSection`/`EntitySection`, or `RequirementGroup.notes` attributes to its owning user_story / spec /
    entity. Self-references are ignored (no self-ref row).
  - **Dangling refs** (token that resolves to nothing): **block** an interactive CLI/MCP write
    (`--force` overrides); **record a non-blocking finding** on bulk import; a `check` finding once
    `cusp check` exists.
  - **Render is per-format**: the Markdown generator rewrites each token to an **Obsidian wikilink**
    `[[vault-path#^anchor|label]]` (vault-relative path, extension dropped; same-file → `[[#^anchor|label]]`).
    Anchors are **Obsidian block references** (`^fr-key`) emitted at the end of each FR list item (and each
    glossary term), so `[[REQ:…]]` resolves to the FR inside its spec. The **HTML renderer** reuses the same
    Markdown pipeline, transforming wikilinks → relative `<a href>` and `^block` anchors → `<a id>` targets
    (goldmark); the **JSON serializer** leaves the raw `[[TYPE:key]]` tokens (data view). A target with no
    generated page (e.g. a milestone) renders label-only but still records the `entity_ref`. The importer
    **converts** the corpus's `[label](./other.md)` cross-spec links to canonical tokens so they round-trip.
- **Glossary is shared project vocabulary, separate from the Entity layer** (the
  [`GlossaryTerm`](glossary.md) + [`GlossaryAlias`](glossary.md) entities). A `GlossaryTerm` defines a
  *concept* once (`make-up-credit` → "minutes of lesson time owed…"); an `Entity` models a *document*
  (Student, Invoice). A term carries a `slug` (the stable `[[TERM:slug]]` link key), a display `term`,
  a `definition` (prose that may itself contain `[[…]]` links — so a term is also an `EntityRef` owner),
  an optional `domain_id` (scoping; null = global, **not** a studio tenant concept), and `status`.
  **Aliases are a child table** (`GlossaryAlias`, `UNIQUE(alias)`) so several surface forms resolve to one
  term; the resolver tries the slug first, then aliases. A term is a first-class
  [cross-reference](requirements.md#entityref) target (`[[TERM:slug]]`, deferred until now) and a generated
  artifact (the `glossary.md` page, one anchor per term). Authored via the CLI (`cusp term …`), not imported
  from the tutor corpus (no structured glossary source there).
- **Physical table names are prefixed by layer (migration `0011`).** MySQL/Dolt has no
  schema-within-database namespace (`CREATE SCHEMA` == `CREATE DATABASE`, a separate version-control unit),
  and Cusp must keep all tables in one database so a changeset is one atomic branch over the whole graph.
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
  content humans see is rendered from the `access_control` [`EntitySection`](authorization.md#entitysection)
  prose, so dropping both tables left **generated output byte-identical** at the time. The tutor importer's privilege parser
  and the `Privilege.scope`/`action` seed enums were removed with it. A *generic*, consumer-driven
  authorization concept can return when there is a real reader for it — see [ROADMAP](../ROADMAP.md).
- **Enum policy: three buckets — closed / seed / table** (the schema stores every enum as `VARCHAR`, so this is
  a *validation-policy* decision, not a storage one; see [enums.md](enums.md)). The import-derived growth had
  baked one corpus's value sets into the core; this draws the line:
  - **closed** — fixed lifecycle/workflow + structural discriminators (`*.status`, `Edge.kind`, the polymorphic
    `*_type`, `cardinality`, `verdict`, …). Validated hard; unknown rejected (`enums.Valid`).
  - **seed** — open value-sets with documented defaults that are project-/tenant-/tooling-specific:
    `Domain.kind`, `Spec.kind`, and the Qase `TestCase.*` taxonomies. Validated **leniently** —
    `app.ValidateEnumSoft` accepts an unknown
    value with a warning; `--strict` restores hard rejection. Import never rejects (it warns), matching the
    existing `unknown-delivery-status` finding.
  - **table** — value-sets graduate to seeded lookup tables. `Requirement.delivery_status`, the one that
    carries policy → [`delivery_status`](requirements.md#deliverystatus) (migration `0009`). And **priority**:
    the inconsistent per-entity schemes (`UserStory` `P1`–`P3`, `TestCase` `low`/`med`/`high`) are unified into
    the standard 0–4 [`Priority`](requirements.md#priority) table (migration `0018`), stored as the INT `level`
    and applied to `UserStory`/`Requirement`/`TestCase`. Both are keyed
    by their **business value** (not a ULID — a deliberate exception, like a classic reference table) and
    referenced by `requirement.delivery_status` **with no FK**, on purpose: the lookup is *soft* so drift is
    tolerated and surfaced by `check`, never rejected at write (record this rationale — a reviewer who sees the
    schema's ~43 FKs should not "fix" it by adding the constraint). The coverage policy (e2e/shared/milestone
    requirements) moves from prose into the table's boolean columns, enforced by the future `cusp check`.
