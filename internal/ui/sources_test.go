package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/FelineStateMachine/012/internal/room"
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
	if !settleSources(m) {
		t.Fatal("the sources never settled")
	}
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
	// Go to, a row or a cell as the tab numbers them.
	press(t, m, "<ctrl+g>", "b2500", "<enter>")
	pumpSources(t, m)
	if name := strings.Fields(line(m, formulaLine))[0]; name != "B2500" || bar(m) != "East" { // row 2500 is the source's 2498th, counting from 0
		t.Errorf("go to B2500: %q %q", name, bar(m))
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

func TestSourceSortRecorded(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	linkSales(t, m, 100)
	src := record(t, m, false, "Top", "", "<right>", "<right>", "<ctrl+k>", "sort sheet z to a", "<enter>")
	if !strings.Contains(src, `run("data.sort_sheet_za")`) {
		t.Errorf("the macro:\n%s", src)
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

// The files' ticks come at once in tests, whose ticks are dropped.
func init() { sourceInterval = 0 }

// A source linked in a room is the room's: the one who keeps the room
// reads it and answers what anyone's formulas ask of it, each
// participant scrolls a tab of their own, and whoever keeps the room
// next goes on reading it.
func TestSharedSources(t *testing.T) {
	r := newRooms(t, room.Edit)
	sum := writeSource(t, filepath.Join(r.dir, "sales.parquet"), 1000)
	ann := r.open("ann", "@src")
	bob := r.open("bob", "@src")
	bob.run(bob.m.runCommand("data.link_source"))
	bob.press("sales.parquet", "<enter>")
	for range 5 {
		r.sync()
	}
	if !strings.Contains(bob.screen(), "North") {
		t.Fatalf("bob's tab:\n%s", bob.screen())
	}
	bob.press("<ctrl+down>")
	r.sync()
	if !strings.Contains(bob.screen(), "1001 ") || strings.Contains(ann.screen(), "1001 ") {
		t.Errorf("bob's scroll isn't his own:\n%s\n%s", bob.screen(), ann.screen())
	}
	bob.press("<ctrl+pgup>", "=SUM(sales[amount])", "<enter>")
	for range 5 {
		r.sync()
	}
	var got float64
	ann.sh.turn(func() { got = ann.m.book().Sheet(0).Value(addr("A1")).Num })
	if got != sum {
		t.Fatalf("SUM %v, want %v", got, sum)
	}
	if ann.m.LeaveRoom() {
		t.Fatal("ann was the last")
	}
	r.all = r.all[1:]
	r.sync()
	bob.press(`=COUNTIF(sales[region],"east")`, "<enter>")
	for range 5 {
		r.sync()
	}
	bob.sh.turn(func() { got = bob.m.book().Sheet(0).Value(addr("A2")).Num })
	if got != 250 {
		t.Errorf("bob, keeping the room now, answered COUNTIF with %v", got)
	}
}

// In a room of one writer, whoever follows may scroll a source's tab
// but not sort it.
func TestSharedSourceOneWriter(t *testing.T) {
	r := newRooms(t, room.View)
	writeSource(t, filepath.Join(r.dir, "sales.parquet"), 100)
	ann := r.open("ann", "@one")
	bob := r.open("bob", "@one")
	ann.run(ann.m.runCommand("data.link_source"))
	ann.press("sales.parquet", "<enter>")
	for range 3 {
		r.sync()
	}
	bob.press("<ctrl+pgdown>", "<down>")
	bob.run(bob.m.runCommand("data.sort_sheet_za"))
	var order *sheet.SourceOrder
	bob.sh.turn(func() {
		info, _ := bob.m.book().LookupSource("sales")
		order = info.Source.Order
	})
	if order != nil || !strings.Contains(bob.screen(), "writes here and you follow") {
		t.Errorf("a follower sorted the source: %+v\n%s", order, bob.screen())
	}
}

func TestMonochromeSource(t *testing.T) {
	t.Chdir(t.TempDir())
	m := newModel()
	linkSales(t, m, 3000)
	m.note = ""
	row := monoLine(m, headerLine+3) // the first row, its first cell active
	if !every(row[6:12], false, isReverse) {
		t.Errorf("the active cell isn't in reverse video: %+v", row[6:12])
	}
	if last := row[len(row)-1]; last.text != "┃" {
		t.Errorf("the scrollbar's thumb at the top is %q", last.text)
	}
	if last := monoLine(m, headerLine+6); last[len(last)-1].text != "│" {
		t.Errorf("the scrollbar's track is %q", last[len(last)-1].text)
	}
	if l := cellText(monoLine(m, contextLine)); !strings.Contains(l, "▦ sales.parquet") {
		t.Errorf("context line: %q", l)
	}
	if err := os.Remove("sales.parquet"); err != nil {
		t.Fatal(err)
	}
	m.sources().host.Reload("sales")
	pumpSources(t, m)
	if l := cellText(monoLine(m, m.height-1)); !strings.Contains(l, "▦ sales ! ") {
		t.Errorf("status line of a source whose file is gone: %q", l)
	}
}
