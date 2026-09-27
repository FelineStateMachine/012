---
title: "Excel files"
sidebar_position: 2
---

# Excel files

**Read:** every sheet, opening on the one Excel showed; values, formulas
(references between sheets too, shared formulas), workbook named ranges,
number formats, bold, italic, underline, strikethrough, alignment,
column widths, column and row styles, frozen panes, [filters](#filters-in-excel),
notes (Excel's notes, its legacy comments; threaded comments aren't
read), [conditional formatting and data validation](#rules-in-excel),
[array formulas](#arrays-in-excel),
dates in the 1904 system, and hidden sheets (unless it's the one Excel
showed).

**Written:** the same, with formulas in Excel's syntax and their results
cached, column and row formats as column and row styles, frozen rows and
columns as frozen panes, a filter as Excel's with the rows it hides
hidden, notes as Excel's notes (which Sheets reads as notes), and
formulas that spill as Excel 365's [dynamic array formulas](#arrays-in-excel).

What changes on the way:

| | |
|---|---|
| Formulas 012 can't read (unknown functions) | Keep their values |
| Excel pivot tables | Come in as the values they showed; 012's go out as their results, not a pivot |
| Excel's sheet protection | Comes in unprotected, with a note saying so: Excel's protection locks cells where 012's only warns. Protected ranges aren't written |
| Array formulas (Excel 365's dynamic arrays and older `{=...}` ones) | Come in as formulas that spill again, without the values Excel kept in the cells they spill into |
| Functions only Sheets has (`SORTN`, `FLATTEN`, `SPLIT`, `REGEXMATCH`, `REGEXEXTRACT`) | Go out as values, counted with the others below |
| JEV functions, `#AND#`, formulas naming a sheet that doesn't exist | Go out as values (their `#REF!`, for a missing sheet: Excel would refuse the reference). The download's result counts the formulas saved as values, with an example |
| A sheet name Excel can't take as is (spaces at its ends, or the same as another's but for them and case) | Written as one it can: without the spaces, with a number when two would clash (`Plan (2)`); formulas and named ranges naming it name that |
| Files past the reader's limits (a zip bomb, 1 GB in one part, 2 GB in all, cells past XFD1048576) | Refused, with a message saying which |

## Filters in Excel

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

## Arrays in Excel

A formula whose array spills (see [Arrays and spills](../formulas/arrays.md))
is written as Excel writes a dynamic array formula: an array formula over
the cells it spills into (`<f t="array" ref="C1:C9">`) on a cell whose
metadata marks it dynamic (`cm="1"`, defined in `xl/metadata.xml`), with
the values in the cells below and right of it, so Excel spills it the
same and older Excels show the values. Formulas calling array functions
(`FILTER`, `SORT`, `UNIQUE`, `SEQUENCE`, `LET`, `LAMBDA` and the like) or
holding an array literal are written that way even when they compute one
value, so Excel doesn't put its implicit intersection (`@`) in front of
them. Excel gets its own names for the functions newer than Excel 2007
(`_xlfn._xlws.FILTER`, `_xlfn.SEQUENCE`), `_xlpm.` before the names LET
and LAMBDA bind, and the formula inside an `ARRAYFORMULA` around a whole
formula, which a dynamic array formula computes over arrays anyway.
What Excel can't hold goes out as the value it showed, counted in the
download's note: functions only Sheets has (`SORTN`, `FLATTEN`, `SPLIT`,
`REGEXMATCH`, and `REGEXEXTRACT`, whose Excel namesake returns the whole
match where Sheets' returns the capture group), array literals holding
references (Excel's hold only constants) and `ARRAYFORMULA` inside a
formula.

## Rules in Excel

[Conditional formats](../sheets/rules.md#conditional-formatting) go out as Excel's
conditional formatting, and [data validation](../sheets/rules.md#data-validation)
as its data validation, and both come back:

| Conditional format | Excel |
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

| Data validation | Excel |
|---|---|
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
