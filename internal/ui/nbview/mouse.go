package nbview

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// The mouse, as in JupyterLab: a click selects a cell, a click in its
// box edits it with the caret where it landed, ▶ runs it (■ stops it),
// Shift+click selects the cells between, a click on an output selects
// it and a double click opens it full-screen, a click left of an output
// folds it, and the wheel over an output's window scrolls it.

// hit is what's at a line of the body.
type hit struct {
	cell int
	kind rowKind
	r    int // which line of its kind
}

// hitAt is what's at line y of the body, if a cell is there.
func (v *View) hitAt(y int) (hit, bool) {
	line := v.top + y
	at := 0
	for i, c := range v.h.Cells() {
		b := v.blockOf(i, c)
		if line >= at && line < at+b.lines() {
			kind, r := b.row(line - at)
			return hit{cell: i, kind: kind, r: r}, true
		}
		at += b.lines()
	}
	return hit{}, false
}

// inBox reports whether column x is inside a box's sides.
func (v *View) inBox(x int) bool { return x > gutter && x < gutter+v.content()+boxW-1 }

// onRun reports whether column x is a box's ▶.
func (v *View) onRun(x int) bool {
	at := gutter + v.content() + boxW + 1
	return x >= at-1 && x <= at
}

// tap is a click, for telling a double click.
type tap struct {
	at   time.Time
	cell int
	kind rowKind
}

// Click takes a left click at column x of line y of the body; shift
// extends the selection to the cell clicked.
func (v *View) Click(x, y int, shift bool) tea.Cmd {
	if v.full != nil {
		return nil
	}
	h, ok := v.hitAt(y)
	if !ok {
		return nil
	}
	cells := v.h.Cells()
	c := cells[h.cell]
	boxed := v.boxed(h.cell, c)
	if v.edit.on && h.cell == v.sel && h.kind == rowSrc && boxed && v.inBox(x) {
		v.edit.area.Place(v.content(), h.r, x-textX)
		v.follow()
		return nil
	}
	double := !shift && v.lastTap.cell == h.cell && v.lastTap.kind == h.kind && time.Since(v.lastTap.at) < 400*time.Millisecond
	v.lastTap = tap{at: time.Now(), cell: h.cell, kind: h.kind}
	v.StopEdit()
	if shift {
		from := v.sel
		if v.anchor >= 0 {
			from = v.anchor
		}
		v.SelectRange(from, h.cell)
		return nil
	}
	return v.clickCell(h, c, x, boxed, double)
}

// clickCell is a plain click on cell c.
func (v *View) clickCell(h hit, c notebook.Cell, x int, boxed, double bool) tea.Cmd {
	switch {
	case h.kind == rowSrc && h.r == 0 && boxed && v.onRun(x):
		v.Select(h.cell, false)
		if v.h.State(c.ID).Running {
			return v.h.Run("nb.stop")
		}
		return v.h.Run("nb.run")
	case h.kind == rowOut && x < gutter:
		v.Select(h.cell, true)
		v.ToggleHidden()
	case h.kind == rowOut:
		v.Select(h.cell, true)
		if double {
			v.OpenFull()
		}
	case h.kind == rowSrc && boxed && v.inBox(x):
		v.Select(h.cell, false)
		cmd := v.StartEdit()
		v.edit.area.Place(v.content(), h.r, x-textX)
		v.follow()
		return cmd
	default:
		v.Select(h.cell, false)
		if double && c.Kind == notebook.Note {
			return v.StartEdit()
		}
	}
	return nil
}

// RightClick selects the cell at line y for its context menu, unless
// it's one of the cells selected.
func (v *View) RightClick(y int) {
	h, ok := v.hitAt(y)
	if !ok {
		return
	}
	if from, to := v.Range(); h.cell < from || h.cell > to {
		v.StopEdit()
		v.Select(h.cell, false)
	}
}

// Wheel scrolls d lines with the mouse at line y: the output's window
// under it while it has more that way, or the body.
func (v *View) Wheel(y, d int) {
	if v.full != nil {
		if g := v.full.Grid(); g != nil {
			g.Scroll(d)
			return
		}
		v.full.row += d
		v.full.settle()
		return
	}
	if h, ok := v.hitAt(y); ok && y >= 0 && h.kind == rowOut {
		if v.scrollBy(v.h.Cells()[h.cell], d) {
			return
		}
	}
	v.top = max(min(v.top+d, v.total()-v.height), 0)
}

// Cursor is where the terminal's caret goes in the body, while a cell
// is edited.
func (v *View) Cursor() (x, y int, ok bool) {
	if !v.edit.on {
		return 0, 0, false
	}
	row, col := v.edit.caret(v.content())
	y = v.startOf(v.sel) + 1 + row - v.top
	if y < 0 || y >= v.height {
		return 0, 0, false
	}
	return textX + col, y, true
}
