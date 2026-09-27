package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/picker"
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
	m      pivotHost    // the model, through what the editor needs of it
	start  int          // the workbook's state before editing, to undo back to
	back   *sheet.Sheet // for a new pivot, the sheet it summarizes, shown again on Esc
	before string       // the pivot when the editor opened, as a macro answers it
	msg    string       // why the last change was refused, for the status line
	overlay.List
}

// pivotHost is what the pivot editor acts on. The model implements it.
// The editor stays in package ui: besides the pivot on the sheet shown,
// it opens the model's pickers, filter values list and range prompt and
// comes back from them, which takes more than a small interface.
type pivotHost interface {
	styles() *theme.Theme
	size() (width, height int)
	book() *sheet.Workbook
	// sheetShown is the sheet the pivot is on while it's edited.
	sheetShown() *sheet.Sheet
	// syncChanged makes the modified flag follow the undo history.
	syncChanged()
	openOverlay(o overlay.Overlay)
	closeOverlay()
	afterSheetsChange(prefer *sheet.Sheet, index int)
	showSheet(s *sheet.Sheet)
	selectRect(r sheet.Rect)
	clearSelection()
	newPicker(title, placeholder string, maxW int, items []picker.Item) *picker.Picker
	// pickValues opens a filter's values list at screen column x.
	pickValues(title string, x int, values []sheet.FilterValue, cond sheet.Condition, apply func(sheet.Criteria), cancel func())
	// pointRange asks for a range, pointed at or typed.
	pointRange(label string, done func(sheet.Rect), cancel func())
	// askText asks for text, such as a value's name.
	askText(label, initial string, done func(string), cancel func())
	// recordDialog records the pivot made, as the command id answered
	// with its definition.
	recordDialog(id, answer string)
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
func (m *Model) openPivotEditor(start int, back *sheet.Sheet) *pivotEditor {
	e := &pivotEditor{m: m, start: start, back: back}
	e.before = e.pivot(m).JSON()
	e.Sel = 2 // the Rows section
	m.clearSelection()
	m.openOverlay(e)
	return e
}

func (e *pivotEditor) Indicator() string { return "PIVOT" }

func (e *pivotEditor) pivot(m pivotHost) sheet.Pivot {
	p, _ := m.sheetShown().Pivot()
	return p
}

// items lists the editor's lines for the pivot's current definition.
func (e *pivotEditor) items(m pivotHost) []pivotItem {
	p := e.pivot(m)
	items := []pivotItem{{kind: itemSource}}
	if m.sheetShown().PivotError() != "" {
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
	for i := e.Sel + d; i >= 0 && i < len(items); i += d {
		if selectable(items[i]) {
			e.Sel = i
			return
		}
	}
}

// current is the highlighted line, kept on a selectable one.
func (e *pivotEditor) current(m pivotHost) pivotItem {
	items := e.items(m)
	e.Sel = clamp(e.Sel, 0, len(items)-1)
	if !selectable(items[e.Sel]) {
		e.step(items, -1)
	}
	return items[e.Sel]
}

// selectItem highlights the line showing it, if any.
func (e *pivotEditor) selectItem(m pivotHost, it pivotItem) {
	for i, x := range e.items(m) {
		if x == it {
			e.Sel = i
		}
	}
}

// set applies a change to the pivot as its own undo step.
func (e *pivotEditor) set(m pivotHost, label string, fn func(p *sheet.Pivot)) {
	p := e.pivot(m)
	fn(&p)
	e.msg = ""
	if err := m.sheetShown().SetPivot(p, label); err != nil {
		e.msg = err.Error()
	}
	m.syncChanged()
}

func (e *pivotEditor) Key(k tea.KeyPressMsg) tea.Cmd {
	m := e.m
	if _, ok := m.sheetShown().Pivot(); !ok {
		m.closeOverlay()
		return nil
	}
	it := e.current(m)
	switch key := k.String(); key {
	case "enter":
		e.keep(m)
	case "esc":
		e.cancel(m)
	case "up", "ctrl+p", "shift+tab":
		e.step(e.items(m), -1)
	case "down", "ctrl+n", "tab":
		e.step(e.items(m), 1)
	case "home", "pgup":
		e.Sel = 0
	case "end", "pgdown":
		e.Sel = len(e.items(m)) - 1
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
	case "r", "f2":
		e.rename(m, it)
	case "delete", "backspace", "x", "-":
		e.remove(m, it)
	}
	return nil
}

// cancel undoes the editor's changes and closes it; a pivot just created
// goes with its sheet.
func (e *pivotEditor) cancel(m pivotHost) {
	for m.sheetShown().StateID() != e.start && m.sheetShown().CanUndo() {
		m.sheetShown().Undo()
	}
	m.closeOverlay()
	m.afterSheetsChange(e.back, m.book().Active())
	m.syncChanged()
}

// keep closes the editor keeping the changes, and records the pivot
// made: a new one as Data > Pivot table, a changed one as Edit pivot
// table, answered with its definition.
func (e *pivotEditor) keep(m pivotHost) {
	m.closeOverlay()
	id, now := "data.pivot_edit", e.pivot(m).JSON()
	if e.back != nil {
		id = "data.pivot"
	} else if now == e.before {
		return
	}
	m.recordDialog(id, now)
}

// answer makes the pivot a macro's answer defines, as a line of the file
// writes it, and closes the editor.
func (e *pivotEditor) answer(m pivotHost, text string) error {
	p, err := sheet.ParsePivot(text)
	if err == nil {
		err = m.sheetShown().SetPivot(p, "")
	}
	m.syncChanged()
	m.closeOverlay()
	return err
}

// reopen returns to the editor after a picker or prompt.
func (e *pivotEditor) reopen() { e.m.openOverlay(e) }

func (e *pivotEditor) Mouse(ev overlay.MouseEvent) tea.Cmd {
	m := e.m
	if ev.Box != pivotEditorID {
		if ev.Kind == overlay.MousePress {
			e.keep(m) // a click elsewhere keeps the changes
		}
		return nil
	}
	items := e.items(m)
	i := e.Top + ev.Row - 1
	switch {
	case ev.Kind == overlay.MouseWheel && ev.Button == tea.MouseWheelUp:
		e.step(items, -1)
	case ev.Kind == overlay.MouseWheel && ev.Button == tea.MouseWheelDown:
		e.step(items, 1)
	case ev.Kind != overlay.MousePress || ev.Button != tea.MouseLeft || i < 0 || i >= len(items) || !selectable(items[i]):
	case i == e.Sel || items[i].kind == itemToggle:
		e.Sel = i
		return e.act(m, items[i])
	default:
		e.Sel = i
	}
	return nil
}

// rows is how many lines of the list show.
func (e *pivotEditor) rows(m pivotHost, n int) int {
	_, height := m.size()
	return max(min(n, height-1-gridTop-2), 1)
}

// box places the editor at the right of the grid.
func (e *pivotEditor) box(m pivotHost) (x, y, inner int) {
	width, _ := m.size()
	inner = min(44, width-2)
	return max(width-inner-2, 0), gridTop, inner
}

func (e *pivotEditor) Layout() []overlay.Box {
	m := e.m
	x, y, inner := e.box(m)
	items := e.items(m)
	e.current(m)
	rows := e.rows(m, len(items))
	e.Show(rows)
	p := e.pivot(m)
	var lines []string
	for r := range rows {
		i := e.Top + r
		if i >= len(items) {
			break
		}
		lines = append(lines, e.line(m, p, items[i], i == e.Sel, inner))
	}
	footer := ""
	if out, ok := m.sheetShown().PivotRange(); ok && m.sheetShown().PivotError() == "" {
		footer = "results " + out.String()
	}
	return []overlay.Box{{ID: pivotEditorID, X: x, Y: y, Lines: m.styles().Frame(inner, "Pivot table", footer, lines)}}
}

// line draws one line of the editor, inner columns wide.
func (e *pivotEditor) line(m pivotHost, p sheet.Pivot, it pivotItem, sel bool, inner int) string {
	base, dim := m.styles().MenuBar, m.styles().Muted
	if sel {
		base, dim = m.styles().MenuSelected, m.styles().MenuSelected
	}
	var left, right string
	switch it.kind {
	case itemSep:
		return theme.SepRow
	case itemError:
		return theme.Cells(m.styles().Warning, " "+m.sheetShown().PivotError(), inner)
	case itemSource:
		left = base.Render(" Data  ") + e.sourceText(m, p, sel)
	case itemSection:
		title := m.styles().Title
		if sel {
			title = base
		}
		left = title.Render(" " + sectNames[it.sect])
		if sel || sectLen(p, it.sect) == 0 {
			right = dim.Render("Space add ")
		}
	case itemField:
		left = base.Render("   " + e.lineName(m, p, it))
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

func (e *pivotEditor) sourceText(m pivotHost, p sheet.Pivot, sel bool) string {
	text := sheet.QuoteSheet(p.Source) + "!" + p.Range.String()
	if p.Lost {
		text = sheet.QuoteSheet(p.Source) + "!#REF!"
	}
	if sel {
		return m.styles().MenuSelected.Render(text)
	}
	return m.styles().Key.Render(text)
}

func (e *pivotEditor) Status() (string, string) {
	m := e.m
	it := e.current(m)
	width, _ := m.size()
	desc, pairs := e.help(m, it)
	if e.msg != "" {
		desc = m.styles().Warning.Render(e.msg)
	}
	pairs = append(pairs, "Enter", "done", "Esc", "cancel")
	for {
		keys := m.styles().KeyHints(pairs...)
		switch {
		case ansi.StringWidth(desc)+3+ansi.StringWidth(keys) <= width:
			return desc, keys
		case desc != "":
			desc = ""
		case len(pairs) > 6:
			// Drop the line's last key but one, keeping its first, the
			// one that matters most, and Enter and Esc.
			pairs = append(pairs[:len(pairs)-6], pairs[len(pairs)-4:]...)
		case len(pairs) > 4:
			pairs = pairs[2:]
		default:
			return "", keys
		}
	}
}

// help is what the highlighted line is and the keys that act on it.
func (e *pivotEditor) help(m pivotHost, it pivotItem) (string, []string) {
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
		desc := v.Summarize.Desc()
		if v.Name != "" {
			desc = e.fieldName(m, p, it) + ": " + desc
		}
		return desc, append([]string{"←/→", "summary", "S", "show as", "R", "name"}, remove...)
	case sectFilters:
		return "Choose which values count, or a condition", append([]string{"Space", "values"}, remove...)
	}
	return "Order the groups by label or by a value", append([]string{"←/→", "order", "Shift+↑/↓", "move"}, remove...)
}
