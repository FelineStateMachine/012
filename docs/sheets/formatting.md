---
title: "Formatting"
sidebar_position: 3
---

# Formatting

Formats change how values show, never what they are. Select cells,
whole columns or rows, then pick a format from the Format menu, a key
([keys](../reference/keys.md#formatting)) or the palette. Entries such as
`$1,200`, `12%` or `9/26/2026` take their format as you type them
([formulas](../formulas/README.md#what-you-type)).

## Number formats

Format > Number, as Sheets names them:

| Format | Shows |
|---|---|
| Automatic | Numbers as typed, or as formulas infer from what they read |
| Plain text | Entries exactly as typed, even numbers and formulas |
| Number | Thousands separators: 1,000.12 |
| Percent | 10.12% |
| Scientific | 1.01E+03 |
| Accounting | $ at the left, negatives in parentheses: $ (1,000.12) |
| Financial | Negatives in parentheses: (1,000.12) |
| Currency | $1,000.12 |
| Currency rounded | $1,000 |
| Date | 9/26/2026 |
| Time | 3:59:00 PM |
| Date time | 9/26/2026 15:59:00 |
| Duration | Elapsed hours, minutes and seconds: 24:01:00 |

Format > Increase decimal places and Format > Decrease decimal places
show one more or one less.
A formula left Automatic shows the format of what it reads
([building formulas](../formulas/building.md)).

## Text

Bold, italic, underline and strikethrough, and alignment left, center
or right. Numbers align right, text left, booleans and errors center, as
in Sheets, until you align them yourself. Format > Clear formatting resets
number formats and text styles and keeps the contents.

## Whole columns and rows

Formatting whole columns or rows (select them with Ctrl+Space or
Shift+Space, or click their headers) keeps the format on the column or
row, as Sheets does, rather than on each of their million cells: every
cell of it shows the format unless it has its own, and a cell typed into
later takes it. A cell's format comes from the cell, else its row, else
its column, else the whole sheet's (Ctrl+A twice, then a format); the
number format and the text style fall back separately. Copies carry what
cells show ([editing](editing.md#copy-paste-and-fill)).

## Column widths

Drag a column header's right edge, or use Format > Column width (1 to
240 characters); double-clicking the edge fits the contents, and Format >
Reset column width goes back to the default.

Conditional formats, which color cells by their values, are on
[their own page](rules.md).
