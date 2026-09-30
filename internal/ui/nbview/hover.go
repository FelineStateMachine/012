package nbview

import (
	"context"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// The word at the caret, on the context line: once the caret rests on
// a command or a flag, its signature and what it does, as nu's hover
// says; on a $name the notebook binds, the cell it comes from and the
// shape of its value, from the notebook's own outputs without asking
// nu. A problem at the caret wins the line. F1 opens the command's
// whole help (HelpMsg).

// Hover is what the context line says of a word of a cell's source.
type Hover struct {
	From, To int    // the word's bytes
	Text     string // sort-by <...comparator: cell-path|closure> --reverse, or $files: table, 12 rows × 4 columns from cell 3
	Desc     string // what it does, or ""
	// Brief is Text where there's no room for it: sort-by <...comparator> ….
	Brief string
	// Help is the command's whole help, for F1; nil for a variable.
	Help *nushell.Help
}

// Hoverer says what the word at a byte of src is.
type Hoverer interface {
	Hover(ctx context.Context, src string, offset int) (Hover, bool)
}

// HelpMsg asks the UI to show a command's help, or its keyboard
// shortcuts when Help is nil: F1 on a word nu has no help for.
type HelpMsg struct{ Help *nushell.Help }

type (
	// restedMsg is the caret having rested since move moves.
	restedMsg struct {
		view  *View
		moves int
	}
	// hoveredMsg is the answer about the word at the caret after moves
	// moves; open says F1 asked, and waits to show its help.
	hoveredMsg struct {
		view  *View
		moves int
		src   string
		hover Hover
		ok    bool
		open  bool
	}
)

// rest notes that the caret moved or the text changed: what was asked
// about the word it was at is stale, and the word it's at is asked
// about once it rests.
func (v *View) rest() tea.Cmd {
	e := &v.edit
	e.moves++
	e.stopHover()
	moves := e.moves
	return tea.Tick(pause, func(time.Time) tea.Msg { return restedMsg{view: v, moves: moves} })
}

func (e *editor) stopHover() {
	if e.hoverStop != nil {
		e.hoverStop()
		e.hoverStop = nil
	}
}

// updateHover takes the hover's messages.
func (v *View) updateHover(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case restedMsg:
		if msg.view != v {
			return nil, false
		}
		if v.edit.on && msg.moves == v.edit.moves {
			return v.askHover(false), true
		}
	case hoveredMsg:
		if msg.view != v {
			return nil, false
		}
		if !v.edit.on || msg.moves != v.edit.moves {
			return nil, true
		}
		v.edit.hover, v.edit.hoverFor = msg.hover, msg.src
		if !msg.ok {
			v.edit.hover = Hover{}
		}
		if msg.open {
			return helpCmd(v.edit.hover.Help), true
		}
	default:
		return nil, false
	}
	return nil, true
}

func helpCmd(h *nushell.Help) tea.Cmd { return func() tea.Msg { return HelpMsg{Help: h} } }

// askHover asks about the word at the caret: a $name of the notebook's
// at once, anything else in the background.
func (v *View) askHover(open bool) tea.Cmd {
	e := &v.edit
	src, off, moves := e.text(), e.area.Offset(), e.moves
	at, ok := wordAt(src, off)
	if !ok || !v.editingCode() {
		e.hover = Hover{}
		if open {
			return helpCmd(nil)
		}
		return nil
	}
	if h, ok := v.cellHover(src, at); ok {
		e.hover, e.hoverFor = h, src
		if open {
			return helpCmd(nil)
		}
		return nil
	}
	hv := v.Providers.Hoverer
	if hv == nil {
		return helpCmdIf(open)
	}
	e.stopHover()
	ctx, stop := context.WithCancel(context.Background())
	e.hoverStop = stop
	return func() tea.Msg {
		h, ok := hv.Hover(ctx, src, at)
		if ctx.Err() != nil {
			return nil
		}
		return hoveredMsg{view: v, moves: moves, src: src, hover: h, ok: ok, open: open}
	}
}

func helpCmdIf(open bool) tea.Cmd {
	if open {
		return helpCmd(nil)
	}
	return nil
}

// editingCode reports whether the cell being edited is code.
func (v *View) editingCode() bool {
	for _, c := range v.h.Cells() {
		if c.ID == v.edit.id {
			return c.Kind == notebook.Code
		}
	}
	return false
}

// WordHelp is F1 in edit mode: the help of the command at the caret,
// asked for if it hasn't been, or the shortcuts when there's none.
func (v *View) WordHelp() tea.Cmd {
	if h, ok := v.hovered(); ok && h.Help != nil {
		return helpCmd(h.Help)
	}
	if !v.edit.on {
		return helpCmd(nil)
	}
	return v.askHover(true)
}

// hovered is what's said of the word at the caret, while the text is
// what it was said of.
func (v *View) hovered() (Hover, bool) {
	e := &v.edit
	if !e.on || e.hover.Text == "" || e.hoverFor != e.text() {
		return Hover{}, false
	}
	off := e.area.Offset()
	return e.hover, off >= e.hover.From && off <= e.hover.To
}

// isWordByte reports whether b is part of a word: not a space, a
// bracket, a pipe or a quote.
func isWordByte(b byte) bool { return !strings.ContainsRune(" \t\n|;(){}[],'\"`", rune(b)) }

// wordAt is the byte of src to ask about for the caret at off: the
// word it's in, or the one it's just after.
func wordAt(src string, off int) (int, bool) {
	switch {
	case off < len(src) && isWordByte(src[off]):
		return off, true
	case off > 0 && off <= len(src) && isWordByte(src[off-1]):
		return off - 1, true
	}
	return 0, false
}

// wordBounds are the bytes of the word holding byte at.
func wordBounds(src string, at int) (int, int) {
	from, to := at, at
	for from > 0 && isWordByte(src[from-1]) {
		from--
	}
	for to < len(src) && isWordByte(src[to]) {
		to++
	}
	return from, to
}

// varName is the name $name at byte from of src reads, and where it
// ends: files of $files.name.
func varName(src string, from int) (string, int) {
	if from >= len(src) || src[from] != '$' {
		return "", from
	}
	to := from + 1
	for to < len(src) && (isNameByte(src[to])) {
		to++
	}
	return src[from+1 : to], to
}

func isNameByte(b byte) bool {
	return b == '_' || b == '-' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= 0x80
}

// cellHover is what the notebook knows of the $name at byte at: the
// cell whose output or variable it is, and its value's shape.
func (v *View) cellHover(src string, at int) (Hover, bool) {
	from, _ := wordBounds(src, at)
	name, to := varName(src, from)
	if name == "" || at >= to {
		return Hover{}, false
	}
	for i, c := range v.h.Cells() {
		if c.Kind != notebook.Code || c.ID == v.edit.id {
			continue
		}
		var shape string
		switch {
		case c.Name() == name:
			shape = shapeWords(v.shown(c), v.h.Output(c.ID))
		case slices.Contains(c.Parse().Assigned(), name):
			shape = varShape(v.h.Output(c.ID), name)
		default:
			continue
		}
		text := "$" + name + ": " + shape + " from cell " + itoa(i+1)
		if shape == "" {
			text = "$" + name + ": from cell " + itoa(i+1) + ", not run yet"
		}
		return Hover{From: from, To: to, Text: text}, true
	}
	return Hover{}, false
}

// varShape is the shape of variable name of a cell's last run.
func varShape(o *notebook.Output, name string) string {
	if o == nil || o.Vars[name] == nil {
		return ""
	}
	val, err := nuon.Parse(o.Vars[name])
	if err != nil {
		return "a value"
	}
	sh := shownOf(val)
	if sh.kind == outValue {
		return val.Kind.String()
	}
	return shapeWords(sh, nil)
}

// shapeWords is an output's shape, "table, 12 rows × 4 columns", or ""
// before it has run.
func shapeWords(sh *shown, o *notebook.Output) string {
	switch sh.kind {
	case outTable:
		return "table, " + more(sh.total, "row") + " × " + more(sh.cols, "column")
	case outRecord:
		return "record, " + more(sh.total, "field")
	case outList:
		return "list, " + more(sh.total, "item")
	case outText:
		if sh.total > 1 {
			return "string, " + more(sh.total, "line")
		}
		return "string"
	case outError:
		return "an error"
	case outValue:
		if o != nil {
			if val, err := nuon.Parse(o.NUON); err == nil {
				return val.Kind.String()
			}
		}
		return "a value"
	case outNone:
		if o != nil && o.Count > 0 {
			return "nothing"
		}
	}
	return ""
}

// hoverLine draws h in room columns: the signature in the text's own
// color, then what it does, muted;
// flags are left out, then the types, then the description cut, before
// the rest is.
func hoverLine(th *theme.Theme, h Hover, room int) string {
	text, desc := h.Text, h.Desc
	width := func() int {
		w := ansi.StringWidth(text)
		if desc != "" {
			w += 2 + ansi.StringWidth(desc)
		}
		return w
	}
	for width() > room && desc != "" {
		i := strings.LastIndex(text, " --")
		if i < 0 {
			break
		}
		text = strings.TrimSuffix(text[:i], " …") + " …"
	}
	if width() > room && h.Brief != "" {
		text = h.Brief
	}
	if desc == "" {
		return th.Cell.Render(ansi.Truncate(text, max(room, 1), "…"))
	}
	out := th.Cell.Render(ansi.Truncate(text, max(room, 1), "…"))
	if left := room - ansi.StringWidth(text) - 2; left > 3 {
		out += "  " + th.Muted.Render(ansi.Truncate(desc, left, "…"))
	}
	return out
}

// Highlighted draws a line of nushell in its syntax roles, as 012's
// own highlighter has them: a command's usage and examples in its help.
func Highlighted(th *theme.Theme, src string) string {
	buf := []rune(src)
	roles := rolesOf(src, Tokens{}.Highlight(context.Background(), src))
	return drawRow(th, buf, lineedit.Row{Start: 0, End: len(buf)}, roles, nil)
}
