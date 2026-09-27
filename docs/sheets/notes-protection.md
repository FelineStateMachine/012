---
title: "Notes and protection"
sidebar_position: 9
---

# Notes and protection

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
