# 012

A spreadsheet for the terminal with a Lotus 1-2-3 look and Google Sheets
behavior, built on [Bubble Tea v2](https://github.com/charmbracelet/bubbletea).

Inside the grid it works the way Sheets does: typing replaces a cell, `=`
starts a formula, Enter and Tab move you on, Shift+arrows and the mouse
select, and Sheets' shortcuts do what you expect. Around the grid, the
control panel, the mode indicator and the character grid keep 1-2-3's look.
It is one pure-Go binary.

![Typing a small budget, pointing at cells in a formula, and watching totals recalculate](docs/media/first-steps.gif)

## Install

```sh
go install github.com/FelineStateMachine/012/cmd/012@latest
```

Requires Go 1.27. Or from a clone: `make build` puts the binary in `bin/012`.

```sh
012                  # a new sheet
012 budget.012       # open or create a sheet
012 sales.xlsx       # import .xlsx, .csv, .tsv, .sqlite, .parquet or Lotus .wk1
012 serve ~/sheets   # serve a directory over SSH, a 012 per session
```

F1 shows every shortcut, F10 or Alt+letter opens the menus, and Ctrl+K
searches every command. `012 config edit` opens the settings file.

## A short tour

- **Formulas as in Sheets**, with [Sheets' functions](docs/reference/functions.md),
  suggestions and argument hints, pointing at cells with the arrows or the
  mouse, named ranges, references between sheets, optional decimal
  arithmetic for money, and undo for everything
  ([formulas](docs/formulas/README.md)).
- **Data tools**: freeze, sort, filter, find and replace, conditional
  formatting, dropdowns and checkboxes, notes, protected ranges, and live
  pivot tables ([working with data](docs/sheets/README.md)).
- **Charts** that float over the grid and follow their data, drawn as real
  images in kitty, Ghostty and WezTerm and as text elsewhere
  ([charts](docs/sheets/charts.md)).
- **Files**: a diff-friendly JSON format, import from CSV, TSV, XLSX,
  SQLite, Parquet and Lotus 1-2-3, export to CSV, TSV, XLSX and SQLite
  ([files](docs/files/README.md)).
- **Macros**, recorded or written as Starlark scripts saved with the sheet
  ([macros](docs/sheets/macros.md)).
- **Made for terminals**: the mouse, hyperlinks, the system clipboard over
  SSH, your terminal's colors or any of hundreds of schemes, optional vim
  keys, and `012 serve` to reach your sheets over SSH
  ([keys](docs/reference/keys.md), [themes](docs/terminal/themes.md), [SSH](docs/terminal/ssh.md)).
- **JEV functions** ask TypeSafe's hosted model about your data from a
  formula ([JEV](docs/formulas/jev.md)).

| | |
|---|---|
| ![Menus and the command palette](docs/media/menus-palette.gif) | ![Inserting a chart that follows its data](docs/media/charts.gif) |
| Menus and the command palette (Ctrl+K) | Charts that float over the grid and follow their data |
| ![A task list with a color scale, a dropdown and checkboxes](docs/media/rules.gif) | ![A pivot table of sales by region and quarter](docs/media/pivot.gif) |
| Conditional formatting and data validation | Pivot tables, live |
| ![Freezing, sorting and filtering](docs/media/freeze-sort-filter.gif) | ![Find and replace](docs/media/find-replace.gif) |
| Freeze, sort and filter | Find and replace |
| ![JEV functions classifying reviews](docs/media/jev.gif) | ![Currency formats and totals](docs/media/formats-budget.png) |
| JEV functions in formulas | Formats detected as you type |

The recordings are [VHS](https://github.com/charmbracelet/vhs) tapes in
[`demos/`](demos), rendered by `make demos` in the Catppuccin Mocha
palette; 012 draws in your terminal's own colors by default
([themes](docs/terminal/themes.md)). Charts show as text here.

## More

- [Documentation](docs/README.md): every guide, for using 012 and for
  working on it
- [Roadmap](ROADMAP.md)
- Working on 012: `make build`, then `make check` before a push; see
  [testing](docs/contributing/testing.md) and [CLAUDE.md](CLAUDE.md)

Young and moving quickly. The file format is versioned and older files
keep loading; the Go packages are internal and may change at any time.

## License

[MIT](LICENSE)
