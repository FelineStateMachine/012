package ui

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Sheets work as in Google Sheets: Ctrl+PgDn and Ctrl+PgUp (or Alt+Right
// and Alt+Left, for terminals that keep Ctrl+PgUp/PgDn for their own
// tabs) move between sheets, Shift+F11 adds one, and each sheet remembers
// its cursor and scroll position. The tabs sit at the left of the status
// line, the way tmux lists windows: the sheet shown is highlighted, a +
// adds a sheet, and when they don't all fit, ‹ and › step through them.
// Clicking a tab shows it, double-clicking renames it, right-clicking
// opens its menu, and dragging it moves it.
//
// While a formula is being typed, switching sheets points into the other
// sheet, as clicking a tab does in Sheets: the entry stays with its cell
// (m.home), and the pointer inserts references such as Sheet2!A1. Enter
// stores the formula and returns to its sheet.

// place is where a sheet's cursor and scroll position were when it was
// last shown.
type place struct {
	cur       sheet.Addr
	top, left int
}

// maxTabName is how much of a long sheet name a tab shows.
const maxTabName = 20

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
		&command{id: "sheet.next", title: "Next sheet", desc: "Show the sheet to the right", enabled: func(m *Model) bool { return m.sheetAt(1) != nil },
			run: func(m *Model) tea.Cmd { return m.stepSheet(1) }},
		&command{id: "sheet.prev", title: "Previous sheet", desc: "Show the sheet to the left", enabled: func(m *Model) bool { return m.sheetAt(-1) != nil },
			run: func(m *Model) tea.Cmd { return m.stepSheet(-1) }},
		&command{id: "sheet.goto", title: "Go to sheet", desc: "Pick a sheet by name", run: func(m *Model) tea.Cmd {
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
			enabled: func(m *Model) bool { return m.book().Len() > 1 }, run: (*Model).confirmDeleteSheet},
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
	{cmd: "sheet.rename", title: "Rename"}, {cmd: "sheet.duplicate", title: "Duplicate"}, {cmd: "sheet.delete", title: "Delete"}, sep,
	{cmd: "sheet.move_left", title: "Move left"}, {cmd: "sheet.move_right", title: "Move right"}, sep,
	{cmd: "sheet.new"}, {cmd: "sheet.goto"},
}

func (m *Model) book() *sheet.Workbook { return m.sheet.Book() }

// sheetAt returns the sheet d tabs from the one shown, or nil past the
// ends: moving between sheets doesn't wrap around, as in Sheets.
func (m *Model) sheetAt(d int) *sheet.Sheet {
	i := m.book().Index(m.sheet) + d
	if i < 0 || i >= m.book().Len() {
		return nil
	}
	return m.book().Sheet(i)
}

func (m *Model) stepSheet(d int) tea.Cmd {
	if s := m.sheetAt(d); s != nil {
		m.switchTo(s)
	}
	return nil
}

func (m *Model) moveSheet(d int) tea.Cmd {
	i := m.book().Index(m.sheet)
	m.book().MoveSheet(m.sheet, i+d)
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
	p := m.places[s]
	m.cur, m.top, m.left = p.cur, p.top, p.left
	m.clearSelection()
	m.lastChart = -1
	m.book().SetActive(s)
}

// leave remembers where the sheet shown is, before another is shown.
func (m *Model) leave() {
	if m.places == nil {
		m.places = map[*sheet.Sheet]place{}
	}
	cur := m.cur
	if m.away() {
		cur = m.point.at // the entry's cell is on another sheet
	}
	m.places[m.sheet] = place{cur: cur, top: m.top, left: m.left}
}

// away reports whether an entry is being typed for a cell on another
// sheet than the one shown.
func (m *Model) away() bool { return m.home != nil && m.home != m.sheet }

// entrySheet is the sheet the entry being typed goes to.
func (m *Model) entrySheet() *sheet.Sheet {
	if m.home != nil {
		return m.home
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
		if !m.isFormula() || !m.canPoint() {
			if m.commit() {
				m.showSheet(s)
			}
			return
		}
		m.pointPrefix = string(m.buf[:m.bufPos])
		m.pointSuffix = string(m.buf[m.bufPos:])
		m.mode = modePoint
	}
	if m.home == nil {
		m.home = m.sheet
	}
	cur := m.cur
	m.leave()
	m.sheet = s
	p := m.places[s]
	if s == m.home {
		p.cur = cur // back home, pointing starts at the entry's cell
	}
	m.top, m.left = p.top, p.left
	m.cur = cur
	m.point = pointer{at: p.cur}
	m.book().SetActive(s)
}

// returnHome shows the entry's sheet again once the entry is stored or
// cancelled.
func (m *Model) returnHome() {
	home := m.home
	m.home = nil
	if home == nil || home == m.sheet || !home.Live() {
		return
	}
	cur := m.cur
	m.places[m.sheet] = place{cur: m.point.at, top: m.top, left: m.left}
	m.sheet = home
	p := m.places[home]
	m.cur, m.top, m.left = cur, p.top, p.left
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
	case prefer != nil && prefer.Live():
		m.showSheet(prefer)
	case !m.sheet.Live():
		s := book.Sheet(clamp(index, 0, book.Len()-1))
		m.sheet = s // the old sheet is gone: nothing to remember of it
		p := m.places[s]
		m.cur, m.top, m.left = p.cur, p.top, p.left
		m.clearSelection()
		m.lastChart = -1
		book.SetActive(s)
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
	m.openOverlay(&choiceBar{msg: what, warn: true, choices: []choice{
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
	var items []pickItem
	sel := 0
	for i, s := range m.book().Sheets() {
		if s == m.sheet {
			sel = i
		}
		detail := "empty"
		if used, ok := s.UsedRange(); ok {
			detail = used.String() + ", " + cellCount(s.Len())
		}
		items = append(items, pickItem{
			title: s.Name(), name: len(s.Name()), detail: detail, desc: "Show " + s.Name(),
			pick: func(m *Model) tea.Cmd {
				m.closeOverlay()
				m.showSheet(s)
				return nil
			},
		})
	}
	p := newPicker(m, "Go to sheet", "Type a sheet name", 60, items)
	p.action = "show"
	p.sel = sel
	m.openOverlay(p)
}

// sheetKey handles the keys that switch sheets while an entry is typed.
func (m *Model) sheetKey(key string) bool {
	switch keymap[key] {
	case "sheet.next":
		if s := m.sheetAt(1); s != nil {
			m.pointInto(s)
		}
	case "sheet.prev":
		if s := m.sheetAt(-1); s != nil {
			m.pointInto(s)
		}
	default:
		return false
	}
	return true
}

// tabSpan is a clickable part of the tab strip on the status line.
type tabSpan struct {
	kind  hitKind // hitTab, hitTabAdd, hitTabPrev or hitTabNext
	index int     // the sheet, for hitTab
	x, w  int
}

// tabLabel is a sheet's name as its tab shows it.
func tabLabel(s *sheet.Sheet) string {
	return " " + ansi.Truncate(s.Name(), maxTabName, "…") + " "
}

// minTabs is the narrowest the tab strip gets: the shown sheet's tab,
// the arrows and the +.
func (m *Model) minTabs() int {
	w := ansi.StringWidth(tabLabel(m.sheet)) + 4
	if m.book().Len() > 1 {
		w += 4
	}
	return w
}

// allTabs is how wide the tab strip is with every tab showing.
func (m *Model) allTabs() int {
	w := 3 // the +
	for _, s := range m.book().Sheets() {
		w += ansi.StringWidth(tabLabel(s)) + 1
	}
	return w
}

// tabStrip lays out the tabs in room columns from the left of the status
// line: as many as fit, always the shown one, the first shown staying put
// until the shown one would fall off (m.tabLeft), ‹ and › where tabs are
// hidden, then the +.
func (m *Model) tabStrip(room int) (string, []tabSpan) {
	sheets := m.book().Sheets()
	active := m.book().Index(m.sheet)
	widths := make([]int, len(sheets))
	for i, s := range sheets {
		widths[i] = ansi.StringWidth(tabLabel(s))
	}
	const add, arrow = 3, 2 // " + ", and "‹" or "›" with the space after it
	fits := func(first int) (last int, ok bool) {
		w := add
		if first > 0 {
			w += arrow
		}
		last = first - 1
		for i := first; i < len(sheets); i++ {
			more := 0
			if i < len(sheets)-1 {
				more = arrow
			}
			if w+widths[i]+1+more > room {
				break
			}
			w += widths[i] + 1
			last = i
		}
		return last, last >= active
	}
	first := clamp(m.tabLeft, 0, active)
	last, ok := fits(first)
	for !ok && first < active {
		first++
		last, ok = fits(first)
	}
	// Show tabs to the left again when there's room, e.g. once the
	// screen is wider.
	for first > 0 {
		l, ok := fits(first - 1)
		if !ok || l < last {
			break
		}
		first, last = first-1, l
	}
	m.tabLeft = first
	if last < active {
		last = active // a name too long for the room is cut below
	}

	var b strings.Builder
	var spans []tabSpan
	x := 0
	part := func(kind hitKind, index int, text string) {
		spans = append(spans, tabSpan{kind: kind, index: index, x: x, w: ansi.StringWidth(text)})
		b.WriteString(text + " ")
		x += ansi.StringWidth(text) + 1
	}
	if first > 0 {
		part(hitTabPrev, 0, m.th.muted.Render("‹"))
	}
	for i := first; i <= last; i++ {
		style := m.th.tab
		switch {
		case i == active:
			style = m.th.tabActive
		case m.hover.kind == hitTab && m.hover.addr.Col == i && (m.drag == dragNone || m.drag == dragTab):
			style = m.th.tabHover
		}
		label := tabLabel(sheets[i])
		if i == active && x+ansi.StringWidth(label)+add > room {
			label = ansi.Truncate(label, max(room-x-add, 3), "…")
		}
		part(hitTab, i, style.Render(label))
	}
	if last < len(sheets)-1 {
		part(hitTabNext, 0, m.th.muted.Render("›"))
	}
	addStyle := m.th.muted
	if m.hover.kind == hitTabAdd {
		addStyle = m.th.tabHover
	}
	part(hitTabAdd, 0, addStyle.Render(" + "))
	return strings.TrimSuffix(b.String(), " "), spans
}

// tabAt returns the part of the tab strip at x on the status line.
func (m *Model) tabAt(x int) (tabSpan, bool) {
	_, spans := m.statusLayout()
	for _, sp := range spans {
		if x >= sp.x && x < sp.x+sp.w {
			return sp, true
		}
	}
	return tabSpan{}, false
}

// tabPress handles a click on the tab strip: show the tab (renaming it on
// a double click, or starting to drag it), add a sheet, or step through
// tabs with the arrows.
func (m *Model) tabPress(h hit, double bool) tea.Cmd {
	switch h.kind {
	case hitTabPrev:
		m.stepEntrySheet(-1)
	case hitTabNext:
		m.stepEntrySheet(1)
	case hitTabAdd:
		if m.mode == modePoint {
			m.resumeEntry(m.pointRef())
		}
		if m.editing() && !m.commit() {
			return nil
		}
		return m.runCommand("sheet.new")
	case hitTab:
		s := m.book().Sheet(h.addr.Col)
		if m.editing() {
			m.pointInto(s)
			return nil
		}
		m.showSheet(s)
		if double {
			m.openRename()
			return nil
		}
		m.drag = dragTab
	}
	return nil
}

func (m *Model) stepEntrySheet(d int) {
	if s := m.sheetAt(d); s != nil {
		m.switchTo(s)
	}
}

// dropTab moves the dragged sheet to the tab it was released over.
func (m *Model) dropTab() {
	if m.hover.kind == hitTab && m.hover.addr.Col != m.book().Index(m.sheet) {
		to := m.hover.addr.Col
		m.book().MoveSheet(m.sheet, to)
		m.note = "Moved " + m.sheet.Name() + " to position " + strconv.Itoa(to+1)
	}
}

// tabRightClick shows the tab's sheet and opens its menu.
func (m *Model) tabRightClick(h hit, x, y int) {
	if h.kind != hitTab {
		if h.kind == hitTabAdd {
			m.showContextMenu([]menuItem{{cmd: "sheet.new"}, {cmd: "sheet.goto"}}, x, y)
		}
		return
	}
	m.showSheet(m.book().Sheet(h.addr.Col))
	m.showContextMenu(tabMenu, x, y)
}
