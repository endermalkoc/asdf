# Requirements layer

[← index](index.md) · see the [master diagram](index.md#master-diagram).

`UserStory`, `AcceptanceScenario`, `Requirement` (the functional requirement), `Milestone`,
and `Edge` (the cross-reference graph).

## UserStory
BDD scaffolding inside a spec. **No global ID** — referenced by its per-spec `ordinal`
("User Story 3"). Carries a priority and the As-a / I-want / So-that persona.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | Surrogate only; not a citable ID |
| `spec_id` | FK → Spec | | |
| `ordinal` | int | | Unique **within spec** (the heading number) |
| `title` | varchar | | |
| `priority` | enum | | `P1`, `P2`, `P3` |
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
| `ordinal` | int | | Order within story |
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
| `delivery_status` | enum | | `covered`, `test-pending`, `not-implemented`, `e2e-sufficient`, `shared`, `schema-only`, `deferred` |
| `milestone_id` | FK → Milestone | | Nullable |
| `owner` | varchar | | e.g. `backend` |
| `notes` | text | | |
| `optout_marker` | enum | | `none`, `visual`, `ops`, `untestable` |
| `optout_reason` | varchar | | Required when `optout_marker != none` |
| `tombstoned_at` | date | | Set when the FR is deleted (tombstone) |
| `created_at` / `updated_at` | datetime | | |

> Constraints worth enforcing: `UNIQUE(spec_id, number, suffix)`; ≥1 linked
> [`TestCase`](testing.md#testcase) of `layer = e2e` when `delivery_status = e2e-sufficient`
> and of `layer = shared` when `= shared`; `milestone_id` required when
> `delivery_status = not-implemented`.

## RequirementGroup
A bold **sub-header** that organizes a spec's FR list ("Student Section", "Family Section
(Children Only)", …), with the optional interspersed `note` (e.g. a `> See [shared/X]` blockquote)
that sits under it. First-class so the FR list round-trips exactly: groups order by `position`,
requirements order by their own `position` within the group — neither follows FR-number order.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `spec_id` | FK → Spec | | |
| `position` | int | | Order of the group within the spec's FR list |
| `header` | varchar | | The sub-header text (unique per spec) |
| `note` | text | | Interspersed prose under the header; nullable |

> `UNIQUE(spec_id, header)`. Replaces the former denormalized `Requirement.section` string.

## Milestone
An ordered delivery target, and the **second cross-cutting join hub** (with `Domain`)
between the spec corpus and the [planning layer](planning.md). Referenced from three places:
`Requirement.milestone_id` (spec side), `Deliverable.milestone_id` (one per deliverable),
and `Capability` (many-to-many — a capability can span milestones). Example value set:
`M0`–`M7`, `Future`.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `abbreviation` | varchar | **UK** | `M0`–`M7`, `Future` |
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
**owner** is the nearest first-class node holding the text (a link in a scenario / `DocSection` /
`RequirementGroup.note` attributes to its owning `UserStory` / `Spec`). See
[decisions.md](decisions.md) for the token grammar, the dangling-ref policy, and the
render mapping; **distinct from [`Edge`](#edge)**, which is hand-authored / structured.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | **Deterministic** — `uuidv5` over the `UNIQUE` identity, not a random ULID (see [Identifiers](identifiers.md)) |
| `owner_type` | enum | | `domain`, `spec`, `requirement`, `user_story`, `entity`, `milestone` (the nearest first-class owner) |
| `owner_id` | bigint / uuid | | Polymorphic FK (type + id) |
| `target_type` | enum | | `domain`, `spec`, `requirement`, `entity`, `milestone` (`glossary_term` when the glossary lands) |
| `target_id` | bigint / uuid | | Polymorphic FK |
| `kind` | enum | | `references` (the only inline-link kind) |

> `UNIQUE(owner_type, owner_id, target_type, target_id, kind)` is the ref's identity, and the PK
> is derived from it (same deterministic rule as `Edge`). Multiple tokens to the same target from
> one owner collapse to one row; per-token display text and position are **not** stored — `generate`
> re-scans the verbatim token in the text to render each occurrence.
