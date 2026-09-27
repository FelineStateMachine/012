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
| Replace current sheet | Puts the data in place of the sheet shown, keeping its name and position, so formulas and named ranges that read it read the new data. Its charts that fit the new data stay: a chart that drew a whole table is re-pointed to the table the file has at the same corner when it has as many columns (rows for a chart by row), so last month's chart draws this month's rows, and one of part of a table, or of whole columns or rows, keeps its range. A chart whose table now has other columns, or whose range is empty now, is removed. The context line lists the removed charts first, then which were re-pointed and which kept their range. One undo step, which brings the removed charts back. Not offered for `.xlsx`, which holds several sheets |
| Replace spreadsheet | Opens the file instead, as File > Open and the command line do, asking first when there are unsaved changes |

| Format | Import | Download |
|---|---|---|
| CSV, TSV | Delimiter (`,` `;` tab `\|`), UTF-8 BOM, UTF-16 and Windows-1252 detected; entries become numbers, dates, currency and percentages as if typed; formulas stay text | Values as shown, as Sheets' Download does |
| Excel `.xlsx` | Every sheet, opening on the one Excel showed: values, formulas (references between sheets too), workbook named ranges, number formats, bold, italic, underline, strikethrough, alignment, column widths, column and row styles, frozen panes, filters (see [Filters in XLSX](#filters-in-xlsx)), notes (Excel's notes, its legacy comments; threaded comments aren't read), shared formulas, dates in the 1904 system; sheets Excel hid stay hidden (unless it's the one Excel showed), conditional formatting and data validation (see [Rules in XLSX](#rules-in-xlsx)). Formulas 012 can't read (unknown functions) keep their values; Excel pivot tables come in as the values they showed. Array formulas (Excel 365's dynamic arrays and older `{=...}` ones) come in as formulas that spill again, without the values Excel kept in the cells they spilled into. A sheet Excel protects comes in unprotected, with a note saying so, since Excel's protection locks cells where 012's only warns. Files past the reader's limits (a zip bomb, 1 GB in one part, 2 GB in all, cells past XFD1048576) are refused with a message saying which | Every sheet, the same, with formulas in Excel's syntax and their results cached, column and row formats as column and row styles, frozen rows and columns as frozen panes, a filter as Excel's with the rows it hides hidden, and notes as Excel's notes (which Sheets reads as notes), and conditional formats and data validation as Excel's. Protected ranges aren't written. A sheet whose name Excel can't take as is (spaces at its ends, or the same as another's but for them and case) gets one it can (without the spaces, with a number when two would clash, e.g. `Plan (2)`), and the formulas and named ranges naming it name that. A formula that spills goes out as Excel 365's dynamic array formula, with the values it spilled in the cells, so Excel spills it the same and older Excels show the values; see [Arrays in XLSX](#arrays-in-xlsx). JEV functions and `#AND#` save as values, and so do formulas naming a sheet that doesn't exist (their `#REF!`; Excel would refuse the reference), functions only Sheets has (`SORTN`, `FLATTEN`, `SPLIT`, `REGEXMATCH`, `REGEXEXTRACT`), and pivot tables: Excel gets the results, not a pivot. The download's result counts the formulas saved as values, with an example |
| SQLite | Pick a table or view, or type a query; a header row names the columns | The sheet shown or the selection as a table, first row as column names; a table of that name is replaced |
| Parquet | Every column, with dates and timestamps; lists joined with commas | |
| Lotus 1-2-3 `.wk1`, `.wks` | Numbers, labels with their alignment, formats, column widths, formulas translated (references, operators, `@SUM`, `@AVG`, `@IF`, `@ROUND`, `@PMT` and 60 more) or kept as values | |

## Arrays in XLSX

A formula whose array spills (see [formulas.md](formulas.md#arrays-and-spills))
is written as Excel writes a dynamic array formula: an array formula over
the cells it spills into (`<f t="array" ref="C1:C9">`) on a cell whose
metadata marks it dynamic (`cm="1"`, defined in `xl/metadata.xml`), with
the values in the cells below and right of it. Formulas calling array
functions (`FILTER`, `SORT`, `UNIQUE`, `SEQUENCE`, `LET`, `LAMBDA` and the
like) or holding an array literal are written that way even when they
compute one value, so Excel doesn't put its implicit intersection (`@`)
in front of them. Excel gets its own names for the functions newer than
Excel 2007 (`_xlfn._xlws.FILTER`, `_xlfn.SEQUENCE`), `_xlpm.` before the
names LET and LAMBDA bind, and the formula inside an `ARRAYFORMULA` around
a whole formula, which a dynamic array formula computes over arrays
anyway. What Excel can't hold goes out as the value it showed, counted in
the download's note: functions only Sheets has (`SORTN`, `FLATTEN`,
`SPLIT`, `REGEXMATCH`, and `REGEXEXTRACT`, whose Excel namesake returns
the whole match where Sheets' returns the capture group), array literals
holding references (Excel's hold only constants) and `ARRAYFORMULA`
inside a formula.

## Filters in XLSX

A sheet's filter goes out as Excel's AutoFilter over the same range, and
an AutoFilter comes in as a filter:

| 012 | Excel |
|---|---|
| Filter by values: the values unchecked | The list of values shown (Excel keeps those checked), with blanks when they're shown. Values are compared as displayed, ignoring case |
| Is empty | Blanks only |
| Is not empty | Custom filter: does not equal a space |
| Text contains, does not contain, starts with, ends with, is exactly | Custom filter: equals or does not equal `*text*`, `text*`, `*text`, `text`, with Excel's wildcards in the text escaped (`~*`) |
| Greater than, less than, is equal to, is not equal to (and or equal to) | Custom filter with that operator; numbers typed as `$1,200` or `12%` go out as the number |

The rows the filter hides are written hidden, since Excel shows a file's
rows as saved rather than filtering again. A column with both unchecked
values and a condition goes out as the list of values both let through,
and comes back as that list. What 012's filters can't do is left out,
with a note saying how many criteria and which: two conditions in one
column (`and`, `or`), wildcards other than at the ends of the text (`a?c`,
`a*c`), top 10, dynamic filters (above average, this month), dates
grouped by year or month, and filtering by color or icon. The filter
itself still comes in over its range. Filters of Excel tables (as
opposed to the sheet's AutoFilter) aren't read.

## Rules in XLSX

Conditional formats ([data.md](data.md#conditional-formatting)) go out
as Excel's conditional formatting and data validation as its data
validation, and come back:

| 012 | Excel |
|---|---|
| Is empty, is not empty | `containsBlanks`, `notContainsBlanks` |
| Text contains, does not contain, starts with, ends with | `containsText`, `notContainsText`, `beginsWith`, `endsWith` |
| Text is exactly | `cellIs` equal to the text |
| Date is today, tomorrow, yesterday | `timePeriod` |
| Date is, is before, is after a date | an expression, `INT(A1)<DATE(2026,9,30)` |
| Greater than ... is not between | `cellIs` with the operator; dates as Excel's serial numbers |
| Custom formula is | `expression`, in Excel's syntax |
| Color scale | `colorScale` with its points (`min`, `max`, `num`, `percent`, `percentile`) |
| A rule's style | a differential style (dxf): bold, italic, underline, strikethrough, the text color and a solid fill |
| Dropdown, from a range | `list` of the items in quotes, or of the range (`Lists!$A$1:$A$20`) |
| Checkbox | `list` of `TRUE,FALSE`, which comes back as a checkbox |
| Number, date, text length | `decimal`, `date`, `textLength` with the operator; any date as a date greater than 0 |
| Custom formula is | `custom` |
| Reject the input, show a warning | error style `stop`, `warning`; help text as the prompt |

Each single-color rule stops the ones after it (`stopIfTrue`) and its
priority is its place, so Excel applies the first that matches, as 012
does. Named colors go out in the colors of Sheets' palette, and Excel's
colors (ARGB, theme and indexed) come in as the nearest named color by
hue, grays as none; a color scale's gray or white point takes the nearest
scale color. Validation in Excel 2010's extension (lists from other
sheets) is read too; whole-number rules come in as number rules.

Left out, with a note saying how many and which: conditional formats 012
has no rule for (data bars, icon sets, top or bottom values, above or
below average, duplicate or unique values, errors, date periods other
than today, tomorrow and yesterday, and what Excel 2010's extension
holds), rules whose style has only a number format or borders, time
validation and lists from named ranges or formulas. Going out, a
formula with no Excel equivalent (`#AND#`) leaves its rule out, and so
do list items with commas or quotes, or longer than Excel's 255
characters.

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
ignore; a chart's options are fields left out at their defaults, which
builds without them draw with the defaults, while builds that chart but
lack a chart's type (area, scatter) can't open the file. A cell's `note`
(the cell is then an object, e.g. `{"input":"1450","note":"Due on the 1st"}`)
and a sheet's `protected` list of protected ranges
(`{"range":"B2:C9","description":"Totals"}`, or `{"sheet":true}`) are
optional too and raise no version. Saves are atomic: 012 writes a temporary file and renames it. Saving over
the open file when something else wrote it since it was opened or last
saved (another program, or another session of [012 serve](ssh.md)) asks
first: Enter overwrites, S saves under another name, Esc cancels. Save
as (and `:w name`, `:wq name` with vim keys) onto another file that
exists asks the same way: Enter replaces it, Esc cancels, and cancelling
`:wq`'s question cancels its quit too.

Under [012 serve](ssh.md), a session that idles out or is ended by the
server stopping keeps its unsaved changes in `.012-recovery/` in the
served directory, and the next session opening that file offers them
back: see [ssh.md](ssh.md#unsaved-work).

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
data. Arrays that formulas spill are the same: the file keeps the formula,
and the cells it spills into only for their formatting. Builds that know only version 4 refuse the file rather than show an
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

## Conditional formats and data validation

A sheet's rules are two optional fields after its cells and charts, one
rule per line; they need no version, and older builds ignore them
(losing the rules, as they lose charts):

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
