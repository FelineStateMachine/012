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
internal/ui      the Bubble Tea model: modes, menus, overlays, rendering
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

## Testing

See [testing.md](testing.md).
