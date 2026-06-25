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
- **Document sections are captured so a regenerate is information-complete** (migration `0003`).
  Goal: `asdf generate` must reproduce the *information* of the source docs (format may differ).
  A section inventory of the tutor corpus set the typed-vs-generic line: entity docs are rigidly
  templated, and feature specs have ~7 recurring template sections — both become **typed columns**
  (`Spec.overview/edge_cases/success_criteria/platform_scope/assumptions/heading/preamble`,
  `Entity.purpose/key_concepts/…`); the per-story `why_priority`/`independent_test` and the FR group
  the FR group are **promoted** from prose. The FR group is a first-class
  [`RequirementGroup`](requirements.md#requirementgroup) (migration `0004`, replacing the earlier
  denormalized `Requirement.section` string): it carries the group's `position`, `header`, and the
  interspersed `note` (e.g. a `> See [shared/X]` blockquote), and requirements link to it with their
  own document `position` — so the FR list regenerates in **source order** (not FR-number order),
  with grouping and notes intact. **Clarifications** are stored verbatim in
  `Spec.clarifications` (the corpus mixes `### Session` blocks, Q/A bullets, and tables — too
  inconsistent for clean rows); **Key Entities** lists become `spec → entity` edges. The long tail
  of ~92 bespoke one-off feature-spec sections (and bespoke entity sections)
  can't each be a column, so they land in the generic [`DocSection`](structure.md#docsection)
  catch-all (heading + body + ordinal). Generation emits a fixed canonical section order — order/format
  may differ from the source, information does not. `EntityAttribute`/`EntityRelationship`/`AccessRule`
  stay the optional finer structured extraction over the same prose, not a second source of truth.
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
