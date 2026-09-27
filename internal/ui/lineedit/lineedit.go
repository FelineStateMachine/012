// Package lineedit is the one-line text editor behind every text field of
// the UI. It depends on nothing in package ui, so components in their own
// packages can edit the shared line.
package lineedit

import (
	"slices"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// Line is the one-line text editor behind every text field: the
// entry typed into a cell (ENTER, EDIT), prompts on the context line, and
// the search fields of the find bar and the pickers. Only one of them is
// edited at a time, so they share it; a component with several fields
// keeps the others' text itself and loads a field here to edit it.
type Line struct {
	Buf []rune
	Pos int // the caret, an index into Buf
}

// Set replaces the text, with the caret at its end.
func (l *Line) Set(text string) {
	l.Buf = []rune(text)
	l.Pos = len(l.Buf)
}

// Clear empties the line.
func (l *Line) Clear() {
	l.Buf, l.Pos = nil, 0
}

// Text is the whole line.
func (l *Line) Text() string { return string(l.Buf) }

// Head and tail are the text before and after the caret.
func (l *Line) Head() string { return string(l.Buf[:l.Pos]) }
func (l *Line) Tail() string { return string(l.Buf[l.Pos:]) }

// Insert types text at the caret. Line breaks and tabs become spaces, and
// other control characters are dropped.
func (l *Line) Insert(text string) {
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if !unicode.IsPrint(r) {
			continue
		}
		l.Buf = slices.Insert(l.Buf, l.Pos, r)
		l.Pos++
	}
}

// Key applies a line-editing Key.
func (l *Line) Key(k tea.KeyPressMsg) {
	switch k.String() {
	case "left":
		l.Pos = max(l.Pos-1, 0)
	case "right":
		l.Pos = min(l.Pos+1, len(l.Buf))
	case "home", "ctrl+a":
		l.Pos = 0
	case "end", "ctrl+e":
		l.Pos = len(l.Buf)
	case "backspace":
		if l.Pos > 0 {
			l.Buf = slices.Delete(l.Buf, l.Pos-1, l.Pos)
			l.Pos--
		}
	case "delete":
		if l.Pos < len(l.Buf) {
			l.Buf = slices.Delete(l.Buf, l.Pos, l.Pos+1)
		}
	default:
		l.Insert(Typed(k))
	}
}

// SetCaret puts the edit caret at display column x of the entry.
func (l *Line) SetCaret(x int) {
	w := 0
	for i, r := range l.Buf {
		if w >= x {
			l.Pos = i
			return
		}
		w += ansi.StringWidth(string(r))
	}
	l.Pos = len(l.Buf)
}

// IsFormula reports whether the edit line holds a formula.
func (l *Line) IsFormula() bool {
	return len(l.Buf) > 0 && (l.Buf[0] == '=' || l.Buf[0] == '+' || l.Buf[0] == '-')
}

// CanPoint reports whether the text before the caret ends where a cell
// reference may follow.
func (l *Line) CanPoint() bool {
	if l.Pos == 0 {
		return false
	}
	return strings.ContainsRune("=+-*/^(,;<>&:", l.Buf[l.Pos-1])
}

// Typed returns the printable text of a key press, if any.
func Typed(k tea.KeyPressMsg) string {
	if k.Mod.Contains(tea.ModCtrl) || k.Mod.Contains(tea.ModAlt) {
		return ""
	}
	return k.Text
}
