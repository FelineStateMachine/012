package paged

import (
	"cmp"
	"fmt"

	"github.com/FelineStateMachine/012/internal/fileio"
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
			"Raise max-cells to %s to read %s whole. It is %s; SUM, COUNTIFS, XLOOKUP and the like read a source of any size.",
			grouped(rows*cols), s.name, grouped(budget))}
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

// SortedNums sorts the numbers of r on disk (functions.Sorter),
// streaming the source's rows: what isn't a number is skipped, as SUM
// skips it in a range, and the first error is the answer.
func (b *book) SortedNums(name string, r sheet.Rect) (functions.Sorted, *sheet.Value) {
	s, ok := b.src(name)
	if !ok {
		return nil, refErr()
	}
	ns := fileio.NewNumberSort(s.dir)
	part, ok := s.clip(r)
	if ok && part.From.Row == 0 { // the header: names, not numbers
		part.From.Row = 1
	}
	if ok && part.From.Row <= part.To.Row {
		if e := b.sortRows(s, part, ns); e != nil {
			ns.Close()
			return nil, e
		}
	}
	sorted, err := ns.Sorted()
	if err != nil {
		b.fail(err)
		return nil, refErr()
	}
	return sorted, nil
}

// sortRows adds the numbers of the rows of part to ns, numbered in the
// order they're read, returning the first error among them, or #REF!
// when reading or spilling failed.
func (b *book) sortRows(s snapshot, part sheet.Rect, ns *fileio.NumberSort) *sheet.Value {
	cols := make([]int, 0, part.To.Col-part.From.Col+1)
	for c := part.From.Col; c <= part.To.Col; c++ {
		cols = append(cols, c)
	}
	last := int64(part.To.Row - 1)
	var e *sheet.Value
	var pos int64
	var failed error
	err := s.h.Scan(b.ctx, int64(part.From.Row-1), cols, func(row int64, vals []sheet.LiveCell) bool {
		for _, v := range vals {
			switch v.V.Kind {
			case sheet.Error:
				e = &v.V
				return false
			case sheet.Number:
				if failed = ns.Add(v.V.Num, pos); failed != nil {
					return false
				}
			}
			pos++
		}
		return row < last
	})
	if err = cmp.Or(err, failed); err != nil {
		b.fail(err)
		return refErr()
	}
	return e
}

// refErr is a #REF! of its own, for an answer to point at.
func refErr() *sheet.Value {
	v := sheet.ErrRef
	return &v
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
