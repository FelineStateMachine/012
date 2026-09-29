package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Tables, as Sheets' Format > Convert to table: a range whose first row
// names its columns, read in formulas by those names (Sales[Amount]).
// Converting asks for the table's name on the context line; Data > Table
// renames, resizes (pointed at like any range), styles and removes the
// table the active cell is in, and lists every table, regions' too, to
// go to one. Filters and sorts started in a table take its range.

// tableItems is Data > Table.
var tableItems = []menuItem{
	{cmd: "data.tables"}, sep,
	{cmd: "table.rename"}, {cmd: "table.resize"}, sep,
	{cmd: "table.banded"}, {cmd: "table.header"}, sep,
	{cmd: "table.remove"},
}

func init() {
	inTable := func(m *Model) bool { _, ok := m.sheet.TableAt(m.cur); return ok }
	register(
		&command{id: "table.create", title: "Convert to table", desc: "Make the selection, or the data around the active cell, a table whose first row names its columns",
			run: (*Model).createTable, edits: func(m *Model) sheet.Rect { return m.dataRange() },
			enabled: func(m *Model) bool { return !inTable(m) }},
		&command{id: "data.tables", macro: macroView, title: "Tables", desc: "List the tables and notebook outputs formulas read by column name: go to, rename or remove them",
			run: func(m *Model) tea.Cmd {
				m.openTables()
				return nil
			}},
		&command{id: "table.rename", title: "Rename table", desc: "Rename the active cell's table, and every formula that reads it",
			run: func(m *Model) tea.Cmd {
				t, _ := m.sheet.TableAt(m.cur)
				m.renameTable(t, false)
				return nil
			}, enabled: inTable},
		&command{id: "table.resize", title: "Resize table", desc: "Point at the range the active cell's table covers, header row first",
			run: func(m *Model) tea.Cmd {
				t, _ := m.sheet.TableAt(m.cur)
				m.resizeTable(t)
				return nil
			}, enabled: inTable},
		&command{id: "table.banded", title: "Banded rows", desc: "Shade every other row of the active cell's table",
			run:     func(m *Model) tea.Cmd { return m.styleTable(func(t *sheet.Table) { t.Banded = !t.Banded }) },
			enabled: inTable, checked: func(m *Model) bool { t, ok := m.sheet.TableAt(m.cur); return ok && t.Banded }},
		&command{id: "table.header", title: "Header style", desc: "Draw the header row of the active cell's table bold, underlined and in color",
			run:     func(m *Model) tea.Cmd { return m.styleTable(func(t *sheet.Table) { t.Header = !t.Header }) },
			enabled: inTable, checked: func(m *Model) bool { t, ok := m.sheet.TableAt(m.cur); return ok && t.Header }},
		&command{id: "table.remove", title: "Remove table", desc: "Make the active cell's table plain cells again; formulas that read it read the cells by address",
			run: func(m *Model) tea.Cmd {
				t, _ := m.sheet.TableAt(m.cur)
				m.removeTable(t)
				return nil
			}, enabled: inTable},
	)
	keymap["ctrl+alt+t"] = "table.create"
}

// createTable asks for a name for a table of the selection, or of the
// data around the active cell.
func (m *Model) createTable() tea.Cmd {
	r := m.dataRange()
	m.openText("Name for the table of "+r.String()+":", m.book().NextTableName(), func(m *Model, name string) tea.Cmd {
		if err := m.sheet.CreateTable(name, r); err != nil {
			m.fail(err.Error())
			return nil
		}
		m.changed = true
		t, _ := m.sheet.TableAt(r.From)
		m.selectRect(t.Range)
		m.note = name + " is a table: read it by column in formulas, e.g. =SUM(" + name + "[" + t.Cols[len(t.Cols)-1] + "])"
		return nil
	})
	m.prompt.indicator = "NAME"
	return nil
}

// renameTable asks for a new name for t. From the picker (back), the
// picker opens again afterwards.
func (m *Model) renameTable(t sheet.Table, back bool) {
	m.openText("Rename table "+t.Name+":", t.Name, func(m *Model, name string) tea.Cmd {
		if err := m.book().RenameTable(t.Name, name); err != nil {
			m.fail(err.Error())
			return nil
		}
		m.changed = true
		if back {
			m.noteUnrecorded()
			m.openTables()
			return nil
		}
		m.note = "Renamed " + t.Name + " to " + name + " in every formula that reads it"
		return nil
	})
	m.prompt.indicator = "NAME"
}

// resizeTable asks for t's new range, pointed at from its own.
func (m *Model) resizeTable(t sheet.Table) {
	m.openRange("Range for "+t.Name+", header row first:", func(m *Model, r sheet.Rect) tea.Cmd {
		if err := m.book().ResizeTable(t.Name, r); err != nil {
			m.fail(err.Error())
			return nil
		}
		m.changed = true
		nt, _ := m.sheet.TableAt(r.From)
		m.selectRect(nt.Range)
		m.note = t.Name + " covers " + nt.Range.String() + ": " + strings.Join(nt.Cols, ", ")
		return nil
	})
	m.point = pointer{anchor: t.Range.From, at: t.Range.To, anchored: true}
}

// styleTable changes the style of the active cell's table with fn.
func (m *Model) styleTable(fn func(*sheet.Table)) tea.Cmd {
	t, ok := m.sheet.TableAt(m.cur)
	if !ok {
		return nil
	}
	fn(&t)
	if err := m.book().SetTableStyle(t.Name, t.Banded, t.Header); err != nil {
		m.fail(err.Error())
		return nil
	}
	m.changed = true
	return nil
}

// removeTable makes t plain cells again.
func (m *Model) removeTable(t sheet.Table) {
	if err := m.book().RemoveTable(t.Name); err != nil {
		m.fail(err.Error())
		return
	}
	m.changed = true
	m.note = "Removed the table " + t.Name + "; its cells stay, and formulas read them by address   " +
		m.th.KeyHints(m.shortcut("edit.undo"), "undo")
}

// tableLine is the context line in a table: its name and the column's,
// as formulas read it.
func (m *Model) tableLine() string {
	t, ok := m.sheet.TableAt(m.cur)
	i := m.cur.Col - t.Range.From.Col
	if !ok || i >= len(t.Cols) {
		return ""
	}
	col := t.Cols[i]
	ref := t.Name + "[" + col + "]"
	if strings.ContainsAny(col, " []#',") {
		ref = t.Name + "[[" + col + "]]"
	}
	return m.th.Muted.Render("Table "+t.Name+", column ") + m.th.Key.Render(col) + m.th.Muted.Render(": "+ref+" in formulas")
}

// tablesHost is what the tables picker acts on. The model implements
// it. The picker stays in package ui, as the named ranges picker does:
// its keys open the model's prompts and move its selection.
type tablesHost interface {
	styles() *theme.Theme
	book() *sheet.Workbook
	closeOverlay()
	tablesItems() []picker.Item
	// renameTable asks for a table's new name, then opens the picker
	// again.
	renameTable(t sheet.Table, back bool)
	// tableChanged follows a table changed from the picker.
	tableChanged()
}

// tablesPicker is the picker of tables with its extra keys.
type tablesPicker struct {
	m tablesHost
	*picker.Picker
	msg string // feedback on the last action, e.g. a removal
}

// openTables opens the picker of tables.
func (m *Model) openTables() {
	p := m.newPicker("Tables", "Type a name", 60, m.tablesItems())
	p.Action = "go to"
	m.clearSelection()
	m.openOverlay(&tablesPicker{m: m, Picker: p})
}

func (m *Model) tableChanged() {
	m.syncChanged()
	m.noteUnrecorded()
}

// tablesItems lists every table formulas can read.
func (m *Model) tablesItems() []picker.Item {
	var items []picker.Item
	for _, t := range m.book().TableInfos() {
		where := sheet.Qualified(t.Sheet.Name(), t.Range)
		desc := "Go to it; columns " + strings.Join(t.Cols, ", ")
		switch {
		case t.Region && !t.Shown():
			where, desc = t.Sheet.Name(), "A region with no rows yet"
		case t.Region:
			desc = "A region: its source names and shapes it; " + strconv.Itoa(len(t.Cols)) + " columns"
		}
		items = append(items, picker.Item{
			Title: t.Name, Name: len(t.Name), Detail: where, Desc: desc, Off: !t.Shown(),
			Pick: func() tea.Cmd {
				m.closeOverlay()
				if !t.Shown() || m.refuseHidden(t.Sheet) {
					return nil
				}
				m.showSheet(t.Sheet)
				m.selectRect(t.Range)
				return nil
			},
		})
	}
	return items
}

// current is the table highlighted in the picker, when it's one the
// table commands change.
func (p *tablesPicker) current() (sheet.Table, bool) {
	if p.Picker.Sel >= len(p.Shown()) {
		return sheet.Table{}, false
	}
	_, t, ok := p.m.book().Table(p.Shown()[p.Picker.Sel].Item.Title)
	return t, ok
}

func (p *tablesPicker) Key(k tea.KeyPressMsg) tea.Cmd {
	m := p.m
	p.msg = ""
	switch k.String() {
	case "f2":
		if t, ok := p.current(); ok {
			m.closeOverlay()
			m.renameTable(t, true)
		}
		return nil
	case "ctrl+d":
		if t, ok := p.current(); ok {
			m.book().RemoveTable(t.Name)
			m.tableChanged()
			p.msg = "Removed the table " + t.Name + "; Ctrl+Z brings it back"
			p.Items = m.tablesItems()
			sel := p.Picker.Sel
			p.Changed()
			p.Picker.Sel = max(min(sel, len(p.Shown())-1), 0)
		}
		return nil
	}
	return p.Picker.Key(k)
}

func (p *tablesPicker) Status() (string, string) {
	th := p.m.styles()
	keys := th.KeyHints("Enter", "go to", "F2", "rename", "Ctrl+D", "remove", "Esc", "close")
	if _, ok := p.current(); !ok {
		keys = th.KeyHints("Enter", "go to", "Esc", "close")
	}
	if len(p.Shown()) == 0 {
		keys = th.KeyHints("Esc", "close")
	}
	if p.msg != "" {
		return p.msg, keys
	}
	if len(p.Items) == 0 {
		return "No tables yet: Format > Convert to table makes one", keys
	}
	desc, _ := p.Picker.Status()
	return desc, keys
}
