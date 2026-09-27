package ui

import (
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// lineEdit is the one-line text editor behind every text field: the
// entry typed into a cell (ENTER, EDIT), prompts on the context line, and
// the search fields of the find bar and the pickers. Only one of them is
// edited at a time, so they share it; a component with several fields
// keeps the others' text itself and loads a field here to edit it.
type lineEdit struct {
	buf []rune
	pos int // the caret, an index into buf
}

// set replaces the text, with the caret at its end.
func (l *lineEdit) set(text string) {
	l.buf = []rune(text)
	l.pos = len(l.buf)
}

// clear empties the line.
func (l *lineEdit) clear() {
	l.buf, l.pos = nil, 0
}

// text is the whole line.
func (l *lineEdit) text() string { return string(l.buf) }

// head and tail are the text before and after the caret.
func (l *lineEdit) head() string { return string(l.buf[:l.pos]) }
func (l *lineEdit) tail() string { return string(l.buf[l.pos:]) }

// insert types text at the caret. Line breaks and tabs become spaces, and
// other control characters are dropped.
func (l *lineEdit) insert(text string) {
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if !unicode.IsPrint(r) {
			continue
		}
		l.buf = slices.Insert(l.buf, l.pos, r)
		l.pos++
	}
}

// key applies a line-editing key.
func (l *lineEdit) key(k tea.KeyPressMsg) {
	switch k.String() {
	case "left":
		l.pos = max(l.pos-1, 0)
	case "right":
		l.pos = min(l.pos+1, len(l.buf))
	case "home", "ctrl+a":
		l.pos = 0
	case "end", "ctrl+e":
		l.pos = len(l.buf)
	case "backspace":
		if l.pos > 0 {
			l.buf = slices.Delete(l.buf, l.pos-1, l.pos)
			l.pos--
		}
	case "delete":
		if l.pos < len(l.buf) {
			l.buf = slices.Delete(l.buf, l.pos, l.pos+1)
		}
	default:
		l.insert(typed(k))
	}
}

// setCaret puts the edit caret at display column x of the entry.
func (l *lineEdit) setCaret(x int) {
	w := 0
	for i, r := range l.buf {
		if w >= x {
			l.pos = i
			return
		}
		w += ansi.StringWidth(string(r))
	}
	l.pos = len(l.buf)
}

// isFormula reports whether the edit line holds a formula.
func (l *lineEdit) isFormula() bool {
	return len(l.buf) > 0 && (l.buf[0] == '=' || l.buf[0] == '+' || l.buf[0] == '-')
}

// canPoint reports whether the text before the caret ends where a cell
// reference may follow.
func (l *lineEdit) canPoint() bool {
	if l.pos == 0 {
		return false
	}
	return strings.ContainsRune("=+-*/^(,;<>&:", l.buf[l.pos-1])
}
