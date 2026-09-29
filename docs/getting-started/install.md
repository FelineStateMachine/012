---
title: "Install and run"
sidebar_position: 2
---

# Install and run

012 is one binary. On macOS or Linux, the install script puts the latest
release in `~/.local/bin`:

```sh
curl -fsSL https://f58b.n.zip/install.sh | sh
```

It picks the archive for your OS and architecture, checks it against the
release's `SHA256SUMS` and says where it put `012`. `PREFIX=/opt/012`
installs into `/opt/012/bin` instead, and `sh -s -- --system` into
`/usr/local/bin`, running `sudo` (and printing the command first) only
when that directory isn't writable; `sh -s -- --version v0.3.0` picks a
release. On Windows, in PowerShell:

```powershell
irm https://f58b.n.zip/install.ps1 | iex
```

which installs `012.exe` into `%LOCALAPPDATA%\Programs\012\bin` and adds
that folder to your PATH, without administrator rights. The archives and
`SHA256SUMS` are also at
[f58b.n.zip/releases/latest/](https://f58b.n.zip/releases/latest/) to
download by hand.

With Go 1.27 or later, build it from source instead:

```sh
go install github.com/FelineStateMachine/012/cmd/012@latest
```

From a clone, `make build` puts it in `bin/012`.

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
