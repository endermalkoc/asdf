# Build & Release

How ASDF is built, versioned, released, and installed — and what the `.gitignore` policy is.
Pairs with [ARCHITECTURE.md](ARCHITECTURE.md) (how it's put together), the
[Build / run](../CLAUDE.md#build--run) notes in CLAUDE.md, and [ROADMAP.md](ROADMAP.md).

ASDF ships as a **single static Go binary**. The build is pure Go (`CGO_ENABLED=0`), so it
cross-compiles to every target with no C toolchain — a darwin/arm64 binary builds on a Linux
host in seconds. That makes [GoReleaser](https://goreleaser.com) a clean fit for cutting
cross-platform releases.

## Components

| File | Purpose |
|---|---|
| [cmd/asdf/version.go](../cmd/asdf/version.go) | `asdf version` (+ `--version`, `--json`). Version/commit/date are injected at build time; falls back to embedded VCS build info for `go install` builds. |
| [.goreleaser.yaml](../.goreleaser.yaml) | Cross-builds linux/darwin/windows × amd64/arm64, archives (`tar.gz`, `zip` on Windows), `checksums.txt`, changelog. |
| [.github/workflows/ci.yml](../.github/workflows/ci.yml) | On PR/push: `go vet` + `go build` + `go test`, plus a GoReleaser snapshot **dry-run** so release breakage is caught before tagging. |
| [.github/workflows/release.yml](../.github/workflows/release.yml) | On a `v*` tag push: runs GoReleaser → publishes a GitHub Release. |
| [install.sh](../install.sh) | `curl \| sh` installer: detects OS/arch, resolves the latest tag, downloads + verifies the checksum, installs the binary. |
| [Makefile](../Makefile) | Local dev: `make build / install / test / vet / tidy / snapshot / release-check / help`. |

## Version stamping

The build metadata lives in [cmd/asdf/version.go](../cmd/asdf/version.go) as package-level
`version` / `commit` / `date` vars (default `version = "dev"`). They are set three ways, in
order of precedence:

1. **Release builds** — GoReleaser passes `-ldflags "-X main.version={{.Version}} …"`, so the
   binary reports the exact tag (e.g. `v0.1.0`).
2. **`make build` / `make install`** — the Makefile injects `git describe --tags` + short
   commit + UTC date.
3. **`go install …/cmd/asdf@v0.1.0`** — no ldflags, so `init()` recovers the module version
   and VCS revision/time from `runtime/debug.ReadBuildInfo()`.

```
$ asdf version
asdf v0.1.0
  commit: abc123def456
  built:  2026-06-24T00:00:00Z
  go:     go1.26.2 linux/amd64
```

`asdf version --json` emits the same fields as a JSON object (consistent with the CLI's
global `--json` flag).

## Local development

```sh
make build          # → ./asdf for the host platform (CGO-free, version-stamped)
make test           # go test ./...
make snapshot       # full cross-platform build into dist/, no publish (needs goreleaser)
make release-check  # validate .goreleaser.yaml
make help           # list all targets
```

> `go` is required on PATH. See the repo's [Build / run](../CLAUDE.md#build--run) notes.

## Cutting a release

Releases are tag-driven. Push a semver tag and the [release workflow](../.github/workflows/release.yml)
does the rest:

```sh
git tag v0.1.0
git push origin v0.1.0
```

GoReleaser then builds every platform, generates `checksums.txt` and a changelog, and creates
the GitHub Release with all assets attached. It uses the Actions-provided `GITHUB_TOKEN` — **no
secrets to configure**. Tags like `v0.1.0-rc1` publish as pre-releases automatically.

> GoReleaser refuses to publish from a **dirty git tree**. Make sure no ignored/generated
> files are staged when you tag. The `before` hook uses `go mod download` (not `go mod tidy`)
> specifically so the release run can't dirty `go.mod`/`go.sum`.

### One-time setup

- The repo must be **public** at `github.com/endermalkoc/asdf` for both the install script
  (release assets must be downloadable) and `go install` to work.
- The workflows only run once `.github/workflows/` is pushed to GitHub.

## Installing (for consumers)

| Method | Command |
|---|---|
| Install script (Linux/macOS) | `curl -fsSL https://raw.githubusercontent.com/endermalkoc/asdf/main/install.sh \| sh` |
| Go | `go install github.com/endermalkoc/asdf/cmd/asdf@latest` |
| From source | `git clone … && cd asdf && make build` |

The installer honors `ASDF_VERSION=v0.1.0` (pin a version) and `ASDF_INSTALL_DIR=~/.local/bin`
(choose the location; defaults to `/usr/local/bin`, falling back to `~/.local/bin`). It verifies
the SHA-256 checksum when `sha256sum`/`shasum` is available.

> **Name collision.** The binary is named `asdf`, which clashes with the
> [asdf version manager](https://asdf-vm.com). `ASDF_INSTALL_DIR` lets consumers control PATH
> precedence; renaming the command (`binary:` in `.goreleaser.yaml`, `BINARY` in the
> Makefile/installer) remains an open option — see the name note in [README.md](../README.md).

## `.gitignore` policy

The [.gitignore](../.gitignore) keeps three classes of files out of git:

- **Generated knowledge artifacts** — `/generated/`, `*.generated.md`, `*.generated.html`.
  These are rendered from the Dolt DB (the source of truth) and must never be committed or
  hand-edited (invariant #2 in [CLAUDE.md](../CLAUDE.md)).
- **Dolt working database** — `.dolt/`. The knowledge store is its own versioned Dolt repo,
  not tracked by this git repo.
- **Build output & local config** — `/asdf`, `/dist/`, `*.exe`, `*.test`, `*.out`,
  `__debug_bin*` (VSCode/Delve), `.env*`, editor/OS files, and `.claude/settings.local.json`.

Two deliberate choices:

- `.claude/settings.local.json` (personal, machine-local) **is** ignored, but `.claude/`
  as a whole is **not** — shared project config like `.claude/settings.json` stays committable.
  This rule lives in the repo's own `.gitignore` (not only a developer's global git config) so
  it holds for every clone and CI checkout.
- Inline trailing comments are **not** valid in `.gitignore` — comments must be on their own
  line, or the `#…` becomes part of the pattern.

## Deferred

Not set up yet; the GoReleaser config is structured so each is a small addition:

- **Homebrew tap** (`brew install …`) — add a `brews:` block + a tap repo and PAT.
- **Docker image** (ghcr.io) — add a `dockers:` block.
- **Linux packages** (deb/rpm) — add an `nfpms:` block.
