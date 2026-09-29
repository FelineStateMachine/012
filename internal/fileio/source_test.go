package fileio

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// sale is a row of the Parquet sources tests read.
type sale struct {
	ID     int64    `parquet:"id"`
	Cat    *string  `parquet:"cat,optional"`
	Amount *float64 `parquet:"amount,optional"`
	Day    int32    `parquet:"day,date"`
	Tags   []string `parquet:"tags,list"`
}

var saleCats = []string{"North", "south", "East", "west", "Éire", "", "north"}

// writeSales writes n rows in row groups of group rows.
func writeSales(t testing.TB, dir string, n, group int) string {
	t.Helper()
	name := filepath.Join(dir, "sales.parquet")
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	w := parquet.NewGenericWriter[sale](f, parquet.MaxRowsPerRowGroup(int64(group)), parquet.PageBufferSize(512))
	day := int32(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix() / 86400)
	rows := make([]sale, n)
	for i := range rows {
		r := &rows[i]
		r.ID, r.Day = int64(i), day+int32(i%40)
		if i%9 != 4 {
			c := saleCats[i%len(saleCats)]
			r.Cat = &c
		}
		if i%11 != 7 {
			a := float64((i*37)%101) - 20.5
			r.Amount = &a
		}
		if i%5 == 0 {
			r.Tags = []string{"a", fmt.Sprint(i)}
		}
	}
	if _, err := w.Write(rows); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	return name
}

// cellsText is a row as its cells show, separated by |.
func cellsText(row []sheet.LiveCell) string {
	parts := make([]string, len(row))
	for i, c := range row {
		parts[i] = sheet.FormatText(c.V, c.F)
	}
	return strings.Join(parts, "|")
}

// imported is a file imported as a sheet, which sources must read as
// the import does, and the rows under its header.
func imported(t *testing.T, name string, opt Options) (*sheet.Sheet, sheet.Rect) {
	t.Helper()
	res, err := Import(context.Background(), name, opt)
	if err != nil {
		t.Fatal(err)
	}
	used, _ := res.Sheet.UsedRange()
	used.From.Row = 1
	return res.Sheet, used
}

// sheetRows are the rows of r a sheet shows, the filter's hidden ones
// left out.
func sheetRows(s *sheet.Sheet, r sheet.Rect) []string {
	var out []string
	for row := r.From.Row; row <= r.To.Row; row++ {
		if s.RowHidden(row) {
			continue
		}
		parts := make([]string, 0, r.To.Col+1)
		for c := r.From.Col; c <= r.To.Col; c++ {
			parts = append(parts, shown(s, sheet.Addr{Col: c, Row: row}))
		}
		out = append(out, strings.Join(parts, "|"))
	}
	return out
}

// viewRows pages through a whole view, n rows at a time.
func viewRows(t *testing.T, v SourceView, n int) ([]int64, []string) {
	t.Helper()
	var nums []int64
	var out []string
	for from := int64(0); from < v.Rows(); from += int64(n) {
		ns, rows, err := v.Page(context.Background(), from, n)
		if err != nil {
			t.Fatal(err)
		}
		if len(ns) != min(n, int(v.Rows()-from)) {
			t.Fatalf("page at %d has %d rows", from, len(ns))
		}
		for i, r := range rows {
			nums, out = append(nums, ns[i]), append(out, cellsText(r))
		}
	}
	return nums, out
}

// orders are the sorts and filters every source is checked with,
// against a sheet sorting and filtering the same rows.
var orders = []SourceOrder{
	{},
	{Sort: []SourceSort{{Col: 1}}},
	{Sort: []SourceSort{{Col: 1, Desc: true}}},
	{Sort: []SourceSort{{Col: 2, Desc: true}, {Col: 1}}},
	{Filter: []SourceFilter{{Col: 2, Cond: sheet.Condition{Op: sheet.CondGreater, Arg: "30"}}}},
	{Filter: []SourceFilter{{Col: 1, Cond: sheet.Condition{Op: sheet.CondContains, Arg: "OR"}}}},
	{Filter: []SourceFilter{{Col: 1, Cond: sheet.Condition{Op: sheet.CondEmpty}}}},
	{Filter: []SourceFilter{{Col: 1, Cond: sheet.Condition{Op: sheet.CondNotEqual, Arg: "north"}}}},
	{Filter: []SourceFilter{{Col: 1, Cond: sheet.Condition{Op: sheet.CondStartsWith, Arg: "e"}}}},
	{Filter: []SourceFilter{{Col: 1, Cond: sheet.Condition{Op: sheet.CondEndsWith, Arg: "TH"}}}},
	{Filter: []SourceFilter{{Col: 1, Cond: sheet.Condition{Op: sheet.CondExactly, Arg: "EAST"}}}},
	{Filter: []SourceFilter{{Col: 2, Cond: sheet.Condition{Op: sheet.CondLessEq, Arg: "-10"}}},
		Sort: []SourceSort{{Col: 2}}},
	{Filter: []SourceFilter{{Col: 1, Cond: sheet.Condition{Op: sheet.CondNotEmpty}},
		{Col: 2, Cond: sheet.Condition{Op: sheet.CondGreaterEq, Arg: "0"}}},
		Sort: []SourceSort{{Col: 1, Desc: true}}},
}

// checkOrders checks each order's view against the sheet the file
// imports as, sorted and filtered the same way.
func checkOrders(t *testing.T, src Source, name string, opt Options, list []SourceOrder) {
	t.Helper()
	for _, o := range list {
		s, r := imported(t, name, opt)
		if len(o.Sort) > 0 {
			var keys []sheet.SortKey
			for _, k := range o.Sort {
				keys = append(keys, sheet.SortKey{Col: k.Col, Desc: k.Desc})
			}
			s.SortRange(r, keys)
		}
		if len(o.Filter) > 0 {
			s.CreateFilter(sheet.Rect{To: r.To})
			for _, f := range o.Filter {
				s.FilterColumn(f.Col, sheet.Criteria{Cond: f.Cond})
			}
		}
		want := sheetRows(s, r)
		v, err := src.View(context.Background(), o)
		if err != nil {
			t.Fatalf("%+v: %v", o, err)
		}
		_, got := viewRows(t, v, 37)
		if !slices.Equal(got, want) {
			i := 0
			for i < min(len(got), len(want)) && got[i] == want[i] {
				i++
			}
			t.Errorf("%+v: %d rows, want %d; from row %d\n got %q\nwant %q", o, len(got), len(want), i, head(got[i:]), head(want[i:]))
		}
		if err := v.Close(); err != nil {
			t.Error(err)
		}
	}
}

func head(rows []string) []string { return rows[:min(len(rows), 12)] }

func TestParquetSource(t *testing.T) {
	dir := t.TempDir()
	name := writeSales(t, dir, 1000, 128)
	tmp := t.TempDir()
	src, err := OpenSource(context.Background(), SourceSpec{Path: name, TempDir: tmp})
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	if src.Rows() != 1000 {
		t.Fatalf("%d rows", src.Rows())
	}
	var cols []string
	for _, c := range src.Columns() {
		cols = append(cols, fmt.Sprintf("%s:%v:%v", c.Name, c.Numeric, c.Format.Kind == sheet.FmtDate))
	}
	if got := strings.Join(cols, " "); got != "id:true:false cat:false:false amount:true:false day:true:true tags.list.element:false:false" {
		t.Errorf("columns %s", got)
	}
	s, r := imported(t, name, Options{})
	want := sheetRows(s, r)

	// Scan from part way, two columns in another order.
	var got []string
	err = src.Scan(context.Background(), 500, []int{2, 0}, func(row int64, vals []sheet.LiveCell) bool {
		got = append(got, fmt.Sprintf("%d:%s", row, cellsText(vals)))
		return row < 510
	})
	if err != nil {
		t.Fatal(err)
	}
	for i, g := range got {
		parts := strings.Split(want[500+i], "|")
		if w := fmt.Sprintf("%d:%s|%s", 500+i, parts[2], parts[0]); g != w {
			t.Errorf("scan %q, want %q", g, w)
		}
	}
	if len(got) != 11 {
		t.Errorf("scan read %d rows", len(got))
	}

	// Fetch rows in any order, across row groups.
	nums := []int64{999, 3, 500, 128, 127, 3}
	rows, err := src.Fetch(context.Background(), nums, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range nums {
		if g := cellsText(rows[i]); g != want[n] {
			t.Errorf("row %d is %q, want %q", n, g, want[n])
		}
	}
	checkOrders(t, src, name, Options{}, orders)
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("views left %d files", len(left))
	}
}

// TestSourceSortSpills sorts in runs spilled to files and merged.
func TestSourceSortSpills(t *testing.T) {
	defer func(n int) { sortRunBytes = n }(sortRunBytes)
	sortRunBytes = 4096
	name := writeSales(t, t.TempDir(), 3000, 1000)
	tmp := t.TempDir()
	src, err := OpenSource(context.Background(), SourceSpec{Path: name, TempDir: tmp})
	if err != nil {
		t.Fatal(err)
	}
	checkOrders(t, src, name, Options{}, orders[1:4])
	v, err := src.View(context.Background(), orders[2])
	if err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 1 {
		t.Errorf("a view keeps %d files, want its row order", len(left))
	}
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("closing left %d files", len(left))
	}
	if err := v.Close(); err != nil {
		t.Errorf("closing a view after its source: %v", err)
	}
}

// TestSourceConcurrent reads a source from several goroutines at once.
func TestSourceConcurrent(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{writeSales(t, dir, 2000, 300), writeSalesDB(t, dir, 2000, false)} {
		src, err := OpenSource(context.Background(), SourceSpec{Path: name, Table: "sales", TempDir: dir})
		if err != nil {
			t.Fatal(err)
		}
		sum := func() float64 {
			total := 0.0
			err := src.Scan(context.Background(), 0, []int{2}, func(_ int64, v []sheet.LiveCell) bool {
				total += v[0].V.Num
				return true
			})
			if err != nil {
				t.Error(err)
			}
			return total
		}
		want := sum()
		var wg sync.WaitGroup
		for i := range 8 {
			wg.Go(func() {
				if got := sum(); got != want {
					t.Errorf("%s: sum %v, want %v", name, got, want)
				}
				if _, err := src.Fetch(context.Background(), []int64{int64(i * 200), 1999}, nil); err != nil {
					t.Error(err)
				}
				v, err := src.View(context.Background(), SourceOrder{Sort: []SourceSort{{Col: 2}}})
				if err != nil {
					t.Error(err)
					return
				}
				if _, _, err := v.Page(context.Background(), 1500, 60); err != nil {
					t.Error(err)
				}
				v.Close()
			})
		}
		wg.Wait()
		src.Close()
	}
}

func TestSourceKind(t *testing.T) {
	for spec, ok := range map[SourceSpec]bool{
		{Path: "a.parquet"}: true, {Path: "a.sqlite"}: true, {Path: "a.db"}: true,
		{Path: "a.csv"}: false, {Path: "a.bin", Format: "Parquet"}: true, {Path: "a.parquet", Format: "CSV"}: false,
	} {
		if _, err := SourceKind(spec); (err == nil) != ok {
			t.Errorf("%+v: %v", spec, err)
		}
	}
	if _, err := OpenSource(context.Background(), SourceSpec{Path: filepath.Join(t.TempDir(), "none.parquet")}); err == nil {
		t.Error("opened a file that isn't there")
	}
}
