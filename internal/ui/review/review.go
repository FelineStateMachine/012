// Package review is live mode's suggestions panel (docs/agents/live.md):
// a box at the right of the grid listing the agents' suggestions waiting
// for the person, each by agent and message with the cells it would
// set, to accept or reject whole or cell by cell, with keys or the
// mouse. The grid beside it shows the cell of the row highlighted. It
// knows the UI only through Host.
package review

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the panel needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Close closes the open overlay.
	Close()
	// Suggestions are those waiting, oldest first.
	Suggestions() []Item
	// Accept and Reject settle the cells of suggestion id picked, by
	// their index in it; nil is all it has waiting.
	Accept(id int, cells []int)
	Reject(id int, cells []int)
	// Reveal puts the pointer on a cell a suggestion sets.
	Reveal(sheet string, at sheet.Addr)
}

// Item is a suggestion as the panel lists it.
type Item struct {
	ID      int
	Agent   string
	Color   int
	Label   string
	Message string
	Whole   bool     // it can only be accepted whole
	Other   []string // what it does besides setting cells
	Cells   []Cell   // the cells still waiting
	Total   int      // the cells it sets, waiting or not
	Failed  string   // why accepting it failed
}

// Cell is one cell a suggestion sets.
type Cell struct {
	Index    int // in the suggestion
	Sheet    string
	At       sheet.Addr
	Was, Now string
	Look     bool // its format, style or note change too
}

// ID identifies the panel's box in mouse events.
const ID = "review"

// row is a line of the list: a suggestion's heading (cell -1), one of
// its cells, or what else it does (other).
type row struct {
	item  int
	cell  int
	other string
}

// Panel is the open suggestions panel.
type Panel struct {
	h        Host
	sel, top int
	items    []Item
	rows     []row
	box      overlay.Box
}

// New opens the panel on the first suggestion.
func New(h Host) *Panel {
	p := &Panel{h: h}
	p.refresh()
	p.show()
	return p
}

func (p *Panel) Indicator() string { return "REVIEW" }

// refresh lists the suggestions as the host has them.
func (p *Panel) refresh() {
	p.items = p.h.Suggestions()
	p.rows = p.rows[:0]
	for i, it := range p.items {
		p.rows = append(p.rows, row{item: i, cell: -1})
		for _, o := range it.Other {
			p.rows = append(p.rows, row{item: i, cell: -1, other: o})
		}
		for j := range it.Cells {
			p.rows = append(p.rows, row{item: i, cell: j})
		}
	}
	p.sel = min(max(p.sel, 0), max(len(p.rows)-1, 0))
	for p.sel > 0 && p.rows[p.sel].other != "" {
		p.sel--
	}
}

// show puts the grid's pointer on the highlighted row's cell.
func (p *Panel) show() {
	if len(p.rows) == 0 {
		return
	}
	r := p.rows[p.sel]
	it := p.items[r.item]
	c := r.cell
	if c < 0 {
		c = 0
	}
	if c < len(it.Cells) {
		p.h.Reveal(it.Cells[c].Sheet, it.Cells[c].At)
	}
}

// visible is how many rows fit.
func (p *Panel) visible() int {
	_, height := p.h.Size()
	return max(min(len(p.rows), height-overlay.GridTop-3), 1)
}

func (p *Panel) Layout() []overlay.Box {
	p.refresh()
	th := p.h.Theme()
	width, _ := p.h.Size()
	inner := max(min(58, width-4), 20)
	n := p.visible()
	p.top = min(max(p.top, p.sel-n+1, 0), p.sel)
	lines := make([]string, n)
	for i := range lines {
		j := p.top + i
		if j >= len(p.rows) {
			lines[i] = strings.Repeat(" ", inner)
			continue
		}
		lines[i] = p.line(th, p.rows[j], inner, j == p.sel)
	}
	if len(p.rows) == 0 {
		lines[0] = theme.Cells(th.Muted, " No suggestions waiting", inner)
	}
	footer := fmt.Sprintf("%d waiting", len(p.items))
	b := th.Frame(inner, "Suggestions", footer, lines)
	p.box = overlay.Box{ID: ID, X: max(width-ansi.StringWidth(b[0]), 0), Y: overlay.GridTop, Lines: b}
	return []overlay.Box{p.box}
}

// line draws a row in inner columns: a suggestion's agent, message and
// size, or a cell's address and its value then and now, with the
// accept and reject chips at the right of the one highlighted.
func (p *Panel) line(th *theme.Theme, r row, inner int, sel bool) string {
	it := p.items[r.item]
	base := th.MenuBar
	if sel {
		base = th.MenuSelected
	}
	chips := ""
	if sel && r.other == "" {
		chips = " " + th.Chip("✓") + " " + th.Chip("✗")
	}
	room := inner - ansi.StringWidth(chips)
	var text string
	switch {
	case r.other != "":
		return theme.Cells(th.Muted, "     also "+r.other, inner)
	case r.cell < 0:
		what := it.Message
		if what == "" {
			what = it.Label
		}
		size := fmt.Sprintf("%d cells", len(it.Cells))
		if it.Whole {
			size = "whole"
		} else if len(it.Cells) == 1 {
			size = "1 cell"
		}
		text = " " + agentMark(it.Agent) + ": " + what + " (" + size + ")"
		if it.Failed != "" {
			text += " ! " + it.Failed
		}
		if !sel {
			return th.Peer[it.Color%theme.Peers].Render(" ◆ ") + theme.Cells(base, strings.TrimPrefix(text, " ◆"), room-3) + chips
		}
	default:
		c := it.Cells[r.cell]
		text = "   ◇ " + cellName(it, c) + "  " + shown(c.Was) + " → " + shown(c.Now)
		if c.Look && c.Was == c.Now {
			text = "   ◇ " + cellName(it, c) + "  format"
		}
	}
	return theme.Cells(base, text, room) + chips
}

// agentMark is one of the agents's name with the mark agents have.
func agentMark(name string) string { return "◆ " + name }

// cellName is a cell's address, with its sheet when the suggestion sets
// cells on more than one.
func cellName(it Item, c Cell) string {
	for _, o := range it.Cells {
		if o.Sheet != c.Sheet {
			return sheet.Qualified(c.Sheet, sheet.Rect{From: c.At, To: c.At})
		}
	}
	return c.At.String()
}

// shown is an input as the panel shows it: blank for none.
func shown(in string) string {
	if in == "" {
		return "blank"
	}
	return in
}

// target is what the highlighted row settles: a suggestion and the
// cells picked, nil for all it has waiting.
func (p *Panel) target() (int, []int, bool) {
	if len(p.rows) == 0 {
		return 0, nil, false
	}
	r := p.rows[p.sel]
	it := p.items[r.item]
	if r.cell < 0 || it.Whole {
		return it.ID, nil, true
	}
	return it.ID, []int{it.Cells[r.cell].Index}, true
}

func (p *Panel) Key(k tea.KeyPressMsg) tea.Cmd {
	switch k.String() {
	case "up", "k":
		p.move(-1)
	case "down", "j":
		p.move(1)
	case "enter", "a", "y":
		p.settle(true)
	case "r", "n", "delete", "backspace":
		p.settle(false)
	case "A":
		p.all(true)
	case "R":
		p.all(false)
	case "esc", "q":
		p.h.Close()
	}
	return nil
}

// move highlights the row d away that settles something.
func (p *Panel) move(d int) {
	for i := p.sel + d; i >= 0 && i < len(p.rows); i += d {
		if p.rows[i].other == "" {
			p.sel = i
			break
		}
	}
	p.show()
}

// settle accepts or rejects what the highlighted row stands for.
func (p *Panel) settle(accept bool) {
	id, cells, ok := p.target()
	if !ok {
		return
	}
	if accept {
		p.h.Accept(id, cells)
	} else {
		p.h.Reject(id, cells)
	}
	p.after()
}

// all accepts or rejects every suggestion waiting.
func (p *Panel) all(accept bool) {
	for _, it := range p.items {
		if accept {
			p.h.Accept(it.ID, nil)
		} else {
			p.h.Reject(it.ID, nil)
		}
	}
	p.after()
}

// after lists what's left, closing when nothing is.
func (p *Panel) after() {
	p.refresh()
	if len(p.rows) == 0 {
		p.h.Close()
		return
	}
	p.show()
}

func (p *Panel) Mouse(e overlay.MouseEvent) tea.Cmd {
	switch {
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelUp:
		p.move(-1)
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelDown:
		p.move(1)
	case e.Kind != overlay.MousePress:
	case e.Box != ID:
		p.h.Close()
	default:
		p.click(e.Col, e.Row)
	}
	return nil
}

// click highlights the row clicked, and on its chips accepts or rejects
// it: the chips are the last eight columns inside the frame.
func (p *Panel) click(col, line int) {
	i := p.top + line - 1 // the frame's top border
	if line < 1 || i >= len(p.rows) || p.rows[i].other != "" {
		return
	}
	was := p.sel
	p.sel = i
	inner := p.box.Width() - 2
	switch {
	case i != was:
		p.show()
	case col > inner-4 && col <= inner:
		p.settle(false)
	case col > inner-8 && col <= inner-4:
		p.settle(true)
	default:
		p.show()
	}
}

func (p *Panel) Status() (string, string) {
	th := p.h.Theme()
	keys := th.KeyHints("Enter", "accept", "R", "reject", "Shift+A", "all", "Esc", "close")
	if len(p.rows) == 0 {
		return "", keys
	}
	it := p.items[p.rows[p.sel].item]
	desc := it.Agent + " suggests: " + it.Label
	if it.Message != "" {
		desc = it.Agent + ": " + it.Message
	}
	return desc, keys
}
