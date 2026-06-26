# Entity layer

[← index](index.md) · see the [master diagram](index.md#master-diagram).

> **No structured authorization model.** Earlier revisions modeled row-level access as a
> `Privilege` (`(resource, scope, action)` triple) + `AccessRule` (entity↔privilege binding) pair.
> That baked one corpus's CASL-style authorization paradigm into the core schema and was never
> consumed (nothing read it; `Privilege` was write-only), so migration `0012` removed both. An
> entity's access rules live as **prose** in its `access_control` [`EntitySection`](#entitysection);
> a generic, consumer-driven authorization concept can return later (see [ROADMAP](../ROADMAP.md)).

> These are **authored business-domain documents**, not a projection of the technical
> schema. They describe what an entity *means* — purpose, domain properties, relationships,
> business rules, validations, access — in domain language. ASDF owns them as the canonical
> domain glossary; they do **not** mirror or sync from a database schema, and a property
> here need not correspond one-to-one with a stored column. The full narrative lives in the
> entity's own [`EntitySection`](#entitysection) prose (the entity is a first-class document,
> not a spec); these tables are its structured head.

## Entity
A business concept from `entities/**` (Student, Family, Event, …). It is a **first-class document
in its own right** — not modeled as a spec, and **not scoped to a `Domain`**: its prose lives in
[`EntitySection`](#entitysection), its structure in `EntityAttribute`/`EntityRelationship`.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `path` | varchar | | Optional **sub-directory** under `entities/` (NULL = directly under `entities/`, the common case — **no filename, no `entities/` prefix**). Full location = `entities/ + [path + "/"] + kebab(name) + ".md"`; the filename is derived from `name` (lower-kebab). Entities are **domain-less** |
| `name` | varchar | **UK** | |
| `description` | text | | Short domain definition (from the entity index); nullable. **Not a doc section** |
| `status` | enum | | `draft`, `active`, `deprecated` |

> Entity docs are templated; their **prose sections** are captured as [`EntitySection`](#entitysection)
> rows, each pointing at a curated [`EntitySectionType`](#entitysectiontype) — the entity-layer mirror
> of [`SpecSection`](structure.md#specsection). There are **no free-form sections**: headings outside the
> curated vocabulary **fold into the `notes` type** on import. `EntityAttribute` / `EntityRelationship`
> remain the **finer structured extraction** of that same prose — populated when a command parses it, not
> a duplicate source of truth. (`access_control` has no structured counterpart — it stays prose-only; see
> the note at the top of this page.)

## EntitySectionType
The **curated vocabulary** of entity prose sections — the entity-layer mirror of
[`SpecSectionType`](structure.md#specsectiontype). A built-in seed ships with migration `0013`; new
types may be added later at a deliberate cost.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `slug` | varchar | **PK** | Stable id the CLI/importer selects by (`purpose`, `key_concepts`, …). Named `slug`, not the SQL reserved word `key` |
| `title` | text | | The section's title, rendered as the `##` heading; `""`/nullable = headingless intro (e.g. `preamble`) |
| `level` | int | | Heading depth: `2` = `##`, `0` = headingless |
| `position` | int | | Canonical render order — **uniform across every doc** |
| `description` | text | | Guidance shown when picking a type; nullable |
| `origin` | enum | | `builtin` (seed) or `authored` (added later) |

> Seed keys: `preamble`, `purpose`, `key_concepts`, `schema_reference`, `relationships`,
> `business_rules`, `validations`, `access_control`, `notes`, `references`.

## EntitySection
One prose section of an entity, addressed by its curated type. Title, depth, and order all come from
the [`EntitySectionType`](#entitysectiontype); the instance carries only the body. Entity docs have **no
structural anchors**, so every section renders in a simple `position`-ordered sweep.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `entity_id` | FK → Entity | | `ON DELETE CASCADE` |
| `section_type_slug` | FK → EntitySectionType | | The curated type — **`NOT NULL`** |
| `body` | text | | The section's Markdown body, verbatim |

> `UNIQUE(entity_id, section_type_slug)` — at most one section of each type per entity.

## EntityAttribute
A documented **domain property** of an entity — its business meaning, not a typed DB
column. Grouped the way entity docs group them (e.g. "Academic", "Lesson Settings",
"Calculated / Derived Fields").

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `entity_id` | FK → Entity | | |
| `name` | varchar | | Domain property name (e.g. `skill level`, `birthday`) |
| `description` | text | | What it means / business rules for it |
| `category` | varchar | | Doc grouping (e.g. `Academic`, `Lesson Settings`); nullable |
| `is_derived` | bool | | Calculated / derived rather than directly entered |
| `position` | int | | Ordering within the entity; nullable |

## EntityRelationship
A relationship between two entities (the "Relationships" section of an entity doc).

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `from_entity_id` | FK → Entity | | |
| `to_entity_id` | FK → Entity | | |
| `cardinality` | enum | | `one_to_one`, `one_to_many`, `many_to_many` |
| `junction_table` | varchar | | Nullable (for m2m) |
| `notes` | text | | |
