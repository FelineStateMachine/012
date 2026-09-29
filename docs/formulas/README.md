---
title: "Formulas"
sidebar_position: 1
---

# Formulas

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

That's in English (United States); in another [locale](../sheets/locale.md)
numbers, currency and dates are typed its way (`1.234,5`, `12,50 €`,
`26.09.2026` in German), and formulas with a decimal comma separate
their arguments with `;`: `=ROUND(A1*1,19; 2)`.

A formula that doesn't parse isn't accepted: 012 stays in EDIT mode with the
caret on the problem and says what's wrong on the context line, e.g.
`Expected , or ) in SUM`.

## Operators

`+ - * / ^` for arithmetic, `&` to join text, `= <> < > <= >=` to compare
(giving TRUE or FALSE), `%` after a number divides by 100. Unary minus binds
tighter than `^` as in Sheets and Excel, so `=-2^2` is 4. 1-2-3's `#AND#`,
`#OR#` and `#NOT#` still work; `AND()`, `OR()` and `NOT()` are the Sheets way.

Numbers are binary floating point, as in Sheets and Excel, and follow
Sheets where that shows:

- Comparisons, and criteria such as `COUNTIF`'s, look at the 15
  significant digits a cell shows, so `=0.1+0.2=0.3` is TRUE. A
  difference itself keeps every digit: `=0.1+0.2-0.3` is 5.551E-17
  ([decimal arithmetic](decimal.md) makes it 0).
- A number written in a formula keeps 15 significant digits:
  `=123456789012345678` is 123456789012345000.
- A negative number has odd roots: `=(-8)^(1/3)` and `=POWER(-8, 1/3)`
  are -2; other fractional powers of it are `#NUM!`.

## Values and errors

Text in arithmetic is `#VALUE!` unless it reads as a number (`="3"*2` is 6);
blanks count as 0 or empty text. A condition (`IF`, `IFS`, `AND`, `OR`,
`NOT`) also reads the text `TRUE` or `FALSE`, and empty text as FALSE, so
`=IF("TRUE", 1, 2)` is 1. Aggregates such as `SUM` over a range skip
text; `COUNT` skips errors too and `COUNTA` counts them, where the others
give the first error in the range. Errors use Sheets' codes:

| Error | Meaning |
|---|---|
| `#DIV/0!` | division by zero |
| `#VALUE!` | the wrong kind of value, e.g. text in arithmetic |
| `#NAME?` | an unknown name |
| `#REF!` | a reference to deleted cells, or a circular reference: every cell of the cycle, even one whose formula catches errors (`IFERROR`) or skips them (`COUNTIF`), and one reached only through an `IF` branch not taken or a part of a range the formula doesn't read (`INDEX`, `VLOOKUP`), as in Sheets |
| `#N/A` | not available, e.g. a lookup found nothing |
| `#NUM!` | a number out of range, e.g. `SQRT(-1)`, or a position outside what `CHOOSE`, `INDEX` or `MID` reads from, as in Sheets (Excel says `#VALUE!` or `#REF!`) |

Error cells are red with a curly underline, and the context line explains the
active cell's error and where it came from, e.g.
`#DIV/0!  From B3: division by zero in B5/0`, or names the chain of a
circular reference.

## Regular expressions

`REGEXMATCH`, `REGEXEXTRACT` and `REGEXREPLACE` use Go's regular
expressions, which are RE2, as Sheets' are: the same patterns mean the
same thing. They take text only: a number is `#VALUE!`, as in Sheets
(`A1&""` makes one text). `REGEXEXTRACT` returns the first
capture group, or the whole match without one, and spills several
groups across; `REGEXREPLACE` writes a group in the replacement as `$1`.
`SPLIT` spills the pieces of a text across, reading those that look like
numbers as numbers.

## More

| Page | For |
|---|---|
| [References](references.md) | Cells, ranges, names, other sheets, and a range where one value is wanted |
| [Building formulas](building.md) | Pointing, suggestions, argument hints |
| [Tracing formulas](tracing.md) | Precedents and dependents in the grid, and a formula evaluated step by step |
| [Arrays and spills](arrays.md) | FILTER, SORT, UNIQUE, SEQUENCE, ARRAYFORMULA, LET and LAMBDA |
| [Decimal arithmetic](decimal.md) | Exact decimal math for money |
| [JEV functions](jev.md) | Asking a hosted model from formulas |
| [Functions](../reference/functions.md) | Every function, generated from the engine |
