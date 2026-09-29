---
title: "Freeze, sort and filter"
sidebar_position: 5
---

# Freeze, sort and filter

## Freeze

View > Freeze keeps rows or columns on screen while the rest scrolls, marked
by thin divider lines. The mouse and keys treat the frozen area as Sheets
does.

## Sort

Data > Sort sheet sorts by the active column A to Z or Z to A, keeping frozen
rows on top. Data > Sort range sorts the selection, or the block of data
around the active cell (in a [table](tables.md), the table, its header
row kept in place); its advanced option opens a bar to sort by several
columns (Left/Right picks a column, Space flips the order, Alt+A adds a
column, Alt+H toggles the header row). The order follows Sheets: numbers,
then text ignoring case, then booleans, then errors, with blanks last.

## Filter

Data > Create a filter puts a filter on the selection or the table around
the active cell (a [table](tables.md)'s range, which the filter then
follows as the table grows); headers show `▾`. Alt+Down or a click on `▾` opens the
column's filter: check values in a list (Space checks one, typing
narrows the list), or pick a condition such as "greater than" or "text
contains". Filtered-out rows are hidden, not
deleted: row numbers show the gap, navigation skips them, formulas still
count them, and the status line says how many rows are hidden.
