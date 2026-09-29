package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Selection follows Google Sheets: the active cell (cur) stays put while
// an extension corner (ext) moves with Shift+arrows, Shift+click or a
// mouse drag. Clicking a column or row header selects whole columns or
// rows, and the corner selects everything.

type wholeKind int

const (
	wholeNone wholeKind = iota
	wholeCols
	wholeRows
	wholeAll
)

type dragKind int

const (
	dragNone dragKind = iota
	dragCells
	dragCols
	dragRows
	dragPoint     // dragging out a range in POINT mode or a range prompt
	dragResize    // dragging a column header border
	dragRowResize // dragging a row number's bottom border
	dragFill      // dragging the fill handle
	dragTab       // dragging a sheet's tab to move it
)

// selection returns the selected range; just the active cell when nothing
// is selected. A range takes in the merged cells it touches, as in Sheets.
func (g *grid) selection() sheet.Rect {
	return g.sheet.Grow(g.corners())
}

// corners is the range between the active cell and the selection's
// moving corner, before merged cells grow it: what a macro records.
func (g *grid) corners() sheet.Rect {
	if !g.selecting {
		return sheet.Rect{From: g.cur, To: g.cur}
	}
	r := sheet.NewRect(g.cur, g.ext)
	switch g.whole {
	case wholeCols:
		r.From.Row, r.To.Row = 0, sheet.MaxRows-1
	case wholeRows:
		r.From.Col, r.To.Col = 0, sheet.MaxCols-1
	case wholeAll:
		r = sheet.Rect{To: sheet.Addr{Col: sheet.MaxCols - 1, Row: sheet.MaxRows - 1}}
	}
	return r
}

// snap returns the cell a stands for: the top-left cell of the merge
// holding it, where a merged cell's contents are.
func (g *grid) snap(a sheet.Addr) sheet.Addr {
	if m, ok := g.sheet.MergeAt(a); ok {
		return m.From
	}
	return a
}

// hasRange reports whether more than one cell is selected: a merged
// cell alone counts as one.
func (g *grid) hasRange() bool {
	r := g.selection()
	if m, ok := g.sheet.MergeAt(g.cur); ok && r == m {
		return false
	}
	return r.From != r.To
}

func (g *grid) clearSelection() {
	g.selecting, g.whole = false, wholeNone
	g.ext = g.cur
}

// extendKey reports whether key is a Shift-modified movement key and
// returns the movement without Shift. Shift+Tab is its own movement.
func extendKey(key string) (string, bool) {
	if key == "shift+tab" || !strings.Contains(key, "shift+") {
		return "", false
	}
	base := strings.Replace(key, "shift+", "", 1)
	return base, isMoveKey(base)
}

// moveKey handles a movement key in READY mode: plain keys move the active
// cell and drop the selection, Shift+keys extend it.
func (g *grid) moveKey(key string) bool {
	if base, ok := extendKey(key); ok {
		if !g.selecting {
			g.selecting, g.ext = true, g.cur
		}
		g.whole = wholeNone
		return g.navigate(base, &g.ext)
	}
	if g.navigate(key, &g.cur) {
		g.entered, g.cur = g.cur, g.snap(g.cur)
		g.clearSelection()
		return true
	}
	return false
}

// leaveFrom is where a step off the merge m leaves from: the cell the
// active cell entered it by, when a is the active cell and that's in m,
// otherwise a.
func (g *grid) leaveFrom(a *sheet.Addr, m sheet.Rect) sheet.Addr {
	if a == &g.cur && m.Contains(g.entered) {
		return g.entered
	}
	return *a
}

// pointMoveKey handles movement in POINT mode and range prompts: arrows
// move the pointer and Shift+arrows stretch it into a range, as when
// selecting a range while typing a formula in Sheets.
func (m *Model) pointMoveKey(key string) bool {
	if key == "tab" || key == "shift+tab" {
		return false
	}
	if base, ok := extendKey(key); ok {
		if !m.point.anchored {
			m.point.anchor, m.point.anchored = m.point.at, true
		}
		return m.navigate(base, &m.point.at)
	}
	if m.navigate(key, &m.point.at) {
		m.point.at = m.snap(m.point.at)
		m.point.anchored = false
		return true
	}
	return false
}

// selectColumns selects the columns spanned by the selection (Ctrl+Space).
func (g *grid) selectColumns() tea.Cmd {
	if !g.selecting {
		g.ext = g.cur
	}
	g.selecting, g.whole = true, wholeCols
	return nil
}

// selectRows selects the rows spanned by the selection (Shift+Space).
func (g *grid) selectRows() tea.Cmd {
	if !g.selecting {
		g.ext = g.cur
	}
	g.selecting, g.whole = true, wholeRows
	return nil
}

func (g *grid) selectAll() tea.Cmd {
	used, ok := g.sheet.UsedRange()
	if ok && !(g.selecting && g.whole == wholeNone && g.selection() == used) {
		g.cur, g.ext = used.From, used.To
		g.selecting, g.whole = true, wholeNone
		return nil
	}
	g.cur = sheet.Addr{Col: g.left, Row: g.top}
	g.selecting, g.whole, g.ext = true, wholeAll, g.cur
	return nil
}

// navigate applies a movement key to a, returning false if it isn't one.
// Keys follow Google Sheets.
func (g *grid) navigate(key string, a *sheet.Addr) bool {
	rows, cols := g.scrollRows(), g.visibleCols(g.left)
	if m, ok := g.sheet.MergeAt(*a); ok { // steps leave a merged cell from its edge
		from := g.leaveFrom(a, m)
		switch key {
		case "up":
			a.Row, a.Col = m.From.Row, from.Col
		case "down":
			a.Row, a.Col = m.To.Row, from.Col
		case "left", "shift+tab":
			a.Col, a.Row = m.From.Col, from.Row
		case "right", "tab":
			a.Col, a.Row = m.To.Col, from.Row
		}
	}
	switch key {
	case "up":
		a.Row = g.stepRow(a.Row, -1)
	case "down":
		a.Row = g.stepRow(a.Row, 1)
	case "left", "shift+tab":
		a.Col--
	case "right", "tab":
		a.Col++
	case "ctrl+up":
		*a = g.sheet.Edge(*a, 0, -1)
	case "ctrl+down":
		*a = g.sheet.Edge(*a, 0, 1)
	case "ctrl+left":
		*a = g.sheet.Edge(*a, -1, 0)
	case "ctrl+right", "end":
		*a = g.sheet.Edge(*a, 1, 0)
	case "pgup":
		a.Row = g.stepRow(a.Row, -rows)
		g.top = g.stepRow(g.top, -rows)
	case "pgdown":
		a.Row = g.stepRow(a.Row, rows)
		g.top = g.stepRow(g.top, rows)
	case "alt+pgup":
		a.Col -= cols
		g.left -= cols
	case "alt+pgdown":
		a.Col += cols
		g.left += cols
	case "home":
		a.Col = 0
	case "ctrl+home":
		*a = sheet.Addr{}
	case "ctrl+end":
		used, _ := g.sheet.UsedRange()
		*a = used.To
	default:
		return false
	}
	*a = clampAddr(*a)
	a.Row = g.visibleRow(a.Row)
	// Moving into the frozen panes by keyboard scrolls the rest back to
	// the start, as in Sheets (Ctrl+Home shows A1 with row 2 under it).
	if fr, fc := g.frozen(); a.Row < fr || a.Col < fc {
		if a.Row < fr {
			g.top = 0
		}
		if a.Col < fc {
			g.left = 0
		}
	}
	g.clampView()
	return true
}

func isMoveKey(key string) bool {
	switch key {
	case "up", "down", "left", "right", "tab", "shift+tab", "pgup", "pgdown",
		"alt+pgup", "alt+pgdown", "home", "end", "ctrl+home", "ctrl+end",
		"ctrl+up", "ctrl+down", "ctrl+left", "ctrl+right":
		return true
	}
	return false
}

func (m *Model) openGoto() {
	m.openText("Go to:", m.cur.String(), func(m *Model, text string) tea.Cmd {
		if !m.gotoText(strings.TrimSpace(text)) {
			m.fail("Not a cell, range, named range or region: " + text)
		}
		return nil
	})
	m.prompt.indicator = "POINT"
}

// gotoText goes to a cell, range, named range or region's table (nu.r1,
// as formulas name it), on any sheet, and reports whether text named
// one. A sheet may lead: Sheet2!A1, 'Q3 plan'!B2:C9, or just Sheet2!.
func (m *Model) gotoText(text string) bool {
	target := m.sheet
	if name, rest := sheet.SplitSheet(text); name != "" {
		if target = m.book().Lookup(name); target == nil {
			m.fail("There's no sheet named " + name)
			return true
		}
		if text = rest; text == "" {
			if !m.refuseHidden(target) {
				m.showSheet(target)
			}
			return true
		}
	}
	r, ok := sheet.ParseRange(text)
	if n, named := m.sheet.LookupName(text); named && !n.Gone() && target == m.sheet {
		r, ok, target = n.Range, true, n.Sheet
	}
	if !ok && target == m.sheet {
		r, target, ok = m.regionTable(text)
	}
	if !ok {
		return false
	}
	if m.refuseHidden(target) {
		return true
	}
	m.showSheet(target)
	if r.From == r.To {
		m.clearSelection()
		m.cur = m.snap(r.From)
		return true
	}
	m.selectRect(r)
	return true
}

// regionTable finds the region formulas name as text (nu.r1), ignoring
// case, and returns its table, header row included, and its sheet. A
// region with no rows yet has no table: its first cell stands for it.
func (m *Model) regionTable(text string) (sheet.Rect, *sheet.Sheet, bool) {
	const prefix = "nu."
	if len(text) <= len(prefix) || !strings.EqualFold(text[:len(prefix)], prefix) {
		return sheet.Rect{}, nil, false
	}
	s, reg, ok := m.book().Region(text[len(prefix):])
	if !ok {
		return sheet.Rect{}, nil, false
	}
	if t, ok := s.RegionTable(reg.Name); ok {
		return t, s, true
	}
	return sheet.Rect{From: reg.At, To: reg.At}, s, true
}
