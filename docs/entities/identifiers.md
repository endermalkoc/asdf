# Identifiers & keys

[← index](index.md)

One rule for every table: a row's `id` is a function of the row, never a coordinated counter
— a random ULID for authored rows, derived from identity for pure relationships.

**Surrogate PK — ULID (UUIDv7-class).** For authored rows, every `id` is a 128-bit, time-ordered ULID minted
client-side. It's the only PK that survives Dolt's Git-style branch/merge when agents
create rows offline on divergent branches: collision-free with **zero coordination**, and
time-ordering preserves index locality (unlike random UUIDv4). All foreign keys reference
the ULID, so renames/renumbers never cascade.
*(For brevity the tables and diagram show `id` generically as `bigint / uuid` — read it as a
ULID everywhere **except** the pure-relationship tables below, where the PK is derived from
the row's identity, and small **reference/lookup tables** keyed directly by their business value —
e.g. [`delivery_status`](requirements.md#deliverystatus) and the section-type tables, whose `slug`
(`covered`, `e2e-sufficient`, … / `overview`, `purpose`, …) **is** the PK, like a classic enum table.
(`slug`, not the SQL reserved word `key`.))*

**Deterministic PK for pure-relationship tables.** A relationship row's identity is entirely
its foreign keys (plus an enum), and two agents on divergent branches routinely create the
*same* one — both add `FR-012 refines FR-001`, both link a requirement to a test. Its PK must
therefore be a **function of that identity, never a random value**: otherwise the two
branches mint different PKs for one logical row, and on merge Dolt raises a **unique-key
violation** the operator has to resolve by hand instead of the rows simply converging. Two
shapes:

- **Junctions** (`requirement_test_case`, `test_run_configuration`, `capability_milestone`,
  `capability_deliverable`, `deliverable_view`, `deliverable_dependency`) — no surrogate; the
  **composite column pair *is* the PK**. Identical links converge automatically.
- **`Edge`, `EntityRef`, and `TestResult`** — keep a surrogate `id` (`Edge`/`EntityRef` are
  polymorphic; `TestResult` is externally referenced and carries payload), but **derive it
  deterministically** from the `UNIQUE` identity: `id = uuidv5(adlg-rel-namespace, identity-columns)`
  (a fixed namespace + the identity columns joined by a separator that cannot occur in them). The
  derivation uses only the immutable identity columns, **never** mutable payload (a `TestResult`'s
  `status`), so editing payload never re-keys the row.

This is the lesson the [beads](https://github.com/steveyegge/beads) project paid for in its
dependency table (a random per-clone UUID made `pull` fail or duplicate edges); we apply the
technique generically. The ULID rule above stands for every other (authored / surrogate-keyed)
table.

**Business keys — DB-enforced `UNIQUE`.** Alongside the surrogate, each table constrains
whatever makes a row meaningful. This is what humans and tooling cite, and what keeps Dolt
merges legible.

| Entity | Business `UNIQUE` constraint | Human / cite form (example) |
|---|---|---|
| Domain | `UNIQUE(slug)` | `scheduling` |
| Spec | `UNIQUE(prefix)` (when not null) + `UNIQUE(domain_id, path, slug)` | `ATT` · `scheduling/events/take-attendance.md` (= `scheduling` / `events` / `take-attendance`) |
| Requirement | `UNIQUE(spec_id, number, suffix)` | `ATT-FR-012`, `ATT-FR-038a` |
| UserStory | `UNIQUE(spec_id, position)` | `ATT#US3` (not globally citable) |
| Milestone | `UNIQUE(slug)` | `M1`, `Future` |
| Entity | `UNIQUE(name)` | `Student` (→ `entities/student.md`) |
| EntityAttribute | `UNIQUE(entity_id, name)` | `Student.birthday` |
| View | `UNIQUE(route)` | `/students/[studentId]?tab=messages` |
| TestCase | `UNIQUE(suite_id, title)` *(or an optional `code`)* | often surrogate-only |
| TestResult | `UNIQUE(run_id, test_case_id, configuration_id)` | — |
| Configuration | `UNIQUE(group, name)` | `Browser:Chrome` |
| Edge | `UNIQUE(from_type, from_id, to_type, to_id, kind)` | — |
| EntityRef | `UNIQUE(owner_type, owner_id, target_type, target_id)` | — |
| GlossaryTerm | `UNIQUE(slug)` | `make-up-credit` |
| GlossaryAlias | `UNIQUE(alias)` | `muc` |
| DeliveryStatus | `PRIMARY KEY(key)` — the business value **is** the PK | `e2e-sufficient` |
| ExternalRef | `UNIQUE(subject_type, subject_id, system)` | `jira:PROJ-123` |
| Actor | `UNIQUE(handle)` | `ender`, `agent:claude` |
| Changeset | `UNIQUE(branch)` | `agent/att-fr-012` (the branch name) |
| Review | `UNIQUE(changeset_id, reviewer_id)` | — |
| Comment | — (surrogate-only) | — |

(Pure-relationship tables — `Edge`, `EntityRef`, `TestResult`, junctions — have no human-facing key; their
composite `UNIQUE` is also their *identity*, from which the PK is derived deterministically so
duplicates converge on merge rather than collide — see the deterministic-PK rule above.)

**Derived human key.** `Requirement.fr_key` is a generated column —
`spec.prefix || '-FR-' || lpad(number, 3, '0') || coalesce(suffix, '')` → `ATT-FR-012`,
`ATT-FR-038a`. It is the citation form (`frTest`, cross-references) but **not** the PK, so
renumbering changes the string while the ULID and every FK stay put.

**Display IDs — tiered (no single style):**

1. **Where a business key exists, it *is* the display ID** — `ATT-FR-012`, `M1`, the route.
   Most of the model has one.
2. **Keyless tables** (`TestRun`, `TestResult`, `Edge`, `EntityRef`, `Deliverable`,
   `Comment`) display a **git-style id prefix** (e.g. `01JBX8Z`) and accept unambiguous prefixes on
   input — Docker/git ergonomics, free, always consistent with the PK (a ULID, or a
   deterministic UUID for `Edge` / `TestResult`).
3. **Avoid Jira-style typed sequences** (`REQ-128`) unless the number is allocated at
   merge-to-`main` time; a per-branch counter reintroduces the auto-increment merge
   collision ULID was chosen to avoid.

**Caveat (separate from the PK choice).** `Requirement.number` is itself sequential, so two
branches adding FRs to the same spec collide on the human number (`ATT-FR-013` twice) even
with ULID PKs. That is a **merge-policy** decision — renumber-on-merge, or per-agent
reserved ranges — and lines up with the corpus's existing "gaps are expected, tombstone
deletions" rule.
