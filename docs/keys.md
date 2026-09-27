# Keys and mouse

Inside the grid, 012 behaves like Google Sheets: if you know a Sheets
shortcut, try it. Everything is also reachable from the menus (F10 or
Alt+letter) and the command palette (Ctrl+K), which shows each command's
shortcut. F1 or Ctrl+/ lists every binding, generated from the same table
the app uses, so it never drifts from what the keys do.

Inside the grid, 012 works like Google Sheets.

| Key | Action |
|---|---|
| Type | Replace the cell. `=` starts a formula, `'` forces text; `$1,200`, `12%`, `9/26/2026` and `14:30` are numbers that keep their format |
| Enter / Tab | Accept and move down / right (Enter returns to where a run of Tabs began) |
| Enter or F2, double-click | Edit the cell |
| Arrows while typing a formula | After an operator, pick a cell; Shift+arrows pick a range |
| Shift+arrows, drag, Shift+click | Select; the status line shows Sum, Avg and Count |
| Ctrl+arrows | Jump to the edge of the data (add Shift to select) |
| Click a header, Ctrl+Space, Shift+Space | Select whole columns or rows |
| Ctrl+A | Select the data, then everything |
| Del / Backspace | Clear the selection (formatting stays) |
| Ctrl+Z, Ctrl+Y or Ctrl+Shift+Z | Undo, redo |
| Ctrl+C, Ctrl+X, Ctrl+V | Copy, cut, paste; references adjust as in Sheets, and copies also go to the system clipboard |
| Ctrl+Shift+V | Paste values only |
| Paste from the terminal | Tab-separated or multi-line text fills a block of cells |
| Ctrl+D, Ctrl+R | Fill down, fill right; top rows that start a series (1, 2 over blanks) continue it |
| Drag the fill handle | The ▟ at the selection's corner, shown on hover: continue a series (1, 2, 3; Jan, Feb; Mon, Tue; dates; Item 1, Item 2) or copy |
| View > Freeze | Keep rows or columns on screen while the rest scrolls |
| Data > Sort sheet, Sort range | Sort by the active column A to Z or Z to A, or pick columns and order on a bar (Left/Right column, Space order, Alt+A add, Alt+H header row) |
| Alt+Down, click ▾ in a header | With a filter (Data > Create a filter): pick the column's values (Space checks, type to search) or a condition |
| Ctrl+Alt+= / Ctrl+Alt+- | Insert rows above / delete the selected rows (columns when whole columns are selected) |
| Ctrl+Enter while typing | Enter the same entry in every selected cell |
| F4 while typing a formula | Cycle the reference at the caret through A1, $A$1, A$1, $A1 |
| Typing a function or range name | Suggestions drop down: Up/Down pick, Tab or Enter insert, Esc hides them; inside a function's parentheses the context line shows its arguments with the current one marked |
| Alt+, / Alt+. | Trace precedents / dependents: highlight the cells a formula reads, or the formulas that read the cell, and jump to the first; press again for the next, Esc to go back (Excel's Ctrl+[ and Ctrl+], which terminals send as Esc) |
| Data > Named ranges | Name ranges for formulas (`=SUM(Sales)`): Enter goes to one, F2 renames or repoints it, Ctrl+D deletes it; Data > Define named range names the selection |
| Ctrl+Shift+1 ... 6 | Number, time, date, currency, percent, scientific format |
| Ctrl+B, Ctrl+I, Ctrl+U, Alt+Shift+5 | Bold, italic, underline, strikethrough |
| Ctrl+Shift+L / E / R | Align left, center, right |
| Ctrl+\ | Clear formatting |
| Ctrl+S, Ctrl+O, Ctrl+Q | Save, open (imports other formats), quit |
| Ctrl+G or F5 | Go to a cell, a range or a named range, on any sheet (`Sheet2!B3`) |
| Ctrl+PgDn / Ctrl+PgUp, Alt+Right / Alt+Left | Next / previous sheet; while typing a formula, point into it to insert `Sheet2!A1` |
| Shift+F11 | New sheet (also Insert > Sheet) |
| Alt+Shift+K | Go to a sheet by name |
| Alt+F, Alt+E, Alt+V, Alt+I, Alt+O, Alt+H, F10, click a title | Open a menu (arrows move, Enter runs, Esc closes) |
| Ctrl+K, Alt+/, Ctrl+Shift+P | Search the menus: find and run any command |
| Right-click, Shift+F10 | Cell, column or row menu |
| F1 or Ctrl+/ | Keyboard shortcuts |
| Insert > Chart | Chart the selection (or the table around the active cell); the editor bar picks the type with Left/Right, S switches rows and columns, H and L toggle the header row and labels, R changes the range, T the title |
| Click a chart, then Arrows / Shift+arrows / Del | Move, resize or delete it; drag the chart or its corner with the mouse; Enter edits it |
| Cmd- or Ctrl-click a URL | Open it: cells holding a URL, and `=HYPERLINK(url, [label])`, are terminal hyperlinks |

## Mouse

Click a cell to select it, drag to select a range, Shift+click to extend,
and click a column or row header (or the corner) to select whole columns,
rows or everything. Double-click edits. Drag a column header's right edge
to resize it; double-click the edge to fit the contents. Dragging past the
edge of the grid scrolls. Right-click opens the cell, column or row menu.
The mouse pointer changes shape over cells, resize handles and the formula
bar in terminals that support it (OSC 22).

The sheet tabs at the left of the status line take the mouse too: click a
tab to show its sheet, double-click to rename it, right-click for its menu
(rename, duplicate, delete, move left or right), drag it onto another tab to
move it there, and click `+` to add a sheet. When the tabs don't all fit,
`‹` and `›` step through them. While typing a formula, clicking a tab points
into that sheet, as in Sheets.

In Ghostty and xterm, Shift+click normally starts the terminal's own text
selection; 012 asks the terminal to pass it through (XTSHIFTESCAPE) and
restores that on exit.

## The screen

The top three lines are the **menu bar** with the mode indicator on the
right, the **formula bar** (name box, then the cell's contents or the entry
being typed) and the **context line**, which holds prompts, key hints,
formula errors and explanations. The bottom line is the **status line**:
the sheet tabs (as tmux lists its windows, the sheet shown highlighted),
the file name, whether it's modified, and Sum, Avg and Count for a
selection.
