---
title: "Sheets and tabs"
sidebar_position: 4
---

# Sheets and tabs

A spreadsheet holds several sheets, shown as tabs at the left of the
status line (as tmux lists its windows). Formulas read other sheets by
name, `=Sheet2!A1` ([references](@/formulas/references.md)).

| To | Do |
|---|---|
| Add a sheet | Shift+F11, Insert > Sheet, or click `+` after the tabs |
| Go to the next or previous sheet | Ctrl+PgDn, Ctrl+PgUp (or Alt+Right, Alt+Left), or click a tab |
| Go to a sheet by name | Alt+Shift+K |
| Rename, duplicate, delete, hide, move left or right | Edit > Sheet, or right-click a tab; double-click a tab to rename it |
| Move a sheet | Drag its tab onto another |
| Show a hidden sheet | View > Hidden sheets |

When the tabs don't all fit, `‹` and `›` step through them. A hidden
sheet is still read by formulas; next, previous and the tabs skip it.
While typing a formula, Ctrl+PgDn or clicking a tab points into that
sheet, as in Sheets. Every change to sheets is an undo step.
