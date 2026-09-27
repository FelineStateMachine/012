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
| 5 | 012 goes where 1-2-3 couldn't | Multiple sheets with cross-sheet references; derived frequency and pivot sheets; recorded and Starlark macros; an SSH server mode; opt-in decimal arithmetic; VHS demo tapes in CI |

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
| Optional vim keymap (`hjkl`, counts, `:` commands) | sc-im, csvlens | `bubbles/v2/key` bindings | M |
| Frequency table of a column as a derived sheet | VisiData Shift+F | | M |

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

- Multiple sheets with `B:A1` style references (1-2-3 R3), sheet tabs. Engine gets a sheet dimension. L.
- Pivot tables as derived sheets. L.
- Macros: replay the command log; Starlark (`go.starlark.net`) for scripts, with step limits.
- Serve over SSH with `charm.land/wish/v2`.
- Decimal mode for currency (`cockroachdb/apd/v3`), opt-in. Done: File > Settings > Decimal arithmetic, boundary in the README.
- Demos in CI with VHS tapes; screenshots with freeze.

## Suggested order

1. Command log + undo, Copy/Move with ref adjustment, Range Format.
2. Command palette, autocomplete, status line stats.
3. CSV and XLSX import/export.
4. Charts (text first, then kitty placeholders).

## Later: sharing a live sheet (shelved)

Explored, not scheduled. One session host that runs a Bubble Tea program
for any byte stream with window-size events, fed by SSH
(`charm.land/wish/v2`), iroh tickets (the Go transport in
`FelineStateMachine/allons` `local/transport/iroh`, which needs cgo and a
prebuilt iroh-ffi archive, so it would sit behind a build tag), and the web
(`NimbleMarkets/go-booba` serves Bubble Tea over WebSocket/WebTransport with
ghostty-web; Bubble Tea v2 support unverified). Open questions: per-user
sessions on one sheet versus mirroring one session, and whether the web page
is served by the host or by a gateway dialing the iroh ticket.
