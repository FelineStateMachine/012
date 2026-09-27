package sheet

import (
	"reflect"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Rules are what a sheet says about ranges of its cells beyond their own
// formatting: conditional formats (condfmt.go), which change how cells
// look by their values, and data validation (validation.go), which says
// what may be entered. Both belong to the sheet, as in Sheets: they're
// saved with it, follow inserted and deleted rows and columns, and every
// change to them is an undo step. They never write cells.

// Color is one of the named colors rules draw with. Each is an ANSI
// color slot, so the terminal's palette or the color scheme applies:
// the UI maps them to theme roles, never to fixed RGB.
type Color uint8

const (
	ColorNone Color = iota
	ColorRed
	ColorYellow
	ColorGreen
	ColorCyan
	ColorBlue
	ColorMagenta
	numColors
)

var colorNames = [numColors]string{"", "red", "yellow", "green", "cyan", "blue", "magenta"}

// NumColors is how many colors there are, ColorNone included, for
// tables indexed by Color.
const NumColors = int(numColors)

// Colors lists the named colors, ColorNone first, in the order the rules
// editor offers them.
func Colors() []Color {
	out := make([]Color, numColors)
	for i := range out {
		out[i] = Color(i)
	}
	return out
}

// String names the color as files store it; "" for none.
func (c Color) String() string {
	if c >= numColors {
		return ""
	}
	return colorNames[c]
}

// Title names the color for people, e.g. "Green", or "None".
func (c Color) Title() string {
	if s := c.String(); s != "" {
		return strings.ToUpper(s[:1]) + s[1:]
	}
	return "None"
}

// ParseColor is the inverse of Color.String, ignoring case.
func ParseColor(s string) (Color, bool) {
	i := slices.Index(colorNames[:], strings.ToLower(strings.TrimSpace(s)))
	return Color(max(i, 0)), i >= 0
}

// RuleStyle is what a conditional format rule does to the cells it
// matches: a text color, a fill, and text styles added to the cell's own.
type RuleStyle struct {
	Text, Fill                             Color
	Bold, Italic, Underline, Strikethrough bool
}

// IsZero reports whether the style changes nothing.
func (s RuleStyle) IsZero() bool { return s == RuleStyle{} }

// RuleOp is a test on a cell's value, as Sheets' conditional formatting
// and data validation offer them. Conditional formats use them all;
// validation uses the comparisons (and RuleNone for "any date").
type RuleOp uint8

const (
	RuleNone RuleOp = iota
	RuleEmpty
	RuleNotEmpty
	RuleContains
	RuleNotContains
	RuleStartsWith
	RuleEndsWith
	RuleExactly
	RuleDateIs
	RuleDateBefore
	RuleDateAfter
	RuleGreater
	RuleGreaterEq
	RuleLess
	RuleLessEq
	RuleEqual
	RuleNotEqual
	RuleBetween
	RuleNotBetween
	RuleFormula
	numRuleOps
)

var ruleOpNames = [numRuleOps]string{"", "empty", "not_empty", "contains", "not_contains",
	"starts_with", "ends_with", "exactly", "date_is", "date_before", "date_after",
	"gt", "ge", "lt", "le", "eq", "ne", "between", "not_between", "formula"}

var ruleOpTitles = [numRuleOps]string{"None", "Is empty", "Is not empty", "Text contains",
	"Text does not contain", "Text starts with", "Text ends with", "Text is exactly",
	"Date is", "Date is before", "Date is after",
	"Greater than", "Greater than or equal to", "Less than", "Less than or equal to",
	"Is equal to", "Is not equal to", "Is between", "Is not between", "Custom formula is"}

// String names the test as files store it.
func (op RuleOp) String() string {
	if op >= numRuleOps {
		return ""
	}
	return ruleOpNames[op]
}

// Title names the test for people, e.g. "Greater than".
func (op RuleOp) Title() string {
	if op >= numRuleOps {
		return ""
	}
	return ruleOpTitles[op]
}

// Args is how many values the test compares with: 0, 1, or 2 for
// between.
func (op RuleOp) Args() int {
	switch op {
	case RuleNone, RuleEmpty, RuleNotEmpty:
		return 0
	case RuleBetween, RuleNotBetween:
		return 2
	}
	return 1
}

// ParseRuleOp is the inverse of RuleOp.String.
func ParseRuleOp(s string) (RuleOp, bool) {
	i := slices.Index(ruleOpNames[:], s)
	return RuleOp(max(i, 0)), i >= 0
}

// CondFormatOps lists the tests a single-color conditional format may
// use, in the order Sheets' menu shows them.
func CondFormatOps() []RuleOp {
	return []RuleOp{RuleEmpty, RuleNotEmpty, RuleContains, RuleNotContains, RuleStartsWith,
		RuleEndsWith, RuleExactly, RuleDateIs, RuleDateBefore, RuleDateAfter, RuleGreater,
		RuleGreaterEq, RuleLess, RuleLessEq, RuleEqual, RuleNotEqual, RuleBetween,
		RuleNotBetween, RuleFormula}
}

// CompareOps are the comparisons data validation offers for numbers,
// dates and text lengths.
func CompareOps() []RuleOp {
	return []RuleOp{RuleBetween, RuleNotBetween, RuleEqual, RuleNotEqual, RuleGreater,
		RuleGreaterEq, RuleLess, RuleLessEq}
}

// rulesState is a sheet's rules. The slices are never changed in place:
// a change replaces them, so undo steps can keep them as they were.
type rulesState struct {
	formats     []CondFormat
	validations []Validation
}

func (r rulesState) equal(o rulesState) bool {
	return reflect.DeepEqual(r.formats, o.formats) && reflect.DeepEqual(r.validations, o.validations)
}

func (r rulesState) empty() bool { return len(r.formats) == 0 && len(r.validations) == 0 }

// recordRules saves the sheet's rules before their first change in the
// open step, and drops what was worked out from them.
func (s *Sheet) recordRules() {
	s.looks.reset()
	st := s.wb.hist.open
	if st == nil {
		return
	}
	if _, seen := st.rules[s]; !seen {
		st.rules[s] = s.rules
	}
}

// setRules replaces the sheet's rules as one undo step.
func (s *Sheet) setRules(label string, focus Rect, r rulesState) {
	if r.equal(s.rules) {
		return
	}
	s.change(label, focus, func() {
		s.recordRules()
		s.rules = r
	})
}

// rangesText writes ranges as Sheets lists them, "A1:A9,C1:C9".
func rangesText(rs []Rect) string {
	parts := make([]string, len(rs))
	for i, r := range rs {
		parts[i] = r.String()
	}
	return strings.Join(parts, ",")
}

// ParseRanges reads ranges written as "A1:A9,C1:C9" (spaces or commas
// between), as the rules editor takes them.
func ParseRanges(s string) ([]Rect, bool) {
	fields := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ' ' })
	if len(fields) == 0 {
		return nil, false
	}
	out := make([]Rect, 0, len(fields))
	for _, f := range fields {
		r, ok := ParseRange(strings.ReplaceAll(f, "$", ""))
		if !ok {
			return nil, false
		}
		out = append(out, r)
	}
	return out, true
}

// RangesText writes ranges as ParseRanges reads them.
func RangesText(rs []Rect) string { return rangesText(rs) }

// inRanges reports whether a is in one of rs.
func inRanges(rs []Rect, a Addr) bool {
	for _, r := range rs {
		if r.Contains(a) {
			return true
		}
	}
	return false
}

// anchor is the top-left cell of the first range, which relative
// references in a rule's formula are written for.
func anchor(rs []Rect) Addr {
	if len(rs) == 0 {
		return Addr{}
	}
	return rs[0].From
}

// shiftRanges moves ranges with inserted or deleted lines, as a formula's
// range would move, dropping those deleted entirely.
func shiftRanges(rs []Rect, rng func(Rect) (Rect, bool)) []Rect {
	var out []Rect
	for _, r := range rs {
		if to, ok := rng(r); ok {
			out = append(out, to)
		}
	}
	return out
}

// shiftFormula rewrites a rule's formula (with its "=") for moved cells,
// or returns it unchanged when it doesn't parse.
func shiftFormula(src string, rw formula.Rewriter) string {
	if src == "" {
		return src
	}
	n, err := Parse(src)
	if err != nil {
		return src
	}
	if out, changed := formula.Rewrite(n, rw); changed {
		return formula.Text(out)
	}
	return src
}

// shiftRules keeps every sheet's rules in step with rows or columns
// inserted or deleted on s: the ranges of s's rules move, and formulas
// and list sources follow the cells they read, on s or on s by name.
func (s *Sheet) shiftRules(rows bool, sp formula.Span) {
	cell, rng := formula.AxisMaps(rows, sp)
	for _, t := range s.wb.sheets {
		if t.rules.empty() {
			continue
		}
		on := func(name string) bool {
			if name == "" {
				return t == s
			}
			return formula.SheetKey(name) == formula.SheetKey(s.name)
		}
		rw := formula.Relocate(on, cell, rng)
		next := t.rules.rewritten(func(rs []Rect) []Rect {
			if t != s {
				return rs
			}
			return shiftRanges(rs, rng)
		}, rw)
		if !next.equal(t.rules) {
			t.recordRules()
			t.rules = next
		}
	}
}

// renameRules rewrites the formulas and list sources of every rule that
// name the sheet with key old, to name.
func (w *Workbook) renameRules(old, name string) {
	rename := func(sheet string) string {
		if sheet != "" && formula.SheetKey(sheet) == old {
			return name
		}
		return sheet
	}
	rw := formula.Rewriter{
		Ref:   func(n formula.Ref) Node { n.Sheet = rename(n.Sheet); return n },
		Range: func(n formula.Range) Node { n.Sheet = rename(n.Sheet); return n },
	}
	for _, t := range w.sheets {
		if t.rules.empty() {
			continue
		}
		next := t.rules.rewritten(func(rs []Rect) []Rect { return rs }, rw)
		if !next.equal(t.rules) {
			t.recordRules()
			t.rules = next
		}
	}
}

// rewritten is the rules with their ranges mapped by ranges and their
// formulas and sources rewritten by rw; rules left with no range go.
func (r rulesState) rewritten(ranges func([]Rect) []Rect, rw formula.Rewriter) rulesState {
	var out rulesState
	for _, f := range r.formats {
		f.Ranges = ranges(f.Ranges)
		if len(f.Ranges) == 0 {
			continue
		}
		if f.Op == RuleFormula {
			f.Args[0] = shiftFormula(f.Args[0], rw)
		}
		out.formats = append(out.formats, f)
	}
	for _, v := range r.validations {
		v.Ranges = ranges(v.Ranges)
		if len(v.Ranges) == 0 {
			continue
		}
		switch v.Kind {
		case ValidFormula:
			v.Args[0] = shiftFormula(v.Args[0], rw)
		case ValidRange:
			v.Source = strings.TrimPrefix(shiftFormula("="+v.Source, rw), "=")
		}
		out.validations = append(out.validations, v)
	}
	return out
}
