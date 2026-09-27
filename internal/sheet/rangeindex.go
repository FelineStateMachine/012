package sheet

// rangeIndex finds the formulas whose range references may contain a
// cell, bucketed by column. Recalculation asks it about every changed
// cell; scanning every range user instead made a change cost O(changed x
// range users), so filling a column of 8192 running totals checked 67
// million ranges. A formula whose ranges span several columns is in each
// of their buckets.
type rangeIndex struct {
	byCol [MaxCols]map[Addr]struct{}
}

// add indexes the formula at a under the columns its ranges cover.
func (x *rangeIndex) add(a Addr, ranges []Rect) {
	for _, r := range ranges {
		for c := max(r.From.Col, 0); c <= min(r.To.Col, MaxCols-1); c++ {
			if x.byCol[c] == nil {
				x.byCol[c] = make(map[Addr]struct{})
			}
			x.byCol[c][a] = struct{}{}
		}
	}
}

// remove undoes add for the same ranges.
func (x *rangeIndex) remove(a Addr, ranges []Rect) {
	for _, r := range ranges {
		for c := max(r.From.Col, 0); c <= min(r.To.Col, MaxCols-1); c++ {
			delete(x.byCol[c], a)
		}
	}
}

// candidates returns the formulas with a range over column col; callers
// still test whether one of their ranges contains the cell.
func (x *rangeIndex) candidates(col int) map[Addr]struct{} {
	if col < 0 || col >= MaxCols {
		return nil
	}
	return x.byCol[col]
}
