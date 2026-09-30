---
title: "Types"
sidebar_position: 4
---

# Types

Each [nushell type](https://www.nushell.sh/book/types_of_data.html)
becomes a cell of a kind a spreadsheet can work with, and goes back to
the same type. A file size is a number of bytes in the Size format, so
`=SUM` adds it and `where size > 1kb` still compares it once it's back
in nushell ([`sheet`](pipelines.md#the-sheet-command) is 012 as a stage
of a pipeline):

```nu
ls | select name size | sheet | describe
# table<name: string, size: filesize>
```

This holds wherever NUON carries a table: [pipelines](pipelines.md),
[notebook cells](notebooks.md) and their outputs sent to sheets, `.nuon` files opened, imported or
downloaded, and linked `.nuon` files.

| Nushell | Cell | Sent back as |
|---|---|---|
| [int, float](https://www.nushell.sh/book/types_of_data.html#integers) | A number | An int when whole, a float otherwise |
| [string](https://www.nushell.sh/book/types_of_data.html#text-strings) | Text, kept as text even when it looks like a number (`"12"`) | A string |
| [bool](https://www.nushell.sh/book/types_of_data.html#booleans) | TRUE or FALSE | A bool |
| [null](https://www.nushell.sh/book/types_of_data.html#nothing-null) | A blank cell | null |
| [filesize](https://www.nushell.sh/book/types_of_data.html#file-sizes) (`1646b`) | The number of bytes, in the [Size format](../sheets/formatting.md#number-formats): `1.6 kB` | A filesize, from any number in the Size format |
| [duration](https://www.nushell.sh/book/types_of_data.html#durations) (`90sec`) | Elapsed time, in the Duration format: `0:01:30`, with milliseconds (`0:00:01.500`) when there are fractions of a second | A duration, from any number in the Duration format |
| [datetime](https://www.nushell.sh/book/types_of_data.html#dates) | A date and time in the local time zone, in the Date time format | A datetime with the local offset, to the microsecond, from any number in a date or date-time format; a time of day alone is a duration since midnight |
| [list](https://www.nushell.sh/book/types_of_data.html#lists), [record](https://www.nushell.sh/book/types_of_data.html#records) | Their NUON text: `[a, b]`, `{k: 1}` | The list or record again, when the text is still exactly NUON |
| [binary](https://www.nushell.sh/book/types_of_data.html#binary-data) | Its NUON text, `0x[DEAD]` | Binary again, as lists and records |

Since the types are the cells' formats, formatting a column in 012
decides what goes back: numbers given the Size format go back as file
sizes, and a date column formatted as plain numbers goes back as
numbers.

## Currency and percentages

Nushell has no type for money or percentages, so `$3.50` goes to
nushell as the number `3.5` and `12%` as `0.12`, which `math sum` and
`where price > 3` work on. The format comes back with the column: when
a [notebook cell](notebooks.md)'s output is sent to a sheet, each of
its columns named as a column of the ranges the cell read
(`$sheet.A1:C9`, `$selection`) takes that column's format again, when
its values are still of the type the format shows. A price column read
as currency and sent back is currency, a date column shows its dates
in its own pattern (`2026-09-29` rather than the Date time format), and
a size column keeps its decimals:

```nu
$sheet.A1:D9 | where price > 3 | sort-by bought   # price is currency again on the sheet
```

A column named anew (`insert total {|r| $r.price * $r.qty}`) has no
format to take, and shows plain numbers. Outside notebooks, [`012 get --format json`](../reference/json.md#values)
keeps currency and percentages as typed values.

## Tables, records and single values

A value that isn't in a table comes in as a table too: a
[record](https://www.nushell.sh/book/types_of_data.html#records) is one
row (`sys host`), and a list of values that aren't records is one
column named `value`. Errors in cells go out as their text
(`"#DIV/0!"`).

## JSON

JSON has fewer types, so a table sent as JSON (`--to json`, or a table
that came in as JSON) carries numbers, booleans, text and null as
themselves, file sizes as bytes, durations as nanoseconds and dates as
RFC 3339 text, as nushell's
[`to json`](https://www.nushell.sh/commands/docs/to_json.html) does.
Use NUON to keep the types.

## What doesn't survive the trip

- An empty string comes back as null: both are a blank cell.
- A whole float such as `2.0` comes back as the int `2`.
- Text with line breaks comes back on one line.
- Dates past the microsecond lose the rest: a cell's date is a count of
  days, which holds about that much.
- Nested values are text in the sheet, so formulas see their text, not
  their contents. Flatten them in nushell first
  ([`flatten`](https://www.nushell.sh/commands/docs/flatten.html),
  [`select`](https://www.nushell.sh/commands/docs/select.html) with a
  cell path) to work with them as columns.
