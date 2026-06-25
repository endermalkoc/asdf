# ASDF — Agentic Software Development Framework

> **Status: early implementation.** The Dolt infrastructure (salvaged from
> [beads](https://github.com/steveyegge/beads), MIT; see [NOTICE](NOTICE)), the schema, **`asdf
> init`**, the command contract, the core verbs (`domain`/`spec`/`req`/`edge`), and the
> **changeset (PR) flow** are built and verified against real Dolt. Generation, `check`/`impact`,
> remote sync, the MCP server, and import are next — see [ROADMAP.md](docs/ROADMAP.md). Some
> commands in the table below run today; others are still the *planned* surface.

ASDF is a command-line tool, backed by a [Dolt](https://www.dolthub.com/) database, that
serves as the **version-controlled system of record for everything a software project knows
about itself** — domains, specs, user stories, functional requirements, tests, milestones,
deliverables, and the links between them. It is built to be driven equally by **humans and
coding agents** (Claude, Codex, Cursor, opencode, …).

## The problem

The new wave of spec-driven, agentic tools — Kiro, GitHub's Spec Kit, and the rest — gets the
premise right: write the spec first, then build to it. But they treat the spec as
**scaffolding, not an artifact**. An agent emits a `requirements.md` and a `design.md`, builds
against them once, and moves on. What it leaves behind doesn't hold together:

- **Specs aren't first-class.** They're loose markdown files with no stable identity — no id a
  test, a commit, or another spec can point at.
- **There's no traceability.** Nothing answers "which test covers this requirement?", "which
  spec does it come from?", or "what else depends on it?" — those links were never recorded.
- **Changes aren't tracked.** Regenerate the file and the previous version is gone: no history
  of how a requirement evolved, who changed it, or why.
- **Nothing is verifiable.** "Every requirement is covered by a test" is a claim no tool can
  check, because requirements and tests live in separate places that never reference each other.

The pre-agentic answer — a markdown repo plus an FR-traceability spreadsheet plus a Notion
board plus Qase plus Jira — has the same disease in slower motion: the pieces drift, none of it
is one queryable graph, and nothing reads or writes across all of it.

## The idea

Put it all in one place, as one graph, in a database that branches and merges like code. ASDF
**inverts the model**: the database is the first-class artifact, and the Markdown is the
throwaway scaffolding — the exact opposite of treating generated `.md` files as the spec. That
inversion is what fixes the four failures above:

- **One source of truth.** The Dolt database is canonical — always. Specs, requirements, and
  tests are real records with stable ids, not loose files. *(first-class)*
- **Generated, never edited.** Human- and agent-friendly **Markdown and HTML are
  auto-generated** from the DB. They are **git-ignored build artifacts** — never
  hand-edited, never agent-edited. Change content through the CLI/MCP, then regenerate.
- **Branch / merge / diff.** Dolt gives the project's knowledge the same version-control
  model as its code: agents work on branches, you review and merge — and every change to a
  spec is a commit you can diff, blame, and revert. *(changes tracked)*
- **Two interfaces, one store.** Humans and agents **read, write, and validate through the
  CLI or an MCP server**. The generated Markdown is an optional *fast read path* for
  agents; it is never a write path.
- **Traceability & impact.** Because everything is one linked graph, ASDF can **validate**
  (e.g. every requirement covered by a test, every deliverable linked) and answer
  **impact** questions ("what breaks if this requirement changes?"). *(traceable + verifiable)*
- **Generic core.** ASDF is domain-agnostic. Its data model was pressure-tested against a
  real corpus, but the core carries no project- or tenant-specific assumptions.

## Who it's for (and who it isn't)

ASDF is infrastructure, and infrastructure is overhead until the thing it manages gets big
enough to lose track of. Below that line, skip it:

- **Not** a weekend project or a vibe-coding session — the ceremony will only slow you down.
- **Not** a small, well-bounded domain with a handful of requirements. Today's agents hold
  that much in working memory and build it well; a system of record buys you nothing.

It's built for the other end of the curve: **medium-to-large applications with enough
requirements, specs, and tests that no one — human or agent — can keep the whole graph in their
head.** When there are hundreds of requirements across many domains, when specs outlive the
session that wrote them, and when "what does this change break?" stops having an obvious answer,
traceability stops being overhead and becomes the thing that keeps the project coherent.

## How it flows

```
             write / validate (CLI · MCP)
  human  ──────────────────────────────▶  ┌──────────────┐  generate   ┌─────────────────────────┐
  agent  ◀──────────────────────────────  │   Dolt  DB   │ ──────────▶ │  Markdown + HTML        │
             query / read (CLI · MCP)      │ (canonical)  │             │  (git-ignored, read-only)│
                                           └──────────────┘             └─────────────────────────┘
                                                                          ▲ agents may read these
                                                                            for fast consumption
```

## Data model

The full entity-relationship model lives in **[docs/entities/](docs/entities/index.md)**. In brief, by layer:

- **Structure** — `Domain`, `Spec` (the document tree; directories derive from a spec's `path`)
- **Requirements** — `UserStory`, `AcceptanceScenario`, `Requirement` (FR), `Milestone`, `Edge` (the cross-reference graph)
- **Testing** (Qase-style) — `TestSuite`, `TestCase`, `TestStep`, `TestRun`, `TestResult`, `Configuration`
- **Planning** — `Capability`, `Deliverable`, `View`
- **Authorization & entities** — `Entity`, `EntityAttribute`, `EntityRelationship`, `Privilege`, `AccessRule` (authored *business-domain* documents, **not** a DB-schema mirror)
- **Interop** — `ExternalRef` (a node's id in an outside tracker: Jira, Rally, beads, …)

Identifiers use ULID surrogate keys plus human-readable unique business keys (e.g.
`ATT-FR-012`). See [Identifiers & keys](docs/entities/identifiers.md).

## Planned commands (illustrative)

The full intended surface. For the subset that **runs today** — with examples — see [Usage](#usage).

| Command | Purpose |
|---|---|
| `asdf init` | Create / connect the Dolt database |
| `asdf spec` · `asdf req` · `asdf test` … | Create, link, and query nodes |
| `asdf query <…>` | Ad-hoc queries over the graph |
| `asdf generate` | Regenerate Markdown + HTML from the DB |
| `asdf check` | Validate traceability / consistency |
| `asdf impact <id>` | Show what a change affects |
| `asdf import <source>` | Generic migration import |
| `asdf serve --mcp` | Run the MCP server |

## Install

> Releases are cut by [GoReleaser](https://goreleaser.com) on every `v*` tag and published to
> [GitHub Releases](https://github.com/endermalkoc/asdf/releases) as static, single-file
> binaries for Linux/macOS/Windows (amd64 + arm64). The CLI surface is still early — `asdf
> version` works today; data commands need a running Dolt server (see [build/run](CLAUDE.md#build--run)).

**Install script** (Linux/macOS — downloads the right binary and verifies its checksum):

```sh
curl -fsSL https://raw.githubusercontent.com/endermalkoc/asdf/main/install.sh | sh
```

Pin a version or change the location with `ASDF_VERSION=v0.1.0` / `ASDF_INSTALL_DIR=~/.local/bin`.

**With Go** (any platform):

```sh
go install github.com/endermalkoc/asdf/cmd/asdf@latest
```

**From source:**

```sh
git clone https://github.com/endermalkoc/asdf && cd asdf
make build   # → ./asdf   (make install puts it on your PATH)
```

> The binary is named `asdf`, which collides with the [asdf version manager](https://asdf-vm.com)
> (see the name note below); use `ASDF_INSTALL_DIR` to control PATH precedence.

## Usage

What runs today. (For the full planned surface see the [illustrative table](#planned-commands-illustrative) above.)

> **Prerequisite:** the [`dolt`](https://github.com/dolthub/dolt) binary on your `PATH`. `asdf init`
> starts and manages a `dolt sql-server` for you — no separate database setup. To use a server you
> run yourself, pass `--dsn` (or set `ASDF_DSN`). From a source checkout, every `asdf <cmd>` below
> can be run as `go run ./cmd/asdf <cmd>`.

### Quickstart

```sh
# 1. Create a workspace in your repo (starts a managed Dolt server, applies the schema)
asdf init

# 2. Add a domain, a spec under it, and a requirement
asdf domain add enrollment Enrollment --description "Student lifecycle"
asdf spec   add enrollment/add-student.md --domain enrollment --prefix ADDS --title "Add Student"
asdf req    add ADDS "System MUST require first and last name" --delivery covered

# 3. List what you created
asdf domain ls
asdf req ls ADDS

# 4. Link two requirements in the cross-reference graph
asdf req  add ADDS "System MUST default Student Type to Child"
asdf edge add ADDS-FR-002 refines ADDS-FR-001
```

Each write becomes **one Dolt commit** attributed to you (`--actor`, else your git user / `$USER`).

### Changesets (the PR model)

Bundle many edits onto one reviewable branch instead of committing each to `main`:

```sh
asdf changeset start "wave 1"     # opens changeset/wave-1 and makes it the active target
asdf req add ADDS "..."           # subsequent edits land on the changeset, not main
asdf changeset diff               # combined diff vs base (the PR view)
asdf changeset submit             # mark open for review (records the head commit)
asdf changeset merge              # merge into main   (or: abandon to discard + delete the branch)
asdf changeset ls                 # list changesets (* marks the active one); alias: asdf cs
```

Any mutating command also accepts `--changeset <name>` for a one-off target without making it active.

### Import the tutor corpus

A deterministic, no-LLM parse of the `tutor` documentation corpus into ASDF's entity shapes:

```sh
asdf import tutor ../tutor/docs                    # parse + report (counts, coverage, drift) — no writes
asdf import tutor ../tutor/docs --json             # the full staged graph + report as JSON
asdf import tutor ../tutor/docs --apply            # load it through the command contract (one commit)
asdf import tutor ../tutor/docs --apply --changeset import-tutor   # …onto a changeset branch
```

`--apply` is idempotent — re-running converges instead of duplicating.

### Command reference (working today)

| Command | Purpose |
|---|---|
| `asdf init` | Create `.asdf/` + a managed Dolt database in the current repo |
| `asdf version` | Print version, commit, and build date |
| `asdf domain add <abbr> <name>` | Add a domain (`--kind`, `--description`) |
| `asdf domain ls` | List domains |
| `asdf spec add <path>` | Add a spec doc (`--domain` required; `--prefix`, `--title`, `--kind`) |
| `asdf req add <spec-prefix> <statement>` | Add a requirement — auto-numbers, derives the FR key (`--delivery`, `--milestone-id`) |
| `asdf req ls <spec-prefix>` | List a spec's requirements |
| `asdf edge add <from-fr> <kind> <to-fr>` | Link two requirements (`kind`: references \| refines \| depends_on \| supersedes \| relates \| defers_to) |
| `asdf changeset start \| diff \| submit \| merge \| abandon \| ls` | The changeset (PR) flow (alias `cs`) |
| `asdf import tutor <docs>` | Parse the tutor corpus; add `--apply` to load it |

**Global flags:** `--json` (machine-readable output) · `--actor <handle>` (attribution) ·
`--changeset <name>` (target a changeset) · `--dsn <dsn>` (use an external Dolt server, env `ASDF_DSN`).

## Tech stack

**Go** (locked) — single static binary across Windows/Mac/Linux, Dolt is itself Go, exposing
**both a CLI and an MCP server**. Its Dolt infrastructure was salvaged directly from
[beads](https://github.com/steveyegge/beads) (MIT), which made the precedent the foundation.
See [ARCHITECTURE.md](docs/ARCHITECTURE.md#tech-stack).

## Roadmap (high level)

1. Generate the Dolt DDL from the [data model](docs/entities/index.md).
2. Core CLI: create / link / query + the generation pipeline.
3. Validation (`check`) and impact analysis.
4. MCP server.
5. **Generic import tool**; first real migration: the `tutor/docs` corpus.

Inspired by — and building on the Dolt infrastructure of — [beads](https://github.com/steveyegge/beads).

## Docs

- [docs/entities/](docs/entities/index.md) — the data model
- [ARCHITECTURE.md](docs/ARCHITECTURE.md) — architecture & principles
- [ROADMAP.md](docs/ROADMAP.md) — what's built, what's next
- [docs/codebase-map.md](docs/codebase-map.md) — folder map + fresh-session orientation
- [docs/build-and-release.md](docs/build-and-release.md) — build, versioning, release, install, `.gitignore` policy
- [CLAUDE.md](CLAUDE.md) — guidance for agents and contributors

> **Name note:** "asdf" collides with the popular [asdf version manager](https://asdf-vm.com/);
> the published binary / command name may change.

## License

MIT — see [LICENSE](LICENSE).
