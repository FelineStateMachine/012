# Roadmap: past 1-2-3

Principles: inside the grid 012 behaves like Google Sheets; 1-2-3 is the
visual identity around it. The main binary stays pure Go (`CGO_ENABLED=0`), and every feature ships with unit
tests, a libghostty e2e test and reviewed golden screens. Visual and UX
quality keep pace with features: see [docs/UX.md](docs/UX.md).

## Goals

| Phase | Goal | Done when |
|---|---|---|
| 1 | The engine does what Sheets users reach for daily, and every change can be undone | Undo/redo (Ctrl+Z/Ctrl+Y) covers every mutation; copy, cut and paste (Ctrl+C/X/V, paste values) and insert/delete rows and columns adjust relative and `$absolute` references; number formats (Format menu, auto-detected `$` and `%` on entry) and named ranges work and persist; Sheets' everyday functions, dates and finance functions match excelize on a differential test suite |
| 2 | Finding and using features is faster than in any terminal spreadsheet | A Sheets-structured menu bar (File Edit View Insert Format Data Help) and a searchable command palette reach every command with its key shown; formulas autocomplete with signature hints; precedents and dependents are visible and jumpable; rows and columns freeze; find/replace and filter work; fill down (Ctrl+D) and series fill work; an optional vim keymap exists |
| 3 | 012 uses the terminal it's running in to the fullest, and degrades gracefully | Copy and paste ranges through the system clipboard; charts render as images in Ghostty/kitty and as text elsewhere; URLs are clickable; error cells are marked beyond color; cursor shape reflects the mode; the theme follows light and dark |
| 4 | Data moves in and out of 012 without friction or cgo | CSV/TSV, XLSX, SQLite, Parquet and Lotus .wk1 import, with export where it makes sense; large imports show progress; the binary still builds with `CGO_ENABLED=0` |
| 5 | 012 goes where 1-2-3 couldn't | Multiple sheets with cross-sheet references; derived frequency and pivot sheets; recorded and Starlark macros; an SSH server mode; opt-in decimal arithmetic; VHS demo tapes |

## Phase 1: engine foundations (unblocks everything else)

| Item | Why | Size |
|---|---|---|
| Command log: every mutation is a command with an inverse | Multi-level undo/redo, macro recording, later sync | M |
| /Copy and /Move with relative and `$absolute` ref adjustment | 1-2-3 parity; `$` is parsed today but ignored | M |
| Insert/Delete rows and columns with reference rewriting | Parity | M |
| /Range Format: Fixed, Currency, Percent, Comma, Date | Parity, and the biggest visual gap today | S |
| Range names (`@SUM(SALES)`) | Parity | S |
| @DATE, @NOW, @PMT, @NPV, @IRR, @VLOOKUP | Lotus serial dates (with the 1900 leap-day quirk) done in-house | M |
| Differential tests against `excelize` `CalcCellValue` | Test oracle: translate our formulas to Excel syntax, compare results | S |

## Phase 2: modern UX

| Item | Source of the idea | Building block | Size |
|---|---|---|---|
| Command palette (Ctrl+P), shows each command's key | VisiData, Quadratic, MS Edit | lipgloss v2 `Compositor` layers, `sahilm/fuzzy` | S-M |
| Formula autocomplete and signature hint while typing `@SU` | Excel, Sheets | third panel line, function table | M |
| Live SUM/AVG/COUNT of the pointed range in the status line | Excel | engine `aggregate` | S |
| Trace precedents/dependents: highlight, `[` and `]` to jump | Excel | dependency graph already exists | M |
| One-key frozen titles | csvlens, sc-im, 1-2-3 /Worksheet Titles | viewport split | S-M |
| Search (`n`/`N`) and non-destructive row filter | csvlens | | M |
| Fill down / fill series (1,2,3, Jan, Feb) | Excel, Sheets | needs Copy ref adjustment | S-M |
| Optional vim keymap (`hjkl`, counts, `:` commands). Done: File > Settings > Vim keys, [keys.md](docs/keys.md#vim-keys) | sc-im, csvlens | tables of command bindings, the command registry for `:` | M |
| Frequency table of a column as a derived sheet | VisiData Shift+F | a preset pivot table | M, done: Data > Frequency table, Alt+Shift+F |

## Phase 3: terminal-native features

| Item | API | Notes |
|---|---|---|
| Copy range as TSV to the system clipboard | `tea.SetClipboard` (OSC 52) | Works over SSH |
| Paste TSV/CSV into a range | `tea.PasteMsg` (bracketed paste, already on) | |
| Text charts in a popup (/Graph) | `ntcharts/v2` | Works everywhere |
| Real image charts | x/ansi `kitty` Unicode placeholders + `tea.Raw` | Ghostty/kitty; survives the diff renderer and tmux. Sixel fallback redrawn on resize |
| Clickable URLs in cells | lipgloss `Style.Hyperlink` (OSC 8) | |
| Red undercurl on ERR cells | `UnderlineStyle(UnderlineCurly).UnderlineColor` | |
| Bar cursor in EDIT, block elsewhere | `View.Cursor.Shape` | |
| Light/dark palette | `tea.RequestBackgroundColor` | |
| Progress bar and notification for long imports | `View.ProgressBar`, `ansi.Notify` | |
| Hold-to-preview, shift+enter | `View.KeyboardEnhancements` | kitty protocol terminals |

Bubble Tea v2 already turns on synchronized output (2026) and grapheme width
(2027) when the terminal supports them.

## Phase 4: data in and out (all pure Go)

| Format | Library |
|---|---|
| CSV/TSV | stdlib `encoding/csv` plus a small dialect sniffer |
| XLSX | `github.com/xuri/excelize/v2` |
| SQLite | `modernc.org/sqlite` |
| Parquet | `github.com/parquet-go/parquet-go` |
| Lotus .wk1 | in-house (no Go reader exists), from the LibreOffice and Gnumeric filters |

Avoid in the main binary: DuckDB and automerge (both need cgo).

## Phase 5: beyond

- Multiple sheets with references between them, in Sheets' style (`Sheet2!A1`) rather than 1-2-3 R3's `B:A1`, and sheet tabs. Done.
- Pivot tables as derived sheets. Done: Data > Pivot table and its editor, live results the engine owns, saved as their definition (file version 5), values in XLSX; see [docs/data.md](docs/data.md#pivot-tables).
- Macros: replay the command log; Starlark (`go.starlark.net`) for scripts, with step limits. Done: Data > Macros records (absolute or relative references), saves, runs (Ctrl+Alt+Shift+digit, the palette) and manages macros, stored as Starlark in the file; see [docs/macros.md](docs/macros.md).
- Serve over SSH with `charm.land/wish/v2`. Done: `012 serve [dir]`, a 012 per session confined to the directory, public-key auth only ([docs/ssh.md](docs/ssh.md)). Not shared editing: see below.
- Decimal mode for currency (`cockroachdb/apd/v3`), opt-in. Done: File > Settings > Decimal arithmetic, boundary in the README.
- Demos with VHS tapes. Done: `make demos` renders `demos/` locally and the README shows them. Not run in CI: the GitHub repo is hosting only.

## Next: vast data

The grid is Excel's, 1,048,576 x 16,384, since step A; memory is bounded
by the `max-cells` budget rather than the grid. Raise that budget in
measured steps (see [docs/limits.md](docs/limits.md)):

| Step | What | Result |
|---|---|---|
| A | Make every operation cost what the data costs, not the grid: clip whole-column and whole-row ranges to the used area, sparse range reads for aggregates, column and row formats instead of per-cell formatting, a range index sized to the used columns, three-letter columns (to XFD), wider row headers. Then raise the grid to 1,048,576 x 16,384 with a `max-cells` budget in the config file, and a stress test that fails when any command on an empty full-grid selection costs more than its data. **Done**: occupancy indexes beside the cell map, sparse and clipped range reads, running aggregates shared per recalculation and interval trees of range users (1000 full-column SUMs: 174 ms to 0.5 ms per edit; 8192 running totals: 0.73 s to 2.6 ms), line formats for columns, rows and the sheet, `A:C` and `2:5` references, `max-cells` for imports and pastes, and `TestCommandsCostTheDataNotTheGrid` | Excel-sized grids, memory still bounded |
| B | Compact column storage behind `cellStore`: typed value blocks, formulas and formats in side tables (about 20 to 40 B per cell instead of 300) | A budget of 10 M+ cells in a few hundred MB |
| C | A streaming or binary file format next to the readable JSON one | Open and save scale with the data |
| Later | Linked, paged read-only ranges over Parquet and SQLite that feed pivots and formulas by streaming | Sources too big for any grid |

## Ahead

Collected after phase 5 from the docs, the limits, and what each piece of
work left open. Sizes: S (a day or two), M (about a week), L (weeks).

### 1. Finish and harden what shipped

| Item | Why | Size |
|---|---|---|
| Open 012's XLSX output in a real spreadsheet app before releases. Done for Google Sheets: sheets, hidden sheets, named ranges, implicit intersection, column and cell formats, widths, frozen panes, filters, links and missing-sheet values all match; repeat with Excel and LibreOffice when available | The writer is otherwise checked by excelize and 012's reader | S |
| Column and row formats travel with copy, cut and move; formulas reading blanks of a newly formatted column re-infer at once. Done: pastes and moves carry what cells show, whole lines their line formats; see [data.md](docs/data.md#copy-paste-and-fill) | Left open by step A | S |
| Save as onto another existing file asks before replacing (local and `012 serve`). Done: Save as, `:w name` and `:wq name` ask on the context line ([files.md](docs/files.md)) | Replacing a file by mistake loses it | S |
| XLSX sheet names Excel can't take: formulas and named ranges that name a renamed sheet use the written name. Done ([files.md](docs/files.md)) | They'd otherwise go out as values | S |
| Replace current sheet keeps its charts; Insert new sheet(s) goes after the current tab. Done: charts of a whole table are re-pointed to the new one when it has as many series, and the note says which; see [files.md](docs/files.md) | Import location details | S |
| Precedent tracing explains when every precedent is on a hidden sheet. Done, for dependents too | A generic "no formula" hides why nothing shows | S |
| Settings > JEV API key checks the key with a test call. Done: one fixed question, "Key saved and checked" or why the check failed ([jev.md](docs/jev.md#setup)) | A mistyped key otherwise shows only as `#ERROR!` in cells | S |
| `012 serve`: open a file from the ssh command line, and save unsaved work to a recovery file on shutdown or idle timeout. Done: `ssh -t host file.012` (a one-word exec request taken as a file name inside the served directory, never run) and `.012-recovery/`, offered back on the next open ([ssh.md](docs/ssh.md#unsaved-work)) | Sessions reach a file in one step, and stopping the server or idling out keeps work | M |

| Vim `p`/`P` after `yy` carries the rows' line formats, as a whole-row paste does | Only cell formats go with it | S |
| Cutting a block clears the source cells to plain, as Sheets does | They keep showing their column or row format | S |
| Replace current sheet removes charts whose range no longer fits the new data, or asks | They stay, pointing at empty cells, with a note | S |

### 2. Spreadsheet features Sheets users reach for

| Item | Notes | Size |
|---|---|---|
| Dynamic arrays: FILTER, SORT, UNIQUE, SEQUENCE, spill ranges, `#SPILL!`; then LET and LAMBDA | The biggest gap in the function library; spills need engine support for ranges a formula owns | L |
| Text and regex: TEXTJOIN, SPLIT, REGEXMATCH, REGEXEXTRACT, REGEXREPLACE | Sheets staples | S |
| Conditional formatting (color scales, rules on values and formulas), drawn in theme roles | Visual; must read under all 349 schemes | M |
| Data validation: dropdown lists (a picker in the cell), number and date rules, checkboxes | Pairs with the filter picker | M |
| Wrap text, row heights, borders, merged cells | Layout changes in the grid renderer | M to L |
| Notes on cells (shown on hover and in the context line) | | S |
| Locale: decimal comma, date order, list separator in formulas | Sheets' File > Settings > Locale | M |
| Charts: scatter, area, stacked columns and bars, axis options, a legend position. Done: area and scatter (with trend lines) types, stacked and 100% stacked columns, bars and areas, value axis minimum, maximum and log scale, gridlines on and off, the legend at the bottom, right or none; see [charts.md](docs/charts.md) | Registry entries in `internal/chart` | M |
| Pivot tables: column subtotals, renaming value columns, check the "(blank)" label against Sheets | Left open by pivots | S |
| Protected ranges and sheets (warn on edit) | | S |

### 3. Scale (see "Next: vast data" above and docs/limits.md)

| Item | Result | Size |
|---|---|---|
| Step B: compact column storage behind `cellStore` | 10 M+ cells in a few hundred MB, cell reads 2 to 3 ns | L |
| Step C: streaming and compact `.012` format | Open and save scale with the data | M |
| Smaller undo steps: formatting changes as diffs | More history in the same memory | S |
| Linked, paged read-only ranges over Parquet and SQLite feeding pivots and formulas | Sources too big for any grid | L |

### 4. Macros and keys

| Item | Size |
|---|---|
| Record dialog choices (sort bar, filter picker, find and replace, chart editor) and chart drags; let scripts run commands that open dialogs, with answers | M |
| Vim: `.` repeat, registers, marks, `cc`/`s`, command-line history, `:w!`, `:wq file` quitting | M |
| Remaining overlays get narrow hosts: filter picker, sort and choice bars, chart editor, shortcuts, named ranges, cell entry and prompts | M |

### 5. Distribution and upkeep

| Item | Size |
|---|---|
| Version tags and release notes; prebuilt binaries attached to tags (built locally, since the repo is hosting only). Tags done from v0.1.0, with notes in the tag message | S |
| `make check`: vet, lint, tests, oracle, e2e in one target, run before every push. Done | S |
| `make stress-report` thresholds that flag regressions over a set percentage against the last release | S |
| Grafana: a trace panel for the nested spans | S |
| Accessibility: a high-contrast theme, and every state readable without color (already a UX rule; audit it) | S |

## Later: sharing a live sheet (shelved)

Explored, not scheduled. `012 serve` gives each SSH session its own
spreadsheet; saving over a file another session saved asks first, and
that is all the sessions know of each other. One session host that runs a Bubble Tea program
for any byte stream with window-size events, fed by SSH
(`charm.land/wish/v2`), iroh tickets (the Go transport in
`FelineStateMachine/allons` `local/transport/iroh`, which needs cgo and a
prebuilt iroh-ffi archive, so it would sit behind a build tag), and the web
(`NimbleMarkets/go-booba` serves Bubble Tea over WebSocket/WebTransport with
ghostty-web; Bubble Tea v2 support unverified). Open questions: per-user
sessions on one sheet versus mirroring one session, and whether the web page
is served by the host or by a gateway dialing the iroh ticket.
