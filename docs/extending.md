# Extending 012

012 grows along two axes: **breadth** (more functions, commands, formats,
charts, overlays, remote sources) and **size** (more cells, sheets and
formulas). Each axis has a few patterns. New work follows them; code that
doesn't yet is listed under [Gaps](#gaps).

## Breadth: one entry, every surface

### 1. Registries are the single source

A capability is one entry in a table, in one file, and every surface that
shows it is derived from the table rather than listing it again.

| Registry | Entry | Derived from it |
|---|---|---|
| `sheet.FuncDef` | a function: name, signature, description, arity, eval | parsing, autocomplete, argument hints, help, [functions.md](functions.md) |
| `ui.command` | an action: id, title, description, run, enabled, checked, what it edits, and how macros treat it | key bindings (Sheets and vim), menu bar, context menus, palette, shortcuts help, the `:` command line and its completions, macro recording and `run()` in scripts |
| `fileio` formats (`formats.go`) | a format: name, extensions, labels, traits, importer, exporter | `Import`, `Export`, detection, import picker, File > Download, command line |
| `chart.types`, with `sheet.ChartTypes` | a type: name and order (sheet, saved in files), a layout drawing text and image (chart) | `chart.Draw`, `chart.Image`, chart editor, Insert > Chart |
| theme roles (`theme.Theme`) | a role: dark and light styles on the 16 ANSI colors, with a contrast minimum for schemes (`minContrast`) | every style in the UI, drawn in the terminal's palette or any color scheme (`FromPalette`); `TestEveryThemeReadable` checks each role under all 349 schemes |
| `config.Options` | an option: name, type, default, environment variables, flag, live or not, description, check | parsing and warnings, flags, `012 config` and its default file, [config.md](config.md), Reload config |

Adding a function, command, format, chart type, option or role means
adding an entry (and its file), not editing switch statements elsewhere.
A new process-wide setting is a `config.Options` entry read with
`Config.String`, `Bool`, `Int` or `Duration`; one that belongs to a
workbook goes in the file instead (pattern 9). A new role goes in
`theme.Theme` with its ANSI colors in `New`; schemes pick it up. When a list has to
be kept in sync by hand, turn it into a registry.

### 2. Components own their state

UI features that take over input (menus, palette, find bar, pickers, chart
editor, sheet tabs, import flow) are components: a type with its own state,
key and mouse handling, layout and status line, owned by `Model` and driven
through the `overlay` interface or its siblings. `Model` routes messages and
composes the screen; it doesn't hold a component's fields. Overlays are
composited over the finished grid, so a new one never shifts the layout.

### 3. Actions go through commands

Every user action is a registered command, whatever reaches it (key, menu,
mouse, palette). This keeps discoverability automatic, and `runCommand`,
the one place commands run, is where the macro recorder listens: a command
that changes the workbook is recorded by id (`run("format.bold")`), with
the answer to the question it asks. Each command says how macros treat it
(`command.macro`): recorded (the default), a view command recorded as the
selection it leaves (Go to, Select all, Next sheet), or never recorded nor
run from scripts (files, menus, help, undo, macros).

The rest of the command log is what isn't a command but matters to a
replay: entries as they're accepted, pastes of text, the fill handle,
column borders and dragged tabs, each recorded where it happens with
`Model.record`, and the selection, recorded as a state (where it is just
before something acts on it) rather than as the keys or clicks that moved
it. So movement keys, typing and mouse selection needn't become commands
to be recorded, and a replay doesn't depend on the window's size. A new
action that changes the workbook outside a command records itself the same
way; anything unrecorded that changes the workbook while recording becomes
a comment in the script, so a gap shows.

### 4. Side effects stay at the edges

`internal/sheet` does no I/O, network or terminal work. Anything outside the
process is a small interface the engine asks (`RemoteSource`, set per
workbook with `SetRemote`) and the UI fulfils asynchronously with
`tea.Cmd`, answering from a cache. Tests substitute fakes (`jev.Client`,
the fake TypeSafe server, `demos/fakejev`).
New remote function families (other models, web lookups, databases) plug in
the same way.

## Size: narrow paths for hot data

### 5. One mutation path

Every change to a workbook goes through `Batch(Change{...}, fn)` and the
history `step`: snapshot what changes, apply, mark dirty, recalculate. Undo,
telemetry, recalculation and the file's dirty flag hang off that one path,
and so will anything later that needs to see every change (live sharing).
New mutations use it; they never write cells around it. A change made over
several calls, as a macro run makes one call per message, opens its step
with `Workbook.Begin` and closes it when done, so it undoes as one; inside
it, `Settle` recalculates what changed so far, so what reads formulas
(scripts, sorting) sees current values. The macro list itself is changed
through the same steps, so saving a macro is undoable and marks the file
modified.

### 6. Storage behind a small API

Code reads and writes cells through a narrow API (`cellStore` in
`internal/sheet/store.go`: get, set, delete, count, iterate everything or
a range, a column being a range), not the map underneath. That lets the
store change shape (compact column blocks, side tables for formulas and
formats; see [limits.md](limits.md)) without touching the rest of the
engine. Beside the map, occupancy indexes (`occupancy.go`) say which rows
of each column hold a cell, and which hold contents, so a range yields its
cells in row order at the cost of what it holds, and the used range, data
edges and the next filled cell are found without scanning.

### 7. Read ranges as ranges

Formulas read single cells through `lookup.cell` and ranges through
`lookup.cells`, which yields only the cells a range holds, so aggregates
(`SUM(A:A)`, running totals) cost their data, and within a recalculation
SUM-like functions share running aggregates per range (`rangememo.go`).
Functions that need positions (`MATCH`, `SUMIF`, `INDEX`) take a `matrix`,
which knows the range's full size and which cells hold something; they
walk those and account for the blanks between at once.

### 8. Work proportional to what changed or what's visible

Recalculation touches the cells an edit affects (interval trees of range
users by column, dependency indexes by sheet), rendering touches only the
visible cells, and status statistics are cached against a version
counter. A feature that scans the whole sheet per keystroke or per frame
needs a cache or an index.

The grid is a million rows by 16,384 columns, so no loop may walk the
addresses of a selection, a range or a line: walk the cells the store
holds (`cellsIn`, `inRange`, the occupancy indexes), and keep formats of
whole columns and rows on the line (`lines.go`). `TestCommandsCostTheDataNotTheGrid`
(`internal/ui/vast_test.go`) holds every registered command to this: it
runs each with the whole sheet, a whole column and a whole row selected
and fails past 250 ms or 8 MB, so a new command that walks its selection
fails it.

### 9. Additive, versioned files

The `.012` format only gains optional fields, and the version rises only when
a file uses a feature older builds can't read (names, freeze and filters in
v3, several sheets in v4, pivot tables in v5). Old files always load. Settings that older
builds can safely ignore (decimal arithmetic) don't raise the version.

## Keeping it honest

### 10. Measure at the seams

Every seam above has a telemetry span and a `-tags stress` benchmark
(`make stress`, `make stress-report`), and [limits.md](limits.md) records
the bounds. A change to a seam reports before and after numbers.

### 11. Shape limits

`make lint`: no function over cognitive complexity 25, no Go file over 500
lines. A package that keeps growing past a few thousand lines gets split
along a boundary where dependencies point one way.

### 12. Every feature at three levels

Unit tests against the engine or model, an e2e test through libghostty, and
a golden screen reviewed in the gallery for anything visible (see
[testing.md](testing.md)).

## Gaps

Where the code doesn't follow the patterns yet:

- Components in `internal/ui` are handed the whole `*Model` rather than a
  narrower interface, so they stay in package `ui` (pattern 2).
- Movement keys, typing, F4 in formulas, Alt+letter menus and direct mouse
  manipulation act without a registered command (pattern 3). The macro
  recorder covers what matters for replay without them (entries, the
  selection, pastes, the fill handle, column borders, dragged tabs), but
  dragging and resizing charts and the choices made in dialogs (the sort
  bar, the filter picker, find and replace, the chart editor) aren't
  recorded: a recording notes them as comments.
