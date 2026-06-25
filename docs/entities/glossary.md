# Glossary layer

[← index](index.md) · see the [master diagram](index.md#master-diagram).

`GlossaryTerm` and `GlossaryAlias` — shared project **vocabulary**, so humans and agents
define a concept **once** and reference it everywhere via an inline `[[TERM:slug]]`
[cross-reference](requirements.md#entityref). Distinct from the business [`Entity`](authorization.md)
layer: an `Entity` models a domain *document* (Student, Invoice); a `GlossaryTerm` is project
*vocabulary* (a definition of "make-up credit", "forecast charge"). A term is both a first-class
**link target** and a generated artifact (the glossary page).

## GlossaryTerm
A defined term. The `slug` is the stable link key (`[[TERM:make-up-credit]]`); `term` is the
display name; `definition` is prose that **may itself contain `[[…]]` links**, so a term is also
an [`EntityRef`](requirements.md#entityref) *owner*.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | ULID surrogate |
| `slug` | varchar | **UK** | kebab-case link key, e.g. `make-up-credit` |
| `term` | varchar | | canonical display name, e.g. `Make-up Credit` |
| `definition` | text | | the definition prose (may contain inline `[[…]]` links) |
| `domain_id` | FK → Domain | | Optional scoping; null = global vocabulary |
| `status` | enum | | `draft`, `active`, `deprecated` |
| `created_at` / `updated_at` | datetime | | |

> `UNIQUE(slug)`. The display defaults to `term` when a `[[TERM:slug]]` link omits its `|display`.

## GlossaryAlias
An alternate surface form that resolves to a term — so `[[TERM:muc]]` and `[[TERM:makeup-credit]]`
both reach the same `GlossaryTerm`. The resolver tries the slug first, then aliases.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | ULID surrogate |
| `term_id` | FK → GlossaryTerm | | |
| `alias` | varchar | **UK** | resolves to exactly one term (global `UNIQUE`) |

> `UNIQUE(alias)` — an alias is unambiguous across the glossary, like the slug.
