# Files

Spreadsheets save as `.012` files (JSON, one line per cell), with every
sheet. Other formats come
in through File > Import, File > Open or the command line, and go out
through File > Download. Imports run in the background with a progress
bar; Esc cancels. Saving an imported sheet asks whether to save it as a
`.012` file or download it back in its format.

| Format | Import | Download |
|---|---|---|
| CSV, TSV | Delimiter (`,` `;` tab `\|`), UTF-8 BOM, UTF-16 and Windows-1252 detected; entries become numbers, dates, currency and percentages as if typed; formulas stay text | Values as shown, as Sheets' Download does |
| Excel `.xlsx` | Every sheet, opening on the one Excel showed: values, formulas (references between sheets too), workbook named ranges, number formats, bold, italic, underline, strikethrough, alignment, column widths. Formulas 012 can't read (unknown functions) keep their values | Every sheet, the same, with formulas in Excel's syntax and their results cached. JEV functions and `#AND#` save as values |
| SQLite | Pick a table or view, or type a query; a header row names the columns | The sheet shown or the selection as a table, first row as column names; a table of that name is replaced |
| Parquet | Every column, with dates and timestamps; lists joined with commas | |
| Lotus 1-2-3 `.wk1`, `.wks` | Numbers, labels with their alignment, formats, column widths, formulas translated (references, operators, `@SUM`, `@AVG`, `@IF`, `@ROUND`, `@PMT` and 60 more) or kept as values | |

## The native format

`.012` files are JSON with one line per cell, keyed by address, so diffs
read naturally and files merge reasonably in version control:

```json
{
  "version": 2,
  "widths": {"A": 14},
  "cells": {
    "A1": "Rent",
    "B1": {"input":"1450","format":"currency","decimals":2},
    "B3": "=SUM(B1:B2)"
  }
}
```

A cell without formatting is just what was typed; a formatted cell is a
small object. Version 3 adds named ranges, frozen panes and a filter, and is
only written when a sheet uses one of them, so older builds of 012 can open
everything else. Charts are an optional `charts` field that older builds
ignore. Saves are atomic: 012 writes a temporary file and renames it.

Macros are an optional `macros` list, one macro per line with its
Starlark script as a string, and `macroOrigin`, the computer they were
made or trusted on; neither raises the version, and older builds ignore
them. Opening a file never runs its macros. See [macros.md](macros.md#in-the-file).

Other formats import as one sheet named after the file (or the SQLite
table); CSV and TSV downloads write the sheet shown, as Sheets' do.

## Several sheets

A file of one sheet, whose formulas name no sheet, keeps the single-sheet
layout above, so older builds of 012 open it; a renamed sheet adds a `name`
field they ignore. Anything else is version 4: the sheets are a list, each
with its name and the fields a version 3 file has at the top, and named
ranges say their sheet. The workbook's settings (named ranges, decimal
arithmetic, the sheet shown when saved) stay at the top:

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
      "cells": {
        "A1": "=Budget!B1*3"
      }
    }
  ]
}
```

Older builds refuse version 4 files rather than lose sheets.
