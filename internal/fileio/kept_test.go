package fileio

import (
	"context"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A notebook's output takes the formats of the columns it read, where
// its values are still of their types.
func TestKeepFormats(t *testing.T) {
	w := sheet.NewBook()
	s := w.Sheet(0)
	for a, in := range map[string]string{"A1": "price", "B1": "bought", "C1": "n", "A2": "$3.50", "B2": "2026-09-29", "C2": "12%"} {
		at, _ := sheet.ParseAddr(a)
		s.Set(at, in)
	}
	formats := ColumnFormats(Snap(s, sheet.NewRect(sheet.Addr{}, sheet.Addr{Col: 2, Row: 1}), "Sheet1"))
	rows, _, err := NUONRows(context.Background(), []byte(`[[price, bought, n, other]; [4.25, 2026-10-01T00:00:00+00:00, "text", 7]]`), 0)
	if err != nil {
		t.Fatal(err)
	}
	KeepFormats(&rows, formats)
	row := rows.Rows[0]
	if row[0].F.Kind != sheet.FmtCurrency || row[1].F.Pattern != "yyyy-mm-dd" || row[2].V.Kind != sheet.Text || !row[3].F.IsZero() {
		t.Errorf("kept: %+v", row)
	}
}
