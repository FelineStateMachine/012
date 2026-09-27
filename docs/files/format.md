---
title: "The .012 format"
sidebar_position: 4
---

# The .012 format

`.012` files are JSON with one line per cell, keyed by address, so diffs
read naturally and files merge reasonably in version control. A cell
without formatting is just what was typed; a formatted cell is a small
object:

```json
{
  "version": 2,
  "widths": {"A": 14},
  "cells": {
    "A1": "Rent",
    "B1": {"input":"1450","format":"currency","decimals":2},
    "B2": {"input":"95","note":"Due on the 1st"},
    "B3": "=SUM(B1:B2)"
  }
}
```

A formatted cell has its `input` and the formatting that isn't the
default: `format` (as `number_format` in [macros](../reference/macro-api.md#cells) names
it: `currency`, `percent`, `date`, ...), `decimals`, `pattern` for a
custom format, `bold`, `italic`, `underline`, `strikethrough` and
`align` (`left`, `center`, `right`).

## Versions

012 writes the lowest version that holds the workbook, so older builds
open what they can, and refuses a version newer than it knows rather
than lose sheets or pivots. Version 1 files (cells as plain strings)
still load.

| Version | Written when | Adds |
|---|---|---|
| 2 | One sheet, using none of the below | `cells`, `widths` |
| 3 | A sheet has named ranges, frozen panes or a filter | `names` (each name's range, `#REF!` once its cells were deleted), `freeze` (`rows`, `cols`), `filter` (its `range`, and `columns` by letter, each with `hidden` values or a `condition` and its `value`) |
| 4 | Several sheets, or a formula naming a sheet | `sheets`, a list; see [Several sheets](#several-sheets) |
| 5 | A pivot table | a sheet's `pivot`; see [Pivot tables](#pivot-tables) |

## Fields that need no version

Builds that don't know these fields ignore them (and drop them if they
save), so they raise no version:

| Field | On | Holds |
|---|---|---|
| `note` | a cell | Its [note](../sheets/notes-protection.md#notes) |
| `own` | a cell | `true` when its formatting is its own, not its column's or row's (Automatic in a currency column) |
| `lines` | a sheet | [Column and row formats](#column-and-row-formats) |
| `name`, `hidden` | a sheet | Its name when renamed; `true` when hidden (a file whose sheets are all hidden opens with the first one shown) |
| `charts` | a sheet | One chart per line: `type` (`column`, `bar`, `line`, `pie`, `area`, `scatter`), `data`, `at`, `width`, `height`, `byRow`, `header`, `labels`, `title`, and options left out at their defaults: `stack` (`stacked`, `percent`), `trend`, `min`, `max`, `log`, `gridlines` (only when off), `legend` (`right`, `none`). A build that charts but lacks a chart's type refuses the file |
| `protected` | a sheet | [Protected ranges](../sheets/notes-protection.md#protected-sheets-and-ranges): `{"range":"B2:C9","description":"Totals"}`, or `{"sheet":true}` |
| `conditionalFormats`, `validations` | a sheet | [Rules](#conditional-formats-and-data-validation), one per line |
| `arithmetic` | the workbook | `decimal` for [decimal arithmetic](../formulas/decimal.md) |
| `macros`, `macroOrigin` | the workbook | Macros as Starlark scripts, and the computer they were made or trusted on: see [macros.md](../reference/macro-api.md#in-the-file). Opening a file never runs them |

## Column and row formats

[Formats of whole columns and rows](../sheets/editing.md#rows-and-columns) are kept
in a `lines` field after the widths, runs of lines with the same
formatting together; `A:XFD` is the whole sheet's format:

```json
  "lines": {"A:XFD": {"italic":true}, "B:D": {"format":"currency","decimals":2}, "1:1": {"bold":true}},
```

## Several sheets

In version 4 the sheets are a list, each with its name and the fields a
version 3 file has at the top, and named ranges say their sheet. The
workbook's settings (named ranges, decimal arithmetic, the sheet shown
when saved as `active`) stay at the top:

```json
{
  "version": 4,
  "names": {"Rent": "Budget!B1"},
  "active": 1,
  "sheets": [
    {
      "name": "Budget",
      "cells": {
        "A1": "Rent",
        "B1": "1450"
      }
    },
    {
      "name": "Q3 plan",
      "hidden": true,
      "cells": {
        "A1": "=Budget!B1*3"
      }
    }
  ]
}
```

## Pivot tables

A pivot's sheet has a `pivot` field, on one line after its cells,
holding the definition. The results are never saved; they are computed
again when the file opens, so the file stays small and can't disagree
with its data.
Arrays that formulas spill are kept the same way: the file keeps the formula,
and the cells it spills into only for their formatting and notes.

```json
{
  "name": "Pivot Table 1",
  "cells": {},
  "pivot": {"source":"Sales!A1:D200","rows":[{"column":"B"},{"column":"A","order":"desc","sortBy":1}],"columns":[{"column":"C"}],"values":[{"column":"D","summarize":"sum"},{"column":"D","summarize":"counta","showAs":"percent_of_total","name":"Share"}],"filters":[{"column":"A","hidden":["North"]}],"rowTotals":true,"columnTotals":false}
}
```

- `source` is the data, with its sheet; `Sales!#REF!` once the range was
  deleted. The sheet is by name, as in formulas, and follows renames.
- `rows` and `columns` are fields by column letter on the source sheet,
  with `order` `desc` for Z to A and `sortBy` the value, counting from 1,
  whose totals order the groups.
- `values` summarize a column: `sum`, `counta`, `count`, `countunique`,
  `average`, `max`, `min`, or `rows` (every row, blank or not, which
  frequency tables use); `showAs` is `percent_of_row`,
  `percent_of_column` or `percent_of_total`; `name` replaces the header.
- `filters` take a filter column's criteria: `hidden` values and a
  `condition` with its `value`.
- `rowTotals` and `columnTotals` are the grand total row and column.

## Conditional formats and data validation

A sheet's rules are two lists after its cells and charts, one rule per
line:

```json
  "conditionalFormats": [
    {"ranges":"A2:A6","condition":"formula","values":["=$D2"],"text":"green","strikethrough":true},
    {"ranges":"C2:C6","scale":[{"type":"min","color":"green"},{"type":"percentile","value":"50","color":"yellow"},{"type":"max","color":"red"}]}
  ],
  "validations": [
    {"ranges":"B2:B6","criteria":"list","items":["Ann","Bo","Cy"],"help":"Pick who does it"},
    {"ranges":"D2:D6","criteria":"checkbox"},
    {"ranges":"C2:C6","criteria":"number","condition":"between","values":["0","80"],"reject":true}
  ]
```

- `ranges` are the rule's ranges, `A2:A9,C2:C9`.
- A single-color rule has a `condition` (`empty`, `not_empty`, `contains`,
  `not_contains`, `starts_with`, `ends_with`, `exactly`, `date_is`,
  `date_before`, `date_after`, `gt`, `ge`, `lt`, `le`, `eq`, `ne`,
  `between`, `not_between`, `formula`), its `values` as typed, and its
  style: `text` and `fill` colors (`red`, `yellow`, `green`, `cyan`,
  `blue`, `magenta`) and `bold`, `italic`, `underline`, `strikethrough`.
- A color scale has `scale`: two or three points, each a `type` (`min`,
  `max`, `num`, `percent`, `percentile`), a `value` where it takes one,
  and a `color`.
- A validation rule has `criteria` (`list`, `range`, `checkbox`,
  `number`, `date`, `length`, `formula`) with its `items`, `source`
  (`Lists!A1:A20`), or `condition` (`between`, `not_between`, `eq`, `ne`,
  `gt`, `ge`, `lt`, `le`; none for any date) and `values`; `reject` refuses
  invalid entries, and `help` replaces the rule's own help text.

A line of either list is also what Add conditional format rule and Add
data validation rule take (in the palette, and as `run(...,
answer=...)` in macros), which is how a macro records a rule added from
the panel.
