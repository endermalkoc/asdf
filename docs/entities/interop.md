# Interop layer

[← index](index.md) · see the [master diagram](index.md#master-diagram).

## ExternalRef
A pointer from a Cusp node to its identity in an external system — a task tracker, or the
source a node was imported from. Cusp is the system of record; this is interop only and is
deliberately **not** tied to any single tool. Polymorphic subject — the confirmed subjects
are **`Deliverable`** (the work unit), **`Requirement`** (FR ↔ external requirement/issue),
**`TestResult`** (imported result ↔ external run/result), and the planning rows
**`Capability`** / **`View`** (linked back to the Notion page they were imported from). The
set widens as real consumers appear (the Notion importer added Capability/View).

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `subject_type` | enum | | `deliverable`, `requirement`, `test_result`, `capability`, `view` |
| `subject_id` | bigint / uuid | | Polymorphic FK (`subject_type` + `subject_id`) |
| `system` | varchar | | open string: `jira`, `rally`, `beads`, `linear`, `github`, `notion`, `other`, … |
| `external_id` | varchar | | The id/key in that system (e.g. `PROJ-123`, `tut-6qjq0`) |
| `url` | varchar | | Deep link; nullable |

> Suggested `UNIQUE(subject_type, subject_id, system)` — one id per subject per system,
> while still allowing the same item to be tracked in several systems.
