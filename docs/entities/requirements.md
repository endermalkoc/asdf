# Requirements layer

[← index](index.md) · see the [master diagram](index.md#master-diagram).

`UserStory`, `AcceptanceScenario`, `Requirement` (the functional requirement), `Milestone`,
and `Edge` (the cross-reference graph).

## UserStory
BDD scaffolding inside a spec. **No global ID** — referenced by its per-spec `position`
("User Story 3"). Carries a priority and the As-a / I-want / So-that persona.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | Surrogate only; not a citable ID |
| `spec_id` | FK → Spec | | |
| `position` | int | | Unique **within spec** (the document position / heading number) |
| `title` | varchar | | |
| `priority` | int | | `0`–`4` level → [`Priority`](#priority) (corpus `P1`–`P4` → `1`–`4`) |
| `as_a` / `i_want` / `so_that` | text | | Persona narrative (role-style stories) |
| `narrative` | text | | Prose body for prose-style stories (no As-a line); nullable. Exactly one of this / the As-a triple is set |
| `why_priority` | text | | The story's "Why this priority" rationale; nullable |
| `independent_test` | text | | The story's "Independent Test" note; nullable |

## AcceptanceScenario
A Given/When/Then scenario under a user story.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `user_story_id` | FK → UserStory | | |
| `position` | int | | Order within story |
| `given` / `when` / `then` | text | | |

## Requirement (Functional Requirement)
A `{PREFIX}-FR-{NNN}{x}` requirement. The human-readable ID is **composed**:
`spec.prefix + "-FR-" + zero-pad(number) + suffix`. Numbering is sequential within the
spec; gaps are allowed (deletions/reservations). Sub-requirements use `suffix` + a
self-reference to the base FR. Delivery metadata is inline. See
[Identifiers & keys](identifiers.md) for the `fr_key` derived column and the merge caveat.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `spec_id` | FK → Spec | | Owns the numbering namespace |
| `number` | int | | Sequential within spec (unique per `spec_id`) |
| `suffix` | char(1) | | Optional single sub-letter (`a`,`b`,…); nullable |
| `parent_id` | FK → Requirement | | Set on sub-requirements; null on base FRs |
| `group_id` | FK → RequirementGroup | | The FR group sub-header it sits under; null for ungrouped FRs |
| `position` | int | | Document order within the spec's FR list (preserves source order, which is not FR-number order) |
| `statement` | text | | The MUST statement |
| `content_status` | enum | | `draft`, `active`, `obsolete` (the requirement's own lifecycle) |
| `delivery_status` | varchar | | a `delivery_status.key` (`covered`, `test-pending`, `not-implemented`, `e2e-sufficient`, `shared`, `schema-only`, `deferred`). A **business-key reference** to the [`delivery_status`](#deliverystatus) lookup table — not an FK, so unknown values are tolerated and surfaced, not rejected |
| `milestone_id` | FK → Milestone | | Nullable |
| `priority` | int | | `0`–`4` level → [`Priority`](#priority); **nullable** (`NULL` = unprioritized; no corpus source yet) |
| `notes` | text | | |
| `tombstoned_at` | date | | Set when the FR is deleted (tombstone) |
| `created_at` / `updated_at` | datetime | | |

> Constraints worth enforcing: `UNIQUE(spec_id, number, suffix)`. The delivery-coverage rules
> (≥1 `layer = e2e` [`TestCase`](testing.md#testcase) when `delivery_status = e2e-sufficient`; ≥1
> `layer = shared` when `= shared`; `milestone_id` required when `= not-implemented`) are now **data,
> not prose** — they live in the [`delivery_status`](#deliverystatus) table's policy columns
> (`requires_e2e_test` / `requires_shared_test` / `requires_milestone`) and are enforced by the future
> `cusp check`.

## DeliveryStatus
A **lookup table** for `Requirement.delivery_status` — the one enum that earned its own table because it
carries policy. Keyed by its business value (e.g. `e2e-sufficient`), like a classic reference table (a
deliberate exception to the ULID-PK rule — see [identifiers.md](identifiers.md)). Requirements reference it
**by key, with no FK**, so a novel status is tolerated and surfaced by `check`, never rejected at write
(consistent with the seed-value leniency). Seeded by migration `0009`; configurable by adding rows.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `slug` | varchar | **PK** | the business value, matches `requirement.delivery_status`. Named `slug`, not the SQL reserved word `key` (migration 0014) |
| `label` | varchar | | display name |
| `description` | text | | what the status means |
| `sequence` | int | | sort/coverage order |
| `counts_as_covered` | bool | | rolls up as "done" in coverage stats |
| `requires_e2e_test` | bool | | needs ≥1 `layer = e2e` TestCase (`e2e-sufficient`) |
| `requires_shared_test` | bool | | needs ≥1 `layer = shared` TestCase (`shared`) |
| `requires_milestone` | bool | | needs `milestone_id` (`not-implemented`) |
| `created_at` / `updated_at` | datetime | | |

## Priority
The **one standard priority taxonomy** (migration `0018`) — a lookup table referenced by
`UserStory.priority`, `Requirement.priority`, and `TestCase.priority`. Stored as the INT `level`
(`0` = most urgent), sortable; the label/description are documentation. Like `DeliveryStatus`, it's a
**soft reference** (no FK) validated in-app. Replaced the old per-entity schemes (`P1`–`P3`,
`low`/`medium`/`high`).

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `level` | int | **PK** | `0` Critical · `1` High · `2` Medium · `3` Low · `4` Backlog |
| `label` | varchar | | display name |
| `description` | text | | what the level is for (e.g. `0` = security, data loss, broken builds) |
| `created_at` / `updated_at` | datetime | | |

## RequirementGroup
A bold **sub-header** that organizes a spec's FR list ("Student Section", "Family Section
(Children Only)", …), with the optional interspersed `notes` (e.g. a `> See [shared/X]` blockquote)
that sits under it. First-class so the FR list round-trips exactly: groups order by `position`,
requirements order by their own `position` within the group — neither follows FR-number order.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `spec_id` | FK → Spec | | |
| `position` | int | | Order of the group within the spec's FR list |
| `title` | varchar | | The sub-header text (unique per spec) |
| `notes` | text | | Interspersed prose under the title; nullable |

> `UNIQUE(spec_id, title)`. Replaces the former denormalized `Requirement.section` string.

## Milestone
An ordered delivery target, and the **second cross-cutting join hub** (with `Domain`)
between the spec corpus and the [planning layer](planning.md). Referenced from three places:
`Requirement.milestone_id` (spec side), `Deliverable.milestone_id` (one per deliverable),
and `Capability` (many-to-many — a capability can span milestones). Example value set:
`M0`–`M7`, `Future`.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `slug` | varchar | **UK** | `M0`–`M7`, `Future` |
| `name` | varchar | | |
| `description` | text | | |
| `sequence` | int | | `Future` sorts last |
| `status` | enum | | `complete`, `in_progress`, `pending` |
| `created_at` / `updated_at` | datetime | | |

## Edge
A typed, directed, **polymorphic** link between two nodes — the **hand-authored / structured**
graph layer (`refines`, `depends_on`, `supersedes`, … asserted via `edge add`). Prose-derived
references — inline `[[…]]` links and the importer's auto-discovered mentions — live in
[`EntityRef`](#entityref) instead; `impact`/`check` read both. (FR↔test coverage is its own
`requirement_test_case` junction — see [testing.md](testing.md#testcase) — not an edge.)

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | **Deterministic** — `uuidv5` over the `UNIQUE` identity, not a random ULID (see [Identifiers](identifiers.md)) |
| `from_type` | enum | | `requirement`, `spec`, `user_story`, `entity`, `milestone` |
| `from_id` | bigint / uuid | | Polymorphic FK (type + id) |
| `to_type` | enum | | same domain as `from_type` |
| `to_id` | bigint / uuid | | Polymorphic FK |
| `kind` | enum | | `references`, `refines`, `depends_on`, `supersedes`, `relates`, `defers_to` |

> `UNIQUE(from_type, from_id, to_type, to_id, kind)` is the edge's identity, and the PK is
> derived from it — so the same edge added on two branches converges on merge instead of
> tripping the unique key. See the [deterministic-PK rule](identifiers.md).

## EntityRef
A **prose-derived** cross-reference: the queryable projection of an inline `[[TYPE:key]]` link
(or an importer-discovered mention) found in a text field. The token stays verbatim in the text
(it is what `generate` rewrites into a link); the `entity_ref` row is its reconciled,
queryable form — re-derived per owner on every write, so editing prose adds/removes rows. The
**owner** is the nearest first-class node holding the text (a link in a scenario /
`SpecSection` / `EntitySection` / `RequirementGroup.notes` attributes to its owning `UserStory` /
`Spec` / `Entity`). See
[decisions.md](decisions.md) for the token grammar, the dangling-ref policy, and the
render mapping; **distinct from [`Edge`](#edge)**, which is hand-authored / structured.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | **Deterministic** — `uuidv5` over the `UNIQUE` identity, not a random ULID (see [Identifiers](identifiers.md)) |
| `owner_type` | enum | | `domain`, `spec`, `requirement`, `user_story`, `entity`, `milestone` (the nearest first-class owner) |
| `owner_id` | bigint / uuid | | Polymorphic FK (type + id) |
| `target_type` | enum | | `domain`, `spec`, `requirement`, `entity`, `milestone` (`glossary_term` when the glossary lands) |
| `target_id` | bigint / uuid | | Polymorphic FK |

> `UNIQUE(owner_type, owner_id, target_type, target_id)` is the ref's identity, and the PK
> is derived from it (same deterministic rule as `Edge`). Unlike `Edge`, entity_ref carries **no
> `kind`** — it is only ever a "references" projection of an inline link (typed relationship kinds
> live on `Edge`). Multiple tokens to the same target from
> one owner collapse to one row; per-token display text and position are **not** stored — `generate`
> re-scans the verbatim token in the text to render each occurrence.
