# one23

A Lotus 1-2-3 style spreadsheet for the terminal, built on Bubble Tea v2.

```sh
make run            # build (pure Go, CGO_ENABLED=0) and start
./bin/one23 budget.o23
```

## Keys

| Key | Action |
|---|---|
| Arrows, PgUp/PgDn, Tab/Shift+Tab, Home | Move the cell pointer |
| Type text | Label (`'` left, `"` right, `^` center) |
| Type `0-9 + - . ( @ # $ =` | Value or formula, e.g. `+A1*2`, `@SUM(A1..A5)` |
| Arrow after an operator | POINT mode: arrow to a cell, `.` anchors a range |
| F2 | Edit the current cell |
| F5 | Go to an address |
| Del | Erase the current cell |
| `/` or `<` | Menu: Worksheet, Range, File, Quit (type first letters, e.g. `/fs`) |
| F1 | Help |

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
