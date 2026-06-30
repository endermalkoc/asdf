# Cusp — VS Code extension

The human **review surface** for [Cusp](../../README.md): browse changesets and (soon)
review the spec/requirement/plan diff they introduce — comment on a requirement row, set a
verdict — without leaving the editor. See the rationale in
[ROADMAP.md → Review surface](../../docs/ROADMAP.md).

The extension holds **no state**. Dolt is the source of truth; this is a third front-end over
the same `Mutate` contract the CLI uses. It reaches a Cusp workspace through a
transport-agnostic [`CuspClient`](src/cusp/client.ts) — today implemented by shelling out to
the `cusp` CLI ([`CliCuspClient`](src/cusp/cliClient.ts)), later swappable for an MCP transport
with no UI changes.

## Prerequisites

- **Node.js + npm** (for building the extension).
- The **`cusp` CLI** on `PATH` (or set `cusp.cliPath`), and a Cusp workspace — a directory
  containing `.cusp` (run `cusp init` to make one).

## The dev loop (edit → reload)

A VS Code extension is a separate Node/TypeScript project. The "keep it refreshed during
development" loop is **watch-compile + reload-window**:

1. **Install deps** (once): `npm install`
2. **Open this folder** (`src/extension`) in VS Code — *not* the repo root. The committed
   `.vscode/launch.json` + `.vscode/tasks.json` only apply when this is the opened folder.
3. **Press `F5`** ("Run Cusp Extension"). This:
   - runs `npm run watch` (esbuild rebuilds `dist/extension.js` on every save), then
   - launches a second window — the **Extension Development Host** — with the extension loaded
     and the debugger attached. Breakpoints in `src/**/*.ts` work; `console.log` goes to the
     **Debug Console** of *this* (the first) window.
4. In the **Extension Development Host** window, open a folder that contains a `.cusp`
   workspace, then click the **Cusp** icon in the activity bar to see the Changesets view.
5. **Edit code → save** (watch recompiles automatically) → in the Extension Development Host
   press **`Ctrl+R`** / **`Cmd+R`** (*Developer: Reload Window*). That reload is the refresh —
   it loads the freshly built bundle.

That's the whole cycle: **save → `Ctrl+R`**. It's reload-window, not browser-style
hot-reload — esbuild recompiles for you; you reload the host to pick it up.

> Prefer a terminal? Run `npm run watch` yourself and use F5 only to launch/relaunch the host.

## Where it runs (WSL / Remote-SSH / containers)

The extension is a **workspace** extension: VS Code runs its host process wherever the
workspace lives. Open a WSL/SSH/dev-container folder and the extension host — and therefore the
`cusp` subprocess it spawns — runs **on that same machine**, next to the `.cusp` data. So
shelling out to `cusp` "just works" across all of those topologies; you never have the
extension on one machine reaching a CLI on another.

## Settings

| Setting | Default | Meaning |
|---|---|---|
| `cusp.cliPath` | `cusp` | Path to the `cusp` binary (resolved on `PATH` by default). |
| `cusp.workspaceFolder` | _(empty)_ | Absolute path to the workspace holding `.cusp`. Empty → first open workspace folder. |

## Scripts

| Command | What |
|---|---|
| `npm run watch` | esbuild in watch mode (the dev build). |
| `npm run compile` | one-shot dev build. |
| `npm run package` | minified production build (`dist/extension.js`). |
| `npm run check-types` | `tsc --noEmit` type check (esbuild does not type-check). |
| `npm run vsce:package` | produce an installable `.vsix` (`@vscode/vsce`). |

## Layout

```
src/extension.ts              activate() — wires the tree + commands
src/cusp/client.ts            CuspClient — the transport-agnostic contract
src/cusp/cliClient.ts         CliCuspClient — runs `cusp … --json`
src/changesets/changesetTree.ts  the Changesets TreeDataProvider
esbuild.js                    bundler (dev / watch / production)
.vscode/launch.json|tasks.json  the F5 dev loop
```
