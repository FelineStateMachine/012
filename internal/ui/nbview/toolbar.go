package nbview

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// The toolbar over the cells, as Jupyter's: run, stop, restart, run
// all; add, cut, copy, paste; the active cell's kind; and at the right
// how running stands, as Jupyter's kernel status. Each button runs its
// command when clicked and shows its key as a chip where there's room:
// the chips go first, from the right, then the words, the icons stay.

// Back is what ToolbarAt says of the button that leaves a full-screen
// output, which has no command.
const Back = "back"

const backButton = "◀ Back"

// button is a toolbar button.
type button struct {
	icon, label, cmd string
	on               func(v *View, k Kernel) bool
}

func always(*View, Kernel) bool       { return true }
func hasCell(v *View, _ Kernel) bool  { return len(v.h.Cells()) > 0 }
func busy(_ *View, k Kernel) bool     { return k.Busy || k.Waiting > 0 }
func canPaste(_ *View, k Kernel) bool { return k.Clip > 0 }
func canRun(v *View, k Kernel) bool   { return hasCell(v, k) && k.Off == "" }
func separator(*View, Kernel) bool    { return false }
func (b button) sep() bool            { return b.cmd == "" }
func (b button) text(level int, chip string) string {
	s := b.icon
	if level > 0 && b.label != "" {
		s += " " + b.label
	}
	if level > 1 && chip != "" {
		s += " " + chip
	}
	return s
}

// buttons are the toolbar's, a blank cmd between groups; the kind's
// label is the active cell's.
func (v *View) buttons() []button {
	kind := "Code ▾"
	if c, ok := v.Cell(); ok && c.Kind == notebook.Note {
		kind = "Markdown ▾"
	}
	return []button{
		{"▶", "Run", "nb.run_next", canRun}, {"■", "Stop", "nb.stop", busy}, {"↻", "Restart", "nb.restart", hasCell},
		{"▶▶", "Run all", "nb.run_all", canRun}, {"│", "", "", separator},
		{"+", "Add", "nb.insert_below", always}, {"✂", "Cut", "nb.cut", hasCell}, {"⧉", "Copy", "nb.copy", hasCell},
		{"⎘", "Paste", "nb.paste", canPaste}, {"│", "", "", separator},
		{kind, "", "nb.kind", hasCell},
	}
}

// toolbarGap is the space between buttons.
const toolbarGap = 2

// layout fits the buttons in width, beside the status: how much of each
// shows (2 with its key, 1 with its word, 0 its icon alone).
func (v *View) layout(bs []button, width int) []int {
	levels := make([]int, len(bs))
	chips := make([]string, len(bs))
	for i, b := range bs {
		levels[i] = 2
		chips[i] = keyFor(v.Keys, b.cmd)
	}
	fits := func() bool {
		w := 1
		for i, b := range bs {
			w += ansi.StringWidth(b.text(levels[i], chips[i])) + 2*boolInt(levels[i] > 1 && chips[i] != "") + toolbarGap
		}
		return w <= width
	}
	for level := 2; level > 0 && !fits(); level-- {
		for i := len(bs) - 1; i >= 0 && !fits(); i-- {
			if levels[i] == level {
				levels[i] = level - 1
			}
		}
	}
	return levels
}

// Toolbar draws the toolbar at width: the full-screen output's title
// while one is open.
func (v *View) Toolbar(width int) string {
	th := v.h.Theme()
	if v.full != nil {
		return " " + backButton + " " + th.Chip("Esc") + "   " + th.Title.Render(v.full.Title) + th.Muted.Render(", full-screen")
	}
	k := v.h.Kernel()
	status := v.status(k)
	bs := v.buttons()
	levels := v.layout(bs, width-ansi.StringWidth(status)-2)
	var b strings.Builder
	b.WriteString(" ")
	for i, bt := range bs {
		if i > 0 {
			b.WriteString(strings.Repeat(" ", toolbarGap))
		}
		b.WriteString(v.drawButton(bt, levels[i], k))
	}
	left := b.String()
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(status) - 1
	if gap < 1 {
		return left
	}
	return left + strings.Repeat(" ", gap) + status
}

// drawButton draws a button as much as level says: a disabled one
// dimmed, a separator as a rule.
func (v *View) drawButton(bt button, level int, k Kernel) string {
	th := v.h.Theme()
	if bt.sep() {
		return th.Border.Render(bt.icon)
	}
	chip := keyFor(v.Keys, bt.cmd)
	if !bt.on(v, k) {
		return th.Disabled.Render(bt.text(min(level, 1), ""))
	}
	s := bt.text(min(level, 1), "")
	if level > 1 && chip != "" {
		s += " " + th.Chip(chip)
	}
	return s
}

// status is how running stands: nu ○ idle, nu ● busy, nu ⊘ off, after
// reactive when the notebook is.
func (v *View) status(k Kernel) string {
	th := v.h.Theme()
	var s string
	switch {
	case k.Off != "":
		s = th.Warning.Render("nu ⊘ off")
	case k.Busy && k.Waiting > 0:
		s = th.Hint.Render("nu ● busy, " + itoa(k.Waiting) + " waiting")
	case k.Busy || k.Waiting > 0:
		s = th.Hint.Render("nu ● busy")
	default:
		s = th.Muted.Render("nu ○ idle")
	}
	if k.Reactive {
		s = th.Muted.Render("reactive") + "  " + s
	}
	return s
}

// ToolbarAt is the command of the button at column x of the toolbar,
// or "".
func (v *View) ToolbarAt(x, width int) string {
	if v.full != nil {
		if x >= 1 && x <= ansi.StringWidth(backButton) {
			return Back
		}
		return ""
	}
	k := v.h.Kernel()
	bs := v.buttons()
	levels := v.layout(bs, width-ansi.StringWidth(v.status(k))-2)
	at := 1
	for i, bt := range bs {
		if i > 0 {
			at += toolbarGap
		}
		w := ansi.StringWidth(v.drawButton(bt, levels[i], k))
		if x >= at && x < at+w && !bt.sep() && bt.on(v, k) {
			return bt.cmd
		}
		at += w
	}
	return ""
}
