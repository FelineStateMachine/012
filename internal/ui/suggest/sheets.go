package suggest

import (
	"slices"
	"strconv"
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
func sheetSuggestion(s *sheet.Sheet) Suggestion {
	detail, desc := "sheet, empty", "Sheet "+s.Name()+", empty"
	if used, ok := s.UsedRange(); ok {
		detail = "sheet, " + used.String()
		desc = "Sheet " + s.Name() + ": " + used.String() + ", " + cellCount(s.Len())
	}
	return Suggestion{Name: sheet.QuoteSheet(s.Name()) + "!", Detail: detail, Desc: desc}
}

// cellCount is n cells in words: "1 cell", "3 cells".
func cellCount(n int) string {
	if n == 1 {
		return "1 cell"
	}
	return strconv.Itoa(n) + " cells"
}

// otherSheets are the sheets offered in formulas typed on sh: the
// visible ones but sh, whose cells need no sheet name.
func otherSheets(sh *sheet.Sheet) []*sheet.Sheet {
	return slices.DeleteFunc(sh.Book().Visible(), func(s *sheet.Sheet) bool { return s == sh })
}

// sheetSuggestions lists the other visible sheets for w, a quoted name
// typed so far in upper case: those starting with it, then from two
// letters on, those containing it.
func sheetSuggestions(sh *sheet.Sheet, w string) []Suggestion {
	var prefix, inner []Suggestion
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
