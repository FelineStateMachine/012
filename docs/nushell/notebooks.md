---
title: "Notebooks"
sidebar_position: 3
---

# Notebooks

A notebook is a sheet where you type nushell pipelines and each one's
table lands in the grid, named, typed and kept live. Written like a
REPL, kept like a spreadsheet.

```nu
sales = open sales.csv | where Region in [North South] | select Region Quarter Units
$sales | where Units > 250 | sort-by Units --reverse
```

![Two pipelines at the nu❯ prompt become regions, the second reading the first as $sales; giving sales a new command runs both again](../media/notebook.gif)

`012 nu` opens one, at its prompt:

```sh
012 nu              # a new notebook
012 nu work.012     # open one, or make it
```

In any workbook, **Data > Shell** (or the palette, or `!` on a notebook
sheet) opens the same prompt, writing into the workbook's notebook
sheet, or a new one named Shell 1 at the first command. On other sheets
`!` starts an entry, as typing does. Commands run `nu` as a separate
process, so [nushell](https://www.nushell.sh/book/installation.html)
must be installed; without it, the prompt says so and the rest of 012
works as before.

## The prompt

The formula bar becomes `nu❯`, and the mode indicator says `NU`:

| Key | Does |
|---|---|
| Enter | Run the line; the prompt stays open for the next |
| Tab, Shift+Tab | Complete the word at the caret: a region (`$r1`) or one of nu's commands, from the box under the context line; again for the next |
| Up, Down | The workbook's earlier lines, those starting with what's typed; the last 100 are saved with the file |
| Esc | Stop the command running; otherwise, back to the grid |

A name and `=` before the pipeline name the region; without one it's
`r1`, `r2` and so on:

```nu
sizes = ls | select name size
```

A name is letters, digits and `_`, not starting with a digit, as a
nushell [variable's](https://www.nushell.sh/book/variables.html) is;
`in`, `env`, `nu` and `it` are nushell's own. Typing a name that exists
gives that region a new command and runs it again, as does F2 on its
label.

## Regions

Each command's table is a region: a label line with its name and
command, dimmed and underlined as wide as the table, then the table,
its first row the column names. Regions stack down the sheet in the
order they ran, a row between them; when a table grows or shrinks, the
rows below it move, as if you'd inserted or deleted rows.

A region is still cells of the grid. Its values are
[typed](types.md) (file sizes in the Size format, durations, dates),
formulas read them, and you can format them, filter them
(**Data > Create a filter**), sort them (the sort is kept, so the table
stays sorted when it runs again) and chart them. Like an
[array's spill](../formulas/arrays.md#spilled-cells), they can't be
typed over: change the command, or freeze the region. Copies paste
values; downloads and XLSX hold the values; undo takes back a run (the
table it replaced comes back) or the region itself.

The context line says what's under the pointer: on a label, the keys
that refresh and edit it, and why it failed if it did; on its table,
which region the cell is part of. A label's end says what it's doing:
`Running…`, `waiting` for a region it reads, `failed`, `stopped`,
`not run`, or `no rows`.

**Data > Shell regions** acts on the region under the pointer:

| Command | Key | Does |
|---|---|---|
| Refresh region | Enter on its label | Runs its command again, then every region that reads it |
| Run all regions | F9 | Runs every region, each after the regions it reads |
| Edit region's command | F2 on its label | Opens its command at the prompt, to change it and run it again |
| Freeze region | | Turns its table into plain values you can edit, and stops running its command |
| Delete region | | Removes the region, its command and its table |
| Stop shell command | Esc | Stops the command running, and those waiting after it |

## Tables between regions

`$r1` in a command is region r1's table, and `$in` is the range
selected when the prompt was opened, on a sheet that isn't the notebook,
read again each time the region runs:

```nu
big = $r1 | where size > 1kb | sort-by size --reverse
$in | group-by region --to-table
```

Both reach nu as NUON in a file it reads, never as text spliced into
the command: `$r1` is a nushell variable holding a table, and `$in` is
the pipeline's input, as nushell's own
[`$in`](https://www.nushell.sh/book/pipelines.html#pipeline-input-and-the-special-in-variable)
is.

A region reading another depends on it. Refreshing a region runs it
again and then every region that reads it, directly or through others,
each after what it reads; a region it reads that hasn't run yet runs
first. A command reading itself, directly or through others, is
refused.

### Linked files

A [linked file](../files/following.md) is a region too, named after the
file ([The region](../files/following.md#the-region)), so commands read
`app.csv` as `$app`. A region reading it doesn't run as rows
arrive: refreshing it runs its command on the rows the file has now.
The linked file itself has no command to run.

```nu
errors = $app | where status >= 500 | select time path status ms
```

[A log file followed live](cookbook.md#a-log-file-followed-live) shows
it working.

### In formulas

Other sheets read a region by name with `nu.` in front, as a named range
of its table, header included: `=SUM(nu.big)`, `=VLOOKUP("go.mod",
nu.r1, 3, FALSE)`, `=COUNTIF(nu.app, 503)`. Formulas follow the table as
it grows, shrinks or moves, and show `#REF!` while it has none.

## Running

Commands run in the background, one at a time: the screen stays live,
and Esc stops the one running and those waiting after it. Each runs as
`nu --no-config-file -c`, so your aliases and custom commands aren't
there unless `nu-config` is on
([nushell's configuration](https://www.nushell.sh/book/configuration.html)).
A command stops after `nu-timeout` (30 seconds unless set), and keeps at
most `max-cells` cells, whole rows, saying how many it left out.

What a command prints is read as a table: a record is one row, a list
of values that aren't records one column named `value`, and a single
value that column with one row. When a command fails, its label says
so and the context line gives nushell's message.

## Saving and trust

The file keeps each region's command, name, place and what it reads,
not its table ([format](../files/format.md#regions)), and the lines
typed at the prompt. Opening a notebook never runs anything: its
regions say `not run`, and F9 runs them all, each after what it reads.

Like [macros](../sheets/macros.md#macros-from-other-computers), a
file's commands run as you, with your files, so a notebook saved on
another computer asks once before they run:

```
Run this file's shell commands?   Enter Run   Esc Cancel
```

What you type at the prompt runs without asking. The options, in
[Configuration](../reference/config.md#nushell-notebooks):

| Option | Default | Does |
|---|---|---|
| `shell` | `ask` | `ask` as above; `on` never asks; `off` runs nothing |
| `nu-timeout` | `30s` | Stops a command that runs longer; `0` lets it run until Esc |
| `nu-config` | `false` | Runs commands with your `config.nu` and `env.nu` |

[012 serve](../terminal/ssh.md#notebooks) shows notebooks but runs no
commands unless the server's `serve-shell` is on.
