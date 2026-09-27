package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/tabstrip"
)

// Sheets work as in Google Sheets: Ctrl+PgDn and Ctrl+PgUp (or Alt+Right
// and Alt+Left, for terminals that keep Ctrl+PgUp/PgDn for their own
// tabs) move between sheets, Shift+F11 adds one, and each sheet remembers
// its cursor and scroll position. The tabs sit at the left of the status
// line (tabstrip.go).
//
// While a formula is being typed, switching sheets points into the other
// sheet, as clicking a tab does in Sheets: the entry stays with its cell
// (entry.home), and the pointer inserts references such as Sheet2!A1.
// Enter stores the formula and returns to its sheet.

func init() {
	register(
		&command{id: "sheet.new", title: "New sheet", desc: "Add an empty sheet after the others", run: func(m *Model) tea.Cmd {
			s, err := m.book().AddSheet("", m.book().Len())
			if err != nil {
				m.fail(err.Error())
				return nil
			}
			m.showSheet(s)
			m.note = "Added " + s.Name()
			return nil
		}},
		&command{id: "sheet.next", macro: macroView, title: "Next sheet", desc: "Show the sheet to the right", enabled: func(m *Model) bool { return m.sheetAt(1) != nil },
			run: func(m *Model) tea.Cmd { return m.stepSheet(1) }},
		&command{id: "sheet.prev", macro: macroView, title: "Previous sheet", desc: "Show the sheet to the left", enabled: func(m *Model) bool { return m.sheetAt(-1) != nil },
			run: func(m *Model) tea.Cmd { return m.stepSheet(-1) }},
		&command{id: "sheet.goto", macro: macroView, title: "Go to sheet", desc: "Pick a sheet by name", run: func(m *Model) tea.Cmd {
			m.openSheetPicker()
			return nil
		}},
		&command{id: "sheet.rename", title: "Rename sheet", desc: "Give the sheet a new name; formulas that use it follow", run: func(m *Model) tea.Cmd {
			m.openRename()
			return nil
		}},
		&command{id: "sheet.duplicate", title: "Duplicate sheet", desc: "Copy the sheet, with its formatting and charts, to a new sheet after it", run: func(m *Model) tea.Cmd {
			cp, err := m.book().DuplicateSheet(m.sheet)
			if err != nil {
				m.fail(err.Error())
				return nil
			}
			m.showSheet(cp)
			m.note = "Duplicated as " + cp.Name()
			return nil
		}},
		&command{id: "sheet.delete", title: "Delete sheet", desc: "Delete the sheet; formulas that use it show #REF!",
			enabled: func(m *Model) bool { return len(m.book().Visible()) > 1 }, run: (*Model).confirmDeleteSheet},
		&command{id: "sheet.move_left", title: "Move sheet left", desc: "Move the sheet's tab one place left", enabled: func(m *Model) bool { return m.sheetAt(-1) != nil },
			run: func(m *Model) tea.Cmd { return m.moveSheet(-1) }},
		&command{id: "sheet.move_right", title: "Move sheet right", desc: "Move the sheet's tab one place right", enabled: func(m *Model) bool { return m.sheetAt(1) != nil },
			run: func(m *Model) tea.Cmd { return m.moveSheet(1) }},
	)
	keymap["shift+f11"] = "sheet.new"
	keymap["ctrl+pgdown"] = "sheet.next"
	keymap["alt+right"] = "sheet.next"
	keymap["ctrl+pgup"] = "sheet.prev"
	keymap["alt+left"] = "sheet.prev"
	keymap["alt+shift+k"] = "sheet.goto"
	keyAliases["alt+K"] = "alt+shift+k"
}

// tabMenu is the menu of a sheet's tab, as in Sheets.
var tabMenu = []menuItem{
	{cmd: "sheet.rename", title: "Rename"}, {cmd: "sheet.duplicate", title: "Duplicate"}, {cmd: "sheet.delete", title: "Delete"}, {cmd: "sheet.hide"}, sep,
	{cmd: "sheet.move_left", title: "Move left"}, {cmd: "sheet.move_right", title: "Move right"}, sep,
	{cmd: "sheet.new"}, {cmd: "sheet.goto"}, {cmd: "sheet.unhide"},
}

// sheetAt returns the sheet d tabs from the one shown, skipping hidden
// sheets, or nil past the ends: moving between sheets doesn't wrap
// around, as in Sheets.
func (g *grid) sheetAt(d int) *sheet.Sheet {
	book, step, n := g.book(), 1, d
	if d < 0 {
		step, n = -1, -d
	}
	for i := book.Index(g.sheet) + step; i >= 0 && i < book.Len() && n > 0; i += step {
		if s := book.Sheet(i); !s.Hidden() {
			if n--; n == 0 {
				return s
			}
		}
	}
	return nil
}

func (m *Model) stepSheet(d int) tea.Cmd {
	if s := m.sheetAt(d); s != nil {
		m.switchTo(s)
	}
	return nil
}

// moveSheet moves the sheet shown past its visible neighbour d tabs
// away (hidden sheets between keep their order).
func (m *Model) moveSheet(d int) tea.Cmd {
	if s := m.sheetAt(d); s != nil {
		m.book().MoveSheet(m.sheet, m.book().Index(s))
	}
	return nil
}

// switchTo shows s: plainly in READY, or pointing into it while a formula
// is typed.
func (m *Model) switchTo(s *sheet.Sheet) {
	if m.editing() {
		m.pointInto(s)
		return
	}
	m.showSheet(s)
}

// showSheet shows s where it was left, remembering where the sheet shown
// now is. The selection doesn't carry over; the copy marker and a trace
// stay with their own sheets.
func (m *Model) showSheet(s *sheet.Sheet) {
	if s == m.sheet || s == nil {
		return
	}
	m.leave()
	m.sheet = s
	p := m.tabs.Places[s]
	m.cur, m.top, m.left = p.Cur, p.Top, p.Left
	m.clearSelection()
	m.charts.last = -1
	m.book().SetActive(s)
}

// leave remembers where the sheet shown is, before another is shown.
func (m *Model) leave() {
	if m.tabs.Places == nil {
		m.tabs.Places = map[*sheet.Sheet]tabstrip.Place{}
	}
	cur := m.cur
	if m.away() {
		cur = m.point.at // the entry's cell is on another sheet
	}
	m.tabs.Places[m.sheet] = tabstrip.Place{Cur: cur, Top: m.top, Left: m.left}
}

// away reports whether an entry is being typed for a cell on another
// sheet than the one shown.
func (m *Model) away() bool { return m.entry.home != nil && m.entry.home != m.sheet }

// entrySheet is the sheet the entry being typed goes to.
func (m *Model) entrySheet() *sheet.Sheet {
	if m.entry.home != nil {
		return m.entry.home
	}
	return m.sheet
}

// pointInto shows s while a formula is being typed, pointing at the cell
// last active there so arrows go on from it. An entry that isn't a
// formula ready for a reference is stored first, as a click elsewhere
// would.
func (m *Model) pointInto(s *sheet.Sheet) {
	if s == m.sheet {
		return
	}
	if m.mode != modePoint {
		if !m.line.IsFormula() || !m.canPoint() {
			if m.commit() {
				m.showSheet(s)
			}
			return
		}
		m.entry.prefix = m.line.Head()
		m.entry.suffix = m.line.Tail()
		m.mode = modePoint
	}
	if m.entry.home == nil {
		m.entry.home = m.sheet
	}
	cur := m.cur
	m.leave()
	m.sheet = s
	p := m.tabs.Places[s]
	if s == m.entry.home {
		p.Cur = cur // back home, pointing starts at the entry's cell
	}
	m.top, m.left = p.Top, p.Left
	m.cur = cur
	m.point = pointer{at: p.Cur}
	m.book().SetActive(s)
}

// returnHome shows the entry's sheet again once the entry is stored or
// cancelled.
func (m *Model) returnHome() {
	home := m.entry.home
	m.entry.home = nil
	if home == nil || home == m.sheet || !home.Live() {
		return
	}
	cur := m.cur
	m.tabs.Places[m.sheet] = tabstrip.Place{Cur: m.point.at, Top: m.top, Left: m.left}
	m.sheet = home
	p := m.tabs.Places[home]
	m.cur, m.top, m.left = cur, p.Top, p.Left
	m.book().SetActive(home)
}

// pointRef is the reference being pointed at as it goes into the formula:
// with the sheet's name when it's on another sheet than the entry's.
func (m *Model) pointRef() string {
	if m.away() {
		return sheet.Qualified(m.sheet.Name(), m.point.rect())
	}
	return m.point.text()
}

// afterSheetsChange keeps a sheet on screen after undo, redo or a delete
// took the shown one away, and forgets deleted sheets' places.
func (m *Model) afterSheetsChange(prefer *sheet.Sheet, index int) {
	book := m.book()
	switch {
	case prefer != nil && prefer.Live() && !prefer.Hidden():
		m.showSheet(prefer)
	case !m.sheet.Live():
		s := m.nearVisible(clamp(index, 0, book.Len()-1))
		m.sheet = s // the old sheet is gone: nothing to remember of it
		p := m.tabs.Places[s]
		m.cur, m.top, m.left = p.Cur, p.Top, p.Left
		m.clearSelection()
		m.charts.last = -1
		book.SetActive(s)
	case m.sheet.Hidden():
		m.showSheet(m.nearVisible(book.Index(m.sheet)))
	}
}

// confirmDeleteSheet deletes the sheet shown, asking first when it has
// contents. Undo brings it back either way.
func (m *Model) confirmDeleteSheet() tea.Cmd {
	s := m.sheet
	del := func(m *Model) tea.Cmd {
		i := m.book().Index(s)
		if err := m.book().DeleteSheet(s); err != nil {
			m.fail(err.Error())
			return nil
		}
		m.afterSheetsChange(nil, i)
		m.note = "Deleted " + s.Name() + "; Ctrl+Z brings it back"
		return nil
	}
	n := s.Len()
	if n == 0 && len(s.Charts()) == 0 {
		return del(m)
	}
	what := "Delete " + s.Name() + " and its " + cellCount(n) + "?"
	if n == 0 {
		what = "Delete " + s.Name() + " and its charts?"
	}
	m.openOverlay(&choiceBar{m: m, msg: what, warn: true, choices: []choice{
		{key: "enter", label: "Delete", run: del},
		{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
	}})
	return nil
}

// openRename asks for the sheet's new name on the context line.
func (m *Model) openRename() {
	s := m.sheet
	m.openText("Rename sheet:", s.Name(), func(m *Model, text string) tea.Cmd {
		old := s.Name()
		if err := m.book().RenameSheet(s, text); err != nil {
			m.fail(err.Error())
			return nil
		}
		if old != s.Name() {
			m.note = "Renamed " + old + " to " + s.Name()
		}
		return nil
	})
}

// openSheetPicker lists the sheets, fzf style, with the one shown
// highlighted.
func (m *Model) openSheetPicker() {
	var items []picker.Item
	sel := 0
	for i, s := range m.visibleSheets() {
		if s == m.sheet {
			sel = i
		}
		detail := "empty"
		if used, ok := s.UsedRange(); ok {
			detail = used.String() + ", " + cellCount(s.Len())
		}
		items = append(items, picker.Item{
			Title: s.Name(), Name: len(s.Name()), Detail: detail, Desc: "Show " + s.Name(),
			Pick: func() tea.Cmd {
				m.closeOverlay()
				m.showSheet(s)
				return nil
			},
		})
	}
	p := m.newPicker("Go to sheet", "Type a sheet name", 60, items)
	p.Action = "show"
	p.Sel = sel
	m.openOverlay(p)
}

// sheetKey runs the commands that switch sheets while an entry is typed,
// which point into the other sheet (see switchTo).
func (m *Model) sheetKey(key string) bool {
	switch id := keymap[key]; id {
	case "sheet.next", "sheet.prev":
		m.runCommand(id)
		return true
	}
	return false
}
