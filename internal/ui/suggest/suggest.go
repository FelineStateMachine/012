// Package suggest is formula assistance, as in Sheets: while a function,
// range or sheet name is being typed, a list of matching functions,
// named ranges and other sheets drops down from the formula bar at the
// caret, and while the caret is inside a function's parentheses the
// context line shows its signature with the current argument marked.
// The list takes only the keys Sheets gives it (Up/Down, Tab, Enter,
// Esc) and only while it shows, so arrows after an operator still point
// at cells. It knows the UI only through Host.
package suggest

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/formula"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the suggestions need of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Line is the edit line the entry is typed in.
	Line() *lineedit.Line
	// Typing reports whether a cell's entry is being typed with nothing
	// open over it, when the list may show.
	Typing() bool
	// EntrySheet is the sheet the entry is on: its named ranges and the
	// other sheets are offered.
	EntrySheet() *sheet.Sheet
	// Stored is typed text as the formula parser reads it, whatever the
	// workbook's locale.
	Stored(buf []rune) []rune
	// TextX is the screen column the formula bar's text starts at.
	TextX() int
}

// List is the suggestion list's state; the zero List is hidden.
type List struct {
	// Active is set when the last key typed or deleted text; moving the
	// caret hides the list.
	Active   bool
	sel, top int // highlighted suggestion and first one shown
}

// Sel is the highlighted suggestion.
func (l *List) Sel() int { return l.sel }

// Rows is the most suggestions shown at once.
const Rows = 8

// ID identifies the list's box.
const ID = "assist"

// Suggestion is a function, named range or sheet offered for the word
// at the caret.
type Suggestion struct {
	Name   string // e.g. "SUM", "Sales" or "'Q3 plan'!"
	Detail string // the signature, the range a name stands for, or a sheet's cells
	Desc   string
	Fn     bool // a function: accepting it adds "("
}

// For lists the functions, named ranges and sheets for word, typed on
// sh: those starting with it first (names, then sheets, then
// functions), then, from two letters on, those containing it, such as
// COUNTIFS for "ifs". A word that is already a cell reference only gets
// those that start with it, and a word in quotes ('Q3) only sheets.
func For(sh *sheet.Sheet, word string) []Suggestion {
	if rest, quoted := strings.CutPrefix(word, "'"); quoted {
		return sheetSuggestions(sh, strings.ToUpper(rest))
	}
	w := strings.ToUpper(word)
	var prefix, inner []Suggestion
	add := func(s Suggestion, name string) {
		switch {
		case strings.HasPrefix(name, w):
			prefix = append(prefix, s)
		case len(w) >= 2 && strings.Contains(name, w):
			inner = append(inner, s)
		}
	}
	for _, n := range sh.Names() {
		add(Suggestion{Name: n.Name, Detail: n.Ref(), Desc: "Named range " + n.Name + ": " + n.Ref()}, strings.ToUpper(n.Name))
	}
	for _, s := range otherSheets(sh) {
		add(sheetSuggestion(s), strings.ToUpper(s.Name()))
	}
	for _, f := range sheet.Funcs() {
		add(Suggestion{Name: f.Name, Detail: f.Name + "(" + f.Args + ")", Desc: f.Desc, Fn: true}, f.Name)
	}
	if _, isRef := sheet.ParseAddr(w); isRef {
		return prefix
	}
	return append(prefix, inner...)
}

// Shown returns the list for the word at the caret, and where the word
// starts, or nothing when the list is hidden: outside an entry, in
// plain text, after the caret moved, or when the word already names a
// range exactly and nothing else matches.
func (l *List) Shown(h Host) ([]Suggestion, int) {
	line := h.Line()
	if !h.Typing() || !line.IsFormula() || !l.Active {
		return nil, 0
	}
	c := formula.ScanCaret(h.Stored(line.Buf), line.Pos)
	if c.Word == "" {
		return nil, 0
	}
	list := For(h.EntrySheet(), c.Word)
	if len(list) == 0 || len(list) == 1 && !list[0].Fn && strings.EqualFold(list[0].Name, c.Word) {
		return nil, 0
	}
	l.sel = min(l.sel, len(list)-1)
	return list, c.WordStart
}

// Key handles the keys the list takes while it shows, and reports
// whether it took key.
func (l *List) Key(h Host, key string) bool {
	list, start := l.Shown(h)
	if list == nil {
		return false
	}
	switch key {
	case "up", "ctrl+p":
		l.sel = (l.sel - 1 + len(list)) % len(list)
	case "down", "ctrl+n":
		l.sel = (l.sel + 1) % len(list)
	case "tab", "enter":
		l.accept(h, list[l.sel], start)
	case "esc":
		l.Active = false
	default:
		return false
	}
	return true
}

// accept replaces the word at the caret with s, adding "(" after a
// function unless one is already there.
func (l *List) accept(h Host, s Suggestion, start int) {
	line := h.Line()
	text := []rune(s.Name)
	rest := line.Buf[line.Pos:]
	if s.Fn && (len(rest) == 0 || rest[0] != '(') {
		text = append(text, '(')
	}
	if strings.HasSuffix(s.Name, "!") {
		rest = trimSheetEnd(rest) // the rest of a sheet name typed before
	}
	pos := start + len(text)
	if s.Fn && len(rest) > 0 && rest[0] == '(' {
		pos++ // into the existing parentheses
	}
	line.Buf = slices.Concat(line.Buf[:start], text, rest)
	line.Pos = pos
	l.Active = false
}

// Box draws the list under the formula bar, its text aligned with the
// word being typed: the name, then the signature or range dimmed.
func (l *List) Box(h Host) (overlay.Box, bool) {
	list, start := l.Shown(h)
	if list == nil {
		return overlay.Box{}, false
	}
	th := h.Theme()
	width, height := h.Size()
	rows := min(len(list), Rows, height-overlay.GridTop-2)
	if rows < 1 {
		return overlay.Box{}, false
	}
	l.top = min(max(l.top, l.sel-rows+1), l.sel)
	l.top = min(max(l.top, 0), len(list)-rows)
	nw, dw := 0, 0
	for _, s := range list {
		nw = max(nw, ansi.StringWidth(s.Name))
		dw = max(dw, ansi.StringWidth(s.Detail))
	}
	inner := min(1+nw+3+dw+1, 64, width-2)
	lines := make([]string, 0, rows)
	for i := l.top; i < l.top+rows; i++ {
		s := list[i]
		base, dim := th.MenuBar, th.Muted
		if i == l.sel {
			base, dim = th.MenuSelected, th.MenuSelected
		}
		detail := s.Detail
		if s.Fn {
			detail = s.Detail[len(s.Name):] // just the arguments: (value1, ...)
		}
		row := base.Render(" "+theme.PadRight(s.Name, nw)+"   ") + dim.Render(detail)
		lines = append(lines, ansi.Truncate(row, inner, "…")+base.Render(strings.Repeat(" ", max(inner-ansi.StringWidth(row), 0))))
	}
	footer := ""
	if len(list) > rows {
		footer = strconv.Itoa(l.sel+1) + " of " + strconv.Itoa(len(list))
	}
	// The box's text starts one column in, under the word's first letter.
	x := h.TextX() + ansi.StringWidth(string(h.Line().Buf[:start])) - 2
	x = min(max(x, 0), max(width-inner-2, 0))
	return overlay.Box{ID: ID, X: x, Y: overlay.ContextLine + 1, Lines: th.Frame(inner, "", footer, lines)}, true
}

// Mouse lets the mouse hover and click suggestions and scroll the list.
// It reports false for events outside the list.
func (l *List) Mouse(h Host, msg tea.MouseMsg) bool {
	b, ok := l.Box(h)
	if !ok {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < b.X || mouse.X >= b.X+b.Width() || mouse.Y < b.Y || mouse.Y >= b.Y+b.Height() {
		return false
	}
	list, start := l.Shown(h)
	i := l.top + mouse.Y - b.Y - 1
	onRow := i >= l.top && i < min(len(list), l.top+b.Height()-2)
	switch msg.(type) {
	case tea.MouseWheelMsg:
		d := 1
		if mouse.Button == tea.MouseWheelUp {
			d = -1
		}
		l.sel = min(max(l.sel+d, 0), len(list)-1)
	case tea.MouseMotionMsg:
		if onRow {
			l.sel = i
		}
	case tea.MouseClickMsg:
		if mouse.Button == tea.MouseLeft && onRow {
			l.accept(h, list[i], start)
		}
	}
	return true
}

// Status is the status line while the list shows: what the highlighted
// suggestion is, and the keys that apply.
func (l *List) Status(h Host) (desc, keys string, ok bool) {
	list, _ := l.Shown(h)
	if list == nil {
		return "", "", false
	}
	return list[l.sel].Desc, h.Theme().KeyHints("Up/Down", "move", "Tab", "insert", "Esc", "hide"), true
}

// SignatureLine puts the signature of the function around the caret on
// the context line, with what the function does and the keys that apply
// (hints) as room allows. It reports false outside a function.
func SignatureLine(th *theme.Theme, width int, buf []rune, pos int, sep byte, hints string) (left, right string, ok bool) {
	sig, desc := Signature(th, buf, pos, sep)
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

// Signature renders the signature of the function around the caret, e.g.
// SUM(value1, [value2, ...]), with the current argument marked, and what
// the function does. It is "" outside a function. buf is the formula in
// the syntax it's parsed in (see Host.Stored); sep separates the
// arguments shown, ; in a locale with a decimal comma.
func Signature(th *theme.Theme, buf []rune, pos int, sep byte) (sig, desc string) {
	c := formula.ScanCaret(buf, pos)
	f, ok := sheet.LookupFunc(c.Fn)
	if c.Fn == "" || !ok {
		return "", ""
	}
	parts := formula.SplitArgs(f.Args)
	cur := formula.ArgPart(parts, c.Arg, f.Max < 0)
	var b strings.Builder
	b.WriteString(th.Key.Render(f.Name) + "(")
	for i, p := range parts {
		if i > 0 {
			b.WriteString(string(sep) + " ")
		}
		if sep != ',' {
			p = strings.ReplaceAll(p, ",", string(sep)) // [value2; ...]
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
