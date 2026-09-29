package fileio

import (
	"bufio"
	"context"
	"errors"
	"os"
	"slices"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Views of a Parquet file stream: building one reads the columns it
// sorts and filters by once, writing the rows it keeps, in its order,
// to a file of row numbers (sourcesort.go); a page reads its row
// numbers there and fetches those rows.

// View orders the file's rows as o says.
func (s *parquetSource) View(ctx context.Context, o SourceOrder) (SourceView, error) {
	if o.IsZero() {
		return pqPlainView{s}, nil
	}
	plan, err := newViewPlan(o, len(s.cols))
	if err != nil {
		return nil, err
	}
	f, err := newRowFile(s.dir)
	if err != nil {
		return nil, err
	}
	rf := &rowFile{f: f}
	if rf.n, err = plan.write(ctx, s, f); err != nil {
		rf.close()
		return nil, err
	}
	s.mu.Lock()
	s.views = append(s.views, rf)
	s.mu.Unlock()
	return &pqView{s: s, rf: rf}, nil
}

// viewPlan is what building a streamed view reads: the columns it sorts
// and filters by, once each, and where each key and test finds its
// column among them.
type viewPlan struct {
	cols  []int
	keys  []int // positions in cols of the sort's columns
	desc  []bool
	tests []viewTest
}

type viewTest struct {
	at   int // the column's position in cols
	pass func(sheet.Value, string) bool
}

func newViewPlan(o SourceOrder, ncols int) (*viewPlan, error) {
	p := &viewPlan{}
	at := func(col int) (int, error) {
		if col < 0 || col >= ncols {
			return 0, errors.New("the source has no such column")
		}
		if i := slices.Index(p.cols, col); i >= 0 {
			return i, nil
		}
		p.cols = append(p.cols, col)
		return len(p.cols) - 1, nil
	}
	for _, k := range o.Sort {
		i, err := at(k.Col)
		if err != nil {
			return nil, err
		}
		p.keys, p.desc = append(p.keys, i), append(p.desc, k.Desc)
	}
	for _, f := range o.Filter {
		i, err := at(f.Col)
		if err != nil {
			return nil, err
		}
		p.tests = append(p.tests, viewTest{at: i, pass: f.Cond.Test()})
	}
	return p, nil
}

// passes reports whether a row's values pass every test, each on the
// value and the text it shows, as a sheet's filter tests them.
func (p *viewPlan) passes(vals []sheet.LiveCell) bool {
	for _, t := range p.tests {
		c := vals[t.at]
		if !t.pass(c.V, sheet.FormatText(c.V, c.F)) {
			return false
		}
	}
	return true
}

// write streams the source, writing the numbers of the rows the view
// keeps to f in its order, and returns how many.
func (p *viewPlan) write(ctx context.Context, s *parquetSource, f *os.File) (int64, error) {
	w := bufio.NewWriterSize(f, 1<<20)
	var n int64
	var sorter *extSorter
	if len(p.keys) > 0 {
		sorter = newExtSorter(s.dir, p.desc)
		defer sorter.remove()
	}
	keys := make([]sheet.LiveCell, len(p.keys))
	var ferr error
	err := s.Scan(ctx, 0, p.cols, func(row int64, vals []sheet.LiveCell) bool {
		if !p.passes(vals) {
			return true
		}
		if sorter == nil {
			ferr = writeRow(w, row)
			n++
			return ferr == nil
		}
		for i, k := range p.keys {
			keys[i] = vals[k]
		}
		ferr = sorter.add(row, keys)
		return ferr == nil
	})
	if err = errors.Join(err, ferr); err != nil {
		return 0, err
	}
	if sorter != nil {
		if n, err = sorter.finish(w); err != nil {
			return 0, err
		}
	}
	return n, w.Flush()
}

// pqView is a sorted or filtered view of a Parquet file.
type pqView struct {
	s  *parquetSource
	rf *rowFile
}

func (v *pqView) Rows() int64 { return v.rf.n }

func (v *pqView) Page(ctx context.Context, from int64, n int) ([]int64, [][]sheet.LiveCell, error) {
	nums, err := v.rf.read(from, n)
	if err != nil || len(nums) == 0 {
		return nil, nil, err
	}
	rows, err := v.s.Fetch(ctx, nums, nil)
	if err != nil {
		return nil, nil, err
	}
	return nums, rows, nil
}

func (v *pqView) Close() error {
	v.s.mu.Lock()
	i := slices.Index(v.s.views, v.rf)
	if i < 0 {
		v.s.mu.Unlock()
		return nil // closed with the source
	}
	v.s.views = slices.Delete(v.s.views, i, i+1)
	v.s.mu.Unlock()
	return v.rf.close()
}

// pqPlainView is the file's rows in its own order.
type pqPlainView struct{ s *parquetSource }

func (v pqPlainView) Rows() int64 { return v.s.Rows() }

func (v pqPlainView) Page(ctx context.Context, from int64, n int) ([]int64, [][]sheet.LiveCell, error) {
	return scanPage(ctx, v.s, from, n)
}

func (pqPlainView) Close() error { return nil }

// scanPage reads n rows from row from of a source in its own order.
func scanPage(ctx context.Context, s Source, from int64, n int) ([]int64, [][]sheet.LiveCell, error) {
	if from < 0 || from >= s.Rows() || n <= 0 {
		return nil, nil, nil
	}
	var nums []int64
	var rows [][]sheet.LiveCell
	err := s.Scan(ctx, from, nil, func(row int64, vals []sheet.LiveCell) bool {
		nums, rows = append(nums, row), append(rows, slices.Clone(vals))
		return len(nums) < n
	})
	return nums, rows, err
}
