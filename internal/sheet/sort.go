package sheet

import (
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// SortKey is one column to sort by.
type SortKey struct {
	Col  int
	Desc bool // Z to A
}

// sortRank orders kinds as Sheets sorts them A to Z: numbers (dates are
// numbers), then text, then booleans, then errors. Blanks always go last,
// in either direction.
func sortRank(v Value) int {
	switch v.Kind {
	case Number:
		return 0
	case Text:
		return 1
	case Bool:
		return 2
	case Error:
		return 3
	}
	return 4
}

// sortCompare orders two values A to Z: by kind, then numerically, or
// alphabetically ignoring case.
func sortCompare(a, b Value) int {
	if d := sortRank(a) - sortRank(b); d != 0 {
		return d
	}
	switch a.Kind {
	case Number, Bool:
		switch {
		case a.Num < b.Num:
			return -1
		case a.Num > b.Num:
			return 1
		}
	case Text, Error:
		return strings.Compare(strings.ToLower(a.Str), strings.ToLower(b.Str))
	}
	return 0
}

// compareRows orders rows i and j by keys, first key first. Blanks go
// last in either direction.
func (s *Sheet) compareRows(keys []SortKey, i, j int) int {
	for _, k := range keys {
		a, b := s.Value(Addr{Col: k.Col, Row: i}), s.Value(Addr{Col: k.Col, Row: j})
		if (a.Kind == Empty) != (b.Kind == Empty) {
			if a.Kind == Empty {
				return 1
			}
			return -1
		}
		d := sortCompare(a, b)
		if k.Desc {
			d = -d
		}
		if d != 0 {
			return d
		}
	}
	return 0
}

// SortRange sorts the rows of r by keys, first key first, as Sheets' Data
// > Sort range does (r excludes any header row). The sort is stable, so
// rows that tie keep their order. Only the cells inside r move. Formulas
// move with their rows and their relative references shift by the
// distance moved, as if copied there; references to the sorted cells
// from elsewhere are left alone. The whole sort is one undo step.
//
// Only rows holding cells are sorted: rows whose keys are all blank go
// last in their order, as blank rows do, so the cost is the rows with
// data, however tall r is.
func (s *Sheet) SortRange(r Rect, keys []SortKey) {
	if used, ok := s.UsedRange(); ok {
		r.To.Row, r.To.Col = min(r.To.Row, used.To.Row), min(r.To.Col, used.To.Col)
	}
	if len(keys) == 0 || r.To.Row <= r.From.Row || r.To.Col < r.From.Col || len(s.MergesIn(r)) > 0 {
		return
	}
	old := map[Addr]*Cell{}
	var rows []int // rows of r with cells, in order
	for a, c := range s.cells.inRange(r) {
		old[a] = c
		if len(rows) == 0 || rows[len(rows)-1] != a.Row {
			rows = append(rows, a.Row)
		}
	}
	dst := s.sortedRows(r, rows, keys)
	src := make(map[int]int, len(dst)) // where each row's new contents come from
	for from, to := range dst {
		src[to] = from
	}
	s.change("sort "+r.String(), r, func() {
		for a := range old {
			if from, ok := src[a.Row]; !ok || old[Addr{Col: a.Col, Row: from}] == nil {
				s.place(a, nil) // nothing moves here
			}
		}
		for a, c := range old {
			if to := dst[a.Row]; to != a.Row {
				s.place(Addr{Col: a.Col, Row: to}, c.rewritten(formula.Shift(0, to-a.Row)))
			}
		}
	})
}

// sortedRows maps each of rows, the rows of r with cells, to where the
// sort puts it. Rows with a key sort to the top; the others (blank keys,
// and blank rows) keep their order below them.
func (s *Sheet) sortedRows(r Rect, rows []int, keys []SortKey) map[int]int {
	var keyed, rest []int
	for _, row := range rows {
		if s.keysBlank(keys, row) {
			rest = append(rest, row)
		} else {
			keyed = append(keyed, row)
		}
	}
	before := slices.Clone(keyed) // in their original order
	slices.SortStableFunc(keyed, func(i, j int) int { return s.compareRows(keys, i, j) })
	dst := make(map[int]int, len(rows))
	for i, row := range keyed {
		dst[row] = r.From.Row + i
	}
	// The other rows close up below: each moves down past the keyed rows
	// that were below it and up past those that were above it.
	n := 0
	for _, row := range rest {
		for n < len(before) && before[n] < row {
			n++
		}
		dst[row] = row + len(keyed) - n
	}
	return dst
}

// keysBlank reports whether every key of row is blank.
func (s *Sheet) keysBlank(keys []SortKey, row int) bool {
	for _, k := range keys {
		if s.Value(Addr{Col: k.Col, Row: row}).Kind != Empty {
			return false
		}
	}
	return true
}
