---
title: "References"
sidebar_position: 2
---

# References

- Cells: `A1`, ranges: `A1:B3` (1-2-3's `A1..B3` also works), whole
  columns `A:C` and whole rows `2:5` (`$A:$A`, `$2:$2` absolute). Whole
  columns and rows stay whole when copied, filled or when lines are
  inserted and deleted; `A1:A1048576` reads back as `A:A`.
- Absolute parts: `$A$1`, `A$1`, `$A1`. F4 while typing cycles the reference
  at the caret through them. Copying, filling, sorting and inserting or
  deleting rows and columns adjust the relative parts, as in Sheets;
  references to deleted cells become `#REF!`.
- Named ranges: `=SUM(Sales)`. Define them from Data > Named ranges or Data >
  Define named range. Names are case-insensitive, follow the selection when
  rows move, and renaming one rewrites the formulas that use it.
- Other sheets: `=Sheet2!A1`, `=SUM('Q3 plan'!B2:B9)`; names with spaces or
  punctuation, or that look like a cell, go in single quotes. While typing a
  formula, Ctrl+PgDn or clicking a tab points into another sheet and inserts
  the reference. Renaming a sheet rewrites the formulas that use it;
  deleting one leaves them as written, showing `#REF!` ("Unresolved sheet
  name") until a sheet of that name exists again, as in Sheets. Inserting
  and deleting rows moves references into that sheet from every sheet, and
  leaves references to other sheets alone. Named ranges belong to the whole
  file and may point into any sheet.
- Each sheet is 16,384 columns (A to XFD) by 1,048,576 rows, Excel's size;
  a reference past them (`XFE1`, `A1048577`) reads as a name. Formulas cost
  what their ranges hold, not their size: `SUM(A:A)` over ten numbers reads
  ten cells, and `ROWS(A:A)` is still 1,048,576. See [Bounds of support](../contributing/limits.md)
  for what that means in practice.

## A range where one value is wanted

A range used where a formula wants one value (with an operator, or as
an argument that takes a number or text) reads as one of its cells, as
in Sheets and Excel (implicit intersection):

- a single column gives the cell in the formula's own row, so with the
  named range Rent = B2:B4, `=Rent*2` in F4 is B4*2 and `=B:B+1` in G5 is
  B5+1;
- a single row gives the cell in the formula's own column: `=B6:D6+1` in
  C8 is C6+1;
- a single cell is itself;
- anything else is `#VALUE!`: a formula outside the range's rows (or
  columns), and a range of several rows and columns.

Ranges on other sheets work the same, by the formula's row or column:
`='Q3 plan'!C2:C9*10` in A3 reads `'Q3 plan'!C3`. A range that is a
cell's whole formula, `=B2:B4` or `=Rent`, is an array and spills, as in
Sheets (see [Arrays and spills](arrays.md)). Functions that
take ranges (`SUM`, `COUNTIF`, `MATCH`, `VLOOKUP`, `SUMPRODUCT` and the
like) read the whole range, and an expression given to them is computed
over arrays: `SUM(B2:B4*2)` doubles each cell and adds them.
