package sheet

import (
	"math"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/FelineStateMachine/012/internal/formula"
	"github.com/FelineStateMachine/012/internal/functions"
	"github.com/FelineStateMachine/012/internal/locale"
)

// Look is how a sheet's rules draw a cell: a conditional format's style
// or place on a color scale, and what its validation rule shows (a
// checkbox, a dropdown's marker, an invalid entry's mark).
type Look struct {
	// Styled is set when a single-color rule applies, with its Style.
	Styled bool
	Style  RuleStyle
	// Scaled is set when a color scale colors the cell, Pos of the way
	// (0 to 1) from From to To.
	Scaled   bool
	From, To Color
	Pos      float64
	// Bar is set when a data bar draws across the cell, BarLen of the
	// way (0 to 1), in BarColor.
	Bar      bool
	BarColor Color
	BarLen   float64
	// Icon, when set, is the icon an icon set draws at the cell's left,
	// in IconColor.
	Icon      string
	IconColor Color
	// ValueHidden is set when a data bar or icon shows without the
	// cell's value.
	ValueHidden bool

	Checkbox bool // a checkbox, checked when Checked
	Checked  bool
	Dropdown bool        // offers a list to pick from
	Display  DropDisplay // how it shows that it does
	Invalid  bool        // holds what its validation rule doesn't accept
}

// Looks are worked out per cell as the screen asks for them, and kept
// until the next recalculation or change of rules: work follows what's
// drawn, and a color scale reads its range's values once per
// recalculation, not per frame. The cache is dropped whole when it
// grows past maxLooks, as scrolling through a long range would make it.
type looksCache struct {
	wb     *Workbook
	gen    uint64 // the workbook's recalculation when filled
	valid  bool
	cells  map[Addr]Look
	tests  map[int]*ruleTest       // compiled conditional formats by index
	scales map[int]*scaleStats     // color scales' points by index
	bars   map[int]*barStats       // data bars' and icon sets' points by index
	ranks  map[int]*rankStats      // what ranking rules compare with, by index
	lists  map[int]map[string]bool // dropdown sources' values by validation index
	rd     *reader
}

const maxLooks = 1 << 16

func (c *looksCache) reset() { c.valid = false }

// fresh empties the cache if it was filled before the last
// recalculation or rule change.
func (c *looksCache) fresh(s *Sheet) {
	if c.valid && c.wb == s.wb && c.gen == s.wb.gen {
		return
	}
	*c = looksCache{wb: s.wb, gen: s.wb.gen, valid: true, cells: map[Addr]Look{}, tests: map[int]*ruleTest{},
		scales: map[int]*scaleStats{}, lists: map[int]map[string]bool{}, bars: map[int]*barStats{}, ranks: map[int]*rankStats{}}
}

// HasRules reports whether the sheet has conditional formats or data
// validation, so drawing can skip asking for looks when it has none.
func (s *Sheet) HasRules() bool { return !s.rules.empty() }

// Look returns how the sheet's rules draw the cell at a.
func (s *Sheet) Look(a Addr) Look {
	if s.rules.empty() {
		return Look{}
	}
	c := &s.looks
	c.fresh(s)
	if l, ok := c.cells[a]; ok {
		return l
	}
	l := c.look(s, a)
	if len(c.cells) >= maxLooks {
		clear(c.cells)
	}
	c.cells[a] = l
	return l
}

func (c *looksCache) look(s *Sheet, a Addr) Look {
	var l Look
	v := s.Value(a)
	for i, f := range s.rules.formats {
		if inRanges(f.Ranges, a) && c.apply(s, i, f, a, v, &l) {
			break
		}
	}
	if i := s.validationIndex(a); i >= 0 {
		r := s.rules.validations[i]
		l.Checkbox, l.Dropdown, l.Display = r.Kind == ValidCheckbox, r.Kind.Dropdown(), r.Display
		l.Checked = l.Checkbox && r.isChecked(v, s.ShownText(a))
		l.Invalid = s.cells.filledAt(a) && !c.check(s, i, a, v, s.DisplayFormat(a))
	}
	return l
}

// apply sets what rule i (f) draws on the cell at a, of value v, on l,
// reporting whether it applies there.
func (c *looksCache) apply(s *Sheet, i int, f CondFormat, a Addr, v Value, l *Look) bool {
	switch {
	case f.IsBar():
		n, ok := c.bar(s, i).barLook(v)
		if ok {
			l.Bar, l.BarLen, l.BarColor, l.ValueHidden = true, n, f.Bar, f.BarOnly
		}
		return ok
	case f.IsIcons():
		k, ok := c.bar(s, i).icon(v, f.Reverse)
		if ok {
			n := len(f.Scale) + 1
			l.Icon, l.IconColor, l.ValueHidden = f.Icons.Glyphs(n)[k], f.Icons.IconColor(k, n), f.BarOnly
		}
		return ok
	case f.IsScale():
		pos, from, to, ok := c.scale(s, i).place(v)
		if ok {
			l.Scaled, l.Pos, l.From, l.To = true, pos, from, to
		}
		return ok
	case f.Op.Ranks():
		if !c.rank(s, i).match(f.Op, v) {
			return false
		}
	case !c.test(s, i).match(c, s, a, v):
		return false
	}
	l.Styled, l.Style = true, f.Style
	return true
}

// reader reads current values for rules' formulas.
func (c *looksCache) reader(s *Sheet) *reader {
	if c.rd == nil {
		c.rd = s.wb.values(s)
	}
	return c.rd
}

// eval evaluates n as if in the cell at a.
func (c *looksCache) eval(s *Sheet, n Node, a Addr) Value {
	return functions.EvalAt(n, c.reader(s).lib, a)
}

// evalFrom evaluates a rule's formula n, written for the cell from, in
// the cell at a: its relative references move as a copy's would.
func (c *looksCache) evalFrom(s *Sheet, n Node, from, a Addr) Value {
	if a != from {
		n, _ = formula.Rewrite(n, formula.Shift(a.Col-from.Col, a.Row-from.Row))
	}
	return c.eval(s, n, a)
}

func truthy(v Value) bool { return (v.Kind == Bool || v.Kind == Number) && v.Num != 0 }

// ruleTest is a single-color rule ready to run: its values parsed, or
// kept as formulas to evaluate in each cell.
type ruleTest struct {
	op     RuleOp
	from   Addr
	args   [2]Node // formulas among the values; nil for constants
	nums   [2]float64
	isNum  [2]bool
	shared func(Value, string) bool // the filter's test, for the ops filters share with constant values
}

// filterOps maps the tests conditional formats share with filters.
var filterOps = map[RuleOp]CondOp{RuleEmpty: CondEmpty, RuleNotEmpty: CondNotEmpty,
	RuleContains: CondContains, RuleNotContains: CondNotContains, RuleStartsWith: CondStartsWith,
	RuleEndsWith: CondEndsWith, RuleExactly: CondExactly, RuleGreater: CondGreater,
	RuleGreaterEq: CondGreaterEq, RuleLess: CondLess, RuleLessEq: CondLessEq, RuleEqual: CondEqual,
	RuleNotEqual: CondNotEqual}

func (c *looksCache) test(s *Sheet, i int) *ruleTest {
	if t, ok := c.tests[i]; ok {
		return t
	}
	f := s.rules.formats[i]
	t := &ruleTest{op: f.Op, from: anchor(f.Ranges)}
	t.parse(f.Op, f.Args)
	if cop, ok := filterOps[f.Op]; ok && t.args[0] == nil {
		t.shared = Condition{Op: cop, Arg: f.Args[0]}.test()
	}
	c.tests[i] = t
	return t
}

// parse reads the values of op: formulas are parsed, dates and numbers
// read.
func (t *ruleTest) parse(op RuleOp, args [2]string) {
	for k := range op.Args() {
		arg := strings.TrimSpace(args[k])
		switch {
		case IsFormulaEntry(arg):
			t.args[k], _ = Parse(arg)
		case op == RuleDateIs || op == RuleDateBefore || op == RuleDateAfter:
			t.nums[0], t.nums[1], t.isNum[0] = datePeriod(arg)
			t.isNum[1] = t.isNum[0]
		default:
			n, _, ok := ParseValue(arg)
			t.nums[k], t.isNum[k] = n, ok
		}
	}
}

// arg is value k of the test in the cell at a.
func (t *ruleTest) arg(c *looksCache, s *Sheet, k int, a Addr) Value {
	if t.args[k] != nil {
		return c.evalFrom(s, t.args[k], t.from, a)
	}
	if t.isNum[k] {
		return Value{Kind: Number, Num: t.nums[k]}
	}
	return Value{}
}

func (t *ruleTest) match(c *looksCache, s *Sheet, a Addr, v Value) bool {
	switch {
	case t.shared != nil:
		return t.shared(v, s.LocalText(a))
	case t.op == RuleFormula:
		return t.args[0] != nil && truthy(c.evalFrom(s, t.args[0], t.from, a))
	}
	if cop, ok := filterOps[t.op]; ok { // a shared test with a formula value
		arg, loc := t.arg(c, s, 0, a), locale.Canonical
		if cop.OnText() {
			loc = s.Locale()
		}
		return Condition{Op: cop, Arg: FormatTextIn(arg, Format{}, loc)}.test()(v, s.LocalText(a))
	}
	if v.Kind != Number {
		return false
	}
	x, lo := v.Num, t.arg(c, s, 0, a)
	if lo.Kind != Number {
		return false
	}
	switch t.op {
	case RuleDateIs, RuleDateBefore, RuleDateAfter:
		first, last, day := float64(int64(lo.Num)), float64(int64(lo.Num)), float64(int64(x))
		if t.args[0] == nil && t.isNum[1] {
			last = t.nums[1] // a period's last day
		}
		switch t.op {
		case RuleDateIs:
			return day >= first && day <= last
		case RuleDateBefore:
			return day < first
		}
		return day > last
	}
	hi := t.arg(c, s, 1, a)
	return hi.Kind == Number && compareNum(t.op, x, lo.Num, hi.Num)
}

// compareNum compares x with one bound, or two for between.
func compareNum(op RuleOp, x, lo, hi float64) bool {
	switch op {
	case RuleBetween:
		return x >= min(lo, hi) && x <= max(lo, hi)
	case RuleNotBetween:
		return x < min(lo, hi) || x > max(lo, hi)
	case RuleEqual:
		return x == lo
	case RuleNotEqual:
		return x != lo
	case RuleGreater:
		return x > lo
	case RuleGreaterEq:
		return x >= lo
	case RuleLess:
		return x < lo
	case RuleLessEq:
		return x <= lo
	}
	return true
}

// scaleStats is a color scale's points among the values of its ranges.
type scaleStats struct {
	ok     bool
	at     []float64
	colors []Color
}

func (c *looksCache) scale(s *Sheet, i int) *scaleStats {
	if st, ok := c.scales[i]; ok {
		return st
	}
	f := s.rules.formats[i]
	// Only a percentile needs the values themselves; the rest need the
	// lowest and highest.
	keep := slices.ContainsFunc(f.Scale, func(p ScalePoint) bool { return p.Kind == PointPercentile })
	lo, hi, n := math.Inf(1), math.Inf(-1), 0
	var vals []float64
	for _, r := range f.Ranges {
		for _, v := range s.cells.valuesIn(r) {
			if v.Kind != Number {
				continue
			}
			v := v.Num
			lo, hi, n = min(lo, v), max(hi, v), n+1
			if keep {
				vals = append(vals, v)
			}
		}
	}
	st := &scaleStats{}
	if n > 0 {
		st.ok = true
		for _, p := range f.Scale {
			st.at = append(st.at, p.at(vals, lo, hi))
			st.colors = append(st.colors, p.Color)
		}
	}
	c.scales[i] = st
	return st
}

// place is where v falls on the scale: the fraction of the way between
// the colors of the points around it.
func (st *scaleStats) place(v Value) (pos float64, from, to Color, ok bool) {
	if !st.ok || v.Kind != Number {
		return 0, 0, 0, false
	}
	k := 0
	if len(st.at) == 3 && v.Num > st.at[1] {
		k = 1
	}
	lo, hi := st.at[k], st.at[k+1]
	switch {
	case hi <= lo:
		pos = 0
		if v.Num >= hi {
			pos = 1
		}
	default:
		pos = min(max((v.Num-lo)/(hi-lo), 0), 1)
	}
	return pos, st.colors[k], st.colors[k+1], true
}

// check reports whether value v, shown in format f, meets validation
// rule i in the cell at a. Blanks always do.
func (c *looksCache) check(s *Sheet, i int, a Addr, v Value, f Format) bool {
	c.fresh(s)
	r := s.rules.validations[i]
	switch r.Kind {
	case ValidList:
		shown := strings.TrimSpace(FormatText(v, f))
		return v.Kind == Empty || slices.ContainsFunc(r.Items, func(it string) bool { return strings.EqualFold(strings.TrimSpace(it), shown) })
	case ValidRange:
		return v.Kind == Empty || c.list(s, i)[strings.ToLower(strings.TrimSpace(FormatText(v, f)))]
	case ValidCheckbox:
		return r.checkboxValid(v, FormatText(v, f))
	case ValidFormula:
		n, err := Parse(strings.TrimSpace(r.Args[0]))
		return err == nil && truthy(c.evalFrom(s, n, anchor(r.Ranges), a))
	case ValidDate:
		if v.Kind != Number || f.Kind < FmtDate || f.Kind > FmtDateTime || f.Kind == FmtTime {
			return false
		}
		if r.Op == RuleNone {
			return true
		}
	case ValidNumber:
		if v.Kind != Number {
			return false
		}
	case ValidLength:
		v = Value{Kind: Number, Num: float64(utf8.RuneCountInString(FormatText(v, f)))}
	}
	return c.bounds(s, r, a, v.Num)
}

// bounds compares x with a rule's values, evaluated in the cell at a.
func (c *looksCache) bounds(s *Sheet, r Validation, a Addr, x float64) bool {
	t := ruleTest{op: r.Op, from: anchor(r.Ranges)}
	t.parse(r.Op, r.Args)
	lo, hi := t.arg(c, s, 0, a), t.arg(c, s, 1, a)
	if lo.Kind != Number || r.Op.Args() == 2 && hi.Kind != Number {
		return false
	}
	return compareNum(r.Op, x, lo.Num, hi.Num)
}

// list is the values of a dropdown's source, by lower case.
func (c *looksCache) list(s *Sheet, i int) map[string]bool {
	if l, ok := c.lists[i]; ok {
		return l
	}
	l := map[string]bool{}
	for _, it := range s.sourceItems(s.rules.validations[i]) {
		l[strings.ToLower(it)] = true
	}
	c.lists[i] = l
	return l
}
