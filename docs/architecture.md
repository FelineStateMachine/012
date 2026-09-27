# Architecture

012 is a single pure-Go binary (`CGO_ENABLED=0`) built on Bubble Tea v2 and
Lip Gloss v2.

```
cmd/012          entry point: flags, JEV setup, opening or importing a file
internal/sheet   the engine: cells, evaluation, functions, recalculation, undo, files
internal/formula the formula language: references, lexer, parser, printer, rewriting
internal/numfmt  number formats, rounding, General, date serials
internal/fileio  import and export: CSV, TSV, XLSX, SQLite, Parquet, Lotus .wk1
internal/chart   chart layout, text rendering and kitty image encoding
internal/jev     JEV configuration, answer cache and TypeSafe client
internal/telemetry  opt-in JSON log and OTLP export of spans, events and frame stats
internal/stress  synthetic worst-case sheets for the -tags stress benchmarks
internal/ui      the Bubble Tea model: modes, menus, overlays, rendering
  theme          style roles and the widgets drawn with them (frames, key chips)
  rowtext        laying out a row of cell text across the columns on screen
  formula        reading the formula being typed (F4, the word and call at the caret)
e2e/             end-to-end tests through libghostty (separate module, cgo)
oracle/          differential tests against excelize (separate module)
```

## The engine

`internal/sheet` knows nothing about terminals. It builds on two packages
that know nothing about cells, and dependencies point one way:

```
internal/ui, internal/fileio, internal/chart, oracle
        |
internal/sheet  --->  internal/formula
        |
        +-------->  internal/numfmt
```

- `internal/formula` is the language: `Addr` and `Rect`, A1 references
  with their `$` markers, sheet names in references and their quoting,
  the lexer and Pratt parser, the syntax tree, the printer and the
  reference rewriting for copies, moves and inserted or deleted rows and
  columns. The parser learns which functions exist through a small
  `Func` interface that the engine's `FuncDef` implements.
- `internal/numfmt` renders numbers: number format patterns (as in TEXT
  and custom formats), Sheets' General form, rounding on 15 significant
  digits, and the calendar of serial day numbers.

`sheet` re-exports what its callers used before (`sheet.Addr`,
`sheet.Rect`, `sheet.Parse`, `sheet.FormatPattern`, the sheet-name
helpers) as aliases and thin wrappers, so the UI, importers and charts see
one engine package. Typed-entry recognition (`ParseValue`, which turns
"$1,200" or "9/26" into a number and a format) stays in `sheet`: it yields
the engine's `Format` and reads its clock.

A `Workbook` holds ordered `Sheet`s, the named ranges, the undo history,
the arithmetic setting, the JEV source and recalculation, as a Sheets
spreadsheet does. Each sheet keeps its cells, sparse, by address, with
its column widths, frozen panes, filter and charts; each cell keeps what
was typed, its parsed expression, its computed value, and its format and
style.

- **Cells.** Each sheet keeps its cells in a `cellStore` (`store.go`):
  get, set, delete, count, iterate all or a range. Nothing else touches
  the map beneath, so the representation can change without the rest of
  the engine noticing.
- **Parsing.** `internal/formula`'s hand-written Pratt parser turns
  formulas into an AST, keeping absolute markers so references can be
  rewritten when cells move. Its printer turns ASTs back into text in
  Sheets' spelling. Decimal arithmetic marks the operators it computes
  with node types of the engine's own, so trees stay as parsed.
- **Recalculation.** Formulas record the cells and ranges they read on
  their own sheet; references that name a sheet (`Sheet2!A1`) are kept by
  name and resolved when evaluated, so renaming rewrites them and a deleted
  sheet's references wait, as `#REF!`, for a sheet of that name. Changing a
  cell marks it and everything that transitively depends on it, on any
  sheet, then evaluates the marked cells lazily in dependency order; a cell
  reached again while it is being evaluated is part of a cycle. Volatile
  functions (TODAY, RAND, the JEV functions) are recomputed on every
  recalculation (`recalc.go`). Formulas read other cells through a
  `lookup`: `cell` for one, `cells` for a whole range, which the
  aggregates (SUM, AVERAGE, COUNT and the rest) use.
- **Functions.** One table (`FuncDef`) holds every function's name,
  signature, description and arity; it drives parsing, evaluation,
  autocomplete, in-app help and [functions.md](functions.md).
- **Undo.** Every mutation goes through a small set of paths that snapshot
  the cells, widths, names, charts, view state and sheet list they change,
  on any sheet, so a step can be reversed exactly; multi-cell operations
  are one step. A deleted sheet keeps its cells, so undo brings it back.
- **Pivot tables.** A sheet may hold a pivot (`pivot.go`): a definition
  naming its source by sheet name, as formulas do, and a region of
  derived cells from A1 that the engine owns. When a recalculation marks
  a cell of a pivot's source range, or its definition changes, the pivot
  is recomputed after the formulas (`pivotcalc.go` groups the source
  rows into a tree of accumulators, `pivotlayout.go` lays out Sheets'
  table) and only the result cells that changed are rewritten, outside
  the undo history, then what reads them recalculates, pivots of pivots
  included (bounded, as a loop can't be defined). Undo restores the
  source and the definition, and the results follow. Derived cells are
  constants to formulas, values to copies and exports, refused by `Set`,
  and never saved: the file keeps the definition (`pivotfile.go`). A
  frequency table is a pivot with a preset definition.
- **JEV.** The engine never touches the network. JEV functions describe a
  question and look up the answer in the workbook's `RemoteSource`, set
  with `SetRemote`; `internal/jev` answers from a cache and queues new
  questions, and the UI sends them as background commands.

### Where the engine can grow

The package boundaries leave the bounds in [limits.md](limits.md) room to
move without touching callers:

- **Storage.** Compact cell storage (column blocks of values, formulas and
  formats in side tables) replaces `cellStore`'s map; its methods are the
  whole contract.
- **Range reads.** Shared range results, prefix sums for running totals
  and column-block scans go behind `lookup.cells`, the one place
  aggregates read ranges.
- **Files.** A streaming or columnar `.012` format replaces `file.go`'s
  whole-document JSON; the format is already separate from the store.
- **Depth limits.** A cap on nesting goes in `internal/formula`'s parser,
  so a pathological formula fails to parse before anything evaluates it.
- **Functions.** The function library (values, evaluation, `FuncDef` and
  the `functions_*.go` tables, about a third of `sheet`) is the next
  boundary: it needs cells only through `lookup`. It stays in `sheet`
  for now because `lookup` is a concrete type (an interface across a
  package boundary would put every range callback on the heap) and
  `Value` and `Format` would have to move below the engine with it.

## The UI

`internal/ui` is a Bubble Tea program. Inside the grid it follows Google
Sheets; around it, the control panel keeps a 1-2-3 look. See
[UX.md](UX.md) for the rules every change follows.

**Commands.** Every action is a registered command with a title and
description (`commands.go`), reached from key bindings, the menu bar,
context menus, the command palette, the help overlay and mouse gestures
that stand for one (double-clicking a tab renames it through
`sheet.rename`), so they can't disagree. `runCommand` is the one place a
command runs: telemetry times it there, and a command log or macro
recorder would attach there. A command that writes cells declares the
range it writes (`edits`), and `runCommand` refuses it over a pivot
table's results, so a new editing command is guarded by saying what it
edits.

**Model and components.** `Model` (`model.go`) is the root: it holds the
file, the mode and the note on the context line, owns one component for
each thing that takes input or draws part of the screen, routes each
message to the component it's for, and composes the screen from what they
draw. The components:

| Component | Type | Owns |
|---|---|---|
| grid (embedded) | `grid` | the sheet shown, active cell, scroll, window size, selection; mapping rows and columns to the screen, frozen panes, moving and selecting (`grid.go`, `panes.go`, `selection.go`) |
| edit line | `lineEdit` | the one-line editor shared by cell entries, prompts and search fields (`line.go`) |
| cell entry | `entry`, `assist` | typing into a cell, pointing at references, other sheets while pointing, formula suggestions and signatures (`entry.go`, `assist.go`) |
| prompt | `prompt` | a question on the context line, typed or pointed at (`prompt.go`) |
| overlays | `overlay` | whatever has taken over input: menus (`menuoverlay.go`), the palette and pickers (`palette.go`, `names.go`), the filter picker, the find, sort and choice bars, the chart editor and selection, the pivot editor (`pivoteditor.go`, `pivotactions.go`), the shortcuts |
| sheet tabs | `tabStrip` | where each sheet was left, the tab strip's scroll, layout and clicks (`tabstrip.go`) |
| mouse | `mouseState` | drags, hover, double clicks, the fill handle (`mouse.go`, `fill.go`) |
| import | `transfer` | the import in progress, its progress display and cancelling (`transfer.go`) |
| others | `clipboard`, `trace`, `chartState`, `jevRunner`, `terminal` | what Ctrl+V pastes, a trace being shown, chart commands' target, JEV questions in flight, what the terminal supports and the chart images sent to it |

Overlays implement the `overlay` interface (`overlay.go`): an indicator
for the mode, `key` and `mouse` handlers, a `layout` of boxes to draw, and
the status line while open; bars on the context line add a
`contextLine`, and those with a text field a `cursor`. Like `assist` and
`prompt`, they are handed the model when they handle input, since acting
on it is their job. A new overlay is a new type; `Model` needs no new
fields, only a way to open it (usually a command).

**Drawing.** `View` (`panel.go`) stacks the control panel, the header,
the grid rows and the status line, then composites the floating layers
with Lip Gloss: charts over the grid, then the open overlay's boxes or the
formula suggestions, so the grid never shifts under them. Only the visible
cells are rendered; `rowtext` lays out each row's text in one pass over
the cells that can reach the screen. Styles come from `theme`, a small set
of roles with dark and light variants on the terminal's 16 ANSI colors,
so the user's palette applies; its widgets (framed boxes, key chips, key
hints) are what every overlay is drawn with.

**Packages.** `theme`, `rowtext` and `formula` depend on nothing in
`ui`, so they can be tested and measured alone. The components stay in
package `ui` because they act on the model; moving them out would mean
exporting most of it. New leaf packages are split off the same way when
a part needs only the sheet or the theme.

## Files

`internal/fileio` moves data between sheets and other formats. Each format
is one row of the table in `formats.go` (its name, extensions, the words
the UI shows for it, whether it holds several sheets or tables, and its
importer and exporter) plus a file of its own; `Import`, `Export`,
recognizing a file by its extension, the import picker and the Download
menu all come from the table. Importers build a new workbook through the
engine's public API with a shared `builder`, which keeps text as text,
falls back to a formula's cached value when it can't be translated, and
counts what didn't fit. They stream: CSV and TSV are read record by
record, Parquet a batch of rows at a time, SQLite a row at a time, and
Parquet files and SQLite tables stop at the sheet's last row, taking the
number of rows left out from the file. XLSX goes through excelize, which
holds the worksheet in memory while its rows are read, so it is the one
format read twice; its files are small next to the others' and it's
bounded by the sheet's size. Exporters write a `Snapshot` taken on the UI
goroutine, in the background, through one atomic write-then-rename; CSV
and TSV are written a row at a time. One package suits formats that share
this much (the builder, number formats, serial dates and Excel formula
translation); a format that grew its own dependencies would move to a
subpackage behind the same table row.

## Charts

`internal/chart` draws a chart in two layers that share one layout: text
for any terminal (block elements for bars, braille for lines, half blocks
for pies) and, on terminals with kitty graphics, an image of the plot area
with the axes and legend still terminal text. Each chart type is a layout
in the `types` table, returning a plan that draws it both ways; the type
constants and their names belong to `internal/sheet`, since charts are
saved with sheets. Work is bounded by the chart's size, not its data: only
the categories that fit are drawn, pie slices are found by binary search,
and a pie image supersamples only pixels on a slice edge or the rim.

## Telemetry

`internal/telemetry` is off unless asked for. When on, spans, events and
per-second frame summaries go to a JSON log, an OTLP/HTTP collector, or
both (see [observability.md](observability.md)). The OTLP exporter uses
only the standard library: bounded queues per signal, drained by one
goroutine in batches; when a queue is full new items are dropped and
counted, so the UI never waits on the network. Operation durations are
histograms keyed by span name, which are fixed in the code.

## Testing

See [testing.md](testing.md).
