# Entity layer

[← index](index.md) · see the [master diagram](index.md#master-diagram).

> **No structured authorization model.** Earlier revisions modeled row-level access as a
> `Privilege` (`(resource, scope, action)` triple) + `AccessRule` (entity↔privilege binding) pair.
> That baked one corpus's CASL-style authorization paradigm into the core schema and was never
> consumed (nothing read it; `Privilege` was write-only), so migration `0012` removed both. An
> entity's access rules live as **prose** in its `row_level_access` [`DocSection`](structure.md#docsection);
> a generic, consumer-driven authorization concept can return later (see [ROADMAP](../ROADMAP.md)).

> These are **authored business-domain documents**, not a projection of the technical
> schema. They describe what an entity *means* — purpose, domain properties, relationships,
> business rules, validations, access — in domain language. ASDF owns them as the canonical
> domain glossary; they do **not** mirror or sync from a database schema, and a property
> here need not correspond one-to-one with a stored column. The full narrative lives in the
> linked entity doc (a [`Spec`](structure.md#spec) with `kind = entity`); these tables are
> its structured head.

## Entity
A domain entity from `entities/**` (Student, Family, Event, …) — a business concept.
Usually has a documenting spec (`kind = entity`) that carries the prose.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `domain_id` | FK → Domain | | |
| `spec_id` | FK → Spec | | The entity doc (full narrative); nullable |
| `name` | varchar | **UK** | |
| `description` | text | | Short domain definition (from the entity index); nullable. **Not a doc section** |
| `status` | enum | | `draft`, `active`, `deprecated` |

> Entity docs are rigidly templated; their recurring sections (`purpose`, `key_concepts`,
> `schema_reference`, `relationships`, `business_rules`, `validations`, `row_level_access`, `notes`,
> `spec_references`) are captured as [`DocSection`](structure.md#docsection) rows keyed by `section_key`
> (migration `0010` replaced the former per-section typed columns), with bespoke sections as
> `section_key = NULL` — a regenerate is then information-complete. `EntityAttribute` /
> `EntityRelationship` remain the **finer structured extraction** of that same prose —
> populated when a command parses it, not a duplicate source of truth. (`row_level_access` has
> no structured counterpart — it stays prose-only; see the note at the top of this page.)

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
