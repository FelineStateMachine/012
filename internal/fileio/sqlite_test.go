package fileio

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// sqliteFixture makes a database with two tables and a view.
func sqliteFixture(t *testing.T) string {
	t.Helper()
	name := filepath.Join(t.TempDir(), "shop.sqlite")
	db, err := sql.Open("sqlite", name)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range []string{
		`CREATE TABLE orders (id INTEGER, customer TEXT, total REAL, placed TEXT, zip TEXT, note BLOB)`,
		`INSERT INTO orders VALUES (1, 'Ada', 12.5, '2026-09-26', '02139', NULL)`,
		`INSERT INTO orders VALUES (2, '=cmd()', 7, '2026-09-27 14:30:00', '10001', x'00ff10')`,
		`CREATE TABLE customers (name TEXT, since INTEGER)`,
		`INSERT INTO customers VALUES ('Ada', 2020)`,
		`CREATE VIEW big AS SELECT * FROM orders WHERE total > 10`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return name
}

func TestSQLiteTables(t *testing.T) {
	name := sqliteFixture(t)
	ts, err := Tables(context.Background(), name)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, tb := range ts {
		got = append(got, tb.Name+":"+strings.Join(tb.Cols, "/")+":"+thousands(tb.Rows))
	}
	want := "big:id/customer/total/placed/zip/note:1 customers:name/since:1 orders:id/customer/total/placed/zip/note:2"
	if strings.Join(got, " ") != want {
		t.Errorf("tables %q", got)
	}
	if !ts[0].View {
		t.Errorf("big isn't a view")
	}
}

func TestSQLiteImport(t *testing.T) {
	name := sqliteFixture(t)
	ctx := context.Background()

	_, err := Import(ctx, name, Options{})
	var need *ErrNeedTable
	if !errors.As(err, &need) || len(need.Tables) != 3 {
		t.Fatalf("import without a table: %v", err)
	}

	res, err := Import(ctx, name, Options{Table: "orders"})
	if err != nil {
		t.Fatal(err)
	}
	s := res.Sheet
	for cell, want := range map[string]string{
		"A1": "id", "F1": "note", "A2": "1", "C2": "12.5", "D2": "2026-09-26", "E2": "02139",
		"B3": "=cmd()", "D3": "9/27/2026 14:30:00", "F3": "(3 bytes)",
	} {
		if g := shown(s, addr(t, cell)); g != want {
			t.Errorf("%s shows %q, want %q", cell, g, want)
		}
	}
	if !s.Cell(addr(t, "A1")).Style.Bold {
		t.Errorf("header isn't bold")
	}
	if res.Rows != 3 || !strings.Contains(strings.Join(res.Notes, ";"), "1 binary value shown as sizes") {
		t.Errorf("rows %d, notes %q", res.Rows, res.Notes)
	}

	res, err = Import(ctx, name, Options{Query: "SELECT customer, total*2 AS twice FROM orders ORDER BY id DESC"})
	if err != nil {
		t.Fatal(err)
	}
	if g := shown(res.Sheet, addr(t, "B1")) + " " + shown(res.Sheet, addr(t, "B3")); g != "twice 25" {
		t.Errorf("query result %q", g)
	}

	// Queries can't change the database.
	if _, err := Import(ctx, name, Options{Query: "DELETE FROM orders"}); err == nil {
		t.Errorf("DELETE ran")
	}
	if _, err := Import(ctx, name, Options{Query: "SELECT nope FROM orders"}); err == nil || !strings.Contains(err.Error(), "no such column: nope") {
		t.Errorf("bad query error: %v", err)
	}
	// A file that isn't a database says so.
	junk := filepath.Join(t.TempDir(), "junk.db")
	os.WriteFile(junk, []byte("hello, this is not sqlite at all, not even close......"), 0o644)
	if _, err := Import(ctx, junk, Options{}); err == nil || !strings.Contains(err.Error(), "not a database") {
		t.Errorf("junk error: %v", err)
	}
	// Opening doesn't create missing files.
	missing := filepath.Join(t.TempDir(), "missing.sqlite")
	if _, err := Import(ctx, missing, Options{}); err == nil {
		t.Errorf("imported a missing file")
	}
	if _, err := os.Stat(missing); err == nil {
		t.Errorf("import created %s", missing)
	}
}

// TestSQLitePastLimit keeps the rows that fit and says how many didn't,
// for a table (counted, not read) and for a query (read to the end).
func TestSQLitePastLimit(t *testing.T) {
	name := filepath.Join(t.TempDir(), "nums.sqlite")
	db, err := sql.Open("sqlite", name)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE nums AS WITH RECURSIVE c(n) AS
		(SELECT 1 UNION ALL SELECT n+1 FROM c WHERE n < 8195) SELECT n FROM c`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	for _, opt := range []Options{{Table: "nums", MaxCells: 8192}, {Query: "SELECT n FROM nums", MaxCells: 8192}} {
		res, err := Import(context.Background(), name, opt)
		if err != nil {
			t.Fatal(err)
		}
		if g := shown(res.Sheet, sheet.Addr{Row: 8191}); g != "8191" {
			t.Errorf("%+v: last row shows %q, want 8191", opt, g)
		}
		if res.Rows != 8196 || len(res.Notes) != 1 || res.Notes[0] != "only the first 8,192 rows fit in max-cells (8,192 cells); 4 rows left out" {
			t.Errorf("%+v: rows %d, notes %q", opt, res.Rows, res.Notes)
		}
	}
}

func TestSQLiteExportRoundTrip(t *testing.T) {
	src := build(t, map[string]string{
		"A1": "Item", "B1": "Qty", "C1": "Price", "D1": "Due", "E1": "", "F1": "Item",
		"A2": "Rent", "B2": "1", "C2": "$1,450.50", "D2": "10/1/2026", "E2": "x", "F2": "TRUE",
		"A3": "Food", "B3": "=B2+2", "C3": "612.4", "D3": "9/28/2026",
		"A5": "Gym", "B5": "4", "C5": "=1/0",
	})
	name := sqliteFixture(t)
	ctx := context.Background()
	res, err := Export(ctx, name, SQLite, Snap(src, sheet.Rect{}, "my bills!"), ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows != 3 {
		t.Errorf("wrote %d rows", res.Rows)
	}
	db, _ := sql.Open("sqlite", name)
	defer db.Close()
	var ddl string
	db.QueryRow(`SELECT sql FROM sqlite_master WHERE name = 'my_bills'`).Scan(&ddl)
	want := `CREATE TABLE "my_bills" ("Item" TEXT, "Qty" INTEGER, "Price" REAL, "Due" TEXT, "column_E" TEXT, "Item_2" INTEGER)`
	if ddl != want {
		t.Errorf("table\n%s\nwant\n%s", ddl, want)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM orders`).Scan(&n)
	if n != 2 {
		t.Errorf("other tables changed: orders has %d rows", n)
	}

	back, err := Import(ctx, name, Options{Table: "my_bills"})
	if err != nil {
		t.Fatal(err)
	}
	for cell, want := range map[string]string{
		"A2": "Rent", "B3": "3", "C2": "1450.5", "D2": "2026-10-01", "F2": "1", "A4": "Gym", "C4": "",
	} {
		if g := shown(back.Sheet, addr(t, cell)); g != want {
			t.Errorf("%s shows %q, want %q", cell, g, want)
		}
	}

	// Exporting a selection writes just it, its first row as the header;
	// exporting again replaces the table.
	r := sheet.NewRect(addr(t, "A2"), addr(t, "B3"))
	if _, err := Export(ctx, name, SQLite, Snap(src, r, "x"), ExportOptions{Table: "my_bills"}); err != nil {
		t.Fatal(err)
	}
	db.QueryRow(`SELECT count(*) FROM my_bills WHERE Rent = 'Food'`).Scan(&n)
	if n != 1 {
		t.Errorf("selection export: %d rows", n)
	}
}

func TestTableName(t *testing.T) {
	for in, want := range map[string]string{"budget": "budget", "my bills!": "my_bills", "2026 sales": "t_2026_sales", "": "sheet", "é": "sheet"} {
		if got := TableName(in); got != want {
			t.Errorf("TableName(%q) = %q, want %q", in, got, want)
		}
	}
}
