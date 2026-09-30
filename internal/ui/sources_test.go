package ui

import (
	"os"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// srcRow is a row of the Parquet files the tests link.
type srcRow struct {
	ID     int64   `parquet:"id"`
	Region string  `parquet:"region"`
	Amount float64 `parquet:"amount"`
}

var srcRegions = []string{"North", "South", "East", "West"}

// writeSource writes n rows to name: ids from 0, regions in turn, and
// amounts of id*3 mod 1000.
func writeSource(t testing.TB, name string, n int) (sum float64) {
	t.Helper()
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	w := parquet.NewGenericWriter[srcRow](f, parquet.MaxRowsPerRowGroup(500))
	rows := make([]srcRow, n)
	for i := range rows {
		rows[i] = srcRow{ID: int64(i), Region: srcRegions[i%len(srcRegions)], Amount: float64(i * 3 % 1000)}
		sum += rows[i].Amount
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
	return sum
}

// pumpSources runs what the sources have queued until nothing is.
func pumpSources(t *testing.T, m *Model) {
	t.Helper()
	for range 50 {
		run(m, m.syncSources())
		if m.src.host == nil || !m.src.host.Pending() && m.src.running == 0 && !m.pagesPending() {
			return
		}
	}
	t.Fatal("the sources never settled")
}

// pagesPending reports whether a page is being read.
func (m *Model) pagesPending() bool {
	for _, p := range m.src.pages {
		if p.Building() {
			return true
		}
	}
	return false
}

// linkSales writes sales.parquet with n rows here and links it.
func linkSales(t *testing.T, m *Model, n int) float64 {
	t.Helper()
	sum := writeSource(t, "sales.parquet", n)
	m.runCommand("data.link_source")
	press(t, m, "sales.parquet", "<enter>")
	pumpSources(t, m)
	m.note = ""
	return sum
}

func TestSourceTab(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	linkSales(t, m, 3000)
	if !m.sheet.IsSource() || m.sheet.Name() != "sales" {
		t.Fatalf("the tab shown is %s", m.sheet.Name())
	}
	if l := line(m, contextLine); !strings.Contains(l, "▦ sales.parquet  3,000 rows, 3 columns") {
		t.Errorf("context line %q", l)
	}
	if l := line(m, menuLine); !strings.Contains(l, "SOURCE") {
		t.Errorf("menu bar %q", l)
	}
	if l := line(m, headerLine+1); !strings.Contains(l, "id") || !strings.Contains(l, "region") {
		t.Errorf("header row %q", l)
	}
	if l := line(m, headerLine+3); !strings.HasPrefix(strings.TrimSpace(l), "2 ") || !strings.Contains(l, "North") {
		t.Errorf("first row %q", l)
	}
	if st := status(m); !strings.Contains(st, "▦ sales") {
		t.Errorf("status %q", st)
	}
	// The last row, numbered as the sheet's row it is.
	press(t, m, "<ctrl+down>")
	pumpSources(t, m)
	if l := screen(m); !strings.Contains(l, "3001 ") {
		t.Errorf("the last row isn't shown:\n%s", l)
	}
	if name := strings.Fields(line(m, formulaLine))[0]; name != "A3001" {
		t.Errorf("name box %q", name)
	}
	if bar(m) != "2999" {
		t.Errorf("formula bar %q", bar(m))
	}
	// Typing doesn't edit.
	press(t, m, "7", "<enter>")
	if !m.sheet.IsSource() {
		t.Error("typing left the tab")
	}
}

func TestSourceSortAndFilter(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	linkSales(t, m, 2000)
	press(t, m, "<right>", "<right>") // amount
	m.runCommand("data.sort_sheet_za")
	pumpSources(t, m)
	if l := line(m, contextLine); !strings.Contains(l, "sorted by amount Z to A") {
		t.Errorf("context line %q", l)
	}
	if bar(m) != "999" {
		t.Errorf("the top amount sorted Z to A is %q", bar(m))
	}
	info, _ := m.sheet.Source()
	o := copyOrder(info.Source.Order)
	o.Filter = []sheet.SourceFilter{{Col: 1, Cond: sheet.Condition{Op: sheet.CondExactly, Arg: "east"}}}
	m.setSourceOrder(info, o)
	pumpSources(t, m)
	if l := line(m, contextLine); !strings.Contains(l, "showing 500") {
		t.Errorf("context line %q", l)
	}
	press(t, m, "<left>")
	if bar(m) != "East" {
		t.Errorf("the filter lets %q through", bar(m))
	}
	m.runCommand("data.source_all")
	pumpSources(t, m)
	if bar(m) != "North" {
		t.Errorf("in the source's order the first region is %q", bar(m))
	}
	// Undo puts the order back.
	press(t, m, "<ctrl+z>")
	pumpSources(t, m)
	if bar(m) != "East" {
		t.Errorf("after undo, %q", bar(m))
	}
}

func TestFormulasReadASourceTab(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	sum := linkSales(t, m, 5000)
	m.showSheet(m.book().Sheet(0))
	press(t, m, "=SUM(sales[amount])", "<enter>", `=COUNTIF(sales[region],"west")`, "<enter>", "=sales!C4", "<enter>")
	pumpSources(t, m)
	if v := m.sheet.Value(addr("A1")); v.Num != sum {
		t.Errorf("SUM %v, want %v", v, sum)
	}
	if v := m.sheet.Value(addr("A2")); v.Num != 1250 {
		t.Errorf("COUNTIF %v", v)
	}
	if v := m.sheet.Value(addr("A3")); v.Num != 6 {
		t.Errorf("sales!C4 %v", v)
	}
}

func TestSourceCommandsOnTheTab(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	linkSales(t, m, 100)
	if commands["format.bold"].available(m) {
		t.Error("Bold is offered on a source's tab")
	}
	m.runCommand("data.frequency")
	pumpSources(t, m)
	if !strings.HasPrefix(m.sheet.Name(), "Frequency of") {
		t.Fatalf("frequency table on %s", m.sheet.Name())
	}
	if v := m.sheet.Value(addr("B2")); v.Num != 1 {
		t.Errorf("each id is there once: %v", v)
	}
}
