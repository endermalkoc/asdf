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
| `heading` | text | | The H1 line verbatim (may differ from `title`); nullable |
| `preamble` | text | | Content between the H1 and the first `##` (e.g. the metadata block); nullable |
| `overview` / `edge_cases` / `success_criteria` / `platform_scope` / `assumptions` / `clarifications` | text | | The recurring template sections, stored as verbatim Markdown; nullable. Bespoke sections go to `DocSection`. |
| `more_info` | text | | FR-area content that is neither a requirement nor a real FR group — note-only bold sub-headers (e.g. "Column Configuration") and their config/table bodies; nullable |
| `created_at` / `updated_at` | date | | From spec header |

> **Section capture.** A spec's structured content (user stories, acceptance scenarios,
> requirements) is modeled in the [requirements layer](requirements.md). Its **recurring template
> prose** — including the `clarifications` decision log — is the typed columns above (stored verbatim,
> since the corpus's Clarifications mix `### Session` blocks, Q/A bullets, and tables); **any other
> section** is preserved generically in [`DocSection`](#docsection) — together making a regenerate
> information-complete. "Key Entities" lists are captured as `spec → entity` [`Edge`](requirements.md#edge)s.

## DocSection
The **generic catch-all** for any document section not modeled as a typed field — the tail of
bespoke per-spec sections (and per-entity sections) that keeps a regenerate information-complete.
Its owner is **polymorphic** (`spec` or `entity`), so it is not an FK.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `owner_type` | enum | | `spec`, `entity` |
| `owner_id` | bigint / uuid | | Polymorphic (type + id) |
| `ordinal` | int | | Original position in the source doc (unique per owner) |
| `level` | int | | Heading depth: `2` = `##`, `3` = `###` |
| `heading` | text | | The section heading |
| `body` | text | | The section's Markdown body, verbatim |
