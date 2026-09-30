package nbview

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Edit mode: the selected cell's source in a multi-line editor
// (lineedit.Area), soft-wrapped to the cell's width with the caret
// always on screen. What the language says of it (highlighting,
// diagnostics) is asked for in the background once typing pauses;
// completions when Tab asks.

// pause is how long typing pauses before the language is asked.
const pause = 80 * time.Millisecond

// afterPause gives msg once typing has paused, on the view's clock.
func (v *View) afterPause(msg tea.Msg) tea.Cmd {
	if v.After != nil {
		return v.After(pause, msg)
	}
	return tea.Tick(pause, func(time.Time) tea.Msg { return msg })
}

// editor is the cell being edited.
type editor struct {
	on      bool
	id      int
	area    lineedit.Area
	version int // counts changes, so answers to older text are dropped
	diags   []Diagnostic
	diagFor string // the text the diagnostics are of
	comp    []Completion
	compSel int
	stop    context.CancelFunc // stops the questions about the text still out
	// moves counts the caret's moves and the text's changes, so an
	// answer about a word the caret has left is dropped.
	moves     int
	hover     Hover  // what's said of the word at the caret
	hoverFor  string // the text it was said of
	hoverStop context.CancelFunc
}

func (e *editor) text() string { return e.area.Text() }

func (e *editor) rows(width int) []lineedit.Row { return lineedit.Wrap(e.area.Buf, width) }

// caret is the caret's row and column at width.
func (e *editor) caret(width int) (int, int) {
	return lineedit.Caret(e.rows(width), e.area.Buf, e.area.Pos)
}

// line draws row r of the text being edited.
func (e *editor) line(th *theme.Theme, r, width int, roles []int8) string {
	rows := e.rows(width)
	if r >= len(rows) {
		return ""
	}
	return drawRow(th, e.area.Buf, rows[r], roles, e.marks())
}

// marks are the runes the diagnostics underline: those of the text they
// were of, where the text is unchanged since, so the underline doesn't
// blink off as keys are typed.
func (e *editor) marks() []bool {
	if len(e.diags) == 0 {
		return nil
	}
	text := e.text()
	marks := diagMarks(e.diagFor, e.diags)
	if e.diagFor == text {
		return marks
	}
	return blend(e.diagFor, text, marks, make([]bool, len(e.area.Buf)))
}

// diagMarks marks the runes a diagnostic covers.
func diagMarks(src string, diags []Diagnostic) []bool {
	marks := make([]bool, len([]rune(src)))
	ri := 0
	for bi := range src {
		for _, d := range diags {
			if bi >= d.From && bi < max(d.To, d.From+1) {
				marks[ri] = true
			}
		}
		ri++
	}
	return marks
}

// drawRow draws a row of buf in its syntax roles (a role per rune, -1
// for none), marked runes curly-underlined, a continuation row indented.
func drawRow(th *theme.Theme, buf []rune, row lineedit.Row, roles []int8, marks []bool) string {
	var b strings.Builder
	if row.Cont {
		b.WriteString(strings.Repeat(" ", lineedit.Indent))
	}
	i := row.Start
	for i < row.End {
		j := i + 1
		for j < row.End && roleAt(roles, j) == roleAt(roles, i) && markAt(marks, j) == markAt(marks, i) {
			j++
		}
		text := string(buf[i:j])
		r, marked := roleAt(roles, i), markAt(marks, i)
		switch {
		case r < 0 && !marked:
			b.WriteString(text)
		case r < 0:
			b.WriteString(th.ErrorMark.Render(text))
		case marked:
			b.WriteString(th.Code[r].Inherit(th.ErrorMark).Render(text))
		default:
			b.WriteString(th.Code[r].Render(text))
		}
		i = j
	}
	return b.String()
}

func roleAt(roles []int8, i int) int8 {
	if i < len(roles) {
		return roles[i]
	}
	return -1
}

func markAt(marks []bool, i int) bool { return i < len(marks) && marks[i] }

// Messages the view sends itself.
type (
	// highlightMsg is the highlighter's answer about src; edited says
	// src was the text being edited.
	highlightMsg struct {
		view   *View
		src    string
		spans  []Span
		edited bool
	}
	// pausedMsg is typing having paused at version.
	pausedMsg struct {
		view    *View
		version int
	}
	// checkedMsg is the checker's answer about src.
	checkedMsg struct {
		view  *View
		src   string
		diags []Diagnostic
	}
	// completedMsg is the completer's answer for version.
	completedMsg struct {
		view    *View
		version int
		comp    []Completion
	}
)

// Update takes the view's own messages, reporting whether msg was one.
func (v *View) Update(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case highlightMsg:
		if msg.view != v {
			return nil, false
		}
		v.keepSpans(msg.src, msg.spans)
		delete(v.syntax.asked, msg.src)
		if msg.edited {
			v.syntax.last = msg.src
		}
	case pausedMsg:
		if msg.view != v {
			return nil, false
		}
		if v.edit.on && msg.version == v.edit.version {
			return v.check(), true
		}
	case checkedMsg:
		if msg.view != v {
			return nil, false
		}
		v.edit.diags, v.edit.diagFor = msg.diags, msg.src
	case completedMsg:
		if msg.view != v {
			return nil, false
		}
		if v.edit.on && msg.version == v.edit.version {
			v.edit.comp, v.edit.compSel = msg.comp, 0
			if len(msg.comp) == 1 {
				v.complete()
			}
		}
	default:
		return v.updateHover(msg)
	}
	return nil, true
}

// changed notes that the text changed: what the language was asked
// about the text before is stale, and it's asked about this text once
// typing pauses.
func (v *View) changed() tea.Cmd {
	v.edit.version++
	v.edit.comp = nil
	v.edit.cancel()
	rest := v.rest()
	if !v.asksLater() {
		return rest
	}
	version := v.edit.version
	return tea.Batch(rest, v.afterPause(pausedMsg{view: v, version: version}))
}

// cancel stops the questions about the text being edited that are
// still out.
func (e *editor) cancel() {
	if e.stop != nil {
		e.stop()
		e.stop = nil
	}
	e.stopHover()
}

// check asks the highlighter and checker about the text being edited,
// in the background; answers that come once it has changed are dropped.
func (v *View) check() tea.Cmd {
	src := v.edit.text()
	v.edit.cancel()
	ctx, stop := context.WithCancel(context.Background())
	v.edit.stop = stop
	var cmds []tea.Cmd
	if hl := v.Providers.Highlighter; hl != nil && !isInstant(hl) {
		cmds = append(cmds, func() tea.Msg {
			spans := hl.Highlight(ctx, src)
			if ctx.Err() != nil {
				return nil
			}
			return highlightMsg{view: v, src: src, spans: spans, edited: true}
		})
	}
	if ck := v.Providers.Checker; ck != nil && !isInstant(ck) {
		cmds = append(cmds, func() tea.Msg {
			diags := ck.Check(ctx, src)
			if ctx.Err() != nil {
				return nil
			}
			return checkedMsg{view: v, src: src, diags: diags}
		})
	}
	return tea.Batch(cmds...)
}

// askCompletions asks what can go at the caret.
func (v *View) askCompletions() tea.Cmd {
	cp := v.Providers.Completer
	if cp == nil {
		return nil
	}
	src, off, version := v.edit.text(), v.edit.area.Offset(), v.edit.version
	return func() tea.Msg {
		return completedMsg{view: v, version: version, comp: cp.Complete(context.Background(), src, off)}
	}
}

// complete puts the highlighted completion in place.
func (v *View) complete() {
	e := &v.edit
	if len(e.comp) == 0 {
		return
	}
	c := e.comp[e.compSel]
	src := e.text()
	from, to := min(c.From, len(src)), min(c.To, len(src))
	e.area.SetText(src[:from] + c.Text)
	tail := src[to:]
	pos := e.area.Pos
	e.area.InsertText(tail)
	e.area.Pos = pos
	e.comp = nil
	e.version++
}

// Diagnostic is the problem the caret is on, for the context line, or
// "" when it is on none.
func (v *View) Diagnostic() string {
	e := &v.edit
	if !e.on || e.diagFor != e.text() {
		return ""
	}
	off := e.area.Offset()
	for _, d := range e.diags {
		if off >= d.From && off <= max(d.To, d.From+1) {
			return d.Msg
		}
	}
	return ""
}

// Completions are the completions shown, and the highlighted one.
func (v *View) Completions() ([]Completion, int) { return v.edit.comp, v.edit.compSel }

// completionBox draws the completions under the caret's row: at most
// eight, the highlighted one in reverse video.
func (v *View) completionBox(th *theme.Theme) []string {
	comp, sel := v.edit.comp, v.edit.compSel
	if len(comp) == 0 {
		return nil
	}
	first := max(0, min(sel-7, len(comp)-8))
	shown := comp[first:min(first+8, len(comp))]
	tw, dw := 0, 0
	for _, c := range shown {
		tw = max(tw, ansi.StringWidth(c.Text))
		dw = max(dw, ansi.StringWidth(c.Desc))
	}
	inner := min(tw+2+dw+2, max(v.width-gutter-4, 10))
	lines := make([]string, len(shown))
	for i, c := range shown {
		text := ansi.Truncate(" "+c.Text+"  "+c.Desc, inner, "…")
		text += strings.Repeat(" ", max(inner-ansi.StringWidth(text), 0))
		if first+i == sel {
			lines[i] = th.MenuSelected.Render(text)
		} else {
			lines[i] = th.MenuBar.Render(text)
		}
	}
	return th.Frame(inner, "Completions", "", lines)
}
