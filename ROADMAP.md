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
Tables past it are read in place as [linked sources](docs/files/sources.md).

### 2. A TUI Jupyter for nushell

Notebook tabs are a TUI Jupyter for nushell
([Notebooks](docs/nushell/notebooks.md)): code and note cells, outputs in
place, sent to sheets as live regions, and cells highlighted, checked and
completed by nu as they're written.

| Item | Result | Size |
|---|---|---|
| A command's signature and description for the word at the caret, on the context line, through `nu --ide-hover` or `nu --lsp` | Writing a cell without leaving for `help` | S |

### 3. Agents

Agents work on workbooks through the same operations people do (the
`Batch`/`Change` path), so their edits are undoable, attributed and
shown by `012 diff`, under the same trust rules as macros. Live mode is
`012 serve`'s shared editing with an agent as one participant
([Architecture](docs/contributing/architecture.md#shared-workbooks)).

| Step | Result | Size |
|---|---|---|
| Live mode: `012 --listen` and `012 mcp --attach`; the agent's cursor and name in the grid, its changes arriving as a suggestion (marked cells, accepted or rejected whole or by cell) unless direct edits are allowed, its own undo, a scope (sheet, range, read-only), elicitation to ask the person | Coworking with an agent in the grid | M to L |

## Later: other transports (shelved)

Explored, not scheduled: carrying the same shared sessions over iroh
tickets (the Go transport in `FelineStateMachine/allons`
`local/transport/iroh`, which needs cgo and a prebuilt iroh-ffi archive, so
it would sit behind a build tag) and the web (`NimbleMarkets/go-booba`
serves Bubble Tea over WebSocket/WebTransport with ghostty-web; Bubble Tea
v2 support unverified). They would reuse the rooms of `012 serve`, whose
server orders every participant's operations, rather than a design of
their own.

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
- Cycles found from the formulas as written, and arrays and notebook outputs in each other's way settled the same in any order, so a workbook reads the same when reopened: [Formulas](docs/formulas/README.md#values-and-errors)
- Formula results checked against Google Sheets once and asserted after, with the differences 012 keeps on purpose listed: [Testing](docs/contributing/testing.md#the-sheets-corpus)

**Finding and using features**

- Sheets-structured menus, a command palette and a key for every command: [Keys and mouse](docs/reference/keys.md)
- Formula suggestions, argument hints and pointing at cells and sheets: [Building formulas](docs/formulas/building.md)
- Tracing precedents and dependents, hidden sheets explained: [Tracing formulas](docs/formulas/tracing.md#stepping-through-them)
- Formula tracing: precedents and dependents marked in the grid as the pointer moves, named ranges, spills, regions and their sources included, listed to go to, and a formula evaluated step by step: [Tracing formulas](docs/formulas/tracing.md)
- An optional vim keymap with a `:` command line: [Keys and mouse](docs/reference/keys.md#vim-keys)
- Vim `.` repeat, registers, marks, `cc` and `s`, `:` line history and `:w!`: [Keys and mouse](docs/reference/keys.md#vim-keys)

**Data tools**

- Freeze, multi-column sort, filters with value pickers and conditions: [Freeze, sort and filter](docs/sheets/sort-filter.md#freeze)
- Find and replace, with regular expressions, across sheets: [Find and replace](docs/sheets/find-replace.md)
- Conditional formatting (single-color rules, color scales) and data validation (dropdowns, checkboxes, bounds), checked on pastes and fills and moving with cut and paste: [Conditional formatting and data validation](docs/sheets/rules.md#conditional-formatting)
- Data bars, icon sets, top values, averages, duplicates and date periods, dropdown chips and checkboxes of their own values, in XLSX both ways; rules moving to other sheets, and macros' pastes and fills checked: [Conditional formatting and data validation](docs/sheets/rules.md#conditional-formatting)
- Pivot tables and frequency tables, live, with subtotals and renamed values: [Pivot tables](docs/sheets/pivots.md)
- Named tables read by column name (`Sales[Amount]`, `Sales[@Amount]`), growing with their rows, in `.012` and XLSX both ways; notebook outputs are tables too: [Tables](docs/sheets/tables.md)
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
- Shared viewing over SSH: sessions opening one file share it live, each with its own cursor, the others' pointers and names shown, one writer handing writing over: [Serving over SSH](docs/terminal/ssh.md#sharing-a-workbook)
- Shared editing over SSH: everyone edits, the server orders every step, undo takes back only your own, changes by others marked, saves and notebook runs by the room: [Serving over SSH](docs/terminal/ssh.md#sharing-a-workbook)
- Shift+Enter, Ctrl+I and keys held to preview, with the kitty keyboard protocol: [Keys and mouse](docs/reference/keys.md#keys-the-terminal-has-to-tell-apart)
- Sixel chart images on terminals without kitty graphics, drawn after the frame and redrawn as the screen moves: [Charts](docs/sheets/charts.md)

**Nushell**

- 012 as a stage in a pipeline: `012 -` reads a table from standard input, `012 --pipe` sends the sheet or selection on, with nushell's types kept through NUON: [Pipelines](docs/nushell/pipelines.md)
- The `sheet` command: a nushell module shipped in the binary (`012 nu --install-module`), so nu calls 012 without `^012` or NUON on either side: [Pipelines](docs/nushell/pipelines.md#the-sheet-command)
- Notebook tabs, a TUI Jupyter for nushell: code and note cells with Jupyter's keys, outputs drawn in place and opened full-screen, `$name` between cells, stale marks and a reactive mode, outputs sent to sheets as live regions (`nu.name`), saved up to a cap, and earlier notebook sheets converted on open: [Notebooks](docs/nushell/notebooks.md)
- Cells run as streams: a pipeline that never ends (`tail -f`, `watch`) followed live, its rows reaching the output and its sheet as nu prints them: [Notebooks](docs/nushell/notebooks.md#streams)
- Code cells highlighted, checked and completed as they're written by nu itself (`--ide-ast`, `--ide-check`, `--ide-complete`), in the background, falling back to 012's own when nu is missing, old, slow or not trusted: [Notebooks](docs/nushell/notebooks.md#writing-a-cell)
- Table and record outputs drawn and worked as 012's own grid: formats by type, fitted and resizable columns, select, copy, sort, filter and find in place, full-screen, charts and pivots on the sheet the output is sent to: [Notebooks](docs/nushell/notebooks.md#outputs-as-grids)

**Files**

- Import CSV, TSV, JSON, NUON, XLSX, SQLite, Parquet and Lotus `.wk1`; export CSV, TSV, JSON, NUON, XLSX and SQLite; import locations; save-as and overwrite checks: [Files](docs/files/README.md)
- `.012` files read and written as a stream, cells straight into and out of the store: [The .012 format](docs/files/format.md#reading-and-writing)
- Commands for scripts without the screen: `012 get` (text, CSV, TSV, JSON or NUON), `012 set`, `012 recalc` and `012 export`, running notebooks and JEV only behind flags: [Scripts](docs/files/scripts.md)
- `012 diff` cell by cell, as git's diff command or textconv, and `012 merge-driver` merging cell by cell with conflicts noted on the cells: [Diff and merge in git](docs/files/git.md)
- Following files: a linked region follows a CSV, TSV, JSON lines or NUON file as it grows and any importable file as it's rewritten, keeping every row or the last ones, its rows arriving as the change stream: [Following files](docs/files/following.md)
- Linked sources: a Parquet file or a SQLite table or query read in place on a tab of its own, scrolled, sorted and filtered however many rows it has, formulas and pivot tables streaming over it: [Linked sources](docs/files/sources.md)
- The `.012` format written down with a version, and each release's workbooks kept as fixtures that every later build opens and saves unchanged: [The .012 format](docs/files/format.md#versions), [Releasing](docs/contributing/releasing.md#file-fixtures)
- Web pages in 012's look: a sheet, a range or a chart as one self-contained `.html` file, charts as SVG, from File > Download and `012 export`: [Files](docs/files/README.md#web-pages)
- A crash keeps unsaved work for recovery, restores the terminal and writes a report, locally and in `012 serve`: [Saving](docs/files/saving.md#if-012-crashes)

**Agents**

- `012 describe`, results as JSON with stable schemas, `012 set --dry-run` as a diff, and a Claude Code skill installed by `012 agent --install-skill`: [Agents](docs/agents/README.md)
- `012 mcp`, an MCP server on a workbook file: tools that read, evaluate, write, sort, filter, chart, pivot and run notebook cells through the same checks as `012 set`, resources and prompts: [MCP server](docs/agents/mcp.md)
- MCP Apps views: ranges read and charts made drawn in the chat from the HTML export, where the host supports the extension: [MCP server](docs/agents/mcp.md#views-in-the-chat)

**Upkeep**

- `make check` before every push; demo tapes (`make demos`); annotated version tags with release notes (v0.2.0 onward; v0.1.0 remains on the Go module proxy): [Testing](docs/contributing/testing.md)
- A release checklist (XLSX output opened in Excel, LibreOffice and Google Sheets) and `make dist`, release archives cross-compiled locally with SHA256SUMS: [Releasing](docs/contributing/releasing.md)
- Installing without Go: `curl -fsSL https://f58b.n.zip/install.sh | sh` (and `install.ps1` on Windows) with the release archives served from the docs site by `make site-release`: [Install and run](docs/getting-started/install.md), [Releasing](docs/contributing/releasing.md#publishing)
- `make stress-report` flags regressions against the last release's run, allowing for noise: [Observability](docs/contributing/observability.md#regressions-against-the-last-release)
- Grafana: recent traces and a trace view of the nested spans: [Observability](docs/contributing/observability.md#the-stack)
- Every overlay, prompts and formula suggestions behind narrow hosts, most in packages of their own with fake-host tests: [Architecture](docs/contributing/architecture.md#the-ui)
- Random sequences of edits, pastes, fills, sorts, inserts, formats and region runs checked for undo, reopening and full recalculation agreeing: [Testing](docs/contributing/testing.md#unit-tests)
- Frame time and recalculation held to a checked-in baseline in `make check`: [Bounds of support](docs/contributing/limits.md#the-speed-gate)
