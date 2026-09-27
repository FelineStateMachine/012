# Keys and mouse

Inside the grid, 012 behaves like Google Sheets: if you know a Sheets
shortcut, try it. Every command is also in the menus (F10 or Alt+letter)
and the command palette (Ctrl+K), which show each command's key. F1 or
Ctrl+/ lists every binding, generated from the table the app uses, so it
never drifts from what the keys do. This page is the same list with the
mouse and the vim keymap added.

## Moving around

| Key | Action |
|---|---|
| Arrows | Move one cell |
| Ctrl+arrows, End | Jump to the edge of the data (End: rightwards) |
| Tab, Shift+Tab | Right, left |
| PgUp, PgDn | A screen up, down |
| Alt+PgUp, Alt+PgDn | A screen left, right |
| Home, Ctrl+Home, Ctrl+End | Column A, cell A1, the last used cell |
| Ctrl+G, F5 | Go to a cell, a range or a named range, on any sheet (`Sheet2!B3`) |

## Entering data

| Key | Action |
|---|---|
| Type | Replace the cell: `=` starts a formula, `'` forces text, and `$1,200`, `12%`, `9/26/2026` or `14:30` are numbers that keep their format ([formulas.md](formulas.md#what-you-type)) |
| Enter, Tab | Accept and move down or right; Enter goes back to the column a run of Tabs began in |
| Enter, F2, double-click | Edit the cell |
| Esc | Cancel the entry |
| Ctrl+Enter | Enter the same entry in every selected cell |
| Arrows, Shift+arrows | While typing a formula, after an operator: point at a cell, or a range |
| F4 | While typing a formula: cycle the reference at the caret through `A1`, `$A$1`, `A$1`, `$A1` |
| Up, Down; Tab, Enter | While suggestions for a function, range or sheet name show: pick one; insert it (a sheet as `Summary!`, then arrows point into it). Esc hides them |

## Selecting

| Key | Action |
|---|---|
| Shift+arrows, Ctrl+Shift+arrows | Extend the selection, a cell at a time or to the edge of the data; the status line shows Sum, Avg and Count |
| Ctrl+Space, Shift+Space | Whole columns, whole rows |
| Ctrl+A | The data around the active cell, then everything |
| Esc | Deselect |

## Changing the sheet

| Key | Action |
|---|---|
| Del, Backspace | Clear the selection; formatting stays |
| Ctrl+Z; Ctrl+Y, Ctrl+Shift+Z | Undo; redo |
| Ctrl+C, Ctrl+X, Ctrl+V | Copy, cut, paste; references adjust as in Sheets, and copies also go to the system clipboard |
| Ctrl+Shift+V | Paste values only |
| Paste in the terminal | Tab-separated or multi-line text fills a block of cells |
| Ctrl+D, Ctrl+R | Fill down, fill right; a series started in the top rows (1, 2 over blanks) continues |
| Ctrl+Alt+=, Ctrl+Alt+- | Insert rows above, delete the selected rows (columns when whole columns are selected) |

[data.md](data.md#copy-paste-and-fill) says how formats travel with
copies and which series the fill handle continues.

## Formatting

| Key | Action |
|---|---|
| Ctrl+Shift+1 | Number |
| Ctrl+Shift+2 | Time |
| Ctrl+Shift+3 | Date |
| Ctrl+Shift+4 | Currency |
| Ctrl+Shift+5 | Percent |
| Ctrl+Shift+6 | Scientific |
| Ctrl+B, Ctrl+I, Ctrl+U, Alt+Shift+5 | Bold, italic, underline, strikethrough |
| Ctrl+Shift+L, Ctrl+Shift+E, Ctrl+Shift+R | Align left, center, right |
| Ctrl+\ | Clear formatting |

The rest of the formats are in the Format menu.

## Sheets

| Key | Action |
|---|---|
| Ctrl+PgDn, Ctrl+PgUp, Alt+Right, Alt+Left | Next, previous sheet; while typing a formula, point into it to insert `Sheet2!A1` |
| Shift+F11 | New sheet |
| Alt+Shift+K | Go to a sheet by name |

## Data tools

| Key | Action | More |
|---|---|---|
| Ctrl+F, Ctrl+H | Find; find and replace | [data.md](data.md#find-and-replace) |
| Alt+Down | With a filter on (Data > Create a filter), open the column's filter: Space checks values, type to search, or pick a condition | [data.md](data.md#filter) |
| Alt+, Alt+. | Trace precedents, dependents: highlight the cells a formula reads, or the formulas that read the cell, and jump to the first; again for the next, Esc to go back (Excel's Ctrl+[ and Ctrl+], which terminals send as Esc) | [formulas.md](formulas.md#building-formulas) |
| Alt+Shift+F | Frequency table of the active column on a new sheet, as VisiData's Shift+F | [data.md](data.md#frequency-tables) |
| Shift+F2 | Add or edit the active cell's note; Alt+Enter or Shift+Enter starts a new line | [data.md](data.md#notes) |

Tools with no key of their own have their keys on screen while they're
open, and in their docs: sorting by several columns and pivot tables
([data.md](data.md#sort), [pivot tables](data.md#pivot-tables)),
protected ranges ([data.md](data.md#protected-sheets-and-ranges)), and
charts ([charts.md](charts.md)). In Data > Named ranges, Enter goes to a
range, F2 renames or repoints it and Ctrl+D deletes it.

## Files, menus and help

| Key | Action |
|---|---|
| Ctrl+S, Ctrl+O, Ctrl+Q | Save, open (and import other formats), quit |
| F10, Alt+F, Alt+E, Alt+V, Alt+I, Alt+O, Alt+D, Alt+H | Open a menu; arrows move, Enter runs, Esc closes |
| Ctrl+K, Alt+/, Ctrl+Shift+P | Search the menus: find and run any command |
| Shift+F10, right-click | The cell, column or row menu |
| F1, Ctrl+/ | Keyboard shortcuts |

## Macros

| Key | Action |
|---|---|
| Ctrl+Alt+Shift+0 to 9 | Run the macro with that shortcut ([macros.md](macros.md#running)) |
| Esc | Stop a macro while it runs (the mode indicator says CMD) |

## Vim keys

File > Settings > Vim keys (also `:settings.vim` or the palette) turns
on a vim keymap in the spirit of sc-im and VisiData. It is off by
default and belongs to you, not the sheet: it's kept in the config file
as `keymap = vim` ([config.md](config.md#keymap)), never in `.012` files.
The mode indicator says **NORMAL** where it would say READY, **VISUAL**
while selecting with `v` or `V`, and **COMMAND** on the `:` line; typing
into a cell is ENTER or EDIT, as always, and Enter, Tab or Esc there
return to NORMAL. Keys vim doesn't take keep their Sheets meaning
(arrows, Del, Ctrl+S, Ctrl+C and Ctrl+V, Alt+letter menus, F-keys);
Ctrl+D, Ctrl+U and Ctrl+R are vim's, so fill down and fill right are in
the Edit menu. F1 lists the vim keys first, and the menus and palette
show them where Sheets' keys are taken.

| Keys (NORMAL) | Action |
|---|---|
| `h` `j` `k` `l`, arrows | Left, down, up, right |
| `w` `b` | Jump to the edge of the data, right or left (as Ctrl+arrows) |
| `gg` `G` | First row, last row with data; with a count, that row (`12G`) |
| `0` `^` `$` | Column A, the last filled cell of the row |
| `H` `M` `L` | Top, middle, bottom row on screen |
| Ctrl+D, Ctrl+U | Half a screen down, up |
| A count (`5j`, `3dd`, `4x`) | Repeats a move or command, or says how many rows or cells an operator takes |
| `i` `a`, Enter | Edit the cell, caret at the start or the end |
| `=` | Start a formula |
| `o` `O` | Insert a row below or above and start typing in it |
| `x` | Clear the cell (with a count, that many to the right) |
| `dd` `yy` | Cut or copy the row (with a count, that many rows) |
| `p` `P` | Paste rows cut or copied with `dd` or `yy` as new rows below or above, with their row formats; other copied cells paste at the active cell |
| `u`, Ctrl+R | Undo, redo |
| `v` `V` | Select cells or whole rows (VISUAL); motions stretch the selection |
| `/`, `n` `N` | Find (Enter stays on the match), next and previous match |
| `gt` `gT` | Next, previous sheet |
| `:` | The command line |
| Esc | Cancel a count or sequence, leave VISUAL, deselect |

| Keys (VISUAL) | Action |
|---|---|
| `d` `x` | Delete the selection (whole rows with `V`, the cells' contents with `v`), keeping a copy to paste |
| `y` | Copy the selection |
| `p` | Paste over the selection |
| `o` | Go to the other corner |
| `:` | The command line, acting on the selection |
| `v` `V`, Esc | Back to NORMAL |

The `:` line takes:

| Command | Action |
|---|---|
| `:B12`, `:Sheet2!A1`, `:C3:D9`, `:Sales` | Go to a cell, range or named range |
| `:40` | Go to row 40 |
| `:w`, `:w name`, `:w out.csv` | Save, Save as, or Download as another format, as the File menu does |
| `:q`, `:q!` | Quit (asking about unsaved changes), quit discarding them |
| `:wq`, `:x` | Save and quit; `:x` saves only if something changed |
| `:e name`, `:e!` | Open a sheet or import a file; refused with unsaved changes unless `:e!` |
| `:edit.fill_down`, `:fill down` | Any command, by its id or title |

Completions for commands appear under the line as you type, drawn from
the same registry as the menus and palette: Tab puts the highlighted
one on the line (Tab again for the next, Shift+Tab back), Up and Down
move, and Enter runs the line, or the highlighted completion when the
line isn't a command itself (`:fill d` Enter fills down).

## Mouse

| Gesture | Action |
|---|---|
| Click, drag, Shift+click | Select a cell, a range, or extend the selection; dragging past the edge scrolls |
| Click a column or row header, or the corner | Select whole columns, rows, or everything |
| Double-click | Edit the cell |
| Drag the fill handle (▟ at the selection's corner, shown on hover) | Continue a series (1, 2, 3; Jan, Feb; Mon, Tue; dates; Item 1, Item 2) or copy |
| Drag a column header's right edge; double-click it | Resize the column; fit it to its contents |
| Right-click | The cell, column or row menu |
| Click `▾` in a header | Open the column's filter |
| Click a chart; drag it or its corner | Select it; move or resize it (then arrows move it, Shift+arrows resize, Enter edits, Del deletes) |
| Cmd- or Ctrl-click a link | Open a URL in a cell or a `=HYPERLINK(url, [label])` |
| Hover a cell with a note | Show the note beside it |

The sheet tabs at the left of the status line take the mouse too: click
a tab to show its sheet, double-click to rename it, right-click for its
menu (rename, duplicate, delete, hide, move left or right), drag it onto
another tab to move it there, and click `+` to add a sheet. When the
tabs don't all fit, `‹` and `›` step through them. While typing a
formula, clicking a cell or a tab points at it, as in Sheets.

The pointer changes shape over cells, resize handles and the formula bar
in terminals that support it (OSC 22). In Ghostty and xterm, Shift+click
normally starts the terminal's own text selection; 012 asks the terminal
to pass it through (XTSHIFTESCAPE) and restores that on exit.

## The screen

| Line | Shows |
|---|---|
| Menu bar (top) | The menus, and the mode indicator on the right (`REC` beside it while a macro records) |
| Formula bar | The name box, then the cell's contents or the entry being typed |
| Context line | Prompts, key hints, formula errors and explanations, and bars such as find and the chart editor |
| Status line (bottom) | The sheet tabs (the one shown highlighted), the file name, whether it's modified, and Sum, Avg and Count for a selection |
