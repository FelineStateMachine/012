package headless

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

func TestOpenReadsSources(t *testing.T) {
	dir := t.TempDir()
	db, err := sql.Open("sqlite", filepath.Join(dir, "n.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"CREATE TABLE n (x INTEGER)", "WITH RECURSIVE c(i) AS (SELECT 1 UNION ALL SELECT i+1 FROM c WHERE i < 100) INSERT INTO n SELECT i FROM c"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	w := sheet.NewBook()
	s := w.Sheet(0)
	if _, err := w.AddSource("n", sheet.LinkSource{Path: "n.sqlite", Table: "n"}, s); err != nil {
		t.Fatal(err)
	}
	if _, err := w.AddSource("far", sheet.LinkSource{Path: "../far.sqlite"}, s); err != nil {
		t.Fatal(err)
	}
	s.Set(sheet.Addr{}, "=SUM(n[x])")
	s.Set(sheet.Addr{Row: 1}, "=ROWS(far)")
	path := filepath.Join(dir, "book.012")
	out, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Write(out); err != nil {
		t.Fatal(err)
	}
	out.Close()
	f, err := Open(path, false)
	if err != nil {
		t.Fatal(err)
	}
	got := f.Book.Sheet(0)
	if v := got.Value(sheet.Addr{}); v.Num != 5050 {
		t.Errorf("SUM over the source: %v", v)
	}
	if why := got.ExplainError(sheet.Addr{Row: 1}); why != "far: Not read: outside the workbook's folder" {
		t.Errorf("a source outside the folder: %q", why)
	}
}
