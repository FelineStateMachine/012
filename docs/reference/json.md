---
title: "JSON output"
sidebar_position: 7
---

# JSON output

Commands without the screen write their results as JSON with
`--format json`, or as NUON (the same values, as nushell reads them)
with `--format nuon`; `012 export` writes to a file, so its result
comes with `--report json`. The schemas below are stable: a later 012
may add fields, but never renames or removes one, or changes its type.
Errors are not JSON: they go to standard error as text, with the exit
statuses in [Command line](command-line.md#exit-status).

## Values

A cell's value in JSON keeps what its format means: money stays money
and a date a date, so what reads it can write it back as it was. The
same form is what [`012 get`](#get) and the MCP server's reads return
and what `012 set --value`, `write_cells`, `write_table` and
`create_workbook` take:

| Value | Is | Shown |
|---|---|---|
| `3.5`, `"00123"`, `true`, `null` | A number in the Automatic format, text (always text, however it looks), a boolean, a blank cell | `3.5`, `00123`, `TRUE` |
| `{"currency": 3.5}` | An amount; `"symbol": "€"` for another currency than dollars | `$3.50`, `€3.50` |
| `{"percent": 0.12}` | A fraction shown as a percentage | `12%` |
| `{"date": "2026-09-29"}` | A date, or with a time `"2026-09-29T14:30:00"` on the sheet's clock (an offset, `Z` or `-06:00`, converts it to the local time zone) | `9/29/2026` |
| `{"time": "14:30:00"}` | A time of day | `2:30:00 PM` |
| `{"duration": "90min"}` | Elapsed time, in [nushell's units](https://www.nushell.sh/book/types_of_data.html#durations) (`ns` to `wk`), or written as a number of seconds | `1:30:00` |
| `{"size": 1500}` | A number of bytes, or written as nushell's `"1.5kb"` | `1.5 kB` |
| `{"number": 1234.5, "decimals": 2}` | A number in another format than Automatic | `1,234.50` |
| `{"text": "00123"}` | Text, written this way or as a string | `00123` |

Any of the objects may add `"decimals"` (0 to 15) or `"format"`, a
[number format](../sheets/formatting.md#number-formats) code shown in
the type's place (`{"date": "2026-09-29", "format": "yyyy-mm-dd"}`).
A value read back carries them when its cell's format isn't the one its
type gives by itself, so writing it again gives the same value and
format. An error reads as its text (`"#DIV/0!"`). NUON has its own
[types](../nushell/types.md) for sizes, durations and dates, and none
for currency or percentages.

## get

`012 get file ref --format json` writes a cell's [value](#values) alone,
or a range as a list of records named by its first row
(`--no-header`: by column letters), each field a value, as
[Scripts](../files/scripts.md#get) describes. In NUON dates, file sizes
and durations are nushell's, and currency and percentages numbers.

## set

```json
{
  "file": "book.012",
  "saved": true,
  "changes": [
    {"kind": "cell", "sheet": "Sheet1", "item": "B7", "field": "input", "old": "12", "new": "=SUM(B1:B6)"}
  ],
  "warnings": ["Sheet1!C2: Enter a number between 1 and 10"]
}
```

| Field | Is |
|---|---|
| `file` | The workbook's path, as given |
| `saved` | Whether the file was written: false with `--dry-run` |
| `changes` | What changed, as [`diff`](#diff) lists it |
| `warnings` | Entries a validation rule marks invalid but lets in |

## recalc

```json
{
  "file": "book.012",
  "errors": [
    {"cell": "Q3!B7", "sheet": "Q3", "addr": "B7", "value": "#DIV/0!", "why": "Division by zero in B6/C6", "jev": false}
  ],
  "circular": false,
  "notebook_failures": []
}
```

`errors` are the formulas showing errors, sheet by sheet and row by
row; `why` is what the screen's context line says, or `""`; `jev` is
set for a JEV function's error that `--jev` would answer. `circular` is
set when a formula reads its own cell. `notebook_failures` are the
lines `--notebooks` reported for cells that failed. The exit status is
1 when there are errors, as without `--format`.

## describe

```json
{
  "sheets": [
    {
      "name": "Sales", "kind": "sheet", "shown": true, "hidden": false,
      "used": "A1:F9", "rows": 9, "cols": 6, "cells": 54, "formulas": 9, "errors": 0,
      "header_row": 1, "columns": ["Date", "Region", "Units", "Price", "Total", "Share"],
      "frozen": [1, 0], "filter": "",
      "tables": [{"name": "Orders", "range": "A1:F9", "columns": ["Date", "Region", "Units", "Price", "Total", "Share"]}],
      "regions": [{"name": "files", "kind": "output", "range": "H1:J12"}],
      "charts": [{"number": 1, "title": "Units by date", "type": "column", "data": "A1:C9", "at": "H2"}]
    }
  ],
  "names": [{"name": "Rate", "range": "Sales!B12"}]
}
```

| Field | Is |
|---|---|
| `kind` | `sheet`, `notebook` or `pivot` |
| `shown`, `hidden` | The sheet shown when the file was saved; a hidden tab |
| `used` | A1 to the last cell with contents, `""` for an empty sheet; `rows` and `cols` its size |
| `cells`, `formulas`, `errors` | Cells with contents, formulas, and formulas showing errors; `formulas` and `errors` are -1 on sheets of over a million cells, which aren't counted |
| `header_row`, `columns` | The row guessed to name the columns (1 for the first; 0 and `[]` when none) and its names, `""` for a blank cell |
| `frozen` | Frozen rows and columns |
| `filter` | The filtered range, or `""` |
| `tables` | Named tables, header row included in `range` |
| `regions` | Notebook outputs (`kind` `output`) and linked files (`linked`, with `file`) sent to the sheet, with the range their rows fill |
| `charts` | Each chart's `number` (for `012 export --chart`), title (its type's name when it has none), `type`, `data` range and `at`, the cell under its top-left corner |
| `pivot` | On a pivot table, `{"source": "Sales!A1:F9"}` |
| `notebook_cells` | On a notebook, each cell: `number`, `kind` (`code` or `note`), `name` (its output's), `source`, and for code `state` (`ran`, `failed`, `not run`) and `error` |
| `names` | Named ranges, `range` `#REF!` when its cells were deleted |

## diff

`012 diff a b --format json` writes a list of changes:

| Field | Is |
|---|---|
| `kind` | `cell`, `sheet`, `region`, `table`, `layout` (another field of a sheet: widths, rules, charts), `name`, `macro`, `notebook` or `workbook` (its locale, its arithmetic) |
| `sheet` | The sheet, by its name in the second workbook where it has one |
| `item` | The cell, region, name, macro or field that changed |
| `field` | What about it: for a cell `input`, `value`, `format` or `note`; for a sheet `added`, `removed`, `renamed` or `moved` |
| `old`, `new` | Its values, `null` where it wasn't or isn't there |

See [Diff and merge in git](../files/git.md#012-diff).

## export

`012 export file out --report json`:

```json
{"file": "out.csv", "format": "csv", "rows": 9, "notes": ["1 formula saved as values"]}
```

## merge-driver

`012 merge-driver base ours theirs --format json` writes
`{"conflicts": [...]}`, each with `where`, `field`, and the `base`,
`ours` and `theirs` versions as text (`""` where it isn't there), and
`what` for a conflict about a whole sheet. The merge and exit status
are as without it.

## MCP tools

The [MCP server](../agents/mcp.md)'s tools return these, as structured
content with the same JSON as text beside it. `describe` with a path returns `path` and
[describe's](#describe) schema, and so does `create_workbook`, of the
workbook it made; without a path, `describe` returns the folders open
to the server and the workbooks in them, `more` set when there were
more than it lists:

```json
{
  "roots": ["/Users/me/Documents/budgets"],
  "workbooks": [
    {"path": "2026.012", "format": "012", "size": 18422, "modified": "2026-09-29T14:02:11Z"},
    {"path": "bank/september.csv", "format": "CSV", "size": 5120, "modified": "2026-09-28T09:30:00Z"}
  ]
}
```

`list_errors` returns recalc's `errors` and `circular`.

`read_range`, and the range resources:

```json
{
  "range": "Sales!A1:C3", "rows": 3, "cols": 3,
  "values": [["Region", "Units", "Price"], ["North", 3, {"currency": 12.5}], ["South", 5, null]],
  "text": [["Region", "Units", "Price"], ["North", "3", "$12.50"], ["South", "5", ""]],
  "formulas": {"C3": "=IF(B3>4, \"\", 9.75)"},
  "truncated": false
}
```

`values` are [values](#values) (blank is `null`, an error its text);
`text` is each cell as shown, left out with `"text": false`;
`truncated` is set when the range held more cells than `max_cells`, and
`rows` says how many came back. The result's `_meta["o12/view"]` holds what
[the view](../agents/mcp.md#views-in-the-chat) draws: `title` (the
workbook's name), `where` and `html`.

`write_cells` takes each entry's `input` as a person types it, or its
`value` as a [value](#values), and `format`, a number format code for
the cell or, alone, for every cell of a range:

```json
{"entries": [
  {"ref": "B2", "value": {"currency": 3.5}},
  {"ref": "C2", "input": "12%"},
  {"ref": "D2", "value": {"date": "2026-09-29"}, "format": "yyyy-mm-dd"},
  {"ref": "E2:E9", "format": "#,##0.00"}
]}
```

`write_table` (and `apply_operations`' `write_table`, and
`create_workbook`'s `data`) writes rows under a header naming their
columns, in `columns`' order (a JSON object's keys can reach the tool
in any order), each row a record of values or a list of them in that
order, and formats each column as its first typed value, or as
`formats` says:

```json
{"at": "A1", "columns": ["Item", "Price", "Bought"],
 "rows": [{"Item": "Tea", "Price": {"currency": 3.5}, "Bought": {"date": "2026-09-29"}},
          ["Cake", 2, {"date": "2026-09-28"}]],
 "formats": {"Price": "$#,##0.00"}}
```

Writes (`write_cells`, `write_table`, `apply_operations`, `sort`, `filter`) return
`{"saved", "changes", "warnings"}` as [`set`](#set) does, `saved` false
for a dry run or a change that changed nothing; `create_chart` adds
`"chart": {"sheet", "number", "title"}`, its view in `_meta` as
`read_range`'s (with `name`, the chart's data), and `create_pivot`
`"sheet"`, the pivot's.

| Tool | Returns |
|---|---|
| `evaluate` | `cell` where it was computed, `value`, `text` as shown, `error` explaining an error value, and `spill`, a `read_range` result, when the formula spilled |
| `find` | `matches`, each `cell`, `text` and `input`, the first `limit` of them, and `total` |
| `run_notebook_cell` | `notebook`, `cell`, `name`, `state` (`ran` or `failed`), `error`, `output` as NUON and `cut` when the output was longer than 64 KB |

## version

```json
{"version": "v0.9.0", "go": "go1.27.1", "os": "darwin", "arch": "arm64"}
```
