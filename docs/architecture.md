# Architecture

012 is a single pure-Go binary (`CGO_ENABLED=0`) built on Bubble Tea v2 and
Lip Gloss v2.

```
cmd/012          entry point: flags and config, 012 config, 012 serve, JEV setup, opening or importing a file
internal/config  the config file and the registry of options
internal/sheet   the engine: cells, evaluation, functions, recalculation, undo, files
internal/formula the formula language: references, lexer, parser, printer, rewriting
internal/numfmt  number formats, rounding, General, date serials
internal/fileio  import and export: CSV, TSV, XLSX, SQLite, Parquet, Lotus .wk1
internal/chart   chart layout, text rendering and kitty image encoding
internal/jev     the API key's resolution, answer cache and TypeSafe client
internal/keyring the OS credential store the API key lives in
internal/macro   macros: the Starlark scripting API, recorded actions as scripts, step limits
internal/telemetry  opt-in JSON log and OTLP export of spans, events and frame stats
internal/serve   the SSH server: auth, host key, a Model per session (charm.land/wish/v2)
internal/confine resolving typed file names, confined to a directory when served
internal/stress  synthetic worst-case sheets for the -tags stress benchmarks
internal/ui      the Bubble Tea model: modes, menus, overlays, rendering
  theme          style roles, color schemes and the widgets drawn with them (frames, key chips)
  rowtext        laying out a row of cell text across the columns on screen
  formula        reading the formula being typed (F4, the word and call at the caret)
e2e/             end-to-end tests through libghostty (separate module, cgo)
oracle/          differential tests against excelize's calculation (separate module)
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
  the engine noticing. Beside it, occupancy indexes (`occupancy.go`,
  bitmaps of 1024 rows per column) record which cells are stored and
  which have contents, so a range of the million-row grid is read at the
  cost of what it holds. Formats of whole columns and rows, and of the
  whole sheet, live on the lines (`lines.go`); a cell falls back on them.
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
  `lookup`: `cell` for one, `cells` for the cells a range holds, which
  the aggregates (SUM, AVERAGE, COUNT and the rest) use, sharing running
  aggregates within a recalculation (`rangememo.go`). The formulas whose
  ranges contain a changed cell are found through interval trees per
  column (`rangeindex.go`).
- **Functions.** One table (`FuncDef`) holds every function's name,
  signature, description and arity; it drives parsing, evaluation,
  autocomplete, in-app help and [functions.md](functions.md).
- **Undo.** Every mutation goes through a small set of paths that snapshot
  the cells, widths, names, charts, view state and sheet list they change,
  on any sheet, so a step can be reversed exactly; multi-cell operations
  are one step. A deleted sheet keeps its cells, so undo brings it back.
  The history keeps at most 100 steps and about 256 MB of before-images,
  counted as they're recorded, dropping the oldest steps first.
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
- **Depth limits.** `internal/formula`'s parser caps nesting at
  `formula.MaxDepth` (1024 levels), so a pathological formula fails to
  parse before anything evaluates it; evaluation puts off cells past
  65,536 levels of a chain rather than recursing further
  (`evaluate.go`).
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
command runs: telemetry times it there, and the macro recorder listens
there (see [Macros](#macros)). A command that writes cells declares the
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
| macros | `recorder`, `macroState` | a recording in progress (`macrorec.go`); a macro running, trust in the file's macros (`macrorun.go`); what scripts act on (`macrohost.go`, `macrohostnav.go`); Data > Macros and the manager (`macro.go`, `macromanage.go`) |
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
number of rows left out from the file. XLSX is read by 012's own
SpreadsheetML reader on `archive/zip` and `encoding/xml`: the workbook,
shared strings (kept end to end in one buffer) and styles first, then
each worksheet a token at a time, a row at a time, expanding shared
formulas only for the cells kept. The file is untrusted, so the reader
caps what it can be made to do (`xlsxLimits`: uncompressed bytes per
part and in all, compression ratio, zip entries, XML nesting and token
size, shared strings, styles, sheets, and the text shared formulas
expand to) and refuses unsafe part names and references past Excel's
edges; `FuzzReadXLSX` and `FuzzReadXLSXParts` fuzz it. XLSX is written
with the same packages, a worksheet part streamed per sheet. Exporters
write a `Snapshot` taken on the UI goroutine, in the background, through
one atomic write-then-rename; CSV and TSV are written a row at a time. One package suits formats that share
this much (the builder, number formats, serial dates and Excel formula
translation); a format that grew its own dependencies would move to a
subpackage behind the same table row.

## Macros

Macros are Starlark scripts (`go.starlark.net`, pure Go) kept in the
workbook (`sheet.Macro`, saved in the file). `internal/macro` knows
neither the engine nor the terminal: it defines the functions scripts call
over a `Host` interface, turns a recording (a list of `Action`s, the
command log) into a script, and runs scripts with a step limit, a cancel
switch and errors placed at their line and column. Scripts have no
`load`, and Starlark itself has no file, network, clock or random access,
so the Host is the only way out.

The UI implements the Host on the model (`scriptHost`), through the same
paths keys take: entries go through `Sheet.Set`, the selection through
the grid, commands through `runCommand`, with a question answered as if
typed. A run's script executes on its own goroutine, and each call it
makes comes back to the UI goroutine as a message (`macroCallMsg`), is
run on the model, and releases the script; the UI keeps taking calls
within one update for a few milliseconds, so a 1000-action replay costs
about 2 ms, and gives the screen back when the script pauses, so Esc can
stop a runaway. A script whose program is gone (it quit, or its SSH
session dropped) stops after waiting 30 s for a call to be taken. The whole run is one undo step opened with
`Workbook.Begin`.

Recording (`macrorec.go`) listens where actions happen: `runCommand` for
commands (with the answer to a prompt or choice bar), the entry commit,
pastes, the fill handle, column borders and tab drags. The selection is
recorded lazily, as absolute `select()` or relative `move()`/`extend()`,
just before something acts on it. Files record the computer their macros
were made or trusted on (an id from `cmd/012`); a macro from elsewhere
asks once before it runs. Only the local app lets scripts be edited in
the user's editor (`AllowEditor`), since that starts a program.

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

## Serving over SSH

`internal/serve` wraps `charm.land/ssh` (wish's server) with only a
session channel, public-key auth against authorized_keys and a shell
request allowed. Each session builds a `ui.Model` of its own and runs it
in its own `tea.Program` over the session (wish's emulated PTY), with
the client's environment and window size; `Model.Serve` gives it the
client's environment for terminal detection and a `confine.Root` for
file names. Everything a model knows lives in the model, so sessions
share nothing but read-only tables (the command and function
registries), the process-wide telemetry and, with JEV on, the HTTP
client; each gets its own `jev.Cache`, whose queue belongs to that
session's program. See [ssh.md](ssh.md).

**File names.** The UI keeps names as typed and turns them into paths
only to read or write, through the model's `confine.Root`: the zero
root (the local app) uses them as they are, a served session's root
keeps them inside its directory. Every open, save, import, download and
file listing goes through `Model.path` or the root.

## Configuration and secrets

`internal/config` reads one Ghostty-style file (`key = value`, `#`
comments, `config-file` includes) from `$XDG_CONFIG_HOME/012` or the OS
config directory; the same directory holds `themes/` and 012 serve's host
key. Every option is one entry in `config.Options`: name, type, default,
environment variables, flag, whether Reload config applies it, and a
description. Parsing and validation, the flags `cmd/012` accepts, `012
config`'s listing and default file, and [config.md](config.md)'s
reference (checked by a test) all come from that table. Values are layered defaults < file < environment < flags, each
remembering its source; problems are warnings, never fatal. The UI gets a
`ui.Settings` (the config, a reload function, the credential store, a JEV
connector) through `Model.Configure`, so tests hand it a temporary config
and a store in memory.

The JEV API key is never in the config. `jev.ResolveKey` takes it from
`TYPESAFE_API_KEY`, then the credential store, then
`jev-api-key-command` (split into words and run without a shell, with a
timeout, only when a sheet first asks JEV something). The base URL comes
from the config or environment only and must be https unless it's
loopback, so a file next to a sheet can't redirect the key.

`internal/keyring` is the credential store, chosen for the least
dependency weight that is still correct on each OS, with `CGO_ENABLED=0`:

| OS | Store | How |
|---|---|---|
| macOS | Keychain | `/usr/bin/security`. Writing uses `security -i`, which reads its command from stdin, with the key hex-encoded (`-X`), so it's never in any process's arguments. |
| Linux, BSD | Secret Service (GNOME Keyring, KWallet, KeePassXC) | libsecret's `secret-tool`, which reads the secret to store from stdin. |
| Windows | Credential Manager | `CredReadW`, `CredWriteW`, `CredDeleteW` from advapi32 through `golang.org/x/sys/windows`, already in the module graph. |

The alternatives were a keyring module (zalando/go-keyring, which does
the same on macOS but brings godbus for Linux, or 99designs/keyring,
which brings several backends and needs cgo for the macOS Keychain) or
cgo bindings to Security.framework and libsecret, which would end the
pure-Go build. Two
small command-line programs and one system DLL cover the three OSes with
no new module. The stores share a `Store` interface; tests use a
recording runner or `keyring.Memory`, and the e2e binary is built with
`-tags fakekeyring`, whose store is a file under `XDG_CONFIG_HOME`, so no
test reaches a real keychain.

## Themes

`theme.Theme` is a set of style roles, each defined on the 16 ANSI colors
and the default background and foreground (`theme.New`). The terminal
theme sends them as ANSI numbers, so the terminal's palette applies. A
color scheme (`theme.Palette`: VHS's embedded catalog, or a Ghostty,
kitty or VHS JSON file in `themes/`) is drawn by `theme.FromPalette`,
which maps each ANSI color to the scheme's, adds full-width bands for the
bars and a screen background, and corrects contrast (see
[themes.md](themes.md)). Bands are applied by `theme.Fill`, one pass over
a rendered line's escape sequences that sets the band's colors at the
start and after each reset and pads to the width; `View` fills the bar
lines before overlays are composited and the whole screen after, so bars
reach the edge under menus too. Charts take their colors from the scheme
when there is one, and from the terminal's palette otherwise.

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
