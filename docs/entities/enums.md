# Enum reference

[← index](index.md)

Consolidated value sets for every enum in the model. Values are `snake_case`, lowercase.

| Enum | Values |
|---|---|
| Domain.kind | `service`, `shared`, `infrastructure`, `entities`, `analysis` |
| Spec.kind | `feature`, `entity`, `journey`, `analysis`, `index`, `meta`, `reference` |
| Spec.status | `draft`, `reviewed`, `active`, `obsolete` |
| Requirement.content_status | `draft`, `active`, `obsolete` |
| UserStory.priority | `P1`, `P2`, `P3` |
| Requirement.delivery_status | `covered`, `test-pending`, `not-implemented`, `e2e-sufficient`, `shared`, `schema-only`, `deferred` |
| Requirement.optout_marker | `none`, `visual`, `ops`, `untestable` |
| Milestone.status | `complete`, `in_progress`, `pending` |
| Milestone.abbreviation | `M0`–`M7`, `Future`, `backlog` (open string; seed/policy) |
| TestCase.layer | `unit`, `integration`, `e2e`, `component`, `shared` |
| TestCase.type | `functional`, `smoke`, `regression`, `acceptance`, `other` |
| TestCase.priority | `low`, `medium`, `high` |
| TestCase.severity | `trivial`, `minor`, `normal`, `major`, `critical`, `blocker` |
| TestCase.automation | `manual`, `automated`, `to_be_automated` |
| TestCase.status | `draft`, `active`, `deprecated` |
| TestRun.status | `active`, `complete`, `aborted` |
| TestResult.status | `passed`, `failed`, `blocked`, `skipped`, `invalid`, `in_progress` |
| Edge.kind | `references`, `refines`, `depends_on`, `supersedes`, `relates`, `defers_to` |
| EntityRef.owner_type | `domain`, `spec`, `requirement`, `user_story`, `entity`, `milestone`, `glossary_term` |
| EntityRef.target_type | `domain`, `spec`, `requirement`, `entity`, `milestone`, `glossary_term` |
| EntityRef.kind | `references` |
| GlossaryTerm.status | `draft`, `active`, `deprecated` |
| Capability.level | `domain`, `epic`, `capability` |
| Deliverable.size | `S`, `M`, `L`, `XL` |
| Deliverable.status | `proposed`, `specced`, `wired`, `built`, `ship` |
| Deliverable.ai_ready | `yes`, `no`, `na` |
| Privilege.scope / action | `owned` \| `studio` / `view` \| `manage` |
| EntityRelationship.cardinality | `one_to_one`, `one_to_many`, `many_to_many` |
| DocSection.owner_type | `spec`, `entity` |
| ExternalRef.subject_type | `deliverable`, `requirement`, `test_result` |
| ExternalRef.system | `jira`, `rally`, `beads`, `linear`, `github`, `other` (open string) |
| Actor.kind | `human`, `agent` |
| Changeset.status | `draft`, `open`, `changes_requested`, `approved`, `denied`, `merged`, `closed` |
| Review.verdict | `approve`, `deny`, `request_changes` |
| Comment.subject_type | `requirement`, `spec`, `user_story`, `test_case`, `entity`, `deliverable` (nullable) |
