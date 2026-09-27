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
012 sales.xlsx       # import .xlsx, .csv, .tsv, .sqlite, .parquet or Lotus .wk1
012 serve ~/sheets   # serve a directory over SSH, a 012 per session
```

A file on the command line opens as File > Open would: a `.012` sheet, or
another format imported ([files](@/files/README.md)). `012 serve` is
described in [Serving over SSH](@/terminal/ssh.md).

`012 config` prints the settings in effect and `012 config edit` opens the
settings file ([configuration](@/reference/config.md)). 012 draws in your
terminal's own colors by default; [themes](@/terminal/themes.md) says
how to pick a color scheme. The [terminal](@/terminal/README.md) page
lists which terminal features 012 uses where they're available.
