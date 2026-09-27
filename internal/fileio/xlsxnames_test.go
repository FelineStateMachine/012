package fileio

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A sheet whose name Excel can't take as is (spaces at its ends, or the
// same as another's but for them and case) gets another, and the
// formulas and named ranges naming it name that one.
func TestXLSXRenamedSheets(t *testing.T) {
	src := build(t, map[string]string{"A1": "1", "A2": "=' Plan'!A2+Plan!A1", "A3": "='Q3 '!A1*2",
		"A4": "=SUM('plan  '!A1:A2)", "A5": "=SUM(Slash)+Spaced"})
	book := src.Book()
	book.RenameSheet(src, "Plan")
	for _, sh := range []struct {
		name  string
		cells map[string]string
	}{
		{" Plan", map[string]string{"A2": "10"}}, {"Q3 ", map[string]string{"A1": "20", "A2": "30"}},
		{"plan  ", map[string]string{"A1": "40", "A2": "50"}},
	} {
		name, cells := sh.name, sh.cells
		s, err := book.AddSheet(name, book.Len())
		if err != nil {
			t.Fatal(err)
		}
		for a, in := range cells {
			s.Set(addr(t, a), in)
		}
	}
	book.DefineName("Slash", book.Lookup("Q3 "), sheet.NewRect(addr(t, "A1"), addr(t, "A2")))
	book.DefineName("Spaced", book.Lookup(" Plan"), sheet.NewRect(addr(t, "A2"), addr(t, "A2")))
	want := map[string]string{"A2": "11", "A3": "40", "A4": "90", "A5": "60"}
	for a, v := range want {
		if got := shown(src, addr(t, a)); got != v {
			t.Fatalf("%s = %s before export, want %s", a, got, v)
		}
	}
	name := filepath.Join(t.TempDir(), "renamed.xlsx")
	res, err := Export(context.Background(), name, XLSX, SnapBook(src), ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Notes) > 0 {
		t.Errorf("notes %q", res.Notes)
	}
	x, err := excelize.OpenFile(name)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	if f, _ := x.GetCellFormula("Plan", "A2"); f != "'Plan (2)'!A2+Plan!A1" {
		t.Errorf("Excel formula in A2 = %q", f)
	}
	got, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := got.Sheet.Book().Lookup("Plan")
	for a, v := range want {
		if g := shown(s, addr(t, a)); g != v {
			t.Errorf("%s = %s after the round trip (%s), want %s", a, g, input(s, addr(t, a)), v)
		}
	}
	if g := input(s, addr(t, "A4")); g != "=SUM('plan (3)'!A1:A2)" {
		t.Errorf("A4 = %q after the round trip", g)
	}
	for n, ws := range map[string]string{"Slash": "Q3", "Spaced": "Plan (2)"} {
		if nm, ok := got.Sheet.Book().LookupName(n); !ok || nm.Sheet.Name() != ws {
			t.Errorf("name %s: %+v", n, nm)
		}
	}
}
