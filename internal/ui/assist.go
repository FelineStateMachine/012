package ui

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Formula assistance, as in Sheets: while a function or range name is
// being typed, a list of matching functions and named ranges drops down
// from the formula bar at the caret, and while the caret is inside a
// function's parentheses the context line shows its signature with the
// current argument marked. The list takes only the keys Sheets gives it
// (Up/Down, Tab, Enter, Esc) and only while it shows, so arrows after an
// operator still point at cells.

// assist is the suggestion list: its state, and the keys, mouse, box
// and status line it takes over while it shows. Like an overlay, it is
// handed the model, for the entry and the sheet's names.
type assist struct {
	active   bool // the last key typed or deleted text; moving the caret hides the list
	sel, top int  // highlighted suggestion and first one shown; reset as the word changes
}

// assistRows is the most suggestions shown at once.
const assistRows = 8

const assistID = "assist"

// suggestion is a function or named range offered for the word at the
// caret.
type suggestion struct {
	name   string // e.g. "SUM" or "Sales"
	detail string // the signature, or the range a name stands for
	desc   string
	fn     bool // a function: accepting it adds "("
}

// suggestions lists the functions and named ranges for word: those
// starting with it first (names before functions), then, from two
// letters on, those containing it, such as COUNTIFS for "ifs". A word
// that is already a cell reference only gets names and functions that
// start with it.
func suggestions(sh *sheet.Sheet, word string) []suggestion {
	w := strings.ToUpper(word)
	var prefix, inner []suggestion
	add := func(s suggestion, name string) {
		switch {
		case strings.HasPrefix(name, w):
			prefix = append(prefix, s)
		case len(w) >= 2 && strings.Contains(name, w):
			inner = append(inner, s)
		}
	}
	for _, n := range sh.Names() {
		add(suggestion{name: n.Name, detail: n.Ref(), desc: "Named range " + n.Name + ": " + n.Ref()}, strings.ToUpper(n.Name))
	}
	for _, f := range sheet.Funcs() {
		add(suggestion{name: f.Name, detail: f.Name + "(" + f.Args + ")", desc: f.Desc, fn: true}, f.Name)
	}
	if _, isRef := sheet.ParseAddr(w); isRef {
		return prefix
	}
	return append(prefix, inner...)
}

// shown returns the list for the word at the caret, and where
// the word starts, or nothing when the list is hidden: outside ENTER and
// EDIT, in plain text, after the caret moved, or when the word already
// names a range exactly and nothing else matches.
func (a *assist) shown(m *Model) ([]suggestion, int) {
	if (m.mode != modeEnter && m.mode != modeEdit) || !m.line.isFormula() || !a.active || m.overlay != nil {
		return nil, 0
	}
	c := scanCaret(m.line.buf, m.line.pos)
	if c.word == "" {
		return nil, 0
	}
	list := suggestions(m.sheet, c.word)
	if len(list) == 0 || len(list) == 1 && !list[0].fn && strings.EqualFold(list[0].name, c.word) {
		return nil, 0
	}
	a.sel = min(a.sel, len(list)-1)
	return list, c.wordStart
}

// typeKey applies a line-editing key to the entry. Typing or deleting
// shows suggestions; moving the caret hides them, as in Sheets.
func (m *Model) typeKey(k tea.KeyPressMsg) {
	before := m.line.text()
	m.line.key(k)
	m.entry.assist = assist{active: m.line.text() != before}
}

// key handles the keys the suggestion list takes while it shows.
func (a *assist) key(m *Model, key string) bool {
	list, start := a.shown(m)
	if list == nil {
		return false
	}
	switch key {
	case "up", "ctrl+p":
		a.sel = (a.sel - 1 + len(list)) % len(list)
	case "down", "ctrl+n":
		a.sel = (a.sel + 1) % len(list)
	case "tab", "enter":
		a.accept(m, list[a.sel], start)
	case "esc":
		a.active = false
	default:
		return false
	}
	return true
}

// accept replaces the word at the caret with s, adding "(" after
// a function unless one is already there.
func (a *assist) accept(m *Model, s suggestion, start int) {
	text := []rune(s.name)
	rest := m.line.buf[m.line.pos:]
	if s.fn && (len(rest) == 0 || rest[0] != '(') {
		text = append(text, '(')
	}
	pos := start + len(text)
	if s.fn && len(rest) > 0 && rest[0] == '(' {
		pos++ // into the existing parentheses
	}
	m.line.buf = slices.Concat(m.line.buf[:start], text, rest)
	m.line.pos = pos
	a.active = false
}

// box draws the list under the formula bar, its text aligned with
// the word being typed: the name, then the signature or range dimmed.
func (a *assist) box(m *Model) (box, bool) {
	list, start := a.shown(m)
	if list == nil {
		return box{}, false
	}
	rows := min(len(list), assistRows, m.height-gridTop-2)
	if rows < 1 {
		return box{}, false
	}
	a.top = clamp(a.top, a.sel-rows+1, a.sel)
	a.top = clamp(a.top, 0, len(list)-rows)
	nw, dw := 0, 0
	for _, s := range list {
		nw = max(nw, ansi.StringWidth(s.name))
		dw = max(dw, ansi.StringWidth(s.detail))
	}
	inner := min(1+nw+3+dw+1, 64, m.width-2)
	lines := make([]string, 0, rows)
	for i := a.top; i < a.top+rows; i++ {
		s := list[i]
		base, dim := m.th.MenuBar, m.th.Muted
		if i == a.sel {
			base, dim = m.th.MenuSelected, m.th.MenuSelected
		}
		detail := s.detail
		if s.fn {
			detail = s.detail[len(s.name):] // just the arguments: (value1, ...)
		}
		row := base.Render(" "+theme.PadRight(s.name, nw)+"   ") + dim.Render(detail)
		lines = append(lines, ansi.Truncate(row, inner, "…")+base.Render(strings.Repeat(" ", max(inner-ansi.StringWidth(row), 0))))
	}
	footer := ""
	if len(list) > rows {
		footer = strconv.Itoa(a.sel+1) + " of " + strconv.Itoa(len(list))
	}
	// The box's text starts one column in, under the word's first letter.
	x := formulaBarTextX() + ansi.StringWidth(string(m.line.buf[:start])) - 2
	x = clamp(x, 0, max(m.width-inner-2, 0))
	return box{id: assistID, x: x, y: contextLine + 1, lines: m.th.Frame(inner, "", footer, lines)}, true
}

// mouse lets the mouse hover and click suggestions and scroll the
// list. It reports false for events outside the list.
func (a *assist) mouse(m *Model, msg tea.MouseMsg) bool {
	b, ok := a.box(m)
	if !ok {
		return false
	}
	mouse := msg.Mouse()
	h := compositor([]box{b}).Hit(mouse.X, mouse.Y)
	if h.Empty() {
		return false
	}
	list, start := a.shown(m)
	i := a.top + mouse.Y - b.y - 1
	switch msg.(type) {
	case tea.MouseWheelMsg:
		d := 1
		if mouse.Button == tea.MouseWheelUp {
			d = -1
		}
		a.sel = clamp(a.sel+d, 0, len(list)-1)
	case tea.MouseMotionMsg:
		if i >= a.top && i < min(len(list), a.top+b.height()-2) {
			a.sel = i
		}
	case tea.MouseClickMsg:
		if mouse.Button == tea.MouseLeft && i >= a.top && i < min(len(list), a.top+b.height()-2) {
			a.accept(m, list[i], start)
		}
	}
	return true
}

// status is the status line while the list shows: what the
// highlighted suggestion is, and the keys that apply.
func (a *assist) status(m *Model) (desc, keys string, ok bool) {
	list, _ := a.shown(m)
	if list == nil {
		return "", "", false
	}
	return list[a.sel].desc, m.th.KeyHints("Up/Down", "move", "Tab", "insert", "Esc", "hide"), true
}

// inFunction reports whether the caret is inside a known function's
// parentheses.
func (l *lineEdit) inFunction() bool {
	_, ok := sheet.LookupFunc(scanCaret(l.buf, l.pos).fn)
	return ok
}

// signatureLine puts the signature of the function around the caret on
// the context line, with what the function does and the keys that apply
// (hints) as room allows. It reports false outside a function.
func signatureLine(th *theme.Theme, width int, buf []rune, pos int, hints string) (left, right string, ok bool) {
	sig, desc := signature(th, buf, pos)
	if sig == "" {
		return "", "", false
	}
	full := sig + "   " + th.Muted.Render(desc)
	for _, try := range [][2]string{{full, hints}, {sig, hints}, {full, ""}} {
		if ansi.StringWidth(try[0])+3+ansi.StringWidth(try[1]) <= width {
			return try[0], try[1], true
		}
	}
	return sig, "", true
}

// signature renders the signature of the function around the caret, e.g.
// SUM(value1, [value2, ...]), with the current argument marked, and what
// the function does. It is "" outside a function.
func signature(th *theme.Theme, buf []rune, pos int) (sig, desc string) {
	c := scanCaret(buf, pos)
	f, ok := sheet.LookupFunc(c.fn)
	if c.fn == "" || !ok {
		return "", ""
	}
	parts := splitArgs(f.Args)
	cur := argPart(parts, c.arg, f.Max < 0)
	var b strings.Builder
	b.WriteString(th.Key.Render(f.Name) + "(")
	for i, p := range parts {
		if i > 0 {
			b.WriteString(", ")
		}
		if i == cur {
			b.WriteString(th.Argument.Render(p))
		} else {
			b.WriteString(p)
		}
	}
	b.WriteString(")")
	return b.String(), f.Desc
}
