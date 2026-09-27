#!/usr/bin/env bash
# report.sh - compare the latest stress run with the previous one, a
# baseline and the last tagged release, using DuckDB over the JSONL run
# records. No server needed. Exits 1 when the latest run regressed
# against the last release's recorded run.
#
#   THRESHOLD=0.10     flag changes over 10% (plus the benchmark's noise
#                      against the release)
#   MIN_DELTA_NS=2000  ignore changes under 2 us (noise)
#   BASELINE=run_id    compare with this run (default: the oldest run)
#   RELEASE=v1.2.3     compare with this tag's runs (default: the newest
#                      tag reachable from HEAD that has a recorded run)
#   RELEASE_RUN=run_id compare with this run instead of a release's
#   FAIL_ON_REGRESSION=1  also exit 1 when the latest run regressed against the previous
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

# Release tags, newest version first, as 'tag commit' pairs.
if [ -n "${RELEASE:-}" ]; then
	tags=$(git -C "$root" rev-parse -q --verify "refs/tags/$RELEASE^{commit}" | sed "s/^/$RELEASE /") || {
		echo "stress-report: no tag $RELEASE" >&2
		exit 1
	}
else
	tags=$(git -C "$root" tag -l 'v*' --merged HEAD --sort=-v:refname \
		--format='%(refname:short) %(if)%(*objectname)%(then)%(*objectname)%(else)%(objectname)%(end)' 2>/dev/null || true)
fi
list=$(printf '%s\n' "$tags" | awk 'NF == 2 { printf "%s'\''%s'\''", (n++ ? ", " : ""), $0 }')

vars="SET VARIABLE results = '$results';
SET VARIABLE threshold = ${THRESHOLD:-0.10};
SET VARIABLE min_delta_ns = ${MIN_DELTA_NS:-2000};
SET VARIABLE baseline = '${BASELINE:-}';
SET VARIABLE release_run = '${RELEASE_RUN:-}';
SET VARIABLE release_tags = [$list]::VARCHAR[];"

sql() {
	cat <(echo "$vars") "$root/scripts/stress/load.sql" "$@"
}

sql "$root/scripts/stress/report.sql" "$root/scripts/stress/release.sql" | duckdb

count() {
	sql "$root/scripts/stress/report.sql" "$root/scripts/stress/release.sql" <(echo "
.mode list
.header off
$1") | duckdb | tail -1
}

status=0
if [ "$(count 'SELECT count(*) FROM release_pick;')" = 0 ]; then
	echo "stress-report: no recorded run of a release to compare with; record one with make stress on a tagged commit" >&2
else
	n=$(count "SELECT count(*) FROM release_compared WHERE flag = 'REGRESSION';")
	if [ "$n" != 0 ]; then
		echo "stress-report: $n regressions against the last release" >&2
		status=1
	fi
fi
if [ "${FAIL_ON_REGRESSION:-}" = 1 ]; then
	n=$(count "SELECT count(*) FROM compared WHERE kind = 'previous' AND flag = 'REGRESSION';")
	if [ "$n" != 0 ]; then
		echo "stress-report: $n regressions against the previous run" >&2
		status=1
	fi
fi
exit $status
