package fileio

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"

	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// NUONRows reads NUON, a notebook cell's output, as a region's rows: a
// table's header and rows, a record as one row, a list of values that
// aren't records as one column named value, and any other value as that
// column with one row. It keeps at most maxCells cells, whole rows (0
// for the max-cells setting), and says what it left out.
func NUONRows(ctx context.Context, data []byte, maxCells int) (TailRows, string, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return TailRows{}, "", nil
	}
	if data[0] != '[' && data[0] != '{' {
		data = append(append([]byte("["), data...), ']')
	}
	res, err := ImportReader(ctx, "nu", bytes.NewReader(data), Options{MaxCells: maxCells})
	if err != nil {
		return TailRows{}, "", err
	}
	return Rows(res.Sheet), strings.Join(res.Notes, "; "), nil
}

// NUONCell is the cell a NUON value makes: its value, text as the
// value's own, and its format (a file size's Size, a date's Date time,
// in the local time zone), as importing it would store it.
func NUONCell(v nuon.Value) sheet.LiveCell { return nuonCell(v, zone()) }

// AppendTable reads data, a NUON table, into s as more rows of the table
// importing a table put there: under its row last, each value in the
// column its header row (row 0, cols columns) names, a column named for
// the first time added after the others with its name in the header,
// the cells as importing stores them. It leaves the widths alone and
// recalculates nothing, the cells being values, and returns the rows
// and the columns the table has now.
func AppendTable(ctx context.Context, s *sheet.Sheet, last, cols int, data []byte) (rows, allCols int, err error) {
	b := &builder{s: s, maxCells: -1}
	t := tableCells{b: b, cols: make(map[string]int, cols), zone: zone(), dated: map[int]bool{}, fixed: true}
	for c := range cols {
		t.cols[s.ShownText(sheet.Addr{Col: c})] = c
	}
	r := nuon.NewReader(bytes.NewReader(data))
	for ; ; rows++ {
		if rows%256 == 0 {
			if err := ctx.Err(); err != nil {
				return rows, len(t.cols), err
			}
		}
		fields, err := r.Next()
		if errors.Is(err, io.EOF) {
			return rows, len(t.cols), nil
		}
		if err != nil {
			return rows, len(t.cols), err
		}
		t.header(r.Header())
		t.row(last+rows+1, fields)
	}
}
