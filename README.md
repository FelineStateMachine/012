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
| Ctrl+Z, Ctrl+Y or Ctrl+Shift+Z | Undo, redo |
| Ctrl+C, Ctrl+X, Ctrl+V | Copy, cut, paste; references adjust as in Sheets, and copies also go to the system clipboard |
| Ctrl+Shift+V | Paste values only |
| Paste from the terminal | Tab-separated or multi-line text fills a block of cells |
| Ctrl+D, Ctrl+R | Fill down, fill right |
| Ctrl+Alt+= / Ctrl+Alt+- | Insert rows above / delete the selected rows (columns when whole columns are selected) |
| Ctrl+Enter while typing | Enter the same entry in every selected cell |
| F4 while typing a formula | Cycle the reference at the caret through A1, $A$1, A$1, $A1 |
| Ctrl+Shift+1 ... 6 | Number, time, date, currency, percent, scientific format |
| Ctrl+B, Ctrl+I, Ctrl+U, Alt+Shift+5 | Bold, italic, underline, strikethrough |
| Ctrl+Shift+L / E / R | Align left, center, right |
| Ctrl+\ | Clear formatting |
| Ctrl+S, Ctrl+O, Ctrl+Q | Save, open, quit |
| Ctrl+G or F5 | Go to a cell |
| Alt+F, Alt+E, Alt+V, Alt+I, Alt+O, Alt+H, F10, click a title | Open a menu (arrows move, Enter runs, Esc closes) |
| Ctrl+K, Alt+/, Ctrl+Shift+P | Search the menus: find and run any command |
| Right-click, Shift+F10 | Cell, column or row menu |
| F1 or Ctrl+/ | Keyboard shortcuts |

The top three lines are the menu bar and mode indicator, the formula bar
(name box, then the cell's contents or the entry being typed) and the
context line (prompts, key hints, formula errors).

## Layout

```
cmd/one23        entry point
internal/sheet   engine: addresses, Pratt parser, evaluator, recalc, file format
internal/ui      Bubble Tea model: modes, menu, prompts, rendering
e2e/             end-to-end tests: real binary on a pty, rendered by libghostty-vt
oracle/          differential tests of formulas and formats against excelize
```

## Tests

```sh
make test     # engine and UI unit tests
make fuzz     # fuzz the formula parser
make e2e      # builds libghostty-vt from source with Zig, then runs e2e tests
make oracle   # compare formulas and number formats with excelize
```

`make e2e` needs Zig 0.16+ and `pkg-config`. It is a separate Go module so the
cgo dependency never reaches the main binary; so is `oracle`, which keeps
excelize out of it. `oracle/oracle_test.go` lists the formulas skipped
because Sheets and Excel disagree or excelize departs from Excel.
