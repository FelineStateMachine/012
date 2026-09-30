---
title: "Testing"
sidebar_position: 5
---

# Testing

```sh
make check    # all of the below that must pass before a push: gofmt, vet, lint, test, speed, oracle, e2e
make lint     # go vet, staticcheck, cognitive complexity at most 25, Go files at most 500 lines, doclint and doccheck (see Docs below)
make test     # engine, file formats and UI unit tests
make fuzz     # fuzz the formula parser, random edits, and the .012, CSV, .wk1, XLSX and NUON readers
make speed    # frames and recalculation against a checked-in baseline (see limits.md)
make oracle   # compare formulas and number formats with excelize
make e2e      # run the real binary in a terminal emulator (needs Zig and pkg-config)
make screens  # rewrite the golden screens and build the review gallery
make stress   # benchmarks on synthetic and real data (see limits.md)
make site     # build the docs site, failing on broken links (needs Node; see site.md)
```

## Unit tests

The engine is tested directly: parsing, every function, recalculation,
undo, reference rewriting, file round trips. `FuzzRead` holds the
streaming `.012` reader to encoding/json decoding the whole file: both
refuse a file or both read it into workbooks that save the same bytes.
`TestFixturesOpenAndSaveUnchanged` opens the workbooks every release
saved and saves them again, byte for byte
([File fixtures](releasing.md#file-fixtures)).

`TestRandomEdits` (`internal/sheet/randedit_test.go`) checks what holds
between features: from each of 3,000 seeds it builds a workbook
of two sheets and a notebook tab, makes up to 40 edits drawn from the
seed (entries, pastes, fills, series, sorts, inserted and deleted rows
and columns, formats, borders, merges, moves, notes, rules, names,
sheets, frozen panes, filters, tables, linked sources and their order,
notebook cells, and outputs sent to sheets, frozen, deleted and sent
rows, with tables standing in for
nushell's), and checks that undoing every step gives back the file and
the values it started with, redoing gives back the end, saving and
reopening gives the same file and values (a recalculation from
scratch), and recalculating everything in place changes no value.
Outputs' rows come from outside the undo history, so the test sends
them again after each edit, undo and reopening, as the UI does. A
failure names the fewest of the seed's edits that still fail. It takes
a few seconds; `-randedit.seeds=20000` runs more, and
`FuzzRandomEdits` (in `make fuzz`) lets the fuzzer choose the edits.

`TestRandomSharedEdits` (`randedit_shared_test.go`) makes the same
edits as three authors taking turns in one workbook with its history
shared, as a room of `012 serve` holds it, then has them undo their own
steps in turns drawn from the seed, out of order wherever the rule
allows: someone can always undo, an undo redone at once gives back the
file it undid, and once every step is undone the workbook is back where
it started.

The UI is tested by sending
Bubble Tea messages (keys, mouse, paste, window size) to the model and
reading what `View` renders, without a terminal. Sessions sharing a
workbook are driven the same way through `ui.Shared`, each room's
changes handed to the others as their programs would get them
(`share_test.go`), and in real Bubble Tea programs typing at once under
the race detector, with the time an edit takes to reach another
session's frame (`sharelive_test.go`).

The MCP server is tested through the SDK's client over an in-memory
transport. `TestViewInBrowser` (`internal/mcp`) draws its
[view](../agents/mcp.md#views-in-the-chat) in Chromium with Playwright
(`internal/mcp/testdata/view.mjs`), once with `window.openai` set as the
OpenAI Apps SDK sets it and once behind a page speaking MCP Apps'
messages, in both themes, and checks the grid, the pointer and formula
bar, scrolling under sticky headers and the frame's height. It needs
Node and the `playwright` package with its browser, and is skipped
without them:

```sh
npm install --prefix ~/.cache/o12-playwright playwright && npx --prefix ~/.cache/o12-playwright playwright install chromium-headless-shell
PLAYWRIGHT_DIR=~/.cache/o12-playwright go test ./internal/mcp -run TestViewInBrowser
VIEW_SHOTS=/tmp/shots PLAYWRIGHT_DIR=~/.cache/o12-playwright go test ./internal/mcp -run TestViewInBrowser   # and screenshots
```

## End to end, through libghostty

`e2e/` builds the real `012` binary, runs it on a pseudo-terminal and feeds
its output into [libghostty-vt](https://github.com/ghostty-org/ghostty),
Ghostty's terminal emulator core. Keystrokes and mouse events are encoded
by libghostty itself from the modes the app turned on, so they are the bytes
Ghostty would send. Tests assert on what a user would see: screen text,
cursor position, window title, hyperlinks, which screen is active after
quitting. A fake TypeSafe server answers JEV questions, and the harness
clears `TYPESAFE_API_KEY` so tests never reach the real service. Every
session points `XDG_CONFIG_HOME` into its working directory (with an
optional config file), so its config, theme files and the machine id
that macros' trust uses are the test's own: sessions that share a
directory are one computer. The binary is built with `-tags
fakekeyring`, whose credential store is a file there, so tests never
read your config or touch your keychain. Unit tests do the same with
`ui.Settings` and `keyring.Memory`. The binary is also built with `-tags
crashtest`, where F12 panics in the update, the view or a command, as
the session's environment says (`internal/ui/crashtest.go`), so `e2e/crash_test.go` can check that a crash
restores the terminal and keeps the work; release builds have no such key.

`e2e/ssh_test.go` runs `012 serve` and reaches it with the system's
`ssh` client inside the same libghostty terminal, so the server path
is tested as a user sees it (skipped when `ssh` isn't installed). `e2e/share_test.go` has two people
reach one `012 serve` that way, each in a terminal of their own, and
share a file; screens of a shared workbook start the others' sessions
first (`screen.peers`).

`make e2e` builds libghostty-vt from source with Zig into `.deps/` on first
use. It is its own Go module so cgo never reaches the main binary.

## Golden screens

`e2e/screens_test.go` records key UI states as HTML drawn from libghostty's
cell grid (colors as palette variables, so one golden serves every theme),
with light-terminal variants. The states are listed in
`e2e/screenlist_test.go`, with the fixtures they share in
`e2e/screensetup_test.go`. `make screens` rewrites them and builds
`e2e/testdata/screens/gallery.html` with dark and light reference palettes.
Every visual change is reviewed there before it's committed. It also
draws the docs' stills, the screens `docScreens` lists, into
`docs/media` ([Pictures of 012](site.md#pictures-of-012)); `make e2e`
fails on one not drawn from its golden as it is.

## The excelize oracle

`oracle/` translates formulas to Excel syntax, computes them with
[excelize](https://github.com/xuri/excelize), and compares results and
formatted values with 012's. Known differences between Sheets and Excel,
and places where excelize departs from Excel, are listed as skips with a
reason.

## The Sheets corpus

012 follows Google Sheets' formula results where people rely on them
([UX](ux.md)), and `internal/fileio/testdata/sheets_corpus.tsv` holds
about 300 formulas that pin it down: dates and times, text, rounding and
floating point, errors and how they propagate, coercion of text,
booleans and blanks, lookups and criteria, array sizes, LET and LAMBDA,
and `TEXT` formats. Each line is a formula, 012's result, Sheets'
result and a note, separated by tabs. Results read `"text"` quoted,
`12.5`, `TRUE`, `#N/A`, or `(blank)`. Formulas read a small fixture on a
sheet named `Data`, and each computes one value: an array is measured
with `ROWS`, `COLUMNS`, `INDEX` or `TEXTJOIN`, so its size is checked
and nothing spills.

`TestSheetsCorpus` holds 012 to the results recorded for it, and fails
on a row where Sheets' result differs unless its note starts
`on purpose:` and says why; that reason also goes in the docs page for
the feature. Sheets' result reads `unconfirmed` until it is checked in
Sheets:

```sh
# Add formulas as lines of their own, then record 012's results.
go test ./internal/fileio -run SheetsCorpus -update-corpus
# Write the corpus as a workbook with 012's XLSX exporter.
go test ./internal/fileio -run SheetsCorpus -corpus-xlsx=$HOME/012-sheets-corpus.xlsx
# In Sheets (locale United States): File > Import the workbook, then
# File > Download > CSV of the Corpus sheet. Record Sheets' results:
go test ./internal/fileio -run SheetsCorpus -update-from="$HOME/Downloads/012-sheets-corpus - Corpus.csv"
```

In the workbook, column C holds each formula, which Sheets recalculates
on import; D and E read its result as text and its kind with formulas
both 012 and Sheets compute, F holds 012's result and G says whether
they agree (for two errors, compare C and F by eye). The exporter saves
formulas Excel has nothing like (`SPLIT`, `REGEXMATCH`, `SORTN`,
`FLATTEN`, `ARRAYFORMULA` inside a formula) as values, so those rows
leave C empty and say so in the note: type the formula from B into C
before downloading, or `-update-from` leaves them unconfirmed. It also
leaves a row unconfirmed where Sheets says `#NAME?` and 012 doesn't: the
import didn't recognize a function, as Sheets doesn't read `FILTER` and
`SORT` written with the `_xlfn._xlws.` prefix Excel gives them. A number
Sheets downloads with a trailing point (`110.` for `=100*1.1`) is read
without it.

## The XLSX differential test

`TestXLSXDifferential` (`internal/fileio/xlsx_diff_test.go`) imports a
corpus of workbooks with 012's own XLSX reader and with an importer
built on excelize (`xlsx_excelize_test.go`), and compares
every sheet as saved, the widths, names, sheet shown, rows and notes.
The corpus is workbooks written by the test (hand-written SpreadsheetML,
excelize and 012's exporter), the stress datasets' XLSX files when
fetched (`STRESS_DIR`), and any directories in `XLSX_CORPUS`, such as
excelize's own `test/` files in the module cache. Where excelize reads a
file wrongly (formulas spread across merged cells, spaces dropped from
shared formulas) the comparison leaves the difference out, with the
reason next to it.

## Live checks

`JEV_LIVE_TEST=1 go test ./internal/jev -run TestLive` asks the real JEV
service one question of each kind, using `TYPESAFE_API_KEY`.

## Demo recordings

`demos/` holds [VHS](https://github.com/charmbracelet/vhs) tapes for the
recordings and stills the README and the docs show. `make demos` renders
all of them (or `make demos DEMOS=jev` for one) into `demos/out/`: full
GIFs, PNG stills of key moments in `demos/out/stills/`, and smaller GIFs
in `demos/out/media/`. It needs vhs 0.12+, ttyd and ffmpeg. The JEV tape
talks to `demos/fakejev`, a local stand-in the target starts on
127.0.0.1, never the real service. On macOS, `demos/lib/ttyd` wraps ttyd
so Alt+letter reaches the app. VHS has no mouse commands, so the tapes
use the keyboard, and they reach commands through the palette (Ctrl+K)
rather than counting menu items, so a new menu item doesn't break them.
Tapes that change settings set them for the run (`O12_KEYMAP=vim`)
rather than writing the config file the other tapes share. VHS's ttyd
has sixel on, so 012 draws charts as sixel images, but VHS records only
xterm.js's text layer: the recordings show the text chart each image
covers ([Charts](../sheets/charts.md)).
The nushell tapes need `nu`: `demos/lib/nu.tape` starts a session
without your config files, with a fixed prompt and 012 on the PATH, and
their data is in `demos/data`, so each recording comes out the same.

A change to what a tape shows means rendering it again, reading its
stills, and copying what the docs show into `docs/media/`: GIFs from
`demos/out/media/`, stills from `demos/out/stills/`. Only files a doc
shows go there, and GIFs stay under 1 MB.

## Docs

The docs are a tree of folders by concept under `docs/`, Markdown that
reads on GitHub and builds with Docusaurus as it is:

- each folder has a `README.md` that links its pages and subfolders (the
  folder's page in Docusaurus, and what GitHub shows for the folder), and
  a `_category_.json` with its sidebar `label`, `position` and a link to
  that README;
- each page starts with front matter: a `title` and a `sidebar_position`
  unique in its folder;
- links are relative, to `.md` files, so they work in both; images live
  in `docs/media/`.

A new page goes in the folder a reader would look in, with front matter,
and a line in the folder's README. `make site` builds them into the
[docs site](site.md) and fails on any link or anchor it can't resolve.

`make lint` runs `scripts/doclint`, which flags wording that narrates
history (see [CLAUDE.md](../../CLAUDE.md)) and code blocks that draw
012's screen ([Pictures of 012](site.md#pictures-of-012)), and `scripts/doccheck`, which
checks the tree above, that relative links and anchors resolve, that
every `docs/...md` path Go code names exists, that every file in
`docs/media` is shown by a doc and made by a tape or drawn from a golden
screen, that every tape and still is shown by a doc, and that a page the site has published
isn't gone without a redirect ([The docs site](site.md#old-addresses-keep-working)).
Unit tests check what the docs say about
the code: menu paths lead to menu items, command ids and keys exist and
every bound key is in [Keys and mouse](../reference/keys.md)
(`internal/ui/docs_test.go`), settings and variables are options
(`internal/config/docs_test.go`), and [the .012 format](../files/format.md)
names every field (`internal/sheet/file_doc_test.go`).
[Functions](../reference/functions.md) and the reference in
[Configuration](../reference/config.md) are generated, and their tests
fail when they're stale.

## No broken windows

`make check` must pass before a push, and warnings are fixed or turned
off with a written reason rather than left standing. The rules are in
[CLAUDE.md](../../CLAUDE.md).
