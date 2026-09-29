package lineedit

import (
	"slices"
	"unicode"
)

// Word keys, by the names terminals send them under. Option+arrows on
// macOS arrive as alt+b and alt+f in most terminals, Ctrl+Backspace as
// ctrl+h without the kitty keyboard protocol, and Cmd+Backspace as
// ctrl+u.
var (
	wordLeftKeys  = []string{"ctrl+left", "alt+left", "alt+b"}
	wordRightKeys = []string{"ctrl+right", "alt+right", "alt+f"}
	wordBackKeys  = []string{"ctrl+backspace", "alt+backspace", "ctrl+h", "ctrl+w"}
	wordDelKeys   = []string{"ctrl+delete", "alt+delete", "alt+d"}
	lineBackKeys  = []string{"ctrl+u", "super+backspace"}
)

// IsWordKey reports whether key moves or deletes by word or to the start
// of the line, so a field can pass it on to Key.
func IsWordKey(key string) bool {
	for _, keys := range [][]string{wordLeftKeys, wordRightKeys, wordBackKeys, wordDelKeys, lineBackKeys} {
		if slices.Contains(keys, key) {
			return true
		}
	}
	return false
}

// wordKey applies a word key and reports whether key was one.
func (l *Line) wordKey(key string) bool {
	switch {
	case slices.Contains(wordLeftKeys, key):
		l.Pos = l.wordStart(l.Pos)
	case slices.Contains(wordRightKeys, key):
		l.Pos = l.wordEnd(l.Pos)
	case slices.Contains(wordBackKeys, key):
		l.DeleteBack(l.wordStart(l.Pos))
	case slices.Contains(wordDelKeys, key):
		l.Buf = slices.Delete(l.Buf, l.Pos, l.wordEnd(l.Pos))
	case slices.Contains(lineBackKeys, key):
		l.DeleteBack(0)
	default:
		return false
	}
	return true
}

// DeleteBack deletes from index from up to the caret.
func (l *Line) DeleteBack(from int) {
	l.Buf = slices.Delete(l.Buf, from, l.Pos)
	l.Pos = from
}

// A word is a run of letters, digits and underscores, or a run of other
// symbols, so in a formula Ctrl+Backspace takes B2 from SUM(A1:B2 and
// then the colon. Spaces and line breaks around a word go with it.
func wordClass(r rune) int {
	switch {
	case unicode.IsSpace(r):
		return 0
	case r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r):
		return 1
	}
	return 2
}

// wordStart is the start of the word before i, skipping spaces first.
func (l *Line) wordStart(i int) int {
	for i > 0 && wordClass(l.Buf[i-1]) == 0 {
		i--
	}
	if i == 0 {
		return 0
	}
	c := wordClass(l.Buf[i-1])
	for i > 0 && wordClass(l.Buf[i-1]) == c {
		i--
	}
	return i
}

// wordEnd is the end of the word after i, skipping spaces first.
func (l *Line) wordEnd(i int) int {
	n := len(l.Buf)
	for i < n && wordClass(l.Buf[i]) == 0 {
		i++
	}
	if i == n {
		return n
	}
	c := wordClass(l.Buf[i])
	for i < n && wordClass(l.Buf[i]) == c {
		i++
	}
	return i
}
