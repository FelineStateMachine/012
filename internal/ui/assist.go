package ui

import (
	"slices"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"012/internal/sheet"
)

// Formula assistance, as in Sheets: while a function or range name is
// being typed, a list of matching functions and named ranges drops down
// from the formula bar at the caret, and while the caret is inside a
// function's parentheses the context line shows its signature with the
// current argument marked. The list takes only the keys Sheets gives it
// (Up/Down, Tab, Enter, Esc) and only while it shows, so arrows after an
// operator still point at cells.

// assist is the state of the suggestion list.
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

// caret describes the formula text before the caret: the word being
// typed, if it may be a function or name, and the innermost function call
// the caret is in.
type caret struct {
	word      string
	wordStart int
	fn        string // e.g. "SUM", or "" outside any function's parentheses
	arg       int    // index of the argument the caret is in
}

func isWordRune(r rune) bool {
	return r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) || r == '_' || r == '.' || r == '$' || r == '@'
}

// scanCaret reads a formula up to pos the way the lexer does: strings are
// skipped, parentheses nest, and commas or semicolons at the innermost
// level separate arguments.
func scanCaret(buf []rune, pos int) caret {
	type frame struct {
		fn  string
		arg int
	}
	var stack []frame
	var c caret
	inStr := false
	start := -1                // start of the word being read
	prev := ""                 // the word just before a space or "(" (SUM ( is allowed)
	for i := 1; i < pos; i++ { // buf[0] is "=", "+" or "-"
		r := buf[i]
		if inStr {
			inStr = r != '"'
			continue
		}
		if isWordRune(r) {
			if start < 0 {
				start = i
			}
			continue
		}
		if start >= 0 {
			prev, start = string(buf[start:i]), -1
		}
		switch r {
		case ' ':
			continue // keep prev for "SUM ("
		case '"':
			inStr = true
		case '(':
			stack = append(stack, frame{fn: strings.ToUpper(strings.TrimPrefix(prev, "@"))})
		case ')':
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case ',', ';':
			if len(stack) > 0 {
				stack[len(stack)-1].arg++
			}
		}
		prev = ""
	}
	if inStr {
		return caret{}
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].fn != "" {
			c.fn, c.arg = stack[i].fn, stack[i].arg
			break
		}
	}
	if start >= 0 && (pos == len(buf) || !isWordRune(buf[pos])) {
		w := []rune(strings.TrimPrefix(string(buf[start:pos]), "@"))
		if len(w) > 0 && (unicode.IsLetter(w[0]) || w[0] == '_') {
			c.word, c.wordStart = string(w), pos-len(w)
		}
	}
	return c
}

// suggestions lists the functions and named ranges for word: those
// starting with it first (names before functions), then, from two
// letters on, those containing it, such as COUNTIFS for "ifs". A word
// that is already a cell reference only gets names and functions that
// start with it.
func (m *Model) suggestions(word string) []suggestion {
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
	for _, n := range m.sheet.Names() {
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

// shownSuggestions returns the list for the word at the caret, and where
// the word starts, or nothing when the list is hidden: outside ENTER and
// EDIT, in plain text, after the caret moved, or when the word already
// names a range exactly and nothing else matches.
func (m *Model) shownSuggestions() ([]suggestion, int) {
	if (m.mode != modeEnter && m.mode != modeEdit) || !m.isFormula() || !m.assist.active || m.overlay != nil {
		return nil, 0
	}
	c := scanCaret(m.buf, m.bufPos)
	if c.word == "" {
		return nil, 0
	}
	list := m.suggestions(c.word)
	if len(list) == 0 || len(list) == 1 && !list[0].fn && strings.EqualFold(list[0].name, c.word) {
		return nil, 0
	}
	m.assist.sel = min(m.assist.sel, len(list)-1)
	return list, c.wordStart
}

// typeKey applies a line-editing key to the entry. Typing or deleting
// shows suggestions; moving the caret hides them, as in Sheets.
func (m *Model) typeKey(k tea.KeyPressMsg) {
	before := string(m.buf)
	m.lineKey(k)
	m.assist = assist{active: string(m.buf) != before}
}

// assistKey handles the keys the suggestion list takes while it shows.
func (m *Model) assistKey(key string) bool {
	list, start := m.shownSuggestions()
	if list == nil {
		return false
	}
	switch key {
	case "up", "ctrl+p":
		m.assist.sel = (m.assist.sel - 1 + len(list)) % len(list)
	case "down", "ctrl+n":
		m.assist.sel = (m.assist.sel + 1) % len(list)
	case "tab", "enter":
		m.acceptSuggestion(list[m.assist.sel], start)
	case "esc":
		m.assist.active = false
	default:
		return false
	}
	return true
}

// acceptSuggestion replaces the word at the caret with s, adding "(" after
// a function unless one is already there.
func (m *Model) acceptSuggestion(s suggestion, start int) {
	text := []rune(s.name)
	rest := m.buf[m.bufPos:]
	if s.fn && (len(rest) == 0 || rest[0] != '(') {
		text = append(text, '(')
	}
	pos := start + len(text)
	if s.fn && len(rest) > 0 && rest[0] == '(' {
		pos++ // into the existing parentheses
	}
	m.buf = slices.Concat(m.buf[:start], text, rest)
	m.bufPos = pos
	m.assist.active = false
}

// assistBox draws the list under the formula bar, its text aligned with
// the word being typed: the name, then the signature or range dimmed.
func (m *Model) assistBox() (box, bool) {
	list, start := m.shownSuggestions()
	if list == nil {
		return box{}, false
	}
	rows := min(len(list), assistRows, m.height-gridTop-2)
	if rows < 1 {
		return box{}, false
	}
	a := &m.assist
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
		base, dim := m.th.menuBar, m.th.muted
		if i == a.sel {
			base, dim = m.th.menuSelected, m.th.menuSelected
		}
		detail := s.detail
		if s.fn {
			detail = s.detail[len(s.name):] // just the arguments: (value1, ...)
		}
		row := base.Render(" "+padRight(s.name, nw)+"   ") + dim.Render(detail)
		lines = append(lines, ansi.Truncate(row, inner, "…")+base.Render(strings.Repeat(" ", max(inner-ansi.StringWidth(row), 0))))
	}
	footer := ""
	if len(list) > rows {
		footer = strconv.Itoa(a.sel+1) + " of " + strconv.Itoa(len(list))
	}
	// The box's text starts one column in, under the word's first letter.
	x := formulaBarTextX() + ansi.StringWidth(string(m.buf[:start])) - 2
	x = clamp(x, 0, max(m.width-inner-2, 0))
	return box{id: assistID, x: x, y: contextLine + 1, lines: m.frame(inner, "", footer, lines)}, true
}

// assistMouse lets the mouse hover and click suggestions and scroll the
// list. It reports false for events outside the list.
func (m *Model) assistMouse(msg tea.MouseMsg) bool {
	b, ok := m.assistBox()
	if !ok {
		return false
	}
	mouse := msg.Mouse()
	h := compositor([]box{b}).Hit(mouse.X, mouse.Y)
	if h.Empty() {
		return false
	}
	list, start := m.shownSuggestions()
	i := m.assist.top + mouse.Y - b.y - 1
	switch msg.(type) {
	case tea.MouseWheelMsg:
		d := 1
		if mouse.Button == tea.MouseWheelUp {
			d = -1
		}
		m.assist.sel = clamp(m.assist.sel+d, 0, len(list)-1)
	case tea.MouseMotionMsg:
		if i >= m.assist.top && i < min(len(list), m.assist.top+b.height()-2) {
			m.assist.sel = i
		}
	case tea.MouseClickMsg:
		if mouse.Button == tea.MouseLeft && i >= m.assist.top && i < min(len(list), m.assist.top+b.height()-2) {
			m.acceptSuggestion(list[i], start)
		}
	}
	return true
}

// assistStatus is the status line while the list shows: what the
// highlighted suggestion is, and the keys that apply.
func (m *Model) assistStatus() (desc, keys string, ok bool) {
	list, _ := m.shownSuggestions()
	if list == nil {
		return "", "", false
	}
	return list[m.assist.sel].desc, m.keyHints("Up/Down", "move", "Tab", "insert", "Esc", "hide"), true
}

// inFunction reports whether the caret is inside a known function's
// parentheses.
func (m *Model) inFunction() bool {
	_, ok := sheet.LookupFunc(scanCaret(m.buf, m.bufPos).fn)
	return ok
}

// signatureLine puts the signature of the function around the caret on
// the context line, with what the function does and the keys that apply
// (hints) as room allows. It reports false outside a function.
func (m *Model) signatureLine(buf []rune, pos int, hints string) (left, right string, ok bool) {
	sig, desc := m.signature(buf, pos)
	if sig == "" {
		return "", "", false
	}
	full := sig + "   " + m.th.muted.Render(desc)
	for _, try := range [][2]string{{full, hints}, {sig, hints}, {full, ""}} {
		if ansi.StringWidth(try[0])+3+ansi.StringWidth(try[1]) <= m.width {
			return try[0], try[1], true
		}
	}
	return sig, "", true
}

// signature renders the signature of the function around the caret, e.g.
// SUM(value1, [value2, ...]), with the current argument marked, and what
// the function does. It is "" outside a function.
func (m *Model) signature(buf []rune, pos int) (sig, desc string) {
	c := scanCaret(buf, pos)
	f, ok := sheet.LookupFunc(c.fn)
	if c.fn == "" || !ok {
		return "", ""
	}
	parts := splitArgs(f.Args)
	cur := argPart(parts, c.arg, f.Max < 0)
	var b strings.Builder
	b.WriteString(m.th.key.Render(f.Name) + "(")
	for i, p := range parts {
		if i > 0 {
			b.WriteString(", ")
		}
		if i == cur {
			b.WriteString(m.th.argument.Render(p))
		} else {
			b.WriteString(p)
		}
	}
	b.WriteString(")")
	return b.String(), f.Desc
}

// splitArgs splits a signature's arguments at the top-level commas, so
// "[value2, ...]" stays one part.
func splitArgs(args string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range args {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(args[start:i]))
				start = i + 1
			}
		}
	}
	if s := strings.TrimSpace(args[start:]); s != "" {
		out = append(out, s)
	}
	return out
}

// argPart maps an argument index to the part of the signature that
// describes it: a repeating part like "[value2, ...]" covers every
// argument from its position on. It returns -1 past the last argument.
func argPart(parts []string, arg int, variadic bool) int {
	rep := slices.IndexFunc(parts, func(p string) bool { return strings.Contains(p, "...") })
	switch {
	case rep >= 0 && variadic && arg >= rep:
		return rep
	case arg < len(parts):
		return arg
	}
	return -1
}
