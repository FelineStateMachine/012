package fileio

import (
	"context"
	"errors"
	"io"
	"os"
	"slices"
	"strings"
	"sync"

	"github.com/parquet-go/parquet-go"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// parquetSource is a Parquet file read in place. Each read opens its
// own readers of the column chunks it needs (the file is read with
// ReadAt), so reads on several goroutines don't meet.
type parquetSource struct {
	f      *os.File
	pf     *parquet.File
	groups []parquet.RowGroup
	starts []int64 // each row group's first row, and the rows in all last
	cols   []parquetColumn
	meta   []SourceColumn
	dir    string

	mu    sync.Mutex
	views []*rowFile // what views wrote, removed by Close
}

func openParquetSource(spec SourceSpec) (*parquetSource, error) {
	f, err := os.Open(spec.Path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	pf, err := parquet.OpenFile(f, st.Size())
	if err != nil {
		f.Close()
		return nil, err
	}
	s := &parquetSource{f: f, pf: pf, groups: pf.RowGroups(), dir: spec.TempDir}
	s.starts = make([]int64, len(s.groups)+1)
	for i, g := range s.groups {
		s.starts[i+1] = s.starts[i] + g.NumRows()
	}
	schema := pf.Schema()
	for _, path := range schema.Columns() {
		var c parquetColumn
		if leaf, ok := schema.Lookup(path...); ok && leaf.Node != nil {
			c = parquetColumnOf(leaf.Node.Type())
			c.repeated = leaf.MaxRepetitionLevel > 0
		}
		f, num := parquetFormat(c)
		s.cols = append(s.cols, c)
		s.meta = append(s.meta, SourceColumn{Name: strings.Join(path, "."), Format: f, Numeric: num})
	}
	return s, nil
}

func (s *parquetSource) Columns() []SourceColumn { return slices.Clone(s.meta) }

func (s *parquetSource) Rows() int64 { return s.starts[len(s.starts)-1] }

func (s *parquetSource) Close() error {
	s.mu.Lock()
	views := s.views
	s.views = nil
	s.mu.Unlock()
	var errs []error
	for _, v := range views {
		errs = append(errs, v.close())
	}
	return errors.Join(append(errs, s.f.Close())...)
}

// group is the row group holding row.
func (s *parquetSource) group(row int64) int {
	i, found := slices.BinarySearch(s.starts, row)
	if !found {
		i--
	}
	return i
}

// Scan streams rows from row from on.
func (s *parquetSource) Scan(ctx context.Context, from int64, cols []int, fn func(int64, []sheet.LiveCell) bool) error {
	cols = allCols(cols, len(s.cols))
	if err := checkCols(cols, len(s.cols)); err != nil {
		return err
	}
	rs := s.newRows(cols)
	defer rs.close()
	vals := make([]sheet.LiveCell, len(cols))
	for row := max(from, 0); row < s.Rows(); row++ {
		if row%1024 == 0 {
			if err := ctx.Err(); err != nil {
				return err
			}
		}
		if err := rs.read(row, vals); err != nil {
			return err
		}
		if !fn(row, vals) {
			return nil
		}
	}
	return nil
}

// Fetch reads the rows numbered rows.
func (s *parquetSource) Fetch(ctx context.Context, rows []int64, cols []int) ([][]sheet.LiveCell, error) {
	cols = allCols(cols, len(s.cols))
	if err := checkCols(cols, len(s.cols)); err != nil {
		return nil, err
	}
	rs := s.newRows(cols)
	defer rs.close()
	out := make([][]sheet.LiveCell, len(rows))
	for _, i := range fetchOrder(rows) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		row := rows[i]
		if row < 0 || row >= s.Rows() {
			return nil, errors.New("no such row in the source")
		}
		vals := make([]sheet.LiveCell, len(cols))
		if err := rs.read(row, vals); err != nil {
			return nil, err
		}
		out[i] = vals
	}
	return out, nil
}

// pqRows reads rows of some columns, each through a cursor of its own.
type pqRows struct {
	s    *parquetSource
	curs []*pqCursor
	buf  []parquet.Value
}

func (s *parquetSource) newRows(cols []int) *pqRows {
	rs := &pqRows{s: s}
	for _, c := range cols {
		rs.curs = append(rs.curs, &pqCursor{s: s, col: c, repeated: s.cols[c].repeated, group: -1})
	}
	return rs
}

// read reads row's values into vals.
func (rs *pqRows) read(row int64, vals []sheet.LiveCell) error {
	for i, c := range rs.curs {
		var err error
		if rs.buf, err = c.row(row, rs.buf[:0]); err != nil {
			return err
		}
		cell, _ := parquetCells(rs.buf, rs.s.cols[c.col])
		vals[i] = cell.typed()
	}
	return nil
}

func (rs *pqRows) close() {
	for _, c := range rs.curs {
		c.close()
	}
}

// pqCursor reads one column's values a row at a time: from the page
// readers of its chunk in the row group being read, sought to a row
// when a read isn't the next one.
type pqCursor struct {
	s        *parquetSource
	col      int
	repeated bool
	group    int   // the row group being read, -1 before any
	next     int64 // the row the next value starts
	pages    parquet.Pages
	vals     parquet.ValueReader
	buf      []parquet.Value
	i, n     int
}

// skipRows is how far ahead a cursor reads its way rather than seeking.
const skipRows = 4096

// row appends the values of row to out: one, or a repeated column's
// several (none for an empty list, read as null).
func (c *pqCursor) row(row int64, out []parquet.Value) ([]parquet.Value, error) {
	if err := c.seek(row); err != nil {
		return out, err
	}
	for first := true; ; first = false {
		if c.i == c.n {
			if err := c.fill(); err != nil {
				if errors.Is(err, io.EOF) && !first {
					break
				}
				return out, err
			}
		}
		v := c.buf[c.i]
		if !first && v.RepetitionLevel() == 0 {
			break
		}
		if c.repeated {
			v = v.Clone() // the next fill may reuse what it points at
		}
		out = append(out, v)
		c.i++
	}
	c.next = row + 1
	return out, nil
}

// seek places the cursor at row: reading forward to it when it is a
// little ahead in the same group, else seeking the group's pages.
func (c *pqCursor) seek(row int64) error {
	if row == c.next && c.group >= 0 && row < c.s.starts[c.group+1] {
		return nil // the next row, as a scan reads them
	}
	g := c.s.group(row)
	if g == c.group && row >= c.next && row-c.next <= skipRows {
		var skip []parquet.Value
		for c.next < row {
			var err error
			if skip, err = c.row(c.next, skip[:0]); err != nil {
				return err
			}
		}
		return nil
	}
	c.close()
	pages := c.s.groups[g].ColumnChunks()[c.col].Pages()
	if err := pages.SeekToRow(row - c.s.starts[g]); err != nil {
		pages.Close()
		return err
	}
	c.pages, c.group, c.next, c.vals, c.i, c.n = pages, g, row, nil, 0, 0
	return nil
}

// fill reads the next values of the group's pages, io.EOF past them.
func (c *pqCursor) fill() error {
	if c.buf == nil {
		c.buf = make([]parquet.Value, 1024)
	}
	for {
		if c.vals != nil {
			n, err := c.vals.ReadValues(c.buf)
			if n > 0 {
				c.i, c.n = 0, n
				return nil
			}
			if err != nil && !errors.Is(err, io.EOF) {
				return err
			}
			c.vals = nil
		}
		if c.pages == nil {
			return io.EOF
		}
		p, err := c.pages.ReadPage()
		if err != nil {
			return err
		}
		c.vals = p.Values()
	}
}

func (c *pqCursor) close() {
	if c.pages != nil {
		c.pages.Close()
	}
	c.pages, c.vals, c.group, c.i, c.n = nil, nil, -1, 0, 0
}
