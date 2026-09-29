package nbview

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Full is an output shown full-screen, read-only: a table as a grid
// with a pointer, sorted by a column (s, S) and filtered to the rows
// holding some text (/), without changing the output; anything else as
// its lines, scrolled. Esc goes back to the notebook.
type Full struct {
	Title string
	sh    *shown
	order []int // the rows shown, in the order shown
	sort  int   // the column sorted by, plus one; 0 for the output's order
	desc  bool
	// filter is the text rows must hold; typing is set while it's typed.
	filter lineedit.Line
	typing bool

	row, col      int // the pointer
	top, left     int
	width, height int
}

func newFull(title string, sh *shown, width, height int) *Full {
	f := &Full{Title: title, sh: sh}
	f.resize(width, height)
	f.apply()
	return f
}

func (f *Full) resize(width, height int) { f.width, f.height = width, max(height, 2) }

// table reports whether the output is a table.
func (f *Full) table() bool { return f.sh.kind == outTable }

// apply works out the rows shown from the filter and the sort.
func (f *Full) apply() {
	if !f.table() {
		return
	}
	f.order = f.order[:0]
	needle := strings.ToLower(f.filter.Text())
	for i, row := range f.sh.rows {
		if needle == "" || slices.ContainsFunc(row, func(lc sheet.LiveCell) bool {
			return strings.Contains(strings.ToLower(cellText(lc, locale.Canonical)), needle)
		}) {
			f.order = append(f.order, i)
		}
	}
	if c := f.sort - 1; c >= 0 {
		slices.SortStableFunc(f.order, func(a, b int) int {
			x, y := cellAt(f.sh.rows[a], c), cellAt(f.sh.rows[b], c)
			if f.desc {
				return compareCells(y, x)
			}
			return compareCells(x, y)
		})
	}
	f.row = min(f.row, max(len(f.order)-1, 0))
}

func cellAt(row []sheet.LiveCell, c int) sheet.LiveCell {
	if c < len(row) {
		return row[c]
	}
	return sheet.LiveCell{}
}

// compareCells orders values as a sort does: numbers before text,
// blanks last.
func compareCells(a, b sheet.LiveCell) int {
	ae, be := a.V.Kind == sheet.Empty, b.V.Kind == sheet.Empty
	switch {
	case ae || be:
		return cmp.Compare(boolInt(ae), boolInt(be))
	case a.V.Kind == sheet.Number && b.V.Kind == sheet.Number:
		return cmp.Compare(a.V.Num, b.V.Num)
	case a.V.Kind == sheet.Number:
		return -1
	case b.V.Kind == sheet.Number:
		return 1
	}
	return cmp.Compare(strings.ToLower(a.V.String()), strings.ToLower(b.V.String()))
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// rowsShown is how many rows (or lines) there are to scroll through.
func (f *Full) rowsShown() int {
	if f.table() {
		return len(f.order)
	}
	return f.sh.height(true, f.width-1)
}

// Key takes a key, reporting whether Esc left the view.
func (f *Full) Key(k tea.KeyPressMsg) (closed bool) {
	key := k.String()
	if f.typing {
		switch key {
		case "esc":
			f.typing = false
			f.filter.Clear()
		case "enter":
			f.typing = false
		default:
			f.filter.Key(k)
		}
		f.apply()
		return false
	}
	page := max(f.height-2, 1)
	switch key {
	case "esc", "q":
		return true
	case "up", "k":
		f.row--
	case "down", "j":
		f.row++
	case "left", "h":
		f.col--
	case "right", "l":
		f.col++
	case "pgup":
		f.row -= page
	case "pgdown", "space":
		f.row += page
	case "home", "g":
		f.row = 0
	case "end", "G":
		f.row = f.rowsShown() - 1
	case "s", "S":
		f.sortBy(f.col+1, key == "S")
	case "/":
		f.typing = true
	}
	f.settle()
	return false
}

// sortBy sorts by column c (counted from 1), again to go back to the
// output's order.
func (f *Full) sortBy(c int, desc bool) {
	if !f.table() {
		return
	}
	if f.sort == c && f.desc == desc {
		f.sort = 0
	} else {
		f.sort, f.desc = c, desc
	}
	f.apply()
}

// settle keeps the pointer on the table and on screen.
func (f *Full) settle() {
	f.row = max(min(f.row, f.rowsShown()-1), 0)
	if f.table() {
		f.col = max(min(f.col, len(f.sh.cols)-1), 0)
		f.left = min(f.left, f.col)
		for f.left < f.col && f.colX(f.col)+f.sh.fit[f.col] > f.width {
			f.left++
		}
	}
	rows := f.height - 1
	if !f.table() {
		rows = f.height
	}
	f.top = max(min(f.top, f.row), f.row-rows+1, 0)
}

// numW is the row numbers' width.
func (f *Full) numW() int { return len(strconv.Itoa(max(len(f.sh.rows), 1))) + 1 }

// colX is where column c starts on screen, with the columns from left.
func (f *Full) colX(c int) int {
	x := f.numW() + 1
	for i := f.left; i < c; i++ {
		x += f.sh.fit[i] + 2
	}
	return x
}

// lines draws the view.
func (f *Full) lines(th *theme.Theme, loc *locale.Locale) []string {
	out := make([]string, 0, f.height)
	if !f.table() {
		for i := f.top; i < f.top+f.height && i < f.rowsShown(); i++ {
			line := f.sh.line(th, loc, i, true, f.width-1)
			if i == f.row {
				line = th.Selection.Render("▌") + line
			} else {
				line = " " + line
			}
			out = append(out, line)
		}
		return pad2Lines(out, f.height)
	}
	out = append(out, f.header(th))
	for i := f.top; i < f.top+f.height-1 && i < len(f.order); i++ {
		out = append(out, f.rowLine(th, loc, i))
	}
	return pad2Lines(out, f.height)
}

func pad2Lines(out []string, n int) []string {
	for len(out) < n {
		out = append(out, "")
	}
	return out
}

// header is the column names, the sorted one marked ▲ or ▼, the
// pointer's column's in reverse video.
func (f *Full) header(th *theme.Theme) string {
	var b strings.Builder
	b.WriteString(th.Header.Render(strings.Repeat(" ", f.numW()+1)))
	for c := f.left; c < len(f.sh.cols) && f.colX(c) < f.width; c++ {
		name := f.sh.cols[c]
		if f.sort == c+1 {
			name += map[bool]string{false: " ▲", true: " ▼"}[f.desc]
		}
		style := th.Header
		if c == f.col {
			style = th.HeaderActive
		}
		b.WriteString(style.Render(pad(ansi.Truncate(name, f.sh.fit[c], "…"), f.sh.fit[c], sheet.AlignLeft)))
		b.WriteString(th.Header.Render("  "))
	}
	return ansi.Truncate(b.String(), f.width, "")
}

// rowLine is the i-th row shown, numbered as the output numbers it.
func (f *Full) rowLine(th *theme.Theme, loc *locale.Locale, i int) string {
	r := f.order[i]
	num := th.Header
	if i == f.row {
		num = th.HeaderActive
	}
	var b strings.Builder
	b.WriteString(num.Render(pad(strconv.Itoa(r+1), f.numW(), sheet.AlignRight)) + " ")
	row := f.sh.rows[r]
	for c := f.left; c < len(f.sh.cols) && f.colX(c) < f.width; c++ {
		lc := cellAt(row, c)
		text := pad(ansi.Truncate(cellText(lc, loc), f.sh.fit[c], "…"), f.sh.fit[c], align(lc))
		if i == f.row && c == f.col {
			text = th.Pointer.Render(text)
		}
		b.WriteString(text + "  ")
	}
	return ansi.Truncate(b.String(), f.width, "")
}

// ContextLine says what's shown and the keys.
func (f *Full) ContextLine(th *theme.Theme) (string, string) {
	if f.typing {
		return th.Key.Render("Filter: ") + f.filter.Text(), th.KeyHints("Enter", "keep", "Esc", "clear")
	}
	what := f.Title
	if f.table() {
		what += "  " + more(len(f.order), "row")
		if f.filter.Text() != "" {
			what += " holding " + strconv.Quote(f.filter.Text())
		}
		if f.sort > 0 {
			what += ", sorted by " + f.sh.cols[f.sort-1]
		}
		return th.Muted.Render(what), th.KeyHints("s", "sort", "S", "descending", "/", "filter", "Esc", "back")
	}
	return th.Muted.Render(what), th.KeyHints("Esc", "back")
}

// Cursor is where the terminal's caret goes while the filter is typed,
// on the context line after "Filter: ".
func (f *Full) Cursor() (int, bool) {
	if !f.typing {
		return 0, false
	}
	return ansi.StringWidth("Filter: " + f.filter.Head()), true
}
