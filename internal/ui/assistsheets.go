package ui

import (
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Sheet names in formula suggestions: typing =Su offers Summary! with the
// functions and named ranges, and ='Q3 offers 'Q3 plan'!, quoted as a
// reference needs; the sheet the formula is on isn't, as its cells need
// no name, and neither are hidden sheets. Accepting one inserts the sheet
// and its "!"; an arrow then points into that sheet, as clicking its tab
// would, so the reference comes out as Summary!B3.

// sheetSuggestion is the suggestion for sheet s.
func sheetSuggestion(s *sheet.Sheet) suggestion {
	detail, desc := "sheet, empty", "Sheet "+s.Name()+", empty"
	if used, ok := s.UsedRange(); ok {
		detail = "sheet, " + used.String()
		desc = "Sheet " + s.Name() + ": " + used.String() + ", " + cellCount(s.Len())
	}
	return suggestion{name: sheet.QuoteSheet(s.Name()) + "!", detail: detail, desc: desc}
}

// otherSheets are the sheets offered in formulas typed on sh: the
// visible ones but sh, whose cells need no sheet name.
func otherSheets(sh *sheet.Sheet) []*sheet.Sheet {
	return slices.DeleteFunc(sh.Book().Visible(), func(s *sheet.Sheet) bool { return s == sh })
}

// sheetSuggestions lists the other visible sheets for w, a quoted name typed
// so far in upper case: those starting with it, then from two letters
// on, those containing it.
func sheetSuggestions(sh *sheet.Sheet, w string) []suggestion {
	var prefix, inner []suggestion
	for _, s := range otherSheets(sh) {
		name := strings.ToUpper(s.Name())
		switch {
		case strings.HasPrefix(name, w):
			prefix = append(prefix, sheetSuggestion(s))
		case len(w) >= 2 && strings.Contains(name, w):
			inner = append(inner, sheetSuggestion(s))
		}
	}
	return append(prefix, inner...)
}

// trimSheetEnd drops the end of a sheet reference ("!" or "'!") that
// follows the caret, which an accepted sheet brings with it.
func trimSheetEnd(rest []rune) []rune {
	switch {
	case len(rest) > 0 && rest[0] == '!':
		return rest[1:]
	case len(rest) > 1 && rest[0] == '\'' && rest[1] == '!':
		return rest[2:]
	}
	return rest
}

// sheetBeforeCaret returns the sheet named just before the caret, as in
// "=SUM(Summary!" or "='Q3 plan'!", and where its name starts.
func (l *lineEdit) sheetBeforeCaret(book *sheet.Workbook) (*sheet.Sheet, int, bool) {
	head := l.buf[:l.pos]
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
	s, start, ok := m.line.sheetBeforeCaret(m.book())
	if !ok || s.Hidden() {
		return false
	}
	if s == m.sheet && !m.away() {
		return m.startPoint(key) // its own sheet: the name stays as typed
	}
	m.line.buf = append(m.line.buf[:start:start], m.line.buf[m.line.pos:]...)
	m.line.pos = start
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
