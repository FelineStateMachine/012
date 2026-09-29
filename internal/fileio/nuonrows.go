package fileio

import (
	"bytes"
	"context"
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
