// Package evalview is Data > Evaluate formula, after Excel's Evaluate
// Formula: a box over the grid showing the active cell's formula with
// the part computed next underlined and its value below. Enter puts the
// value in its place and underlines the next part, until the formula's
// value is left; → steps into a reference to another formula, ← and Esc
// step back out. The engine does the evaluating (sheet.Steps); the view
// draws it. It knows the UI only through Host.
package evalview

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the view needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Close closes the open overlay.
	Close()
	// Locale writes formulas as the user types them.
	Locale() *locale.Locale
}

// View is the open Evaluate formula box.
type View struct {
	h     Host
	stack []*sheet.Steps // the formula stepped into last on top
}

// ID identifies the view's box in mouse events.
const ID = "evaluate"

// maxInner is the widest the box's inside gets.
const maxInner = 76

// New returns the view of st, a formula about to be stepped through.
func New(h Host, st *sheet.Steps) *View { return &View{h: h, stack: []*sheet.Steps{st}} }

// Steps is the formula being stepped through: the one stepped into last.
func (v *View) Steps() *sheet.Steps { return v.stack[len(v.stack)-1] }

// Depth is how many formulas are open, the first one included.
func (v *View) Depth() int { return len(v.stack) }

func (v *View) Indicator() string { return "EVAL" }

// Key: Enter computes the next part (at the end, restarts, or steps
// out of a formula stepped into), → steps in, ← steps out, Esc steps
// out or closes.
func (v *View) Key(k tea.KeyPressMsg) tea.Cmd {
	st := v.Steps()
	switch k.String() {
	case "enter", "space":
		switch {
		case !st.Done():
			st.Step()
		case len(v.stack) > 1:
			v.out()
		default:
			st.Restart()
		}
	case "right":
		if in := st.Into(); in != nil {
			v.stack = append(v.stack, in)
		}
	case "left":
		if len(v.stack) > 1 {
			v.out()
		}
	case "esc":
		if len(v.stack) > 1 {
			v.out()
			return nil
		}
		v.h.Close()
	}
	return nil
}

// out steps out of the formula stepped into: the reference to it takes
// its value in the formula before.
func (v *View) out() {
	v.stack = v.stack[:len(v.stack)-1]
	v.Steps().Step()
}

// Mouse closes the box on a click outside it.
func (v *View) Mouse(e overlay.MouseEvent) tea.Cmd {
	if e.Kind == overlay.MousePress && e.Box != ID {
		v.h.Close()
	}
	return nil
}

// Status says what the keys do now.
func (v *View) Status() (string, string) {
	th, st := v.h.Theme(), v.Steps()
	var keys []string
	switch {
	case !st.Done():
		keys = append(keys, "Enter", "evaluate")
	case len(v.stack) > 1:
		keys = append(keys, "Enter", "step out")
	default:
		keys = append(keys, "Enter", "restart")
	}
	if st.CanStepIn() {
		keys = append(keys, "→", "step in")
	}
	if len(v.stack) > 1 {
		keys = append(keys, "←", "step out")
	}
	keys = append(keys, "Esc", "close")
	desc := "The underlined part is computed next"
	if st.Done() {
		desc = "Every part is computed"
	}
	return desc, th.KeyHints(keys...)
}

// Layout is the box: the formula, wrapped, then the next part and its
// value, under the menu bar and centered.
func (v *View) Layout() []overlay.Box {
	th := v.h.Theme()
	width, height := v.h.Size()
	inner := min(width-2, maxInner)
	st := v.Steps()
	text := v.formulaLines(inner-2, max(height-12, 1))
	rows := make([]string, 0, len(text)+2)
	for _, l := range text {
		rows = append(rows, th.MenuBar.Render(" ")+l+th.MenuBar.Render(strings.Repeat(" ", max(inner-1-ansi.StringWidth(l), 0))))
	}
	rows = append(rows, theme.SepRow, v.nextRow(inner))
	done, total := st.Progress()
	footer := strconv.Itoa(done) + " of " + strconv.Itoa(total)
	return []overlay.Box{{ID: ID, X: (width - inner - 2) / 2, Y: overlay.MenuLine + 1, Lines: th.Frame(inner, v.title(), footer, rows)}}
}

// title names the formulas open: Evaluate C1 › B1.
func (v *View) title() string {
	first, _ := v.stack[0].Cell()
	names := make([]string, len(v.stack))
	for i, st := range v.stack {
		s, a := st.Cell()
		names[i] = a.String()
		if s != first {
			names[i] = sheet.Qualified(s.Name(), sheet.Rect{From: a, To: a})
		}
	}
	return "Evaluate " + strings.Join(names, " › ")
}

// nextRow is "Next  B1 = 8", or once done "Value  14".
func (v *View) nextRow(inner int) string {
	th, st, loc := v.h.Theme(), v.Steps(), v.h.Locale()
	line := th.Muted.Render(" Value  ") + th.Evaluated.Render(formula.Localize(st.Result(), loc))
	if expr, value, ok := st.Next(); ok {
		line = th.Muted.Render(" Next  ") + th.EvalNext.Render(formula.Localize(expr, loc)) + th.Muted.Render(" = ") + th.Evaluated.Render(formula.Localize(value, loc))
	}
	line = ansi.Truncate(line, inner, "…")
	return line + th.MenuBar.Render(strings.Repeat(" ", max(inner-ansi.StringWidth(line), 0)))
}

// run is text in one style.
type run struct {
	text  string
	style *lipgloss.Style
}

// formulaLines draws the formula as it stands in lines of w columns, at
// most max of them: those from the line the next part starts on.
func (v *View) formulaLines(w, most int) []string {
	th, loc := v.h.Theme(), v.h.Locale()
	both := th.Evaluated.Bold(true).Underline(true)
	var lines [][]run
	var cur []run
	col, nextLine := 0, -1
	for _, sg := range v.Steps().Text() {
		style := &th.MenuBar
		switch {
		case sg.Value && sg.Next:
			style = &both
		case sg.Value:
			style = &th.Evaluated
		case sg.Next:
			style = &th.EvalNext
		}
		if sg.Next && nextLine < 0 {
			nextLine = len(lines)
		}
		for _, r := range formula.Localize(sg.Text, loc) {
			rw := ansi.StringWidth(string(r))
			if col+rw > w && col > 0 {
				lines, cur, col = append(lines, cur), nil, 0
			}
			if n := len(cur); n > 0 && cur[n-1].style == style {
				cur[n-1].text += string(r)
			} else {
				cur = append(cur, run{string(r), style})
			}
			col += rw
		}
	}
	lines = append(lines, cur)
	start := 0
	if nextLine >= 0 && len(lines) > most {
		start = min(max(nextLine-1, 0), len(lines)-most)
	}
	out := make([]string, 0, min(len(lines), most))
	for _, l := range lines[start:min(len(lines), start+most)] {
		var b strings.Builder
		for _, r := range l {
			b.WriteString(r.style.Render(r.text))
		}
		out = append(out, b.String())
	}
	return out
}
