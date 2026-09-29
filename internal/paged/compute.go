package paged

import (
	"fmt"

	"github.com/FelineStateMachine/012/internal/functions"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// rangeValues is a range of a source read whole, as an array: what an
// operator or a function reading it some other way is given, no bigger
// than budget cells.
func (b *book) rangeValues(name string, r sheet.Rect, budget int) sheet.SourceAnswer {
	s, ok := b.src(name)
	if !ok {
		return sheet.SourceAnswer{V: sheet.ErrRef}
	}
	part, ok := s.clip(r)
	if !ok {
		return sheet.SourceAnswer{A: &functions.Array{Rows: 1, Cols: 1}}
	}
	rows, cols := part.To.Row-part.From.Row+1, part.To.Col-part.From.Col+1
	if rows > budget/cols {
		return sheet.SourceAnswer{V: sheet.ErrValue, Why: fmt.Sprintf(
			"Reading %s whole takes %s cells, more than max-cells (%s): SUM, COUNTIFS, XLOOKUP and the like read a source of any size",
			s.name, grouped(rows*cols), grouped(budget))}
	}
	a := functions.NewArray(rows, cols)
	addrs, vals := make([]sheet.Addr, 1024), make([]sheet.Value, 1024)
	for from := part.From; ; {
		n := b.Scan(name, part, from, addrs, vals)
		for i := range n {
			a.Set(addrs[i].Row-part.From.Row, addrs[i].Col-part.From.Col, vals[i])
		}
		if n < len(addrs) && (n == 0 || vals[n-1].Kind != sheet.Error) {
			break
		}
		from = after(part, addrs[n-1])
	}
	return sheet.SourceAnswer{A: a}
}

// after is the cell after a in r, row by row.
func after(r sheet.Rect, a sheet.Addr) sheet.Addr {
	if a.Col < r.To.Col {
		return sheet.Addr{Col: a.Col + 1, Row: a.Row}
	}
	return sheet.Addr{Col: r.From.Col, Row: a.Row + 1}
}

// pivot gathers a pivot table's groups over the source, streaming the
// columns it reads.
func (b *book) pivot(name string, p sheet.Pivot) sheet.SourceAnswer {
	s, ok := b.src(name)
	if !ok {
		return sheet.SourceAnswer{V: sheet.ErrRef}
	}
	cols := sheet.PivotCols(p)
	width := len(s.shape.Cols)
	for _, c := range cols {
		if c >= width {
			return sheet.SourceAnswer{V: sheet.ErrRef, Why: fmt.Sprintf("%s has no column %s", s.name, sheet.ColName(c))}
		}
	}
	row := make([]sheet.LiveCell, width)
	g, err := sheet.GatherPivot(p, s.shape, func(each func(int, []sheet.LiveCell) bool) error {
		return s.h.Scan(b.ctx, 0, cols, func(n int64, vals []sheet.LiveCell) bool {
			for i, c := range cols {
				row[c] = vals[i]
			}
			return each(int(n), row)
		})
	})
	if err != nil {
		b.fail(err)
		return sheet.SourceAnswer{V: sheet.ErrRef}
	}
	return sheet.SourceAnswer{Pivot: g}
}

// grouped is n with thousands separators: 12,000,000.
func grouped(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
