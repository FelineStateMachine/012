package ui

import (
	"errors"
	"fmt"
	"image/color"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/rules"
)

// Rules follow Sheets: Format > Conditional formatting and Data > Data
// validation open a panel of the sheet's rules at the right of the grid
// (package rules), Insert > Checkbox and Insert > Dropdown add the two
// rules people reach for most, Space toggles a checkbox and Alt+Down
// opens a dropdown, and an entry a rule doesn't accept is refused or
// kept with a warning on the context line. The panel's rules are saved
// as the commands that add a rule from a line of the file, so a macro
// recording keeps the rules added.

func init() {
	hasFormats := func(m *Model) bool { return len(m.sheet.CondFormats()) > 0 }
	hasValidations := func(m *Model) bool { return len(m.sheet.Validations()) > 0 }
	register(
		&command{id: "format.conditional", macro: macroView, title: "Conditional formatting",
			desc: "Color cells by their values: rules on ranges, and color scales",
			run: func(m *Model) tea.Cmd {
				m.openOverlay(rules.Formats(m.host()))
				return nil
			}},
		&command{id: "format.conditional_add", title: "Add conditional format rule",
			desc: `Add a rule written as a line of the file, e.g. {"ranges":"B2:B9","condition":"gt","values":["100"],"fill":"green"}`,
			run: func(m *Model) tea.Cmd {
				m.openText("Rule:", "", func(m *Model, text string) tea.Cmd {
					f, err := sheet.ParseCondFormat(text)
					if err == nil {
						err = m.host().SaveFormat(-1, f)
					}
					m.failOn(err)
					return nil
				})
				return nil
			}},
		&command{id: "format.conditional_clear", title: "Clear conditional formats", enabled: hasFormats,
			desc: "Take the selected cells out of every conditional format rule",
			run: func(m *Model) tea.Cmd {
				m.sheet.ClearCondFormats(m.selection())
				m.changed = true
				return nil
			}},
		&command{id: "data.validation", macro: macroView, title: "Data validation",
			desc: "Rules on what cells may hold: dropdowns, checkboxes, numbers, dates, formulas",
			run: func(m *Model) tea.Cmd {
				m.openOverlay(rules.Validations(m.host()))
				return nil
			}},
		&command{id: "data.validation_add", title: "Add data validation rule",
			desc: `Add a rule written as a line of the file, e.g. {"ranges":"D2:D9","criteria":"list","items":["Yes","No"]}`,
			run: func(m *Model) tea.Cmd {
				m.openText("Rule:", "", func(m *Model, text string) tea.Cmd {
					v, err := sheet.ParseValidation(text)
					if err == nil {
						err = m.host().SaveValidation(-1, v)
					}
					m.failOn(err)
					return nil
				})
				return nil
			}},
		&command{id: "data.validation_clear", title: "Remove data validation", enabled: hasValidations,
			desc: "Take the selected cells out of every data validation rule",
			run: func(m *Model) tea.Cmd {
				m.sheet.ClearValidations(m.selection())
				m.changed = true
				return nil
			}},
		&command{id: "insert.checkbox", title: "Checkbox", edits: (*Model).selection, changes: noCells,
			desc: "Make the selected cells checkboxes: TRUE or FALSE, toggled with Space or a click",
			run: func(m *Model) tea.Cmd {
				m.failOn(m.host().SaveValidation(-1, sheet.Validation{Ranges: []sheet.Rect{m.selection()}, Kind: sheet.ValidCheckbox}))
				return nil
			}},
		&command{id: "insert.dropdown", macro: macroView, title: "Dropdown", edits: (*Model).selection, changes: noCells,
			desc: "Give the selected cells a list of items to pick from",
			run: func(m *Model) tea.Cmd {
				m.openOverlay(rules.Validations(m.host()).Add(true))
				return nil
			}},
		&command{id: "data.dropdown", macro: macroView, title: "Open dropdown",
			desc:    "Pick the active cell's value from its dropdown list, or filter its column",
			enabled: func(m *Model) bool { return m.onDropdown() || commands["data.filter_column"].available(m) },
			run: func(m *Model) tea.Cmd {
				if !m.onDropdown() {
					return m.runCommand("data.filter_column")
				}
				if m.refuseEdit(sheet.Rect{From: m.cur, To: m.cur}, false) {
					return nil // a pivot's result or a spilled value
				}
				m.openDropdown()
				return nil
			}},
		&command{id: "data.checkbox_toggle", title: "Toggle checkbox", typed: true, edits: (*Model).selection,
			desc:    "Check or uncheck the selected checkboxes",
			enabled: func(m *Model) bool { return m.sheet.HasRules() && m.sheet.Look(m.cur).Checkbox },
			run:     (*Model).toggleCheckboxes},
	)
	keymap["alt+down"] = "data.dropdown"
	keymap["space"] = "data.checkbox_toggle"
}

// failOn shows err, if any, as ERROR mode does.
func (m *Model) failOn(err error) {
	if err != nil {
		m.fail(err.Error())
	}
}

// onDropdown reports whether the active cell has a dropdown list.
func (m *Model) onDropdown() bool {
	return m.sheet.HasRules() && m.sheet.Look(m.cur).Dropdown
}

// openDropdown opens the active cell's list in a picker under the cell;
// picking an item enters it, as typing it would.
func (m *Model) openDropdown() {
	a := m.cur
	items := m.sheet.DropdownItems(a)
	current := m.sheet.ShownText(a)
	var list []picker.Item
	sel := 0
	for _, it := range items {
		pi := picker.Item{Title: it, Name: len(it), Desc: "Enter " + it + " in " + a.String()}
		pi.Pick = func() tea.Cmd {
			m.pickItem(a, it)
			return nil
		}
		if it == current {
			sel = len(list)
			pi.Detail = "✓"
		}
		list = append(list, pi)
	}
	if len(list) == 0 {
		m.note = "The dropdown of " + a.String() + " has no items"
		return
	}
	p := m.newPicker(a.String(), "Search the items", 40, list)
	p.Action = "choose"
	p.Sel = sel
	if y, ok := m.rowY(a.Row); ok {
		p.At = &[2]int{m.colStart(a.Col), y + 1}
	}
	m.openOverlay(p)
}

// pickItem enters a dropdown's item in the cell at a.
func (m *Model) pickItem(a sheet.Addr, item string) {
	m.closeOverlay()
	if m.askProtected(sheet.Rect{From: a, To: a}, func(m *Model) tea.Cmd {
		m.pickItem(a, item)
		return nil
	}) {
		return
	}
	if err := m.sheet.Set(a, item); err != nil {
		m.note = err.Error()
		return
	}
	m.changed = true
	m.recordEntry(item, false)
}

// toggleCheckboxes checks the selected checkboxes, or unchecks them when
// the active cell is checked, as Sheets' Space does. Blank checkboxes
// are unchecked, so unchecking leaves them blank. Whole columns or rows
// selected are cut to the data, so a million blank checkboxes aren't
// written.
func (m *Model) toggleCheckboxes() tea.Cmd {
	s := m.sheet
	on := !s.Look(m.cur).Checked
	targets := []sheet.Addr{m.cur}
	r := m.selection()
	if used, ok := s.UsedRange(); r.AllRows() || r.AllCols() {
		if !ok {
			used = sheet.Rect{From: m.cur, To: m.cur}
		}
		r = intersect(r, used)
	}
	if m.hasRange() {
		for _, v := range s.Validations() {
			if v.Kind != sheet.ValidCheckbox {
				continue
			}
			for _, vr := range v.Ranges {
				targets = appendCells(targets, intersect(vr, r), m.cur, func(a sheet.Addr) bool { return on || s.Filled(a) })
			}
		}
	}
	label := "uncheck " + r.String()
	if on {
		label = "check " + r.String()
	}
	err := s.Batch(sheet.Change{Label: label, Focus: r, Sheet: s}, func() error {
		for _, a := range targets {
			if err := s.Set(a, s.CheckboxInput(a, on)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		m.note = err.Error()
	}
	m.changed = true
	return nil
}

// appendCells appends the cells of r that keep keeps to out, but for
// skip; an empty r (From past To) adds nothing.
func appendCells(out []sheet.Addr, r sheet.Rect, skip sheet.Addr, keep func(sheet.Addr) bool) []sheet.Addr {
	for row := r.From.Row; row <= r.To.Row; row++ {
		for col := r.From.Col; col <= r.To.Col; col++ {
			if a := (sheet.Addr{Col: col, Row: row}); a != skip && keep(a) {
				out = append(out, a)
			}
		}
	}
	return out
}

// intersect is the cells a and b share, From past To when none.
func intersect(a, b sheet.Rect) sheet.Rect {
	return sheet.Rect{
		From: sheet.Addr{Col: max(a.From.Col, b.From.Col), Row: max(a.From.Row, b.From.Row)},
		To:   sheet.Addr{Col: min(a.To.Col, b.To.Col), Row: min(a.To.Row, b.To.Row)},
	}
}

// checkEntry checks an entry against the cell's validation: a rule that
// rejects it keeps the entry open with its help text, and one that
// warns lets it in, saying so. It reports whether the entry may go in.
func (m *Model) checkEntry(input string) (warn string, ok bool) {
	bad := m.entrySheet().CheckEntry(m.cur, input)
	switch {
	case bad == nil:
		return "", true
	case bad.Reject:
		m.entryError(bad, input)
		return "", false
	}
	return bad.Error(), true
}

// writeChecked runs a paste or fill (what), which returns the range it
// wrote, and checks what it wrote against the cells' validation, as
// Sheets does: when a rule that rejects fails, the whole change is
// taken back and ERROR mode says why; when only rules that warn fail,
// it stays, the cells are marked, and the context line says how many.
// Inside a macro's run, taking it back leaves the rest of the run as it
// is (sheet.Workbook.Try). It reports the range and whether the change
// stayed.
func (m *Model) writeChecked(what string, write func() (sheet.Rect, error)) (sheet.Rect, bool) {
	s := m.sheet
	var r sheet.Rect
	var bad []*sheet.InvalidEntry
	var refused *sheet.InvalidEntry
	kept, err := m.book().Try(func() error {
		var err error
		r, err = write()
		return err
	}, func() bool {
		if !s.HasRules() {
			return false
		}
		bad = s.InvalidIn(r)
		for _, b := range bad {
			if b.Reject {
				refused = b
				return true
			}
		}
		return false
	})
	switch {
	case err != nil:
		m.fail(err.Error())
		return r, false
	case !kept:
		m.fail(what + " undone: " + refused.Error())
		return r, false
	case len(bad) > 0:
		m.warn = invalidNote(bad)
	}
	return r, true
}

// checkEntryFill handles the cells an entry filled (Ctrl+Enter) that fail
// their validation: a rule that rejects takes the fill back and keeps
// the entry open with the rule's help, as for one cell; rules that warn
// let it in, saying so. It reports whether the entry stays open.
func (m *Model) checkEntryFill(bad []*sheet.InvalidEntry, input string) bool {
	for _, b := range bad {
		if b.Reject {
			m.entrySheet().Discard()
			m.entryError(b, input)
			return true
		}
	}
	if len(bad) > 0 {
		m.warn = invalidNote(bad)
	}
	return false
}

// invalidNote says how many cells a paste or fill left invalid, and why
// the first is.
func invalidNote(bad []*sheet.InvalidEntry) string {
	return fmt.Sprintf("%s invalid, first %s: %s", cellCount(len(bad)), bad[0].Addr, bad[0].Help)
}

// The rules panel's host.

func (h host) Sheet() *sheet.Sheet    { return h.m.sheet }
func (h host) Selection() sheet.Rect  { return h.m.selection() }
func (h host) Slot(i int) color.Color { return h.m.slotColor(i) }
func (h host) refused(rs []sheet.Rect) bool {
	for _, r := range rs {
		if h.m.sheet.InPivot(r) {
			return true
		}
	}
	return false
}

// SaveFormat adds or replaces a conditional format; an added one is
// recorded as the command that adds it.
func (h host) SaveFormat(i int, f sheet.CondFormat) error {
	m := h.m
	var err error
	if i < 0 {
		err = m.sheet.AddCondFormat(f)
	} else {
		err = m.sheet.SetCondFormat(i, f)
	}
	if err == nil {
		m.changed = true
		if i < 0 {
			m.recordRule("format.conditional_add", f.JSON())
		} else {
			m.recordRule("format.conditional_set", numbered(i, f.JSON()))
		}
	}
	return err
}

// Reworked follows a rule removed or moved in the panel, recorded as the
// command id that does the same, answered with answer.
func (h host) Reworked(id, answer string) {
	h.m.changed = true
	h.m.recordRule(id, answer)
}

// noteUnrecorded notes in a recording the last undo step, a change no
// command replays, as the named ranges picker makes.
func (m *Model) noteUnrecorded() {
	if r := m.rec; r != nil && r.depth == 0 && r.pending == "" {
		r.flush(m)
		r.actions = append(r.actions, macro.Note("Not recorded: "+m.book().UndoLabel()))
		r.acted = true
	}
}

// SaveValidation adds or replaces a validation rule, refusing one over a
// pivot table's results, whose cells take no entries; an added one is
// recorded as the command that adds it.
func (h host) SaveValidation(i int, v sheet.Validation) error {
	m := h.m
	if h.refused(v.Ranges) {
		return errors.New("Data validation can't go on a pivot table's results")
	}
	var err error
	if i < 0 {
		err = m.sheet.AddValidation(v)
	} else {
		err = m.sheet.SetValidation(i, v)
	}
	if err == nil {
		m.changed = true
		if i < 0 {
			m.recordRule("data.validation_add", v.JSON())
		} else {
			m.recordRule("data.validation_set", numbered(i, v.JSON()))
		}
	}
	return err
}

// recordRule records a change the panel made to rules as the command
// that makes it, answered with a rule's line or number (rulemacro.go). A
// command that changes one itself is recorded where it runs.
func (m *Model) recordRule(id, answer string) {
	if m.rec == nil || m.rec.depth > 0 || m.rec.pending != "" {
		return
	}
	m.rec.flush(m)
	m.rec.add(m, macro.Call("run", id).With("answer", macro.JSON(answer)))
}
