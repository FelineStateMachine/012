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
at them follow. Ctrl+Shift+V pastes values only, keeping the destination's
formats.

Formats travel with the cells. A pasted or moved cell shows what its
source showed, whether the format was the cell's own or came from its
row, its column or the whole sheet, and pasting plain cells into a
currency column leaves them plain. Whole columns or rows (Ctrl+Space,
Shift+Space) copied and pasted at the top of a column, or the start of a
row, take their column or row formats along, so a pasted column is
currency all the way down, not only where it had data; cut and pasted,
they move them, leaving the source columns plain. Pasted anywhere else,
they paste as a block of the cells that hold something. A block cut and
pasted leaves the cells it came from plain, as Sheets does, even where
a formatted column or row crosses them. Vim's `yy` and `dd` copy whole
rows, so `p` and `P` bring their row formats along. Formulas reading
a column whose format changes, blank cells included, show the new format
at once (`=B5*2` shows currency when column B becomes currency). Copies also go to the
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
names, charts, freeze, filters, pivot tables, and adding, deleting,
renaming, moving and duplicating sheets. A multi-cell change is one step,
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

## Conditional formatting

Format > Conditional formatting opens the sheet's rules in a panel at the
right of the grid, as Sheets' sidebar: `+ Add rule` adds one over the
selection, Enter edits the highlighted rule, Del removes it and
Shift+Up/Down moves it. In a rule's form, Up/Down pick a line,
Left/Right change a choice, Space flips a check, text lines are typed
into, Enter saves and Esc goes back without saving. Every change is an
undo step.

A rule applies to ranges (`A2:A100`, or several: `A2:A9,C2:C9`) and is
one of:

- **Single color**: cells that meet a condition get a text color, a fill,
  and bold, italic, underline or strikethrough. The conditions are
  Sheets': is empty, is not empty, text contains, does not contain,
  starts with, ends with, is exactly (ignoring case), date is, is before,
  is after (a date, or today, tomorrow, yesterday), greater than, greater
  than or equal to, less than, less than or equal to, is equal to, is not
  equal to, is between, is not between, and custom formula is. A value
  may be a formula (`=B2`), and a custom formula (`=$C2>100`) is written
  for the first cell of the first range: its relative references move
  with each cell, as a copied formula's would.
- **Color scale**: every number in the ranges is shaded along two or three
  colors, from its minimum to its maximum; each point is the minimum or
  maximum value, a number, a percent of the way between them, or a
  percentile, and a midpoint is optional.

Rules are tried in order, and the first that applies to a cell formats
it, so a rule higher in the list wins. The pointer and the selection draw
over a rule's colors; its text styles show through them.

Colors are named (red, yellow, green, cyan, blue, magenta) rather than
picked from a color wheel, because each is one of the terminal's 16 ANSI
colors: the terminal's palette or the color scheme ([themes.md](themes.md))
decides what they look like, and a fill's text is drawn in whichever of
black or white reads on it. A text color that wouldn't read on a rule's
fill takes the fill's ink instead. Color scales blend the scheme's colors
(or the terminal's, as it reports them) and pick readable text for each
shade.

Rules only change how cells look, so they may go on a pivot table's
results. They follow inserted and deleted rows and columns, and a rule
whose cells are all deleted goes. Format > Clear conditional formats
takes the selection out of every rule.

## Data validation

Data > Data validation opens the sheet's validation rules in the same
panel; Insert > Dropdown opens it with a dropdown for the selection, and
Insert > Checkbox makes the selection checkboxes at once. The criteria:

| Criteria | Cells may hold | Shows |
|---|---|---|
| Dropdown | one of the items typed (`Yes, No, Maybe`), ignoring case | ▾ at the right of the cell |
| Dropdown (from a range) | one of the values in a range, on any sheet (`Lists!A1:A20`) | ▾ |
| Checkbox | TRUE or FALSE; a blank cell is unchecked | `[✓]` or `[ ]`, centered |
| Number | a number between, not between, equal to, greater than ... values | |
| Date | any date, or one on, after, before, between ... dates | |
| Text length | text whose length is between, less than ... | |
| Custom formula is | anything for which the formula (`=B2<=C2`) is TRUE | |

Alt+Down, or a click on the ▾, opens a dropdown's list under the cell:
type to search, Enter enters the item as if typed. Space, or a click on
the box, checks or unchecks the selected checkboxes (all checked when
the active one isn't, as in Sheets); Space anywhere else starts an entry
as usual. When the active cell has no dropdown, Alt+Down opens the
column's filter.

Each rule either **rejects** an invalid entry or **shows a warning**.
Rejected, the entry stays open with Sheets' message on the context line
("Invalid entry in C2: Input must be a number between 0 and 80"), for
fixing or Esc. Warned, it goes in, and the cell's text gets a dotted
underline; on the active cell the context line says why ("Invalid:
Input must be ..."). A rule's help text, when it has one, replaces its
own words there. A formula is checked by the value it computes. Blanks
are always valid.

Each cell has one rule: a rule added over cells that had another takes
them from it. Validation checks what's typed, picked from a dropdown and
entered by macros; pastes, fills and imports aren't refused, and what
they leave invalid is marked. Validation can't go on a pivot table's
results, which take no entries. Data > Remove data validation takes the
selection out of every rule.

Adding a checkbox or a dropdown changes no cells, so on a protected range
it doesn't ask first (on a protected sheet it does); checking a box or
picking an item asks as typing does ([protection](#protected-sheets-and-ranges)).

## Pivot tables

Data > Pivot table summarizes the selection, or the table around the
active cell, on a new sheet named `Pivot Table 1`, as Sheets' Insert >
Pivot table does. The data's first row names the fields. The pivot editor
opens docked at the right of the grid, with the results updating behind it:

- **Rows** group the data by a field's values, one row per value; with two
  or more, the groups nest and each outer group gets a subtotal row.
- **Columns** spread the groups across columns by a field's values; with
  two or more, each outer group gets a subtotal column (`East Total`)
  after its last column.
- **Values** summarize a field for each group: SUM, COUNTA, COUNT,
  COUNTUNIQUE, AVERAGE, MAX or MIN (Sheets' "Summarize by"), shown as they
  are or as a share of their row, column or grand total ("Show as").
  A new value is summed when its field holds numbers, and counted
  otherwise. Its header is Sheets' `SUM of Units` until you rename it.
- **Filters** leave out rows by a field's values or a condition, in the
  same picker as a filter's column.
- **Grand total row** and **Grand total column** add Sheets' totals, and
  turn the subtotal rows and columns on and off with them.

Up and Down pick a line. Space adds a field to the section it's on (pick
it from the fields of the data, type to narrow the list), opens a filter's
values, or flips a toggle; Space on the data range points at a new one on
its sheet. On a row or column field, Left and Right order its groups A to
Z, Z to A, or by a value's total, smallest or largest first, and
Shift+Up and Shift+Down move it before or after the others, which changes
how the groups nest. On a value, Left and Right change how it's
summarized, S how it's shown, and R (or F2) renames it on the context
line, the name heading its columns; an empty name goes back to Sheets'.
A value keeps its name when its summary changes. Del removes a field. Every change is an
undo step; Enter keeps them, Esc undoes them, and removes a pivot just
created. Data > Edit pivot table opens the editor again.

Groups follow Sheets: text ignoring case (`east` joins `East`), numbers
and dates by value, with the source's format, so dates show as dates.
Numbers and text never share a group: `1` and the text `'1` are two.
Groups sort numbers first, then text, booleans and errors, and blank
cells last; as in Sheets, the group of blank cells has a blank label,
and its subtotal is just `Total`. Sorting an outer column field by a
value's total orders whole outer groups, so their columns stay
together. Rows blank across the whole range are
left out, so a range can reach past the data. SUM, AVERAGE, MIN and MAX
keep the column's number format; an error in a summed column shows as that
error, as SUM would.

The results are live: any change to the data, typed, pasted, filled or
computed by a formula, recomputes the pivot, and rows or columns inserted
inside the range widen it. Formulas can read the results (`='Pivot Table
1'!B5`), and copying them pastes their values. The results themselves
can't be edited, as in Sheets: typing, pasting, clearing, formatting,
sorting or inserting rows or columns over them is refused with a note on
the context line. A pivot shows `#REF!` in A1, explained on the context
line, when its data's sheet is deleted or when its results would
overwrite a cell typed next to them; undo or moving the cell brings it
back. The sheet's columns widen to fit the results until you set a width
yourself.

## Frequency tables

Data > Frequency table (column stats), or Alt+Shift+F (VisiData's
Shift+F; plain Shift+F types an F), counts each value of the active
column of the table around the active cell on a new sheet, `Frequency of
<field>`: the value, how many rows have it and their percentage, most
frequent first, with a Grand Total. Blank cells are counted too. It is a
pivot table like any other, so it stays live, and the pivot editor
changes it.

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

## Notes

Insert > Note (Shift+F2, or Insert note on the right-click menu) adds a
note to the active cell, as Sheets' notes: plain text typed on the context
line, where Alt+Enter or Shift+Enter starts a new line (shown as ↵), and
an empty note deletes it. A cell with a note shows ▝ in its top-right
corner; while it is the active cell, the context line shows the note, and
while the mouse is over it, a box beside it does. Delete notes (on the
right-click menu) removes the notes of the selection.

A note belongs to its cell: clearing the contents (Del) keeps it, it moves
with the cell when rows or columns are inserted, deleted or sorted, and a
copy or a cut takes it along, as in Sheets. Ctrl+Enter fills the entry
into the selection and leaves each cell's note alone. Every change to a
note is an undo step. Pivot table results can't take notes.

## Protected sheets and ranges

Data > Protect sheets and ranges works as Sheets' protection with "Show a
warning when editing this range": it locks nothing, but an edit that
touches a protected range, or anything on a protected sheet, asks first on
the context line: Enter edits anyway, Esc cancels. The picker lists the
sheet's protections; its first rows protect the selection or the whole
sheet, with an optional description, Enter on a protection selects its
range, and Ctrl+D removes it. Protect range and Remove protection on the
right-click menu and in the palette do the same for the selection.

What asks: typing into a cell, commands that change cells (clear, paste,
fill, formats, sort, delete rows), pasting text from the terminal and the
fill handle. Inserting rows or columns moves a protected range without
asking, unless the whole sheet is protected. Macros that run don't ask, as
Sheets' scripts don't. Protected ranges follow inserted and deleted rows
and columns, save with the sheet, and adding or removing one is an undo
step.

