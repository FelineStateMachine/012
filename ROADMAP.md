# Roadmap

Principles: Google Sheets is the seed for behavior inside the grid and
1-2-3 for the visual identity around it; both are inspiration, not specs
to match in every detail. The main binary stays pure Go
(`CGO_ENABLED=0`), and every feature ships with unit tests, a libghostty
e2e test and reviewed golden screens. Visual and UX quality keep pace
with features: see [UX and visual bar](docs/contributing/ux.md).

This page is what's ahead. What has shipped is listed once, at the end,
with the doc that describes it; when an item ships, it moves there as
one line.

## Ahead

Sizes: S (a day or two), M (about a week), L (weeks).

### 1. Scale

The grid is Excel's, 1,048,576 x 16,384, and memory is bounded by the
`max-cells` budget (ten million cells) rather than the grid. The budget
rises in measured steps; see [Bounds of support](docs/contributing/limits.md#what-would-raise-the-bounds).

| Item | Result | Size |
|---|---|---|
| Linked, paged read-only ranges over Parquet and SQLite that feed pivots and formulas by streaming | Sources too big for any grid | L |

### 2. A TUI Jupyter for nushell

A notebook is its own kind of tab in a workbook: cells, not a grid. Code
cells hold a nushell pipeline (several lines if needed) with its output
under it (a scrollable table view, text, a record or an error); note cells
hold Markdown. You run cells in the order you choose, with the usual
controls, and send any output to a sheet, where it is a live region that
formulas, charts and pivots use. It replaces the grid-based notebook sheet.

| Item | Result | Size |
|---|---|---|
| Notebook tabs: code and note cells, outputs rendered in place (tables that scroll and open full-screen as a grid, text, records, errors), Jupyter keys inside the notebook (Shift+Enter run and next, Ctrl+Enter run, a/b add above/below, dd delete, m/y note/code, Esc/Enter command/edit mode) plus menus and palette; run cell, run all, run above, run below, stop, clear outputs; run counts and timings; cells named for their output (`$files`), stale marks when a cell they read has changed, an opt-in reactive mode that re-runs dependents; send an output to a sheet as a live region (`nu.files`); outputs saved in the workbook up to a size cap; existing notebook sheets converted, one code cell per region; `012 nu` opens a notebook | A TUI Jupyter with nushell and 012 | L |
| Code cells highlighted, completed and checked as you type by nushell itself: `nu --ide-ast` token shapes mapped to theme roles, `nu --ide-complete` plus 012's cell, region and sheet names on Tab, `nu --ide-check` errors underlined; debounced in the background, plain text when nu is missing or slow; hover docs through `nu --lsp` later | Writing pipelines in 012 feels like nushell's own prompt | S to M |

### 3. Toward multiplayer

Sessions share one workbook through the server that already hosts them
(`012 serve`), so every change is ordered in one place: the workbook's one
mutation path (`Batch`/`Change`) becomes a stream of operations, the same
log macros record. Each step is useful on its own.

| Step | Result | Size |
|---|---|---|
| A nushell region follows a streaming pipeline through the linked regions' live sources (`live.Source`), rows arriving as the pipeline writes them | Pipelines as live sheets | S |
| Shared viewing over SSH: several `012 serve` sessions open the same workbook; one edits, the others follow live with their own cursor, scroll and theme; presence shows who is where | Watch-along and review, one writer | M |
| Shared editing over SSH: every session edits, the server orders operations (no CRDT needed while one server holds the workbook), per-user undo, presence and edit ownership shown in the grid, saves by the server | Multiplayer 012 over SSH | L |

### 4. Toward 1.0

Solid before shared: what 012 already does keeps working from release to
release, and shared editing builds on these guarantees. These come before
shared editing in section 3.

| Item | Result | Size |
|---|---|---|
| Files saved by each release kept as fixtures that every later build opens and saves unchanged; the `.012` format written down with a version | Old files keep opening | S |
| Random sequences of edits, sorts, fills and region runs checked for invariants: undo all restores the start, save and reopen match, a full recalc agrees with the incremental one | Bugs between features found before users hit them | M |
| A workbook of formulas where 012 matches Sheets on purpose (dates, text, rounding, errors, spills), results checked once in Sheets and asserted after; differences chosen on purpose listed in the docs | Formula results people rely on stay put | S |
| Frame time and recalculation checked in `make check` against a baseline with a noise margin | Slowdowns fail the check, not a later benchmark | S |
| A panic in any session saves the workbook for recovery and writes a short report, tested end to end | A crash loses nothing | S |
| A week of real use by the owner, problems triaged into this page | Rough edges found by use | S |

### 5. Around the grid

| Item | Result | Size |
|---|---|---|
| Commands for scripts without the screen: `012 get`, `012 set`, `012 recalc`, `012 export` on a workbook file | 012 in scripts, cron and nushell pipelines | S to M |
| `012 diff` cell by cell (values, formulas, formats, regions) and a git diff and merge driver for `.012` | Sheets kept in git review like code | M |
| Formula tracing: precedents and dependents shown in the grid, a formula evaluated step by step | Finding why a number is wrong | M |
| Named tables with structured references (`Sales[Amount]`); notebook regions are tables | Formulas that read by column name | M |
| A sheet or chart exported as a static HTML page in 012's look | Sharing a sheet with someone without 012 | S |
| Release archives and an install script served from the owner's nzip server | Installing without Go | S |

## Later: other transports (shelved)

Explored, not scheduled: carrying the same shared sessions over iroh
tickets (the Go transport in `FelineStateMachine/allons`
`local/transport/iroh`, which needs cgo and a prebuilt iroh-ffi archive, so
it would sit behind a build tag) and the web (`NimbleMarkets/go-booba`
serves Bubble Tea over WebSocket/WebTransport with ghostty-web; Bubble Tea
v2 support unverified). They would reuse the server-ordered operation
stream above rather than a design of their own.

## Shipped

**Engine**

- Undo and redo for every change, across sheets: [Editing](docs/sheets/editing.md#undo)
- Copy, cut, paste and fill with relative and `$absolute` references; insert and delete rows and columns: [Editing](docs/sheets/editing.md#copy-paste-and-fill)
- Entries detected as typed (currency, percent, dates, times) and number formats: [Formulas](docs/formulas/README.md#what-you-type)
- Wrapped and clipped text, row heights, borders and merged cells, in `.012` files and XLSX both ways: [Formatting](docs/sheets/formatting.md#wrapping)
- Vertical alignment, border colors, outlines along the sheet's edges, merged cells entered as wide as they are and centered by line: [Formatting](docs/sheets/formatting.md#vertical-alignment)
- Named ranges; several sheets with references between them, hidden sheets: [References](docs/formulas/references.md)
- Sheets' everyday, math, text, lookup, date and finance functions, checked against excelize: [Functions](docs/reference/functions.md), [Testing](docs/contributing/testing.md#the-excelize-oracle)
- Dynamic arrays that spill (FILTER, SORT, UNIQUE, SEQUENCE and more, ARRAYFORMULA), LET and LAMBDA, SPLIT and the REGEX functions: [Arrays and spills](docs/formulas/arrays.md)
- Opt-in decimal arithmetic for money: [Decimal arithmetic](docs/formulas/decimal.md)
- A locale per file, as Sheets' File > Settings > Locale: decimal commas, date order, currency and `;` in formulas, typed and shown while files store en-US's form: [Locale](docs/sheets/locale.md)
- Month and day names in the locale's language, Excel downloads in its Currency and Date formats, filters and parse errors in its rendering: [Locale](docs/sheets/locale.md#what-follows-the-locale)
- An Excel-sized grid in compact column storage, with a ten-million-cell `max-cells` budget; operations cost the data, not the grid: [Bounds of support](docs/contributing/limits.md#sheet-size)
- Undo steps in the compact form: clearing a full ten-million-cell sheet holds about what the sheet does, and a step past 1 GB asks first: [Bounds of support](docs/contributing/limits.md#undo)
- Spilled cells and pivot results in the compact form, about 20 B each: [Bounds of support](docs/contributing/limits.md#sheet-size)

**Finding and using features**

- Sheets-structured menus, a command palette and a key for every command: [Keys and mouse](docs/reference/keys.md)
- Formula suggestions, argument hints and pointing at cells and sheets: [Building formulas](docs/formulas/building.md)
- Tracing precedents and dependents, hidden sheets explained: [Building formulas](docs/formulas/building.md)
- An optional vim keymap with a `:` command line: [Keys and mouse](docs/reference/keys.md#vim-keys)
- Vim `.` repeat, registers, marks, `cc` and `s`, `:` line history and `:w!`: [Keys and mouse](docs/reference/keys.md#vim-keys)

**Data tools**

- Freeze, multi-column sort, filters with value pickers and conditions: [Freeze, sort and filter](docs/sheets/sort-filter.md#freeze)
- Find and replace, with regular expressions, across sheets: [Find and replace](docs/sheets/find-replace.md)
- Conditional formatting (single-color rules, color scales) and data validation (dropdowns, checkboxes, bounds), checked on pastes and fills and moving with cut and paste: [Conditional formatting and data validation](docs/sheets/rules.md#conditional-formatting)
- Data bars, icon sets, top values, averages, duplicates and date periods, dropdown chips and checkboxes of their own values, in XLSX both ways; rules moving to other sheets, and macros' pastes and fills checked: [Conditional formatting and data validation](docs/sheets/rules.md#conditional-formatting)
- Pivot tables and frequency tables, live, with subtotals and renamed values: [Pivot tables](docs/sheets/pivots.md)
- Notes on cells: [Notes and protection](docs/sheets/notes-protection.md#notes)
- Protected sheets and ranges that warn on edit: [Notes and protection](docs/sheets/notes-protection.md#protected-sheets-and-ranges)
- Charts (column, bar, line, area, pie, scatter; stacking, trend lines, axis and legend options), as images or text: [Charts](docs/sheets/charts.md)
- Macros, recorded or written in Starlark: [Macros](docs/sheets/macros.md)
- Macros record the choices made in dialogs and chart drags, and scripts answer dialogs: [Macro scripting API](docs/reference/macro-api.md#dialogs)
- JEV functions, with the API key in the OS keychain: [JEV functions](docs/formulas/jev.md)

**The terminal**

- System clipboard, hyperlinks, curly error underlines, cursor shapes, progress and notifications, light and dark: [The terminal](docs/terminal/README.md#terminal-features-012-uses)
- Color schemes and a config file: [Themes](docs/terminal/themes.md), [Configuration](docs/reference/config.md)
- A high-contrast theme at WCAG AAA, and every state readable without color: [Themes](docs/terminal/themes.md#high-contrast), [UX](docs/contributing/ux.md#reading-without-color)
- `012 serve` over SSH, with files on the ssh command line and recovery of unsaved work: [Serving over SSH](docs/terminal/ssh.md)
- Shift+Enter, Ctrl+I and keys held to preview, with the kitty keyboard protocol: [Keys and mouse](docs/reference/keys.md#keys-the-terminal-has-to-tell-apart)
- Sixel chart images on terminals without kitty graphics, drawn after the frame and redrawn as the screen moves: [Charts](docs/sheets/charts.md)

**Nushell**

- 012 as a stage in a pipeline: `012 -` reads a table from standard input, `012 --pipe` sends the sheet or selection on, with nushell's types kept through NUON: [Pipelines](docs/nushell/pipelines.md)
- The `sheet` command: a nushell module shipped in the binary (`012 nu --install-module`), so nu calls 012 without `^012` or NUON on either side: [Pipelines](docs/nushell/pipelines.md#the-sheet-command)
- `012 nu` and notebook sheets: nushell pipelines at a prompt on the formula bar become live, named regions that read each other and refresh in dependency order, saved as commands and never run on open: [Notebooks](docs/nushell/notebooks.md)

**Files**

- Import CSV, TSV, JSON, NUON, XLSX, SQLite, Parquet and Lotus `.wk1`; export CSV, TSV, JSON, NUON, XLSX and SQLite; import locations; save-as and overwrite checks: [Files](docs/files/README.md)
- `.012` files read and written as a stream, cells straight into and out of the store: [The .012 format](docs/files/format.md#reading-and-writing)
- Following files: a linked region follows a CSV, TSV, JSON lines or NUON file as it grows and any importable file as it's rewritten, keeping every row or the last ones, its rows arriving as the change stream: [Following files](docs/files/following.md)

**Upkeep**

- `make check` before every push; demo tapes (`make demos`); annotated version tags with release notes (v0.2.0 onward; v0.1.0 remains on the Go module proxy): [Testing](docs/contributing/testing.md)
- A release checklist (XLSX output opened in Excel, LibreOffice and Google Sheets) and `make dist`, release archives cross-compiled locally with SHA256SUMS: [Releasing](docs/contributing/releasing.md)
- `make stress-report` flags regressions against the last release's run, allowing for noise: [Observability](docs/contributing/observability.md#regressions-against-the-last-release)
- Grafana: recent traces and a trace view of the nested spans: [Observability](docs/contributing/observability.md#the-stack)
- Every overlay, prompts and formula suggestions behind narrow hosts, most in packages of their own with fake-host tests: [Architecture](docs/contributing/architecture.md#the-ui)
