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
| `ui.command` | an action: id, title, description, run, enabled, checked | key bindings, menu bar, context menus, palette, shortcuts help |
| `fileio` formats (`formats.go`) | a format: name, extensions, labels, traits, importer, exporter | `Import`, `Export`, detection, import picker, File > Download, command line |
| `chart.types`, with `sheet.ChartTypes` | a type: name and order (sheet, saved in files), a layout drawing text and image (chart) | `chart.Draw`, `chart.Image`, chart editor, Insert > Chart |
| theme roles | a role: dark and light styles on the 16 ANSI colors | every style in the UI |

Adding a function, command, format or chart type means adding an entry
(and its file), not editing switch statements elsewhere. When a list has to
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
mouse, palette). This keeps discoverability automatic, and gives one place
to add a command log for macros and replay later.

### 4. Side effects stay at the edges

`internal/sheet` does no I/O, network or terminal work. Anything outside the
process is a small interface the engine asks (`RemoteSource`) and the UI
fulfils asynchronously with `tea.Cmd`, answering from a cache. Tests
substitute fakes (`jev.Client`, the fake TypeSafe server, `demos/fakejev`).
New remote function families (other models, web lookups, databases) plug in
the same way.

## Size: narrow paths for hot data

### 5. One mutation path

Every change to a workbook goes through `Batch(Change{...}, fn)` and the
history `step`: snapshot what changes, apply, mark dirty, recalculate. Undo,
telemetry, recalculation and the file's dirty flag hang off that one path,
and so will anything later that needs to see every change (macros, a
command log, live sharing). New mutations use it; they never write cells
around it.

### 6. Storage behind a small API

Code reads and writes cells through a narrow API (get, set, iterate a
range, iterate a column), not the map underneath. That lets the store change
shape (compact column blocks, side tables for formulas and formats; see
[limits.md](limits.md)) without touching the rest of the engine.

### 7. Read ranges as ranges

Formulas read single cells through `lookup` and ranges through range
iteration, so aggregates (`SUM(A:A)`, running totals) can be served from
column blocks, prefix sums or cached aggregates instead of one lookup per
cell.

### 8. Work proportional to what changed or what's visible

Recalculation touches the cells an edit affects (dependency indexes by
column and by sheet), rendering touches only the visible cells, and status
statistics are cached against a version counter. A feature that scans the
whole sheet per keystroke or per frame needs a cache or an index.

### 9. Additive, versioned files

The `.012` format only gains optional fields, and the version rises only when
a file uses a feature older builds can't read (names, freeze and filters in
v3, several sheets in v4). Old files always load. Settings that older
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

- `sheet.Remote` is a package global; it belongs to the workbook (pattern 4).
- Cells are a bare `map[Addr]*Cell` used directly across the engine
  (pattern 6).
- Aggregates read ranges cell by cell through `lookup` (pattern 7).
- Undo history is capped by step count, not bytes (pattern 8).
- `Model` holds many components' fields directly (pattern 2).
