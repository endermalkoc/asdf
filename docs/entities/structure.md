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
| `abbreviation` | varchar | **UK** | |
| `name` | varchar | | Canonical name |
| `description` | text | | One-line summary (from the domain index); nullable |
| `kind` | enum | | `service`, `shared`, `infrastructure`, `entities`, `analysis` |
| `status` | enum | | `draft`, `active`, `deprecated` |

## Spec
A document — one `.md` file — and the unit that owns FR numbering. The directory tree is
**derived from `path`** (no separate folder table); `path` is the full docs-relative
location. FR-bearing specs have a unique `prefix`; FR-exempt docs (entity glossary,
journeys, analysis, index/meta) have `prefix = NULL` and a `kind` that classifies them.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `domain_id` | FK → Domain | | From frontmatter `domain` (also the first path segment) |
| `prefix` | varchar | **UK** | 2–6 upper; **nullable** for FR-exempt docs |
| `slug` | varchar | | Filename without extension |
| `path` | varchar | **UK** | Full docs-relative path; **source of the directory tree** |
| `title` | varchar | | |
| `kind` | enum | | `feature`, `entity`, `journey`, `analysis`, `index`, `meta`, `reference` |
| `status` | enum | | `draft`, `reviewed`, `active`, `obsolete` (`reviewed` = under review, before active) |
| `heading` | text | | The H1 line verbatim (may differ from `title`); nullable. **Not a section** — the document's identity line (the `#`) |
| `created_at` / `updated_at` | date | | From spec header |

> **Section capture.** A spec's structured content (user stories, acceptance scenarios,
> requirements) is modeled in the [requirements layer](requirements.md). **All of its prose sections** —
> the recurring template ones (`overview`, `edge_cases`, `success_criteria`, `platform_scope`,
> `assumptions`, `clarifications`, `preamble`, the FR-area `more_info` tail) **and** the bespoke tail —
> are captured as [`DocSection`](#docsection) rows: a recognized section carries a normalized `section_key`
> (e.g. `overview`), a bespoke one carries `section_key = NULL`. This keeps the core generic (a different
> corpus's sections just get keys or go bespoke — no schema change) while a regenerate stays
> information-complete. "Key Entities" lists are also captured as `spec → entity`
> [`EntityRef`](requirements.md#entityref)s.

## DocSection
The model for **every document section** — recurring template sections (addressed by `section_key`) and
the bespoke tail (`section_key = NULL`) — keeping a regenerate information-complete. Replaced the former
per-section typed columns on `Spec`/`Entity` (migration `0010`). Its owner is **polymorphic** (`spec` or
`entity`), so it is not an FK.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `owner_type` | enum | | `spec`, `entity` |
| `owner_id` | bigint / uuid | | Polymorphic (type + id) |
| `ordinal` | int | | Original position in the source doc (unique per owner) |
| `section_key` | varchar | | Normalized id of a recognized section (`overview`, `edge_cases`, `purpose`, …); **`NULL` for a bespoke section** |
| `level` | int | | Heading depth: `2` = `##`, `3` = `###` (a `0`/empty-heading row is bare intro prose, e.g. `preamble`) |
| `heading` | text | | The section heading (don't-care for keyed sections — they render at a canonical spot) |
| `body` | text | | The section's Markdown body, verbatim |

> `UNIQUE(owner_type, owner_id, ordinal)` and `UNIQUE(owner_type, owner_id, section_key)` — the latter
> permits many bespoke rows (NULLs are distinct), one row per recognized key.
