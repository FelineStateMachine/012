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

## get

`012 get file ref --format json` writes a cell's value alone (a number,
a string, `true`, `null` for a blank cell), or a range as a list of
records named by its first row (`--no-header`: by column letters), as
[Scripts](../files/scripts.md#get) describes. Dates are strings in ISO
8601, and in NUON dates, file sizes and durations keep their types.

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
content with the same JSON as text beside it. `describe` returns
[describe's](#describe) schema, `list_errors` recalc's `errors` and
`circular`.

`read_range`, and the range resources:

```json
{
  "range": "Sales!A1:C3", "rows": 3, "cols": 3,
  "values": [["Region", "Units", "Price"], ["North", 3, 12.5], ["South", 5, null]],
  "text": [["Region", "Units", "Price"], ["North", "3", "$12.50"], ["South", "5", ""]],
  "formulas": {"C3": "=IF(B3>4, \"\", 9.75)"},
  "truncated": false
}
```

`values` are typed as [`get`](#get) types one cell (blank is `null`, an
error its text); `text` comes only when asked for; `truncated` is set
when the range held more cells than `max_cells`, and `rows` says how
many came back.

Writes (`write_cells`, `apply_operations`, `sort`, `filter`) return
`{"saved", "changes", "warnings"}` as [`set`](#set) does, `saved` false
for a dry run or a change that changed nothing; `create_chart` adds
`"chart": {"sheet", "number", "title"}` and `create_pivot` `"sheet"`,
the pivot's.

| Tool | Returns |
|---|---|
| `evaluate` | `cell` where it was computed, `value`, `text` as shown, `error` explaining an error value, and `spill`, a `read_range` result, when the formula spilled |
| `find` | `matches`, each `cell`, `text` and `input`, the first `limit` of them, and `total` |
| `run_notebook_cell` | `notebook`, `cell`, `name`, `state` (`ran` or `failed`), `error`, `output` as NUON and `cut` when the output was longer than 64 KB |

## version

```json
{"version": "v0.9.0", "go": "go1.27.1", "os": "darwin", "arch": "arm64"}
```
