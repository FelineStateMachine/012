# Roadmap: past 1-2-3

Principles: keyboard first, 1-2-3 muscle memory stays the default, the main
binary stays pure Go (`CGO_ENABLED=0`), and every feature ships with unit
tests plus a libghostty e2e test.

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
- Decimal mode for currency (`cockroachdb/apd/v3`), opt-in.
- Demos in CI with VHS tapes; screenshots with freeze.

## Suggested order

1. Command log + undo, Copy/Move with ref adjustment, Range Format.
2. Command palette, autocomplete, status line stats.
3. CSV and XLSX import/export.
4. Charts (text first, then kitty placeholders).
