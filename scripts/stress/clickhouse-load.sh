#!/usr/bin/env bash
# clickhouse-load.sh - load the stress run records (runs.jsonl) into the
# observability stack's ClickHouse, table stress.results, for Grafana.
# Loading the same runs again is harmless: the table deduplicates, and
# queries read it with FINAL. make stress runs this when the stack is up.
set -euo pipefail

root=$(cd "$(dirname "$0")/../.." && pwd)
results=${RESULTS:-"$root/.deps/stress/results/runs.jsonl"}
url=${CLICKHOUSE_URL:-http://localhost:8123}
auth="${CLICKHOUSE_USER:-o12}:${CLICKHOUSE_PASSWORD:-o12}"
compose="$root/deploy/observability/docker-compose.yml"

if [ ! -s "$results" ]; then
	echo "stress-load: no results in $results; run make stress first" >&2
	exit 1
fi
if ! curl -fsS -o /dev/null "$url/ping"; then
	echo "stress-load: ClickHouse isn't answering at $url; run make obs-up" >&2
	exit 1
fi

# The schema is created when the stack first starts; this keeps an older
# volume current.
docker compose -f "$compose" exec -T clickhouse \
	clickhouse-client --user "${auth%%:*}" --password "${auth#*:}" --multiquery \
	<"$root/deploy/observability/clickhouse/init.sql"

query='INSERT INTO stress.results SETTINGS date_time_input_format = '\''best_effort'\'' FORMAT JSONEachRow'
# The body is the query, then the rows.
{ echo "$query"; cat "$results"; } | curl -fsS -u "$auth" --data-binary @- -o /dev/null "$url/"
runs=$(curl -fsS -u "$auth" --get --data-urlencode "query=SELECT uniqExact(run_id), count() FROM stress.results FINAL FORMAT TSV" "$url/")
echo "stress-load: ClickHouse stress.results now holds $(echo "$runs" | cut -f1) runs, $(echo "$runs" | cut -f2) rows"
