#!/usr/bin/env bash
#
# Test coverage over the packages Cusp *authors* — excluding the salvaged Dolt infra we don't own
# and the test-only harness — measured from the full suite so the integration + CLI tests count
# toward app/store/cmd. The `dolt` binary is required: without it those tests skip and the number
# is meaningless, so we fail loudly rather than report a false low.
#
# Usage:  scripts/coverage.sh [report|check|commands|html]
#   report    (default) run the suite; print the owned-total and a per-package breakdown
#   check     run the suite; fail if the owned-total is below the ratchet floor ($MIN)
#   commands  per-command-file coverage (which cmd/cusp commands have tests)
#   html      open the HTML coverage report
#
# The ratchet: raise $MIN in a commit as coverage grows; never lower it silently.
set -euo pipefail

MIN=85.0

MODE="${1:-report}"
CLI_DIR="$(cd "$(dirname "$0")/../src/cli" && pwd)"
cd "$CLI_DIR"
PROFILE="coverage.out"

# Packages we own = everything minus the salvaged infra, the harness, and the integration package.
owned_packages() {
	go list ./... | grep -vE 'internal/(storage|doltserver|remotecache|doltremote|config|configfile|selfupdate|atomicfile|lockfile|debug|timeparsing|git|testutil|integration)($|/)' | paste -sd, -
}

run_suite() {
	if ! command -v dolt >/dev/null 2>&1; then
		echo "coverage: the 'dolt' binary is not on PATH — the integration and CLI tests would skip," >&2
		echo "          undercounting coverage. Install dolt, then re-run." >&2
		exit 1
	fi
	# Quiet on success; on a test failure, surface the full output (matters in CI).
	local log
	log="$(mktemp)"
	if ! go test -coverpkg="$(owned_packages)" -coverprofile="$PROFILE" ./... >"$log" 2>&1; then
		cat "$log" >&2
		rm -f "$log"
		echo "coverage: test run failed (see output above)." >&2
		exit 1
	fi
	rm -f "$log"
}

owned_total() { go tool cover -func="$PROFILE" | awk '/^total:/ { sub(/%/,"",$3); print $3 }'; }

# The breakdowns below are STATEMENT-weighted per file/package, computed from the profile: for each
# command file, "% of its statements exercised". This is the meaningful signal for "which commands
# have tests" — a per-function average would read ~100% everywhere, because every command file's
# init() (which registers the command) runs at test-binary load and is trivially covered, drowning
# out the RunE logic. (The headline `owned_total` uses `go tool cover -func`, the canonical
# `go test -cover` number, which attributes statements to functions; the two metrics differ slightly.)
#
# With -coverpkg every test binary emits blocks for all owned packages, so each block appears once
# per binary; we merge by block key (covered if ANY copy ran), as `go tool cover` does, then sum
# unique statements.

# Per-package coverage (statements covered / total, by package dir).
per_package() {
	awk 'NR>1 { nstmt[$1]=$2; if ($3>0) hit[$1]=1 }
	END {
		for (k in nstmt) {
			path=k; sub(/:.*/,"",path); pkg=path; sub(/\/[^/]+$/,"",pkg)
			tot[pkg]+=nstmt[k]; if (k in hit) cov[pkg]+=nstmt[k]
		}
		for (p in tot) { short=p; sub(/.*endermalkoc\/cusp\//,"",short)
			printf "  %-34s %5.1f%%  (%d/%d)\n", short, 100*cov[p]/tot[p], cov[p], tot[p] }
	}' "$PROFILE" | sort
}

# Per-command-file coverage — the "which commands have tests" tracker.
per_command() {
	awk 'NR>1 { nstmt[$1]=$2; if ($3>0) hit[$1]=1 }
	END {
		for (k in nstmt) {
			file=k; sub(/:.*/,"",file)
			if (file ~ /cmd\/cusp\//) { tot[file]+=nstmt[k]; if (k in hit) cov[file]+=nstmt[k] }
		}
		for (f in tot) { short=f; sub(/.*cmd\/cusp\//,"",short)
			printf "  %-20s %6.1f%%  (%d/%d)\n", short, 100*cov[f]/tot[f], cov[f], tot[f] }
	}' "$PROFILE" | sort -k2 -rn
}

case "$MODE" in
check)
	run_suite
	total="$(owned_total)"
	printf 'coverage (owned packages): %s%%   ratchet floor: %s%%\n' "$total" "$MIN"
	if awk -v t="$total" -v m="$MIN" 'BEGIN { exit (t+0 < m+0) ? 0 : 1 }'; then
		echo "FAIL: coverage ${total}% is below the floor ${MIN}% — add tests, or lower the floor deliberately in scripts/coverage.sh." >&2
		exit 1
	fi
	echo "OK (>= ${MIN}%)"
	;;
report)
	run_suite
	printf '\ncoverage (owned packages): %s%%   ratchet floor: %s%%\n\n' "$(owned_total)" "$MIN"
	per_package
	;;
commands)
	[ -f "$PROFILE" ] || run_suite
	echo "cmd/cusp — coverage per command file (which commands have tests):"
	per_command
	;;
html)
	[ -f "$PROFILE" ] || run_suite
	go tool cover -html="$PROFILE"
	;;
*)
	echo "usage: scripts/coverage.sh [report|check|commands|html]" >&2
	exit 2
	;;
esac
