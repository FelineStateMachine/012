# Architecture

012 is a single pure-Go binary (`CGO_ENABLED=0`) built on Bubble Tea v2 and
Lip Gloss v2.

```
cmd/012          entry point: flags, JEV setup, opening or importing a file
internal/sheet   the engine: cells, parsing, evaluation, recalculation, undo, files
internal/fileio  import and export: CSV, TSV, XLSX, SQLite, Parquet, Lotus .wk1
internal/chart   chart layout, text rendering and kitty image encoding
internal/jev     JEV configuration, answer cache and TypeSafe client
internal/telemetry  opt-in JSON log and OTLP export of spans, events and frame stats
internal/stress  synthetic worst-case sheets for the -tags stress benchmarks
internal/ui      the Bubble Tea model: modes, menus, overlays, rendering
e2e/             end-to-end tests through libghostty (separate module, cgo)
oracle/          differential tests against excelize (separate module)
```

## The engine

`internal/sheet` knows nothing about terminals. A `Workbook` holds ordered
`Sheet`s, the named ranges, the undo history, the arithmetic setting and
recalculation, as a Sheets spreadsheet does. Each sheet keeps its cells in
a sparse map from address to cell, with its column widths, frozen panes,
filter and charts; each cell keeps what was typed, its parsed expression,
its computed value, and its format and style.

- **Parsing.** A hand-written Pratt parser turns formulas into an AST,
  keeping absolute markers so references can be rewritten when cells move.
  A printer turns ASTs back into text in Sheets' spelling.
- **Recalculation.** Formulas record the cells and ranges they read on
  their own sheet; references that name a sheet (`Sheet2!A1`) are kept by
  name and resolved when evaluated, so renaming rewrites them and a deleted
  sheet's references wait, as `#REF!`, for a sheet of that name. Changing a
  cell marks it and everything that transitively depends on it, on any
  sheet, then evaluates the marked cells lazily in dependency order; a cell
  reached again while it is being evaluated is part of a cycle. Volatile
  functions (TODAY, RAND, the JEV functions) are recomputed on every
  recalculation.
- **Functions.** One table (`FuncDef`) holds every function's name,
  signature, description and arity; it drives parsing, evaluation,
  autocomplete, in-app help and [functions.md](functions.md).
- **Undo.** Every mutation goes through a small set of paths that snapshot
  the cells, widths, names, charts, view state and sheet list they change,
  on any sheet, so a step can be reversed exactly; multi-cell operations
  are one step. A deleted sheet keeps its cells, so undo brings it back.
- **JEV.** The engine never touches the network. JEV functions describe a
  question and look up the answer in a pluggable source; `internal/jev`
  answers from a cache and queues new questions, and the UI sends them as
  background commands.

## The UI

`internal/ui` is one Bubble Tea model. Inside the grid it follows Google
Sheets; around it, the control panel keeps a 1-2-3 look. Every action is a
registered command with a title and description, reached from key bindings,
the menu bar, context menus, the command palette and the help overlay, so
they can't disagree. Menus, the palette and pickers are overlays composited
with Lip Gloss layers over the grid, which never shifts under them. Only the
visible cells are rendered. Styles come from a small set of theme roles
with dark and light variants built on the terminal's 16 ANSI colors, so the
user's palette applies.

See [UX.md](UX.md) for the rules every change follows.

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
