#!/usr/bin/env bash
# report.sh - compare the latest stress run with the previous one and a
# baseline, using DuckDB over the JSONL run records. No server needed.
#
#   THRESHOLD=0.10     flag changes over 10%
#   MIN_DELTA_NS=2000  ignore changes under 2 us (noise)
#   BASELINE=run_id    compare with this run (default: the oldest run)
#   FAIL_ON_REGRESSION=1  exit 1 when the latest run regressed against the previous
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
results=${RESULTS:-"$root/.deps/stress/results/runs.jsonl"}
if [ ! -s "$results" ]; then
	echo "stress-report: no results yet in $results; run make stress first" >&2
	exit 1
fi
if ! command -v duckdb >/dev/null 2>&1; then
	echo "stress-report: needs the duckdb CLI (brew install duckdb)" >&2
	exit 1
fi

vars="SET VARIABLE results = '$results';
SET VARIABLE threshold = ${THRESHOLD:-0.10};
SET VARIABLE min_delta_ns = ${MIN_DELTA_NS:-2000};
SET VARIABLE baseline = '${BASELINE:-}';"

cat <(echo "$vars") "$root/scripts/stress/load.sql" "$root/scripts/stress/report.sql" | duckdb

if [ "${FAIL_ON_REGRESSION:-}" = 1 ]; then
	n=$(cat <(echo "$vars") "$root/scripts/stress/load.sql" "$root/scripts/stress/report.sql" <(echo "
.mode list
.header off
SELECT count(*) FROM compared WHERE kind = 'previous' AND flag = 'REGRESSION';") | duckdb | tail -1)
	if [ "$n" != 0 ]; then
		echo "stress-report: $n regressions against the previous run" >&2
		exit 1
	fi
fi
