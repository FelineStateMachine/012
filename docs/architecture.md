# Architecture

012 is a single pure-Go binary (`CGO_ENABLED=0`) built on Bubble Tea v2 and
Lip Gloss v2.

```
cmd/012          entry point: flags, JEV setup, opening or importing a file
internal/sheet   the engine: cells, parsing, evaluation, recalculation, undo, files
internal/fileio  import and export: CSV, TSV, XLSX, SQLite, Parquet, Lotus .wk1
internal/chart   chart layout, text rendering and kitty image encoding
internal/jev     JEV configuration, answer cache and TypeSafe client
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

## Testing

See [testing.md](testing.md).
