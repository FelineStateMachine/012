# Macros

Macros repeat work: record what you do once, then run it again with a
key. They work like Google Sheets' Extensions > Macros, under
**Data > Macros**. A macro is a [Starlark](https://github.com/bazelbuild/starlark)
script (a small, deterministic dialect of Python) saved in the `.012`
file, as Sheets saves Apps Script with a spreadsheet. Recorded macros are
scripts too, so you can read them, change them, or write your own.

![Recording a column total with relative references, then replaying it from Run macro and the palette](media/macros.gif)

## Recording

1. **Data > Macros > Record macro** (or search the palette for "record").
   `REC` appears beside the mode indicator, and the context line says
   what's being recorded.
2. Do the work: type entries, move and select, run commands from the
   menus, keys or palette, paste, drag the fill handle or a column border.
3. **Data > Macros > Stop and save recording**. Name the macro on the
   context line, then give it a shortcut digit, or leave it empty. Esc at
   either question goes back to recording; **Discard recording** stops
   without saving.

Two ways to record, as in Sheets:

| | Recorded as | Replays |
|---|---|---|
| **Record macro** (absolute references) | `select("B3")`: the cells you selected | On the same cells, wherever the active cell is |
| **Record macro with relative references** | `move(0, 1)`, `extend(2, 0)`: steps from the active cell | From wherever the active cell is when you run it; formulas typed move with it, as in a copy, and Ctrl+arrows, Home, Ctrl+Home and Ctrl+End are recorded as `jump()`, since where they land depends on the data |

What gets recorded:

- entries as they're accepted (`enter("=SUM(B2:B8)")`), including
  Ctrl+Enter over a selection (`fill=True`);
- the selection, only when something acts on it: pressing Down five times
  then typing records one `move(0, 5)` (or one `select()`), so a replay
  doesn't depend on the size of the window;
- commands that change the sheet, by id (`run("format.bold")`), with the
  answer to the question they ask, if any
  (`run("column.width", answer="15")`, `run("sheet.delete", answer="enter")`);
- pasted text (`paste_text(...)`), the fill handle (`fill(...)`), column
  borders dragged or fitted (`set_width(...)`) and tabs dragged
  (`move_sheet(...)`);
- rules added in the conditional formatting and data validation panels,
  as the commands that add a rule from a line of the file
  (`run("format.conditional_add", answer='{"ranges":"B2:B9",...}')`), and
  items picked from a dropdown, as entries. Editing, removing or moving a
  rule in the panel is noted as a comment, as the dialogs below are.

What doesn't: dialogs such as sorting by several columns, the filter
picker, find and replace, the chart editor, dragging a chart, and undo.
Anything like that which changes the workbook is noted in the script as a
comment, `# Not recorded: sort A2:C9`, so the script never silently does
less than you did. Files, menus, help and other macros are never part of a
macro.

## Running

- **The shortcut**, Ctrl+Alt+Shift+digit, in READY. Terminals without the
  kitty keyboard protocol send it as Ctrl+Alt with the shifted symbol
  (`Ctrl+Alt+!` for 1 on a US layout), which works too.
- **Data > Macros > Run macro**, a list of the saved macros.
- **The palette** (Ctrl+K): type the macro's name.

A run is one undo step: Ctrl+Z undoes everything the macro did. While it
runs, the mode indicator says `CMD` (as 1-2-3 did), keys and the mouse
wait, and **Esc stops it**; what it did so far stays, and Ctrl+Z undoes it.
When it ends, the context line says `Ran Totals`, or the last line the
script printed, or where it failed:

```
Totals:3:5: get: not a cell or range: ZZ   Ctrl+Z undoes what it did
```

(`Totals` is the macro's name, `3:5` the line and column in its script.)

Every run has a limit of 10 million Starlark steps, so a script that loops
forever stops within a second or so with
`stopped after 10000000 steps: does it loop forever?`.

## Managing

**Data > Macros > Manage macros** lists the saved macros with their
shortcuts:

| Key | Action |
|---|---|
| Enter | Run the highlighted macro |
| F2 | Rename it |
| F3 | Change its shortcut: a digit, or empty for none |
| F4 | Edit its script in your editor |
| Ctrl+D | Delete it (Ctrl+Z brings it back) |
| Type | Search by name |

The first row, **+ Write a macro** (also **Data > Macros > Write a
macro**), names a new macro and opens a script for it in your editor.

Editing uses `$VISUAL`, then `$EDITOR`, then `vi`, with the screen handed
over until the editor exits; the script is saved when it does. If it has
a mistake, it's saved anyway and the context line says where
(`Saved Totals, but it has a mistake at Totals:4:1: undefined: sett`), so
you can fix it with F4. Sessions served over SSH don't start an editor:
their macros can be recorded, run, renamed and deleted, not edited.

Changes to macros (recording, renaming, deleting, editing) are undo steps
and mark the file modified, like any edit.

## Macros from other computers

Opening a file never runs its macros. The file remembers which computer
its macros were made or trusted on (a random id kept in 012's
config directory, the one [config.md](config.md) describes). Running
a macro from a file made elsewhere asks once, on the context line:

```
Trust this file's macros?   Enter Run   Esc Cancel
```

Enter trusts all of the file's macros for this session and, once the file
is saved, on this computer. Sessions served over SSH have no id of their
own, so they ask once per session.

Scripts can't read or write files, reach the network, start programs, or
read the clock or random numbers: all they can reach is the functions
below. What a macro can do is what you could do with the keyboard in that
workbook, which is why it asks before running someone else's.

## Writing scripts

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

### Cells

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

### The selection

| Function | Does |
|---|---|
| `selection()` | The selected range, e.g. `"B2:D9"`, `"B:D"` |
| `active_cell()` | The active cell, e.g. `"B2"` |
| `select(ref, active=None)` | Select a cell or range, on another sheet if it names one; `active` puts the active cell elsewhere in it |
| `move(cols=0, rows=0)` | Move the active cell, dropping the selection |
| `extend(cols=0, rows=0, whole=None)` | Select from the active cell to `cols`, `rows` away; `whole="columns"` or `"rows"` selects whole ones |
| `jump(to, extend=False)` | Move as keys do: `"up"`, `"down"`, `"left"`, `"right"` (Ctrl+arrows, to the edge of the data), `"home"` (column A), `"start"` (A1), `"end"` (the last used cell) |
| `enter(text, fill=False, origin=None)` | Type into the active cell and accept, as Enter does without moving; `fill=True` fills every selected cell (Ctrl+Enter). With `origin`, a formula is taken as typed at that cell and its relative references move with the distance to the active cell |
| `paste_text(text)` | Paste tab-separated text from the active cell, as pasting from another program |
| `fill(to=None, rows=0, cols=0)` | Drag the fill handle: continue the selection's series or copy it over `to`, or `rows` down (up if negative), or `cols` right (left if negative) |

### Sheets

| Function | Does |
|---|---|
| `sheets()` | The sheet names, in tab order, hidden ones included |
| `active_sheet()` | The name of the sheet shown |
| `activate_sheet(name)` | Show a sheet (not a hidden one: `run("sheet.unhide", answer=name)` shows it again) |
| `add_sheet(name=None)` | Add a sheet after the one shown and show it; returns its name |
| `move_sheet(position)` | Move the sheet shown to a position, counting from 1 |
| `set_width(cols, width)` | Set column widths, 1 to 240: `set_width("B", 14)`, `set_width("B:D", 8)` |

### Commands

`run(id, answer=None)` runs any command from the menus or palette by its
id, on the selection, as if picked. A command that asks something (a
width, a name, a confirmation) needs `answer`: the text you'd type, a range
for a range question, the key of a choice (`"enter"`, `"d"`) or its
label, or the item picked from a list (View > Hidden sheets takes the
sheet's name). Commands that open a dialog or another picker (Sort
range, Insert chart, Named ranges) can't run in a script; nor can files,
menus, help, undo or macros. The ids are those in the recorded scripts; a few:

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
| `format.conditional_add`, `data.validation_add` (answer: the rule as a line of the file, see [files.md](files.md#conditional-formats-and-data-validation)), `insert.checkbox`, `format.conditional_clear`, `data.validation_clear`, `data.checkbox_toggle` | Conditional formats, data validation and checkboxes |
| `sheet.new`, `sheet.duplicate`, `sheet.rename` (answer: the name), `sheet.delete` (answer: `"enter"` when it asks), `sheet.hide`, `sheet.unhide` (answer: the sheet's name) | Sheets |
| `view.freeze_rows1`, `view.freeze_cols1`, `view.freeze_rows0`, ... | Frozen panes |

Record a macro that uses a command to see its id.

### Output and errors

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
