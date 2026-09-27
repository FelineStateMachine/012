package rules

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// A form is a list of rows in the manner of lazygit's panels: Up and
// Down pick a row, Left and Right change a choice, Space flips a check,
// and a text row is typed into (in the host's edit line) while it's
// picked.

type rowKind uint8

const (
	rowText rowKind = iota
	rowChoice
	rowCheck
	rowSep
	rowPreview
)

// row is one line of a form, pointing into the form's state.
type row struct {
	kind        rowKind
	label       string
	hint        string // what the row sets, for the status line
	text        *string
	placeholder string
	choices     []string
	at          *int
	// swatch, when set, draws a sample before each choice's name.
	swatch  func(i int) string
	on      *bool
	preview func(w int) string
}

// picks reports whether the row can be picked.
func (r row) picks() bool { return r.kind != rowSep && r.kind != rowPreview }

// labelW is the width of the label column.
const labelW = 12

// draw lays out the row in w columns; sel marks it as picked, and
// editing is the text being typed into it.
func (r row) draw(th *theme.Theme, w int, sel bool, editing string) string {
	base, dim := th.MenuBar, th.Muted
	if sel {
		base, dim = th.MenuSelected, th.MenuSelected
	}
	lead := base.Render(" " + theme.PadRight(r.label, labelW-1))
	var val string
	switch r.kind {
	case rowSep:
		return theme.SepRow
	case rowCheck:
		box := "[ ] "
		if *r.on {
			box = "[x] "
		}
		return theme.Cells(base, " "+box+r.label, w)
	case rowPreview:
		return spread(base, lead+r.preview(w-labelW-1), "", w)
	case rowChoice:
		chip := th.KeyChip
		if sel {
			chip = th.Indicator
		}
		i := min(max(*r.at, 0), len(r.choices)-1)
		val = chip.Render("‹ " + r.choices[i] + " ›")
		if r.swatch != nil {
			val = r.swatch(i) + base.Render(" ") + val
		}
	case rowText:
		text := *r.text
		if sel {
			text = editing
		}
		switch {
		case text == "" && !sel:
			val = dim.Render(r.placeholder)
		case sel:
			val = base.Render(text)
		default:
			val = th.Key.Render(text)
		}
	}
	return spread(base, lead+base.Render(" ")+val, "", w)
}

// spread lays left and right out in w columns on base, cutting left to
// make room for right.
func spread(base lipgloss.Style, left, right string, w int) string {
	rw := ansi.StringWidth(right)
	left = ansi.Truncate(left, max(w-rw-1, 1), "…")
	gap := max(w-ansi.StringWidth(left)-rw, 0)
	return ansi.Truncate(left+base.Render(strings.Repeat(" ", gap))+right, w, "")
}

// cycle moves a choice row d steps, wrapping around.
func (r row) cycle(d int) {
	n := len(r.choices)
	*r.at = ((*r.at+d)%n + n) % n
}

// textX is where a text row's value starts, from the box's inner left.
func textX() int { return labelW + 1 }

// canonicalArgs are a rule's two arguments typed in loc, trimmed, as the
// rule stores them (see sheet.CanonicalArg).
func canonicalArgs(args [2]string, loc *locale.Locale) [2]string {
	return [2]string{sheet.CanonicalArg(strings.TrimSpace(args[0]), loc), sheet.CanonicalArg(strings.TrimSpace(args[1]), loc)}
}

// localArgs are a rule's two arguments as typed in loc.
func localArgs(args [2]string, loc *locale.Locale) [2]string {
	return [2]string{sheet.LocalArg(args[0], loc), sheet.LocalArg(args[1], loc)}
}
