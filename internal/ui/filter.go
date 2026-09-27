package ui

import (
	"log/slog"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Filters follow Sheets' Data > Create a filter: the headers of the
// filtered range get a button (▾, ▼ once the column hides something),
// and clicking it or pressing Alt+Down in the column opens a picker in the
// manner of fzf: a condition on top, then the column's values with
// checkboxes, narrowed by a fuzzy search as you type. Rows that don't
// pass are hidden, not deleted.

func init() {
	register(
		&command{id: "data.filter", title: "Create a filter", desc: "Filter the rows of the selection, or of the data around the active cell; the first row holds the headers",
			run:     (*Model).createFilter,
			enabled: func(m *Model) bool { _, on := m.sheet.FilterRange(); return !on }},
		&command{id: "data.filter_column", title: "Filter by column", desc: "Choose which values of the active column show, or a condition they must meet",
			run: func(m *Model) tea.Cmd {
				m.openFilterPicker(m.cur.Col)
				return nil
			},
			enabled: func(m *Model) bool {
				r, on := m.sheet.FilterRange()
				return on && m.cur.Col >= r.From.Col && m.cur.Col <= r.To.Col
			}},
		&command{id: "data.filter_remove", title: "Remove filter", desc: "Remove the filter and show every row again",
			run: func(m *Model) tea.Cmd {
				m.sheet.RemoveFilter()
				m.changed = true
				return nil
			},
			enabled: func(m *Model) bool { _, on := m.sheet.FilterRange(); return on }},
	)
	keymap["alt+down"] = "data.filter_column"
}

// createFilter filters the selection, or the data around the active cell.
func (m *Model) createFilter() tea.Cmd {
	r := m.dataRange()
	m.sheet.CreateFilter(r)
	m.changed = true
	m.note = "Created a filter on " + r.String() + "   " + m.th.KeyHints(m.shortcut("data.filter_column"), "filter the active column")
	return nil
}

// dataRange is the range data commands act on: the selection when there
// is one, cut to the data in it, or else the block of data around the
// active cell.
func (g *grid) dataRange() sheet.Rect {
	if !g.hasRange() {
		return g.sheet.Region(g.cur)
	}
	r := g.selection()
	if used, ok := g.sheet.UsedRange(); ok {
		r.To.Row = max(min(r.To.Row, used.To.Row), r.From.Row)
		r.To.Col = max(min(r.To.Col, used.To.Col), r.From.Col)
	}
	return r
}

// filterMark is the filter button in column c's header, if c is in the
// filter's range, and whether the column's filter hides anything.
func (g *grid) filterMark(c int) (string, bool) {
	r, on := g.sheet.FilterRange()
	switch {
	case !on || c < r.From.Col || c > r.To.Col:
		return "", false
	case g.sheet.ColumnFiltered(c):
		return "▼", true
	}
	return "▾", false
}

// filterButtonX is the screen x of column c's filter button, or -1.
func (g *grid) filterButtonX(c int) int {
	name := sheet.ColName(c)
	w := g.sheet.ColWidth(c)
	if mark, _ := g.filterMark(c); mark == "" || w < len(name)+3 {
		return -1
	}
	lw := len(name) + 2
	return g.colStart(c) + (w-lw)/2 + lw - 1
}

// filterPicker is the values list and condition for one column: of the
// sheet's filter, or of a pivot table's (pivoteditor.go), which set what
// applying and cancelling do.
type filterPicker struct {
	title   string
	x       int // where the box goes, e.g. over its column
	onApply func(m *Model, cr sheet.Criteria)
	// onCancel, when set, runs after Esc or a click outside closes the
	// picker.
	onCancel func(m *Model)
	values   []sheet.FilterValue
	checked  map[string]bool
	cond     sheet.Condition
	field    int       // 0 is the search, 1 the condition's value
	fields   [2]string // the text of each field
	shown    []int     // values matching the search, best first
	list               // over the rows: "Select all", then shown
}

const filterID = "filter"

// Rows of the box: the top border, the condition, a separator, the
// search, a separator, then the list.
const filterFirstRow = 5

// openFilterPicker opens the picker for column col of the filter.
func (m *Model) openFilterPicker(col int) {
	r, on := m.sheet.FilterRange()
	if !on || col < r.From.Col || col > r.To.Col {
		return
	}
	title := "Filter " + sheet.ColName(col)
	if h := m.sheet.ShownText(sheet.Addr{Col: col, Row: r.From.Row}); h != "" {
		title += "  " + h
	}
	m.openValuesPicker(title, m.colStart(col), m.sheet.FilterValues(col), m.sheet.Filter().Cols[col].Cond,
		func(m *Model, cr sheet.Criteria) { m.filterColumn(col, cr) })
}

// openValuesPicker opens a filter picker titled title at screen column x
// over values, with cond as the condition, calling apply with the
// criteria chosen.
func (m *Model) openValuesPicker(title string, x int, values []sheet.FilterValue, cond sheet.Condition, apply func(*Model, sheet.Criteria)) *filterPicker {
	p := &filterPicker{title: title, x: x, onApply: apply, values: values, checked: map[string]bool{}, cond: cond}
	for _, v := range p.values {
		p.checked[v.Text] = v.Shown
	}
	p.fields[1] = p.cond.Arg
	m.openOverlay(p)
	m.line.clear()
	p.search()
	return p
}

// close closes the picker without applying it.
func (p *filterPicker) close(m *Model) {
	m.closeOverlay()
	if p.onCancel != nil {
		p.onCancel(m)
	}
}

func (p *filterPicker) indicator() string { return "FILTER" }

// focus moves editing to field i, keeping the other field's text.
func (p *filterPicker) focus(m *Model, i int) {
	p.fields[p.field] = m.line.text()
	p.field = i
	m.line.set(p.fields[i])
}

func (p *filterPicker) changed(m *Model) {
	p.fields[p.field] = m.line.text()
	if p.field == 0 {
		p.search()
	}
}

// valueLabel is how a value shows in the list.
func valueLabel(text string) string {
	if text == "" {
		return "(Blanks)"
	}
	return text
}

// search narrows the list to the values matching the search.
func (p *filterPicker) search() {
	p.sel, p.top = 0, 0
	p.shown = p.shown[:0]
	q := strings.TrimSpace(p.fields[0])
	if q == "" {
		for i := range p.values {
			p.shown = append(p.shown, i)
		}
		return
	}
	labels := make([]string, len(p.values))
	for i, v := range p.values {
		labels[i] = valueLabel(v.Text)
	}
	for _, mt := range fuzzy.Find(q, labels) {
		p.shown = append(p.shown, mt.Index)
	}
}

// toggle checks or unchecks list row i: "Select all" (row 0) checks every
// shown value, or unchecks them if they all are.
func (p *filterPicker) toggle(i int) {
	if i > 0 {
		t := p.values[p.shown[i-1]].Text
		p.checked[t] = !p.checked[t]
		return
	}
	all := p.allChecked()
	for _, k := range p.shown {
		p.checked[p.values[k].Text] = !all
	}
}

func (p *filterPicker) allChecked() bool {
	for _, k := range p.shown {
		if !p.checked[p.values[k].Text] {
			return false
		}
	}
	return true
}

// cycle steps the condition through Sheets' list.
func (p *filterPicker) cycle(d int) {
	n := len(sheet.CondOps())
	p.cond.Op = sheet.CondOp((int(p.cond.Op) + d + n) % n)
}

// apply sets the column's criteria and closes the picker.
func (p *filterPicker) apply(m *Model) {
	p.fields[p.field] = m.line.text()
	var cr sheet.Criteria
	for _, v := range p.values {
		if !p.checked[v.Text] {
			cr.Hidden = append(cr.Hidden, v.Text)
		}
	}
	cr.Cond = sheet.Condition{Op: p.cond.Op, Arg: strings.TrimSpace(p.fields[1])}
	if !cr.Cond.Op.TakesArg() {
		cr.Cond.Arg = ""
	} else if cr.Cond.Arg == "" {
		cr.Cond = sheet.Condition{}
	}
	m.closeOverlay()
	p.onApply(m, cr)
}

// filterColumn sets the criteria of column col of the sheet's filter.
func (m *Model) filterColumn(col int, cr sheet.Criteria) {
	span := telemetry.Start("filter")
	m.sheet.FilterColumn(col, cr)
	span.End(slog.Int("hidden", m.sheet.HiddenRows()))
	m.changed = true
	if n := m.sheet.HiddenRows(); n > 0 {
		m.note = "Filtered column " + sheet.ColName(col) + ": " + rowCount(n) + " hidden"
	} else {
		m.note = "The filter shows every row"
	}
}

func rowCount(n int) string {
	if n == 1 {
		return "1 row"
	}
	return strconv.Itoa(n) + " rows"
}

func (p *filterPicker) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
	n := len(p.shown) + 1
	switch key := k.String(); {
	case key == "esc":
		p.close(m)
	case key == "enter":
		p.apply(m)
	case key == "tab" || key == "shift+tab":
		p.focus(m, 1-p.field)
	case p.field == 1 && (key == "up" || key == "down"):
		if key == "up" {
			p.cycle(-1)
		} else {
			p.cycle(1)
		}
	case key == "up" || key == "ctrl+p":
		p.move(-1, n)
	case key == "down" || key == "ctrl+n":
		p.move(1, n)
	case key == "pgup":
		p.sel = max(p.sel-p.rows(m), 0)
	case key == "pgdown":
		p.sel = min(p.sel+p.rows(m), n-1)
	case p.field == 0 && key == "space":
		p.toggle(p.sel)
	default:
		before := m.line.text()
		m.line.key(k)
		if m.line.text() != before {
			p.changed(m)
		}
	}
	return nil
}

func (p *filterPicker) mouse(m *Model, e mouseEvent) tea.Cmd {
	if e.box != filterID {
		if e.kind == mousePress {
			p.close(m)
		}
		return nil
	}
	n := len(p.shown) + 1
	i := p.top + e.row - filterFirstRow
	switch {
	case e.kind == mouseWheel && e.button == tea.MouseWheelUp:
		p.move(-1, n)
	case e.kind == mouseWheel && e.button == tea.MouseWheelDown:
		p.move(1, n)
	case e.kind != mousePress && e.kind != mouseMotion:
	case e.row == 1 && e.kind == mousePress:
		if p.field != 1 {
			p.focus(m, 1)
		}
		switch x := e.col - 1 - len(" If "); {
		case x == 0:
			p.cycle(-1)
		case x == ansi.StringWidth(p.condChip())-1:
			p.cycle(1)
		}
	case e.row == 3 && e.kind == mousePress:
		p.focus(m, 0)
	case e.row < filterFirstRow || i >= n || i >= p.top+p.rows(m):
	case e.kind == mouseMotion:
		p.sel = i
	case e.button == tea.MouseLeft:
		p.sel = i
		p.toggle(i)
	}
	return nil
}

func (p *filterPicker) status(m *Model) (string, string) {
	if p.field == 1 {
		return "Rows must also meet the condition", m.th.KeyHints("Up/Down", "condition", "Tab", "values", "Enter", "apply", "Esc", "cancel")
	}
	pairs := []string{"Space", "check", "Tab", "condition", "Enter", "apply", "Esc", "cancel"}
	desc := "Type to search the values"
	for {
		keys := m.th.KeyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= m.width:
			return m.th.Muted.Render(desc), keys
		case desc != "":
			desc = ""
		case len(pairs) > 4:
			pairs = append(pairs[:2], pairs[4:]...)
		default:
			return "", keys
		}
	}
}

// rows is how many list rows show: all of them if they fit above the
// status line, at most twelve.
func (p *filterPicker) rows(m *Model) int {
	return max(min(len(p.shown)+1, 12, m.height-1-gridTop-filterFirstRow-1), 1)
}

func (p *filterPicker) box(m *Model) (x, y, inner int) {
	w := 0
	for _, v := range p.values {
		w = max(w, ansi.StringWidth(valueLabel(v.Text))+len(strconv.Itoa(v.Count)))
	}
	inner = clamp(w+10, 36, 48)
	inner = min(inner, m.width-2)
	h := p.rows(m) + filterFirstRow + 1
	x, y = m.clampBox(p.x, gridTop, inner+2, h)
	return x, min(y, max(m.height-1-h, 0)), inner
}

func (p *filterPicker) cursor(m *Model) (int, int) {
	x, y, _ := p.box(m)
	caret := ansi.StringWidth(m.line.head())
	if p.field == 1 {
		return x + 1 + len(" If ") + ansi.StringWidth(p.condChip()) + 2 + caret, y + 1
	}
	return x + 1 + ansi.StringWidth(searchPrompt) + caret, y + 3
}

// condChip is the condition's name between arrows that change it.
func (p *filterPicker) condChip() string {
	return "‹ " + p.cond.Op.Title() + " ›"
}

func (p *filterPicker) layout(m *Model) []box {
	x, y, inner := p.box(m)
	rows := p.rows(m)
	p.show(rows)

	chipStyle := m.th.KeyChip
	if p.field == 1 {
		chipStyle = m.th.MenuSelected
	}
	cond := m.th.Muted.Render(" If ") + chipStyle.Render(p.condChip())
	if p.cond.Op.TakesArg() {
		arg := p.fields[1]
		if p.field == 1 {
			arg = m.line.text()
		}
		if arg == "" && p.field != 1 {
			arg = m.th.Muted.Render("value")
		}
		cond += "  " + arg
	}
	search := p.fields[0]
	if p.field == 0 {
		search = m.line.text()
	}
	input := m.th.Title.Render(searchPrompt) + search
	if search == "" {
		input += m.th.Muted.Render("Search values")
	}
	lines := []string{theme.Cells(m.th.MenuBar, cond, inner), theme.SepRow, theme.Cells(m.th.MenuBar, input, inner), theme.SepRow}

	for r := range rows {
		i := p.top + r
		if i > len(p.shown) {
			lines = append(lines, theme.Cells(m.th.MenuBar, "", inner))
			continue
		}
		label, count, checked := "Select all", "", p.allChecked()
		dim := false
		if i > 0 {
			v := p.values[p.shown[i-1]]
			label, count, checked, dim = valueLabel(v.Text), strconv.Itoa(v.Count), p.checked[v.Text], v.Text == ""
		}
		box := "[ ] "
		if checked {
			box = "[x] "
		}
		base, muted := m.th.MenuBar, m.th.Muted
		if i == p.sel {
			base, muted = m.th.MenuSelected, m.th.MenuSelected
		}
		labelStyle := base
		if dim {
			labelStyle = muted
		}
		text := base.Render(" "+box) + theme.Cells(labelStyle, label, inner-6-len(count)-1)
		text += muted.Render(theme.PadLeft(count, len(count)+1)) + base.Render(" ")
		lines = append(lines, ansi.Truncate(text, inner, ""))
	}
	title := p.title
	footer := strconv.Itoa(len(p.shown)) + " of " + strconv.Itoa(len(p.values))
	return []box{{id: filterID, x: x, y: y, lines: m.th.Frame(inner, title, footer, lines)}}
}
