package ui

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/picker"
)

// Tracing that stays on (Data > Formula tracing > Show precedents and
// dependents, Alt+;): as the pointer moves, the cells the active
// cell's formula reads are marked in the Precedent role and the
// formulas reading it in the Dependent role, and the context line lists
// both, those on other sheets or off screen included, with an arrow
// toward the ones off screen.
// Alt+' lists them in a picker to go to one, which then shows its own.
//
// The links are found once per cell and change (the engine's
// DependentLinks and PrecedentLinks), and each cell drawn asks the
// engine whether its formula reads the active cell (Reads), so drawing
// costs the cells on screen however many formulas read the cell. Off,
// it costs nothing: the model holds no traceView.

// traceView is what tracing shows of the active cell.
type traceView struct {
	key     traceViewKey
	home    *sheet.Sheet
	subject sheet.Rect // the cell, or the cells its formula spills into
	prec    []sheet.Link
	deps    []sheet.Link
	more    bool // dependents past maxListed left out
}

// traceViewKey is what the links were found for: the cell and the
// workbook's state and values.
type traceViewKey struct {
	s       *sheet.Sheet
	a       sheet.Addr
	state   int
	version uint64
}

func init() {
	register(
		&command{id: "view.trace", macro: macroView, title: "Show precedents and dependents",
			desc:    "Mark the cells the active cell's formula reads and the formulas that read it, as the pointer moves",
			checked: func(m *Model) bool { return m.tview != nil },
			run: func(m *Model) tea.Cmd {
				if m.tview != nil {
					m.tview = nil
					return nil
				}
				m.tview = &traceView{}
				m.syncTrace()
				return nil
			}},
		&command{id: "data.trace_list", macro: macroView, title: "Go to a precedent or dependent",
			desc: "List what the active cell reads and the formulas that read it, on any sheet, and go to one",
			run: func(m *Model) tea.Cmd {
				m.openTraceList()
				return nil
			}},
	)
	keymap["alt+;"] = "view.trace"
	keymap["alt+'"] = "data.trace_list"
}

// syncTrace finds the active cell's links again when it or the workbook
// changed.
func (m *Model) syncTrace() {
	v := m.tview
	if v == nil {
		return
	}
	k := traceViewKey{m.sheet, m.cur, m.sheet.StateID(), m.sheet.Version()}
	if k == v.key {
		return
	}
	*v = traceView{key: k, home: m.sheet, subject: sheet.Rect{From: m.cur, To: m.cur}}
	v.prec = m.sheet.PrecedentLinks(m.cur)
	v.deps, v.more = m.sheet.DependentLinks(m.cur, maxListed)
	if area, ok := m.sheet.SpillArea(m.cur); ok {
		v.subject = area
	}
}

// traceRole is the role a, on the sheet shown, is drawn in for a trace,
// or nil.
func (m *Model) traceRole(a sheet.Addr) *lipgloss.Style {
	if m.trace != nil {
		return m.trace.role(&m.th, m.sheet, a)
	}
	v := m.tview
	if v == nil {
		return nil
	}
	for _, l := range v.prec {
		if l.Sheet == m.sheet && l.Range.Contains(a) {
			return &m.th.Precedent
		}
	}
	if m.sheet == v.home && v.subject.Contains(a) || m.sheet.Reads(a, v.home, v.subject) {
		return &m.th.Dependent
	}
	return nil
}

// traceLine is the context line while tracing is on: "C1 reads A1,
// Data!B2:B9; read by D1 +3 more", with an arrow after cells off screen
// toward them, and the keys.
func (m *Model) traceLine() (left, right string) {
	v, th := m.tview, &m.th
	right = th.KeyHints(m.shortcut("data.trace_list"), "list", m.shortcut("view.trace"), "stop")
	name := m.cur.String()
	if len(v.prec) == 0 && len(v.deps) == 0 {
		return th.Muted.Render(name + " reads no cells and no formula reads it"), right
	}
	room := m.width - ansi.StringWidth(right) - 3
	if room < 40 {
		right, room = "", m.width
	}
	// Labels past what the line can show aren't made: a cell read by a
	// thousand formulas shows a few and counts the rest.
	shown := screenArea(m)
	labels := func(links []sheet.Link) []string {
		out := make([]string, min(len(links), room/3+1))
		for i := range out {
			out[i] = linkLabel(m.sheet, links[i], m.sheet) + shown.arrow(m.sheet, links[i])
		}
		return out
	}
	var b strings.Builder
	if len(v.prec) > 0 {
		lead := name + " reads "
		fit := room - len(lead)
		if len(v.deps) > 0 {
			fit = room/2 - len(lead)
		}
		b.WriteString(th.Muted.Render(lead) + listFit(th, labels(v.prec), len(v.prec), fit, false))
	}
	if len(v.deps) > 0 {
		lead := name + " is read by "
		if b.Len() > 0 {
			lead = "; read by "
		}
		fit := room - ansi.StringWidth(b.String()) - len(lead)
		b.WriteString(th.Muted.Render(lead) + listFit(th, labels(v.deps), len(v.deps), fit, v.more))
	}
	return b.String(), right
}

// area is the rows and columns on screen, frozen ones included.
type area struct {
	top, bottom, left, right int
	ok                       bool
}

func screenArea(m *Model) area {
	rows := slices.DeleteFunc(m.screenRows(), func(r int) bool { return r == divider })
	cols := slices.DeleteFunc(m.screenCols(), func(c int) bool { return c == divider })
	if len(rows) == 0 || len(cols) == 0 {
		return area{}
	}
	return area{slices.Min(rows), slices.Max(rows), slices.Min(cols), slices.Max(cols), true}
}

// arrow points toward a link on the sheet shown that is off screen, or
// is "".
func (sa area) arrow(shown *sheet.Sheet, l sheet.Link) string {
	if l.Sheet != shown || !sa.ok {
		return ""
	}
	r := l.Range
	switch {
	case r.To.Row < sa.top:
		return "↑"
	case r.From.Row > sa.bottom:
		return "↓"
	case r.To.Col < sa.left:
		return "←"
	case r.From.Col > sa.right:
		return "→"
	}
	return ""
}

// openTraceList opens a picker of the active cell's precedents, then its
// dependents; picking one goes to it.
func (m *Model) openTraceList() {
	prec := m.sheet.PrecedentLinks(m.cur)
	deps, more := m.sheet.DependentLinks(m.cur, maxListed)
	if len(prec) == 0 && len(deps) == 0 {
		m.note = m.cur.String() + " reads no cells and no formula reads it"
		return
	}
	var items []picker.Item
	for i, links := range [][]sheet.Link{prec, deps} {
		for _, l := range links {
			items = append(items, m.traceItem(l, i == 1))
		}
	}
	title := "Precedents and dependents of " + m.cur.String()
	if more {
		title += ", the first " + strconv.Itoa(maxListed) + " dependents"
	}
	p := m.newPicker(title, "Type a cell, range or name", 72, items)
	p.Action = "go to"
	m.openOverlay(p)
}

// traceItem is a picker row for a link: its label, and what it is to
// the active cell.
func (m *Model) traceItem(l sheet.Link, dependent bool) picker.Item {
	label := linkLabel(m.sheet, l, m.sheet)
	detail := "dependent"
	switch {
	case dependent && l.Kind == sheet.LinkSpill:
		detail = "dependent, the cells it spills into"
	case !dependent:
		detail = "precedent" + map[sheet.LinkKind]string{sheet.LinkName: ", named range " + l.Range.String(),
			sheet.LinkRegion: ", a region's table", sheet.LinkTable: ", a table's " + l.Range.String(), sheet.LinkSpill: ", the formula that spilled it",
			sheet.LinkFile: ", the linked file its rows come from", sheet.LinkOutput: ", the notebook cell its rows come from"}[l.Kind]
	}
	if l.Sheet != m.sheet && l.OnGrid() {
		detail += ", on " + l.Sheet.Name()
	}
	return picker.Item{Title: label, Name: len(label), Detail: detail, Desc: "Go to " + label,
		Pick: func() tea.Cmd {
			m.closeOverlay()
			m.goToLink(l)
			return nil
		}}
}

// goToLink shows a link: its cells selected, or a notebook's cell.
func (m *Model) goToLink(l sheet.Link) {
	if m.refuseHidden(l.Sheet) {
		return
	}
	m.showSheet(l.Sheet)
	if !l.OnGrid() {
		if v := m.viewOf(l.Sheet); v != nil {
			v.Select(l.Range.From.Row, false)
		}
		return
	}
	if l.Range.From == l.Range.To {
		m.clearSelection()
		m.cur = l.Range.From
		return
	}
	m.selectRect(l.Range)
}
