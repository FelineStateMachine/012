---
title: "Nushell and pipelines"
sidebar_position: 4
---

# Nushell and pipelines

012 can be a stage in a pipeline: a table comes in on standard input,
you look at it or edit it in the terminal, and with `--pipe` the result
goes out on standard output for the next command. With
[nushell](https://www.nushell.sh), tables travel as NUON, nushell's own
notation, so file sizes, durations and dates arrive as themselves and
leave as themselves.

```nu
ls | to nuon | 012 --pipe | from nuon | where size > 1kb
```

The screen is the terminal's own (`/dev/tty`, or the console on
Windows), not standard input or output, so the pipeline's data never
mixes with what 012 draws, and nothing but the table reaches standard
output.

The other way round, a [notebook](#notebooks) runs nushell from inside
012: each pipeline's table becomes a live part of the sheet.

## Notebooks

![Running ls and a pipeline reading it at the nu❯ prompt, then refreshing the first and watching the second follow](../media/notebook.gif)

`012 nu` opens a notebook: a sheet where you type nushell pipelines and
each one's table lands in the grid, named, typed and kept live. Written
like a REPL, kept like a spreadsheet.

```sh
012 nu              # a new notebook
012 nu work.012     # open one, or make it
```

In any workbook, `!` (or **Data > Shell**, or the palette) opens the
same prompt, writing into the workbook's notebook sheet, or a new one
named Shell 1 at the first command. Commands run `nu` as a separate
process, so nushell must be installed; without it, the prompt says so
and the rest of 012 works as before.

### The prompt

The formula bar becomes `nu❯`, and the mode indicator says `NU`:

| Key | Does |
|---|---|
| Enter | Run the line; the prompt stays open for the next |
| Tab, Shift+Tab | Complete the word at the caret: a region (`$r1`) or one of nu's commands, from the box under the context line; again for the next |
| Up, Down | The workbook's earlier lines, those starting with what's typed; they're saved with the file |
| Esc | Stop the command running; otherwise, back to the grid |

A name and `=` before the pipeline name the region; without one it's
`r1`, `r2` and so on:

```nu
sizes = ls | select name size
```

Typing a name that exists gives that region a new command and runs it
again, as does F2 on its label.

### Regions

Each command's table is a region: a label line with its name and
command, dimmed and underlined as wide as the table, then the table,
its first row the column names. Regions stack down the sheet in the
order they ran, a row between them; when a table grows or shrinks, the
rows below it move, as if you'd inserted or deleted rows.

A region is still cells of the grid. Its values are
[typed](#types) (file sizes in the Size format, durations, dates),
formulas read them, and you can format them, filter them
(**Data > Create a filter**), sort them (the sort is kept, so the table
stays sorted when it runs again) and chart them. Like an
[array's spill](../formulas/arrays.md#spilled-cells), they can't be
typed over: change the command, or **Data > Shell regions > Freeze
region** turns the table into plain values and stops running it. Copies
paste values; downloads and XLSX hold the values; undo takes back a run
(the table it replaced comes back) or the region itself.

The context line says what's under the pointer: on a label, the keys
that refresh and edit it, and why it failed if it did; on its table,
which region the cell is part of. A label's end says what it's doing:
`Running…`, `waiting` for a region it reads, `failed`, `stopped`,
`not run`, or `no rows`.

### Tables between regions

`$r1` in a command is region r1's table, handed to nu as NUON in a file
nu reads, never as text spliced into the command. `$in` is the range
selected when the prompt was opened, on a sheet that isn't the
notebook, read again each time the region runs:

```nu
big = $r1 | where size > 1kb | sort-by size --reverse
$in | group-by region | transpose region rows
```

A region reading another depends on it. Refreshing a region (Enter on
its label, **Data > Refresh region**) runs it again and then every region
that reads it, directly or through others, each after what it reads.
F9 (**Data > Shell regions > Run all regions**) runs them all. A command
reading itself, directly or through others, is refused.

Other sheets read a region by name with `nu.` in front, as a named range
of its table, header included: `=SUM(nu.big)`, `=VLOOKUP("go.mod",
nu.r1, 3, FALSE)`. Formulas follow the table as it grows, shrinks or
moves, and show `#REF!` while it has none.

### Running

Commands run in the background, one at a time: the screen stays live,
and Esc stops the one running and those waiting after it. Each runs as
`nu --no-config-file -c` (with your own config files if `nu-config` is
on), stops after `nu-timeout` (30 seconds unless set), and keeps at
most `max-cells` cells, whole rows, saying how many it left out. When a
command fails, its label says so and the context line gives nushell's
message.

### Saving and trust

The file keeps each region's command, name, place and what it reads,
not its table ([format](../files/format.md#regions)). Opening a notebook
never runs anything: its regions say `not run`, and F9 runs them all,
each after what it reads.

Like [macros](../sheets/macros.md#macros-from-other-computers), a
file's commands run as you, with your files, so a notebook saved on
another computer asks once before they run:

```
Run this file's shell commands?   Enter Run   Esc Cancel
```

What you type at the prompt runs without asking. The `shell` option
decides: `ask` (the default) as above, `on` never asks, and `off` runs
nothing. [012 serve](ssh.md#notebooks) runs no commands unless the
server's `serve-shell` is on. See [Configuration](../reference/config.md#nushell-notebooks)
for the options.

## Reading standard input

`012 -` reads a table from standard input into a new sheet named
`stdin`, and tells its format from the text:

| Text starts with | Read as |
|---|---|
| `[` or `{`, and is all JSON | JSON: a list of records, a record, or records one after another (NDJSON) |
| `[` or `{` otherwise | NUON: a table (`[[name, size]; [a, 1kb]]`), a list of records, a record |
| anything else | delimited text: TSV when tabs separate the fields, otherwise CSV, as [imports](../files/README.md#import-and-download) read them |

The first row holds the column names; a record that brings a new key
adds a column. The sheet has no file and counts as unsaved, so quitting
asks first, as with any unsaved sheet. `max-cells` applies as it does to
imports: whole rows while they fit, and the context line says how many
were left out. While a long input is still coming in, the status line
counts its rows and Esc stops reading.

## Sending a table on

`012 --pipe` reads standard input as `012 -` does (or a file named on
the command line, `012 --pipe sales.csv`), and quitting sends a table to
standard output:

- The status line says what Ctrl+Q sends and in which format
  (`Ctrl+Q  send the sheet as NUON`, or `send B2:D9 as NUON` while a
  range is selected).
- Quit (Ctrl+Q, File > Quit, `:q`) asks: Enter sends the selection when
  one is made, or else the sheet; S sends the whole sheet instead; D
  quits without sending; Esc goes back to the sheet.
- File > Quit and send selection and File > Quit and send sheet send
  without asking. They're in the menus and the palette only with
  `--pipe`.

The sheet is its used range from A1. A selection is sent as selected,
and its first row names the columns. Either way, rows a filter hides
stay behind, as Sheets copies a filtered range. Quitting without sending writes nothing and exits with status
1, so the pipeline stops rather than carrying on with nothing.

The table goes out in the format it came in (NUON, JSON, CSV or TSV);
from a file of another kind, such as `.xlsx` or `.012`, as NUON.
`--to nuon`, `--to json`, `--to csv` or `--to tsv` picks the format
instead:

```nu
open budget.xlsx | to nuon | 012 --pipe --to csv | save budget.csv
```

In other shells, any command that writes CSV, TSV or JSON can feed 012,
and the exit status works as usual:

```sh
ps -eo pid,comm,%cpu | awk '{$1=$1}1' OFS='\t' | 012 --pipe | sort -t$'\t' -k3 -rn
curl -s https://api.example.com/items | 012 --pipe --to csv > items.csv
sqlite3 -csv -header app.db 'select * from users' | 012 --pipe > users.csv || echo "not sent"
```

`.nuon` and `.json` files also open, import and download like any other
[file](../files/README.md).

## Types

Each nushell type becomes a cell of a kind a spreadsheet can work with,
and goes back to the same type:

| Nushell | Cell | Sent back as |
|---|---|---|
| int, float | A number | An int when whole, a float otherwise |
| string | Text, kept as text even when it looks like a number (`"12"`) | A string |
| bool | TRUE or FALSE | A bool |
| null | A blank cell | null |
| filesize (`1646b`) | The number of bytes, in the [Size format](../sheets/formatting.md#number-formats): `1.6 kB` | A filesize, from any number in the Size format |
| duration (`90sec`) | Elapsed time, in the Duration format: `0:01:30`, with milliseconds (`0:00:01.500`) when there are fractions of a second | A duration, from any number in the Duration format |
| datetime | A date and time in the local time zone, in the Date time format | A datetime with the local offset, to the microsecond, from any number in a date or date-time format; a time of day alone is a duration since midnight |
| list, record | Their NUON text: `[a, b]`, `{k: 1}` | The list or record again, when the text is still exactly NUON |
| binary | Its NUON text, `0x[DEAD]` | Binary again, as lists and records |

A value that isn't in a table (a record, a list of numbers, a single
value) comes in as a table too: a record is one row, and a list of
values that aren't records is one column named `value`. Errors in cells
go out as their text (`"#DIV/0!"`). JSON sends numbers, booleans, text
and null as themselves, file sizes as bytes, durations as nanoseconds and
dates as RFC 3339 text, as nushell's `to json` does.

A few things don't survive the trip: an empty string comes back as null
(both are a blank cell), a whole float such as `2.0` as the int `2`,
text with line breaks on one line, and dates past the microsecond (a
cell's date is a count of days, which holds about that much). Nested
values are text in the sheet, so formulas see their text, not their
contents.
