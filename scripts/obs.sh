#!/usr/bin/env bash
# obs.sh up|down|status - the local observability stack in
# deploy/observability (OpenTelemetry Collector, ClickHouse, Grafana).
# make obs-up, obs-down and obs-status call this.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
compose=(docker compose -f "$root/deploy/observability/docker-compose.yml")
logs="$root/.deps/obs/logs"
ch=${CLICKHOUSE_URL:-http://localhost:8123}
auth="${CLICKHOUSE_USER:-o12}:${CLICKHOUSE_PASSWORD:-o12}"

query() { curl -fsS -u "$auth" --get --data-urlencode "query=$1" "$ch/" 2>/dev/null; }

case "${1:-}" in
up)
	mkdir -p "$logs"
	"${compose[@]}" up -d --wait
	cat <<EOF

Observability stack is up:
  Grafana     http://localhost:3000   (dashboards: 012 runtime, 012 stress runs)
  ClickHouse  http://localhost:8123   (user o12, password o12; databases otel, stress)
  Collector   OTLP on localhost:4317 (gRPC) and localhost:4318 (HTTP)

Point 012 at it, one way or the other (both would store its logs twice):
  export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318  # OTLP: logs, traces, metrics
  export O12_LOG=$logs/012.jsonl                           # or: a JSON log the collector tails

make stress loads each run into ClickHouse while the stack is up;
make stress-load loads the runs recorded so far.
EOF
	;;
down)
	"${compose[@]}" down
	;;
status)
	"${compose[@]}" ps
	echo
	if ! query "SELECT 1" >/dev/null; then
		echo "ClickHouse: not answering at $ch"
		exit 0
	fi
	echo "ClickHouse row counts:"
	query "SELECT database || '.' || name, total_rows FROM system.tables WHERE database IN ('otel', 'stress') AND engine NOT LIKE '%View%' ORDER BY 1 FORMAT PrettyCompactNoEscapes"
	echo "012 events by type (last hour):"
	query "SELECT LogAttributes['event'] AS event, count() AS n, round(quantile(0.95)(toFloat64OrZero(LogAttributes['dur_ms'])), 2) AS p95_ms FROM otel.otel_logs WHERE ServiceName = '012' AND Timestamp > now() - INTERVAL 1 HOUR GROUP BY event ORDER BY n DESC FORMAT PrettyCompactNoEscapes" || true
	echo "012 spans by name (last hour, OTLP):"
	query "SELECT SpanName AS span, count() AS n, round(quantile(0.95)(Duration) / 1e6, 2) AS p95_ms FROM otel.otel_traces WHERE ServiceName = '012' AND Timestamp > now() - INTERVAL 1 HOUR GROUP BY span ORDER BY n DESC FORMAT PrettyCompactNoEscapes" || true
	;;
*)
	echo "usage: $0 up|down|status" >&2
	exit 2
	;;
esac
