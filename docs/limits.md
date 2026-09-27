# Bounds of support

What 012 handles well, where it degrades, and why, measured. Numbers come
from `make stress` (synthetic worst cases and real datasets, see
[Measuring](#measuring)), `make stress-e2e` (key press to screen through a
real terminal emulator), and the telemetry described in
[observability.md](observability.md), which tracks how they move.

Machine: Apple M5 Pro (18 cores), 48 GB, macOS, Go 1.27.1, arm64. Times
are single-threaded: the engine and the UI run on one goroutine. Last full
run: commit 4dbe37d (after the engine and UI restructure), `BENCHTIME=2s`
on an otherwise idle machine; it matched the numbers below within noise.

The yardstick is a keystroke through to its frame: under 16 ms feels
instant (one frame at 60 Hz), under 100 ms feels responsive, past that
it lags, and past a second it stalls.

## Summary

| Dimension | Comfortable | Degraded | Unsupported | Dominant cost |
|---|---|---|---|---|
| Sheet size | Everything up to the hard limit, 8192 x 256 (2.1 M cells): navigation and drawing don't depend on size | Loading and saving a full sheet: 1 to 2 s; 600 MB of heap | More than 8192 rows or 256 columns: imports keep the first 8192 x 256 and say what they dropped | Heap per cell (about 300 B), JSON file format |
| Incremental recalc | A change that makes formulas read under about 500,000 cells in total (fan-out, chains and volatiles of 8192 cells: 1 to 2 ms) | 1000 SUMs over a full column (1000 x 8192 reads): 170 ms per edit | 8192 running totals (`=SUM($A$1:An)`, 33 M reads): 0.8 s per edit | About 20 ns per cell read: two map lookups |
| Full recalc | Any sheet without heavy range fan-in: under 40 ms for 213 k numbers | 1000 full-column SUMs: 170 ms | 8192 running totals: 0.7 s | Same as above |
| Rendering | Any sheet at up to 200 x 60: 1 ms a frame; 400 x 120: 4.5 ms | 20 charts at 400 x 120: 7 ms | | View building styled strings, then Bubble Tea parsing and diffing them |
| Selection statistics | Any selection, once computed (cached) | Extending a selection over 2 M cells: 30 ms per key | | Summing 2 M map entries per change |
| Imports | CSV, SQLite, Parquet: 1 to 3 M cells/s (a full 2.1 M-cell sheet in about 1 s) | XLSX with formulas: 200 k cells/s | Data past the limits (dropped, with a note) | Building cells one at a time; XLSX formula translation |
| Undo | One step of any size: undo costs what the edit cost | 100 steps that each rewrite a whole column: 217 MB held | | Whole-cell before-images, 264 B per cell per step |
| JEV | Up to about 1000 JEV cells: 0.3 ms of CPU per answer | 4000 JEV cells: 1.3 ms per answer, 5 s of CPU to answer them all | | Every answer recalculates every JEV cell (they're volatile) |
| Formula depth | 10,000 nested parentheses or IFs: under 5 ms | | No explicit limit; recursion grows the stack | Recursive parser and evaluator |
| Find, filter, sort, fill | Filter or sort 8191 rows: 1 to 32 ms; find over 213 k cells: 38 ms; fill 8192 rows: 4 ms | Replace all over 213 k cells: 280 ms | | Per-cell string conversion and regexp |

## Sheet size

Hard limits: `sheet.MaxRows = 8192`, `sheet.MaxCols = 256` (A..IV), as in
Lotus 1-2-3 Release 2.

| Measure | 8192 x 26 (213 k cells) | 8192 x 256 (2.1 M cells) |
|---|---|---|
| Build through `Load`, then full recalc | 90 ms | 1.4 s |
| Live heap | 64 MB | 602 MB (about 300 B per number cell) |
| Save `.012` (JSON, one line per cell) | 74 ms, 4.6 MB | 1.04 s, 47 MB |
| Open `.012` | 134 ms (116 MB allocated) | 2.08 s (1.3 GB allocated) |
| Export CSV / TSV / XLSX / SQLite | 41 / 55 / 87 / 40 ms | 0.71 / 0.66 / 0.91 / 0.72 s |
| Frame at 200 x 60, whole | 1.06 ms | 1.05 ms |

Heap per non-blank cell after loading (`BenchmarkMemory`,
`BenchmarkImport`): numbers 301 B, formulas 707 B (the parsed tree and
reference lists), 200-character text 790 B, imported CSV 275 to 300 B.
A `Cell` struct is 216 B; the rest is the map entry, the input string
and a boxed number for the parsed literal.

## Formula topologies

`BenchmarkEdit` sets one cell and recalculates what depends on it;
`BenchmarkRecalcAll` recomputes everything. 8192 is a full column.

| Topology | Edit | Full recalc | Undo + redo |
|---|---|---|---|
| Independent cell in 8192 x 26 numbers | 0.5 us | 38 ms | 0.8 us |
| Chain: A(n) = A(n-1)+1 down 8192 rows | 1.7 ms | 1.1 ms | 3.4 ms |
| Fan-out: one cell read by 8192 formulas | 1.3 ms | 0.9 ms | 2.7 ms |
| Volatile: 8192 TODAY() and RAND() | 1.25 ms | 0.8 ms | 2.5 ms |
| 1000 named ranges, one formula each | 48 us | 2.8 ms | 99 us |
| Fan-in: 1000 x SUM(A1:A8192) | 174 ms | 172 ms | 350 ms |
| Running totals: 8192 x SUM($A$1:An) | 0.73 to 0.80 s | 0.75 s | 1.45 s |

Every volatile formula is recomputed on every change, as in Sheets, so
8192 volatile cells add about 1.2 ms to every edit anywhere.

Through a real terminal (`make stress-e2e`: 200 x 60, 8192 x 26 imported,
200 SUMs over column A): arrow keys p50 16.7 ms, p95 17.5 ms; an entry
that recalculates the 200 SUMs p50 57 ms, p95 72 ms. The arrow floor is
Bubble Tea's renderer, which draws at most 60 frames a second, so a key
waits up to 16 ms for the next frame; 012's own share of an arrow key is
0.2 to 1 ms.

## Rendering

Only visible rows and columns are drawn, so frame time depends on the
terminal size, not the sheet. A frame is `View` plus what Bubble Tea
does with its string: parse it into a cell buffer and diff it onto the
terminal (`BenchmarkFrame`).

| Sheet | 80 x 24 | 200 x 60 | 400 x 120 |
|---|---|---|---|
| Empty | 0.19 ms | 0.89 ms | 3.2 ms |
| 8192 x 256 numbers | 0.20 ms | 1.05 ms | 4.1 ms |
| 8192 rows of 500-character unicode text | 0.34 ms | 1.37 ms | 4.4 ms |
| 1000 named ranges | 0.30 ms | 1.06 ms | 3.5 ms |
| 20 charts (text) | 0.82 ms | 2.9 ms | 7.3 ms |

About half of a frame at 400 x 120 is Bubble Tea's parse and diff of the
view string.

A chart drawn as text costs 4 to 50 us at 24 x 10 to 120 x 40 cells,
whatever its data: only the categories that fit are drawn (a pie of 8192
slices, 0.44 ms, is the worst). An image for kitty graphics is redrawn
when its data, size or theme changes: 25 to 260 us at 24 x 10 cells and
1 to 1.5 ms at 120 x 40, a pie 8.6 ms (`internal/chart`,
`BenchmarkDraw`, `BenchmarkImage`). Key presses add little: an arrow key through to its frame
is 0.21 ms at 80 x 24 and 1.06 ms at 200 x 60; Page Down 1.26 ms.

With the whole of an 8192 x 256 sheet selected, the status line's Sum,
Avg and Count cover 2.1 M cells: 29 ms to compute, then cached until a
cell changes, so frames with a standing selection cost 0.3 ms; each key
that changes the selection pays the 29 ms again.

## Imports

Real, openly licensed datasets fetched by `scripts/stress-data.sh`
(sources, licenses and checksums in `scripts/stress-data.tsv`):

| File | Shape | Import | Throughput | Heap per cell |
|---|---|---|---|---|
| OWID energy (CSV) | 23,377 x 130, 345 k cells kept | 195 ms | 1.8 M cells/s, 47 MB/s | 277 B |
| Airport codes (CSV) | 86,134 x 13, first 8192 rows kept | 83 ms | 107 MB/s | 274 B |
| NOAA daily CO2 (CSV) | 18,304 x 2 | 6 ms | 2.7 M cells/s | 294 B |
| Country codes (CSV, Arabic, CJK) | 249 x 56 | 5.6 ms | 2.2 M cells/s | 272 B |
| Apache POI formula tests (XLSX) | 788 rows, 1189 formulas | 19 ms | 205 k cells/s | 358 B |
| Chinook PlaylistTrack (SQLite) | 8715 x 2 | 5.5 ms | 3.0 M cells/s | 293 B |
| Parquet alltypes_tiny_pages | 7300 x 13 | 40 ms | 2.4 M cells/s | 276 B |

Past the limits, imports keep the first 8192 rows and 256 columns and
say how much they left out. Parquet files and SQLite tables stop reading
at the last row and take the count of the rest from the file; CSV, TSV
and SQLite queries are read to their end to count it, without keeping
it (the airport codes file is read to its end). Every importer but XLSX
streams, so memory follows the sheet, not the file; excelize holds an
XLSX worksheet in memory while its rows are read.

## Undo

Each step keeps a copy of every cell it changed (`MaxUndo = 100`
steps). Undoing costs about what the change cost, since it recalculates
the same cells. Clearing 213 k cells and undoing it takes 158 ms. The
history is bounded by steps, not bytes: 100 steps that each rewrite a
whole column of 8192 cells hold 217 MB.

## JEV

Each JEV formula is volatile: every recalculation looks its question up
in the cache, and every answer that arrives recalculates all JEV
cells. With a fake client answering instantly (`BenchmarkJEV`), the CPU
per answer grows with the number of JEV cells:

| JEV cells | Per answer | All answers |
|---|---|---|
| 100 | 26 us | 2.7 ms |
| 1000 | 261 us | 0.26 s |
| 4000 | 1.3 ms | 5.1 s |

Against the real service the network dominates: at most 8 questions are
in flight, so 4000 questions at 300 ms each take 2.5 minutes, of which
5 s is recalculation spread over that time. Each answer blocks the UI
for its recalculation (1.3 ms at 4000 cells).

## Several sheets

A workbook recalculates all its sheets together. Formulas that name a
sheet (`Data!A1`) are indexed by the sheet names they use: a changed cell
on a sheet some formula names scans those cross-sheet formulas, as range
users were scanned before they were indexed by column, so a change there
costs O(changed cells x cross-sheet formulas); cells on sheets no formula
names skip the scan. Each sheet's lookup, which resolves sheet names, is
made once per recalculation, and the recalculation state stays on each
sheet keyed by address, so a single-sheet workbook computes as before.

| Benchmark (`internal/sheet`, `go test -bench SheetEdit`) | Edit |
|---|---|
| 5000 formulas on one sheet reading `A(n)` and `SUM(A1:A10)` | 2.7 ms |
| The same 5000 formulas reading `Data!A(n)` and `SUM(Data!A1:A10)` | 3.5 ms |

The stress shapes before and after workbooks (5 runs each, same machine;
differences under about 5% are noise):

| Benchmark | Before | After |
|---|---|---|
| RecalcAll dense 8192 x 26 | 36.1 ms | 36.7 ms |
| RecalcAll fan-in / running | 170 / 706 ms | 177 / 742 ms |
| Edit chain / fan-in / running | 1.64 / 168 / 694 ms | 1.92 / 173 / 726 ms |
| Undo chain | 3.4 ms | 4.0 ms |
| BigUndo | 140 ms, 431 k allocs | 158 ms, 856 k allocs |
| Open 8192 x 26 / 8192 x 256 | 120 ms / 1.85 s | 120 ms / 1.91 s |

Cell reads now go through one more function call to resolve the sheet,
which is most of the 3 to 5% on read-heavy shapes; undo steps key their
before-images by sheet and address, which is BigUndo's extra allocation.

## Hotspots found and fixed

Each with its benchmark before and after, in the commit that fixed it.

| Hotspot | Evidence | Before | After |
|---|---|---|---|
| SUM, AVERAGE, COUNT allocated twice per cell read: `return &v` of the `Value` parameter moved it to the heap on every call | CPU profile of a fan-in edit: `toNum` and `aggregate.func1.1` allocating 3 GB | Edit/fanin 320 ms, 16.4 M allocs | 203 ms, 2 k allocs |
| Every changed cell scanned every formula with a range reference | Running totals edit: 67 M `Rect.Contains` calls | Edit/running 1.83 s | 0.70 s |
| `RecalcAll` traced dependents from every cell (it found nothing new) | RecalcAll/names spent its time in the name scan | RecalcAll/names 157 ms, fanin 395 ms | 2.8 ms, 172 ms |
| Overflowing text cut with `ansi.Cut` per column, rescanning the text each time | CPU profile of View on long text: 90% in `ansi.Cut` | View/longtext 400 x 120: 40.5 ms | 2.1 ms |
| `Names()` upper-cased both names in every sort comparison, every frame | 21 k allocations per View with 1000 names | View/names 80 x 24: 662 us | 197 us |
| Status line summed a whole-sheet selection with 2 M map lookups, every frame | CPU profile: `RangeStats` 87% of a frame | 91 ms per frame | 0.3 ms (cached), 29 ms when the selection changes |
| JEV cache keys encoded with `encoding/json` | CPU profile of answers: JSON 60% of recalc | JEV/1000 575 ms, JEV/4000 9.3 s | 263 ms, 5.1 s |

## What would raise the bounds

In order of value for effort. The first three are what raising the
limits toward Google Sheets' 10 M cells (or Excel's 1,048,576 x 16,384)
would take; without them a 10 M-cell sheet would need about 3 GB of
heap, take 10 s to open and 140 ms per key to extend a selection over
it.

1. **Compact cell storage** (L, 2 to 3 weeks). Cells live in a
   `map[Addr]*Cell` behind `cellStore` (`internal/sheet/store.go`), 300 B
   each: a 216 B struct carrying formula-only
   fields (expression, reference lists, inferred format) on every cell,
   plus the map entry, the input string and a boxed literal. Storing
   columns in row blocks (say 1024 rows) of compact values, with
   formulas, formats and styles in side tables keyed by address, and
   the input string kept only when it differs from the canonical text
   of the value, would bring numbers to about 20 to 40 B per cell and
   turn every cell read in recalc from two hash lookups (20 ns) into an
   index (2 to 3 ns). Cells are already per sheet in a workbook, so the
   storage can change sheet by sheet, and nothing outside `cellStore`
   touches the map, so the change stays inside it.
2. **Range dependency index and shared range results** (M, 1 week).
   Range users are now indexed by column; an interval index per column
   (or per row block) would make finding them independent of how many
   share a column. Memoizing aggregate results per range within one
   recalc would make 1000 identical `SUM(A1:A8192)` cost one, and
   running totals could be computed from a prefix sum per column block:
   the fan-in and running-total rows of the table above would drop to
   milliseconds. Aggregates already read ranges through `lookup.cells`
   (`internal/sheet/recalc.go`), where such results would be served.
3. **Streaming, compact file format** (M, 1 week). The `.012` file is
   JSON decoded whole (1.3 GB allocated to open 47 MB). A streaming
   decoder over the same format would roughly halve open time and cut
   allocation tenfold; a columnar or gzip-compressed variant would cut
   the size about fourfold. The format is `internal/sheet/file.go`, apart
   from the cell store.
4. **Incremental selection statistics** (S, 2 days). Keep per-column
   sums and counts, updated in `place` and after recalc, and compute a
   selection's Sum and Count from column totals minus the rows outside
   it; extending a whole-sheet selection would drop from 29 ms to well
   under 1 ms.
5. **JEV recalc by question** (S to M, 3 days). Index JEV cells by
   question key and recalculate only the cells whose question was
   answered (and their dependents), and coalesce answers that arrive in
   the same frame: answering 4000 questions would cost 4000 small
   recalcs instead of 4000 full ones.
6. **Undo bounded by bytes** (S, 1 day). Cap the history by the memory
   its before-images hold, and store formatting-only changes as diffs.
7. **Frame rate** (S, hours). Bubble Tea draws at most 60 frames a
   second; asking for 120 would halve the key-to-screen floor from about
   16 ms to 8 ms at the cost of more redraws.
8. **Recursion limits** (S, hours). The parser (`internal/formula`) and
   the evaluator recurse without a limit; a depth cap (Excel allows 64
   nested functions) would turn a pathological file into an error rather
   than a deep stack.

## Measuring

```sh
make stress           # fetch datasets, run everything, record the run (5 minutes)
BENCH='Edit|Frame' make stress   # a subset; BENCHTIME=2s for steadier numbers
make stress-report    # latest against previous and baseline, with trends
make stress-e2e       # key press to screen through libghostty
```

Benchmarks are built only with `-tags stress`, so `go test ./...` stays
fast. They live in `internal/sheet/stress_test.go` (engine),
`internal/sheet/hotpath_stress_test.go` (parsing and number formats),
`internal/ui/stress_test.go` (View, frames, keystrokes, JEV, telemetry
overhead), `internal/fileio/stress_test.go` (imports and exports),
`internal/chart/stress_test.go` (charts as text and images) and
`e2e/stress_test.go`; the synthetic sheets are built by
`internal/stress`. Results accumulate in `.deps/stress/results` and, with
the observability stack up, in ClickHouse.
