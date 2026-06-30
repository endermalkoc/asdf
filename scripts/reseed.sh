#!/usr/bin/env bash
# reseed.sh — rebuild the cusp binary and reset a workspace from scratch:
# delete the database, reinitialize (applies the current schema), and re-import the
# tutor corpus. This is the fast iteration loop while the schema is still churning —
# we don't bother with forward migrations; we just rebuild from the source of truth.
#
# The managed dolt server is pinned to a fixed port (CUSP_DOLT_SERVER_PORT) so Dolt
# Workbench keeps the same connection across reseeds — no manual server, no handoff.
#
# Usage:   scripts/reseed.sh
# Tunables (env vars, with defaults):
#   WORKSPACE   workspace dir to (re)seed         (default: $HOME/cusp-tutor)
#   CORPUS      tutor docs corpus to import       (default: $HOME/repos/endermalkoc/tutor/docs)
#   PORT        managed dolt server port          (default: 3306)
#   NO_GENERATE set to 1 to skip `cusp generate`  (default: generate runs)
set -euo pipefail

WORKSPACE="${WORKSPACE:-$HOME/cusp-tutor}"
CORPUS="${CORPUS:-$HOME/repos/endermalkoc/tutor/docs}"
PORT="${PORT:-3306}"

# Repo root = parent of this script's directory.
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Make sure go and dolt are reachable (this machine keeps them off the default PATH).
for d in "$HOME/sdk/go/bin" /home/linuxbrew/.linuxbrew/bin; do
  [ -d "$d" ] && case ":$PATH:" in *":$d:"*) ;; *) PATH="$d:$PATH" ;; esac
done
command -v go   >/dev/null || { echo "reseed: 'go' not found on PATH" >&2; exit 1; }
command -v dolt >/dev/null || { echo "reseed: 'dolt' not found on PATH" >&2; exit 1; }
[ -d "$CORPUS/specs" ] || { echo "reseed: corpus not found at $CORPUS (no specs/)" >&2; exit 1; }

echo "==> building cusp from $REPO"
go build -C "$REPO" -o "$WORKSPACE/cusp" ./cmd/cusp

# Free the port: stop a dolt server still bound to it (managed or hand-started), so the
# data dir lock is released before init --force wipes and rebinds. Only kills dolt.
pid="$(ss -ltnpH "sport = :$PORT" 2>/dev/null | grep -oP 'pid=\K[0-9]+' | head -1 || true)"
if [ -n "${pid:-}" ] && tr '\0' ' ' < "/proc/$pid/cmdline" 2>/dev/null | grep -q "dolt sql-server"; then
  echo "==> stopping dolt server on :$PORT (pid $pid)"
  kill "$pid" 2>/dev/null || true
  sleep 2
fi

export CUSP_DOLT_SERVER_PORT="$PORT"
cd "$WORKSPACE"

echo "==> cusp init --force  (port $PORT)"
./cusp init --force

echo "==> cusp import tutor --apply"
./cusp import tutor "$CORPUS" --apply

if [ "${NO_GENERATE:-0}" != "1" ]; then
  echo "==> cusp generate"
  ./cusp generate
fi

echo
echo "reseed complete — workspace $WORKSPACE is live on 127.0.0.1:$PORT"
echo "  Dolt Workbench / DSN:  root@tcp(127.0.0.1:$PORT)/cusp"
