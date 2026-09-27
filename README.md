# 012

A spreadsheet for the terminal with a Lotus 1-2-3 look and Google Sheets
behavior, built on Bubble Tea v2.

```sh
make run            # build (pure Go, CGO_ENABLED=0) and start
./bin/012 budget.012
```

## Keys

Inside the grid, 012 works like Google Sheets.

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

## JEV functions

With a TypeSafe API key, four functions ask the hosted JEV model about a
value (a cell, a range or text). They follow Sheets' argument style: the
value, the question, then what the answers mean.

| Function | Returns | Like |
|---|---|---|
| `=JEV.TEST(A2, "Is this a complaint?", [yes means], [no means])` | TRUE or FALSE | an `IF` condition, a boolean mask |
| `=JEV.PROB(A2, "Is this a complaint?")` | probability of yes, as a percent | `predict_proba` |
| `=JEV.CLASSIFY(A2, "Sentiment", "negative, positive", [descriptions])` | the best label | `SWITCH`, `pd.cut` |
| `=JEV.SCORE(A2, "Urgency", "low, mid, high")` | a score from 0 to levels-1 | a rating scale |

Labels, descriptions and levels can also be ranges. Answers arrive in the
background (cells show `Loading…`), are cached by question, and the context
line shows the confidence for the selected cell. Data > Ask JEV again
re-asks the selection.

Set `TYPESAFE_API_KEY` in the environment or a `.env` file in the current
directory or next to the sheet; `TYPESAFE_BASE_URL` and
`TYPESAFE_DEFAULT_MODEL` are optional. Without a key the functions show
`#N/A` and say why. `JEV_LIVE_TEST=1 go test ./internal/jev -run TestLive`
checks the real service.

## Layout

```
cmd/012        entry point
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
