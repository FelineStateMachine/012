package ui

import (
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/tabstrip"
)

// The sheet tabs (package tabstrip) as the model shows them and what
// clicking them does: clicking a tab shows it, double-clicking renames
// it, right-clicking opens its menu, and dragging it moves it.

// tabView is what the tab strip shows: the sheets, the one shown, and
// what the mouse is over or dragging.
func (m *Model) tabView() tabstrip.View {
	sheets := m.visibleSheets()
	v := tabstrip.View{Sheets: sheets, Active: slices.Index(sheets, m.sheet), Busy: m.mouse.drag != dragNone && m.mouse.drag != dragTab}
	switch m.mouse.hover.kind {
	case hitTab:
		v.Hover, v.HoverIndex = tabstrip.Tab, m.mouse.hover.addr.Col
	case hitTabAdd:
		v.Hover = tabstrip.Add
	}
	return v
}

// tabHits are the hits of the parts of the tab strip.
var tabHits = map[tabstrip.Kind]hitKind{tabstrip.Tab: hitTab, tabstrip.Add: hitTabAdd, tabstrip.Prev: hitTabPrev, tabstrip.Next: hitTabNext}

// tabAt returns what part of the tab strip is at x on the status line.
func (m *Model) tabAt(x int) (hit, bool) {
	_, spans := m.statusLayout()
	for _, sp := range spans {
		if x >= sp.X && x < sp.X+sp.W {
			return hit{kind: tabHits[sp.Kind], addr: sheet.Addr{Col: sp.Index}}, true
		}
	}
	return hit{}, false
}

// tabPress handles a click on the tab strip: show the tab (renaming it on
// a double click, or starting to drag it), add a sheet, or step through
// tabs with the arrows.
func (m *Model) tabPress(h hit, double bool) tea.Cmd {
	switch h.kind {
	case hitTabPrev:
		return m.runCommand("sheet.prev")
	case hitTabNext:
		return m.runCommand("sheet.next")
	case hitTabAdd:
		if m.mode == modePoint {
			m.resumeEntry(m.pointRef())
		}
		if m.editing() && !m.commit() {
			return nil
		}
		return m.runCommand("sheet.new")
	case hitTab:
		s := m.tabSheet(h.addr.Col)
		if m.editing() {
			m.pointInto(s)
			return nil
		}
		m.showSheet(s)
		if double {
			return m.runCommand("sheet.rename")
		}
		m.mouse.drag = dragTab
	}
	return nil
}

// dropTab moves the dragged sheet to the tab it was released over.
func (m *Model) dropTab() {
	if m.mouse.hover.kind != hitTab {
		return
	}
	if to := m.book().Index(m.tabSheet(m.mouse.hover.addr.Col)); to != m.book().Index(m.sheet) {
		m.book().MoveSheet(m.sheet, to)
		m.note = "Moved " + m.sheet.Name() + " to position " + strconv.Itoa(to+1)
		m.record(macro.Call("move_sheet", to+1))
	}
}

// tabSheet is the sheet of tab i, counting the tabs shown (hidden sheets
// have none).
func (m *Model) tabSheet(i int) *sheet.Sheet {
	tabs := m.visibleSheets()
	return tabs[clamp(i, 0, len(tabs)-1)]
}

// tabRightClick shows the tab's sheet and opens its menu.
func (m *Model) tabRightClick(h hit, x, y int) {
	if h.kind != hitTab {
		if h.kind == hitTabAdd {
			m.showContextMenu([]menuItem{{cmd: "sheet.new"}, {cmd: "sheet.goto"}}, x, y)
		}
		return
	}
	m.showSheet(m.tabSheet(h.addr.Col))
	m.showContextMenu(tabMenu, x, y)
}
