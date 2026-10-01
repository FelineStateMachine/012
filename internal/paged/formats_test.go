package paged

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// priced is a row of a Parquet file whose types carry formats: a price
// in DECIMAL(12,2), a day and a moment.
type priced struct {
	ID    int64 `parquet:"id"`
	Price int64 `parquet:"price,decimal(2:12)"`
	Day   int32 `parquet:"day,date"`
	At    int64 `parquet:"at,timestamp(millisecond)"`
}

func writePriced(t *testing.T, name string, n int) {
	t.Helper()
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := parquet.NewGenericWriter[priced](f, parquet.MaxRowsPerRowGroup(64))
	day := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	rows := make([]priced, n)
	for i := range rows {
		rows[i] = priced{ID: int64(i), Price: int64(i*7919%10000) + 100, Day: int32(day.Unix()/86400) + int32(i%90),
			At: day.Add(time.Duration(i) * time.Hour).UnixMilli()}
	}
	if _, err := w.Write(rows); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

// TestFormatsOverASource checks that a formula over a source's column
// shows in the format its type gives, as the same formula over an
// imported copy does, and that its counts show as whole numbers.
func TestFormatsOverASource(t *testing.T) {
	name := filepath.Join(t.TempDir(), "sales.parquet")
	writePriced(t, name, 3000)
	w, h := linked(t, name, 100000)
	s := w.Sheet(0)
	price := sheet.Format{Kind: sheet.FmtNumber, Decimals: 2}
	cases := []struct {
		formula string
		want    sheet.Format
	}{
		{"=SUM({T}[price])", price}, {"=AVERAGE({T}[price])", price}, {"=MIN({T}[price])", price},
		{"=MAX({T}[price])", price}, {"=MEDIAN({T}[price])", price}, {"=SUM({T}[price])*2", price},
		{"=MAX({T}[day])", sheet.Format{Kind: sheet.FmtDate}}, {"=MIN({T}[at])", sheet.Format{Kind: sheet.FmtDateTime}},
		{"=XLOOKUP(5,{T}[id],{T}[price])", price}, {"=SUM({T}[id])", sheet.Format{}},
	}
	for i, c := range cases {
		s.Set(sheet.Addr{Col: 10, Row: i}, strings.ReplaceAll(c.formula, "{T}", "sales"))
		s.Set(sheet.Addr{Col: 11, Row: i}, strings.ReplaceAll(c.formula, "{T}", "ref"))
	}
	counts := []string{"=ROWS(sales)", "=COUNT(sales[price])", "=COUNTA(sales[id])", `=COUNTIF(sales[price],">50")`,
		`=COUNTIFS(sales[id],">5",sales[day],">0")`, "=COUNTBLANK(sales[id])", "=COLUMNS(sales)"}
	for i, f := range counts {
		s.Set(sheet.Addr{Col: 12, Row: i}, f)
	}
	s.Set(sheet.Addr{Col: 13}, "=ROWS(ref)")
	h.Settle(context.Background(), w, Links(w, same, ""))
	for i, c := range cases {
		got, ref := s.DisplayFormat(sheet.Addr{Col: 10, Row: i}), s.DisplayFormat(sheet.Addr{Col: 11, Row: i})
		if got != c.want || ref != c.want {
			t.Errorf("%s: %+v over the source, %+v over the copy, want %+v", c.formula, got, ref, c.want)
		}
		if g, r := s.ShownText(sheet.Addr{Col: 10, Row: i}), s.ShownText(sheet.Addr{Col: 11, Row: i}); g != r {
			t.Errorf("%s shows %q over the source, %q over the copy", c.formula, g, r)
		}
	}
	for i, f := range counts {
		a := sheet.Addr{Col: 12, Row: i}
		if got := s.DisplayFormat(a); got != (sheet.Format{Kind: sheet.FmtNumber}) {
			t.Errorf("%s: %+v, want a whole number", f, got)
		}
	}
	if got := s.ShownText(sheet.Addr{Col: 12}); got != "3,000" {
		t.Errorf("ROWS shows %q", got)
	}
	if got := s.DisplayFormat(sheet.Addr{Col: 13}); !got.IsZero() {
		t.Errorf("a count over a sheet shows in %+v", got)
	}
}
