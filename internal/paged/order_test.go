package paged

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// orderFormulas are order statistics over the source as {T}, each the
// same as over the table ref holding the same rows, though the source
// holds more cells than the budget: its column is sorted on disk.
var orderFormulas = []string{
	"=MEDIAN({T}[amount])", "=MEDIAN({T}[id])", "=MEDIAN({T}[day])", "=MEDIAN({T}[cat])",
	"=MODE({T}[amount])", "=MODE.SNGL({T}[day])", "=MODE({T}[id])",
	"=PERCENTILE({T}[amount],0.3)", "=PERCENTILE.INC({T}[amount],0.999)", "=PERCENTILE({T}[amount],2)",
	"=PERCENTILE.EXC({T}[amount],0.1)", "=PERCENTILE.EXC({T}[amount],0.0001)",
	"=QUARTILE({T}[amount],1)", "=QUARTILE({T}[amount],3)", "=QUARTILE.INC({T}[id],4)",
	"=QUARTILE.EXC({T}[amount],2)", "=QUARTILE.EXC({T}[amount],4)",
	"=LARGE({T}[amount],10)", "=SMALL({T}[amount],1)", "=LARGE({T}[id],1)", "=SMALL({T}[amount],100000)",
	"=RANK(50.5,{T}[amount])", "=RANK.EQ(17,{T}[amount],1)", "=RANK(0.25,{T}[amount])",
	"=MEDIAN({T}[[id]:[amount]])",
}

// checkSame sets each of formulas over the source sales and over the
// table ref, and checks the two agree.
func checkSame(t *testing.T, w *sheet.Workbook, h *Host, formulas []string) {
	t.Helper()
	s := w.Sheet(0)
	for i, f := range formulas {
		s.Set(sheet.Addr{Col: 10, Row: i}, strings.ReplaceAll(f, "{T}", "sales"))
		s.Set(sheet.Addr{Col: 11, Row: i}, strings.ReplaceAll(f, "{T}", "ref"))
	}
	h.Settle(context.Background(), w, nil)
	for i, f := range formulas {
		got, want := s.Value(sheet.Addr{Col: 10, Row: i}), s.Value(sheet.Addr{Col: 11, Row: i})
		if testing.Verbose() {
			t.Logf("%s: %v", f, got)
		}
		if got != want {
			t.Errorf("%s: %v over the source, %v over the sheet (%s)", f, got, want, s.ExplainError(sheet.Addr{Col: 10, Row: i}))
		}
	}
}

func TestOrderOverAParquetSource(t *testing.T) {
	name := filepath.Join(t.TempDir(), "sales.parquet")
	writeSales(t, name, 5000, 0)
	w, h := linked(t, name, 1000)
	checkSame(t, w, h, orderFormulas)
}

func TestOrderOverASQLiteSource(t *testing.T) {
	name := filepath.Join(t.TempDir(), "sales.sqlite")
	db, err := sql.Open("sqlite", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("CREATE TABLE sales (id INTEGER, cat TEXT, amount REAL, day DATE)"); err != nil {
		t.Fatal(err)
	}
	for i := range 3000 {
		var amount any
		if i%11 != 7 {
			amount = float64((i*37)%101) - 20.5
		}
		_, err := db.Exec("INSERT INTO sales VALUES (?, ?, ?, ?)", i, cats[i%len(cats)], amount,
			"2026-01-"+string(rune('1'+i%9))+"0")
		if err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	w, h := linked(t, name, 1000)
	checkSame(t, w, h, orderFormulas)
}

// TestOrderAnswersKept checks an order statistic is answered once, then
// kept as the other answers are.
func TestOrderAnswersKept(t *testing.T) {
	name := filepath.Join(t.TempDir(), "sales.parquet")
	writeSales(t, name, 500, 0)
	w, h := linked(t, name, 100)
	s := w.Sheet(0)
	s.Set(sheet.Addr{Col: 10}, "=MEDIAN(sales[amount])")
	h.Settle(context.Background(), w, nil)
	s.Set(sheet.Addr{Col: 10, Row: 1}, "=MEDIAN(sales[amount])+1")
	if jobs := h.Jobs(); len(jobs) != 0 {
		t.Errorf("asked again: %v", jobs)
	}
	if v := s.Value(sheet.Addr{Col: 10, Row: 1}); v.Kind != sheet.Number {
		t.Errorf("the kept answer: %v", v)
	}
}
