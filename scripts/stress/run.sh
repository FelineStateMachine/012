#!/usr/bin/env bash
# run.sh - what `make stress` does: fetch the datasets, run the stress
# benchmarks (built with -tags stress), print a summary table, and append
# the run to .deps/stress/results/runs.jsonl for make stress-report. When
# the observability stack is up (make obs-up), the run also goes to
# ClickHouse.
#
#   BENCH=Edit|Frame    only these benchmarks (a go test -bench pattern)
#   BENCHTIME=500ms     per benchmark (slow ones still run once)
#   PKGS="./internal/sheet"
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$root"
"$root/scripts/stress-data.sh"

out="$root/.deps/stress/results"
mkdir -p "$out"
raw="$out/raw-$(date -u +%Y%m%dT%H%M%SZ).txt"
pkgs=${PKGS:-"./internal/sheet ./internal/ui ./internal/fileio ./internal/chart ./internal/telemetry ./internal/serve"}

# shellcheck disable=SC2086 # pkgs is a list
STRESS_DIR="$root/.deps/stress" go test -tags stress -run '^$' \
	-bench "${BENCH:-.}" -benchmem -benchtime "${BENCHTIME:-500ms}" -timeout 60m $pkgs 2>&1 |
	tee "$raw" | awk '/^Benchmark/ { print "  " $1; fflush() } /^(FAIL|panic|--- FAIL)/ { print; fflush() }'

sha=$(git -C "$root" rev-parse --short HEAD 2>/dev/null || echo unknown)
dirty=0
if [ -n "$(git -C "$root" status --porcelain --untracked-files=no 2>/dev/null)" ]; then
	dirty=1
fi
GIT_SHA=$sha GIT_DIRTY=$dirty go run ./internal/stress/benchrec -out "$out/runs.jsonl" <"$raw"
echo "raw output: $raw"

if curl -fsS -o /dev/null "${CLICKHOUSE_URL:-http://localhost:8123}/ping" 2>/dev/null; then
	"$root/scripts/stress/clickhouse-load.sh"
fi
