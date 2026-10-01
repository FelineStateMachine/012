package paged

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// sale is a row of the Parquet files the tests link.
type sale struct {
	ID     int64    `parquet:"id"`
	Cat    *string  `parquet:"cat,optional"`
	Amount *float64 `parquet:"amount,optional"`
	Day    int32    `parquet:"day,date"`
	Tags   []string `parquet:"tags,list"`
}

var cats = []string{"North", "south", "East", "west", "", "north"}

// writeSales writes n rows to name, amounts shifted by shift, in row
// groups of 64 rows.
func writeSales(t testing.TB, name string, n int, shift float64) {
	t.Helper()
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	w := parquet.NewGenericWriter[sale](f, parquet.MaxRowsPerRowGroup(64))
	day := int32(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix() / 86400)
	rows := make([]sale, n)
	for i := range rows {
		r := &rows[i]
		r.ID, r.Day = int64(i), day+int32(i%40)
		if i%9 != 4 {
			c := cats[i%len(cats)]
			r.Cat = &c
		}
		if i%11 != 7 {
			a := float64((i*37)%101) - 20.5 + shift
			r.Amount = &a
		}
		if i%5 == 0 {
			r.Tags = []string{"a", "b"}
		}
	}
	if _, err := w.Write(rows); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

// linked is a workbook holding the file at name imported on Sheet1, as
// the table ref, and linked as the source sales, settled.
func linked(t *testing.T, name string, budget int) (*sheet.Workbook, *Host) {
	t.Helper()
	res, err := fileio.Import(context.Background(), name, fileio.Options{})
	if err != nil {
		t.Fatal(err)
	}
	w := res.Sheet.Book()
	s := w.Sheet(0)
	if err := w.RenameSheet(s, "Data"); err != nil {
		t.Fatal(err)
	}
	b, _ := s.UsedRange()
	if err := s.CreateTable("ref", b); err != nil {
		t.Fatal(err)
	}
	if _, err := w.AddSource("sales", sheet.LinkSource{Path: name}, s); err != nil {
		t.Fatal(err)
	}
	h := NewHost(budget)
	h.Settle(context.Background(), w, Links(w, same, ""))
	return w, h
}

// sameFormulas are formulas that read the source as {T}: each gives the
// same over the source as over the table ref holding the same rows.
var sameFormulas = []string{
	"=SUM({T}[amount])", "=AVERAGE({T}[amount])", "=COUNT({T}[amount])", "=COUNTA({T}[cat])",
	"=MIN({T}[amount])", "=MAX({T}[amount])", "=COUNTA({T}[tags])",
	`=COUNTIF({T}[cat],"north")`, `=COUNTIF({T}[amount],">10")`, `=COUNTIFS({T}[cat],"North",{T}[amount],"<0")`,
	`=SUMIF({T}[cat],"East",{T}[amount])`, `=SUMIFS({T}[amount],{T}[cat],"west",{T}[id],">100")`,
	`=AVERAGEIF({T}[cat],"south",{T}[amount])`, `=AVERAGEIFS({T}[amount],{T}[cat],"<>East")`,
	"=COUNTBLANK({T}[cat])", "=SUMPRODUCT({T}[amount],{T}[id])", `=COUNTIF({T}[day],">="&DATE(2026,1,20))`,
	"=MATCH(17,{T}[id],0)", "=MATCH(50.5,{T}[amount],0)", "=XLOOKUP(42,{T}[id],{T}[amount])",
	`=XLOOKUP("east",{T}[cat],{T}[id],"none",0,-1)`, "=XLOOKUP(10,{T}[amount],{T}[id],,1)",
	"=XLOOKUP(10,{T}[amount],{T}[id],,-1,-1)", `=XLOOKUP("nowhere",{T}[cat],{T}[id],"none")`,
	"=VLOOKUP(7,{T},3,FALSE)", "=INDEX({T}[amount],10)", "=ROWS({T})", "=COLUMNS({T}[#All])",
	"=MEDIAN({T}[amount])", "=SUM({T}[amount]*2)", "=ROUND(SUM({T}[amount]),2)",
}

func TestFormulasOverASource(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "sales.parquet")
	writeSales(t, name, 5000, 0)
	w, h := linked(t, name, 100000)
	s := w.Sheet(0)
	for i, f := range sameFormulas {
		at := sheet.Addr{Col: 10, Row: i}
		ref := sheet.Addr{Col: 11, Row: i}
		if err := s.Set(at, strings.ReplaceAll(f, "{T}", "sales")); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if err := s.Set(ref, strings.ReplaceAll(f, "{T}", "ref")); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
	}
	h.Settle(context.Background(), w, Links(w, same, ""))
	for i, f := range sameFormulas {
		got, want := s.Value(sheet.Addr{Col: 10, Row: i}), s.Value(sheet.Addr{Col: 11, Row: i})
		if testing.Verbose() {
			t.Logf("%s: %v", f, got)
		}
		if got.Kind == sheet.Error && !strings.Contains(f, "nowhere") {
			t.Errorf("%s: %v", f, got)
		}
		if got != want {
			t.Errorf("%s: %v over the source, %v over the sheet", f, got, want)
		}
	}
}

func TestPendingUntilAnswered(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "sales.parquet")
	writeSales(t, name, 100, 0)
	w, h := linked(t, name, 1000)
	s := w.Sheet(0)
	a := sheet.Addr{Col: 10}
	if err := s.Set(a, "=SUM(sales[amount])"); err != nil {
		t.Fatal(err)
	}
	if v := s.Value(a); !sheet.IsPending(v) {
		t.Fatalf("before the answer: %v, want Loading…", v)
	}
	jobs := h.Jobs()
	if len(jobs) != 1 {
		t.Fatalf("%d jobs queued, want 1", len(jobs))
	}
	jobs[0].Run(context.Background())
	Apply(w, h.Store(jobs[0]))
	if v := s.Value(a); v.Kind != sheet.Number {
		t.Fatalf("after the answer: %v", v)
	}
	// Asked again, the answer is kept: no job.
	if err := s.Set(sheet.Addr{Col: 11}, "=SUM(sales[amount])+1"); err != nil {
		t.Fatal(err)
	}
	if jobs := h.Jobs(); len(jobs) != 0 {
		t.Errorf("%d jobs for a kept answer", len(jobs))
	}
}

func TestPastTheBudget(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "sales.parquet")
	writeSales(t, name, 300, 0)
	w, h := linked(t, name, 100)
	s := w.Sheet(0)
	med, sum := sheet.Addr{Col: 10}, sheet.Addr{Col: 10, Row: 1}
	s.Set(med, "=STDEV(sales[amount])")
	s.Set(sum, "=SUM(sales[amount])")
	h.Settle(context.Background(), w, Links(w, same, ""))
	if v := s.Value(med); v != sheet.ErrValue {
		t.Errorf("STDEV past the budget: %v, want #VALUE!", v)
	}
	if why := s.ExplainError(med); !strings.Contains(why, "max-cells") {
		t.Errorf("STDEV past the budget explained as %q", why)
	}
	if v := s.Value(sum); v.Kind != sheet.Number {
		t.Errorf("SUM streams past the budget: %v", v)
	}
}

func TestPivotOverASource(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "sales.parquet")
	writeSales(t, name, 500, 0)
	w, h := linked(t, name, 1000)
	ref := w.Sheet(0)
	src, _ := w.LookupSource("sales")
	p := sheet.Pivot{
		Rows:      []sheet.PivotGroup{{Col: 1}},
		Columns:   []sheet.PivotGroup{{Col: 3}},
		Values:    []sheet.PivotValue{{Col: 2, Summarize: sheet.SumBy}, {Col: 2, Summarize: sheet.CountUniqueBy}},
		RowTotals: true, ColumnTotals: true,
	}
	b, _ := ref.UsedRange()
	want, err := w.CreatePivot(ref, b, "want", p)
	if err != nil {
		t.Fatal(err)
	}
	got, err := w.CreatePivot(src.Sheet, sheet.Rect{To: sheet.Addr{Col: 4, Row: sheet.MaxRows - 1}}, "got", p)
	if err != nil {
		t.Fatal(err)
	}
	h.Settle(context.Background(), w, Links(w, same, ""))
	wb, _ := want.UsedRange()
	gb, _ := got.UsedRange()
	if wb != gb {
		t.Fatalf("pivot over the source covers %v, over the sheet %v", gb, wb)
	}
	for row := wb.From.Row; row <= wb.To.Row; row++ {
		for col := wb.From.Col; col <= wb.To.Col; col++ {
			a := sheet.Addr{Col: col, Row: row}
			if g, x := got.Value(a), want.Value(a); g != x && !(col == 0 && row == 0) {
				t.Errorf("%s: %v over the source, %v over the sheet", a, g, x)
			}
		}
	}
}

func TestSourceChanges(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "sales.parquet")
	writeSales(t, name, 100, 0)
	w, h := linked(t, name, 1000)
	s := w.Sheet(0)
	a := sheet.Addr{Col: 10}
	s.Set(a, "=SUM(sales[amount])")
	h.Settle(context.Background(), w, Links(w, same, ""))
	before := s.Value(a)
	now := time.Now()
	h.Now = func() time.Time { return now }
	writeSales(t, name, 100, 1)
	if h.Poll() {
		t.Fatal("read again before the file held still")
	}
	now = now.Add(time.Second)
	if !h.Poll() {
		t.Fatal("a changed file isn't read again")
	}
	h.Settle(context.Background(), w, Links(w, same, ""))
	after := s.Value(a)
	if after.Kind != sheet.Number || after.Num-before.Num < 80 {
		t.Errorf("SUM %v before the file changed, %v after", before, after)
	}
}

func TestMissingSource(t *testing.T) {
	w := sheet.NewBook()
	s := w.Sheet(0)
	if _, err := w.AddSource("gone", sheet.LinkSource{Path: filepath.Join(t.TempDir(), "gone.parquet")}, s); err != nil {
		t.Fatal(err)
	}
	a := sheet.Addr{}
	s.Set(a, "=SUM(gone[x])")
	NewHost(1000).Settle(context.Background(), w, Links(w, same, ""))
	if v := s.Value(a); v != sheet.ErrRef {
		t.Errorf("a missing source: %v, want #REF!", v)
	}
	if why := s.ExplainError(a); !strings.Contains(why, "isn't there") {
		t.Errorf("explained as %q", why)
	}
}

// same is a path as it is: the tests' files are named whole.
func same(p string) string { return p }
