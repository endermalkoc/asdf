# Interop layer

[← index](index.md) · see the [master diagram](index.md#master-diagram).

## ExternalRef
A pointer from an ADLG node to its identity in an external task system. ADLG is the system
of record; this is interop only and is deliberately **not** tied to any single tracker.
Polymorphic subject — the confirmed subjects are **`Deliverable`** (the work unit),
**`Requirement`** (FR ↔ external requirement/issue), and **`TestResult`** (imported result
↔ external run/result).

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `subject_type` | enum | | `deliverable`, `requirement`, `test_result` |
| `subject_id` | bigint / uuid | | Polymorphic FK (`subject_type` + `subject_id`) |
| `system` | varchar | | `jira`, `rally`, `beads`, `linear`, `github`, `other` |
| `external_id` | varchar | | The id/key in that system (e.g. `PROJ-123`, `tut-6qjq0`) |
| `url` | varchar | | Deep link; nullable |

> Suggested `UNIQUE(subject_type, subject_id, system)` — one id per subject per system,
> while still allowing the same item to be tracked in several systems.
