---
title: "Conditional formatting and data validation"
sidebar_position: 7
---

# Conditional formatting and data validation

## Conditional formatting

Format > Conditional formatting opens the sheet's rules in a panel at the
right of the grid, as Sheets' sidebar: `+ Add rule` adds one over the
selection, Enter edits the highlighted rule, Del removes it and
Shift+Up/Down moves it. In a rule's form, Up/Down pick a line,
Left/Right change a choice, Space flips a check, text lines are typed
into, Enter saves and Esc goes back without saving. Every change is an
undo step.

![Checking a task off, picking an owner from a dropdown, a rejected entry, and the rules panel](../media/rules.gif)

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
colors: the terminal's palette or the color scheme ([Themes](../terminal/themes.md))
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
them from it. Validation can't go on a pivot table's results, which take
no entries. Data > Remove data validation takes the selection out of
every rule.

What validation checks:

- **Typing, a dropdown's pick and macros' entries**, as they're
  entered, rejected or warned as above.
- **Pastes and fills** (Ctrl+V, paste values, text pasted from another
  program; Ctrl+D, Ctrl+R, Ctrl+Enter, the fill handle), after they're
  written, as in Sheets. When a cell's rule rejects what landed there,
  the whole paste or fill is undone and ERROR mode says which cell and
  why ("Paste undone: Invalid entry in B2: ..."); Ctrl+Enter keeps the
  entry open instead. When only rules that warn fail, it stays, the
  cells are marked, and the context line says how many ("2 cells
  invalid, first B2: ...").
- **Imports** aren't checked; what they leave invalid is marked.

Adding a checkbox or a dropdown changes no cells, so on a protected range
it doesn't ask first (on a protected sheet it does); checking a box or
picking an item asks as typing does ([protection](notes-protection.md#protected-sheets-and-ranges)).

## Moving cells

Conditional formats and validation rules move with cut and paste on a
sheet, as in Sheets: the cut cells take their rules to where
they land, and the cells they land on lose theirs. A rule whose cells
partly move keeps both parts; with a custom formula it becomes two
rules, each with the formula written for its own first cell, so the
cells that stayed read what they read and the moved ones read the cells
beside their new places. Formulas in rules follow cells that
move. Cut on one sheet and pasted on another, the cells leave their
rules behind and meet the rules where they land.
