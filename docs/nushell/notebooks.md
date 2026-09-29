---
title: "Notebooks"
sidebar_position: 3
---

# Notebooks

A notebook is a tab of the workbook, beside its sheets, that holds cells
rather than a grid: code cells of nushell, each with its output under it,
and note cells of Markdown. You run the cells you choose, in the order
you choose, as in Jupyter; any output can go to a sheet, where formulas,
charts and pivots read it and it follows the cell's next run.

```nu
files = ls | where type == file
$files | where size > 1kb | sort-by size --reverse
```

![A notebook: a cell reads a CSV as sales, a second reads it as $sales, its output is sent to a sheet and summed there, and with the notebook reactive, editing sales runs both again and the sum follows](../media/notebook.gif)

Open one from a shell with `012 nu`, or from nushell with `sheet nu`
(`sheet` comes from 012's nushell module: [Install the `sheet`
command](README.md#install-the-sheet-command)):

```sh
012 nu              # a new notebook, its first cell ready to type in
012 nu work.012     # the workbook's notebook, made if it has none
```

In any workbook, **Data > Shell > Open notebook** (or the palette)
shows the workbook's notebook, or adds one after the sheet shown. A
notebook's tab is marked `❯`. Cells run `nu` as a separate process, so
[nushell](https://www.nushell.sh/book/installation.html) must be
installed; without it, a cell's output says so and the rest of 012
works as before.

## Cells

The tab looks and works like Jupyter's: a toolbar over the cells, a
code cell's pipeline in a box with its prompt at the left and a `▶` that
runs it at the right, its output under it, and note cells drawn as text:

```
 ▶ Run  ■ Stop  ↻ Restart  ▶▶ Run all  │  + Add  ✂ Cut  ⧉ Copy  ⎘ Paste  │  Code ▾   nu ○ idle
 big: reads $files; read as $big, nu.big in formulas ──────────────── Enter  edit
            Files
            What's big in the repo, from ls.

▌         ╭─ big ───────────────────────────────────────────── ✓ <1s ─╮
▌    [2]: │ big = $files | where size > 1kb                            │ ▶
▌         │   | sort-by size --reverse                                 │
▌         ╰────────────────────────────────────────────────────────────╯
▌ Out[2]:   name       size
▌           README.md  9.8 kB
▌           012        4.2 kB
```

The prompt says how many runs came before this one (`[2]:`, `[*]:` while
it runs, `[ ]:` before it has), and `Out[2]:` marks the output of that
run. The box's top border names the cell and says at its right how its
run stands, `saved` for an output read from the file:

```mermaid
stateDiagram-v2
    notrun: not run
    waiting: waiting
    running: running
    done: ✓ and how long it took
    failed: failed, and nu's message under it
    stale: stale
    [*] --> notrun
    notrun --> waiting: Shift+Enter, Ctrl+Enter, F9
    waiting --> running: the cell before it ends
    running --> done
    running --> failed: an error, or ii
    done --> stale: a cell it reads runs again, or its source changes
    stale --> waiting: run again
    failed --> waiting: run again
    done --> waiting: run again
```

A source is shown whole: a line too long for the screen wraps before a
pipe where it can, its next rows indented.

Outputs are drawn by what they are, with values formatted as cells
([Types](types.md)):

| Output | Shows |
|---|---|
| A table (a list of records) | Its columns fitted to their values, numbers right-aligned, under its header |
| A record | Its fields, one a line: `key  value` |
| A list | Its items, numbered from 0, as nushell numbers them |
| Text | Its lines, wrapped |
| A value | As a cell shows it: `4.2 kB`, `9/27/2026 11:27:31` |
| An error | `×` and nushell's message, then its help line, without an `Out` prompt |

A long output scrolls in a window of its own, 10 rows high, so the cells
under it stay where they are: with the output selected, Up and Down
scroll it before they move on, the wheel scrolls the window under the
mouse, and its last line says which rows show (`rows 11 to 20 of 45`).
`O` shows every row instead, and again the window; `o` hides the output
to one line, and again shows it, as does a click left of it. **Data >
Shell > Clear output** clears the selected cells' outputs, **Clear
outputs** every one.

```mermaid
stateDiagram-v2
    window: a window of 10 rows, scrolled
    whole: every row
    hidden: one line, output hidden
    [*] --> window
    window --> whole: O
    whole --> window: O
    window --> hidden: o, or a click left of it
    whole --> hidden: o
    hidden --> window: o
```

**Enter** on an output, or a double click, opens it full-screen: arrows
(or `h` `j` `k` `l`) move, `s` sorts by the pointer's column and `S` in
descending order (again for the output's own order), `/` keeps the rows
holding what's typed, and Esc (or `◀ Back`) goes back. The output itself
doesn't change.

Note cells are Markdown: headings, **bold**, *italic*, `code`, links (the
terminal opens them), lists and quotes. They're drawn as text, without a
box or a prompt, and show their Markdown in a box only while edited.

## Keys

Like Jupyter, a notebook has two modes. In command mode (the mode
indicator says `NOTEBOOK`) keys act on cells; in edit mode (`EDIT`) they
type into the selected cell.

```mermaid
stateDiagram-v2
    [*] --> Command
    Command --> Edit: Enter, !
    Edit --> Command: Esc
    Edit --> Command: Shift+Enter or Ctrl+Enter runs
    Command --> Output: Enter on an output
    Output --> Command: Esc
```

A bar at the left marks the active cell: `▌`, blue in command mode and
green in edit mode, where the cell's box is drawn in heavy lines too, so
the mode reads without color. Shift+Up and Shift+Down (or `K` and `J`,
or Shift+click) select the cells passed over as well, marked `▎`, and
the commands below that act on "the cells" act on all of them: run,
delete, copy, cut, move, make notes or code, hide or clear outputs.

| Key | In command mode |
|---|---|
| Up, Down, `j`, `k` | Move between cells, stopping at each output; on an output, scroll its window first |
| Shift+Up, Shift+Down, `K`, `J` | Select the cells passed over too |
| Home, End, PgUp, PgDn | The first cell, the last, a screen up or down |
| Enter | Edit the cell; on an output, open it full-screen |
| Shift+Enter | Run the cells and select the next, adding one at the end |
| Ctrl+Enter, `r` | Run the cells, staying on them |
| Alt+Enter | Run the cells and add a code cell under them |
| F9 | Run every cell |
| `a`, `b` | Add a code cell above, below |
| `!` | Add a code cell below and edit it |
| `dd` | Delete the cells |
| `z` | Undo, bringing back deleted cells (as Ctrl+Z) |
| Alt+Up, Alt+Down (or Ctrl+Shift+Up, Ctrl+Shift+Down) | Move the cells up or down |
| `m`, `y` | Make the cells notes (Markdown), or code |
| `c`, `x`, `v`, `V` | Copy, cut the cells; paste below, above |
| `n` | Name the cell |
| `o`, `O` | Hide the output, or show it again; show every row, or the window again |
| `G` | Send the output to a sheet |
| Ctrl+G | Go to a cell by its number, name, code or heading |
| `ii` | Stop: kill the cell running, and forget those waiting |
| `00` | Restart: stop, clear every output, count runs from 1 |

| Key | In edit mode |
|---|---|
| Esc | Keep what's typed and go back to command mode |
| Shift+Enter, Ctrl+Enter, Alt+Enter | Run, as in command mode |
| Enter | A new line, as indented as the one before |
| Up, Down, Home, End | Move by the lines on screen, wrapped lines too |
| Ctrl+A, Ctrl+E | The start, the end of the line |
| Tab | Complete the word at the caret: a cell's `$name`, `$selection`, a linked file, then what nu completes there (commands, flags, paths); one completion goes in at once ([Writing a cell](#writing-a-cell)) |

A terminal without the kitty keyboard protocol sends Shift+Enter and
Ctrl+Enter as Enter ([Keys the terminal has to tell
apart](../reference/keys.md#keys-the-terminal-has-to-tell-apart)): there
Esc then `r` runs the cell, and Alt+Enter runs it and adds one under it.

Every action is a command, in **Data > Shell**, the palette and the
shortcuts (Ctrl+/), and File, Edit and the sheet tabs work as anywhere;
commands for a sheet's cells are off on a notebook's tab. Changes to
cells are undo steps, as any edit is.

### Toolbar and mouse

The toolbar runs the commands Jupyter's does: `▶ Run` (the cells, then
the next), `■ Stop`, `↻ Restart`, `▶▶ Run all`, `+ Add`, `✂ Cut`, `⧉ Copy`,
`⎘ Paste`, and `Code ▾` or `Markdown ▾`, which makes the cells either. Each
shows its key where the width allows. At the right, `nu ○ idle`, `nu ●
busy` with how many cells wait, or `nu ⊘ off` where cells don't run,
stands for Jupyter's kernel; `reactive` beside it says the notebook is.
While every cell runs, the view follows the cell running until you
scroll.

The mouse works as in JupyterLab: a click selects a cell and a click in
a code cell's box edits it with the caret where you clicked; `▶` runs the
cell and `■` stops it while it runs; a double click edits a note or opens
an output full-screen; the wheel scrolls an output's window, then the
notebook; right-click opens the cell menu. All of them are listed in
[Keys and mouse](../reference/keys.md#mouse).

### Moving around

**Ctrl+G** (Go to cell) lists every cell by its number, name and first
line, and the notes' headings, to jump to by typing any of them. **View
> Table of contents** lists the headings alone, indented by level, as
Jupyter's table of contents.

## Writing a cell

The cell being written is read by nu itself, as nushell's own prompt
reads what you type: once typing pauses, 012 asks `nu --ide-ast` what
each word is and colors it (commands, strings, variables, numbers,
keywords, operators; flags stay plain), and `nu --ide-check` what's
wrong. A problem is underlined with a curly line, and with the caret on
it the context line says what nu said:

```
  [ ]                                                        not run
│ $files | sort-by size --revrse
                        ~~~~~~~~
The `sort-by` command doesn't have flag `revrse`.
```

Tab asks `nu --ide-complete` too, after the notebook's own names, so it
completes flags, subcommands and paths as well as cells and commands.
nu doesn't know the variables a notebook binds, so it's asked about the
cell with `$name` for each named cell and linked file, `$selection` and
`$sheet` declared before it; a `$sheet.A1:C9` range reads as one
variable, and `name =` isn't part of what nu reads.

Questions to nu run in the background and never on each key: one
process for each question after a pause, at most two at a time, each
stopped once the text changes and after a second and a half. Until nu
answers, the cell shows 012's own highlighting, and what's unchanged
keeps nu's colors, so nothing flickers or moves. 012 falls back to its
own highlighting and names, without saying so, when:

- nu isn't installed, or is older than 0.100;
- nu timed out three times in a row (asked again in the next session);
- the cells couldn't run without asking: a file from another computer
  not yet trusted ([Saving and trust](#saving-and-trust)), `shell =
  off`, or [012 serve](../terminal/ssh.md#notebooks) without
  `serve-shell`.

nu reads the cell without your `config.nu` or its standard library, so
your own commands aren't known to it; they still run when the cell does
with `nu-config`.

## Running

Cells run in the background, one at a time, so the screen stays live:
the one running shows `[*]` and those after it `waiting`. **Data >
Shell** runs one cell, every cell (F9), the cells above the selected
one or it and those below; each runs after the cells it reads. A cell
that fails stops the rest.

Each cell runs as `nu --no-config-file -c`, so your aliases and custom
commands aren't there unless `nu-config` is on
([nushell's configuration](https://www.nushell.sh/book/configuration.html)).
A cell stops after `nu-timeout` (30 seconds unless set); `ii` stops it
sooner.

## Names and $name

A cell that starts `name =` gives its output that name; `n` names a
cell for you. Later cells read the output as `$name`, a nushell
variable holding the value, and formulas on any sheet as `nu.name` once
it's [sent to a sheet](#send-to-a-sheet):

```nu
files = ls
big = $files | where size > 1kb
$big | get name | str join ", "
```

A name is letters, digits and `_`, not starting with a digit or `__`;
`in`, `env`, `nu`, `it`, `selection` and `sheet` are taken. Two cells
can't share a name: the second says so and doesn't run until it's
renamed.

What a cell reads decides the order cells run in: each after the cells
it reads, and a cell that hasn't run first when one reading it runs. A
cell reading itself, directly or through others, doesn't run.

```mermaid
flowchart TD
    files["files = ls"] -->|$files| big["big = $files | where size > 1kb"]
    app["a linked file, app.csv"] -->|$app| errors["errors = $app | where status >= 500"]
    big -->|G sends it to a sheet| region["the region big, on a sheet"]
    region --> sum["=SUM(nu.big)"]
```

A cell reads the sheets two ways, each a table:

| In a cell | Is |
|---|---|
| `$selection` | The range selected on the sheet shown last before the notebook, with its first row as the header |
| `$sheet.A1:C9`, `$sheet.Sales!A1:C9`, `$sheet.'Q1 data'!B2:B40` | That range, of the sheet shown last or the sheet named |
| `$app` | A [linked file](../files/following.md) named `app`, its rows as they are when the cell runs |

They reach nu as NUON in a file it reads, never as text spliced into
the pipeline, so types survive ([Types](types.md)): sizes stay sizes,
dates stay dates.

### Stale outputs and reactive notebooks

Running a cell again makes the outputs of the cells that read it,
directly or through others, `stale`: they were made from what it printed
before. So is a cell's own output once its source is changed.

```mermaid
flowchart TD
    run["files runs again"] --> stale["big, and cells reading big: stale"]
    stale -->|reactive off| you["run them when you want"]
    stale -->|reactive on| again["they run again, each after what it reads"]
    you --> fresh["fresh outputs"]
    again --> fresh
```

**Data > Shell > Reactive notebook** (off unless turned on, saved
with the notebook) runs them again whenever a cell they read runs.

## Send to a sheet

`G` sends the selected cell's output to a sheet: a new sheet named after
the cell, or a cell of a sheet you pick. There it's a region named after
the cell, italic as a linked file's rows are, that formulas read as
`nu.name`, header row included (`=SUM(nu.big)`, `=VLOOKUP("go.mod",
nu.files, 3, FALSE)`), and charts and pivot tables use. A cell without a
name is given one first.

```mermaid
sequenceDiagram
    participant C as Cell big
    participant R as Region big on a sheet
    participant F as =SUM(nu.big)
    C->>R: G sends the output's rows
    R->>F: recalculates
    C->>C: runs again
    C->>R: the new rows replace the old
    R->>F: recalculates
```

Like an [array's spill](../formulas/arrays.md#spilled-cells), its cells
can't be typed over: **Data > Shell > Freeze output** turns them into
plain values, and **Remove output** takes them off the sheet. Undo takes
back sending it, and the rows come back from the cell's output whenever
undo brings the region back.

## Saving and trust

The file keeps the cells and their outputs as NUON
([format](../files/format.md#notebooks)), so opening it shows every
output without running anything; the heads say `saved`. An output larger
than `nu-save-cell-kb` (1 MB), or past `nu-save-notebook-kb` (8 MB) for
all of them, is left out: the cell shows `not saved; run to see`, and
saving says which.

Opening a file never runs its cells. Like
[macros](../sheets/macros.md#macros-from-other-computers), a file's cells
run as you, with your files, so a notebook saved on another computer
asks once before its cells run:

```mermaid
sequenceDiagram
    participant You
    participant N as The notebook
    participant Nu as nu
    You->>N: open a file saved on another computer
    N-->>You: its cells and saved outputs, nothing run
    You->>N: Shift+Enter, or F9
    N-->>You: Run this file's notebook cells?
    You->>N: Enter
    N->>Nu: the cells, one at a time
    Nu-->>N: their outputs
```

Cells you write in a notebook made here run without asking. The options,
in [Configuration](../reference/config.md#nushell-notebooks):

| Option | Default | Does |
|---|---|---|
| `shell` | `ask` | `ask` as above; `on` never asks; `off` runs no cells |
| `nu-timeout` | `30s` | Stops a cell that runs longer; `0` lets it run until `ii` |
| `nu-config` | `false` | Runs cells with your `config.nu` and `env.nu` |
| `nu-save-cell-kb`, `nu-save-notebook-kb` | `1024`, `8192` | How much of the outputs a file keeps |

[012 serve](../terminal/ssh.md#notebooks) shows notebooks and their
saved outputs, but runs no cells unless the server's `serve-shell` is on.

### Notebook sheets

A file with the notebook sheets of earlier versions of 012, whose
pipelines were typed at a prompt on the formula bar, opens with each of
those pipelines as a code cell of the workbook's notebook, named as the
region was. Their tables stay where they were on the sheet, as the
cells' outputs sent there, so formulas reading `nu.r1` keep working once
the cells run; a note on opening says which sheets were converted.
