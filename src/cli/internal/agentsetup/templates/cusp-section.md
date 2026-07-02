## Cusp — project source of truth

This project's **specs, requirements, tests, and plans** live in **Cusp** — a Dolt-backed,
version-controlled graph edited through the `cusp` CLI, not in hand-written Markdown.

**At session start, run `cusp prime`** for live state (active/open changesets, review status,
counts) and the full workflow. Core rules:

- The Cusp **database is the single source of truth.** Generated Markdown/HTML are build
  artifacts — never hand-edit them; change content through `cusp` and regenerate.
- Group edits in a **changeset** (the PR model): `cusp changeset start "<title>"` → edit
  (`cusp req add|edit`, `cusp spec add`, `cusp entity …`, `cusp test …`, `cusp edge add`) →
  `cusp changeset diff` → `cusp changeset submit` → `cusp changeset merge`.
- Read with `cusp req tree` / `cusp search` / `cusp stats` / `cusp impact`; gate with `cusp check`.
- Prefer `cusp <command> --json` for machine-readable output; run `cusp <command> --help` for details.
