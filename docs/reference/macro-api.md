---
title: "Macro scripting API"
sidebar_position: 5
---

# Macro scripting API

What a [macro](../sheets/macros.md) script can do. Recorded macros use
the same functions, so recording one is a quick way to see them.

A script runs from top to bottom. It's Starlark: Python's syntax
(`for`, `if`, `def`, lists, dicts, string formatting, `while`) without
imports, classes or exceptions. Statements may be at the top level.

```python
# Total every numeric column of the selection, in the row below it.
sel = selection()                       # e.g. "B2:D9"
rows = get(sel)
if type(rows) != "list":
    rows = [[rows]]                     # a single cell
below = offset(sel.split(":")[-1], 0, 1)
first = offset(sel.split(":")[0], 0, len(rows))
for c in range(len(rows[0])):
    col = [r[c] for r in rows]
    if all([type(v) in ("int", "float") for v in col]):
        cell = offset(first, c, 0)
        set_formula(cell, "SUM(%s:%s)" % (offset(sel.split(":")[0], c, 0), offset(cell, 0, -1)))
select(first)
run("format.bold")
```

References are strings in A1 notation: a cell (`"B3"`), a range
(`"B3:D9"`), whole columns (`"B:D"`) or rows (`"3:5"`), a named range
(`"Sales"`), any of them after a sheet name (`"Sheet2!A1"`,
`"'Q3 plan'!A1:B2"`). Values come back as Starlark values: whole numbers
as `int`, others as `float`, text as `str`, `True` and `False`, `None`
for a blank cell, and error values as their code (`"#DIV/0!"`).

## Cells

| Function | Does |
|---|---|
| `get(ref)` | The value of a cell, or a list of rows of values for a range |
| `get_formula(ref)` | A cell's formula as typed (`"=SUM(B2:B8)"`), `""` if it holds none; a list of rows for a range |
| `set(ref, value)` | Enter a value as if typed: numbers, `True`/`False`, `None` (clears), or text, which is read as typed (`"=A1*2"` is a formula, `"$5"` currency, `"'=x"` text). A range gets `value` in every cell, or a list of rows (`[[1, 2], [3, 4]]`) from its first cell |
| `set_formula(ref, formula)` | Enter a formula (the `=` is optional) in the first cell and fill it over the range, relative references adjusting as in a copy |
| `clear(ref=None)` | Clear contents, keeping formatting; the selection by default |
| `number_format(ref, kind, decimals=None, pattern="")` | Format numbers: kind is `auto`, `text`, `number`, `percent`, `scientific`, `accounting`, `financial`, `currency`, `date`, `time`, `datetime`, `duration`, or `custom` with a `pattern` such as `"0.0%"` |
| `get_number_format(ref)` | The kind of a cell's number format |
| `offset(ref, cols=0, rows=0)` | `ref` moved, keeping its size and sheet: `offset("A1", 2, 3)` is `"C4"` |

Formulas read inside a script are up to date: a value set a line earlier
is already in the formulas that use it.

## The selection

| Function | Does |
|---|---|
| `selection()` | The selected range, e.g. `"B2:D9"`, `"B:D"` |
| `active_cell()` | The active cell, e.g. `"B2"` |
| `select(ref, active=None)` | Select a cell or range, on another sheet if it names one; `active` puts the active cell elsewhere in it |
| `move(cols=0, rows=0)` | Move the active cell, dropping the selection |
| `extend(cols=0, rows=0, whole=None)` | Select from the active cell to `cols`, `rows` away; `whole="columns"` or `"rows"` selects whole ones |
| `jump(to, extend=False)` | Move as keys do: `"up"`, `"down"`, `"left"`, `"right"` (Ctrl+arrows, to the edge of the data), `"home"` (column A), `"start"` (A1), `"end"` (the last used cell) |
| `enter(text, fill=False, origin=None)` | Type into the active cell and accept, as Enter does without moving; `fill=True` fills every selected cell (Ctrl+Enter). With `origin`, a formula is taken as typed at that cell and its relative references move with the distance to the active cell. `text` is in en-US's form (`1.5`, `9/26/2026`, `=ROUND(A1, 2)`) whatever the file's [locale](../sheets/locale.md), as a recording writes it, so a macro does the same everywhere |
| `paste_text(text)` | Paste tab-separated text from the active cell, as pasting from another program: read in the file's [locale](../sheets/locale.md) |
| `fill(to=None, rows=0, cols=0)` | Drag the fill handle: continue the selection's series or copy it over `to`, or `rows` down (up if negative), or `cols` right (left if negative) |

## Sheets

| Function | Does |
|---|---|
| `sheets()` | The sheet names, in tab order, hidden ones included |
| `active_sheet()` | The name of the sheet shown |
| `activate_sheet(name)` | Show a sheet (not a hidden one: `run("sheet.unhide", answer=name)` shows it again) |
| `add_sheet(name=None)` | Add a sheet after the one shown and show it; returns its name |
| `move_sheet(position)` | Move the sheet shown to a position, counting from 1 |
| `set_width(cols, width)` | Set column widths, 1 to 240: `set_width("B", 14)`, `set_width("B:D", 8)` |
| `set_height(rows, height)` | Set row heights in lines, 1 to 50, or 0 to fit their contents: `set_height("3", 2)`, `set_height("3:5", 0)` |

## Commands

`run(id, answer=None)` runs any command from the menus or palette by its
id, on the selection, as if picked. A command that asks something (a
width, a name, a confirmation) needs `answer`: the text you'd type, a range
for a range question, the key of a choice (`"enter"`, `"d"`) or its
label, or the item picked from a list (View > Hidden sheets takes the
sheet's name). A command that opens a dialog takes what you'd choose in
it as a dict (see [Dialogs](#dialogs)). Other pickers (Named ranges) can't
run in a script; nor can files, menus, help, undo or macros. The ids are
those in the recorded scripts; a few:

| Id | Command |
|---|---|
| `format.bold`, `format.italic`, `format.underline`, `format.strikethrough` | Text styles (toggles, as the keys) |
| `format.currency`, `format.percent`, `format.date`, `format.decimals_more`, ... | Number formats |
| `format.align_left`, `format.align_center`, `format.align_right`, `format.clear` | Alignment, clear formatting |
| `edit.copy`, `edit.cut`, `edit.paste`, `edit.paste_values` | Clipboard (within 012; copying also sets the system clipboard) |
| `edit.fill_down`, `edit.fill_right`, `clear` | Fill, clear the selection |
| `insert.row_above`, `insert.row_below`, `insert.col_left`, `insert.col_right`, `delete.row`, `delete.col` | Rows and columns |
| `column.width` (answer: the width), `column.reset` | Column widths |
| `data.sort_sheet_az`, `data.sort_range_az`, `data.sort_range_za`, `data.filter`, `data.filter_remove` | Sorting and filters |
| `data.define_name` (answer: the name) | Name the selection |
| `format.conditional_add`, `data.validation_add` (answer: the rule as a line of the file, see [The .012 format](../files/format.md#conditional-formats-and-data-validation)), `insert.checkbox`, `format.conditional_clear`, `data.validation_clear`, `data.checkbox_toggle` | Conditional formats, data validation and checkboxes |
| `format.conditional_set`, `data.validation_set` (answer: `{"rule": 2, ...}`, the rule's number and its new line), `format.conditional_remove`, `data.validation_remove` (answer: the rule's number), `format.conditional_move` (answer: `{"rule": 2, "to": 1}`) | Change the rules of the sheet shown, numbered from 1 as the panel lists them |
| `sheet.new`, `sheet.duplicate`, `sheet.rename` (answer: the name), `sheet.delete` (answer: `"enter"` when it asks), `sheet.hide`, `sheet.unhide` (answer: the sheet's name) | Sheets |
| `view.freeze_rows1`, `view.freeze_cols1`, `view.freeze_rows0`, ... | Frozen panes |

Record a macro that uses a command to see its id.

## Dialogs

A command that opens a dialog runs in a script with `answer` set to what
you'd choose in it, as a dict: the dialog doesn't open, and what it would
do is done. A recording writes dialogs this way, so recording one is the
quickest way to get its answer right. Fields left out keep what the
dialog would start with.

```python
run("data.sort_range", answer={"by": [{"column": "B", "order": "desc"}, {"column": "A"}], "header": True})
run("data.filter_column", answer={"column": "A", "hidden": ["North"]})
run("chart.edit", answer={"chart": 1, "at": "F3", "width": 30})
```

| Id | Answer |
|---|---|
| `data.sort_range` | `by`: the columns to sort by, in order, each `{"column": "B"}`, with `"order": "desc"` for Z to A; `header`: whether the range's first row is a header that stays in place (left out: as the bar guesses). The range is the selection, or the data around the active cell |
| `data.filter_column` | `column`: the column of the filter (left out: the active one); `hidden`: the values it hides; `condition` and `value`: the condition rows must meet, as a filter column is written in [the .012 format](../files/format.md) (`gt`, `contains`, ...), the value in en-US's form |
| `edit.replace` | `find` and `replace`; `matchCase`, `wholeCell`, `regex`, `inFormulas` set to `True` to turn them on; `within`: `"all"` for every sheet or a range of the sheet shown (left out: the sheet shown); `cell`: replace only the match in that cell (left out: replace all) |
| `insert.chart` | The chart as a line of the file: `type`, `data`, `at`, `width`, `height`, `header`, `labels`, `title` and the options, see [The .012 format](../files/format.md); what's left out is the chart Insert > Chart makes of the selection |
| `chart.edit` | `chart`: the chart's number on the sheet shown, counting from 1 (left out: the chart the command would edit), and the fields of its line to change, as for `insert.chart`; `None` puts a field back to its default |
| `data.pivot`, `data.pivot_edit` | The pivot table as a line of the file: `source`, `rows`, `columns`, `values`, `filters`, `rowTotals`, `columnTotals`, see [The .012 format](../files/format.md). `data.pivot` makes it on a new sheet, as Data > Pivot table does; `data.pivot_edit` changes the pivot of the sheet shown |

A dict's keys are strings, and its values `None`, `True`, `False`,
numbers, strings, lists and dicts; `run` hands it on as JSON with the
keys in the order written.

## Output and errors

`print(...)` shows its last line on the context line when the macro ends.
A failure stops the macro and shows where it happened; there's no
`try`. Calling a function wrongly (`move(1, 2, 3)`) or on a reference
that doesn't exist fails the same way. Only what the functions above do is
possible: no `load`, no files, no time, no randomness, so a script does
the same thing every time it runs on the same sheet.

## In the file

Macros are saved in the `.012` file, one line each, after the named
ranges:

```json
"macroOrigin": "3f9c0b1e5a7d42c8b61f0e9a2d4c7b18",
"macros": [
  {"name": "Totals", "key": "1", "api": 1, "source": "# Recorded with relative references...\nenter(\"Total\")\n"}
],
```

`api` is the version of the functions above the script was written for.
The file's version doesn't change: older builds of 012 open files with
macros and ignore them (and drop them if they save).
