package nbview

import (
	"context"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/notebook"
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
	var marks []bool
	if e.diagFor == e.text() && len(e.diags) > 0 {
		marks = diagMarks(e.text(), e.diags)
	}
	return drawRow(th, e.area.Buf, rows[r], roles, marks)
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

// syntaxCache keeps what the highlighter said of each source, as a role
// per rune, and which sources it has been asked about.
type syntaxCache struct {
	spans map[string][]Span
	roles map[string][]int8
	asked map[string]bool
	want  []string // sources drawn without an answer yet
}

// Instant is a highlighter quick enough to ask while drawing, as the
// built-in tokenizer is.
type Instant interface{ Instant() bool }

// Instant implements Instant: the tokenizer is.
func (Tokens) Instant() bool { return true }

// spansFor is src's roles, a role per rune, or nil until the
// highlighter has answered.
func (v *View) spansFor(src string, kind notebook.Kind) []int8 {
	hl := v.Providers.Highlighter
	if kind != notebook.Code || hl == nil {
		return nil
	}
	c := &v.syntax
	if roles, ok := c.roles[src]; ok {
		return roles
	}
	if in, ok := hl.(Instant); ok && in.Instant() {
		v.keepSpans(src, hl.Highlight(context.Background(), src))
		return c.roles[src]
	}
	if c.asked == nil {
		c.asked = map[string]bool{}
	}
	if !c.asked[src] {
		c.asked[src] = true
		c.want = append(c.want, src)
	}
	return nil
}

// keepSpans keeps what the highlighter said of src.
func (v *View) keepSpans(src string, spans []Span) {
	c := &v.syntax
	if c.roles == nil || len(c.roles) > 512 {
		c.roles, c.spans = map[string][]int8{}, map[string][]Span{}
	}
	c.spans[src] = spans
	roles := make([]int8, len([]rune(src)))
	for i := range roles {
		roles[i] = -1
	}
	ri := 0
	for bi := range src {
		for _, s := range spans {
			if bi >= s.From && bi < s.To {
				roles[ri] = int8(s.Kind)
			}
		}
		ri++
	}
	c.roles[src] = roles
}

// Messages the view sends itself.
type (
	// highlightMsg is the highlighter's answer about src.
	highlightMsg struct {
		src   string
		spans []Span
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

// Fetch asks the highlighter about the sources drawn without an answer,
// in the background.
func (v *View) Fetch() tea.Cmd {
	hl := v.Providers.Highlighter
	want := v.syntax.want
	v.syntax.want = nil
	if hl == nil || len(want) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, len(want))
	for i, src := range want {
		cmds[i] = func() tea.Msg { return highlightMsg{src: src, spans: hl.Highlight(context.Background(), src)} }
	}
	return tea.Batch(cmds...)
}

// Update takes the view's own messages, reporting whether msg was one.
func (v *View) Update(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case highlightMsg:
		v.keepSpans(msg.src, msg.spans)
		if v.syntax.asked != nil {
			delete(v.syntax.asked, msg.src)
		}
	case pausedMsg:
		if msg.view == v && v.edit.on && msg.version == v.edit.version {
			return v.check(), true
		}
	case checkedMsg:
		if msg.view == v {
			v.edit.diags, v.edit.diagFor = msg.diags, msg.src
		}
	case completedMsg:
		if msg.view == v && v.edit.on && msg.version == v.edit.version {
			v.edit.comp, v.edit.compSel = msg.comp, 0
			if len(msg.comp) == 1 {
				v.complete()
			}
		}
	default:
		return nil, false
	}
	return nil, true
}

// changed notes that the text changed: the language is asked about it
// once typing pauses.
func (v *View) changed() tea.Cmd {
	v.edit.version++
	v.edit.comp = nil
	if v.Providers.Checker == nil && (v.Providers.Highlighter == nil || isInstant(v.Providers.Highlighter)) {
		return nil
	}
	version := v.edit.version
	return tea.Tick(pause, func(time.Time) tea.Msg { return pausedMsg{view: v, version: version} })
}

func isInstant(hl Highlighter) bool {
	in, ok := hl.(Instant)
	return ok && in.Instant()
}

// check asks the highlighter and checker about the text being edited.
func (v *View) check() tea.Cmd {
	src := v.edit.text()
	var cmds []tea.Cmd
	if hl := v.Providers.Highlighter; hl != nil && !isInstant(hl) {
		cmds = append(cmds, func() tea.Msg { return highlightMsg{src: src, spans: hl.Highlight(context.Background(), src)} })
	}
	if ck := v.Providers.Checker; ck != nil {
		cmds = append(cmds, func() tea.Msg { return checkedMsg{view: v, src: src, diags: ck.Check(context.Background(), src)} })
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

// Diagnostic is the problem at the caret, for the context line.
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
	if len(e.diags) > 0 {
		return e.diags[0].Msg
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
