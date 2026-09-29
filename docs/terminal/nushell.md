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
