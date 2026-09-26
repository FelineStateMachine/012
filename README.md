# one23

A spreadsheet for the terminal with a Lotus 1-2-3 look and Google Sheets
behavior, built on Bubble Tea v2.

```sh
make run            # build (pure Go, CGO_ENABLED=0) and start
./bin/one23 budget.o23
```

## Keys

Inside the grid, one23 works like Google Sheets.

| Key | Action |
|---|---|
| Type | Replace the cell. `=` starts a formula, `'` forces text; `$1,200`, `12%`, `9/26/2026` and `14:30` are numbers that keep their format |
| Enter / Tab | Accept and move down / right (Enter returns to where a run of Tabs began) |
| Enter or F2, double-click | Edit the cell |
| Arrows while typing a formula | After an operator, pick a cell; Shift+arrows pick a range |
| Shift+arrows, drag, Shift+click | Select; the status line shows Sum, Avg and Count |
| Ctrl+arrows | Jump to the edge of the data (add Shift to select) |
| Click a header, Ctrl+Space, Shift+Space | Select whole columns or rows |
| Ctrl+A | Select the data, then everything |
| Del / Backspace | Clear the selection (formatting stays) |
| Ctrl+Shift+1 ... 6 | Number, time, date, currency, percent, scientific format |
| Ctrl+B, Ctrl+I, Ctrl+U, Alt+Shift+5 | Bold, italic, underline, strikethrough |
| Ctrl+Shift+L / E / R | Align left, center, right |
| Ctrl+\ | Clear formatting |
| Ctrl+S, Ctrl+O, Ctrl+Q | Save, open, quit |
| Ctrl+G or F5 | Go to a cell |
| F10 | Menu |
| F1 or Ctrl+/ | Keyboard shortcuts |

## Layout

```
cmd/one23        entry point
internal/sheet   engine: addresses, Pratt parser, evaluator, recalc, file format
internal/ui      Bubble Tea model: modes, menu, prompts, rendering
e2e/             end-to-end tests: real binary on a pty, rendered by libghostty-vt
```

## Tests

```sh
make test     # engine and UI unit tests
make fuzz     # fuzz the formula parser
make e2e      # builds libghostty-vt from source with Zig, then runs e2e tests
```

`make e2e` needs Zig 0.16+ and `pkg-config`. It is a separate Go module so the
cgo dependency never reaches the main binary.
