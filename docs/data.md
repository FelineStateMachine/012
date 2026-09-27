# Working with data

## Selection

Shift+arrows, a mouse drag or Shift+click select a range; Ctrl+arrows jump
to the edge of the data (add Shift to select as you go); Ctrl+A selects the
data around the active cell, then everything; Ctrl+Space and Shift+Space
select whole columns and rows. The active cell stays distinct from the rest
of the selection, and the status line shows Sum, Avg and Count, dropping the
ones that don't fit on a narrow terminal.

## Copy, paste and fill

Ctrl+C, Ctrl+X and Ctrl+V work as in Sheets: relative references shift,
absolute ones don't, cut and paste moves cells and the formulas that point
at them follow. Ctrl+Shift+V pastes values only. Copies also go to the
system clipboard as tab-separated text (OSC 52, so it works over SSH), and
pasting tab-separated or multi-line text from the terminal fills a block.

Ctrl+D and Ctrl+R fill down and right. The fill handle, shown when you hover
the corner of the selection, can be dragged to continue a series: 1, 2, 3;
2, 4, 6; Jan, Feb; Mon, Tue; dates by day or month; Item 1, Item 2. Ctrl+Enter
while typing enters the same entry into every selected cell.

## Undo

Ctrl+Z undoes and Ctrl+Y or Ctrl+Shift+Z redoes, 100 steps deep, across
every sheet: undo shows the sheet a step changed. Every change is covered:
typing, clearing, paste, fill, sort, insert and delete, formats, widths,
names, charts, freeze, filters, and adding, deleting, renaming, moving and
duplicating sheets. A multi-cell change is one step,
and the context line says what was undone, e.g. `Undid: clear B3:B5`.

## Rows and columns

Insert and delete rows and columns from the Insert and Edit menus, the
right-click menus, or Ctrl+Alt+= and Ctrl+Alt+- (rows, or columns when whole
columns are selected). Formulas, names, charts, filters and widths follow.
Drag a column header's edge to resize it, or use Format > Column width.

## Freeze

View > Freeze keeps rows or columns on screen while the rest scrolls, marked
by thin divider lines. The mouse and keys treat the frozen area as Sheets
does.

## Sort

Data > Sort sheet sorts by the active column A to Z or Z to A, keeping frozen
rows on top. Data > Sort range sorts the selection, or the block of data
around the active cell; its advanced option opens a bar to sort by several
columns (Left/Right picks a column, Space flips the order, Alt+A adds a
column, Alt+H toggles the header row). The order follows Sheets: numbers,
then text ignoring case, then booleans, then errors, with blanks last.

## Filter

Data > Create a filter puts a filter on the selection or the table around
the active cell; headers show `▾`. Alt+Down or a click on `▾` opens the
column's filter: check values in a searchable list, or pick a condition such
as "greater than" or "text contains". Filtered-out rows are hidden, not
deleted: row numbers show the gap, navigation skips them, formulas still
count them, and the status line says how many rows are hidden.

## Find and replace

Ctrl+F opens a find bar on the context line: matches highlight as you type,
the active cell follows the current one, and Enter and Shift+Enter step
through them. Ctrl+H adds a replacement field; Enter replaces and moves on,
Ctrl+Enter replaces all as one undo step. Chips toggle match case (Alt+C),
whole cell (Alt+W), regular expressions (Alt+R, with `$1` in replacements),
searching formulas (Alt+=). The scope chip says where to search, as Sheets'
"Search" choice: this sheet, all sheets, or the range selected when the bar
opened; Alt+S goes through them. Searching all sheets steps from sheet to
sheet in tab order, and replacing all across sheets is still one undo step.
