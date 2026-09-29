package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Named ranges, as Sheets' Data > Named ranges: a picker lists them with
// their ranges, Enter goes to one, F2 renames it or changes its range,
// Ctrl+D deletes it, and the first row adds one for the selection.
// Naming and renaming are asked on the context line; changing the range
// is pointed at with the arrows, like any range prompt.

func init() {
	register(
		&command{id: "data.named_ranges", macro: macroView, title: "Named ranges", desc: "List the named ranges: go to, add, edit or delete them", run: func(m *Model) tea.Cmd {
			m.openNames(m.selection())
			return nil
		}},
		&command{id: "data.define_name", title: "Define named range", desc: "Name the selected range, to use the name in formulas", run: func(m *Model) tea.Cmd {
			m.defineName(m.selection(), false)
			return nil
		}},
	)
}

// namesHost is what the named ranges picker acts on. The model
// implements it. The picker stays in package ui: it is a few keys over a
// picker, and what they do (naming, renaming and pointing at a range on
// the context line, going to one) are the model's prompts and
// selection, which open the picker again.
type namesHost interface {
	styles() *theme.Theme
	sheetShown() *sheet.Sheet
	closeOverlay()
	// namesItems are the picker's rows: "Add a range" for sel, then the
	// named ranges.
	namesItems(sel sheet.Rect) []picker.Item
	// editName asks for a new name and range for n, then opens the
	// picker again with sel.
	editName(n sheet.Name, sel sheet.Rect)
	// nameChanged follows a named range changed from the picker: the
	// file is modified, and a macro being recorded notes what it can't
	// replay.
	nameChanged()
}

// namesPicker is the picker of named ranges with its extra keys.
type namesPicker struct {
	m namesHost // the model, through what the picker needs of it
	*picker.Picker
	sel sheet.Rect // the selection when the picker opened, for "Add a range"
	msg string     // feedback on the last action, e.g. a deletion
}

// openNames opens the picker; sel is what "Add a range" names.
func (m *Model) openNames(sel sheet.Rect) {
	p := m.newPicker("Named ranges", "Type a name", 60, m.namesItems(sel))
	p.Action = "go to"
	m.clearSelection()
	m.openOverlay(&namesPicker{m: m, Picker: p, sel: sel})
}

func (m *Model) nameChanged() {
	m.syncChanged()
	m.noteUnrecorded()
}

// namesItems lists "Add a range" and then every named range.
func (m *Model) namesItems(sel sheet.Rect) []picker.Item {
	add := "+ Add a range"
	items := []picker.Item{{
		Title: add, Name: len(add), Detail: sel.String(), Desc: "Name " + sel.String() + ", to use the name in formulas",
		Pick: func() tea.Cmd {
			m.closeOverlay()
			m.defineName(sel, true)
			return nil
		},
	}}
	for _, n := range m.sheet.Names() {
		desc := "Go to " + n.Ref()
		switch users := m.sheet.NameUsers(n.Name); {
		case n.Lost:
			desc = "Its cells were deleted; formulas using it show #REF!"
		case n.Gone():
			desc = "Its sheet was deleted; formulas using it show #REF!"
		case users == 1:
			desc += ", used in 1 formula"
		case users > 1:
			desc += ", used in " + strconv.Itoa(users) + " formulas"
		}
		items = append(items, picker.Item{
			Title: n.Name, Name: len(n.Name), Detail: n.Ref(), Desc: desc, Off: n.Gone(),
			Pick: func() tea.Cmd {
				m.closeOverlay()
				if m.refuseHidden(n.Sheet) {
					return nil
				}
				m.showSheet(n.Sheet)
				m.selectRect(n.Range)
				return nil
			},
		})
	}
	return items
}

// current is the named range highlighted in the picker, if any.
func (p *namesPicker) current(m namesHost) (sheet.Name, bool) {
	if p.Picker.Sel >= len(p.Shown()) {
		return sheet.Name{}, false
	}
	return m.sheetShown().LookupName(p.Shown()[p.Picker.Sel].Item.Title)
}

func (p *namesPicker) Key(k tea.KeyPressMsg) tea.Cmd {
	m := p.m
	p.msg = ""
	switch k.String() {
	case "f2":
		if n, ok := p.current(m); ok {
			m.closeOverlay()
			m.editName(n, p.sel)
		}
		return nil
	case "ctrl+d":
		if n, ok := p.current(m); ok {
			m.sheetShown().DeleteName(n.Name)
			m.nameChanged()
			p.msg = "Deleted " + n.Name + "; Ctrl+Z brings it back"
			p.Items = m.namesItems(p.sel)
			sel := p.Picker.Sel
			p.Changed()
			p.Picker.Sel = max(min(sel, len(p.Shown())-1), 0)
		}
		return nil
	}
	return p.Picker.Key(k)
}

func (p *namesPicker) Status() (string, string) {
	m := p.m
	keys := m.styles().KeyHints("Enter", "go to", "F2", "edit", "Ctrl+D", "delete", "Esc", "close")
	if _, ok := p.current(m); !ok {
		keys = m.styles().KeyHints("Enter", "add", "Esc", "close")
	}
	if p.msg != "" {
		return p.msg, keys
	}
	desc, _ := p.Picker.Status()
	return desc, keys
}

// defineName asks for a name for r on the context line. From the picker
// (back), the picker opens again afterwards.
func (m *Model) defineName(r sheet.Rect, back bool) {
	m.openText("Name for "+r.String()+":", "", func(m *Model, text string) tea.Cmd {
		if err := m.sheet.DefineName(text, r); err != nil {
			m.fail(err.Error())
			return nil
		}
		m.changed = true
		if back {
			m.noteUnrecorded() // named from the picker, not as the command
			m.openNames(r)
			return nil
		}
		m.selectRect(r)
		m.note = text + " names " + r.String() + ": use it in formulas, e.g. =SUM(" + text + ")"
		return nil
	})
	m.prompt.indicator = "NAME"
}

// editName asks for a new name, then for the range, pointed at from the
// current one; Enter keeps either as it is. The picker opens again after.
func (m *Model) editName(n sheet.Name, sel sheet.Rect) {
	m.openText("Rename "+n.Name+":", n.Name, func(m *Model, text string) tea.Cmd {
		if err := sheet.ValidName(text); err != nil {
			m.fail(err.Error())
			return nil
		}
		r := n.Range
		if m.refuseHidden(n.Sheet) {
			return nil
		}
		if n.Sheet.Live() {
			m.showSheet(n.Sheet) // the range is pointed at on its own sheet
		}
		m.openRange("Range for "+text+":", func(m *Model, r sheet.Rect) tea.Cmd {
			if err := m.sheet.EditName(n.Name, text, r); err != nil {
				m.fail(err.Error())
				return nil
			}
			m.nameChanged()
			m.openNames(sel)
			return nil
		})
		m.point = pointer{anchor: r.From, at: r.To, anchored: r.From != r.To}
		return nil
	})
	m.prompt.indicator = "NAME"
}

// namedSelection is the name of the selected range, if it has one, for
// the name box, as in Sheets: a named range's, or a table's.
func (g *grid) namedSelection() (string, bool) {
	r := g.selection()
	for _, n := range g.sheet.Names() {
		if !n.Gone() && n.Sheet == g.sheet && n.Range == r {
			return n.Name, true
		}
	}
	if !g.sheet.HasTables() {
		return "", false
	}
	for _, t := range g.sheet.Tables() {
		if t.Range == r {
			return t.Name, true
		}
	}
	return "", false
}
