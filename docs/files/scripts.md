---
title: "Scripts"
sidebar_position: 6
---

# Scripts

Four commands read and change a `.012` workbook without the screen, for
shell scripts, cron jobs, Makefiles and nushell pipelines:

```sh
012 get budget.012 B7                        # a cell's value, as the sheet shows it
012 get budget.012 'Q3 plan'!A1:C9 --format csv
012 set budget.012 B7 '=SUM(A1:A6)' B8 1200  # type entries, then save
012 recalc budget.012                        # recalculate, save, list the errors
012 export budget.012 budget.xlsx            # what File > Download writes
```

None of them runs a program or reaches the network unless a flag asks:
see [Notebooks and JEV functions](#notebooks-and-jev-functions). The flags
and exit statuses of every command are in
[Command line](../reference/command-line.md).

## References

A reference names cells as a formula does, with the sheet in front when
it isn't the one shown when the file was saved:

| Reference | Names |
|---|---|
| `B7`, `A1:C9`, `A:A`, `$B$7` | Cells of the sheet shown when the file was saved |
| `Q3!B7`, `'Q3 plan'!A1:C9` | Cells of the sheet named, quoted as in formulas when the name has spaces (the shell needs quotes around it too) |
| `Sales` | A [named range](../formulas/references.md), or a [table](../sheets/tables.md) or notebook output, header row included, so JSON and NUON name its columns |
| `Sales[Amount]`, `Sales[[#Headers],[Amount]]` | A table's cells by a [structured reference](../formulas/references.md#tables-by-column-name), as a formula outside the table reads them: `Sales[Amount]` is the column's data, without its header |
| `Q3`, `Q3!` | The whole sheet, from A1 to its last cell with contents |

A reference that names nothing says what the workbook has: `no sheet
named "Q4" (sheets: Q1, Q2, Q3)`.

## get

`012 get file ref` writes the value of a cell, or a table of a range or
sheet (the sheet shown when there's no reference). `--format` picks the
form:

| Format | One cell | A range or sheet |
|---|---|---|
| `text`, the default | As the cell shows it: `$1,200.00`, `9/29/2026`, `#DIV/0!` | Aligned columns, numbers to the right |
| `csv`, `tsv` | One line | Rows as shown, in the workbook's [locale](../sheets/locale.md), as a CSV download |
| `json` | The value: a number, a string, `true`, `null` for a blank cell | A list of records named by the range's first row |
| `nuon` | The value with its nushell type: a date, a file size, a duration | A nushell table named by the range's first row |

JSON and NUON type values by their cells' formats as a
[NUON download](../nushell/types.md) does, and take the columns' names
from the range's first row, as a table in 012 has its header there.
`--no-header` names the columns by their letters instead (`A`, `B`) and
makes the first row a record too. A filter doesn't hide rows from `get`:
a script reads all the range holds.

`--input` writes what was typed rather than what it computes: a
formula, or an entry as the file stores it (numbers and dates in
en-US's form whatever the workbook's locale), so
`012 get budget.012 B7 --input` prints `=SUM(A1:A6)`.

In nushell, NUON comes back as a table with its types:

```nu
012 get budget.012 A1:C9 --format nuon | from nuon | where amount > 100
012 get budget.012 B7 --format nuon | from nuon      # a number, not a string
```

## set

`012 set file ref input [ref input ...]` types each input into its cell,
in order, as one change, and saves the file atomically: into a temporary
file beside it, renamed over it once complete. An empty input clears the
cell's contents, keeping its formatting and note, as Delete does. A file
that doesn't exist is created, as `012 budget.012` does.

Inputs are what the file stores, not what the sheet's locale types: `1.5`
and `=ROUND(A1, 2)` in every workbook, so a script means the same in a
de-DE workbook as in an en-US one. Entries that imply a format (`$1,200`,
`12%`, `2026-09-29`) set it, as typing them does.

`set` stops, and leaves the file as it was, on the first input a cell
can't take, naming the cell:

| Refused | Says |
|---|---|
| A formula that doesn't parse | `Sheet1!B7: Expected , or ) in SUM, at character 11 of =SUM(A1:A6` |
| An entry a validation rule rejects | the rule's help, as the context line would |
| A cell of an array's result, a pivot table, a linked file or a notebook's output | the same message as on the screen |
| A cell in a [protected range](../sheets/notes-protection.md) | `Sheet1!B7 is protected (B1:B9): --force sets it anyway` |
| A reference to a range, not one cell | `A1:B2 is the range A1:B2: name one cell` |

An entry a rule only marks invalid is set, with a warning on standard
error. An input that starts with `--` (a flag's shape) comes after `--`:
`012 set budget.012 -- A1 --`.

## recalc

`012 recalc file` recomputes every formula, saves the file (leaving it
untouched when nothing it stores changed) and lists each formula showing
an error, one a line, with what the screen's context line says about it:

```
Q3!B7  #DIV/0!  Division by zero in B6/C6
Q3!D2  #N/A  JEV functions need --jev and an API key to be answered
```

It exits 1 when any cell shows an error or a formula reads its own cell,
so `012 recalc budget.012 && deploy` stops on a broken sheet.

## export

`012 export file out [ref]` writes the workbook in another format, as
File > Download does: the format by `out`'s extension or `--format`
(`csv`, `tsv`, `xlsx`, `sqlite`, `json`, `nuon`, `html`). XLSX gets every
sheet; the others get the sheet shown, or `ref`'s range or sheet. A
[web page](README.md#web-pages) of a range draws the charts whose corner
is in it; `--chart` writes one chart alone, found by its number on the
sheet (1 for the first) or its title:

```sh
012 export budget.012 q3.html 'Q3 plan'!A1:F20
012 export budget.012 spending.html Q3 --chart 'Spending by month'
```
 SQLite writes
a table named by `--table`, replacing one of that name. Formulas are
written as their values where the format has no formulas, and a note on
standard error says how many. CSV, TSV, JSON and NUON leave out the rows
a filter hides, as a download does.

## Notebooks and JEV functions

A `.012` file stores what was typed and computes the rest, so a few
things need more than the file:

- **[Notebook](../nushell/notebooks.md) outputs** are kept in the file
  as their cells last left them, and these commands read them as saved,
  sent to sheets as on the screen; `get` notes on standard error when
  the sheet it reads has an output the file doesn't hold (never run, or
  too large to save). `--notebooks` runs every notebook's
  code cells first, each after the cells it reads, as F9 does; a cell
  that fails stops the rest of its notebook, and is reported. A cell
  reading `$selection` fails, having no selection to read.
- As on the screen, a notebook
  [saved on another computer](../nushell/notebooks.md#saving-and-trust)
  asks first: `--notebooks` refuses it unless `--trust` gives the answer
  Run would, and `recalc --notebooks --trust` saves that trust in the
  file as the screen does, so later runs don't need it. `shell = off` in
  the [configuration](../reference/config.md#shell) turns `--notebooks`
  off, `shell = on` runs every file's cells, each cell stops after
  `nu-timeout`, and saving keeps as much of the outputs as
  `nu-save-cell-kb` and `nu-save-notebook-kb` allow.
- **[Linked files](following.md)** are followed only on the screen; their
  regions are empty here.
- **[JEV functions](../formulas/jev.md)** ask a hosted model over the
  network, and show `#N/A` here unless `--jev` asks it, with the API key
  from the environment, the credential store or `jev-api-key-command`.

`get`, `recalc` and `export` take these flags; `set` computes values only
to check entries.
