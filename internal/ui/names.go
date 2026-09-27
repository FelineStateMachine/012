package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Named ranges, as Sheets' Data > Named ranges: a picker lists them with
// their ranges, Enter goes to one, F2 renames it or changes its range,
// Ctrl+D deletes it, and the first row adds one for the selection.
// Naming and renaming are asked on the context line; changing the range
// is pointed at with the arrows, like any range prompt.

func init() {
	register(
		&command{id: "data.named_ranges", title: "Named ranges", desc: "List the named ranges: go to, add, edit or delete them", run: func(m *Model) tea.Cmd {
			m.openNames(m.selection())
			return nil
		}},
		&command{id: "data.define_name", title: "Define named range", desc: "Name the selected range, to use the name in formulas", run: func(m *Model) tea.Cmd {
			m.defineName(m.selection(), false)
			return nil
		}},
	)
}

// namesPicker is the picker of named ranges with its extra keys.
type namesPicker struct {
	*picker
	sel sheet.Rect // the selection when the picker opened, for "Add a range"
	msg string     // feedback on the last action, e.g. a deletion
}

// openNames opens the picker; sel is what "Add a range" names.
func (m *Model) openNames(sel sheet.Rect) {
	p := newPicker(m, "Named ranges", "Type a name", 60, namesItems(m, sel))
	p.action = "go to"
	m.clearSelection()
	m.openOverlay(&namesPicker{picker: p, sel: sel})
}

// namesItems lists "Add a range" and then every named range.
func namesItems(m *Model, sel sheet.Rect) []pickItem {
	add := "+ Add a range"
	items := []pickItem{{
		title: add, name: len(add), detail: sel.String(), desc: "Name " + sel.String() + ", to use the name in formulas",
		pick: func(m *Model) tea.Cmd {
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
		items = append(items, pickItem{
			title: n.Name, name: len(n.Name), detail: n.Ref(), desc: desc, off: n.Gone(),
			pick: func(m *Model) tea.Cmd {
				m.closeOverlay()
				m.showSheet(n.Sheet)
				m.selectRect(n.Range)
				return nil
			},
		})
	}
	return items
}

// current is the named range highlighted in the picker, if any.
func (p *namesPicker) current(m *Model) (sheet.Name, bool) {
	if p.picker.sel >= len(p.shown) {
		return sheet.Name{}, false
	}
	return m.sheet.LookupName(p.shown[p.picker.sel].item.title)
}

func (p *namesPicker) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
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
			m.sheet.DeleteName(n.Name)
			m.changed = true
			p.msg = "Deleted " + n.Name + "; Ctrl+Z brings it back"
			p.items = namesItems(m, p.sel)
			sel := p.picker.sel
			p.changed(m)
			p.picker.sel = max(min(sel, len(p.shown)-1), 0)
		}
		return nil
	}
	return p.picker.key(m, k)
}

func (p *namesPicker) status(m *Model) (string, string) {
	keys := m.th.KeyHints("Enter", "go to", "F2", "edit", "Ctrl+D", "delete", "Esc", "close")
	if _, ok := p.current(m); !ok {
		keys = m.th.KeyHints("Enter", "add", "Esc", "close")
	}
	if p.msg != "" {
		return p.msg, keys
	}
	desc, _ := p.picker.status(m)
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
		if n.Sheet.Live() {
			m.showSheet(n.Sheet) // the range is pointed at on its own sheet
		}
		m.openRange("Range for "+text+":", func(m *Model, r sheet.Rect) tea.Cmd {
			if err := m.sheet.EditName(n.Name, text, r); err != nil {
				m.fail(err.Error())
				return nil
			}
			m.changed = true
			m.openNames(sel)
			return nil
		})
		m.point = pointer{anchor: r.From, at: r.To, anchored: r.From != r.To}
		return nil
	})
	m.prompt.indicator = "NAME"
}

// namedSelection is the name of the selected range, if it has one, for
// the name box, as in Sheets.
func (m *Model) namedSelection() (string, bool) {
	r := m.selection()
	for _, n := range m.sheet.Names() {
		if !n.Gone() && n.Sheet == m.sheet && n.Range == r {
			return n.Name, true
		}
	}
	return "", false
}
