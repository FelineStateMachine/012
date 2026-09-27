package findbar

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// answerJSON is a replacement as a macro answers Find and replace:
//
//	{"find": "Rent", "replace": "Lease", "matchCase": true, "within": "all"}
//
// the query and the replacement, the options that are on, where to
// search (left out for the sheet shown, "all" for every sheet, or a
// range of the sheet shown), and "cell" to replace only the match in
// that cell rather than all of them.
type answerJSON struct {
	Find       string `json:"find"`
	Replace    string `json:"replace"`
	MatchCase  bool   `json:"matchCase,omitempty"`
	WholeCell  bool   `json:"wholeCell,omitempty"`
	Regex      bool   `json:"regex,omitempty"`
	InFormulas bool   `json:"inFormulas,omitempty"`
	Within     string `json:"within,omitempty"`
	Cell       string `json:"cell,omitempty"`
}

// answer is the bar's search and replacement as a macro records them,
// replacing only in cell when it isn't "".
func (f *Bar) answer(cell string) string {
	a := answerJSON{Find: f.fields[0], Replace: f.fields[1], MatchCase: f.opts.MatchCase, WholeCell: f.opts.WholeCell,
		Regex: f.opts.Regex, InFormulas: f.opts.InFormulas, Cell: cell}
	switch f.where {
	case inAll:
		a.Within = "all"
	case inRange:
		a.Within = f.scope.String()
	}
	raw, _ := json.Marshal(a)
	return string(raw)
}

// Answer makes the replacement a macro recorded (see answerJSON), as if
// typed and chosen in the bar.
func (f *Bar) Answer(text string) error {
	var a answerJSON
	if err := json.Unmarshal([]byte(text), &a); err != nil {
		return fmt.Errorf("the answer: %w", err)
	}
	if a.Find == "" {
		return errors.New(`the answer has nothing to find: give "find"`)
	}
	f.fields, f.replace, f.field = [2]string{a.Find, a.Replace}, true, 1
	f.h.Line().Set(a.Replace)
	f.opts = sheet.FindOptions{MatchCase: a.MatchCase, WholeCell: a.WholeCell, Regex: a.Regex, InFormulas: a.InFormulas}
	s, at := f.h.At()
	switch a.Within {
	case "":
		f.where = inSheet
	case "all":
		f.where = inAll
	default:
		r, ok := sheet.ParseRange(strings.ToUpper(a.Within))
		if !ok {
			return fmt.Errorf(`"within" is %q: "all", or a range such as B2:D9`, a.Within)
		}
		f.scope, f.home, f.where = &r, s, inRange
	}
	f.from = Match{s, at}
	if f.search(); f.err != "" {
		return errors.New(f.err)
	}
	if a.Cell == "" {
		f.replaceAll()
		return nil
	}
	c, ok := sheet.ParseAddr(strings.ToUpper(a.Cell))
	if !ok {
		return fmt.Errorf("%q isn't a cell", a.Cell)
	}
	i := slices.Index(f.matches, Match{s, c})
	if i < 0 {
		return fmt.Errorf("%s doesn't match %q", c, a.Find)
	}
	f.cur = i
	f.replaceOne()
	return nil
}
