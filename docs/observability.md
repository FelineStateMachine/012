# Observability

How to see what 012 spends its time on, in a session or across stress
runs. There are two paths that share one set of numbers:

- **Zero setup**: 012 writes JSON event logs, `make stress` writes JSONL
  run records, and DuckDB queries both files directly.
- **Stack**: `make obs-up` starts an OpenTelemetry Collector, ClickHouse
  and Grafana in Docker; the collector tails 012's logs and the stress
  runs load into ClickHouse, with dashboards on top.

The measured bounds these tools track are in [limits.md](limits.md).

## Turning it on

Telemetry is off unless a log file is given:

```sh
012 --log events.jsonl budget.012     # or --log=events.jsonl
O12_LOG=events.jsonl 012 budget.012   # the same through the environment
O12_LOG_LEVEL=debug                   # also one event per frame
```

Stdout belongs to the terminal UI, so events only ever go to the file,
appended one JSON object per line. With no log file nothing is timed:
each instrumented call site costs one atomic load (15 ns, no
allocation; `go test -bench . ./internal/telemetry`), and a keystroke
through to its frame costs the same with telemetry on or off (1.06 ms on
a 200x60 screen of an 8192x26 sheet, `BenchmarkTelemetry` under
`-tags stress`). With it on, each event costs about 0.5 us.

Events carry sizes, counts and durations, never cell contents, file
contents or secrets. The JEV API key is never logged; neither are file
names.

## What is recorded

Every event has `time`, `level`, `msg` (the event name), `event` (the
same, as an attribute for queries), `service` (`012`) and `dur_ms`.

| Event | When | Attributes |
|---|---|---|
| `start` | the log opens | `version` (VCS revision), `go`, `os`, `arch`, `cpus`, `pid` |
| `recalc` | every recalculation | `full` (after loading), `evaluated` (dirty cells recomputed), `cells` (stored), `volatile`, `circular` |
| `frames` | once a second while frames are drawn | `frames`, `render_p50_ms`, `render_p95_ms`, `render_max_ms` (View), `keys`, `key_p50_ms`, `key_p95_ms`, `key_max_ms` (key press to the end of its frame), `heap_bytes`, and gauges: `cells`, `jev_in_flight`, `jev_queued` |
| `frame` | every frame, at debug level | `key_ms` |
| `command` | every registered command | `id`, e.g. `data.sort` |
| `sort`, `filter`, `find`, `replace`, `fill` | the operation itself | `rows`, `cols`, `keys`; `hidden`; `matches`; `replaced`; `cells` |
| `import`, `export` | file transfers (`internal/fileio`) | `format`, `bytes`, `rows`, `cells`, `notes` |
| `save`, `open` | the native `.012` file | `cells`, `bytes` (save times serializing, on the UI goroutine) |
| `jev` | each question sent | `kind`, `queued` (waiting when sent), `outcome` (`ok` or `failed`) |
| `chart.image` | each kitty-graphics chart image | `type`, `w`, `h`, `bytes` |

Failures are logged at `WARN` with an `error` message.

The engine doesn't log: `sheet.OnRecalc` hands each recalculation's
counts to `cmd/012`, which logs them. The call sites use
`internal/telemetry` (`Start`/`End` spans, `Event`, `Frame`, `Set`), so a
different backend can replace the JSON file without touching them.

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
| `otel-collector` | `otel/opentelemetry-collector-contrib:0.161.0` | OTLP gRPC (4317) and HTTP (4318) receivers; `file_log` receiver tailing `.deps/obs/logs/*.jsonl` (checkpointed, so restarts don't re-read); `memory_limiter` and `batch`; `clickhouse` exporter for logs, traces and metrics |
| `clickhouse` | `clickhouse/clickhouse-server:26.3.34.136` | OLAP store: `otel.otel_logs` (and the traces and metrics tables the exporter creates), `stress.results`, `stress.latest_vs_previous`. 3 GB container limit, 75% of it for queries. User `o12`, password `o12` |
| `grafana` | `grafana/grafana:13.2.2` with `grafana-clickhouse-datasource` 4.21.3 | http://localhost:3000, anonymous viewing; dashboards `012 runtime` and `012 stress runs` |

To send a session to it, log into the directory the collector tails:

```sh
make obs-up
O12_LOG=$PWD/.deps/obs/logs/012.jsonl ./bin/012 big.csv
make obs-status
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
```

The `012 runtime` dashboard shows recalculation time (p50, p95, max),
frame and key-to-frame p95 per second, heap and cells, JEV answer time
and queue, imports and exports with throughput, the slowest operations,
and events per minute. `012 stress runs` shows edit latency per
topology, frame and keystroke times, full recalc and file operations,
memory per cell (log scales where the spread is wide), the latest run
against the previous one, and the runs.

The stack starts from scratch with `make obs-up` after
`docker compose -f deploy/observability/docker-compose.yml down -v`.
`stress.results` is created on first start by
`clickhouse/init.sql`, and again (idempotently) by every
`make stress-load`.

## Not yet: native OTLP from 012

012 doesn't link the OpenTelemetry Go SDK: its events reach OTLP
through the collector's `file_log` receiver, as logs. Emitting traces
and metrics directly (OTLP/HTTP, configured by the standard
`OTEL_EXPORTER_OTLP_*` variables) would add the SDK as a dependency;
the backend would sit behind `internal/telemetry`, whose spans and
frame summaries map onto OTLP spans and histograms, with no call site
changes. The collector's OTLP receivers are already running for it.
