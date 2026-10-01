package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// The context line explains the active cell's error, or why a linked
// source's tab can't show its rows, in one line. An explanation longer
// than the line is cut where a sentence ends when one fits, else with …,
// and an F1 chip at the right says how to read the rest: F1 there opens
// the whole explanation in a box (beside the cell, as a note shows on
// hover), and anywhere else the keyboard shortcuts, as F1 at a word of a
// notebook's cell does.

func init() {
	register(&command{id: "help.here", macro: macroNever, title: "Help here",
		desc: "Explain the active cell's error in full, or else show the keyboard shortcuts",
		run: func(m *Model) tea.Cmd {
			if b := m.explained(); b != nil {
				m.openOverlay(b)
				return nil
			}
			return m.runCommand("help")
		}})
	keymap["f1"] = "help.here"
}

// explained is the box explaining what the context line explains, or
// nil when it explains nothing.
func (m *Model) explained() *errorBox {
	if m.srcView() != nil {
		if title, why := m.sourceTrouble(); why != "" {
			return &errorBox{m: m, title: title, why: why, beside: false}
		}
		return nil
	}
	if v, why := m.errorWhy(); why != "" {
		return &errorBox{m: m, title: v.Str + " in " + m.cur.String(), why: why, at: m.cur, beside: true}
	}
	return nil
}

// errorWhy is the active cell's error and why it shows it, "" when it
// shows none or there's nothing to say.
func (m *Model) errorWhy() (sheet.Value, string) {
	if m.srcView() != nil || m.nbView() != nil {
		return sheet.Value{}, ""
	}
	v := m.sheet.Value(m.cur)
	if v.Kind != sheet.Error {
		return v, ""
	}
	return v, m.sheet.ExplainError(m.cur)
}

// errorLine explains the active cell's error on the context line, e.g.
// "#DIV/0!  Division by zero in B3/0", and when that is too long for the
// line, the part that fits and an F1 chip for the rest.
func (m *Model) errorLine() (string, string) {
	v, why := m.errorWhy()
	if why == "" {
		return "", ""
	}
	return m.explainLine(m.th.ErrorCell.Render(v.Str)+"  ", why, m.th.Muted)
}

// explainLine is head then why in style, on one line: the part of why
// that fits and an F1 chip for the rest when it doesn't.
func (m *Model) explainLine(head, why string, style lipgloss.Style) (string, string) {
	if ansi.StringWidth(head)+ansi.StringWidth(why) <= m.width {
		return head + style.Render(why), ""
	}
	chip := m.th.KeyHints("F1", "more")
	room := m.width - ansi.StringWidth(head) - ansi.StringWidth(chip) - 3
	return head + style.Render(cutExplanation(why, room)), chip
}

// spread puts right at the right edge after left, dropping it if there
// isn't room for both.
func (m *Model) spread(left, right string) string {
	gap := m.width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if right == "" || gap < 3 {
		return left
	}
	return left + strings.Repeat(" ", gap) + right
}

// cutExplanation is the start of why in at most room columns: its
// sentences that fit, when at least one does, followed by …; else as
// much of it as fits.
func cutExplanation(why string, room int) string {
	if room < 2 {
		return ""
	}
	end := -1
	for i := 0; i < len(why); i++ {
		if (why[i] == '.' || why[i] == ';') && i+1 < len(why) && why[i+1] == ' ' && ansi.StringWidth(why[:i+1])+2 <= room {
			end = i + 1
		}
	}
	if end > 0 {
		return why[:end] + " …"
	}
	return ansi.Truncate(why, room, "…")
}

// errorBox is an error's whole explanation in a box over the grid,
// beside its cell or under the context line; any key closes it.
type errorBox struct {
	m      *Model
	title  string
	why    string
	at     sheet.Addr
	beside bool // beside the cell at, else at the grid's top left
}

const errorBoxID = "error"

// errorBoxWidth is the widest the explanation's lines run, for reading.
const errorBoxWidth = 48

func (b *errorBox) Indicator() string { return "HELP" }

// Layout places the box as a note's: right of the cell, or left of it
// when there's no room, wrapped to its width.
func (b *errorBox) Layout() []overlay.Box {
	m := b.m
	inner := min(errorBoxWidth, m.width-2)
	lines := strings.Split(ansi.Wordwrap(b.why, inner-2, ""), "\n")
	rows := make([]string, len(lines))
	for i, line := range lines {
		rows[i] = m.th.MenuBar.Render(theme.PadRight(" "+strings.TrimRight(line, " "), inner))
	}
	framed := m.th.Frame(inner, b.title, "", rows)
	if !b.beside {
		return []overlay.Box{{ID: errorBoxID, X: 0, Y: gridTop, Lines: framed}}
	}
	x, y := m.cellPos(b.at)
	x += m.sheet.ColWidth(b.at.Col)
	if x+inner+2 > m.width {
		x = max(x-m.sheet.ColWidth(b.at.Col)-inner-2, 0)
	}
	x, y = m.clampBox(x, y, inner+2, len(framed))
	return []overlay.Box{{ID: errorBoxID, X: x, Y: max(y, gridTop), Lines: framed}}
}

// Key closes the box, and a key that moves or runs a command does so
// too, as it would have without the box.
func (b *errorBox) Key(k tea.KeyPressMsg) tea.Cmd {
	b.m.closeOverlay()
	switch k.String() {
	case "esc", "enter", "f1":
		return nil
	}
	return b.m.handleKey(k)
}

// Mouse closes the box on a click anywhere.
func (b *errorBox) Mouse(e overlay.MouseEvent) tea.Cmd {
	if e.Kind == overlay.MousePress {
		b.m.closeOverlay()
	}
	return nil
}

func (b *errorBox) Status() (string, string) {
	return b.title, b.m.th.KeyHints("Esc", "close")
}
