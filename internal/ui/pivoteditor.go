package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// pivotEditor is the pivot editor: a panel docked at the right of the
// grid, in the manner of lazygit's side panels rather than Sheets' GUI
// sidebar, listing the data range, the Rows, Columns, Values and Filters
// with their fields, and the grand totals. The results update behind it
// as it changes; each change is an undo step. Up/Down pick a line, Space
// acts on it (adds a field to a section, opens a filter's values, flips
// a toggle), Left/Right change a field's order or summary, Del removes
// it. Enter keeps the changes; Esc undoes them, removing a pivot just
// created.
type pivotEditor struct {
	start int          // the workbook's state before editing, to undo back to
	back  *sheet.Sheet // for a new pivot, the sheet it summarizes, shown again on Esc
	msg   string       // why the last change was refused, for the status line
	list
}

// Sections of the editor, in order.
const (
	sectRows = iota
	sectColumns
	sectValues
	sectFilters
	numSects
)

var sectNames = [numSects]string{"Rows", "Columns", "Values", "Filters"}

var sectDescs = [numSects]string{
	"Group the rows by the values of a field",
	"Spread the groups across columns by the values of a field",
	"Summarize a field for each group: sum, count, average...",
	"Leave out rows by the values of a field",
}

type pivotItemKind uint8

const (
	itemSource pivotItemKind = iota
	itemError
	itemSection
	itemField
	itemToggle
	itemSep
)

// pivotItem is a line of the editor: a field is item i of section sect,
// a toggle is i (0 the total row, 1 the total column).
type pivotItem struct {
	kind    pivotItemKind
	sect, i int
}

const pivotEditorID = "pivot"

// openPivotEditor edits the shown sheet's pivot. start is the state Esc
// returns to; back, for a pivot just created, the sheet to show then.
func (m *Model) openPivotEditor(start int, back *sheet.Sheet) {
	e := &pivotEditor{start: start, back: back}
	e.sel = 2 // the Rows section
	m.clearSelection()
	m.openOverlay(e)
}

func (e *pivotEditor) indicator() string { return "PIVOT" }

func (e *pivotEditor) pivot(m *Model) sheet.Pivot {
	p, _ := m.sheet.Pivot()
	return p
}

// items lists the editor's lines for the pivot as it is now.
func (e *pivotEditor) items(m *Model) []pivotItem {
	p := e.pivot(m)
	items := []pivotItem{{kind: itemSource}}
	if m.sheet.PivotError() != "" {
		items = append(items, pivotItem{kind: itemError})
	}
	items = append(items, pivotItem{kind: itemSep})
	for sect := range numSects {
		items = append(items, pivotItem{kind: itemSection, sect: sect})
		for i := range sectLen(p, sect) {
			items = append(items, pivotItem{kind: itemField, sect: sect, i: i})
		}
	}
	return append(items, pivotItem{kind: itemSep}, pivotItem{kind: itemToggle}, pivotItem{kind: itemToggle, i: 1})
}

func sectLen(p sheet.Pivot, sect int) int {
	return [numSects]int{len(p.Rows), len(p.Columns), len(p.Values), len(p.Filters)}[sect]
}

func selectable(it pivotItem) bool { return it.kind != itemSep && it.kind != itemError }

// step moves the highlight d selectable lines, stopping at the ends.
func (e *pivotEditor) step(items []pivotItem, d int) {
	for i := e.sel + d; i >= 0 && i < len(items); i += d {
		if selectable(items[i]) {
			e.sel = i
			return
		}
	}
}

// current is the highlighted line, kept on a selectable one.
func (e *pivotEditor) current(m *Model) pivotItem {
	items := e.items(m)
	e.sel = clamp(e.sel, 0, len(items)-1)
	if !selectable(items[e.sel]) {
		e.step(items, -1)
	}
	return items[e.sel]
}

// selectItem highlights the line showing it, if any.
func (e *pivotEditor) selectItem(m *Model, it pivotItem) {
	for i, x := range e.items(m) {
		if x == it {
			e.sel = i
		}
	}
}

// set applies a change to the pivot as its own undo step.
func (e *pivotEditor) set(m *Model, label string, fn func(p *sheet.Pivot)) {
	p := e.pivot(m)
	fn(&p)
	e.msg = ""
	if err := m.sheet.SetPivot(p, label); err != nil {
		e.msg = err.Error()
	}
	m.changed = m.sheet.StateID() != m.saved
}

func (e *pivotEditor) key(m *Model, k tea.KeyPressMsg) tea.Cmd {
	if _, ok := m.sheet.Pivot(); !ok {
		m.closeOverlay()
		return nil
	}
	it := e.current(m)
	switch key := k.String(); key {
	case "enter":
		m.closeOverlay()
	case "esc":
		e.cancel(m)
	case "up", "ctrl+p", "shift+tab":
		e.step(e.items(m), -1)
	case "down", "ctrl+n", "tab":
		e.step(e.items(m), 1)
	case "home", "pgup":
		e.sel = 0
	case "end", "pgdown":
		e.sel = len(e.items(m)) - 1
	case "shift+up", "shift+down":
		e.reorder(m, it, key == "shift+down")
	case "left", "right":
		e.adjust(m, it, map[bool]int{false: -1, true: 1}[key == "right"])
	case "space":
		return e.act(m, it)
	case "a", "+":
		if it.kind == itemSection || it.kind == itemField {
			e.addField(m, it.sect)
		}
	case "s":
		e.cycleShowAs(m, it)
	case "delete", "backspace", "x", "-":
		e.remove(m, it)
	}
	return nil
}

// cancel undoes the editor's changes and closes it; a pivot just created
// goes with its sheet.
func (e *pivotEditor) cancel(m *Model) {
	for m.sheet.StateID() != e.start && m.sheet.CanUndo() {
		m.sheet.Undo()
	}
	m.closeOverlay()
	m.afterSheetsChange(e.back, m.book().Active())
	m.changed = m.sheet.StateID() != m.saved
}

// reopen returns to the editor after a picker or prompt.
func (e *pivotEditor) reopen(m *Model) { m.openOverlay(e) }

func (e *pivotEditor) mouse(m *Model, ev mouseEvent) tea.Cmd {
	if ev.box != pivotEditorID {
		if ev.kind == mousePress {
			m.closeOverlay() // a click elsewhere keeps the changes
		}
		return nil
	}
	items := e.items(m)
	i := e.top + ev.row - 1
	switch {
	case ev.kind == mouseWheel && ev.button == tea.MouseWheelUp:
		e.step(items, -1)
	case ev.kind == mouseWheel && ev.button == tea.MouseWheelDown:
		e.step(items, 1)
	case ev.kind != mousePress || ev.button != tea.MouseLeft || i < 0 || i >= len(items) || !selectable(items[i]):
	case i == e.sel || items[i].kind == itemToggle:
		e.sel = i
		return e.act(m, items[i])
	default:
		e.sel = i
	}
	return nil
}

// rows is how many lines of the list show.
func (e *pivotEditor) rows(m *Model, n int) int {
	return max(min(n, m.height-1-gridTop-2), 1)
}

// box places the editor at the right of the grid.
func (e *pivotEditor) box(m *Model) (x, y, inner int) {
	inner = min(44, m.width-2)
	return max(m.width-inner-2, 0), gridTop, inner
}

func (e *pivotEditor) layout(m *Model) []box {
	x, y, inner := e.box(m)
	items := e.items(m)
	e.current(m)
	rows := e.rows(m, len(items))
	e.show(rows)
	p := e.pivot(m)
	var lines []string
	for r := range rows {
		i := e.top + r
		if i >= len(items) {
			break
		}
		lines = append(lines, e.line(m, p, items[i], i == e.sel, inner))
	}
	footer := ""
	if out, ok := m.sheet.PivotRange(); ok && m.sheet.PivotError() == "" {
		footer = "results " + out.String()
	}
	return []box{{id: pivotEditorID, x: x, y: y, lines: m.th.Frame(inner, "Pivot table", footer, lines)}}
}

// line draws one line of the editor, inner columns wide.
func (e *pivotEditor) line(m *Model, p sheet.Pivot, it pivotItem, sel bool, inner int) string {
	base, dim := m.th.MenuBar, m.th.Muted
	if sel {
		base, dim = m.th.MenuSelected, m.th.MenuSelected
	}
	var left, right string
	switch it.kind {
	case itemSep:
		return theme.SepRow
	case itemError:
		return theme.Cells(m.th.Warning, " "+m.sheet.PivotError(), inner)
	case itemSource:
		left = base.Render(" Data  ") + e.sourceText(m, p, sel)
	case itemSection:
		title := m.th.Title
		if sel {
			title = base
		}
		left = title.Render(" " + sectNames[it.sect])
		if sel || sectLen(p, it.sect) == 0 {
			right = dim.Render("Space add ")
		}
	case itemField:
		left = base.Render("   " + e.fieldName(m, p, it))
		right = e.fieldDetail(m, p, it, sel) + base.Render(" ")
	case itemToggle:
		on := p.RowTotals
		label := "Grand total row"
		if it.i == 1 {
			on, label = p.ColumnTotals, "Grand total column"
		}
		check := "[ ] "
		if on {
			check = "[x] "
		}
		left = base.Render(" " + check + label)
	}
	return spreadIn(base, left, right, inner)
}

// spreadIn lays left and right out in w columns on base, cutting left to
// make room for right.
func spreadIn(base lipgloss.Style, left, right string, w int) string {
	rw := ansi.StringWidth(right)
	left = ansi.Truncate(left, max(w-rw-1, 1), "…")
	gap := max(w-ansi.StringWidth(left)-rw, 0)
	return ansi.Truncate(left+base.Render(strings.Repeat(" ", gap))+right, w, "")
}

func (e *pivotEditor) sourceText(m *Model, p sheet.Pivot, sel bool) string {
	text := sheet.QuoteSheet(p.Source) + "!" + p.Range.String()
	if p.Lost {
		text = sheet.QuoteSheet(p.Source) + "!#REF!"
	}
	if sel {
		return m.th.MenuSelected.Render(text)
	}
	return m.th.Key.Render(text)
}

func (e *pivotEditor) status(m *Model) (string, string) {
	it := e.current(m)
	desc, pairs := e.help(m, it)
	if e.msg != "" {
		desc = m.th.Warning.Render(e.msg)
	}
	pairs = append(pairs, "Enter", "done", "Esc", "cancel")
	for {
		keys := m.th.KeyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= m.width:
			return desc, keys
		case desc != "":
			desc = ""
		case len(pairs) > 4:
			pairs = pairs[2:]
		default:
			return "", keys
		}
	}
}

// help is what the highlighted line is and the keys that act on it.
func (e *pivotEditor) help(m *Model, it pivotItem) (string, []string) {
	p := e.pivot(m)
	switch it.kind {
	case itemSource:
		return "The data summarized; its first row names the fields", []string{"Space", "change"}
	case itemSection:
		return sectDescs[it.sect], []string{"Space", "add a field"}
	case itemToggle:
		return "Show totals of every group", []string{"Space", "on/off"}
	}
	remove := []string{"Del", "remove"}
	switch it.sect {
	case sectValues:
		v := p.Values[it.i]
		return v.Summarize.Desc(), append([]string{"←/→", "summarize by", "S", "show as"}, remove...)
	case sectFilters:
		return "Choose which values count, or a condition", append([]string{"Space", "values"}, remove...)
	}
	return "Order the groups by label or by a value", append([]string{"←/→", "order", "Shift+↑/↓", "move"}, remove...)
}
