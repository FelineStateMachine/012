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
The grid step (see [Step A: the grid](#step-a-the-grid)) was measured
before and after back to back, `-benchtime 1s`, commits bdc5259 and
bb4ff53.

The yardstick is a keystroke through to its frame: under 16 ms feels
instant (one frame at 60 Hz), under 100 ms feels responsive, past that
it lags, and past a second it stalls.

## Summary

| Dimension | Comfortable | Degraded | Unsupported | Dominant cost |
|---|---|---|---|---|
| Sheet size | A grid of 1,048,576 x 16,384 (A..XFD), with up to `max-cells` cells (two million by default): navigation, drawing and every command cost what the cells cost, not the grid | Loading and saving two million cells: 1 to 2 s; 600 MB of heap | More than `max-cells` cells: imports keep whole rows up to it and say what they dropped; larger pastes and fills are refused | Heap per cell (about 300 B), JSON file format |
| Incremental recalc | A change that makes formulas read under about 500,000 cells in total (fan-out, chains and volatiles of 8192 cells: 1 to 2 ms); 1000 SUMs over a full column: 0.7 ms; 8192 running totals: 2.7 ms | | | About 20 ns per cell read: two map lookups |
| Full recalc | Any sheet: under 40 ms for 213 k numbers; 1000 full-column SUMs 1.1 ms; 8192 running totals 2.6 ms | | | Same as above |
| Rendering | Any sheet at up to 200 x 60: 1 ms a frame; 400 x 120: 4.5 ms | 20 charts at 400 x 120: 7 ms | | View building styled strings, then Bubble Tea parsing and diffing them |
| Selection statistics | Any selection: extending one over all 2 M cells costs 0.4 ms a key | | | Per column with data, 1024 or 64 rows at a time from an index on the blocks of filled cells, plus the rows at the selection's ends |
| Imports | CSV, SQLite, Parquet: 1 to 3 M cells/s (two million cells in about 1 s); XLSX numbers or text: 0.7 to 1.1 M cells/s | XLSX with formulas: 0.4 to 0.6 M cells/s | Data past `max-cells` or the grid (dropped, with a note); XLSX files past the reader's limits (refused) | Building cells one at a time; XML decoding; XLSX formula translation |
| Undo | One step of any size: undo costs what the edit cost | History capped at 100 steps and 256 MB of before-images: 100 whole-column steps hold 250 MB | | Whole-cell before-images, about 300 B per cell per step |
| JEV | 4000 JEV cells: 4 us of CPU per answer, 15 ms to answer them all | | | An answer recalculates the cells that asked it; answers within a frame recalculate together |
| Formula depth | 1000 nested parentheses or IFs: under 0.5 ms; chains of formulas through every cell of a sheet | | More than 1024 levels of nesting in one formula: a parse error | Recursive parser; evaluation puts off cells past 65,536 levels |
| Macros | Replaying 1000 recorded actions: 2.2 ms, one undo step; a script's call to the sheet: about 1.4 us | | Scripts past 10 M Starlark steps: stopped, with the line | One message per call to the sheet, served in batches on the UI goroutine |
| Find, filter, sort, fill | Filter or sort 8191 rows: 1 to 32 ms; find over 213 k cells: 38 ms; fill 8192 rows: 4 ms | Replace all over 213 k cells: 280 ms | | Per-cell string conversion and regexp |
| SSH sessions (`012 serve`) | 50 sessions typing at once: frames within one frame interval (p95 8.7 ms), 1.3 MiB per session plus its sheets (upper bound) | | More than `--max-sessions` (8 by default): turned away | Bubble Tea's 120 fps pacing; per session, the terminal's cell buffers |
| Pivot tables | A pivot over 8191 rows recomputes after an edit to its source in 1 to 4 ms | | Results past the grid (the pivot shows #REF!) | Reading each source cell of its fields: a map lookup each |

## Sheet size

Hard limits: `sheet.MaxRows = 1,048,576`, `sheet.MaxCols = 16,384`
(A..XFD), as in Excel. Memory is bounded by `max-cells` (the config file,
two million cells by default): imports keep whole rows up to it, pastes
and fills write at most that many cells at once. WK1 files keep their own
8192 x 256. Past about two million cells the costs below grow with the
cells, as they always did; only the grid is no longer a cost.

| Measure | 8192 x 26 (213 k cells) | 8192 x 256 (2.1 M cells) |
|---|---|---|
| Build through `Load`, then full recalc | 90 ms | 1.4 s |
| Live heap | 64 MB | 602 MB (about 300 B per number cell) |
| Save `.012` (JSON, one line per cell) | 74 ms, 4.6 MB | 1.04 s, 47 MB |
| Open `.012` | 134 ms (116 MB allocated) | 2.08 s (1.3 GB allocated) |
| Export CSV / TSV / XLSX / SQLite | 41 / 55 / 62 / 40 ms | 0.71 / 0.66 / 0.81 / 0.72 s |
| Frame at 200 x 60, whole | 1.06 ms | 1.05 ms |

XLSX exports are written by 012 with archive/zip since excelize went:
measured back to back with the excelize writer, 8192 x 26 went from 335
to 62 ms and 379 to 1.9 MB allocated, 8192 x 256 from 0.97 to 0.81 s
(mostly compression) and 259 to 17 MB allocated.

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
| Fan-in: 1000 x SUM(A1:A8192), or SUM(A:A) | 0.72 ms | 1.1 ms | 1.4 ms |
| Running totals: 8192 x SUM($A$1:An) | 2.7 ms | 2.6 ms | 5.4 ms |
| Sparse: 10,000 numbers spread down a million rows, 1000 each of SUM(A:A), running totals and VLOOKUP over A:B | 2.6 ms | 3.9 ms | 5.1 ms |

Every volatile formula is recomputed on every change, as in Sheets, so
8192 volatile cells add about 1.2 ms to every edit anywhere.

Through a real terminal (`make stress-e2e`: 200 x 60, 8192 x 26 imported,
200 SUMs over column A): arrow keys p50 8.3 ms, p95 10.2 ms; an entry
that recalculates the 200 SUMs p50 62 to 70 ms, p95 67 to 75 ms. The
arrow floor is Bubble Tea's renderer, which draws at most 120 frames a
second (`ui.FrameRate`, its maximum; the default is 60), so a key waits
up to 8 ms for the next frame; 012's own share of an arrow key is 0.2
to 1 ms. At 60 frames a second the same arrows took p50 16.6 ms, p95
18.4 ms. The renderer writes a frame only when the view changed, so an
idle 012 costs the same at either rate: 0.6 to 2.2 ms of CPU a second
on an empty sheet over 30 s, within the noise of measuring it.

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
Avg and Count cover 2.1 M cells. On a sheet of more than 16,384 cells,
a selection of more than 4096 is summed from an index of each column's
sum, count and count of numbers in blocks of 64 rows (`internal/sheet/stats.go`):
the whole blocks it covers, plus the rows at its ends a cell at a time.
The index is built on first use, in one pass over the cells (the 30 ms
the whole sum used to take every time), and a stored, deleted or
recalculated cell marks its block to be summed again when a selection
next covers it. Sums are recomputed, never adjusted, so they don't
drift. The result is still cached until a cell changes. Each key that
changes a selection over the whole sheet, through to its frame
(`BenchmarkKeystroke`, 3 runs each, same machine):

| Key, 8192 x 256 numbers | 80 x 24 before | after | 200 x 60 before | after |
|---|---|---|---|---|
| Shift+Up and Shift+Down with the data selected (`extend-data`) | 31 to 44 ms | 0.53 ms | 32 to 46 ms | 2.1 ms |
| Ctrl+Shift+Down and Ctrl+Shift+Up over the first row (`extend-edge`) | 16 to 23 ms | 0.43 ms | 18 to 24 ms | 1.7 ms |
| Ctrl+A, the selection standing (`select-data`) | 0.30 ms | 0.30 ms | 1.8 ms | 1.8 ms |

A CPU profile of `extend-data` before put 54% of the time in
`RangeStats` walking the map; the rest is building the benchmark's sheet.
The index costs 0.5 MB per sheet that has one (32,768 blocks of 16 B),
nothing on sheets that never had a large selection, and a nil check per
stored or recalculated cell.

## Imports

Real, openly licensed datasets fetched by `scripts/stress-data.sh`
(sources, licenses and checksums in `scripts/stress-data.tsv`):

| File | Shape | Import | Throughput | Heap per cell |
|---|---|---|---|---|
| OWID energy (CSV) | 23,377 x 130, 345 k cells kept | 195 ms | 1.8 M cells/s, 47 MB/s | 277 B |
| Airport codes (CSV) | 86,134 x 13, first 8192 rows kept | 83 ms | 107 MB/s | 274 B |
| NOAA daily CO2 (CSV) | 18,304 x 2 | 6 ms | 2.7 M cells/s | 294 B |
| Country codes (CSV, Arabic, CJK) | 249 x 56 | 5.6 ms | 2.2 M cells/s | 272 B |
| Apache POI formula tests (XLSX) | 788 rows, 1189 formulas, 3828 cells | 9.7 ms | 395 k cells/s | 425 B |
| Chinook PlaylistTrack (SQLite) | 8715 x 2 | 5.5 ms | 3.0 M cells/s | 293 B |
| Parquet alltypes_tiny_pages | 7300 x 13 | 40 ms | 2.4 M cells/s | 276 B |

Past `max-cells` (or the grid), imports keep whole rows up to it and
say how many rows they left out. Parquet files and SQLite tables stop
reading at the last row and take the count of the rest from the file;
CSV, TSV and SQLite queries are read to their end to count it, without
keeping it. The table above is from before the grid grew, when files
were cut at row 8192; now the airport codes (855 k cells, 0.80 s) and
OWID energy (1.04 M cells, 0.95 s) come in whole, at the same rate of
about a million cells a second. Every importer streams,
so memory follows the sheet, not the file. XLSX used to go through
excelize, which held each worksheet in memory while its rows were read;
012's own reader streams it a token at a time, keeping only the shared
strings (in one buffer, 4 bytes a string besides the text), the cell
formats, and the first cell of each shared formula.

XLSX imports of full sheets written by 012's exporter
(`BenchmarkImportXLSX`) and the POI file, before (excelize) and after
(012's reader), on an Apple M5 Pro. Peak heap is sampled while
importing, per cell kept, and includes garbage not yet collected:

| Workbook | Time | Allocated | Allocations | Heap per cell after | Peak heap per cell |
|---|---|---|---|---|---|
| 8192 x 26 numbers (213 k cells) | 480 -> 199 ms | 491 -> 170 MB | 8.4 -> 3.3 M | 304 -> 290 B | 1365 -> 589 B |
| 8192 x 26 table, text and numbers | 642 -> 296 ms | 644 -> 234 MB | 14.4 -> 7.4 M | 297 -> 297 B | 1432 -> 722 B |
| 8192 formulas (`=A1+1` down a column) | 33 -> 13.9 ms | 38 -> 17 MB | 718 -> 321 k | 752 -> 746 B | 4790 -> 2188 B |
| Apache POI formula tests | 21.1 -> 9.7 ms | 22 -> 9.7 MB | 373 -> 179 k | 429 -> 425 B | 2343 -> 1288 B |

The reader refuses files past `xlsxLimits` (in `internal/fileio/xlsxpkg.go`),
with a message naming the limit: 10,000 files in the zip; 1 GB
uncompressed in one part and 2 GB in all, counted as the bytes come out
whatever the zip's headers say; a part compressed more than 250 to 1
once past 16 MB (a zip bomb); XML nested more than 256 deep or a single
tag or text over 32 MB; 16.7 M shared strings; 65,536 cell formats,
fonts, number formats or names; 4096 sheets; 256 MB of text from
expanding shared formulas; rows past 1,048,576, columns past XFD and
rows of more than 16,384 cells; part names that are absolute, climb with
`..` or hold a backslash. encoding/xml expands no external or declared
entities, so entity bombs fail as unknown entities.

## Undo

Each step keeps a copy of every cell it changed. Undoing costs about
what the change cost, since it recalculates the same cells. Clearing
213 k cells and undoing it takes 158 ms.

The history is bounded by steps (`MaxUndo = 100`) and by an estimate of
the memory its before-images hold (`MaxUndoBytes = 256 MB`); past either,
the oldest steps are dropped first, and the newest step is kept however
large it is. Each cell is counted as it is recorded (a fixed cost per
cell plus its text, more for a formula, whose parsed tree stays alive),
so the budget costs nothing to check. The estimate follows the measured
heap within about 10% (`BenchmarkHistoryFull`, `BenchmarkHistoryWide`,
3 runs each, same machine):

| Benchmark | Before | After |
|---|---|---|
| 100 edits rewriting a whole column (819 k before-images) | 250 MB held, all 100 kept | 250 MB held (239 MB estimated), all 100 kept |
| 12 edits rewriting all of 8192 x 26 (2.6 M before-images) | 705 MB held, all 12 kept | 230 MB held (248 MB estimated), the last 4 kept |
| BigUndo: clear 8192 x 26, undo | 154 ms, 430 k allocs | 158 ms, 430 k allocs |
| Undo + redo of one cell (dense, names) | 0.98 us, 106 us | 1.0 us, 106 us |

Counting cells as they're recorded costs about 20 ns a cell, which is
BigUndo's 3%; everything else is unchanged.

## JEV

Each JEV formula is volatile: every recalculation looks its question up
in the cache. A formula shown Loading… is noted against the question it
waits for (by `RemoteCall.Key`), and an answer recalculates only the
formulas waiting for it and what reads them
(`Workbook.RecalcAnswered`). Answers are stored as they arrive; the
first since the last recalculation schedules one a frame later
(`ui.FrameInterval`), which covers every answer stored meanwhile, so
answers arriving together recalculate once. With a fake client
answering instantly (`BenchmarkJEV`, 3 runs each, same machine; the
benchmark doesn't wait the frame, so answers queued together, up to 8,
recalculate together):

| JEV cells | Per answer before | after | All answers before | after |
|---|---|---|---|---|
| 100 | 29 us | 3.5 us | 2.9 ms | 0.35 ms |
| 1000 | 270 us | 3.6 us | 0.27 s | 3.6 ms |
| 4000 | 1.2 ms | 3.8 us | 4.9 s | 15 ms |

Against the real service the network dominates: at most 8 questions are
in flight, so 4000 questions at 300 ms each take 2.5 minutes, and the
recalculation is now a few microseconds per answer whatever the number
of JEV cells. Other changes still recalculate every JEV cell, as every
volatile formula, looking the answers up in the cache.

## Formula depth

The parser (`internal/formula`) counts nesting as it recurses:
parentheses, function calls and prefix operators each open a level. Past
`formula.MaxDepth`, 1024 levels, a formula fails to parse with "Formula
is nested too deeply (more than 1024 levels)", shown on the context line
with the caret at the level past the cap, as other parse errors are.
Excel allows 64 nested functions; 1024 is far beyond a formula written
by hand, and bounds the recursion of everything that walks a formula
(the printer, reference rewriting, the evaluator within one formula). A
`.012` file holding a deeper formula (none could have been written by
hand) fails to open with that message and the cell; imports keep such a
formula's text or cached value, as for any formula they can't read.

Evaluation recurses once per link of a chain of formulas, which no
parser cap bounds: a chain through all 2.1 M cells of a sheet used to
overflow the goroutine's 1 GB stack and kill the program. Evaluation now
counts its depth in cells and operators; a cell reached past 65,536
levels is put off: the evaluation in progress is abandoned (its cells go
back to dirty), the cell is evaluated from the top of the stack, and the
abandoned evaluation is retried and finds it done. Values are exactly as
before, a cycle longer than the limit is still a cycle, and the stack
stays under 32 MB (`TestFullSheetChain`, run with
`SHEET_FULL_CHAIN=1`, checks it under a 64 MB limit: 4 s for 2.1 M
cells, building included).

Same machine, 8 runs each with before and after interleaved, other
work running (differences under 10% are noise):

| Benchmark | Before | After |
|---|---|---|
| Parse (`BenchmarkParse`) | 2.36 us | 2.41 us |
| 1000 nested parentheses / IFs, parse and evaluate | 195 / 425 us | 203 / 438 us |
| 10,000 nested parentheses / IFs | 1.8 / 4.4 ms, accepted | 1.1 / 2.0 ms to refuse |
| RecalcAll chain of 8192 / dense 8192 x 26 | 1.33 / 50 ms | 1.28 to 1.38 / 41 to 51 ms |
| Edit chain of 8192 / 8192 volatiles | 2.36 / 1.68 ms | 2.34 / 1.72 ms |

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

## Serving over SSH

`BenchmarkSessions` (`internal/serve/stress_test.go`) opens N sessions,
each over its own SSH connection on loopback at 120 x 40, optionally
opening the same 1000 x 26 sheet of numbers in every one, then has every
session press an arrow key at once, 200 times, timing each key from the
client's write to the first bytes of its frame arriving. The clients
run in the same process, so the heap and CPU figures include their side
of the connections and are upper bounds for the server. Medians of 3
runs, `-benchtime 200x`, at 60 frames a second:

| Sessions, sheet | Heap per session | Key to frame p50 / p95 / max | CPU per frame |
|---|---|---|---|
| 10, new sheet | 1.30 MiB | 10.7 / 16.6 / 17.1 ms | 0.95 ms |
| 10, 1000 x 26 numbers | 8.5 MiB | 11.2 / 16.6 / 18.0 ms | 1.15 ms |
| 50, new sheet | 1.30 MiB | 9.2 / 16.3 / 19.2 ms | 0.53 ms |
| 50, 1000 x 26 numbers | 8.5 MiB | 9.4 / 16.7 / 20.5 ms | 0.69 ms |

Sessions now ask for 120 frames a second, as the local app does. One
run each, back to back, before and after:

| Sessions, sheet | p50 / p95 / max at 60 fps | at 120 fps | CPU per frame, 60 / 120 fps |
|---|---|---|---|
| 10, new sheet | 10.8 / 16.6 / 16.8 ms | 5.1 / 8.3 / 9.6 ms | 0.78 / 0.67 ms |
| 10, 1000 x 26 numbers | 9.7 / 16.6 / 20.7 ms | 4.3 / 8.3 / 8.5 ms | 1.41 / 0.71 ms |
| 50, new sheet | 9.3 / 16.3 / 18.5 ms | 5.0 / 8.7 / 17.8 ms | 0.58 / 0.53 ms |
| 50, 1000 x 26 numbers | 9.5 / 16.7 / 19.7 ms | 5.1 / 8.9 / 12.8 ms | 0.71 / 0.66 ms |

Latency doesn't move from 10 to 50 sessions: it's Bubble Tea's frame
pacing (at most one frame every 8.3 ms, so a key waits half a frame on
average), not load. A heap profile with 50 sessions open
(`SERVE_HEAP_PROFILE=file`) puts three quarters of a session's heap in
Bubble Tea's render buffers (about 1 MiB at 120 x 40, growing with the
window), 130 KiB in its input key table and under 50 KiB in both ends
of the SSH connection, so the figure is close to the server's own. On
top come about 300 B per cell of the sheets a session has open;
sessions opening the same file each hold a copy. CPU per frame covers both ends of the connection (the
encryption twice, the client reading the frame); at 0.7 ms, 50 sessions
typing continuously at 60 frames a second would keep about two cores
busy, and at 120 about four: frames are drawn only when a session's
screen changes, so the rate is the most keys can cause, not a cost of
being connected.

## Pivot tables

A pivot table is recomputed whole whenever a cell of its source range is
recalculated: it reads every source row once, groups it, and rewrites
only the result cells that changed, so what reads the results
recalculates only when they move. `BenchmarkPivot` edits one cell of a
full 8191-row table (an id, a category of 8, a number and one of 1000
items, twice) under a pivot and times the edit, the recalculation and
the pivot together:

| Pivot | Result rows | Edit | Allocations |
|---|---|---|---|
| Rows: 8 categories; SUM and COUNTA | 10 | 0.95 ms | 196 |
| Rows: 8 x 8 categories with subtotals; columns: 8; SUM and AVERAGE | 75 | 2.7 to 2.9 ms | 2.5 k |
| Rows: 1000 items sorted by their sum; SUM and COUNTUNIQUE | 1001 | 3.5 to 4.2 ms | 14 k |
| Frequency table of the 1000 items | 1001 | 2.5 to 2.8 ms | 24 k |

Groups are found by a comparable key struct rather than a string built
per row: that took the 8-category pivot from 8.4 k to 196 allocations
per edit, the others from 32 to 39 k to the figures above.

## Macros

A macro's script runs on its own goroutine; every call it makes to the
sheet (an entry, a move, a command) is served on the UI goroutine as a
message, in batches of up to 12 ms per update, so replaying doesn't wait
a frame per call and the screen stays live for Esc. The run is one undo
step whatever its size (`BenchmarkMacroReplay`, `BenchmarkMacroScript`,
`internal/ui`, `-tags stress`, `BENCHTIME=2s`):

| Benchmark | Time | Per action or call | Allocations |
|---|---|---|---|
| Replay 1000 recorded actions, absolute references (entries, selects, extends, a command) | 2.16 ms | 2.2 us | 30 k, 1.6 MB |
| The same with relative references (moves, formulas moved with the active cell) | 2.19 ms | 2.2 us | 31 k, 1.7 MB |
| A script's loop: 5000 `set` and 5000 `get` calls | 13.7 ms | 1.4 us per call | 220 k, 7.8 MB |

Reading a range walks its cells; a range larger than 4096 cells (whole
columns) is first trimmed to its last row and column with contents,
found through the sheet's index. Every run stops after 10
million Starlark steps (`macro.DefaultMaxSteps`), about a second of pure
computation.

## Step A: the grid

The grid grew from 8192 x 256 to 1,048,576 x 16,384 once nothing cost
what the grid costs. Beside the cell map, occupancy indexes (bitmaps of
1024 rows per column) say which cells are stored and which have
contents; ranges are read through them, so a whole column of ten cells is
ten visits. Formats of whole columns, rows and the sheet live on the
lines. `TestCommandsCostTheDataNotTheGrid` (`internal/ui`) runs every
command with the whole sheet, a whole column and a whole row selected on
the full grid and fails past 250 ms or 8 MB; each takes 0.1 to 4 ms.
Pasting whole columns or rows sets line formats; a pasted block whose
source or destination has line formats gives its blank cells formatting
of their own up to 65,536 cells, as formatting a block does, and past
that only the cells stored.

Code that cost the grid, and what it does now:

| Where | Was | Now |
|---|---|---|
| Range reads for aggregates (then `reader.cells`; now `Book.Fold` and `Book.Scan`) | every address of the range | the stored cells, in row order |
| SUMIF(S), COUNTIF(S), AVERAGEIF(S), SUMPRODUCT, COUNTBLANK | a mask over every cell | the cells any range holds; the blanks between counted at once |
| MATCH, VLOOKUP, HLOOKUP, XLOOKUP | every entry of the column searched | the stored entries, the blanks between as one; ROWS and INDEX keep the full size |
| Range users (`rangeIndex`) | a fixed array of 256 column buckets, scanned per change | interval trees per used column, whole rows in one |
| Formatting whole columns or rows | up to 65,536 cells made, then only existing ones | column, row and sheet formats |
| Used range, Ctrl+arrow, text running into view | every cell, or every row or column between | the index of filled cells |
| Filter over whole columns | every row tested and hidden one by one | rows with data, one test for the blank rest, stepped over at once |
| Sort | every row of the range | the rows with cells |
| Paste, fill of blanks | every address of the destination | the clip's cells per tile; what blanks land on is cleared |
| Copy to the system clipboard | every blank row between values | refused past `max-cells` cells |
| Pivot sources, JEV value ranges, chart detection | every row of the range | the rows with data; JEV ranges past 4096 cells trimmed to their data |
| Exports of a selection, script reads | every row of whole columns | trimmed to the data |
| Row numbers, name box | 5 digits, IV | 7 digits (the header widens past 9999), XFD |
| Column width reset, set | an undo step per column of the selection | one step; a reset touches only columns with a width |
| XLSX column widths and styles | 256 columns | the data's width (at least 256), a style on A and XFD as the sheet's |

Before and after, back to back (`-benchtime 1s`, medians of one run):

| Benchmark | Before | After |
|---|---|---|
| Edit fan-in, 1000 x SUM(A1:A8192) | 176 ms | 0.72 ms |
| Edit running totals, 8192 x SUM($A$1:An) | 0.73 s | 2.7 ms |
| RecalcAll fan-in / running totals | 178 / 739 ms | 1.1 / 2.6 ms |
| Undo + redo fan-in / running totals | 352 ms / 1.47 s | 1.4 / 5.4 ms |
| Typing an entry under 1000 SUMs, through to its frame (200 x 60) | 59 ms | 0.91 ms |
| Edit sparse-1M (10,000 numbers over a million rows, 3000 whole-column formulas) | | 2.6 ms |
| Frame at 200 x 60 on sparse-1M; arrow keys at XFD1048576 | | 0.68 ms; 0.65 ms |
| Edit a cell of 8192 x 26 numbers | 619 ns | 662 ns |
| Open 8192 x 26 / 8192 x 256 | 127 ms / 1.97 s | 130 ms / 1.99 s |
| Build 8192 x 26 through `Load` | 88 ms | 90 ms |
| BigUndo: clear 8192 x 26, undo | 159 ms | 164 ms |
| Sort 8191 rows | 31 ms | 32 ms |
| Pivot over 8191 rows, 8 categories | 0.89 ms | 0.77 ms |
| Extend a selection of 8192 x 256 by a row | 31 ms | 35 ms (measured alone; 0.41 ms once merged with main's selection statistics, below) |

The index costs a few percent on edits and loads (keeping two bitmaps
current per cell), and running aggregates cost memory for their
checkpoints (a 1000-SUM fan-in allocates 2 MB per recalculation instead
of 0.2 MB). A range is shared only from its second read in a
recalculation, so ranges read once cost nothing extra.

### Merged with main

Step A was merged with main at 7431646 (selection statistics by block,
JEV recalc by question, 120 fps, depth limits, the streaming XLSX
reader). Main's dense statistics index, one entry per 64 rows of every
column, would have been 268 M entries at the new grid; it now lives on
the blocks of the index of filled cells, an entry per 64 rows and a
total per 1024, made where there is data. Main and the merge back to
back (`-benchtime 1s`):

| Benchmark | Main | Merged |
|---|---|---|
| Extend a selection of 8192 x 256 by a row, 80 x 24 | 0.47 ms | 0.41 ms |
| Select all of 8192 x 256, 80 x 24 | 0.28 ms | 0.28 ms |
| JEV, 4000 cells answered | 14.5 ms | 14.3 ms |
| Import XLSX 8192 x 26 numbers / table / 8192 formulas | 187 / 313 / 13.6 ms | 189 / 312 / 13.8 ms |
| Import the POI formula tests (XLSX) | 9.7 ms | 9.5 ms |
| Edit fan-in / running totals | 186 / 777 ms | 0.73 / 2.7 ms |
| Sort 8191 rows | 29.7 ms | 32.3 ms |
| BigUndo | 150 ms | 156 ms |
| Open 8192 x 26 | 117 ms | 121 ms |

CSV imports take longer only because they keep more: the airport codes
(855 k cells, 0.78 s) and OWID energy (1.04 M cells, 0.84 s) now come in
whole. XLSX imports set column widths and styles for the data's columns
(at least 256), without an undo step each: setting 16,384 of them one
step at a time took the POI file from 9.7 to 44 ms before the fix.

## The function library

The function library moved out of the engine into `internal/functions`,
behind a `Book` interface whose methods pass only values (see
[architecture.md](architecture.md#functions)): SUM-like functions have
the engine add a range up (`Fold`), other range walks read chunks into
buffers the `Reader` reuses (`Scan`), and lookups walk positions with a
cursor. The criteria and lookup shapes (`criteria-60xSUMIF8192`: SUMIF,
COUNTIFS and AVERAGEIF over whole columns of 8192 rows;
`lookup-300xVLOOKUP8192`: VLOOKUP, MATCH and XLOOKUP into 8192 keys) were
added for it. Before and after, back to back (`-benchtime 1s`, medians of
three alternating runs; differences under about 3% are noise):

| Benchmark | Before | After |
|---|---|---|
| Edit fan-in, 1000 x SUM(A1:A8192) / SUM(A:A) | 742 / 741 us | 749 / 750 us |
| Edit running totals | 2.79 ms | 2.85 ms |
| Edit sparse-1M | 2.64 ms, 9611 allocs | 2.45 ms, 3610 allocs |
| Edit criteria / lookup | 40.7 / 26.3 ms | 37.1 / 25.3 ms |
| Edit chain / fan-out / volatile / dense | 1.88 / 1.39 / 1.29 ms / 673 ns | 1.85 / 1.40 / 1.29 ms / 662 ns |
| RecalcAll fan-in / running / sparse-1M | 0.97 / 2.50 / 3.82 ms | 0.97 / 2.51 / 3.62 ms |
| RecalcAll criteria / lookup | 66.6 / 27.5 ms | 58.8 / 26.5 ms |
| RecalcAll dense 8192 x 26 | 32.5 ms | 31.8 ms |
| Arithmetic, binary / decimal (1000 formulas and a SUM) | 20.6 / 160 us | 19.8 / 156 us |

Allocations are the same or fewer everywhere: lookups no longer make a
closure per search, and criteria collect positions from the occupancy
index without a call per cell.

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
| Aggregates read every cell of every range, 1000 identical SUMs a thousand times, running totals O(n^2) | Fan-in and running-total edits | Edit/fanin 176 ms, running 0.73 s | 0.72 ms, 2.7 ms (running aggregates shared per recalculation) |
| Range users bucketed by a fixed array of 256 columns, scanned whole per changed cell | 16,384 columns; whole-row ranges in every bucket | | Interval trees per column, wide ranges in one tree |
| A range read visited every address, blank or not | A whole column is a million addresses | | Occupancy index: only stored cells are visited |
| Changing a selection summed every cell in it | CPU profile of Shift+Up over the whole sheet: `RangeStats` 54% | extend-data 80 x 24: 31 to 44 ms | 0.53 ms |
| Every JEV answer recalculated every JEV cell | Per-answer cost grew with the number of JEV cells | JEV/4000 4.9 s | 15 ms |
| Bubble Tea drew at most 60 frames a second | Arrow keys through a terminal waited for the next frame | p50 16.6 ms, p95 18.4 ms | 8.3 ms, 10.2 ms |
| Lazy evaluation recursed once per link of a chain | A chain through all 2 M cells of a sheet overflowed the 1 GB stack and killed the program | crash | 4 s to build and evaluate, under 32 MB of stack |

## What would raise the bounds

In order of value for effort. The grid is Excel's since step A; the
first two are what raising `max-cells` toward Google Sheets' 10 M cells
would take; without them a 10 M-cell sheet would need about 3 GB of
heap and take 10 s to open. (Shared range results, which was the second,
is done: see Step A.)

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
2. **Streaming, compact file format** (M, 1 week). The `.012` file is
   JSON decoded whole (1.3 GB allocated to open 47 MB). A streaming
   decoder over the same format would roughly halve open time and cut
   allocation tenfold; a columnar or gzip-compressed variant would cut
   the size about fourfold. The format is `internal/sheet/file.go`, apart
   from the cell store.
3. **Incremental selection statistics.** Done: per-column block
   aggregates, marked stale as cells change (see [Rendering](#rendering));
   extending a whole-sheet selection went from 31 to 44 ms to 0.53 ms.
4. **JEV recalc by question.** Done: an answer recalculates the cells
   waiting for it, and answers within a frame recalculate together (see
   [JEV](#jev)); answering 4000 questions went from 4.9 s of CPU to
   15 ms.
5. **Smaller undo steps** (S, 1 day). The history is capped by the
   memory its before-images hold; storing formatting-only changes as
   diffs rather than whole cells would let it keep more of them.
6. **Frame rate.** Done: 120 frames a second, Bubble Tea's maximum,
   locally and over SSH; arrow keys through a terminal went from p50
   16.6 ms to 8.3 ms, with no more CPU when idle.
7. **Recursion limits.** Done: see [Formula depth](#formula-depth).

## Measuring

```sh
make stress           # fetch datasets, run everything, record the run (5 minutes)
BENCH='Edit|Frame' make stress   # a subset; BENCHTIME=2s for steadier numbers
make stress-report    # latest against previous and baseline, with trends
make stress-e2e       # key press to screen through libghostty
```

`TestCommandsCostTheDataNotTheGrid` runs with `go test ./internal/ui`
(it skips under `-short`) and guards the grid: a command whose cost grows
with its selection's area fails it.

Benchmarks are built only with `-tags stress`, so `go test ./...` stays
fast. They live in `internal/sheet/stress_test.go` (engine),
`internal/sheet/hotpath_stress_test.go` (parsing and number formats),
`internal/ui/stress_test.go` (View, frames, keystrokes, JEV, telemetry
overhead), `internal/fileio/stress_test.go` (imports and exports),
`internal/chart/stress_test.go` (charts as text and images),
`internal/serve/stress_test.go` (concurrent SSH sessions) and
`e2e/stress_test.go`; the synthetic sheets are built by
`internal/stress`. Results accumulate in `.deps/stress/results` and, with
the observability stack up, in ClickHouse.
