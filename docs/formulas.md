# Entries and formulas

## What you type

Typing replaces the active cell, as in Google Sheets. 012 decides what the
entry is the way Sheets does:

| You type | You get |
|---|---|
| `Rent`, `Q3 total` | text |
| `42`, `-3.5`, `1,234`, `1e3` | a number |
| `$1,200`, `12%` | a number with currency or percent format |
| `9/26/2026`, `2026-09-26`, `Sep 26, 2026`, `14:30`, `2pm` | a date or time (a serial number with a date or time format) |
| `TRUE`, `false` | a boolean |
| `=A1*2`, `+A1*2` | a formula |
| `'42` | text, even though it looks like a number |

A formula that doesn't parse isn't accepted: 012 stays in EDIT mode with the
caret on the problem and says what's wrong on the context line, e.g.
`Expected , or ) in SUM`.

## References

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
  ten cells, and `ROWS(A:A)` is still 1,048,576. See [limits.md](limits.md)
  for what that means in practice.

## Building formulas

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
- Alt+, and Alt+. trace precedents and dependents: the cells a formula reads
  and the formulas that read a cell. Press again to step through them.

## Operators

`+ - * / ^` for arithmetic, `&` to join text, `= <> < > <= >=` to compare
(giving TRUE or FALSE), `%` after a number divides by 100. Unary minus binds
tighter than `^` as in Sheets and Excel, so `=-2^2` is 4. 1-2-3's `#AND#`,
`#OR#` and `#NOT#` still work; `AND()`, `OR()` and `NOT()` are the Sheets way.

## A range where one value is wanted

A range used where a formula wants one value (with an operator, as an
argument that takes a number or text, or as a cell's whole result) reads
as one of its cells, as in Sheets and Excel (implicit intersection):

- a single column gives the cell in the formula's own row, so with the
  named range Rent = B2:B4, `=Rent*2` in F4 is B4*2 and `=B:B+1` in G5 is
  B5+1;
- a single row gives the cell in the formula's own column: `=B6:D6` in C8
  is C6;
- a single cell is itself;
- anything else is `#VALUE!`: a formula outside the range's rows (or
  columns), and a range of several rows and columns.

Ranges on other sheets work the same, by the formula's row or column:
`='Q3 plan'!C2:C9` in A3 reads `'Q3 plan'!C3`. Functions that take ranges
(`SUM`, `COUNTIF`, `MATCH`, `VLOOKUP`, `SUMPRODUCT` and the like) still
read the whole range. Inside their range arguments an expression over
ranges, such as `SUM(B2:B4*2)` or `SUMPRODUCT(A1:A3*B1:B3)`, is `#VALUE!`:
Sheets computes those as arrays, which 012 doesn't yet, so it doesn't give
a single cell's answer instead.

## Values and errors

Text in arithmetic is `#VALUE!` unless it reads as a number (`="3"*2` is 6);
blanks count as 0 or empty text. Aggregates such as `SUM` over a range skip
text. Errors use Sheets' codes:

| Error | Meaning |
|---|---|
| `#DIV/0!` | division by zero |
| `#VALUE!` | the wrong kind of value, e.g. text in arithmetic |
| `#NAME?` | an unknown name |
| `#REF!` | a reference to deleted cells, or a circular reference |
| `#N/A` | not available, e.g. a lookup found nothing |
| `#NUM!` | a number out of range, e.g. `SQRT(-1)` |

Error cells are red with a curly underline, and the context line explains the
active cell's error and where it came from, e.g.
`#DIV/0!  From B3: division by zero in B5/0`, or names the chain of a
circular reference.

## Decimal arithmetic

Like Sheets and Excel, 012 computes in binary floating point, so
`=0.1+0.2=0.3` is FALSE and `=INT($4.35*100)` is 434. File > Settings >
Decimal arithmetic (or search the palette for "decimal") switches the whole
file, every sheet of it, to decimal math for money. The status line then says
`decimal`, the menu shows a check mark, and the setting is saved with the
file and can be undone.

| Computed in decimal | Stays binary |
|---|---|
| `+ - * /` and postfix `%`; `SUM`, `AVERAGE`, `PRODUCT`, `SUMIF`, `SUMIFS`, `SUMPRODUCT`; `ROUND`, `ROUNDUP`, `ROUNDDOWN`, `TRUNC` | `^`, `SQRT`, statistics, finance, dates and everything else |

Values are still stored as doubles. Each decimal step reads its inputs as the
shortest decimal that round-trips (what the cell shows), computes exactly
(division to 34 significant digits) with
[apd](https://github.com/cockroachdb/apd), and keeps the double nearest the
result, so comparisons, lookups and formats see `0.3` rather than
`0.30000000000000004`. Division still rounds: `=1/3*3` is 0.9999999999999999,
as in any decimal system. Displayed numbers are already rounded to 15
significant digits, so the setting matters for comparisons, `INT`/`MOD`-style
truncation and residues such as `=1.1-1-0.1`. Older builds ignore the setting
and compute in binary.

## Functions

See [functions.md](functions.md) for all of them, generated from the engine.
