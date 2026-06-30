# Review & collaboration layer

[← index](index.md) · see the [master diagram](index.md#master-diagram).

Human review of agent (or human) changes — **approve / deny / comment** — layered on top of
Dolt's native version control. **Dolt provides history and diff; these tables provide the
workflow state Dolt has no concept of.** The bridge is `Changeset`, which stores the
Dolt branch + commit coordinates so the UI computes the actual diff/history *from Dolt* on
demand.

History and diffs are deliberately **not** modeled here — query Dolt directly:
`dolt_history_<table>`, `dolt_diff_<table>` / `dolt_commit_diff_<table>`, `dolt_log`,
`dolt_blame_<table>`.

Status → Dolt operation: **approve → `dolt_merge`** (records `merge_commit`);
**deny → close** (optionally drop the branch); **comment / request-changes → branch stays
open**.

> **Where these rows live:** keep the review tables on a long-lived branch (e.g. `main`),
> **not** inside the changeset's own branch — otherwise the review record would merge away (or
> vanish) with the change it describes.

## Actor
Identity for attribution — humans and agents. Maps to the Dolt committer via `handle`, which
is how commits made on a branch are correlated back to a reviewer/author.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `kind` | enum | | `human`, `agent` |
| `name` | varchar | | Display name |
| `handle` | varchar | **UK** | Stable identity (email/username); maps to the Dolt committer |
| `agent_tool` | varchar | | `claude`, `codex`, `cursor`, `opencode`, … — agents only; nullable |

> `Actor` can later back free-text attribution fields elsewhere (`Requirement.owner`,
> `TestResult.executed_by`); those stay plain strings for now.

## Changeset
The reviewable unit (PR-like) **and** the bridge to Dolt. The diff under review is computed
from `base_commit`→`head_commit` on `branch`; it is never duplicated into Cusp tables.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `title` | varchar | | |
| `description` | text | | |
| `author_id` | FK → Actor | | Who proposed the change |
| `status` | enum | | `draft`, `open`, `changes_requested`, `approved`, `denied`, `merged`, `closed` |
| `branch` | varchar | **UK** | The Dolt branch holding the proposed change |
| `base_commit` | varchar | | Merge base |
| `head_commit` | varchar | | Branch head under review |
| `merge_commit` | varchar | | Set when approved & merged; nullable |
| `created_at` / `updated_at` | datetime | | |

## Review
A reviewer's disposition on a changeset. Multiple reviewers per changeset; one *current*
verdict each (re-review updates the row).

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `changeset_id` | FK → Changeset | | |
| `reviewer_id` | FK → Actor | | |
| `verdict` | enum | | `approve`, `deny`, `request_changes` |
| `summary` | text | | |
| `created_at` | datetime | | |

> `UNIQUE(changeset_id, reviewer_id)` — one current verdict per reviewer per changeset.

## Comment
Threaded discussion on a changeset, optionally **anchored** to a specific node and location
within the diff.

| Attribute | Type | Key | Notes |
|---|---|---|---|
| `id` | bigint / uuid | **PK** | |
| `changeset_id` | FK → Changeset | | |
| `author_id` | FK → Actor | | |
| `parent_id` | FK → Comment | | Self-ref for threading; nullable |
| `body` | text | | |
| `subject_type` | enum | | Polymorphic anchor: `requirement`, `spec`, `user_story`, `test_case`, `entity`, `deliverable`; **nullable** (changeset-level comment) |
| `subject_id` | bigint / uuid | | The anchored node; nullable |
| `locator` | varchar | | Field / diff-hunk locator within the subject; nullable |
| `resolved` | bool | | |
| `created_at` / `updated_at` | datetime | | |
