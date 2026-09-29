---
title: "Observability"
sidebar_position: 7
---

# Observability

How to see what 012 spends its time on, in a session or across stress
runs. There are two paths that share one set of numbers:

- **Zero setup**: 012 writes JSON event logs, `make stress` writes JSONL
  run records, and DuckDB queries both files directly.
- **Stack**: `make obs-up` starts an OpenTelemetry Collector, ClickHouse
  and Grafana in Docker; 012 sends its logs, traces and metrics over
  OTLP (or the collector tails its log file), the stress runs load into
  ClickHouse, and dashboards sit on top.

The measured bounds these tools track are in [Bounds of support](limits.md).

## Turning it on

Telemetry is off unless a log file or an OTLP endpoint is given:

```sh
012 --log events.jsonl budget.012     # or --log=events.jsonl
O12_LOG=events.jsonl 012 budget.012   # the same through the environment
O12_LOG_LEVEL=debug                   # also one event per frame

012 --otlp http://localhost:4318 budget.012                     # OTLP/HTTP
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 012 budget.012  # the same
```

Or keep them in the [config file](../reference/config.md) as `log-file`, `log-level`
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
`-tags stress`). With it on, each span costs about 0.66 us written to
the file (`BenchmarkSpanOn`; 0.7 us nested under an open span,
`BenchmarkTraceOn`), and about 0.67 us queued for OTLP
(`BenchmarkSpanOTLP`).

Events carry sizes, counts and durations, never cell contents, file
contents or secrets. The JEV API key is never logged; neither are file
names, OTLP endpoints or OTLP headers.

## What is recorded

Every event has `time`, `level`, `msg` (the event name), `event` (the
same, as an attribute for queries), `service` (`012`) and `dur_ms`.
Spans (every event below but `start`, `otlp`, `frames` and `frame`) also
have `trace_id`, `span_id` and, below a root, `parent_id`, the same ids
OTLP sends (see [Nested spans](#nested-spans)).

| Event | When | Attributes |
|---|---|---|
| `start` | the log opens | `version` (VCS revision), `go`, `os`, `arch`, `cpus`, `pid`, `file` and `otlp` (which outputs are on) |
| `otlp` | on exit, with OTLP on | per signal (`logs`, `traces`, `metrics`): `sent`, `dropped` (queue full), `failed` (request failed) so far |
| `recalc` | every recalculation, including the pivot refreshes it causes (nested in it) | `full` (after loading), `evaluated` (dirty cells recomputed), `cells` (stored), `volatile`, `circular` |
| `frames` | once a second while frames are drawn | `frames`, `render_p50_ms`, `render_p95_ms`, `render_max_ms` (View), `keys`, `key_p50_ms`, `key_p95_ms`, `key_max_ms` (key press to the end of its frame), `heap_bytes`, and gauges: `cells`, `jev_in_flight`, `jev_queued` |
| `frame` | every frame, at debug level | `key_ms` |
| `command` | every registered command | `id`, e.g. `data.sort` |
| `macro` | each macro run, from start to end | `steps` (Starlark steps), `calls` (calls to the spreadsheet), `outcome` (`ok`, `error`, `cancelled` or `limit`) |
| `sort`, `filter`, `find`, `replace`, `fill` | the operation itself | `rows`, `cols`, `keys`; `hidden`; `matches`; `replaced`; `cells` |
| `pivot` | every pivot table recomputation (a change to its data or definition) | `records` (source rows summarized), `groups` (row groups at every depth), `cells` (results), `failed` (shows `#REF!`) |
| `frequency` | making a frequency table | `rows` (data rows counted) |
| `live` | each update of a [linked file](../files/following.md)'s rows applied, including the recalculation it causes (nested in it) | `rows` (rows it brought), `cells` (cells that changed), `reset` (the file read again whole) |
| `import`, `export` | file transfers (`internal/fileio`) | `format`, `bytes`, `rows`, `cells`, `notes` |
| `save`, `open` | the native `.012` file | `cells`, `bytes` (save times serializing, on the UI goroutine) |
| `jev` | each question sent | `kind`, `queued` (waiting when sent), `outcome` (`ok` or `failed`) |
| `chart.image` | each kitty-graphics chart image | `type`, `w`, `h`, `bytes` |
| `serve.session` | each `012 serve` SSH session, from start to end | `term`, `idle` |

Failures are logged at `WARN` with an `error` message.

The engine doesn't log: `sheet.OnBegin` says a recalculation or pivot
refresh starts, and `sheet.OnRecalc` and `sheet.OnPivot` hand its counts
to `cmd/012` as it ends, which logs them. The call sites use
`internal/telemetry` (`Start`/`End` spans, `Event`, `Frame`, `Set`), so
the backends (the JSON file, OTLP) sit behind it without touching them.

## Nested spans

Spans nest, so a trace shows what an action caused:

```
command format.bold
  recalc
    pivot
    recalc          (what reads the pivot's results)
import
  recalc
macro
  command set / recalc ...
serve.session
  command ...
```

A child shares its parent's trace id and names it as its parent span.
No `context.Context` goes through the engine for it; instead
(`internal/telemetry/trace.go`):

- **A `Trace` per owner.** Each UI `Model` keeps a `telemetry.Trace`,
  the stack of spans it has open, used only on its program's goroutine.
  `m.spans.Start("command", ...)` opens a span under the innermost one;
  its `End` closes it (and anything left open inside it). There is no
  goroutine-global current span: `012 serve` runs many programs in one
  process, and background `tea.Cmd`s run on other goroutines.
- **The workbook carries its owner's trace.** `Workbook.SetTrace` hands
  it the trace, opaque to the engine (the UI sets its own; an import's
  builder and `sheet.ReadTraced` set one under the import's or open's
  span). The engine's hooks get it back: `OnBegin` begins a span in it,
  `OnRecalc`/`OnPivot` end it, so recalculations and pivot refreshes
  nest in whatever command, import or macro changed the sheet. A
  workbook with no trace logs them as roots.
- **Explicit `Parent` handles across goroutines.** `m.spans.Parent()`
  (or `span.Parent()`) is a small value any goroutine can start under:
  JEV questions, opening a file, and imports and exports, which take it
  in the `context.Context` they already have (`telemetry.WithParent`,
  `ParentFrom`).
- **Long-lived spans are entered.** A macro run's span starts under the
  command that ran it and is entered (`Trace.Enter`/`Leave`) while the
  UI serves the script's calls, so its commands and recalculations nest
  in it. `012 serve` enters each session's `serve.session` span at the
  bottom of that session's Model's trace for good, so every command of a
  session is in its trace, and sessions never share one.

Off, every one of these calls is still one atomic load and no
allocation (`TestOffAllocatesNothing`, `TestTraceOff`,
`BenchmarkTraceOff` 16 ns).

In ClickHouse, a join on `ParentSpanId` shows the tree:

```sql
SELECT p.SpanName AS parent, if(p.SpanName = 'command', p.SpanAttributes['id'], '') AS id,
       c.SpanName AS child, count() AS n
FROM otel.otel_traces AS c
JOIN otel.otel_traces AS p ON c.ParentSpanId = p.SpanId AND c.TraceId = p.TraceId
WHERE c.ServiceName = '012'
GROUP BY parent, id, child ORDER BY parent, child;
```

From a scripted session (import a CSV, paste a table, make a pivot of
it, clear a source cell, bold it):

```
┌─parent──┬─id──────────┬─child──┬─n─┐
│ command │ data.pivot  │ recalc │ 2 │
│ command │ format.bold │ recalc │ 1 │
│ command │ clear       │ recalc │ 1 │
│ import  │             │ recalc │ 4 │
│ recalc  │             │ pivot  │ 6 │
│ recalc  │             │ recalc │ 3 │
└─────────┴─────────────┴────────┴───┘
```

and one trace, `command clear` > `recalc` > (`pivot`, `recalc`). With
the JSON log, the same tree comes from `span_id` and `parent_id`:

```sql
SELECT p.msg AS parent, p.id, c.msg AS child, count(*) AS n
FROM read_json_auto('events.jsonl') c JOIN read_json_auto('events.jsonl') p
  ON c.parent_id = p.span_id GROUP BY ALL ORDER BY n DESC;
```

## OTLP

012 speaks OTLP/HTTP with the JSON encoding itself, with the Go
standard library only (no OpenTelemetry SDK, no new dependency):

| Signal | What | Shape |
|---|---|---|
| Logs (`/v1/logs`) | every event above | body = event name, severity from the level (INFO 9, WARN 13, DEBUG 5), the event's attributes (`event`, `dur_ms`, counts, `parent_id`) as typed values. The log record of a span carries its `traceId` and `spanId` |
| Traces (`/v1/traces`) | every `Start`/`End` span and `Event` (recalc, import, sort, jev, command, ...) | nested (see [Nested spans](#nested-spans)): a child carries `parentSpanId` and its parent's `traceId`; a root has no `parentSpanId`. Kind INTERNAL; status ERROR with the message on `Fail`. Frames aren't spans |
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
On exit, what is queued is sent within a second at most, and not at all
once a request has failed this session.

012 honors the standard `OTEL_*` variables it inherits, like any
OpenTelemetry program, unless its own config or flags override them. So
if your desktop session sets `OTEL_EXPORTER_OTLP_ENDPOINT` for every app,
012 exports there too; when that collector is gone (a laptop whose tailnet
collector isn't up), sends fail in the background, are counted, and cost
nothing at exit. Set `otlp-endpoint` in the config, or `OTEL_SDK_DISABLED=true`,
to change that for 012 alone.

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
`MB-file` or `us/answer`. `COUNT=n` runs each benchmark n times, one
record per sample. `make stress-report` loads them with
`scripts/stress/load.sql` (a benchmark's time in a run is the median of
its samples) and runs `scripts/stress/report.sql`: the latest run
against the previous one on the same CPU and against a baseline
(`BASELINE=run_id`, default the oldest run that covered most of the
same benchmarks), flagging changes past `THRESHOLD` (0.10) and
`MIN_DELTA_NS` (2000), and the headline benchmarks over the last eight
runs. With the default 500 ms per benchmark, runs on a busy machine (say
with the Docker stack ingesting) differ by 10 to 30% on benchmarks of a
few tens of milliseconds; use `BENCHTIME=2s` on a quiet machine before
trusting a single regression. `FAIL_ON_REGRESSION=1` also makes it exit
1 on a regression against the previous run. The same records load into
ClickHouse (`stress.results`) with the same columns; there, samples of
one benchmark in one run collapse into one row.

### Regressions against the last release

`make stress-report` then compares the latest run with the last
release's (`scripts/stress/release.sql`) and exits 1 when anything
regressed, so a release checklist ([Releasing](releasing.md)) can stop
on it:

- **The release** is the newest `v*` tag reachable from HEAD whose
  commit has a clean recorded run on the latest run's CPU and OS (other
  than the latest run, and not on the latest run's commit). All such
  runs of that commit are pooled. `RELEASE=v1.2.3` picks the tag,
  `RELEASE_RUN=run_id` one run instead. With none recorded, the report
  says so and exits 0: record one with `make stress` on the tagged
  commit.
- **Noise** is each benchmark's spread, `(max - min) / median` of its
  samples, the larger of the release's and the latest run's. It is 0
  for a benchmark with one sample on each side, so `COUNT=3` or more on
  both runs gives the threshold room where a benchmark is noisy.
- **A regression** is a median slower than the release's by more than
  `THRESHOLD` plus the noise (10% plus, by default) and by more than
  `MIN_DELTA_NS`. Faster past the same bar is listed as `faster`.

```sh
COUNT=3 BENCHTIME=2s make stress     # on the tagged commit, then on the change
make stress-report                   # exit 1 on a regression against the tag
THRESHOLD=0.2 RELEASE=v0.2.0 make stress-report
```

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
the metrics tables a summary, heap and cells gauge per second. The
import's trace holds two full recalculations under it (33 ms each of
127 ms): the builder's, and the one renaming the sheet after the file
causes.

The `012 runtime` dashboard shows recalculation time (p50, p95, max),
frame and key-to-frame p95 per second, heap and cells, JEV answer time
and queue, imports and exports with throughput, the slowest operations,
and events per minute (all from the logs, so either way in works), and
from OTLP the key-to-frame latency summary, the slowest spans, the
recent traces with nested spans (root span, command id, span count and
which spans nest in it), and a trace view: the spans of one trace as a
tree on a timeline, in Grafana's trace panel (the ClickHouse plugin's
trace format). A trace id in either table links to it, through the
dashboard's Trace id box; with the box empty the view shows the latest
trace with nested spans. `012 stress runs` shows edit latency per
topology, frame and keystroke times, full recalc and file operations,
memory per cell (log scales where the spread is wide), the latest run
against the previous one, and the runs.

The stack starts from scratch with `make obs-up` after
`docker compose -f deploy/observability/docker-compose.yml down -v`.
`stress.results` is created on first start by
`clickhouse/init.sql`, and again (idempotently) by every
`make stress-load`.
