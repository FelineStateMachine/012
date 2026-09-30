---
name: 012
description: Read, change, check and share 012 spreadsheet workbooks (.012 files) from the shell with the 012 command. Use when a task involves a .012 file, or when a spreadsheet, table of numbers or chart should be built, inspected, recalculated or exported (CSV, XLSX, JSON, NUON, SQLite, HTML).
---

# Working with 012 workbooks

012 is a terminal spreadsheet. Its `.012` files are workbooks of sheets,
formulas, tables, charts and nushell notebooks. Work on them with the
`012` command, never by editing the file's JSON: `012 set` checks every
entry as the screen does and saves atomically, and its changes show in
`012 diff` like a person's.

## Look before you read

```sh
012 describe book.012                  # sheets, used ranges, header rows, tables, names, charts, notebooks
012 describe book.012 --format json    # the same as JSON
```

Read what describe points at rather than whole sheets.

## Read

```sh
012 get book.012 B7                          # one cell, as shown: $1,200.00
012 get book.012 B7 --input                  # what was typed: =SUM(B2:B6)
012 get book.012 'Q3 plan'!A1:F20 --format json   # records named by the first row
012 get book.012 Sales --format json         # a table or named range by name
012 get book.012 'Sales[Amount]' --format json    # one column of a table
```

References are written as in formulas: `B7`, `A1:C9`, `A:A`, `Q3!B7`,
`'Q3 plan'!A1:C9`, a named range, a table (`Sales`, `Sales[Amount]`) or a
sheet name for the whole sheet. Without a sheet, the sheet shown when the
file was saved is meant. `--no-header` names JSON columns by letter.

## Change

Preview first, then write:

```sh
012 set book.012 B7 '=SUM(B2:B6)' B8 1200 --dry-run   # prints the change as 012 diff would; saves nothing
012 set book.012 B7 '=SUM(B2:B6)' B8 1200             # types the entries, as one change, and saves
012 set book.012 B7 '=SUM(B2:B6)' --format json       # {file, saved, changes, warnings}
```

- Inputs are what a person types, in en-US form whatever the workbook's
  locale: `1.5`, `=ROUND(A1, 2)`, `$1,200` (sets a currency format),
  `12%`, `2026-09-29`. An empty input clears a cell.
- Formulas are Google Sheets' (`SUM`, `XLOOKUP`, `FILTER`, `LET`,
  `LAMBDA`, structured references like `Sales[Amount]`).
- `set` refuses, and changes nothing, on the first entry a cell can't
  take: a formula that doesn't parse, a validation rule, a protected
  range (`--force` overrides only that), or a cell of a spill, pivot
  table or notebook output. Read the message; it names the cell.
- An input starting with `--` goes after `--`: `012 set book.012 -- A1 --`.

## Check

```sh
012 recalc book.012                 # recalculates, saves, lists cells showing errors; exit 1 if any
012 recalc book.012 --format json   # {file, errors: [{cell, value, why}], circular, notebook_failures}
012 diff old.012 new.012 --format json
```

Run `012 recalc` after changing formulas and fix what it lists.

## Share

```sh
012 export book.012 out.xlsx                 # every sheet, with formulas and formats
012 export book.012 out.csv 'Q3'!A1:F20      # values as shown
012 export book.012 report.html              # a web page in 012's look, charts as SVG
012 export book.012 chart.html --chart 1     # one chart alone, by number or title
```

## Safety

Nothing runs a program or reaches the network unless a flag asks:
`--notebooks` runs the workbook's nushell notebook cells (and
`--trust` a workbook saved on another computer), `--jev` asks the JEV
model service. Don't add them unless the user wants that.

## Exit status

0 done; 1 an error on standard error (or, for `recalc`, cells showing
errors; for `diff`, the workbooks differ); 2 the command was used
wrongly, with its usage. Every command's `--help` prints its usage.

## The MCP server

To give Claude Code, Codex or Claude Desktop 012's tools, install
the server once, for every workbook:

```sh
012 agent --install-mcp codex      # or claude-code, or claude-desktop
012 agent --install-mcp codex --print   # show the entry without writing it
```

It writes the host's entry to run this 012's `mcp` by its full path,
with no workbook: each tool takes a workbook's `path`, inside the
host's project folders or the folders `--root dir` adds. Don't name a
file in the entry (`012 mcp demo.012`), which ties the server to that
workbook, and don't edit the host's configuration by hand. Restart the
host afterwards.

When the server is connected, its tools do what the commands do, with
the same checks: describe without a path lists the workbooks, then
describe, read_range, write_cells with dry_run, evaluate, list_errors
and create_workbook, each given the workbook's path.

The full reference: https://github.com/FelineStateMachine/012/blob/main/docs/agents/README.md
