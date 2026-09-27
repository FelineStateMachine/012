# Files

Spreadsheets save as `.012` files (JSON, one line per cell), with every
sheet. Other formats come
in through File > Import, File > Open or the command line, and go out
through File > Download. Imports run in the background with a progress
bar; Esc cancels. Saving an imported sheet asks whether to save it as a
`.012` file or download it back in its format.

File > Import asks where the data goes, as Sheets' Import location does
(a new, empty spreadsheet is simply replaced):

| Location | Does |
|---|---|
| Insert new sheet(s) | Adds the file's sheets after the sheet shown: every sheet of an `.xlsx`, named after the file for other formats. A name already taken gets a number (`Sales 2`) and the file's formulas follow it; named ranges come along unless their name is taken. One undo step, and the spreadsheet stays the file you're editing |
| Replace current sheet | Puts the data in place of the sheet shown, keeping its name and position, so formulas and named ranges that read it read the new data. Its charts stay: a chart that drew a whole table is re-pointed to the table the file has at the same corner when it has as many columns (rows for a chart by row), so last month's chart draws this month's rows; any other chart keeps its range. The context line says which chart was re-pointed, which kept its range, and which range is empty now. One undo step. Not offered for `.xlsx`, which holds several sheets |
| Replace spreadsheet | Opens the file instead, as File > Open and the command line do, asking first when there are unsaved changes |

| Format | Import | Download |
|---|---|---|
| CSV, TSV | Delimiter (`,` `;` tab `\|`), UTF-8 BOM, UTF-16 and Windows-1252 detected; entries become numbers, dates, currency and percentages as if typed; formulas stay text | Values as shown, as Sheets' Download does |
| Excel `.xlsx` | Every sheet, opening on the one Excel showed: values, formulas (references between sheets too), workbook named ranges, number formats, bold, italic, underline, strikethrough, alignment, column widths, column and row styles, shared formulas, dates in the 1904 system; sheets Excel hid stay hidden (unless it's the one Excel showed). Formulas 012 can't read (unknown functions) keep their values; Excel pivot tables come in as the values they showed. Files past the reader's limits (a zip bomb, 1 GB in one part, 2 GB in all, cells past XFD1048576) are refused with a message saying which | Every sheet, the same, with formulas in Excel's syntax and their results cached, and column and row formats as column and row styles. JEV functions and `#AND#` save as values, and so do formulas naming a sheet that doesn't exist (their `#REF!`; Excel would refuse the reference) or a sheet whose name Excel can't take as is (renamed in the file, e.g. `Plan (2)`), and pivot tables: Excel gets the results, not a pivot. The download's result counts the formulas saved as values, with an example |
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
everything else (version 4 adds several sheets and version 5 pivot tables,
below). Charts are an optional `charts` field that older builds
ignore. Saves are atomic: 012 writes a temporary file and renames it. Saving over
the open file when something else wrote it since it was opened or last
saved (another program, or another session of [012 serve](ssh.md)) asks
first: Enter overwrites, S saves under another name, Esc cancels.

Macros are an optional `macros` list, one macro per line with its
Starlark script as a string, and `macroOrigin`, the computer they were
made or trusted on; neither raises the version, and older builds ignore
them. Opening a file never runs its macros. See [macros.md](macros.md#in-the-file).

Other formats import as one sheet named after the file (or the SQLite
table); CSV and TSV downloads write the sheet shown, as Sheets' do, and
a selection of whole columns or rows downloads their data, not a million
blank lines.

## Size

A sheet is 1,048,576 rows by 16,384 columns (A to XFD), as in Excel.
Imports keep at most `max-cells` cells ([config.md](config.md), two
million by default, about 600 MB): whole rows, as many as fit, and the
context line says how many rows were left out, e.g. `only the first
166,666 rows fit in max-cells (2,000,000 cells); 12,000 rows left out`.
Data past the grid's edges is left out the same way. WK1 files keep their
own 8,192 by 256. Pastes and fills that would write more than `max-cells`
cells at once are refused. See [limits.md](limits.md).

## Column and row formats

Formatting whole columns or rows (select them with Ctrl+Space or
Shift+Space, or click their headers) keeps the format on the column or
row, as Sheets does, rather than on each of their million cells: every
cell of it shows the format unless it has its own, and a cell typed into
later takes it. A cell's format comes from the cell, else its row, else
its column, else the whole sheet's (Ctrl+A twice, then a format); the
number format and the text style fall back separately. The file keeps
them in a `lines` field after the widths, runs of lines with the same
formatting together, and older builds ignore it:

```json
  "lines": {"A:XFD": {"italic":true}, "B:D": {"format":"currency","decimals":2}, "1:1": {"bold":true}},
```

`A:XFD` is the whole sheet's format. A cell whose formatting its lines
can't express (Automatic in a currency column) has `"own": true`.

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

A hidden sheet (Hide sheet on its tab) has `"hidden": true` after its
name. It needs no version bump: builds without hidden sheets ignore the
field and show the sheet. A file whose sheets are all hidden opens with
the first one shown. XLSX downloads write hidden sheets hidden, and
sheets hidden in Excel import hidden.

## Pivot tables

A workbook with a pivot table is version 5: version 4 with a `pivot`
field, on one line after the cells of the pivot's sheet, holding its
definition. The results are never saved; they are computed again when
the file opens, so the file stays small and can't disagree with its
data. Builds that know only version 4 refuse the file rather than show an
empty sheet; a workbook without pivots is still written as version 4.

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
