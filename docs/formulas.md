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

- Cells: `A1`, ranges: `A1:B3` (1-2-3's `A1..B3` also works).
- Absolute parts: `$A$1`, `A$1`, `$A1`. F4 while typing cycles the reference
  at the caret through them. Copying, filling, sorting and inserting or
  deleting rows and columns adjust the relative parts, as in Sheets;
  references to deleted cells become `#REF!`.
- Named ranges: `=SUM(Sales)`. Define them from Data > Named ranges or Data >
  Define named range. Names are case-insensitive, follow the selection when
  rows move, and renaming one rewrites the formulas that use it.
- The worksheet is 256 columns (A to IV) by 8,192 rows, 1-2-3's size; see
  [limits.md](limits.md) for what that means in practice.

## Building formulas

- After an operator or `(`, arrow keys (or a click) pick a cell and insert
  its reference; Shift+arrows (or a drag) pick a range. Keep typing to go on.
- While you type a name, suggestions drop down: named ranges first, then
  functions with their arguments. Tab or Enter inserts `NAME(`.
- Inside a function's parentheses the context line shows its signature with
  the current argument marked, e.g. `SUMIF(range, criterion, [sum_range])`.
- Alt+, and Alt+. trace precedents and dependents: the cells a formula reads
  and the formulas that read a cell. Press again to step through them.

## Operators

`+ - * / ^` for arithmetic, `&` to join text, `= <> < > <= >=` to compare
(giving TRUE or FALSE), `%` after a number divides by 100. Unary minus binds
tighter than `^` as in Sheets and Excel, so `=-2^2` is 4. 1-2-3's `#AND#`,
`#OR#` and `#NOT#` still work; `AND()`, `OR()` and `NOT()` are the Sheets way.

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
Decimal arithmetic (or search the palette for "decimal") switches the file to
decimal math for money. The status line then says `decimal`, the menu shows a
check mark, and the setting is saved with the file and can be undone.

| Computed in decimal | Stays binary |
|---|---|
| `+ - * /` and postfix `%`; `SUM`, `AVERAGE`; `ROUND`, `ROUNDUP`, `ROUNDDOWN`, `TRUNC` | `^`, `SQRT`, `PRODUCT`, `SUMIF`, `SUMPRODUCT`, statistics, finance, dates and everything else |

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
