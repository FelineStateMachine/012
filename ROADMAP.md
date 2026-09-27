# Roadmap

Principles: inside the grid 012 behaves like Google Sheets; 1-2-3 is the
visual identity around it. The main binary stays pure Go
(`CGO_ENABLED=0`), and every feature ships with unit tests, a libghostty
e2e test and reviewed golden screens. Visual and UX quality keep pace
with features: see [UX and visual bar](docs/contributing/ux.md).

This page is what's ahead. What has shipped is listed once, at the end,
with the doc that describes it; when an item ships, it moves there as
one line.

## Ahead

Sizes: S (a day or two), M (about a week), L (weeks).

### 1. Harden what shipped

| Item | Why | Size |
|---|---|---|
| Open 012's XLSX output in Excel and LibreOffice before releases, as was done for Google Sheets | The writer is otherwise checked by excelize and 012's reader | S |
| Conditional formatting and validation, what's left: data bars and icon sets, "top 10", "duplicate values", date periods (this week, last month), rules moving with cells cut to another sheet, a dropdown's chip look, and custom checkbox values | Excel's other rule types come in as notes | M |
| Remaining overlays get narrow hosts: filter picker, sort and choice bars, chart editor, shortcuts, named ranges, cell entry and prompts | Components testable without the model ([Architecture](docs/contributing/architecture.md#the-ui)) | M |

### 2. Spreadsheet features Sheets users reach for

| Item | Notes | Size |
|---|---|---|
| Wrap text, row heights, borders, merged cells | Layout changes in the grid renderer | M to L |
| Locale: decimal comma, date order, list separator in formulas | Sheets' File > Settings > Locale | M |

### 3. Scale

The grid is Excel's, 1,048,576 x 16,384, and memory is bounded by the
`max-cells` budget (ten million cells) rather than the grid. The budget
rises in measured steps; see [Bounds of support](docs/contributing/limits.md#what-would-raise-the-bounds).

| Item | Result | Size |
|---|---|---|
| A streaming or binary `.012` format next to the readable JSON one | Open and save scale with the data | M |
| Smaller undo steps: plain cells' before-images as slots, formatting changes as diffs | More history in the same memory; clearing a full sheet costs what the sheet does | S to M |
| Linked, paged read-only ranges over Parquet and SQLite that feed pivots and formulas by streaming | Sources too big for any grid | L |

### 4. Macros, keys and the terminal

| Item | Size |
|---|---|
| Record dialog choices (sort bar, filter picker, find and replace, chart editor) and chart drags; let scripts run commands that open dialogs, with answers | M |
| Vim: `.` repeat, registers, marks, `cc`/`s`, command-line history, `:w!` | M |
| Hold-to-preview and Shift+Enter on terminals with the kitty keyboard protocol (`View.KeyboardEnhancements`) | S |
| Sixel chart images, redrawn on resize, for terminals without kitty graphics | M |

## Later: sharing a live sheet (shelved)

Explored, not scheduled. `012 serve` gives each SSH session its own
spreadsheet; saving over a file another session saved asks first, and
that is all the sessions know of each other. One session host that runs
a Bubble Tea program for any byte stream with window-size events, fed by
SSH (`charm.land/wish/v2`), iroh tickets (the Go transport in
`FelineStateMachine/allons` `local/transport/iroh`, which needs cgo and a
prebuilt iroh-ffi archive, so it would sit behind a build tag), and the
web (`NimbleMarkets/go-booba` serves Bubble Tea over
WebSocket/WebTransport with ghostty-web; Bubble Tea v2 support
unverified). Open questions: per-user sessions on one sheet versus
mirroring one session, and whether the web page is served by the host or
by a gateway dialing the iroh ticket.

## Shipped

**Engine**

- Undo and redo for every change, across sheets: [Editing](docs/sheets/editing.md#undo)
- Copy, cut, paste and fill with relative and `$absolute` references; insert and delete rows and columns: [Editing](docs/sheets/editing.md#copy-paste-and-fill)
- Entries detected as typed (currency, percent, dates, times) and number formats: [Formulas](docs/formulas/README.md#what-you-type)
- Named ranges; several sheets with references between them, hidden sheets: [References](docs/formulas/references.md)
- Sheets' everyday, math, text, lookup, date and finance functions, checked against excelize: [Functions](docs/reference/functions.md), [Testing](docs/contributing/testing.md#the-excelize-oracle)
- Dynamic arrays that spill (FILTER, SORT, UNIQUE, SEQUENCE and more, ARRAYFORMULA), LET and LAMBDA, SPLIT and the REGEX functions: [Arrays and spills](docs/formulas/arrays.md)
- Opt-in decimal arithmetic for money: [Decimal arithmetic](docs/formulas/decimal.md)
- An Excel-sized grid with a `max-cells` budget; operations cost the data, not the grid: [Bounds of support](docs/contributing/limits.md#sheet-size)
- Compact column storage: 20 B a number, a ten-million-cell `max-cells` budget, 3 ns cell reads: [Bounds of support](docs/contributing/limits.md#sheet-size)

**Finding and using features**

- Sheets-structured menus, a command palette and a key for every command: [Keys and mouse](docs/reference/keys.md)
- Formula suggestions, argument hints and pointing at cells and sheets: [Building formulas](docs/formulas/building.md)
- Tracing precedents and dependents, hidden sheets explained: [Building formulas](docs/formulas/building.md)
- An optional vim keymap with a `:` command line: [Keys and mouse](docs/reference/keys.md#vim-keys)

**Data tools**

- Freeze, multi-column sort, filters with value pickers and conditions: [Freeze, sort and filter](docs/sheets/sort-filter.md#freeze)
- Find and replace, with regular expressions, across sheets: [Find and replace](docs/sheets/find-replace.md)
- Conditional formatting (single-color rules, color scales) and data validation (dropdowns, checkboxes, bounds), checked on pastes and fills and moving with cut and paste: [Conditional formatting and data validation](docs/sheets/rules.md#conditional-formatting)
- Pivot tables and frequency tables, live, with subtotals and renamed values: [Pivot tables](docs/sheets/pivots.md)
- Notes on cells: [Notes and protection](docs/sheets/notes-protection.md#notes)
- Protected sheets and ranges that warn on edit: [Notes and protection](docs/sheets/notes-protection.md#protected-sheets-and-ranges)
- Charts (column, bar, line, area, pie, scatter; stacking, trend lines, axis and legend options), as images or text: [Charts](docs/sheets/charts.md)
- Macros, recorded or written in Starlark: [Macros](docs/sheets/macros.md)
- JEV functions, with the API key in the OS keychain: [JEV functions](docs/formulas/jev.md)

**The terminal**

- System clipboard, hyperlinks, curly error underlines, cursor shapes, progress and notifications, light and dark: [The terminal](docs/terminal/README.md#terminal-features-012-uses)
- Color schemes and a config file: [Themes](docs/terminal/themes.md), [Configuration](docs/reference/config.md)
- A high-contrast theme at WCAG AAA, and every state readable without color: [Themes](docs/terminal/themes.md#high-contrast), [UX](docs/contributing/ux.md#reading-without-color)
- `012 serve` over SSH, with files on the ssh command line and recovery of unsaved work: [Serving over SSH](docs/terminal/ssh.md)

**Files**

- Import CSV, TSV, XLSX, SQLite, Parquet and Lotus `.wk1`; export CSV, TSV, XLSX and SQLite; import locations; save-as and overwrite checks: [Files](docs/files/README.md)

**Upkeep**

- `make check` before every push; demo tapes (`make demos`); annotated version tags with release notes (v0.2.0 onward; v0.1.0 remains on the Go module proxy): [Testing](docs/contributing/testing.md)
- A release checklist and `make dist`, release archives cross-compiled locally with SHA256SUMS: [Releasing](docs/contributing/releasing.md)
- `make stress-report` flags regressions against the last release's run, allowing for noise: [Observability](docs/contributing/observability.md#regressions-against-the-last-release)
- Grafana: recent traces and a trace view of the nested spans: [Observability](docs/contributing/observability.md#the-stack)
