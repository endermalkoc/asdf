# Structure layer

[← index](index.md) · see the [master diagram](index.md#master-diagram) and
[Identifiers & keys](identifiers.md).

`Domain` and `Spec` form the document tree. There is **no folder table** — the directory
structure is derived from `Spec.path`.

## Domain
A top-level area, and the **shared classification dimension** that ties the spec corpus to
the planning layer. It maps to a root directory under `docs/specs/`; **every domain has
specs**. Service boundaries (`enrollment`, `scheduling`, `finance`, `identity`, `platform`,
`staffing`, `communication`, …) plus the special `entities` / `shared` / `infrastructure`
trees.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `slug` | varchar | **UK** | |
| `name` | varchar | | Canonical name |
| `description` | text | | One-line summary (from the domain index); nullable |
| `status` | enum | | `draft`, `active`, `deprecated` |

## Spec
A document — one `.md` file — and the unit that owns FR numbering. The full location is
**reconstructed**, never stored whole: `domain.slug + "/" + [path + "/"] + slug + ".md"`. The
domain isn't duplicated in `path`, and the filename isn't either — it is `slug.md`. FR-bearing specs
have a unique `prefix`; FR-exempt docs (entity glossary,
journeys, analysis, index/meta) have `prefix = NULL` and a `kind` that classifies them.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `domain_id` | FK → Domain | | From frontmatter `domain`; the **sole** home of the domain — it is the top-level output directory and is **not** repeated in `path` |
| `prefix` | varchar | **UK** | 2–6 upper; **nullable** for FR-exempt docs |
| `slug` | varchar | | **Filename** without extension — the file is `slug.md` |
| `path` | varchar | | The **directory** only (domain-relative, no leading domain segment, **no filename**); **`NULL` = top-level** (directly under the domain — same convention as `ent_entity.path`). Full location = `domain.slug + "/" + [path + "/"] + slug + ".md"` |
| — | | **UK** (`domain_id`,`path`,`slug`) | The full location identity (`uk_spec_location`, migration `0017`) |
| `title` | varchar | | |
| `status` | enum | | `draft`, `reviewed`, `active`, `obsolete` (`reviewed` = under review, before active) |
| `created_at` / `updated_at` | date | | From spec header |

> The document's H1 is rendered from `title` (`# {title}`). The spec carries no separate
> `heading` column — the verbatim H1's kind-prefix (`Feature Specification:` …) was a
> corpus-ism derivable from `kind`, so `title` is the single spec label (migration `0019`).

> **Section capture.** A spec's structured content (user stories, acceptance scenarios,
> requirements) is modeled in the [requirements layer](requirements.md). Its **prose sections** are
> captured as [`SpecSection`](#specsection) rows, each pointing at a curated
> [`SpecSectionType`](#specsectiontype) — there are **no free-form sections**. A section *type*
> defines the title, depth, and canonical render position once; an *instance* just carries the body.
> The curated vocabulary is selected from, not invented per-document: headings outside it **fold into
> the `notes` type** on import, and adding a new type is a deliberate, separate step (so the vocabulary
> stays small and shared). "Key Entities" lists are also captured as `spec → entity`
> [`EntityRef`](requirements.md#entityref)s.

## SpecSectionType
The **curated vocabulary** of spec prose sections — the set agents and importers *select from* rather
than invent. A built-in seed ships with migration `0013`; new types may be added later at a deliberate
cost (a dedicated CLI call, not an inline flag), keeping the vocabulary from sprawling.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `slug` | varchar | **PK** | Stable id the CLI/importer selects by (`overview`, `success_criteria`, …). Named `slug`, not the SQL reserved word `key` |
| `title` | text | | The section's title, rendered as the `##` heading; `""`/nullable = headingless intro (e.g. `preamble`) |
| `level` | int | | Heading depth: `2` = `##`, `3` = `###`, `0` = headingless |
| `position` | int | | Canonical render order — **uniform across every doc** |
| `description` | text | | Guidance shown when picking a type; nullable |
| `origin` | enum | | `builtin` (seed) or `authored` (added later) |

> Seed keys: `preamble`, `overview`, `edge_cases`, `more_info`, `success_criteria`, `assumptions`,
> `scope`, `open_questions`, `notes`. `edge_cases` and `more_info` are **block-owned** — rendered inside
> their structural anchor (below), not as standalone sections.

## SpecSection
One prose section of a spec, addressed by its curated type. Title, depth, and order all come from the
[`SpecSectionType`](#specsectiontype); the instance carries only the body.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `spec_id` | FK → Spec | | `ON DELETE CASCADE` |
| `section_type_slug` | FK → SpecSectionType | | The curated type — **`NOT NULL`** |
| `body` | text | | The section's Markdown body, verbatim |

> `UNIQUE(spec_id, section_type_slug)` — at most one section of each type per spec.

> **Render order is canonical.** Generation emits sections in `SpecSectionType.position` order (not
> source order). Two structural blocks are interleaved at fixed positions and own two section types:
> **User Scenarios & Testing** (user stories + `edge_cases`) and **Requirements** (the FR list +
> `more_info`). The same `SpecSectionType`/`SpecSection` pair has an entity-layer mirror —
> [`EntitySectionType`](authorization.md#entitysectiontype) / [`EntitySection`](authorization.md#entitysection).
