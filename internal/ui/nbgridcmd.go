package ui

import (
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/nbview"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
)

// Commands on an output's grid, and what the control panel says of it.
// The grid runs the commands that look at its values or change how they
// show (selecting, copying, finding, sorting, filtering, column widths,
// undoing those) and refuses the rest, which would change a value. A
// chart, a pivot table or a frequency table from the output is made on
// a sheet, where they live: the output goes to a sheet as G sends it,
// so what's made from it follows the cell's next run, and a notebook's
// cells stay code and what it printed.

// gridCommands are the commands an output's grid runs itself.
var gridCommands = map[string]bool{
	"edit.copy": true, "edit.find": true, "edit.find_next": true, "edit.find_prev": true,
	"select.none": true, "select.all": true, "select.columns": true, "select.rows": true,
	"column.width": true, "column.reset": true, "menu.context": true, "data.dropdown": true,
	"data.sort_sheet_az": true, "data.sort_sheet_za": true, "data.sort_range_az": true, "data.sort_range_za": true, "data.sort_range": true,
	"data.filter": true, "data.filter_column": true, "data.filter_remove": true,
}

// onSheet are the commands whose result goes on a sheet of its own or
// beside the data: run on the output sent to a sheet.
var onSheet = map[string]bool{"insert.chart": true, "data.pivot": true, "data.frequency": true}

// allows reports whether a command runs on the grid.
func (g *outGrid) allows(id string) bool { return gridCommands[id] || gridTakes(id) || onSheet[id] }

// gridMenu is the cell menu on an output's grid.
var gridMenu = []menuItem{
	{cmd: "edit.copy"}, sep, {cmd: "data.sort_sheet_az", title: "Sort A to Z"}, {cmd: "data.sort_sheet_za", title: "Sort Z to A"}, sep,
	{cmd: "data.filter"}, {cmd: "data.filter_remove"}, sep, {cmd: "insert.chart"}, {cmd: "data.pivot"},
}

// cellMenu is the right-click menu of the cells.
func (m *Model) cellMenu() []menuItem {
	if m.out != nil {
		return gridMenu
	}
	return cellMenu
}

// offers reports whether a menu on the grid lists a command: those it
// runs and the program's own. A nil grid, the program's model, lists
// every one.
func (g *outGrid) offers(id string) bool { return g == nil || g.allows(id) || notebookSafe(id) }

// command takes a command run on the grid's model, reporting whether it
// did: the program's own commands go back to it, Edit shows the grid
// full-screen, a chart or pivot table goes on a sheet, and anything
// that would change a value is refused.
func (g *outGrid) command(id string) (tea.Cmd, bool) {
	p := g.parent
	switch {
	case g.allows(id) && !onSheet[id]:
		return nil, false
	case id == "edit":
		p.gridFull()
	case onSheet[id]:
		return p.outputOnSheet(g, id), true
	case notebookSafe(id):
		return p.runCommand(id), true
	default:
		g.child.note = commands[id].title + " would change the output: G sends it to a sheet, where its copy can change"
	}
	return nil, true
}

// outputOnSheet runs command id on the output sent to a sheet: sent to a
// new sheet first unless it's on one, with the columns selected in the
// grid selected there, every row under the header.
func (m *Model) outputOnSheet(g *outGrid, id string) tea.Cmd {
	nb, v := m.sheet, m.nbView()
	if v == nil {
		return nil
	}
	name, ok := m.outputName(nb, g.id)
	if !ok {
		return nil
	}
	if _, _, sent := m.book().Region(name); !sent {
		m.sendToNewSheet(nb, name)
	}
	t, _, ok := m.book().Region(name)
	if !ok {
		return nil
	}
	table, ok := t.RegionTable(name)
	if !ok {
		m.note = "The output sent to " + t.Name() + " has no rows yet"
		return nil
	}
	c := g.child
	r := table
	if c.hasRange() {
		sel := c.selection()
		r.From.Col = table.From.Col + min(sel.From.Col, g.cols-1)
		r.To.Col = table.From.Col + min(sel.To.Col, g.cols-1)
	}
	v.CloseFull()
	m.nb.out.in = nil
	m.showSheet(t)
	m.selectRect(r)
	m.cur = sheet.Addr{Col: table.From.Col + min(c.cur.Col, g.cols-1), Row: r.From.Row}
	return m.runCommand(id)
}

// outputName is the name of cell id of notebook nb, naming it first as
// sending it does when it has none.
func (m *Model) outputName(nb *sheet.Sheet, id int) (string, bool) {
	for i, c := range nb.NotebookCells() {
		if c.ID != id {
			continue
		}
		name := c.Name()
		if name == "" {
			name = m.book().FreeCellName("cell" + strconv.Itoa(i+1))
			m.renameCell(nb, c, name)
		}
		return name, true
	}
	return "", false
}

// gridIndicator is the mode while a grid is entered: OUTPUT, or its
// menu's or prompt's.
func (m *Model) gridIndicator(g *outGrid) string {
	c := g.child
	switch {
	case c.overlay != nil:
		return c.overlay.Indicator()
	case c.mode == modePrompt:
		return c.prompt.indicator
	}
	return "OUTPUT"
}

// gridBusy reports whether the grid's own context line is to be shown:
// a bar, a prompt, a menu, a resize or a message of its own.
func (g *outGrid) gridBusy() bool {
	c := g.child
	return c.mode != modeReady || c.overlay != nil || c.mouse.drag == dragResize
}

// gridContext is the context line while grid g is entered: the grid's
// own when it has something to say, otherwise what's shown and the keys.
func (m *Model) gridContext(g *outGrid, v *nbview.View) string {
	c := g.child
	switch {
	case g.gridBusy():
		return c.contextLineText()
	case c.warn != "":
		return v.Rule(m.th.Warning.Render(c.warn), "", m.width)
	case c.note != "":
		return v.Rule(m.th.Hint.Render(c.note), "", m.width)
	}
	name, _ := v.Head()
	keys := []string{"Shift+arrows", "select", "Ctrl+C", "copy", "Ctrl+F", "find", "Enter", "full-screen", "Esc", "back"}
	if v.FullOpen() {
		keys = slices.Delete(keys, 6, 8)
	}
	for len(keys) > 4 && ansi.StringWidth("Output of "+name+m.th.KeyHints(keys...))+8 > m.width {
		keys = keys[2:] // the keys that matter most are last
	}
	return v.Rule(m.th.Muted.Render("Output of "+name), m.th.KeyHints(keys...), m.width)
}

// gridCursor is where the terminal's caret goes while grid g is
// entered: in its bar or prompt, or its box's search field.
func (m *Model) gridCursor(g *outGrid, v *nbview.View) (int, int, bool) {
	x, y, ok := g.child.cursorPos()
	if !ok || y == formulaLine || y == contextLine {
		return x, y, ok
	}
	ox, oy := g.origin(v)
	return x + ox, windowY(y) + oy, true
}

// rangeLabel names a selected range for the status line: its address,
// or on an output's grid the columns' names and the rows as numbered.
func (m *Model) rangeLabel(r sheet.Rect) string {
	if m.out == nil {
		return r.String()
	}
	name := func(c int) string { return ansi.Truncate(m.sheet.ShownText(sheet.Addr{Col: c}), 16, "…") }
	cols := name(r.From.Col)
	if r.To.Col > r.From.Col {
		cols += " to " + name(min(r.To.Col, m.out.cols-1))
	}
	from, to := max(r.From.Row, 1), min(r.To.Row, m.out.rows)
	if from == to {
		return cols + ", row " + strconv.Itoa(from)
	}
	return cols + ", rows " + strconv.Itoa(from) + " to " + strconv.Itoa(to)
}

// boxWidth is how wide a box is drawn.
func boxWidth(b overlay.Box) int {
	w := 0
	for _, l := range b.Lines {
		w = max(w, ansi.StringWidth(l))
	}
	return w
}
