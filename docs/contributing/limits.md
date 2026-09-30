---
title: "Bounds of support"
sidebar_position: 6
---

# Bounds of support

What 012 handles well, where it degrades, and why, measured. Numbers come
from `make stress` (synthetic worst cases and real datasets, see
[Measuring](#measuring)), `make stress-e2e` (key press to screen through a
real terminal emulator), and the telemetry described in
[Observability](observability.md), which tracks how they move.

Machine: Apple M5 Pro (18 cores), 48 GB, macOS, Go 1.27.1, arm64. Times
are single-threaded: the engine and the UI run on one goroutine. Figures
are medians of `-benchtime 1s` or `BENCHTIME=2s` runs on an otherwise
idle machine; a range means runs differed by that much, and differences
under about 5% are noise.

The yardstick is a keystroke through to its frame: under 16 ms feels
instant (one frame at 60 Hz), under 100 ms feels responsive, past that
it lags, and past a second it stalls.

## Summary

| Dimension | Comfortable | Degraded | Unsupported | Dominant cost |
|---|---|---|---|---|
| Sheet size | A grid of 1,048,576 x 16,384 (A..XFD), with up to `max-cells` cells (ten million by default): navigation, drawing and every command cost what the cells cost, not the grid; ten million numbers take 205 MB, build in 1.8 s and save in 0.54 s | Opening a `.012` file of ten million cells: 1.2 s, 311 MB of heap at its peak | More than `max-cells` cells: imports keep whole rows up to it and say what they dropped; larger pastes and fills are refused | Parsing each cell's entry from the file's JSON; heap per cell (about 20 B for numbers, 20 to 60 B for real data, 750 B for formulas) |
| Incremental recalc | A change that makes formulas read under about 500,000 cells in total (fan-out, chains and volatiles of 8192 cells: 1.3 to 1.9 ms); 1000 SUMs over a full column: 0.53 ms; 8192 running totals: 2.7 ms | 60 criteria functions (SUMIF, COUNTIFS, AVERAGEIF) over whole columns of 8192 rows: 29 ms an edit | | About 3 ns per cell read: an index into the column's block |
| Full recalc | Any sheet: numbers and text hold their values and cost nothing; 1000 full-column SUMs 0.5 ms; 8192 running totals 1.8 ms | 60 whole-column criteria functions over 8192 rows: 44 ms | | Same as above |
| Rendering | Any sheet at up to 200 x 60: 1.3 ms a frame; 400 x 120: 4.2 ms; a color scale on every cell shown adds 0.4 ms at 200 x 60, borders on every cell and wrapped text 0.4 ms | 20 charts at 400 x 120: 6 ms | | View building styled strings, then Bubble Tea parsing and diffing them |
| Tracing (`BenchmarkTrace`) | An arrow key through to its frame at 200 x 60 with tracing on, on a million cells: a chain of a million formulas 0.66 ms (0.64 off), a million numbers 0.82 ms (0.82 off), a cell read by a million formulas 1.6 ms (0.85 off); off, it costs nothing | | A cell's dependents listed past a thousand: the list says there are more | Finding a cell's links (dependents capped at a thousand), and asking each formula on screen whether it reads the cell |
| Selection statistics | Any selection: extending one over all 2.1 M cells of 8192 x 256 costs 0.3 ms a key | | | Per column with data, 1024 or 64 rows at a time from an index on the blocks of filled cells, plus the rows at the selection's ends |
| Imports | CSV, SQLite, Parquet: 2 to 4 M cells/s (a million cells in 0.25 to 0.4 s); XLSX numbers or text: 0.8 to 1.4 M cells/s | XLSX with formulas: 0.4 to 0.6 M cells/s | Data past `max-cells` or the grid (dropped, with a note); XLSX files past the reader's limits (refused) | Building cells one at a time; XML decoding; XLSX formula translation |
| Undo | One step of any size: undo costs what the edit cost; clearing all ten million cells of a full sheet holds 193 MB | History capped at 100 steps and 256 MB of before-images: 100 whole-column steps hold 15 MB | A step past 1 GB (millions of formulas or long distinct texts at once): asks, and runs without undo if told to | Before-images in the store's form: a slot (20 B) per plain cell, a whole `Cell` per formula |
| JEV | 4000 JEV cells: 4 us of CPU per answer, 15 ms to answer them all | | | An answer recalculates the cells that asked it; answers within a frame recalculate together |
| Formula depth | 1000 nested parentheses or IFs: under 0.5 ms; chains of formulas through every cell of a sheet | | More than 1024 levels of nesting in one formula: a parse error | Recursive parser; evaluation puts off cells past 65,536 levels |
| Macros | Replaying 1000 recorded actions: 2.2 ms, one undo step; a script's call to the sheet: about 1.4 us | | Scripts past 10 M Starlark steps: stopped, with the line | One message per call to the sheet, served in batches on the UI goroutine |
| Find, filter, sort, fill | Filter or sort 8191 rows: 1 to 32 ms; find over 213 k cells: 38 ms; fill 8192 rows: 4 ms | Replace all over 213 k cells: 280 ms | | Per-cell string conversion and regexp |
| SSH sessions (`012 serve`) | 50 sessions typing at once: frames within one frame interval (p95 8.7 ms), 1.3 MiB per session plus its sheets (upper bound) | | More than `--max-sessions` (8 by default): turned away | Bubble Tea's 120 fps pacing; per session, the terminal's cell buffers |
| Pivot tables | A pivot over 8191 rows recomputes after an edit to its source in 0.8 to 2.5 ms | | Results past the grid (the pivot shows #REF!) | Grouping the source rows |
| Linked sources | Ten million rows of Parquet or SQLite: opened in 0.4 ms or 0.65 s, a frame scrolling them 0.9 ms at 200 x 60, a page anywhere 0.2 to 12 ms; SUM of a column 0.42 s (Parquet) or 1.4 s (SQLite), COUNTIFS 1.2 or 2.3 s, a pivot table 2.5 or 3.9 s, each in a few MB | Sorting ten million rows: 3.5 s (Parquet) or 19 s (SQLite), in the background | Functions that hold what they read, over more than `max-cells` cells: `#VALUE!`, saying why | Reading the column from the file; sorting |
| Following files | A log growing by 10,000 rows a second, formulas over it: each update (about 2,500 rows, four times a second) applied in 7 ms keeping every row, 1.7 to 2 ms keeping the last 1000, 14 ms the last 10,000; frames 2.5 to 3.6 ms at 200 x 60 | | Rows past `max-cells`: left out, with a note | Writing the rows and recalculating what reads them; a window rewrites its rows on every update |
| Notebook streams (`TestStreamGridAtFrameSpeed`) | 10,000 rows arriving in batches of 250, the output's grid on screen: 2.3 ms a batch through to its frame at 200 x 60 (3.6 ms at worst) | Past 20,000 rows in one run: the grid is read again from the 10,000 the output keeps, about 60 ms once every 10,000 rows | | Reading the batch's rows into the grid and measuring their text; the output's NUON copied once a batch |
| Arrays and spills | FILTER, SORT or UNIQUE over 8192 rows: about 0.1 ms each per edit; 1000 of them spilling 516 k cells, an edit recomputing 500: 52 ms | Full recalculation of those 1000: 106 ms | An array past `maxArray` values (2,097,152) stored: `#VALUE!`; a spill past the sheet's edge or `max-cells` cells: `#REF!` | Computing each array's values; writing only the spilled cells that changed |

## Sheet size

Hard limits: `sheet.MaxRows = 1,048,576`, `sheet.MaxCols = 16,384`
(A..XFD), as in Excel. Memory is bounded by `max-cells` (the config file,
ten million cells by default): imports keep whole rows up to it, pastes
and fills write at most that many cells at once. WK1 files keep their own
8192 x 256. The costs below grow with the cells; the grid itself costs
nothing (see [The grid](#the-grid)). A `.012` file is read and written
as a stream, so opening costs about the memory the workbook then holds
([The .012 file](#the-012-file)).

| Measure | 8192 x 26 (213 k cells) | 8192 x 256 (2.1 M cells) | 1,000,000 x 10 (10 M cells) |
|---|---|---|---|
| Build through `Load`, then full recalc | 48 ms | 0.47 s | 1.8 s |
| Live heap | 4.4 MB | 43 MB (20.5 B per number cell) | 205 MB |
| Save `.012` (JSON, one line per cell; heap used at the peak) | 12 ms, 4.6 MB (0.3 MB) | 0.14 s, 47 MB (0.1 MB) | 0.54 s, 236 MB (0.7 MB) |
| Open `.012` (heap at the peak, the workbook included) | 24 ms (15 MB) | 0.24 s (60 MB) | 1.2 s (311 MB; 0.9 GB resident, the file's bytes and a sheet built beside it included) |
| Export CSV / TSV / XLSX / SQLite | 33 / 31 / 45 / 38 ms | 0.51 / 0.52 / 0.65 / 0.68 s | |
| Edit a number, undo and redo it | 0.6 us, 1.2 us | | 0.6 us, 1.2 us |
| Frame at 200 x 60, whole | 0.80 ms | 0.80 ms | |

XLSX exports are written by 012 with archive/zip: 1.9 MB allocated for
8192 x 26 and 17 MB for 8192 x 256, whose time is mostly compression.

Heap per non-blank cell after loading (`BenchmarkMemory`,
`BenchmarkImport`): numbers 20.5 B, opened from a file too; formulas 750
B (the parsed tree and reference lists); imported CSV, SQLite and
Parquet 21 to 63 B; text about 60 B plus its length, once per distinct string
(a column of 90 texts repeated takes 27 B a cell). A plain cell (a
number, boolean or text as typed, with a format and style) is a 16-byte
slot in its column's block of 1024 rows, and so is a derived cell, what
a pivot or a spill writes: a pivot's results take 20.6 B a cell and
spilled numbers 20.5 B, plus the 32 B a value of the array the anchor
keeps to compare with the next one (`BenchmarkMemoryDerived`). Formulas
and notes are whole `Cell`s, about 300 B each with their entry and
input (the `Cell` itself is 216 B). How the store lays
them out is in [Architecture](architecture.md#the-engine). Reading a
cell's value is an index into its column's block: 2.8 ns
(`BenchmarkRead`), 8.6 ns a cell read by a SUM in a full recalculation.

### The .012 file

The reader and writer stream (`internal/sheet/fileread.go`,
`filescan.go`): each cell goes into its sheet as its line is read, a
number typed plainly straight into its 16-byte slot, and the other
fields are small and decoded whole. Saving writes the cells in
row-major order as it reads them from the sheet, straight into the
file.

There is one format. A binary one that stored the slots themselves
would open ten million numbers in about 0.3 s at best (storing ten
million slots alone takes 0.17 s) where the JSON takes 1.2 s: a second
saved on sheets near `max-cells`, and nothing a smaller sheet would
notice, for the loss of what the JSON gives, diffs, merges, and a file
anyone can read and fix.

## The grid

Nothing costs what the grid costs. The cells' occupancy
indexes (bitmaps of 1024 rows per column) say which cells are stored and
which have contents; ranges are read through them, so a whole column of
ten cells is ten visits. Formats of whole columns, rows and the sheet
live on the lines, not on cells. `TestCommandsCostTheDataNotTheGrid`
(`internal/ui`) runs every command with the whole sheet, a whole column
and a whole row selected on the full grid and fails past 250 ms or 8 MB;
each takes 0.1 to 4 ms.

What each part of 012 visits when given a range or selection:

| Where | Visits |
|---|---|
| Range reads for aggregates (`Book.Fold` and `Book.Scan`) | the stored cells, in row order |
| SUMIF(S), COUNTIF(S), AVERAGEIF(S), SUMPRODUCT, COUNTBLANK | the cells any range holds; the blanks between counted at once |
| MATCH, VLOOKUP, HLOOKUP, XLOOKUP | the stored entries, the blanks between as one; ROWS and INDEX keep the full size |
| Range users (`rangeIndex`), found per changed cell | interval trees per used column, whole rows in one |
| Formatting whole columns or rows | column, row and sheet formats |
| Used range, Ctrl+arrow, text running into view | the index of filled cells |
| Filter over whole columns | rows with data, one test for the blank rest, stepped over at once |
| Sort | the rows with cells |
| Paste, fill of blanks | the clip's cells per tile; what blanks land on is cleared |
| Formats pasted or moved | whole columns and rows: their line formats, and the cells where formatted lines cross them; a block: its blank cells get formats of their own up to 65,536 cells, then only the stored cells do |
| Formulas re-inferring after a line's format changes | the dependency indexes, not the line's cells |
| Copy to the system clipboard | the cells; refused past `max-cells` |
| Pivot sources, JEV value ranges, chart detection | the rows with data; JEV ranges past 4096 cells trimmed to their data |
| Exports of a selection, script reads | trimmed to the data |
| Row numbers, name box | 7 digits (the header widens past 9999), XFD |
| Column width reset, set | one undo step; a reset touches only columns with a width |
| Row heights set, fit | one undo step; kept per row, so more than 65,536 rows (whole columns) stop at the last row holding a cell; a fit touches only rows with a height |
| Borders over whole columns or rows | their line formats, and the cells where formatted lines cross them; the facing edges of the neighbors along the outline |
| Merging | the cells the range holds; one change makes at most 10,000 merged cells, so merging a whole column's rows is refused |
| Drawing wrapped text, borders and merges | the rows on screen: the cells of each whose own style wraps or draws borders, from an index by row, and the merges crossing it |
| XLSX column widths and styles | the data's width (at least 256), a style on A and XFD as the sheet's |

A sheet of 10,000 numbers spread down a million rows under 3000
whole-column formulas (sparse-1M) edits in 1.6 ms, draws a 200 x 60
frame in 0.68 ms, and answers arrow keys at XFD1048576 in 0.65 ms.
Typing an entry under 1000 SUMs over a column, through to its frame at
200 x 60, takes 0.91 ms.

The index costs a few percent on edits and loads (keeping two bitmaps
current per cell), and running aggregates cost memory for their
checkpoints (a 1000-SUM fan-in allocates 2 MB per recalculation). A
range is shared only from its second read in a recalculation, so ranges
read once cost nothing extra.

## Formula topologies

`BenchmarkEdit` sets one cell and recalculates what depends on it;
`BenchmarkRecalcAll` recomputes everything. 8192 is a full column.

| Topology | Edit | Full recalc | Undo + redo |
|---|---|---|---|
| Independent cell in 8192 x 26 numbers | 0.7 us | nothing to compute | 1.2 us |
| Chain: A(n) = A(n-1)+1 down 8192 rows | 1.9 ms | 1 ms | 3.8 ms |
| Fan-out: one cell read by 8192 formulas | 1.3 ms | 0.8 ms | 2.6 ms |
| Volatile: 8192 TODAY() and RAND() | 1.3 ms | 0.8 ms | 2.6 ms |
| 1000 named ranges, one formula each | 55 us | 1.2 ms | 110 us |
| Fan-in: 1000 x SUM(A1:A8192), or SUM(A:A) | 0.53 ms | 0.48 ms | 1.1 to 1.2 ms |
| Running totals: 8192 x SUM($A$1:An) | 2.7 ms | 1.8 ms | 5.8 ms |
| Sparse: 10,000 numbers spread down a million rows, 1000 each of SUM(A:A), running totals and VLOOKUP over A:B | 1.6 ms | 1.5 ms | 3.2 ms |
| Criteria: 60 SUMIF, COUNTIFS and AVERAGEIF over whole columns of 8192 rows | 29 ms | 44 ms | 59 ms |
| Lookup: 300 VLOOKUP, MATCH and XLOOKUP into 8192 keys | 12 ms | 12 ms | 27 ms |
| 10 M numbers (1,000,000 x 10) | 0.7 us | nothing to compute | 1.2 us |

A full recalculation computes only formulas and what pivots and spills
write: numbers and text hold their values from the moment they are
stored. Every volatile formula is recomputed on every change, as in
Sheets, so 8192 volatile cells add about 1.2 ms to every edit anywhere. Criteria
and lookups cost what their ranges hold, not what the grid holds: the
criteria collect positions from the occupancy index, and lookups walk
the stored entries with a cursor.

Through a real terminal (`make stress-e2e`: 200 x 60, 8192 x 26 imported,
200 SUMs over column A): arrow keys p50 8.3 ms, p95 10.2 ms; an entry
that recalculates the 200 SUMs p50 62 to 70 ms, p95 67 to 75 ms. The
arrow floor is Bubble Tea's renderer, which draws at most 120 frames a
second (`ui.FrameRate`, its maximum; the default is 60), so a key waits
up to 8 ms for the next frame; 012's own share of an arrow key is 0.2
to 1 ms. At 60 frames a second the same arrows take p50 16.6 ms, p95
18.4 ms. The renderer writes a frame only when the view changed, so an
idle 012 costs the same at either rate: 0.6 to 2.2 ms of CPU a second
on an empty sheet over 30 s, within the noise of measuring it.

## The function library

Functions live in `internal/functions`, behind a `Book` interface whose
methods pass only values (see [Architecture](architecture.md#functions)):
SUM-like functions have the engine add a range up (`Fold`), other range
walks read chunks into buffers the `Reader` reuses (`Scan`), and lookups
walk positions with a cursor, so no function makes a call or an
allocation per cell it reads. The criteria and lookup shapes in
[Formula topologies](#formula-topologies) exercise the paths other than
`Fold`. Plain arithmetic, 1000 formulas and a SUM, recalculates in 20 us
in binary floating point and 156 us in decimal.

## Rendering

Only visible rows and columns are drawn, so frame time depends on the
terminal size, not the sheet. A frame is `View` plus what Bubble Tea
does with its string: parse it into a cell buffer and diff it onto the
terminal (`BenchmarkFrame`).

| Sheet | 80 x 24 | 200 x 60 | 400 x 120 |
|---|---|---|---|
| Empty | 0.14 ms | 0.63 ms | 2.3 ms |
| 8192 x 256 numbers | 0.17 ms | 0.80 ms | 3.1 ms |
| 8192 rows of 500-character unicode text | 0.30 ms | 1.26 ms | 4.2 ms |
| 1000 named ranges | 0.24 ms | 0.73 ms | 2.4 ms |
| 20 charts (text) | 0.78 ms | 2.7 ms | 6.0 ms |

More than half of a frame at 400 x 120 is Bubble Tea's parse and diff of the
view string.

Conditional formats and validation cost what the screen shows. A cell's
look is worked out when it's drawn and kept until the next
recalculation (at most 65,536 cells, then the cache starts over), and a
color scale reads its range's numbers once per recalculation: its
lowest and highest, and a percentile by selection, not by sorting.
With a 3-point color scale over all of 8192 x 26 numbers (213 k cells,
every cell on screen a shade), at 200 x 60 (`BenchmarkFrame`,
`BenchmarkKeystroke`, `scale-8192x26`):

| | Without rules | With the scale |
|---|---|---|
| A frame, or an arrow key through to it | 0.80 ms | 1.2 ms |
| Typing a number and Enter, through to the frame | 0.81 ms | 2.2 ms (the scale's percentile over 213 k numbers) |

Shades and rule colors keep their escape codes, so a plain cell on one
costs a string concatenation, not a style render. A custom formula is
evaluated for each cell drawn, once per recalculation, with its
references moved for the cell. Top values, averages and duplicates
read their ranges once per recalculation, as a scale does.

Data bars and icons are drawn a column at a time, their glyphs with the
escape codes of their roles kept for the frame, so a cell under one costs
more than a shade. `bars-8192x26` puts a data bar over
13 columns of 8192 x 26 numbers and arrows over the other 13
(`BenchmarkFrame`):

| | 80 x 24 | 200 x 60 | 400 x 120 |
|---|---|---|---|
| 8192 x 26 numbers, a frame | 0.17 ms | 0.80 ms | 2.8 ms |
| The same with bars and icons on every cell | 0.26 ms | 1.4 ms | 4.6 ms |

Wrapped text, borders, row heights and merged cells cost what the
screen shows too. A sheet with none of them is drawn a line per row
without asking more; with them, each row drawn is measured once per
change to the sheet (the text its cells wrap, from an index of the cells
that wrap or draw borders, and whether a border lies along its top), and
a frame keeps each border string it has drawn in a role for the rest of
the frame. `laidout-8192x26` borders every cell of 8192 x 26 numbers,
wraps a column of notes over three lines and merges a title across the
top (`BenchmarkFrame`, `BenchmarkKeystroke`):

| | 80 x 24 | 200 x 60 | 400 x 120 |
|---|---|---|---|
| 8192 x 26 numbers, a frame | 0.17 ms | 0.80 ms | 2.8 ms |
| The same laid out, a frame | 0.24 ms | 1.22 ms | 4.1 ms |
| The same laid out, an arrow key through to its frame | 0.25 ms | 1.23 ms | |

A chart drawn as text costs 4 to 50 us at 24 x 10 to 120 x 40 cells,
whatever its data: only the categories that fit are drawn (a pie of 8192
slices, 0.44 ms, is the worst). An image for kitty graphics is redrawn
when its data, size or theme changes: 25 to 260 us at 24 x 10 cells and
1 to 1.5 ms at 120 x 40, a pie 8.6 ms (`internal/chart`,
`BenchmarkDraw`, `BenchmarkImage`). A sixel image is encoded then too,
in 0.07 to 0.3 ms at 24 x 10 cells and 3 to 7 ms at 120 x 40 (10 x 20
pixel cells), 0.6 to 67 KB to send (`BenchmarkSixel`); it is sent again,
without encoding, whenever the screen under it is redrawn. Key presses add little: an arrow key
through to its frame is 0.17 ms at 80 x 24 and 0.8 ms at 200 x 60;
Page Down 1.0 ms.

## Selection statistics

With the whole of an 8192 x 256 sheet selected, the status line's Sum,
Avg and Count cover 2.1 M cells. On a sheet of more than 16,384 cells,
a selection of more than 4096 is summed from an index of each column's
sum, count and count of numbers (`internal/sheet/stats.go`), kept on the
blocks of the index of filled cells: an entry per 64 rows and a total
per 1024, made only where there is data. A selection adds up the whole
blocks it covers, plus the rows at its ends a cell at a time. The index
is built on first use, in one pass over the cells, and a stored, deleted
or recalculated cell marks its block to be summed again when a selection
next covers it. Sums are recomputed, never adjusted, so they don't
drift. The result is cached until a cell changes. Each key that changes
a selection over the whole sheet, through to its frame
(`BenchmarkKeystroke`):

| Key, 8192 x 256 numbers | 80 x 24 | 200 x 60 |
|---|---|---|
| Shift+Up and Shift+Down with the data selected (`extend-data`) | 0.33 ms | 1.85 ms |
| Ctrl+Shift+Down and Ctrl+Shift+Up over the first row (`extend-edge`) | 0.29 ms | 1.56 ms |
| Ctrl+A, the selection standing (`select-data`) | 0.29 ms | 1.78 ms |

The index costs memory only on sheets that had a large selection, in
proportion to their data, and a nil check per stored or recalculated
cell.

## Imports

Real, openly licensed datasets fetched by `scripts/stress-data.sh`
(sources, licenses and checksums in `scripts/stress-data.tsv`):

| File | Shape | Import | Throughput | Heap per cell |
|---|---|---|---|---|
| OWID energy (CSV) | 23,377 x 130, 1.04 M cells | 0.25 s | 4.2 M cells/s | 23 B |
| Airport codes (CSV) | 86,134 x 13, 855 k cells | 0.39 s | 2.2 M cells/s | 55 B |
| NOAA daily CO2 (CSV) | 18,304 x 2 | 12 ms | 3.1 M cells/s | 63 B |
| Country codes (CSV, Arabic, CJK) | 249 x 56 | 5.4 ms | 2.3 M cells/s | 59 B |
| Apache POI formula tests (XLSX) | 788 rows, 1189 formulas, 3828 cells | 8.5 to 10 ms | 380 to 450 k cells/s | 239 B |
| Chinook PlaylistTrack (SQLite) | 8715 x 2 | 6.1 ms | 2.9 M cells/s | 21 B |
| Parquet alltypes_tiny_pages | 7300 x 13 | 27 ms | 3.5 M cells/s | 44 B |

Past `max-cells` (or the grid), imports keep whole rows up to it and
say how many rows they left out. Parquet files and SQLite tables stop
reading at the last row and take the count of the rest from the file;
CSV, TSV and SQLite queries are read to their end to count it, without
keeping it. Every importer streams, so memory follows the sheet, not the
file. 012's XLSX reader streams a worksheet a token at a time, keeping
only the shared strings (in one buffer, 4 bytes a string besides the
text), the cell formats, and the first cell of each shared formula.
XLSX imports set column widths and styles for the data's columns (at
least 256) without an undo step each.

XLSX imports of full sheets written by 012's exporter
(`BenchmarkImportXLSX`) and the POI file. Peak heap is sampled while
importing, per cell kept, and includes garbage not yet collected:

| Workbook | Time | Allocated | Allocations | Heap per cell after | Peak heap per cell |
|---|---|---|---|---|---|
| 8192 x 26 numbers (213 k cells) | 147 to 154 ms | 171 MB | 3.3 M | 20.5 B | 114 B |
| 8192 x 26 table, text and numbers | 262 to 275 ms | 246 MB | 7.7 M | 21 B | 119 B |
| 8192 formulas (`=A1+1` down a column) | 14.5 ms | 18.6 MB | 321 k | 755 B | 2061 B |
| Apache POI formula tests | 8.5 to 10 ms | 9 MB | 162 k | 239 B | 965 B |

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

## Following files

`BenchmarkFollow` (`internal/ui/followstress_test.go`) follows a CSV
log while another goroutine appends 10,000 rows a second to it for three
seconds, with `=SUM(D:D)`, `=COUNTIF(B:B,"error")` and an AVERAGE over
the region, polling as the UI does and drawing a 200 x 60 frame after
each update:

| Rows kept | Update applied (slowest) | Frame (slowest) | Rows a second taken in |
|---|---|---|---|
| Every row (30,000) | 6.7 to 6.8 ms | 3.2 to 3.6 ms | 8,200 |
| The last 1000 | 1.7 to 2 ms | 2.8 to 3.2 ms | 8,300 |
| The last 10,000 | 13.8 to 13.9 ms | 2.6 to 3 ms | 8,000 |

(Rows a second counts to the last row read, after the writer stopped;
the writer itself manages a little under 10,000 with its pauses.) An
update costs the rows it brings, and with a window the rows it keeps:
the window's rows move up one row each on every update, so a window of
100,000 rows under a fast log would take about 140 ms an update. A
followed file costs one `os.Stat` four times a second while it doesn't
change; a growing file is read a megabyte a poll at most, so a large
file loads over several frames. Rows are held once, in the region's
cells; a window reads the rows it keeps back from them.

## Linked sources

A [linked source](../files/sources.md) is read in place: its tab keeps
at most 48 pages of 128 rows, and what formulas and pivot tables ask
of it is worked out by streaming the file in the background, a few
questions at a time, each answer kept until the file changes. None of
it costs what the source's size does in memory. Measured on ten million
rows of sales (an id, one of eight categories, an amount and a date;
`stress.SalesParquet` and `stress.SalesSQLite`, 204 MB and 385 MB), one
run each, the peak heap above what was live before:

| Reading the file (`BenchmarkSource`, `internal/fileio`) | Parquet | SQLite |
|---|---|---|
| Open | 0.39 ms, 0.2 MB | 0.65 s (counting the rows) |
| Scan one column | 0.35 s (28.5 M rows/s), 4.5 MB | 1.36 s (7.4 M rows/s), 3.4 MB |
| A page of 60 rows deep in, in the source's order | 0.28 ms | 0.19 ms |
| Sorting by a column, Z to A | 3.5 s, 113 MB | 19.1 s, in SQLite's own memory and files |
| A page deep in, sorted | 12 ms (the rows are scattered) | 0.41 ms |
| Filtering to one category (1,248,497 rows) | 1.26 s, 6.7 MB | 1.19 s |
| A page deep in, filtered | 0.23 ms | 0.23 ms |

| Answering a question (`BenchmarkSourceFormulas`, `internal/paged`) | Parquet | SQLite |
|---|---|---|
| `SUM`, `AVERAGE` of a column | 0.42 s, 3.8 MB | 1.42 s, 2.6 MB |
| `COUNTIFS`, `SUMIFS` over two columns | 1.2 s, 8.3 MB | 2.3 s, 7.5 MB |
| `SUMPRODUCT` of two columns | 0.44 s, 9 MB | 1.56 s, 7.7 MB |
| `XLOOKUP` of the last id | 0.62 s, 5.9 MB | 1.28 s, 4.3 MB |
| `MATCH` of the middle id | 0.31 s, 5.9 MB | 0.63 s, 3.8 MB |
| `MEDIAN`, which holds every value: ten million cells are within `max-cells` | 1.15 s, 248 MB | 2.15 s, 247 MB |
| A pivot table: SUM and COUNT of the amounts by category | 2.45 s, 5.9 MB | 3.9 s, 2.7 MB |

SQLite's own memory, through modernc's libc, isn't in the Go heap.
Functions over aligned ranges (the criteria functions, `SUMPRODUCT`)
read their ranges a window of 2048 rows at a time, each column
streamed by a goroutine of its own in batches of 4096 rows. A function
that holds what it reads is refused past `max-cells` cells before it
reads any, so `MEDIAN` of a hundred million values says so at once
rather than holding them.

Scrolling (`BenchmarkSourceScroll`, `internal/ui`) draws a frame in
0.26 ms at 80 x 24 and 0.9 ms at 200 x 60 within the pages read, as a
sheet's frame costs. A jump anywhere, as dragging the scrollbar makes,
reads the pages it shows and a window either side, in the background
while the frames go on showing `…`: 0.64 ms at 80 x 24 and 1.5 ms at
200 x 60, the pages read and the frame drawn.
The speed gate holds a frame on a 200,000-row source and a SUM over it
to their baselines (`frame/source-200000x4`, `source/sum-200000`).

## Undo

Each step keeps the cells it changed as they were, in the form the
sheet keeps them (`historyimage.go`): a plain cell as its 16-byte slot
in columns of blocks, its text and formatting in the step's own tables,
and a whole `Cell` only for a formula or a note. A step
of one cell or a few keeps them in a list. Undoing costs about what the
change cost, since it recalculates the same cells. Clearing 213 k cells
and undoing it takes 86 ms; clearing all ten million cells of a full
`max-cells` sheet of numbers holds 193 MB, and with its undo takes 6.2
s. A deleted or replaced sheet is kept whole, so its step costs nothing
more.

The history is bounded by steps (`MaxUndo = 100`) and by an estimate of
the memory its before-images hold (`MaxUndoBytes = 256 MB`); past either,
the oldest steps are dropped first, and the newest step is kept however
large it is. Each cell is counted as it is recorded (a slot, a block's
share, its text once per step, more for a formula, whose parsed tree
stays alive), so the budget costs nothing to check. The estimate follows
the measured heap within about 10% (`BenchmarkHistoryFull`,
`BenchmarkHistoryWide`, `BenchmarkClearMax`):

| Benchmark | Result |
|---|---|
| 100 edits rewriting a whole column (819 k before-images) | 15.9 MB held (15.9 MB estimated), all 100 kept |
| 12 edits rewriting all of 8192 x 26 (2.6 M before-images) | 49.3 MB held (49.4 MB estimated), all 12 kept |
| Clearing all of 1,000,000 x 10 numbers | 193 MB held (193 MB estimated) |
| Undo + redo of one cell (dense, names) | 1.2 us, 109 us |

A change whose step would hold more than `MaxStepBytes` (1 GB), as
`Sheet.UndoCost` estimates it before the change (a slot a cell, the
formulas whole, the range's share of the strings), asks first on the
context line: "This can't be undone: it would take N MB of undo
history." Enter runs it `WithoutUndo`, recording nothing and forgetting
the history, which could no longer be undone past it. Plain cells cost
about 20 B each, so only millions of formulas or of long distinct texts
changed at once come near it.

## JEV

Each JEV formula is volatile: every recalculation looks its question up
in the cache. A formula shown Loading… is noted against the question it
waits for (by `RemoteCall.Key`), and an answer recalculates only the
formulas waiting for it and what reads them
(`Workbook.RecalcAnswered`). Answers are stored as they arrive; the
first since the last recalculation schedules one a frame later
(`ui.FrameInterval`), which covers every answer stored meanwhile, so
answers arriving together recalculate once. With a fake client
answering instantly (`BenchmarkJEV`; the benchmark doesn't wait the
frame, so answers queued together, up to 8, recalculate together):

| JEV cells | Per answer | All answers |
|---|---|---|
| 100 | 3.5 us | 0.35 ms |
| 1000 | 3.6 us | 3.6 ms |
| 4000 | 3.8 us | 14 to 15 ms |

Against the real service the network dominates: at most 8 questions are
in flight, so 4000 questions at 300 ms each take 2.5 minutes, while the
recalculation costs a few microseconds per answer whatever the number
of JEV cells. Other changes recalculate every JEV cell, as every
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
`.012` file holding a deeper formula fails to open with that message and
the cell; imports keep such a formula's text or cached value, as for any
formula they can't read.

Evaluation recurses once per link of a chain of formulas, which no
parser cap bounds, so it counts its depth in cells and operators: a cell
reached past 65,536 levels is put off. The evaluation in progress is
abandoned (its cells go back to dirty), the cell is evaluated from the
top of the stack, and the abandoned evaluation is retried and finds it
done. Values are the same as with unbounded recursion, a cycle longer
than the limit is still a cycle, and the stack stays under 32 MB
(`TestFullSheetChain`, run with `SHEET_FULL_CHAIN=1`, checks it under a
64 MB limit: 4 s for a chain through all 2.1 M cells, building
included).

| Benchmark | Time |
|---|---|
| Parse (`BenchmarkParse`) | 2.4 us |
| 1000 nested parentheses / IFs, parse and evaluate | 203 / 438 us |
| 10,000 nested parentheses / IFs | 1.1 / 2.0 ms to refuse |

## Several sheets

A workbook recalculates all its sheets together. Formulas that name a
sheet (`Data!A1`) are indexed by the sheet names they use: a changed cell
on a sheet some formula names scans those cross-sheet formulas, so a
change there costs O(changed cells x cross-sheet formulas); cells on
sheets no formula names skip the scan. Each sheet's lookup, which
resolves sheet names, is made once per recalculation, and the
recalculation state stays on each sheet keyed by address. Every cell
read goes through one function call to resolve its sheet, 3 to 5% of a
read-heavy recalculation.

| Benchmark (`internal/sheet`, `go test -bench SheetEdit`) | Edit |
|---|---|
| 5000 formulas on one sheet reading `A(n)` and `SUM(A1:A10)` | 1.65 ms |
| The same 5000 formulas reading `Data!A(n)` and `SUM(Data!A1:A10)` | 2 ms |

## Serving over SSH

`BenchmarkSessions` (`internal/serve/stress_test.go`) opens N sessions,
each over its own SSH connection on loopback at 120 x 40, optionally
opening the same 1000 x 26 sheet of numbers in every one, a copy each
(`--share off`) or shared in one room, then has every
session press an arrow key at once, 200 times, timing each key from the
client's write to the first bytes of its frame arriving. The clients
run in the same process, so the heap and CPU figures include their side
of the connections and are upper bounds for the server. Sessions ask
for 120 frames a second, as the local app does (`-benchtime 200x`):

| Sessions, sheet | Heap per session | Key to frame p50 / p95 / max | CPU per frame |
|---|---|---|---|
| 10, new sheet | 1.35 MiB | 5.1 / 8.3 / 9.8 ms | 0.83 ms |
| 10, 1000 x 26 numbers | 1.9 MiB | 3.9 / 8.3 / 10.8 ms | 0.79 ms |
| 50, new sheet | 1.30 MiB | 5.0 / 8.7 / 17.8 ms | 0.53 ms |
| 50, 1000 x 26 numbers | 1.9 MiB | 5.1 / 8.9 / 12.8 ms | 0.66 ms |
| 10, 1000 x 26 numbers, shared | 1.5 MiB | 5.0 / 8.3 / 16.6 ms | 1.27 ms |
| 50, 1000 x 26 numbers, shared | 1.5 MiB | 7.1 / 15.8 / 33.2 ms | 0.79 ms |

Latency doesn't move from 10 to 50 sessions: it's Bubble Tea's frame
pacing (at most one frame every 8.3 ms, so a key waits half a frame on
average), not load. A heap profile with 50 sessions open
(`SERVE_HEAP_PROFILE=file`) puts three quarters of a session's heap in
Bubble Tea's render buffers (about 1 MiB at 120 x 40, growing with the
window), 130 KiB in its input key table and under 50 KiB in both ends
of the SSH connection, so the figure is close to the server's own. On
top come 20 to 60 B per cell of data of the sheets a session has open;
sessions opening the same file each hold a copy. CPU per frame covers
both ends of the connection (the encryption twice, the client reading
the frame); at 0.7 ms, 50 sessions typing continuously at 120 frames a
second would keep about four cores busy: frames are drawn only when a
session's screen changes, so the rate is the most keys can cause, not a
cost of being connected.

Sessions sharing a workbook hold it once, so each holds only its
screen. They take turns on it: each key is handled under the room's
lock, and moves a pointer every other session draws, so a key costs a
frame in every session of the room (CPU per frame counts the pressing
session's alone), and fifty sessions pressing keys in the same
millisecond queue for the lock, the last waiting about two frames. A
change reaching the others is quick next to the frame: from the end of
one session's turn to another's frame showing it takes about 0.1 ms
(`TestSharedEditWithinAFrame`), so it goes out with the other's next
frame.

## Pivot tables

A pivot table is recomputed whole whenever a cell of its source range is
recalculated: it reads every source row once, groups it by a comparable
key struct (no string built per row), and rewrites only the result cells
that changed, so what reads the results recalculates only when they
move. `BenchmarkPivot` edits one cell of a full 8191-row table (an id, a
category of 8, a number and one of 1000 items, twice) under a pivot and
times the edit, the recalculation and the pivot together:

| Pivot | Result rows | Edit | Allocations |
|---|---|---|---|
| Rows: 8 categories; SUM and COUNTA | 10 | 0.79 ms | 196 |
| Rows: 8 x 8 categories with subtotals; columns: 8; SUM and AVERAGE | 75 | 2.2 ms | 2.5 k |
| Rows: 1000 items sorted by their sum; SUM and COUNTUNIQUE | 1001 | 2.5 ms | 14 k |
| Frequency table of the 1000 items | 1001 | 1.8 ms | 24 k |

## Arrays and spills

An array stores only its block holding data: a whole column read as one
(`FILTER(A:A, ...)`, `SORT(B:B)`) holds the rows with data, and the
blank rows past them are one fill value, spilled as nothing. A range
read whole is read once per recalculation and shared by every formula
reading it (`Reader.Forget`), so a thousand FILTERs of one column read
it once. An array stores at most `maxArray` values (2,097,152; about 64
MB), past which the formula is `#VALUE!`; a spill writes at most
`max-cells` cells. Spilled cells are written after each evaluation pass
(`spill.go`), only those that changed, and not at all when an anchor
computes the array it already spilled with nothing in its cells
changed; formulas reading spilled cells that changed are recalculated in
another pass, at most 64 of them.

`BenchmarkEdit/arrays-1000xFILTER8192` (`internal/stress`: a column of
8192 numbers beside eight categories, 500 FILTERs of the numbers by a
category, 1024 rows each, and 500 UNIQUEs of the categories, all from
row 1, 516 k spilled cells) edits one number, recomputing the 500
FILTERs, 63 of whose arrays change:

| Measure | Time | Allocated |
|---|---|---|
| Edit a number | 52 ms | 161 MB, 7.8 k allocations |
| Undo and redo it | 106 ms | |
| Full recalculation | 106 ms | |
| Build (load and recalculate) | 181 ms | |

Most of an edit is the FILTERs' own work: comparing 8192 categories each
(`B1:B8192="alpha"`, about 20 ns a value) and picking the rows that
pass. Array arithmetic allocates the array it computes, so an edit
allocates about 32 bytes per value its conditions compare.

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

## What would raise the bounds

Sizes and scheduling are in the [roadmap](../../ROADMAP.md#1-scale).
`max-cells` stands at ten million, as Google Sheets' limit does; the
sheet itself would hold several times that in a few GB, and opening,
saving and undoing cost about what the cells do ([Sheet size](#sheet-size),
[Undo](#undo)). Derived cells cost what plain cells cost, a slot each
([Sheet size](#sheet-size)); what stays large is a formula, about 750 B
with its parsed tree and reference lists, and the array a spilling
formula keeps, 32 B a value.

## Measuring

```sh
make stress           # fetch datasets, run everything, record the run (5 minutes)
BENCH='Edit|Frame' make stress   # a subset; BENCHTIME=2s for steadier numbers
make stress-report    # latest against previous, baseline and last release, with trends
make stress-e2e       # key press to screen through libghostty
```

`make stress` fetches the datasets with `scripts/stress-data.sh`, which
checks each against its checksum in `scripts/stress-data.tsv`.
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
`internal/stress`, up to `dense-1Mx10`, ten million numbers, the
default `max-cells`. Results accumulate in `.deps/stress/results` and, with
the observability stack up, in ClickHouse.

### The speed gate

`make speed`, part of `make check`, catches a clear slowdown before a
push rather than at the next stress run. `TestSpeed`
(`internal/ui/speed_test.go`) builds mid-sized sheets with
`internal/stress` and draws frames through the stress benchmarks' fake
terminal, so it measures what `BenchmarkKeystroke` and `BenchmarkEdit`
do, on fewer cases and briefly: an arrow key through to its frame at
200 x 60 (dense, laid out, color scale and chart sheets of 8192 rows,
and a notebook output's grid of 100,000 rows, entered), single edits
(fan-in, a chain, criteria functions, lookups) and full recalculations
(running totals, arrays). It takes about two seconds.

Each case is held to `internal/ui/testdata/speed.json` two ways:

- **Allocations** an operation, counted with `testing.AllocsPerRun`,
  so the same code gives the same count on any machine and under any
  load. A case fails past 1.25 times its baseline plus 16.
- **Time** relative to a calibration workload (formatting numbers,
  string map lookups, sorting) timed beside it in the same process,
  the fastest of seven timings of each, so a slower or busier machine
  slows both. A case fails past twice its baseline's ratio. Ratios
  still differ between CPUs, so they are compared only on the
  baseline's OS and architecture (the file records them); elsewhere
  only allocations are.

A case over its margin is measured once more before it fails. Runs on
one machine vary by up to about a third, under load from other tests
too, well inside the margin; the margins are for regressions of a
multiple, and finer ones are what `make stress-report` is for.

The gate runs alone (`go test ./...` skips `TestSpeed` without
`-speed`), and while it times it holds `/tmp/012-bench.lock`, the lock
other timed runs on the machine take: with flock(2) when the path is a
file (`flock /tmp/012-bench.lock make stress` where the `flock` command
exists), and otherwise as a directory it makes and removes, which needs
no `flock` command. It waits up to ten minutes for a directory someone
else holds.

When it fails, run `make speed` again on a quiet machine, then look at
the case with the stress benchmarks (`BENCH='Keystroke|Edit' make
stress`). A slowdown that is intended, or a new case, is taken into the
baseline with `make speed-update`, which measures every case and
rewrites `speed.json` for the machine it runs on; commit it with the
change that explains it.
