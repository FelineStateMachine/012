# Observability

How to see what 012 spends its time on, in a session or across stress
runs. There are two paths that share one set of numbers:

- **Zero setup**: 012 writes JSON event logs, `make stress` writes JSONL
  run records, and DuckDB queries both files directly.
- **Stack**: `make obs-up` starts an OpenTelemetry Collector, ClickHouse
  and Grafana in Docker; 012 sends its logs, traces and metrics over
  OTLP (or the collector tails its log file), the stress runs load into
  ClickHouse, and dashboards sit on top.

The measured bounds these tools track are in [limits.md](limits.md).

## Turning it on

Telemetry is off unless a log file or an OTLP endpoint is given:

```sh
012 --log events.jsonl budget.012     # or --log=events.jsonl
O12_LOG=events.jsonl 012 budget.012   # the same through the environment
O12_LOG_LEVEL=debug                   # also one event per frame

012 --otlp http://localhost:4318 budget.012                     # OTLP/HTTP
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 012 budget.012  # the same
```

Or keep them in the [config file](config.md) as `log-file`, `log-level`
and `otlp-endpoint`; flags win over the environment, which wins over the
file.

The two can be on together or separately. Stdout belongs to the
terminal UI, so events go to the file, appended one JSON object per
line, and/or to the OTLP endpoint (see [OTLP](#otlp) below). With
neither, nothing is timed: each instrumented call site costs one atomic
load (15 ns, no allocation; `go test -bench . ./internal/telemetry`,
and `TestOffAllocatesNothing` guards the zero), and a keystroke
through to its frame costs the same with telemetry on or off (1.06 ms on
a 200x60 screen of an 8192x26 sheet, `BenchmarkTelemetry` under
`-tags stress`). With it on, each event costs about 0.5 us written to
the file, and about 0.6 us queued for OTLP (`BenchmarkSpanOTLP`).

Events carry sizes, counts and durations, never cell contents, file
contents or secrets. The JEV API key is never logged; neither are file
names, OTLP endpoints or OTLP headers.

## What is recorded

Every event has `time`, `level`, `msg` (the event name), `event` (the
same, as an attribute for queries), `service` (`012`) and `dur_ms`.

| Event | When | Attributes |
|---|---|---|
| `start` | the log opens | `version` (VCS revision), `go`, `os`, `arch`, `cpus`, `pid`, `file` and `otlp` (which outputs are on) |
| `otlp` | on exit, with OTLP on | per signal (`logs`, `traces`, `metrics`): `sent`, `dropped` (queue full), `failed` (request failed) so far |
| `recalc` | every recalculation | `full` (after loading), `evaluated` (dirty cells recomputed), `cells` (stored), `volatile`, `circular` |
| `frames` | once a second while frames are drawn | `frames`, `render_p50_ms`, `render_p95_ms`, `render_max_ms` (View), `keys`, `key_p50_ms`, `key_p95_ms`, `key_max_ms` (key press to the end of its frame), `heap_bytes`, and gauges: `cells`, `jev_in_flight`, `jev_queued` |
| `frame` | every frame, at debug level | `key_ms` |
| `command` | every registered command | `id`, e.g. `data.sort` |
| `sort`, `filter`, `find`, `replace`, `fill` | the operation itself | `rows`, `cols`, `keys`; `hidden`; `matches`; `replaced`; `cells` |
| `pivot` | every pivot table recomputation (a change to its data or definition) | `records` (source rows summarized), `groups` (row groups at every depth), `cells` (results), `failed` (shows `#REF!`) |
| `frequency` | making a frequency table | `rows` (data rows counted) |
| `import`, `export` | file transfers (`internal/fileio`) | `format`, `bytes`, `rows`, `cells`, `notes` |
| `save`, `open` | the native `.012` file | `cells`, `bytes` (save times serializing, on the UI goroutine) |
| `jev` | each question sent | `kind`, `queued` (waiting when sent), `outcome` (`ok` or `failed`) |
| `chart.image` | each kitty-graphics chart image | `type`, `w`, `h`, `bytes` |

Failures are logged at `WARN` with an `error` message.

The engine doesn't log: `sheet.OnRecalc` hands each recalculation's
counts to `cmd/012`, which logs them, and `sheet.OnPivot` each pivot
table's. The call sites use
`internal/telemetry` (`Start`/`End` spans, `Event`, `Frame`, `Set`), so
the backends (the JSON file, OTLP) sit behind it without touching them.

## OTLP

012 speaks OTLP/HTTP with the JSON encoding itself, with the Go
standard library only (no OpenTelemetry SDK, no new dependency):

| Signal | What | Shape |
|---|---|---|
| Logs (`/v1/logs`) | every event above | body = event name, severity from the level (INFO 9, WARN 13, DEBUG 5), the event's attributes (`event`, `dur_ms`, counts) as typed values. The log record of a span carries its `traceId` and `spanId` |
| Traces (`/v1/traces`) | every `Start`/`End` span and `Event` (recalc, import, sort, jev, command, ...) | one root span per operation, in a trace of its own: call sites pass no context, so spans don't nest. Kind INTERNAL; status ERROR with the message on `Fail`. Frames aren't spans |
| Metrics (`/v1/metrics`) | the frame summary and running totals | `o12.frame.duration` and `o12.key.latency` (ms): summaries per second with count, sum and the 0.5, 0.95 and 1 quantiles. `o12.heap` (bytes) and the gauges `o12.cells`, `o12.jev_in_flight`, `o12.jev_queued`. Cumulative: `o12.frames` (sum), `o12.operation.duration` (ms histogram per `event`, buckets 0.5 ms to 30 s), `o12.telemetry.dropped` (per `signal`) |

Every request carries the resource `service.name=012`,
`service.version` (VCS revision), `service.instance.id` (random per
run), `os.type`, `host.arch`, `process.runtime.name` and
`process.runtime.version`, plus any from `OTEL_RESOURCE_ATTRIBUTES`.

Configuration is the standard environment; nothing is sent unless an
endpoint is set:

| Variable | Effect |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | base URL; `/v1/logs`, `/v1/traces`, `/v1/metrics` are appended. `012 --otlp URL` sets it too (and wins). A URL without a scheme gets `http://` |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT`, `_TRACES_ENDPOINT`, `_METRICS_ENDPOINT` | full URL for one signal, used as is; a signal with no endpoint (its own or the base) is not sent |
| `OTEL_LOGS_EXPORTER`, `OTEL_TRACES_EXPORTER`, `OTEL_METRICS_EXPORTER` | `none` leaves that signal out |
| `OTEL_EXPORTER_OTLP_HEADERS` | `key=value,...` (values percent-decoded) sent with every request, e.g. `Authorization=Bearer%20...` |
| `OTEL_EXPORTER_OTLP_TIMEOUT` | per request, in milliseconds; 3000 by default (shorter than the spec's 10 s, so quitting never waits long) |
| `OTEL_EXPORTER_OTLP_COMPRESSION` | `none` for plain JSON; gzip otherwise |
| `OTEL_RESOURCE_ATTRIBUTES` | extra resource attributes (`service.name` stays `012`) |
| `OTEL_SDK_DISABLED` | `true` turns OTLP off whatever else is set |

`OTEL_EXPORTER_OTLP_PROTOCOL` is ignored: 012 only sends `http/json`,
which the collector's HTTP receiver (4318) accepts; the gRPC port (4317)
won't work.

The UI never waits on the network. Events go into bounded in-memory
queues (2048 items per signal); a background goroutine sends them every
2 seconds, or sooner once 512 are waiting, gzipped, with no retries. A
full queue drops new items and a failed request drops its batch; both
are counted in `o12.telemetry.dropped` and the `otlp` event on exit.
On exit, what is queued is sent within one request timeout (so at most
about 3 s with a collector that doesn't answer).

`internal/telemetry/otlp_test.go` checks the payloads against the
spec's JSON mapping with a fake collector (hex ids, 64-bit integers as
strings, enums as numbers, resource attributes, gzip), plus the drops
on overflow, the flush on close, the bounded close with a stuck
collector, and that nothing is sent while off.

## Querying logs with DuckDB

Nothing to install beyond the `duckdb` CLI:

```sql
-- Time per event type
SELECT msg AS event, count(*) AS n,
       round(quantile_cont(dur_ms, 0.5), 2) AS p50_ms,
       round(quantile_cont(dur_ms, 0.95), 2) AS p95_ms,
       round(max(dur_ms), 2) AS max_ms
FROM read_json_auto('events.jsonl') WHERE msg <> 'frames'
GROUP BY ALL ORDER BY n DESC;
```

From `make stress-e2e` (an 8192x26 import, 200 SUMs over a column, then
arrows and entries):

```
┌─────────┬───────┬────────┬────────┬────────┐
│  event  │   n   │ p50_ms │ p95_ms │ max_ms │
├─────────┼───────┼────────┼────────┼────────┤
│ recalc  │   442 │    1.0 │  49.77 │  56.94 │
│ command │     6 │    0.0 │    0.0 │    0.0 │
│ import  │     2 │   96.4 │  97.55 │  97.68 │
│ start   │     2 │        │        │        │
└─────────┴───────┴────────┴────────┴────────┘
```

```sql
-- The seconds with the slowest key presses, with memory and size
SELECT time, frames, render_p95_ms, key_p95_ms, key_max_ms,
       round(heap_bytes / 1048576) AS heap_mb, cells
FROM read_json_auto('events.jsonl') WHERE msg = 'frames'
ORDER BY key_max_ms DESC LIMIT 5;

-- Slowest recalculations and how much they touched
SELECT time, dur_ms, evaluated, cells, full
FROM read_json_auto('events.jsonl') WHERE msg = 'recalc'
ORDER BY dur_ms DESC LIMIT 10;

-- JEV latency percentiles and failures
SELECT count(*) AS asked, count(*) FILTER (WHERE outcome = 'failed') AS failed,
       quantile_cont(dur_ms, [0.5, 0.95, 0.99]) AS p50_p95_p99_ms
FROM read_json_auto('events.jsonl') WHERE msg = 'jev';
```

## Stress runs

`make stress` appends one JSON line per benchmark to
`.deps/stress/results/runs.jsonl` (gitignored): `run_id`, `ts`,
`git_sha`, `git_dirty`, `go_version`, `goos`, `goarch`, `cpu`, `host`,
`pkg`, `name`, `procs`, `n`, `ns_op`, `b_op`, `allocs_op`, `mb_s`, and
`metrics`, a map of custom units such as `B/cell`, `cells/s`,
`MB-file` or `us/answer`. `make stress-report` loads them with
`scripts/stress/load.sql` and runs `scripts/stress/report.sql`: the
latest run against the previous one on the same CPU and against a
baseline (`BASELINE=run_id`, default the oldest run that covered most
of the same benchmarks), flagging changes past `THRESHOLD` (0.10) and
`MIN_DELTA_NS` (2000), and the headline benchmarks over the last eight
runs. With the default 500 ms per benchmark, runs on a busy machine (say
with the Docker stack ingesting) differ by 10 to 30% on benchmarks of a
few tens of milliseconds; use `BENCHTIME=2s` on a quiet machine before
trusting a single regression. `FAIL_ON_REGRESSION=1` makes it
exit 1 on a regression, for CI. The same records load into ClickHouse
(`stress.results`) with the same columns.

## The stack

```sh
make obs-up       # start, and print the environment for 012
make obs-status   # containers, ClickHouse row counts, events of the last hour
make stress-load  # load recorded stress runs into ClickHouse
make obs-down     # stop; data stays in the named volumes
```

`deploy/observability/docker-compose.yml`, all ports on 127.0.0.1:

| Service | Image | Role |
|---|---|---|
| `otel-collector` | `otel/opentelemetry-collector-contrib:0.161.0` | OTLP gRPC (4317) and HTTP (4318, what 012 --otlp sends to) receivers; `file_log` receiver tailing `.deps/obs/logs/*.jsonl` (checkpointed, so restarts don't re-read); `memory_limiter` and `batch`; `clickhouse` exporter for logs, traces and metrics |
| `clickhouse` | `clickhouse/clickhouse-server:26.3.34.136` | OLAP store: `otel.otel_logs`, `otel.otel_traces` and `otel.otel_metrics_*` (created by the exporter), `stress.results`, `stress.latest_vs_previous`. 3 GB container limit, 75% of it for queries. User `o12`, password `o12` |
| `grafana` | `grafana/grafana:13.2.2` with `grafana-clickhouse-datasource` 4.21.3 | http://localhost:3000, anonymous viewing; dashboards `012 runtime` and `012 stress runs` |

To send a session to it, point 012 at the collector's OTLP/HTTP
receiver (127.0.0.1:4318), which feeds logs, traces and metrics into
ClickHouse:

```sh
make obs-up
./bin/012 --otlp http://localhost:4318 big.csv
# or, for the e2e stress check:
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 make stress-e2e
make obs-status
```

Logs land in `otel.otel_logs` (`ServiceName` `012`, the event's
attributes as strings in `LogAttributes`, `TraceId`/`SpanId` on span
events), spans in `otel.otel_traces`, and metrics in
`otel.otel_metrics_summary`, `_gauge`, `_sum` and `_histogram`.

The alternative, logs only, is to log into the directory the collector
tails (use one or the other, or the logs arrive twice):

```sh
O12_LOG=$PWD/.deps/obs/logs/012.jsonl ./bin/012 big.csv
```

The collector parses each line: `time` becomes the timestamp, `level`
the severity, `msg` the body, `service` the `service.name` resource
attribute, and everything else a log attribute (as strings in
ClickHouse's `LogAttributes` map). For example:

```sql
-- ClickHouse: recalc and import percentiles for 012
SELECT LogAttributes['event'] AS event, count() AS n,
       round(quantile(0.5)(toFloat64OrZero(LogAttributes['dur_ms'])), 2) AS p50_ms,
       round(quantile(0.95)(toFloat64OrZero(LogAttributes['dur_ms'])), 2) AS p95_ms
FROM otel.otel_logs
WHERE ServiceName = '012' AND event NOT IN ('frames', 'start')
GROUP BY event ORDER BY n DESC;

-- Stress trend of one benchmark
SELECT ts, git_sha, round(ns_op / 1e6, 2) AS ms
FROM stress.results FINAL WHERE name = 'Edit/fanin-1000xSUM8192' ORDER BY ts;

-- What regressed since the previous run
SELECT * FROM stress.latest_vs_previous WHERE change > 0.1 ORDER BY change DESC;

-- OTLP spans: time per operation
SELECT SpanName, count() AS n, round(quantile(0.95)(Duration) / 1e6, 2) AS p95_ms,
       countIf(StatusCode = 'Error') AS failed
FROM otel.otel_traces WHERE ServiceName = '012' GROUP BY SpanName ORDER BY n DESC;

-- OTLP metrics: key-to-frame p95 per second
SELECT TimeUnix, Count AS keys,
       ValueAtQuantiles.Value[indexOf(ValueAtQuantiles.Quantile, 0.95)] AS p95_ms
FROM otel.otel_metrics_summary
WHERE ServiceName = '012' AND MetricName = 'o12.key.latency' ORDER BY TimeUnix;
```

From `make stress-e2e` sent over OTLP, `otel_traces` held 221 `recalc`
spans (p95 51 ms), 1 `import` (94 ms, 8192 rows) and 3 `command`, and
the metrics tables a summary, heap and cells gauge per second.

The `012 runtime` dashboard shows recalculation time (p50, p95, max),
frame and key-to-frame p95 per second, heap and cells, JEV answer time
and queue, imports and exports with throughput, the slowest operations,
and events per minute (all from the logs, so either way in works), and
from OTLP the key-to-frame latency summary and the slowest spans with
their trace ids. `012 stress runs` shows edit latency per
topology, frame and keystroke times, full recalc and file operations,
memory per cell (log scales where the spread is wide), the latest run
against the previous one, and the runs.

The stack starts from scratch with `make obs-up` after
`docker compose -f deploy/observability/docker-compose.yml down -v`.
`stress.results` is created on first start by
`clickhouse/init.sql`, and again (idempotently) by every
`make stress-load`.
