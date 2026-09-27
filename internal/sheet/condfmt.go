package sheet

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/value"
)

// Conditional formats follow Sheets' Format > Conditional formatting: a
// rule applies to ranges of the sheet and either formats the cells that
// meet a test in a single color (a text color, a fill, text styles) or
// colors every number along a color scale from its lowest to its
// highest. Rules are tried in order and the first that applies to a cell
// formats it; see looks.go for how they're evaluated.

// CondFormat is a conditional format rule.
type CondFormat struct {
	Ranges []Rect
	// Op is the test of a single-color rule and Args its values, as
	// typed: "100", "=B2" (a formula, relative to the first range's
	// top-left cell as a copied formula would be), a date, or for
	// RuleFormula the formula, e.g. "=$C2>100".
	Op    RuleOp
	Args  [2]string
	Style RuleStyle
	// Scale, when set, makes the rule a color scale of 2 or 3 points,
	// lowest first, and Op, Args and Style mean nothing.
	Scale []ScalePoint
}

// ScalePoint is a point of a color scale: where it is among the values,
// and its color.
type ScalePoint struct {
	Kind  PointKind
	Value string // the number, percent or percentile; unused by min and max
	Color Color
}

// PointKind is how a color scale point is placed.
type PointKind uint8

const (
	PointMin        PointKind = iota // the lowest value
	PointMax                         // the highest value
	PointNumber                      // a number
	PointPercent                     // a percentage of the way from lowest to highest
	PointPercentile                  // a percentile of the values
	numPointKinds
)

var pointNames = [numPointKinds]string{"min", "max", "num", "percent", "percentile"}
var pointTitles = [numPointKinds]string{"Min value", "Max value", "Number", "Percent", "Percentile"}

func (k PointKind) String() string {
	if k >= numPointKinds {
		return ""
	}
	return pointNames[k]
}

// Title names the kind for people, e.g. "Percentile".
func (k PointKind) Title() string {
	if k >= numPointKinds {
		return ""
	}
	return pointTitles[k]
}

// TakesValue reports whether the point needs a value.
func (k PointKind) TakesValue() bool { return k >= PointNumber }

// ParsePointKind is the inverse of PointKind.String.
func ParsePointKind(s string) (PointKind, bool) {
	i := slices.Index(pointNames[:], s)
	return PointKind(max(i, 0)), i >= 0
}

// PointKinds lists the kinds in the order the rules editor offers them.
func PointKinds() []PointKind {
	return []PointKind{PointMin, PointMax, PointNumber, PointPercent, PointPercentile}
}

// IsScale reports whether the rule is a color scale.
func (f CondFormat) IsScale() bool { return len(f.Scale) > 0 }

// Summary describes the rule in a few words, e.g. "Greater than 100".
func (f CondFormat) Summary() string {
	if f.IsScale() {
		return "Color scale"
	}
	switch f.Op.Args() {
	case 1:
		return f.Op.Title() + " " + f.Args[0]
	case 2:
		return f.Op.Title() + " " + f.Args[0] + " and " + f.Args[1]
	}
	return f.Op.Title()
}

// clone copies the rule's slices, so a stored rule is never shared.
func (f CondFormat) clone() CondFormat {
	f.Ranges, f.Scale = slices.Clone(f.Ranges), slices.Clone(f.Scale)
	return f
}

// Check reports why a rule can't be used, in words for the rules editor.
func (f CondFormat) Check() error {
	if len(f.Ranges) == 0 {
		return errors.New("Enter the range the rule applies to, e.g. A2:A100")
	}
	if f.IsScale() {
		return checkScale(f.Scale)
	}
	if !slices.Contains(CondFormatOps(), f.Op) {
		return errors.New("Choose a condition")
	}
	for i := range f.Op.Args() {
		if err := checkArg(f.Op, strings.TrimSpace(f.Args[i])); err != nil {
			return err
		}
	}
	if f.Style.IsZero() {
		return errors.New("Choose a text color, a fill or a text style")
	}
	return nil
}

// checkArg checks one value of a test.
func checkArg(op RuleOp, arg string) error {
	switch {
	case arg == "":
		return errors.New("Enter a value")
	case op == RuleFormula && !IsFormulaEntry(arg):
		return errors.New("A custom formula starts with =, e.g. =$C2>100")
	case IsFormulaEntry(arg):
		if _, err := Parse(arg); err != nil {
			return fmt.Errorf("Formula: %w", err)
		}
	case op == RuleDateIs || op == RuleDateBefore || op == RuleDateAfter:
		if _, ok := dateArg(arg); !ok {
			return errors.New("Enter a date, e.g. 2026-09-30, or today, tomorrow or yesterday")
		}
	case op == RuleBetween || op == RuleNotBetween:
		if _, _, ok := ParseValue(arg); !ok {
			return errors.New("Enter a number")
		}
	}
	return nil
}

// checkScale checks a color scale's points: 2 or 3, each with a color
// and a value where it takes one.
func checkScale(ps []ScalePoint) error {
	if len(ps) < 2 || len(ps) > 3 {
		return errors.New("A color scale has a minimum, a maximum and at most one midpoint")
	}
	for _, p := range ps {
		if p.Kind >= numPointKinds {
			return errors.New("Unknown kind of point")
		}
		if p.Color == ColorNone {
			return errors.New("Choose a color for every point")
		}
		if !p.Kind.TakesValue() {
			continue
		}
		n, _, ok := ParseValue(strings.TrimSpace(p.Value))
		switch {
		case !ok:
			return errors.New("Enter a number for the " + strings.ToLower(p.Kind.Title()))
		case p.Kind != PointNumber && (n < 0 || n > 100):
			return errors.New(p.Kind.Title() + " goes from 0 to 100")
		}
	}
	return nil
}

// dateArg reads a date condition's value: a date as typed, a number of
// days, or today, tomorrow or yesterday, as Sheets offers them.
func dateArg(arg string) (float64, bool) {
	today := float64(int64(numfmt.SerialOf(value.Now())))
	switch strings.ToLower(strings.TrimSpace(arg)) {
	case "today":
		return today, true
	case "tomorrow":
		return today + 1, true
	case "yesterday":
		return today - 1, true
	}
	n, _, ok := ParseValue(strings.TrimSpace(arg))
	return float64(int64(n)), ok
}

// CondFormats returns the sheet's conditional format rules, in the order
// they're tried.
func (s *Sheet) CondFormats() []CondFormat {
	out := make([]CondFormat, len(s.rules.formats))
	for i, f := range s.rules.formats {
		out[i] = f.clone()
	}
	return out
}

// AddCondFormat adds a rule after the others, as one undo step.
func (s *Sheet) AddCondFormat(f CondFormat) error {
	if err := f.Check(); err != nil {
		return err
	}
	r := s.rules
	r.formats = append(slices.Clip(r.formats), f.clone())
	s.setRules("add conditional format "+rangesText(f.Ranges), f.Ranges[0], r)
	return nil
}

// SetCondFormat replaces rule i, as one undo step.
func (s *Sheet) SetCondFormat(i int, f CondFormat) error {
	if i < 0 || i >= len(s.rules.formats) {
		return errors.New("That rule was removed")
	}
	if err := f.Check(); err != nil {
		return err
	}
	r := s.rules
	r.formats = slices.Clone(r.formats)
	r.formats[i] = f.clone()
	s.setRules("edit conditional format "+rangesText(f.Ranges), f.Ranges[0], r)
	return nil
}

// DeleteCondFormat removes rule i.
func (s *Sheet) DeleteCondFormat(i int) {
	if i < 0 || i >= len(s.rules.formats) {
		return
	}
	r := s.rules
	r.formats = slices.Delete(slices.Clone(r.formats), i, i+1)
	f := s.rules.formats[i]
	s.setRules("remove conditional format "+rangesText(f.Ranges), f.Ranges[0], r)
}

// MoveCondFormat moves rule i to position to, changing which rule wins
// where several apply.
func (s *Sheet) MoveCondFormat(i, to int) {
	n := len(s.rules.formats)
	if i < 0 || i >= n || to < 0 || to >= n || i == to {
		return
	}
	r := s.rules
	r.formats = slices.Clone(r.formats)
	f := r.formats[i]
	r.formats = slices.Insert(slices.Delete(r.formats, i, i+1), to, f)
	s.setRules("reorder conditional formats", f.Ranges[0], r)
}

// ClearCondFormats takes the cells of cr out of every rule, removing
// rules left with no cells, as Sheets' "Clear formatting rules" for a
// selection.
func (s *Sheet) ClearCondFormats(cr Rect) {
	r := s.rules
	r.formats = nil
	for _, f := range s.rules.formats {
		f.Ranges = subtractAll(f.Ranges, cr)
		if len(f.Ranges) > 0 {
			r.formats = append(r.formats, f)
		}
	}
	s.setRules("clear conditional formats from "+cr.String(), cr, r)
}

// LoadCondFormats adds rules as a loader does: without recording undo.
// Rules that don't check are left out and counted.
func (s *Sheet) LoadCondFormats(fs []CondFormat) (skipped int) {
	for _, f := range fs {
		if f.Check() != nil {
			skipped++
			continue
		}
		s.rules.formats = append(s.rules.formats, f.clone())
	}
	s.looks.reset()
	return skipped
}

// subtractAll removes cut from every range of rs.
func subtractAll(rs []Rect, cut Rect) []Rect {
	var out []Rect
	for _, r := range rs {
		out = append(out, subtract(r, cut)...)
	}
	return out
}

// subtract is r without the cells of cut: r itself, nothing, or up to
// four ranges around cut.
func subtract(r, cut Rect) []Rect {
	lo := Addr{Col: max(r.From.Col, cut.From.Col), Row: max(r.From.Row, cut.From.Row)}
	hi := Addr{Col: min(r.To.Col, cut.To.Col), Row: min(r.To.Row, cut.To.Row)}
	if lo.Col > hi.Col || lo.Row > hi.Row {
		return []Rect{r}
	}
	var out []Rect
	if r.From.Row < lo.Row { // above
		out = append(out, Rect{From: r.From, To: Addr{Col: r.To.Col, Row: lo.Row - 1}})
	}
	if hi.Row < r.To.Row { // below
		out = append(out, Rect{From: Addr{Col: r.From.Col, Row: hi.Row + 1}, To: r.To})
	}
	if r.From.Col < lo.Col { // left, beside cut
		out = append(out, Rect{From: Addr{Col: r.From.Col, Row: lo.Row}, To: Addr{Col: lo.Col - 1, Row: hi.Row}})
	}
	if hi.Col < r.To.Col { // right, beside cut
		out = append(out, Rect{From: Addr{Col: hi.Col + 1, Row: lo.Row}, To: Addr{Col: r.To.Col, Row: hi.Row}})
	}
	return out
}

// scaleValue is where a point sits among values, sorted ascending.
func (p ScalePoint) at(vals []float64) float64 {
	lo, hi := vals[0], vals[len(vals)-1]
	n, _, _ := ParseValue(strings.TrimSpace(p.Value))
	switch p.Kind {
	case PointMin:
		return lo
	case PointMax:
		return hi
	case PointPercent:
		return lo + (hi-lo)*n/100
	case PointPercentile:
		return percentile(vals, n/100)
	}
	return n
}

// percentile interpolates between sorted values, as PERCENTILE.INC.
func percentile(vals []float64, p float64) float64 {
	p = min(max(p, 0), 1)
	x := p * float64(len(vals)-1)
	i := int(x)
	if i >= len(vals)-1 {
		return vals[len(vals)-1]
	}
	return vals[i] + (x-float64(i))*(vals[i+1]-vals[i])
}

// pointText writes a point as the rules editor shows it, e.g. "Min" or
// "Percentile 50".
func (p ScalePoint) String() string {
	if p.Kind.TakesValue() {
		return p.Kind.Title() + " " + p.Value
	}
	return p.Kind.Title()
}
