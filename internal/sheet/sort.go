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

// SortRange sorts the rows of r by keys, first key first, as Sheets' Data
// > Sort range does (r excludes any header row). The sort is stable, so
// rows that tie keep their order. Only the cells inside r move. Formulas
// move with their rows and their relative references shift by the
// distance moved, as if copied there; references to the sorted cells
// from elsewhere are left alone. The whole sort is one undo step.
func (s *Sheet) SortRange(r Rect, keys []SortKey) {
	if used, ok := s.UsedRange(); ok {
		r.To.Row, r.To.Col = min(r.To.Row, used.To.Row), min(r.To.Col, used.To.Col)
	}
	if len(keys) == 0 || r.To.Row <= r.From.Row || r.To.Col < r.From.Col {
		return
	}
	order := make([]int, r.To.Row-r.From.Row+1)
	for i := range order {
		order[i] = r.From.Row + i
	}
	slices.SortStableFunc(order, func(i, j int) int {
		for _, k := range keys {
			a, b := s.Value(Addr{Col: k.Col, Row: i}), s.Value(Addr{Col: k.Col, Row: j})
			if (a.Kind == Empty) != (b.Kind == Empty) { // blanks last either way
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
	})
	old := map[Addr]*Cell{}
	for _, a := range s.cellsIn(r) {
		old[a] = s.cells.get(a)
	}
	s.change("sort "+r.String(), r, func() {
		for k, src := range order {
			dst := r.From.Row + k
			if dst == src {
				continue
			}
			for col := r.From.Col; col <= r.To.Col; col++ {
				to := Addr{Col: col, Row: dst}
				switch c := old[Addr{Col: col, Row: src}]; {
				case c != nil:
					s.place(to, c.rewritten(formula.Shift(0, dst-src)))
				case s.cells.get(to) != nil:
					s.place(to, nil)
				}
			}
		}
	})
}
