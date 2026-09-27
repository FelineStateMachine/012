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
  theme          style roles and the widgets drawn with them (frames, key chips)
  rowtext        laying out a row of cell text across the columns on screen
  formula        reading the formula being typed (F4, the word and call at the caret)
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

`internal/ui` is a Bubble Tea program. Inside the grid it follows Google
Sheets; around it, the control panel keeps a 1-2-3 look. See
[UX.md](UX.md) for the rules every change follows.

**Commands.** Every action is a registered command with a title and
description (`commands.go`), reached from key bindings, the menu bar,
context menus, the command palette, the help overlay and mouse gestures
that stand for one (double-clicking a tab renames it through
`sheet.rename`), so they can't disagree. `runCommand` is the one place a
command runs: telemetry times it there, and a command log or macro
recorder would attach there.

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
| overlays | `overlay` | whatever has taken over input: menus (`menuoverlay.go`), the palette and pickers (`palette.go`, `names.go`), the filter picker, the find, sort and choice bars, the chart editor and selection, the shortcuts |
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

## Testing

See [testing.md](testing.md).
