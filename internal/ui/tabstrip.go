package ui

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// The tab strip at the left of the status line, the way tmux lists
// windows: the sheet shown is highlighted, a + adds a sheet, and when
// the tabs don't all fit, ‹ and › step through them. Clicking a tab shows
// it, double-clicking renames it, right-clicking opens its menu, and
// dragging it moves it.

// tabStrip is the sheet tabs' state: where each sheet was left, and how
// far the strip is scrolled.
type tabStrip struct {
	places map[*sheet.Sheet]place // where each sheet's cursor and scroll were left
	left   int                    // the first tab shown when they don't all fit
}

// place is where a sheet's cursor and scroll position were when it was
// last shown.
type place struct {
	cur       sheet.Addr
	top, left int
}

// maxTabName is how much of a long sheet name a tab shows.
const maxTabName = 20

// The widths of the strip's fixed parts: " + ", and "‹" or "›" with the
// space after it.
const tabAddW, tabArrowW = 3, 2

// tabView is what the tab strip shows: the sheets, the one shown, and
// what the mouse is over or dragging.
type tabView struct {
	sheets []*sheet.Sheet
	active int
	hover  hit
	drag   dragKind
}

func (m *Model) tabView() tabView {
	sheets := m.visibleSheets()
	return tabView{sheets: sheets, active: slices.Index(sheets, m.sheet), hover: m.mouse.hover, drag: m.mouse.drag}
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

// minWidth is the narrowest the tab strip gets: the shown sheet's tab,
// the arrows and the +.
func (v tabView) minWidth() int {
	w := ansi.StringWidth(tabLabel(v.sheets[v.active])) + 4
	if len(v.sheets) > 1 {
		w += 4
	}
	return w
}

// fullWidth is how wide the tab strip is with every tab showing.
func (v tabView) fullWidth() int {
	w := tabAddW
	for _, s := range v.sheets {
		w += ansi.StringWidth(tabLabel(s)) + 1
	}
	return w
}

// style is the style of tab i: the one shown, one under the mouse (or
// where a dragged tab would go), or plain.
func (v tabView) style(th *theme.Theme, i int) lipgloss.Style {
	switch {
	case i == v.active:
		return th.TabActive
	case v.hover.kind == hitTab && v.hover.addr.Col == i && (v.drag == dragNone || v.drag == dragTab):
		return th.TabHover
	}
	return th.Tab
}

// layout lays out the tabs in room columns: as many as fit, always the
// shown one, the first shown staying put until the shown one would fall
// off (t.left), ‹ and › where tabs are hidden, then the +.
func (t *tabStrip) layout(th *theme.Theme, v tabView, room int) (string, []tabSpan) {
	widths := make([]int, len(v.sheets))
	for i, s := range v.sheets {
		widths[i] = ansi.StringWidth(tabLabel(s))
	}
	first, last := t.scroll(widths, v.active, room)
	var w tabWriter
	if first > 0 {
		w.part(hitTabPrev, 0, th.Muted.Render("‹"))
	}
	for i := first; i <= last; i++ {
		label := tabLabel(v.sheets[i])
		if i == v.active && w.x+ansi.StringWidth(label)+tabAddW > room {
			label = ansi.Truncate(label, max(room-w.x-tabAddW, 3), "…")
		}
		w.part(hitTab, i, v.style(th, i).Render(label))
	}
	if last < len(v.sheets)-1 {
		w.part(hitTabNext, 0, th.Muted.Render("›"))
	}
	addStyle := th.Muted
	if v.hover.kind == hitTabAdd {
		addStyle = th.TabHover
	}
	w.part(hitTabAdd, 0, addStyle.Render(" + "))
	return strings.TrimSuffix(w.b.String(), " "), w.spans
}

// scroll picks the first and last tab shown, keeping the first where it
// was unless the active tab would fall off, and showing tabs to the left
// again when there's room, e.g. once the screen is wider.
func (t *tabStrip) scroll(widths []int, active, room int) (first, last int) {
	first = clamp(t.left, 0, active)
	last, ok := tabsFit(widths, first, active, room)
	for !ok && first < active {
		first++
		last, ok = tabsFit(widths, first, active, room)
	}
	for first > 0 {
		l, ok := tabsFit(widths, first-1, active, room)
		if !ok || l < last {
			break
		}
		first, last = first-1, l
	}
	t.left = first
	return first, max(last, active) // a name too long for the room is cut
}

// tabsFit returns the last tab that fits in room when the strip starts
// at tab first, and whether the active tab is among those shown.
func tabsFit(widths []int, first, active, room int) (last int, ok bool) {
	w := tabAddW
	if first > 0 {
		w += tabArrowW
	}
	last = first - 1
	for i := first; i < len(widths); i++ {
		more := 0
		if i < len(widths)-1 {
			more = tabArrowW
		}
		if w+widths[i]+1+more > room {
			break
		}
		w += widths[i] + 1
		last = i
	}
	return last, last >= active
}

// tabWriter writes the parts of the tab strip, noting where each is.
type tabWriter struct {
	b     strings.Builder
	spans []tabSpan
	x     int
}

func (w *tabWriter) part(kind hitKind, index int, text string) {
	width := ansi.StringWidth(text)
	w.spans = append(w.spans, tabSpan{kind: kind, index: index, x: w.x, w: width})
	w.b.WriteString(text + " ")
	w.x += width + 1
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
