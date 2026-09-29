---
title: "Nushell"
sidebar_position: 1
---

# Nushell

[Nushell](https://www.nushell.sh/book/) is a shell whose pipelines carry
tables: `ls`, `ps`, `open data.csv` and `http get` all hand the next
command rows and columns, with file sizes, durations and dates as values
of their own type. 012 is a spreadsheet for those tables. A table can go
through 012 in the middle of a pipeline, and nushell can run inside 012,
each pipeline's table a live part of the sheet.

![In nushell, ls's table goes through 012, three of its rows are selected and sent on, and nu keeps filtering them by size](../media/pipeline.gif)

## Three ways to use them together

| Way | Looks like | For |
|---|---|---|
| [A stage in a pipeline](pipelines.md) | `ls \| sheet \| where size > 1kb` | Looking at a table on the terminal, editing it, and sending it on to the next command |
| [A notebook](notebooks.md) | `012 nu`, then `ls \| select name size` at the `nu❯` prompt | Pipelines you keep: each one's table is a named region of the sheet that formulas, charts and other pipelines read, run again when you ask |
| [A followed file](../files/following.md) | Data > Linked file, then `$app \| where status >= 500` | A log or export that keeps growing: its rows come in as they're written, and pipelines read the rows it has now |

The first needs nothing but nushell. The other two run `nu` from inside
012, so it has to be installed ([Installing
nushell](https://www.nushell.sh/book/installation.html)); without it the
prompt says so, and the rest of 012 works as before.

## Install the `sheet` command

012 comes with a nushell module, `012.nu`, whose `sheet` command runs
012 in a pipeline. Write it into nushell's `scripts` folder:

```nu
^012 nu --install-module
```

and add the line it prints to `config.nu` (`config nu` opens it):

```nu
use scripts/012.nu *
```

In a new nushell, `help sheet` describes it:

| Command | Does |
|---|---|
| `sheet` | Opens the table piped in (or a file: `sheet budget.xlsx`) and returns what you send back, with its types ([Pipelines](pipelines.md#the-sheet-command)) |
| `sheet view` | Opens the table piped in and returns nothing |
| `sheet nu` | Opens a [notebook](notebooks.md), as `012 nu` |

`^012 nu --install-module` refuses to replace a different file already
there unless given `--force`; given a path, it writes the module there
instead. `^012 nu --module` prints the module, to read or to put
somewhere yourself.

Without the module, 012 is run as `^012`: in nushell, a word that
starts with a digit is a number, and the caret calls an external
command by name ([Running external
commands](https://www.nushell.sh/book/running_externals.html)). Other
shells run it as `012`.

## A 30-second tour

```nu
# A table in, edited, and out again, with its types
ls | select name size | sheet | where size > 1kb

# A notebook: each pipeline's table lands in the grid
sheet nu work.012
```

At the notebook's `nu❯` prompt:

```nu
sales = open sales.csv | update Revenue { str replace -ar '[$,]' '' | into float }
$sales | group-by Region --to-table | update items { get Revenue | math sum }
```

The second region reads the first as `$sales`; typing a new command for
`sales` runs it and then the region that reads it. In a formula on any
sheet, `=SUM(nu.sales)` reads the same table.

## Pages

| Page | For |
|---|---|
| [Pipelines](pipelines.md) | `sheet`, `012 -` and `012 --pipe`: tables in on standard input and out on standard output, sending back, the exit status |
| [Notebooks](notebooks.md) | `012 nu` and the `nu❯` prompt: regions, `$r1` and `$in`, `nu.r1` in formulas, refreshing, saving and trust |
| [Types](types.md) | How each nushell type becomes a cell and goes back, and what doesn't survive the trip |
| [Cookbook](cookbook.md) | Worked examples: disk usage, a log followed live, a CSV cleaned and summarized, an API, processes, git history |

Nushell's own [book](https://www.nushell.sh/book/) explains
[pipelines](https://www.nushell.sh/book/pipelines.html),
[tables](https://www.nushell.sh/book/working_with_tables.html) and
[types](https://www.nushell.sh/book/types_of_data.html); the pages here
link to the parts they build on.
