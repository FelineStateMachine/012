---
title: "Install and run"
sidebar_position: 2
---

# Install and run

```sh
go install github.com/FelineStateMachine/012/cmd/012@latest
```

012 needs Go 1.27 to build and is one pure-Go binary. From a clone,
`make build` puts it in `bin/012`.

```sh
012                  # a new sheet
012 budget.012       # open a sheet, or create it when it doesn't exist
012 sales.xlsx       # import .xlsx, .csv, .tsv, .json, .nuon, .sqlite, .parquet or Lotus .wk1
012 - < sales.csv    # a table from standard input
012 serve ~/sheets   # serve a directory over SSH, a 012 per session
012 get budget.012 B7        # a cell's value, for scripts
012 diff old.012 new.012     # what changed, cell by cell
```

A file on the command line opens as File > Open would: a `.012` sheet, or
another format imported ([files](../files/README.md)). `012 -` and
`012 --pipe`, 012 as a stage in a pipeline, are in
[Pipelines](../nushell/pipelines.md), and `012 nu`, a nushell notebook,
in [Notebooks](../nushell/notebooks.md). `012 serve` is
described in [Serving over SSH](../terminal/ssh.md). `012 get`, `set`,
`recalc` and `export` work on a workbook without the screen
([scripts](../files/scripts.md)), and `012 diff` and `012 merge-driver`
compare and merge workbooks, in git too ([diff and merge in
git](../files/git.md)). Every command is in
[Command line](../reference/command-line.md).

`012 config` prints the settings in effect and `012 config edit` opens the
settings file ([configuration](../reference/config.md)). 012 draws in your
terminal's own colors by default; [themes](../terminal/themes.md) says
how to pick a color scheme. The [terminal](../terminal/README.md) page
lists which terminal features 012 uses where they're available.
