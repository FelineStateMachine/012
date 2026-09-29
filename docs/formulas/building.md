---
title: "Building formulas"
sidebar_position: 3
---

# Building formulas

- After an operator or `(`, arrow keys (or a click) pick a cell and insert
  its reference; Shift+arrows (or a drag) pick a range. Keep typing to go on.
- While you type a name, suggestions drop down: named ranges first, then
  the other sheets, then functions with their arguments. Tab or Enter
  inserts `NAME(`, or a sheet with its `!`, quoted when it needs to be
  (`=Su` offers `Summary!`; `=Q3` or `='Q3` offers `'Q3 plan'!`). An arrow
  after it points into that sheet, as clicking its tab does. Hidden
  sheets aren't offered, though formulas still read them.
- Inside a function's parentheses the context line shows its signature with
  the current argument marked, e.g. `SUMIF(range, criterion, [sum_range])`.
- Alt+; marks what the active cell's formula reads and the formulas that
  read it, as the pointer moves; Alt+= steps through a formula one part
  at a time. See [Tracing formulas](tracing.md).
- A formula left Automatic shows the format of what it reads: `=B5*2` of
  a currency cell shows currency, and so does `=SUM(B2:B9)`. A blank cell
  counts with its column's, row's or sheet's format, and changing those
  formats changes what the formulas show at once.
