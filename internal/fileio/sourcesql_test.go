package fileio

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// writeSalesDB writes a database whose table sales has n rows (fewer,
// with rowids missing, when sparse), a view of it, a WITHOUT ROWID
// table, and a column of mixed kinds with no declared type.
func writeSalesDB(t testing.TB, dir string, n int, sparse bool) string {
	t.Helper()
	name := filepath.Join(dir, fmt.Sprintf("sales-%d-%v.sqlite", n, sparse))
	db, err := sql.Open("sqlite", name)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"CREATE TABLE sales (id INTEGER, cat TEXT, amount REAL, day TEXT, mixed)",
		"CREATE VIEW big AS SELECT * FROM sales WHERE amount > 50",
		"CREATE TABLE keyed (k TEXT PRIMARY KEY, v REAL) WITHOUT ROWID",
		"CREATE TABLE other (x)",
	} {
		if _, err := tx.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	ins, err := tx.Prepare("INSERT INTO sales VALUES (?, ?, ?, ?, ?)")
	if err != nil {
		t.Fatal(err)
	}
	for i := range n {
		var cat, amount, mixed any
		if i%9 != 4 {
			cat = saleCats[i%len(saleCats)]
		}
		if i%11 != 7 {
			amount = float64((i*37)%101) - 20.5
		}
		switch i % 6 {
		case 0:
			mixed = int64(i % 13)
		case 1:
			mixed = fmt.Sprintf("t%d", i%7)
		case 2:
			mixed = 2.5
		case 3:
			mixed = "12"
		case 4:
			mixed = ""
		}
		day := fmt.Sprintf("2026-01-%02d", 1+i%28)
		if _, err := ins.Exec(i, cat, amount, day, mixed); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec("INSERT INTO keyed VALUES (?, ?)", fmt.Sprintf("k%05d", n-i), i); err != nil {
			t.Fatal(err)
		}
	}
	if sparse {
		if _, err := tx.Exec("DELETE FROM sales WHERE id % 7 = 3"); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	return name
}

// sqlOrders are orders only a SQLite source's table has columns for.
var sqlOrders = []SourceOrder{
	{Sort: []SourceSort{{Col: 4}}},
	{Sort: []SourceSort{{Col: 4, Desc: true}}},
	{Filter: []SourceFilter{{Col: 4, Cond: sheet.Condition{Op: sheet.CondGreater, Arg: "2"}}}},
	{Filter: []SourceFilter{{Col: 4, Cond: sheet.Condition{Op: sheet.CondLess, Arg: "T3"}}}},
	{Filter: []SourceFilter{{Col: 4, Cond: sheet.Condition{Op: sheet.CondEqual, Arg: "12"}}}},
}

func TestSQLiteSource(t *testing.T) {
	dir := t.TempDir()
	for _, sparse := range []bool{false, true} {
		t.Run(fmt.Sprint("sparse=", sparse), func(t *testing.T) { checkSQLiteTable(t, writeSalesDB(t, dir, 700, sparse)) })
	}
}

// checkSQLiteTable reads the table sales of name every way a source
// reads, as the sheet its import makes shows it.
func checkSQLiteTable(t *testing.T, name string) {
	tmp := t.TempDir()
	src, err := OpenSource(context.Background(), SourceSpec{Path: name, Table: "sales", TempDir: tmp})
	if err != nil {
		t.Fatal(err)
	}
	s, r := imported(t, name, Options{Table: "sales"})
	want := sheetRows(s, r)
	if src.Rows() != int64(len(want)) {
		t.Fatalf("%d rows, want %d", src.Rows(), len(want))
	}
	var cols []string
	for _, c := range src.Columns() {
		cols = append(cols, fmt.Sprintf("%s:%v", c.Name, c.Numeric))
	}
	if !slices.Equal(cols, []string{"id:true", "cat:false", "amount:true", "day:false", "mixed:false"}) {
		t.Errorf("columns %q", cols)
	}
	var got []string
	err = src.Scan(context.Background(), 300, nil, func(row int64, v []sheet.LiveCell) bool {
		got = append(got, cellsText(v))
		return true
	})
	if err != nil || !slices.Equal(got, want[300:]) {
		t.Errorf("scan from 300: %v %q", err, head(got))
	}
	rows, err := src.Fetch(context.Background(), []int64{int64(len(want) - 1), 0, 350}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for i, n := range []int{len(want) - 1, 0, 350} {
		if g := cellsText(rows[i]); g != want[n] {
			t.Errorf("row %d is %q, want %q", n, g, want[n])
		}
	}
	checkOrders(t, src, name, Options{Table: "sales"}, append(slices.Clone(orders), sqlOrders...))
	if err := src.Close(); err != nil {
		t.Fatal(err)
	}
	if left, _ := os.ReadDir(tmp); len(left) != 0 {
		t.Errorf("closing left %d files", len(left))
	}
}

// TestSQLiteSourceKinds reads a view, a WITHOUT ROWID table and a
// query, which are paged through a copy in the work database.
func TestSQLiteSourceKinds(t *testing.T) {
	dir := t.TempDir()
	name := writeSalesDB(t, dir, 300, false)
	for _, spec := range []SourceSpec{
		{Path: name, Table: "big"},
		{Path: name, Table: "keyed"},
		{Path: name, Query: "SELECT cat, sum(amount) AS total, count(*) AS n FROM sales GROUP BY cat"},
		{Path: name, Query: "SELECT id, id FROM sales WHERE id < 40"},
	} {
		spec.TempDir = t.TempDir()
		src, err := OpenSource(context.Background(), spec)
		if err != nil {
			t.Fatalf("%+v: %v", spec, err)
		}
		s, r := imported(t, name, Options{Table: spec.Table, Query: spec.Query})
		want := sheetRows(s, r)
		v, err := src.View(context.Background(), SourceOrder{})
		if err != nil {
			t.Fatal(err)
		}
		if _, got := viewRows(t, v, 50); !slices.Equal(got, want) {
			t.Errorf("%+v: %q, want %q", spec, head(got), head(want))
		}
		sorted := []SourceOrder{{Sort: []SourceSort{{Col: 1, Desc: true}}}}
		checkOrders(t, src, name, Options{Table: spec.Table, Query: spec.Query}, sorted)
		src.Close()
		if left, _ := os.ReadDir(spec.TempDir); len(left) != 0 {
			t.Errorf("%+v: closing left %d files", spec, len(left))
		}
	}
	if _, err := OpenSource(context.Background(), SourceSpec{Path: name}); err == nil {
		t.Error("a database of several tables opened without naming one")
	}
	if _, err := OpenSource(context.Background(), SourceSpec{Path: name, Query: "SELECT nope FROM sales"}); err == nil {
		t.Error("a query that fails opened")
	}
}

// TestSQLiteSourceReadOnly checks the database file is never written.
func TestSQLiteSourceReadOnly(t *testing.T) {
	name := writeSalesDB(t, t.TempDir(), 200, true)
	before, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	src, err := OpenSource(context.Background(), SourceSpec{Path: name, Table: "sales", TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	v, err := src.View(context.Background(), SourceOrder{Sort: []SourceSort{{Col: 2}}})
	if err != nil {
		t.Fatal(err)
	}
	v.Page(context.Background(), 100, 20)
	src.Close()
	after, _ := os.ReadFile(name)
	if !slices.Equal(before, after) {
		t.Error("the database changed")
	}
	if _, err := os.Stat(name + "-journal"); err == nil {
		t.Error("the database has a journal")
	}
}
