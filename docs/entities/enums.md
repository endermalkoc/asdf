# Enum reference

[← index](index.md)

Consolidated value sets for every enum in the model. Values are `snake_case`, lowercase.

Each set is one of three **policy buckets** ([decisions.md](decisions.md)):
- **closed** (default, unmarked) — a fixed lifecycle/workflow or structural discriminator; unknown values are
  rejected (`enums.Valid`).
- **seed** — an open value-set with documented defaults; project-/tenant-/tooling-specific, so unknown values
  are **accepted with a warning** (`--strict` rejects). Not baked into the core.
- **table** — a data-driven lookup table (carries attributes/policy), configurable by adding rows.

| Enum | Values |
|---|---|
| Domain.status | `draft`, `active`, `deprecated` |
| Spec.status | `draft`, `reviewed`, `active`, `obsolete` |
| Requirement.content_status | `draft`, `active`, `obsolete` |
| UserStory.priority / Requirement.priority / TestCase.priority | `0`–`4` (INT level, `0` = most urgent) — **table** (`req_priority`: `0` Critical, `1` High, `2` Medium, `3` Low, `4` Backlog; carries `label`/`description`). The one standard priority scheme (migration `0018`) |
| Requirement.delivery_status | `covered`, `test-pending`, `not-implemented`, `e2e-sufficient`, `shared`, `schema-only`, `deferred` — **table** (`delivery_status`: carries `counts_as_covered` / `requires_e2e_test` / `requires_shared_test` / `requires_milestone`) |
| Milestone.status | `complete`, `in_progress`, `pending` |
| Milestone.slug | `M0`–`M7`, `Future`, `backlog` — **seed** (open string) |
| TestCase.layer | `unit`, `integration`, `e2e`, `component`, `shared` — **seed** (Qase) |
| TestCase.type | `functional`, `smoke`, `regression`, `acceptance`, `other` — **seed** (Qase) |
| TestCase.severity | `trivial`, `minor`, `normal`, `major`, `critical`, `blocker` — **seed** (Qase) |
| TestCase.automation | `manual`, `automated`, `to_be_automated` — **seed** (Qase) |
| TestCase.status | `draft`, `active`, `deprecated` |
| TestRun.status | `active`, `complete`, `aborted` |
| TestResult.status | `passed`, `failed`, `blocked`, `skipped`, `invalid`, `in_progress` |
| Edge.kind | `references`, `refines`, `depends_on`, `supersedes`, `relates`, `defers_to` |
| EntityRef.owner_type | `domain`, `spec`, `requirement`, `user_story`, `entity`, `milestone`, `glossary_term` |
| EntityRef.target_type | `domain`, `spec`, `requirement`, `entity`, `milestone`, `glossary_term` |
| GlossaryTerm.status | `draft`, `active`, `deprecated` |
| Capability.level | `domain`, `epic`, `capability` |
| Deliverable.size | `S`, `M`, `L`, `XL` |
| Deliverable.status | `proposed`, `specced`, `wired`, `built`, `ship` |
| Deliverable.ai_ready | `yes`, `no`, `na` |
| Entity.status | `draft`, `active`, `deprecated` |
| EntityRelationship.cardinality | `one_to_one`, `one_to_many`, `many_to_many` |
| SpecSectionType.origin / EntitySectionType.origin | `builtin` (seed), `authored` (added later) |
| SpecSectionType.slug | **table** (`req_spec_section_type` seed): `preamble`, `overview`, `edge_cases`, `more_info`, `success_criteria`, `assumptions`, `scope`, `open_questions`, `notes` |
| EntitySectionType.slug | **table** (`ent_entity_section_type` seed): `preamble`, `purpose`, `key_concepts`, `schema_reference`, `relationships`, `business_rules`, `validations`, `access_control`, `notes`, `references` |
| ExternalRef.subject_type | `deliverable`, `requirement`, `test_result` |
| ExternalRef.system | `jira`, `rally`, `beads`, `linear`, `github`, `other` (open string) |
| Actor.kind | `human`, `agent` |
| Changeset.status | `draft`, `open`, `changes_requested`, `approved`, `denied`, `merged`, `closed` |
| Review.verdict | `approve`, `deny`, `request_changes` |
| Comment.subject_type | `requirement`, `spec`, `user_story`, `test_case`, `entity`, `deliverable` (nullable) |
