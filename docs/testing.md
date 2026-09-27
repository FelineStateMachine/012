# Testing

```sh
make test     # engine, file formats and UI unit tests
make fuzz     # fuzz the formula parser and the CSV and .wk1 readers
make oracle   # compare formulas and number formats with excelize
make e2e      # run the real binary in a terminal emulator (needs Zig and pkg-config)
make screens  # rewrite the golden screens and build the review gallery
make stress   # benchmarks on synthetic and real data (see limits.md)
```

## Unit tests

The engine is tested directly: parsing, every function, recalculation,
undo, reference rewriting, file round trips. The UI is tested by sending
Bubble Tea messages (keys, mouse, paste, window size) to the model and
reading what `View` renders, without a terminal.

## End to end, through libghostty

`e2e/` builds the real `012` binary, runs it on a pseudo-terminal and feeds
its output into [libghostty-vt](https://github.com/ghostty-org/ghostty),
Ghostty's terminal emulator core. Keystrokes and mouse events are encoded
by libghostty itself from the modes the app turned on, so they are the bytes
Ghostty would send. Tests assert on what a user would see: screen text,
cursor position, window title, hyperlinks, which screen is active after
quitting. A fake TypeSafe server answers JEV questions, and the harness
clears `TYPESAFE_API_KEY` so tests never reach the real service.

`make e2e` builds libghostty-vt from source with Zig into `.deps/` on first
use. It is its own Go module so cgo never reaches the main binary.

## Golden screens

`e2e/screens_test.go` records key UI states as HTML drawn from libghostty's
cell grid (colors as palette variables, so one golden serves every theme),
with light-terminal variants. `make screens` rewrites them and builds
`e2e/testdata/screens/gallery.html` with dark and light reference palettes.
Every visual change is reviewed there before it's committed.

## The excelize oracle

`oracle/` translates formulas to Excel syntax, computes them with
[excelize](https://github.com/xuri/excelize), and compares results and
formatted values with 012's. Known differences between Sheets and Excel,
and places where excelize departs from Excel, are listed as skips with a
reason.

## Live checks

`JEV_LIVE_TEST=1 go test ./internal/jev -run TestLive` asks the real JEV
service one question of each kind, using `TYPESAFE_API_KEY`.
