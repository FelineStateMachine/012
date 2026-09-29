package nbview

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Full is an output shown full-screen, read-only: a table or record as
// the UI's grid at the body's size, which the UI gives the keys, and
// anything else as its lines, scrolled with a pointer. Esc goes back to
// the notebook.
type Full struct {
	Title string
	sh    *shown

	row, top      int // the pointer and the first line shown, for lines
	width, height int
}

func newFull(title string, sh *shown, width, height int) *Full {
	f := &Full{Title: title, sh: sh}
	f.resize(width, height)
	return f
}

func (f *Full) resize(width, height int) { f.width, f.height = width, max(height, 2) }

// Grid is the output's grid, when it's drawn as one: the UI gives it
// the keys.
func (f *Full) Grid() Grid { return f.sh.grid }

// rowsShown is how many lines there are to scroll through.
func (f *Full) rowsShown() int { return f.sh.height(true, f.width-1) }

// Key takes a key, reporting whether Esc left the view.
func (f *Full) Key(k tea.KeyPressMsg) (closed bool) {
	page := max(f.height-1, 1)
	switch k.String() {
	case "esc", "q":
		return true
	case "up", "k":
		f.row--
	case "down", "j":
		f.row++
	case "pgup":
		f.row -= page
	case "pgdown", "space":
		f.row += page
	case "home", "g":
		f.row = 0
	case "end", "G":
		f.row = f.rowsShown() - 1
	}
	f.settle()
	return false
}

// settle keeps the pointer on the lines and on screen.
func (f *Full) settle() {
	f.row = max(min(f.row, f.rowsShown()-1), 0)
	f.top = max(min(f.top, f.row), f.row-f.height+1, 0)
}

// lines draws the view: the grid at full size, its header on the first
// line, or the output's lines with the pointer's marked.
func (f *Full) lines(th *theme.Theme, loc *locale.Locale) []string {
	out := make([]string, 0, f.height)
	if g := f.sh.grid; g != nil {
		rows := f.height - 1
		from := g.Top()
		for i := range min(g.Rows(), rows) + 1 {
			out = append(out, g.Line(i, from, rows, f.width))
		}
		return padLines(out, f.height)
	}
	for i := f.top; i < f.top+f.height && i < f.rowsShown(); i++ {
		line := f.sh.line(th, loc, i, fold{whole: true}, f.width-1)
		if i == f.row {
			line = th.Selection.Render("▌") + line
		} else {
			line = " " + line
		}
		out = append(out, line)
	}
	return padLines(out, f.height)
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func padLines(out []string, n int) []string {
	for len(out) < n {
		out = append(out, "")
	}
	return out
}

// ContextLine says what's shown and the keys.
func (f *Full) ContextLine(th *theme.Theme) (string, string) {
	return th.Muted.Render(f.Title), th.KeyHints("Esc", "back")
}
