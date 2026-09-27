package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// Protected ranges and sheets follow Sheets' Data > Protect sheets and
// ranges with "Show a warning when editing this range": an edit that
// touches one asks first on the context line, Enter to go on and Esc to
// back out. Commands are guarded through what they edit (command.edits,
// or command.changes where that's narrower), and so are typing into a
// cell, pasting text, the fill handle and sorting. Macros that run don't
// ask, as Sheets' scripts don't. A picker lists the protections, like
// Sheets' side panel.

func init() {
	register(
		&command{id: "data.protect", macro: macroView, title: "Protect sheets and ranges", desc: "List the protected ranges: add, go to or remove them", run: func(m *Model) tea.Cmd {
			m.openProtections(m.selection())
			return nil
		}},
		&command{id: "data.protect_range", title: "Protect range", desc: "Warn before edits to the selected range",
			run: func(m *Model) tea.Cmd { return m.protectAsk(m.selection(), false, false) }},
		&command{id: "data.protect_sheet", title: "Protect sheet", desc: "Warn before any edit to this sheet",
			run: func(m *Model) tea.Cmd { return m.protectAsk(m.selection(), true, false) }},
		&command{id: "data.unprotect", title: "Remove protection", desc: "Stop warning before edits to the protected ranges the selection touches",
			enabled: func(m *Model) bool { _, ok := m.sheet.Protecting(m.selection()); return ok },
			run: func(m *Model) tea.Cmd {
				n := m.sheet.UnprotectRange(m.selection())
				if n > 0 {
					m.changed = true
				}
				switch n {
				case 0:
					m.note = "Nothing here is protected"
				case 1:
					m.note = "Removed 1 protection; Ctrl+Z brings it back"
				default:
					m.note = "Removed " + strconv.Itoa(n) + " protections; Ctrl+Z brings them back"
				}
				return nil
			}},
	)
}

// protectAsk asks for a description of the protection on the context
// line, then protects r, or the sheet; from the picker (back), the picker
// opens again afterwards.
func (m *Model) protectAsk(r sheet.Rect, whole, back bool) tea.Cmd {
	what := r.String()
	if whole {
		what = m.sheet.Name()
	}
	m.openText("Describe "+what+" (optional):", "", func(m *Model, text string) tea.Cmd {
		m.sheet.Protect(sheet.Protection{Range: r, Sheet: whole, Desc: text})
		m.changed = true
		if back {
			m.openProtections(r)
			return nil
		}
		m.note = what + " is protected: edits to it ask first"
		return nil
	})
	m.prompt.indicator = "PROTECT"
	return nil
}

// protectedTarget is what a command changes, for protected ranges, and
// false for a command that changes no cells.
func (m *Model) protectedTarget(c *command) (sheet.Rect, bool) {
	if c.changes != nil {
		return c.changes(m)
	}
	if c.edits != nil {
		return c.edits(m), true
	}
	return sheet.Rect{}, false
}

// askProtected reports whether r touches a protected range, or the
// sheet is protected, and if so asks on the context line whether to go
// on: Enter runs retry, which then isn't asked again.
func (m *Model) askProtected(r sheet.Rect, retry func(*Model) tea.Cmd) bool {
	if m.protectOK || m.macros.run != nil {
		return false
	}
	p, ok := m.sheet.Protecting(r)
	if !ok {
		return false
	}
	msg := p.Range.String() + " is protected."
	if p.Sheet {
		msg = m.sheet.Name() + " is protected."
	}
	desc := "This part of the sheet shouldn't be changed by accident"
	if p.Desc != "" {
		desc = p.Desc + ": shouldn't be changed by accident"
	}
	m.ask(question{
		msg: msg, warn: true, desc: desc,
		choices: []choice{
			{key: "enter", label: "Edit anyway", run: func(m *Model) tea.Cmd {
				m.protectOK = true
				defer func() { m.protectOK = false }()
				return retry(m)
			}},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		},
	})
	return true
}

// askCommand asks before running a command that changes a protected
// range; a command that changes no cells asks only when the whole sheet
// is protected, as inserting rows into it would.
func (m *Model) askCommand(c *command) bool {
	if c.edits == nil && c.changes == nil {
		return false
	}
	retry := func(m *Model) tea.Cmd { return m.runCommand(c.id) }
	if r, ok := m.protectedTarget(c); ok {
		return m.askProtected(r, retry)
	}
	for _, p := range m.sheet.Protections() {
		if p.Sheet {
			return m.askProtected(p.Range, retry)
		}
	}
	return false
}

// protectionsPicker lists the sheet's protections, with keys to remove
// them.
type protectionsPicker struct {
	m *Model
	*picker.Picker
	sel sheet.Rect // the selection when the picker opened, to protect
	msg string     // feedback on the last removal
}

func (m *Model) openProtections(sel sheet.Rect) {
	p := m.newPicker("Protected sheets and ranges", "Type to search", 60, protectionItems(m, sel))
	p.Action = "go to"
	m.openOverlay(&protectionsPicker{m: m, Picker: p, sel: sel})
}

// protectionItems lists adding a range and the sheet, then every
// protection.
func protectionItems(m *Model, sel sheet.Rect) []picker.Item {
	addRange, addSheet := "+ Protect a range", "+ Protect the sheet"
	items := []picker.Item{
		{Title: addRange, Name: len(addRange), Detail: sel.String(), Desc: "Warn before edits to " + sel.String(),
			Pick: func() tea.Cmd { m.closeOverlay(); return m.protectAsk(sel, false, true) }},
		{Title: addSheet, Name: len(addSheet), Detail: m.sheet.Name(), Desc: "Warn before any edit to " + m.sheet.Name(),
			Pick: func() tea.Cmd { m.closeOverlay(); return m.protectAsk(sel, true, true) }},
	}
	for _, p := range m.sheet.Protections() {
		detail, desc := p.Range.String(), "Go to "+p.Range.String()
		if p.Sheet {
			detail, desc = "whole sheet", "Every edit to "+m.sheet.Name()+" asks first"
		}
		title := p.Desc
		if title == "" {
			title = detail
		}
		items = append(items, picker.Item{Title: title, Name: len(title), Detail: detail, Desc: desc, Pick: func() tea.Cmd {
			m.closeOverlay()
			if !p.Sheet {
				m.selectRect(p.Range)
			}
			return nil
		}})
	}
	return items
}

// current is the index of the protection highlighted, if one is.
func (p *protectionsPicker) current() (int, bool) {
	shown := p.Shown()
	if p.Picker.Sel >= len(shown) {
		return 0, false
	}
	for i := range p.Items[2:] {
		if &p.Items[2+i] == shown[p.Picker.Sel].Item {
			return i, true
		}
	}
	return 0, false
}

func (p *protectionsPicker) Key(k tea.KeyPressMsg) tea.Cmd {
	m := p.m
	p.msg = ""
	if k.String() != "ctrl+d" {
		return p.Picker.Key(k)
	}
	if i, ok := p.current(); ok {
		label := m.sheet.Protections()[i].Label()
		m.sheet.Unprotect(i)
		m.changed = true
		p.msg = "Removed the protection of " + label + "; Ctrl+Z brings it back"
		p.Items = protectionItems(m, p.sel)
		sel := p.Picker.Sel
		p.Changed()
		p.Picker.Sel = max(min(sel, len(p.Shown())-1), 0)
	}
	return nil
}

func (p *protectionsPicker) Status() (string, string) {
	m := p.m
	keys := m.th.KeyHints("Enter", "go to", "Ctrl+D", "remove", "Esc", "close")
	if _, ok := p.current(); !ok {
		keys = m.th.KeyHints("Enter", "add", "Esc", "close")
	}
	if p.msg != "" {
		return p.msg, keys
	}
	desc, _ := p.Picker.Status()
	return desc, keys
}
