---
name: cusp
description: >-
  Use when working in a repository that uses Cusp (a `.cusp/` workspace) as the
  version-controlled source of truth for specs, requirements, tests, and plans. Trigger when
  the user asks to read or change requirements / specs / entities / tests / plans, review or
  merge a changeset, check coverage or impact, or recover project context. Everything is driven
  through the `cusp` CLI — never hand-edit the generated Markdown.
---

# Cusp

This repository uses **Cusp**: a Dolt-backed, version-controlled graph of the project's specs,
requirements, tests, and plans, edited through the `cusp` CLI.

## First step

Run **`cusp prime`** to load live workspace state (active/open changesets, review status,
counts) and the current workflow. A SessionStart hook may already have injected this — if so,
you don't need to run it again.

## Rules

- The Cusp **database is the single source of truth.** Generated Markdown/HTML are build
  artifacts — **never hand-edit them**; change content through `cusp` and regenerate.
- All writes go through `cusp` (or its MCP server). You may *read* generated Markdown for speed,
  but it is never a write path.
- Prefer `cusp <command> --json` for machine-readable output; consult `cusp <command> --help`.

## Changeset workflow (the PR model)

1. `cusp changeset start "<title>"` — begin; it becomes the active changeset.
2. Edit — `cusp req add|edit`, `cusp spec add`, `cusp entity …`, `cusp test …`, `cusp edge add`.
3. `cusp changeset diff` — review the pending change.
4. `cusp check` — integrity gate (dangling `[[TYPE:key]]` refs, edge cycles).
5. `cusp changeset submit` — mark ready; `cusp review --verdict …` / `cusp comment add` to review.
6. `cusp changeset merge` — land it on `main`.

## Reading & analysis

- `cusp req tree`, `cusp search <text>`, `cusp stats` — browse and search the graph.
- `cusp impact <TYPE:key>` — what references or depends on an entity (blast radius).
- `cusp ref add|ls` — external references (jira / github / …) for a subject.
