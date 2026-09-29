---
title: "Command line"
sidebar_position: 6
---

# Command line

Every way to start 012, with the page that describes each. A file named
like a command (`get`, `diff`) opens as `./get`.

## With the screen

| Command | Does |
|---|---|
| `012 [flags] [file]` | Opens a `.012` workbook (created on saving when it doesn't exist), or imports another format: [Files](../files/README.md) |
| `012 [flags] -` | A table from standard input: [Pipelines](../nushell/pipelines.md) |
| `012 [flags] --pipe [--to nuon\|json\|csv\|tsv] [--send ask\|selection\|sheet] [file]` | On quitting, sends a table to standard output: [Pipelines](../nushell/pipelines.md) |
| `012 nu [flags] [file]` | A nushell notebook, at its prompt: [Notebooks](../nushell/notebooks.md) |
| `012 serve [flags] [dir]` | Serves the sheets in a directory over SSH: [Serving over SSH](../terminal/ssh.md) |

The flags are the options' flags, listed in [Configuration](config.md#options).

## Without the screen

| Command | Does |
|---|---|
| `012 get file.012 [ref]` | Writes a cell's value, or a range or sheet as a table: [Scripts](../files/scripts.md#get) |
| `012 set file.012 ref input [ref input ...]` | Types entries into cells and saves: [Scripts](../files/scripts.md#set) |
| `012 recalc file.012` | Recalculates, saves and lists the cells showing errors: [Scripts](../files/scripts.md#recalc) |
| `012 export file.012 out [ref]` | Writes the workbook, a sheet or a range in another format: [Scripts](../files/scripts.md#export) |
| `012 diff a.012 b.012` | Lists what changed, cell by cell: [Diff and merge in git](../files/git.md#012-diff) |
| `012 diff --textconv file.012` | Writes a workbook as lines, for git's textconv: [Diff and merge in git](../files/git.md#git) |
| `012 merge-driver base ours theirs [path]` | Merges theirs into ours, for git: [Diff and merge in git](../files/git.md#merging) |
| `012 nu --module`, `012 nu --install-module [--force] [path]` | Prints or installs the nushell module: [Nushell](../nushell/README.md#install-the-sheet-command) |
| `012 config [path\|edit\|default\|themes\|set-key\|delete-key]` | The settings: [Configuration](config.md#commands) |
| `012 version` | Prints the version |

`get`, `set`, `recalc`, `export`, `diff` and `merge-driver` take their
flags anywhere after the command's name, and `--help` prints their
usage. `--` ends the flags, so what follows is taken as it is
(`012 set book.012 -- A1 --`), and an argument such as `-5` is never a
flag.

| Flag | Commands | Does |
|---|---|---|
| `--format text\|csv\|tsv\|json\|nuon` | `get` | The form of what's written; text by default |
| `--input` | `get` | What was typed (a formula) rather than its value |
| `--no-header` | `get` | JSON and NUON columns named by their letters, the first row a record |
| `--force` | `set` | Sets cells in protected ranges |
| `--format kind`, `--table name` | `export` | The format when the file's extension doesn't say; SQLite's table |
| `--chart n\|title` | `export` | One chart of the sheet as a web page, by its number or title |
| `--notebooks` | `get`, `recalc`, `export` | Runs the notebooks' cells first, when the file is trusted here |
| `--trust` | with `--notebooks` | Runs the cells of a file saved on another computer |
| `--jev` | `get`, `recalc`, `export` | Asks JEV for the JEV functions' answers first |
| `--format text\|json\|nuon` | `diff` | The form of the changes; text by default |
| `--color auto\|always\|never` | `diff` | Colors the text; auto colors on a terminal or git's pager unless `NO_COLOR` is set |

## Exit status

| Status | Means |
|---|---|
| 0 | Done; for `diff`, the workbooks are the same |
| 1 | An error, said on standard error; for `recalc`, cells show errors (after saving); for `diff`, the workbooks differ; for `merge-driver`, conflicts; for `get`, `recalc` and `export` with `--notebooks`, a notebook cell failed (after writing) |
| 2 | The command was used wrongly (its usage is printed); for `diff` and `merge-driver`, trouble reading or merging the files |
