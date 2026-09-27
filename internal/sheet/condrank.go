package sheet

import (
	"errors"
	"math"
	"strconv"
	"strings"
)

// Rules that compare a cell with the other cells of the rule's ranges,
// as Excel's top 10, above average and duplicate values: the numbers in
// the top or bottom N (or N percent), those above or below their
// average, and values that appear more than once, or once. What they
// compare with is worked out once per recalculation, reading the values
// the ranges hold, as a color scale does.

// rankStats is what a ranking rule compares a cell's value with.
type rankStats struct {
	cut   float64        // top and bottom: the Nth value; average: the mean
	ok    bool           // the ranges hold a number to compare with
	count map[string]int // duplicates and uniques: how often each value appears
}

func (c *looksCache) rank(s *Sheet, i int) *rankStats {
	if st, ok := c.ranks[i]; ok {
		return st
	}
	f := s.rules.formats[i]
	st := &rankStats{}
	switch f.Op {
	case RuleDuplicate, RuleUnique:
		st.count = map[string]int{}
		for _, r := range f.Ranges {
			for _, v := range s.cells.valuesIn(r) {
				if k, ok := dupKey(v); ok {
					st.count[k]++
				}
			}
		}
	default:
		var vals []float64
		for _, r := range f.Ranges {
			for _, v := range s.cells.valuesIn(r) {
				if v.Kind == Number {
					vals = append(vals, v.Num)
				}
			}
		}
		st.cut, st.ok = rankCut(f.Op, f.Args[0], vals)
	}
	c.ranks[i] = st
	return st
}

// rankCut is what op compares with among vals: the Nth largest or
// smallest value, or the mean. vals is reordered.
func rankCut(op RuleOp, arg string, vals []float64) (float64, bool) {
	if len(vals) == 0 {
		return 0, false
	}
	if op == RuleAboveAverage || op == RuleBelowAverage {
		sum := 0.0
		for _, v := range vals {
			sum += v
		}
		return sum / float64(len(vals)), true
	}
	n, _ := strconv.ParseFloat(strings.TrimSpace(arg), 64)
	if op == RuleTopPercent || op == RuleBottomPercent {
		n = math.Floor(float64(len(vals)) * n / 100) // as Excel: at least one
	}
	k := min(max(int(n), 1), len(vals))
	if op == RuleTop || op == RuleTopPercent {
		return nth(vals, len(vals)-k), true
	}
	return nth(vals, k-1), true
}

// dupKey is what makes two values the same for duplicates: a number's
// value, or text ignoring case, as COUNTIF compares them. Blanks and
// errors have none.
func dupKey(v Value) (string, bool) {
	switch v.Kind {
	case Number:
		return "n" + strconv.FormatFloat(v.Num, 'g', -1, 64), true
	case Bool:
		return "b" + strconv.FormatFloat(v.Num, 'g', -1, 64), true
	case Text:
		if v.Str == "" {
			return "", false
		}
		return "t" + strings.ToLower(v.Str), true
	}
	return "", false
}

// match reports whether a cell's value v meets ranking rule op.
func (st *rankStats) match(op RuleOp, v Value) bool {
	switch op {
	case RuleDuplicate, RuleUnique:
		k, ok := dupKey(v)
		return ok && (st.count[k] > 1) == (op == RuleDuplicate)
	}
	if !st.ok || v.Kind != Number {
		return false
	}
	switch op {
	case RuleTop, RuleTopPercent:
		return v.Num >= st.cut
	case RuleBottom, RuleBottomPercent:
		return v.Num <= st.cut
	case RuleAboveAverage:
		return v.Num > st.cut
	}
	return v.Num < st.cut
}

// checkRank checks the N of a top or bottom rule: a count from 1 to
// 1000, or a percent from 1 to 100, as Excel takes them.
func checkRank(op RuleOp, arg string) error {
	n, err := strconv.Atoi(strings.TrimSpace(arg))
	switch {
	case (op == RuleTopPercent || op == RuleBottomPercent) && (err != nil || n < 1 || n > 100):
		return errors.New("Enter a percent from 1 to 100")
	case err != nil || n < 1 || n > 1000:
		return errors.New("Enter how many values, from 1 to 1000")
	}
	return nil
}

// rankSummary describes a ranking rule: "Top 10", "Bottom 5%".
func rankSummary(op RuleOp, arg string) string {
	arg = strings.TrimSpace(arg)
	switch op {
	case RuleTop:
		return "Top " + arg
	case RuleBottom:
		return "Bottom " + arg
	case RuleTopPercent:
		return "Top " + arg + "%"
	case RuleBottomPercent:
		return "Bottom " + arg + "%"
	}
	return op.Title()
}
