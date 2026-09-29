// Package nuprompt is the nushell prompt of a notebook: a pipeline typed
// on the formula bar after nu❯, with the workbook's history on Up and
// Down and completions of regions ($r1) and nu's commands in a box
// under the context line. It knows the UI only through Host: running a
// line, and what's running, are the host's.
package nuprompt

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the prompt needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Line is the shared edit line the command is typed in.
	Line() *lineedit.Line
	// Close closes the prompt.
	Close()
	// Submit runs a line; the prompt stays open for the next.
	Submit(line string) tea.Cmd
	// Words are what can be completed: the regions and nu's commands.
	Words() []Word
	// ShellHistory is the workbook's lines, oldest first.
	ShellHistory() []string
	// Running names the region running, or "" when none is.
	Running() string
	// Stop stops what's running.
	Stop()
	// Said is what the last run said: how many rows, or why it failed.
	Said() string
}

// Word is a completion.
type Word struct {
	Text string // what Tab puts on the line: $r1, or a command
	Desc string // what it is: a region's command, or "command"
}

// ID identifies the completions' box in mouse events.
const ID = "nuprompt"

// Mark starts the prompt on the formula bar.
const Mark = "nu❯ "

// Prompt is the open prompt.
type Prompt struct {
	h Host
	overlay.List
	shown  []Word
	tabbed bool // the word is a completion Tab put there; Tab again moves on
	hist   browse
}

// New returns a prompt with the edit line holding text.
func New(h Host, text string) *Prompt {
	p := &Prompt{h: h}
	h.Line().Set(text)
	p.Changed()
	return p
}

// Shown are the completions of the word at the caret.
func (p *Prompt) Shown() []Word { return p.shown }

func (p *Prompt) Indicator() string { return "NU" }

// word is the word before the caret, and where it starts: a $name, or a
// command where one goes (at the start, or after |, ( or ;).
func (p *Prompt) word() (string, int, bool) {
	head := []rune(p.h.Line().Head())
	i := len(head)
	for i > 0 && !strings.ContainsRune(" \t|(;{[", head[i-1]) {
		i--
	}
	w := string(head[i:])
	if strings.HasPrefix(w, "$") {
		return w, i, true
	}
	before := strings.TrimRight(string(head[:i]), " \t")
	command := before == "" || strings.ContainsAny(before[len(before)-1:], "|(;{") ||
		strings.HasSuffix(before, "=") && !strings.HasSuffix(before, "==")
	return w, i, command && w != ""
}

// Changed completes the word at the caret.
func (p *Prompt) Changed() {
	p.Sel, p.Top, p.tabbed = 0, 0, false
	p.shown = p.shown[:0]
	w, _, ok := p.word()
	if !ok {
		return
	}
	lw := strings.ToLower(w)
	var contains []Word
	for _, it := range p.h.Words() {
		if strings.HasPrefix(it.Text, "$") != strings.HasPrefix(w, "$") {
			continue
		}
		switch t := strings.ToLower(it.Text); {
		case t == lw:
		case strings.HasPrefix(t, lw):
			p.shown = append(p.shown, it)
		case len(lw) > 1 && strings.Contains(t, lw):
			contains = append(contains, it)
		}
	}
	p.shown = append(p.shown, contains...)
}

func (p *Prompt) Key(k tea.KeyPressMsg) tea.Cmd {
	line := p.h.Line()
	switch key := k.String(); key {
	case "esc":
		if p.h.Running() != "" {
			p.h.Stop()
			return nil
		}
		p.h.Close()
	case "enter":
		text := strings.TrimSpace(line.Text())
		if text == "" {
			return nil
		}
		line.Clear()
		p.hist = browse{}
		p.Changed()
		return p.h.Submit(text)
	case "tab":
		p.complete(1)
	case "shift+tab":
		p.complete(-1)
	case "up", "down":
		p.recall(key)
	case "ctrl+p":
		p.Move(-1, len(p.shown))
	case "ctrl+n":
		p.Move(1, len(p.shown))
	default:
		before := line.Text()
		line.Key(k)
		if line.Text() != before {
			p.hist = browse{}
			p.Changed()
		}
	}
	return nil
}

// recall is Up and Down: an older or newer line of the history, among
// those starting with what was typed.
func (p *Prompt) recall(key string) {
	d := -1
	if key == "down" {
		d = 1
	}
	text, ok := p.hist.step(p.h.ShellHistory(), p.h.Line().Text(), d)
	if !ok {
		return
	}
	p.h.Line().Set(text)
	p.Changed()
	p.shown = p.shown[:0] // a line recalled is complete
}

// complete puts the highlighted completion in place of the word at the
// caret; pressed again, it moves on to the next (d = 1) or the previous.
func (p *Prompt) complete(d int) {
	if len(p.shown) == 0 {
		return
	}
	if p.tabbed {
		p.Move(d, len(p.shown))
	}
	p.put(p.shown[p.Sel].Text)
	p.tabbed = true
}

// put replaces the word at the caret with text.
func (p *Prompt) put(text string) {
	line := p.h.Line()
	_, start, _ := p.completionStart()
	tail := line.Tail()
	head := string(line.Buf[:start]) + text
	line.Set(head + tail)
	line.Pos = len([]rune(head))
}

// completionStart is where the word Tab replaces starts: the word at the
// caret, or where it was before the last Tab.
func (p *Prompt) completionStart() (string, int, bool) {
	head := []rune(p.h.Line().Head())
	i := len(head)
	for i > 0 && !strings.ContainsRune(" \t|(;{[", head[i-1]) {
		i--
	}
	if p.tabbed { // the word Tab put may hold spaces: str join
		for _, it := range p.shown {
			if n := len([]rune(it.Text)); n <= len(head) && string(head[len(head)-n:]) == it.Text {
				return it.Text, len(head) - n, true
			}
		}
	}
	return string(head[i:]), i, true
}

// FormulaBar is what the formula bar shows after the name box.
func (p *Prompt) FormulaBar() string {
	return p.h.Theme().Title.Render(Mark) + p.h.Line().Text()
}

// Cursor is on the formula bar, after the prompt's mark.
func (p *Prompt) Cursor() (int, int) {
	return overlay.FormulaBarX + ansi.StringWidth(Mark) + ansi.StringWidth(p.h.Line().Head()), overlay.FormulaLine
}

// ContextLine says what's running, or what the last run said.
func (p *Prompt) ContextLine() (string, string) {
	th := p.h.Theme()
	if r := p.h.Running(); r != "" {
		return th.Hint.Render("Running " + r + "…"), th.KeyHints("Esc", "stop")
	}
	if said := p.h.Said(); said != "" {
		return said, th.KeyHints("Esc", "back to the grid")
	}
	return th.KeyHints("Enter", "run", "name = ", "name it", "$r1", "a region", "$in", "the selection"), ""
}

func (p *Prompt) Status() (string, string) {
	if p.Sel >= len(p.shown) {
		return "", "" // the sheet's tabs, with the notebook
	}
	return p.shown[p.Sel].Desc, p.h.Theme().KeyHints("Tab", "complete", "Ctrl+N", "next", "Enter", "run", "Esc", "grid")
}

// rows is how many completions show.
func (p *Prompt) rows() int {
	_, height := p.h.Size()
	return max(min(len(p.shown), 8, height-overlay.ContextLine-4), 0)
}

// Layout draws the completions in a box under the context line.
func (p *Prompt) Layout() []overlay.Box {
	rows := p.rows()
	if rows == 0 {
		return nil
	}
	p.Show(rows)
	th := p.h.Theme()
	width, _ := p.h.Size()
	tw, dw := 0, 0
	for _, it := range p.shown {
		tw = max(tw, ansi.StringWidth(it.Text))
		dw = max(dw, ansi.StringWidth(it.Desc))
	}
	inner := min(max(1+tw+2+dw+1, 30), 72, width-2)
	tw = min(tw, inner-2)
	dw = max(inner-1-tw-2-1, 0)
	lines := make([]string, rows)
	for r := range rows {
		i := p.Top + r
		it := p.shown[i]
		base, dim := th.MenuBar, th.Muted
		if i == p.Sel {
			base, dim = th.MenuSelected, th.MenuSelected
		}
		row := base.Render(" "+theme.PadRight(ansi.Truncate(it.Text, tw, "…"), tw)+"  ") +
			dim.Render(theme.PadRight(ansi.Truncate(it.Desc, dw, "…"), dw)) + base.Render(" ")
		lines[r] = row
	}
	footer := strconv.Itoa(p.Sel+1) + " of " + strconv.Itoa(len(p.shown))
	x := min(overlay.FormulaBarX, max(width-inner-2, 0))
	return []overlay.Box{{ID: ID, X: x, Y: overlay.ContextLine + 1, Lines: th.Frame(inner, "Completions", footer, lines)}}
}

func (p *Prompt) Mouse(e overlay.MouseEvent) tea.Cmd {
	if e.Box != ID {
		return nil // the grid stays where it is while typing
	}
	i := p.Top + e.Row - 1 // under the top border
	switch {
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelUp:
		p.Move(-1, len(p.shown))
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelDown:
		p.Move(1, len(p.shown))
	case e.Row < 1 || i >= len(p.shown) || i >= p.Top+p.rows():
	case e.Kind == overlay.MouseMotion:
		p.Sel = i
	case e.Kind == overlay.MousePress && e.Button == tea.MouseLeft:
		p.Sel = i
		p.put(p.shown[i].Text)
		p.Changed()
	}
	return nil
}

// browse is a walk through the history with Up and Down: only lines
// starting with what was typed before the first Up.
type browse struct {
	on     bool
	at     int    // the line shown, len(lines) for what was typed
	prefix string // what was typed
}

// step moves d lines through lines (-1 older, 1 newer) from what's
// shown, skipping lines that don't start with the prefix, and returns
// the text to show and whether there was a line to move to.
func (b *browse) step(lines []string, typed string, d int) (string, bool) {
	if !b.on {
		*b = browse{on: true, at: len(lines), prefix: typed}
	}
	for i := b.at + d; i >= 0 && i <= len(lines); i += d {
		if i == len(lines) {
			b.at = i
			return b.prefix, true
		}
		if strings.HasPrefix(lines[i], b.prefix) {
			b.at = i
			return lines[i], true
		}
	}
	return "", false
}
