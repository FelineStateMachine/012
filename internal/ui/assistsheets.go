package ui

import (
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
)

// Pointing after a sheet's name: once a suggestion (package suggest) or
// typing has put Summary! before the caret, an arrow points into that
// sheet, as clicking its tab would, so the reference comes out as
// Summary!B3.

// sheetBeforeCaret returns the sheet named just before the caret, as in
// "=SUM(Summary!" or "='Q3 plan'!", and where its name starts.
func sheetBeforeCaret(l *lineedit.Line, book *sheet.Workbook) (*sheet.Sheet, int, bool) {
	head := l.Buf[:l.Pos]
	end := len(head) - 1 // the "!"
	if end < 2 || head[end] != '!' {
		return nil, 0, false
	}
	start := end
	if head[end-1] == '\'' {
		start = -1
		for i := end - 2; i >= 1; i-- {
			if head[i] != '\'' {
				continue
			}
			if head[i-1] == '\'' {
				i-- // '' inside a quoted name
				continue
			}
			start = i
			break
		}
	} else {
		for start > 1 && isNameRune(head[start-1]) {
			start--
		}
	}
	if start < 1 || start >= end {
		return nil, 0, false
	}
	name, _ := sheet.SplitSheet(string(head[start:]))
	s := book.Lookup(name)
	return s, start, s != nil
}

func isNameRune(r rune) bool {
	return r == '_' || r == '.' || r >= '0' && r <= '9' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r > 0x7f
}

// pointAfterSheet starts pointing with key when the caret follows a
// sheet reference such as "Summary!": on that sheet, the name typed
// giving way to the reference pointed at (Summary!B3), as if its tab had
// been clicked. It reports whether it did.
func (m *Model) pointAfterSheet(key string) bool {
	if _, extend := extendKey(key); !extend && (!isMoveKey(key) || key == "tab" || key == "shift+tab") {
		return false
	}
	s, start, ok := sheetBeforeCaret(&m.line, m.book())
	if !ok || s.Hidden() {
		return false
	}
	if s == m.sheet && !m.away() {
		return m.startPoint(key) // its own sheet: the name stays as typed
	}
	m.line.Buf = append(m.line.Buf[:start:start], m.line.Buf[m.line.Pos:]...)
	m.line.Pos = start
	if s == m.sheet {
		return m.startPoint(key)
	}
	m.pointInto(s)
	if m.mode != modePoint {
		return true
	}
	if base, extend := extendKey(key); extend {
		m.point.anchor, m.point.anchored = m.point.at, true
		key = base
	}
	m.navigate(key, &m.point.at)
	return true
}
