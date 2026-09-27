package fileio

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Reading frozen panes and filters; see xlsxview.go for how they are
// written.

// readPane reads a <pane> of the sheet's first view: frozen panes
// (state frozen or frozenSplit) freeze xSplit columns and ySplit rows.
// A split that isn't frozen is left out.
func (r *xlsxSheetReader) readPane(se xml.StartElement) {
	if r.paneRead {
		return
	}
	r.paneRead = true
	if st, _ := attr(se, "state"); st != "frozen" && st != "frozenSplit" {
		return
	}
	split := func(name string) int {
		f, err := strconv.ParseFloat(attrOr(se, name, "0"), 64)
		if err != nil || !(f > 0) {
			return 0
		}
		return int(min(f, sheet.MaxFrozen))
	}
	r.frozenCols, r.frozenRows = split("xSplit"), split("ySplit")
}

// xlsxAutoFilter is a worksheet's <autoFilter>: its range and the
// criteria of its columns.
type xlsxAutoFilter struct {
	ref  string
	cols []xlsxFilterColumn
}

// xlsxFilterColumn is a <filterColumn>.
type xlsxFilterColumn struct {
	col     int      // colId, from the range's first column
	filters bool     // a <filters> element: the values shown
	values  []string // its <filter val>s
	blank   bool     // blanks are shown
	dates   bool     // it has <dateGroupItem>s
	custom  []xlsxCustomFilter
	other   string // a kind of criteria 012 has no equivalent for
}

// xlsxCustomFilter is a <customFilter>: an operator and a value.
type xlsxCustomFilter struct{ op, val string }

// maxFilterValues caps the values one filter column lists.
const maxFilterValues = excelRows

// readTail reads what follows the rows, returning the sheet's
// autoFilter or nil, and keeping its rules in the reader.
func (r *xlsxSheetReader) readTail() (*xlsxAutoFilter, error) {
	var af *xlsxAutoFilter
	for {
		t, err := r.x.next()
		if err == io.EOF {
			return af, nil
		}
		if err != nil {
			return nil, err
		}
		se, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		rule, err := r.readRuleElement(se)
		switch {
		case err != nil:
			return nil, err
		case rule || r.x.depth != 2:
		case se.Name.Local == "sheetProtection":
			v, _ := attr(se, "sheet")
			r.protected = v == "1" || v == "true"
		case se.Name.Local == "autoFilter" && af == nil:
			if af, err = r.readAutoFilter(se); err != nil {
				return nil, err
			}
		}
	}
}

// readAutoFilter reads an <autoFilter>, just started.
func (r *xlsxSheetReader) readAutoFilter(se xml.StartElement) (*xlsxAutoFilter, error) {
	af := &xlsxAutoFilter{}
	af.ref, _ = attr(se, "ref")
	for depth := r.x.depth; ; {
		t, err := r.x.next()
		if err != nil {
			return nil, eofAsUnexpected(err)
		}
		if _, ok := t.(xml.EndElement); ok && r.x.depth < depth {
			return af, nil
		}
		se, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		if se.Name.Local == "filterColumn" && r.x.depth == depth+1 {
			af.cols = append(af.cols, xlsxFilterColumn{col: intAttr(se, "colId", -1)})
		} else if n := len(af.cols); n > 0 {
			af.cols[n-1].add(se)
		}
	}
}

// add reads an element inside the column's <filterColumn>.
func (fc *xlsxFilterColumn) add(se xml.StartElement) {
	switch se.Name.Local {
	case "filters":
		fc.filters, fc.blank = true, boolAttr(se, "blank", false)
	case "filter":
		if len(fc.values) < maxFilterValues {
			fc.values = append(fc.values, attrOr(se, "val", ""))
		}
	case "dateGroupItem":
		fc.dates = true
	case "customFilter":
		fc.custom = append(fc.custom, xlsxCustomFilter{op: attrOr(se, "operator", "equal"), val: attrOr(se, "val", "")})
	case "top10":
		fc.other = "top 10"
	case "dynamicFilter":
		fc.other = "a dynamic filter"
	case "colorFilter":
		fc.other = "by color"
	case "iconFilter":
		fc.other = "by icon"
	}
}

// applyFilters puts each sheet's autoFilter (filters[i] on sheet i) on
// book, whose values are computed, and returns the notes on what was
// left out.
func applyFilters(book *sheet.Workbook, filters []*xlsxAutoFilter) []string {
	var fn filterNotes
	for i, af := range filters {
		if af != nil {
			fn.apply(book.Sheet(i), af)
		}
	}
	return fn.notes()
}

// filterNotes counts the filter criteria left out across a workbook,
// keeping the first as an example.
type filterNotes struct {
	n       int
	example string
}

// apply puts filter af on s, whose values are computed: a filter over
// its range, with the criteria of each column that 012 can apply.
func (fn *filterNotes) apply(s *sheet.Sheet, af *xlsxAutoFilter) {
	r, ok := sheet.ParseRange(strings.ReplaceAll(af.ref, "$", ""))
	if !ok {
		fn.leftOut(s, "", "its range "+strconv.Quote(af.ref))
		return
	}
	s.LoadFilter(&sheet.Filter{Range: r}) // no criteria yet, so every column lists all its values
	f := &sheet.Filter{Range: r, Cols: map[int]sheet.Criteria{}}
	for _, fc := range af.cols {
		c := r.From.Col + fc.col
		if fc.col < 0 || c > r.To.Col {
			continue
		}
		cr, why := fc.criteria(s, c)
		if why != "" {
			fn.leftOut(s, sheet.ColName(c), why)
			continue
		}
		if !cr.IsZero() {
			f.Cols[c] = cr
		}
	}
	s.LoadFilter(f)
}

func (fn *filterNotes) leftOut(s *sheet.Sheet, col, why string) {
	fn.n++
	if fn.example == "" {
		where := "on " + sheet.QuoteSheet(s.Name())
		if col != "" {
			where = "column " + col + " " + where
		}
		fn.example = where + " (" + why + ")"
	}
}

// notes is what the notes say of the criteria left out.
func (fn *filterNotes) notes() []string {
	if fn.n == 0 {
		return nil
	}
	return []string{fmt.Sprintf("%s of a filter 012 can't apply left out, e.g. %s",
		count(fn.n, "criterion", "criteria"), fn.example)}
}

// criteria is the column's criteria in 012's terms, for column c of s,
// or why they can't be.
func (fc *xlsxFilterColumn) criteria(s *sheet.Sheet, c int) (sheet.Criteria, string) {
	switch {
	case fc.other != "":
		return sheet.Criteria{}, fc.other
	case fc.dates:
		return sheet.Criteria{}, "dates by year or month"
	case len(fc.custom) > 1:
		return sheet.Criteria{}, "two conditions"
	case len(fc.custom) == 1:
		cond, ok := conditionOf(fc.custom[0])
		if !ok {
			return sheet.Criteria{}, "the condition " + fc.custom[0].op + " " + strconv.Quote(fc.custom[0].val)
		}
		return sheet.Criteria{Cond: cond}, ""
	case !fc.filters:
		return sheet.Criteria{}, ""
	case len(fc.values) == 0 && fc.blank:
		return sheet.Criteria{Cond: sheet.Condition{Op: sheet.CondEmpty}}, ""
	}
	// Excel lists the values shown, matching them as displayed and
	// ignoring case; 012 keeps those hidden.
	shown := make(map[string]bool, len(fc.values))
	for _, v := range fc.values {
		shown[strings.ToLower(v)] = true
	}
	var cr sheet.Criteria
	for _, v := range s.FilterValues(c) {
		if v.Text == "" && !fc.blank || v.Text != "" && !shown[strings.ToLower(v.Text)] {
			cr.Hidden = append(cr.Hidden, v.Text)
		}
	}
	return cr, ""
}

// excelConds are 012's comparisons by custom filter operator.
var excelConds = map[string]sheet.CondOp{
	"greaterThan": sheet.CondGreater, "greaterThanOrEqual": sheet.CondGreaterEq,
	"lessThan": sheet.CondLess, "lessThanOrEqual": sheet.CondLessEq,
}

// conditionOf is a custom filter as a condition: comparisons as they
// are; equal and notEqual with wildcards only at the ends as contains,
// starts with and ends with; notEqual to a space as not empty.
func conditionOf(cf xlsxCustomFilter) (sheet.Condition, bool) {
	if op, ok := excelConds[cf.op]; ok {
		return sheet.Condition{Op: op, Arg: cf.val}, true
	}
	lead, core, trail, ok := wildcardShape(cf.val)
	if !ok {
		return sheet.Condition{}, false
	}
	switch op := cf.op; {
	case op == "notEqual" && cf.val == " ":
		return sheet.Condition{Op: sheet.CondNotEmpty}, true
	case op == "equal" && lead && trail:
		return sheet.Condition{Op: sheet.CondContains, Arg: core}, true
	case op == "equal" && lead:
		return sheet.Condition{Op: sheet.CondEndsWith, Arg: core}, true
	case op == "equal" && trail:
		return sheet.Condition{Op: sheet.CondStartsWith, Arg: core}, true
	case op == "equal":
		if _, _, isNum := sheet.ParseValue(strings.TrimSpace(core)); isNum {
			return sheet.Condition{Op: sheet.CondEqual, Arg: core}, true
		}
		return sheet.Condition{Op: sheet.CondExactly, Arg: core}, true
	case op == "notEqual" && lead && trail:
		return sheet.Condition{Op: sheet.CondNotContains, Arg: core}, true
	case op == "notEqual" && !lead && !trail:
		return sheet.Condition{Op: sheet.CondNotEqual, Arg: core}, true
	}
	return sheet.Condition{}, false
}

// wildcardShape splits a filter pattern into whether it starts and ends
// with *, and the text between, unescaped (~ escapes the next character);
// false when it has other wildcards.
func wildcardShape(p string) (lead bool, core string, trail bool, ok bool) {
	var b strings.Builder
	rs := []rune(p)
	for i := 0; i < len(rs); i++ {
		switch r := rs[i]; {
		case r == '~' && i+1 < len(rs):
			i++
			b.WriteRune(rs[i])
		case r == '*' && i == 0:
			lead = true
		case r == '*' && i == len(rs)-1:
			trail = true
		case r == '*' || r == '?':
			return false, "", false, false
		default:
			b.WriteRune(r)
		}
	}
	return lead, b.String(), trail, true
}
