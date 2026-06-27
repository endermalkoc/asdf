#!/bin/sh
# Install the ADLG (Agentic Delivery Lifecycle Graph) CLI from GitHub Releases.
#
#   curl -fsSL https://raw.githubusercontent.com/endermalkoc/adlg/main/install.sh | sh
#
# Environment overrides:
#   ADLG_VERSION       version/tag to install, e.g. v0.1.0   (default: latest release)
#   ADLG_INSTALL_DIR   directory to install into             (default: /usr/local/bin,
#                                                             falling back to ~/.local/bin)
#
# The installed binary is named `adlg`. Set ADLG_INSTALL_DIR to choose where it
# lands on PATH.

set -eu

REPO="endermalkoc/adlg"
BINARY="adlg"

log()  { printf '%s\n' "$*" >&2; }
err()  { printf 'error: %s\n' "$*" >&2; exit 1; }
have() { command -v "$1" >/dev/null 2>&1; }

# ── pick a downloader ───────────────────────────────────────────────────────
if have curl; then
  dl() { curl -fsSL "$1"; }
  dlo() { curl -fsSL -o "$2" "$1"; }
elif have wget; then
  dl() { wget -qO- "$1"; }
  dlo() { wget -qO "$2" "$1"; }
else
  err "need curl or wget to download"
fi

# ── detect OS / arch (must match GoReleaser's GOOS/GOARCH names) ─────────────
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux)  os=linux ;;
  darwin) os=darwin ;;
  *) err "unsupported OS: $os (use 'go install $REPO/cmd/$BINARY@latest' instead)" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) err "unsupported architecture: $arch" ;;
esac

# ── resolve version ─────────────────────────────────────────────────────────
version="${ADLG_VERSION:-}"
if [ -z "$version" ]; then
  log "Resolving latest release…"
  version=$(dl "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name":' | head -n1 | sed 's/.*"tag_name": *"\([^"]*\)".*/\1/')
  [ -n "$version" ] || err "could not determine latest release (set ADLG_VERSION=vX.Y.Z to pin one)"
fi
vnum=${version#v} # archive names use the version without the leading 'v'

asset="${BINARY}_${vnum}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$version"

log "Installing $BINARY $version ($os/$arch)…"

# ── download + verify + extract in a temp dir ───────────────────────────────
tmp=$(mktemp -d 2>/dev/null || mktemp -d -t adlg)
trap 'rm -rf "$tmp"' EXIT INT TERM

dlo "$base/$asset" "$tmp/$asset" || err "download failed: $base/$asset"

# Verify the checksum when a hashing tool is available.
if dlo "$base/checksums.txt" "$tmp/checksums.txt" 2>/dev/null; then
  want=$(grep " $asset\$" "$tmp/checksums.txt" | awk '{print $1}')
  if [ -n "$want" ]; then
    if have sha256sum; then
      got=$(sha256sum "$tmp/$asset" | awk '{print $1}')
    elif have shasum; then
      got=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')
    else
      got=""
    fi
    if [ -n "$got" ]; then
      [ "$got" = "$want" ] || err "checksum mismatch for $asset"
      log "Checksum verified."
    else
      log "warning: no sha256 tool found; skipping checksum verification"
    fi
  fi
else
  log "warning: checksums.txt unavailable; skipping checksum verification"
fi

tar -xzf "$tmp/$asset" -C "$tmp" || err "failed to extract $asset"
[ -f "$tmp/$BINARY" ] || err "$BINARY not found in archive"
chmod +x "$tmp/$BINARY"

# ── choose an install dir and place the binary ──────────────────────────────
dir="${ADLG_INSTALL_DIR:-/usr/local/bin}"
install_to() { # $1 = dir
  mkdir -p "$1" 2>/dev/null || return 1
  if [ -w "$1" ]; then
    mv "$tmp/$BINARY" "$1/$BINARY"
  elif have sudo; then
    log "Elevating with sudo to write to $1…"
    sudo mv "$tmp/$BINARY" "$1/$BINARY"
  else
    return 1
  fi
}

if install_to "$dir"; then
  :
elif [ -z "${ADLG_INSTALL_DIR:-}" ] && install_to "$HOME/.local/bin"; then
  dir="$HOME/.local/bin"
else
  err "could not write to $dir (set ADLG_INSTALL_DIR to a writable directory)"
fi

log ""
log "Installed: $dir/$BINARY"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) log "NOTE: $dir is not on your PATH — add it, e.g.:"
     log "      export PATH=\"$dir:\$PATH\"" ;;
esac
log "Run '$BINARY version' to verify."
