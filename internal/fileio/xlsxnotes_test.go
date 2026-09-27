package fileio

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestXLSXNotes(t *testing.T) {
	src := build(t, map[string]string{"A1": "Rent", "B1": "1450"})
	src.SetNote(addr(t, "B1"), "Due on\nthe 1st")
	src.SetNote(addr(t, "D9"), "Blank cell")
	book := src.Book()
	other, _ := book.AddSheet("Other", 1)
	other.SetNote(addr(t, "A1"), "_x0041_ stays & <so> do these")
	name := filepath.Join(t.TempDir(), "notes.xlsx")
	if _, err := Export(context.Background(), name, XLSX, SnapBook(src), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	// excelize reads them as Excel would.
	x, err := excelize.OpenFile(name)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := x.GetComments(x.GetSheetName(0))
	if err != nil || len(cs) != 2 || cs[0].Cell != "B1" || cs[0].Text != "Due on\nthe 1st" {
		t.Errorf("excelize comments %+v, %v", cs, err)
	}
	x.Close()
	got, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	s := got.Sheet
	if s.Note(addr(t, "B1")) != "Due on\nthe 1st" || s.Note(addr(t, "D9")) != "Blank cell" {
		t.Errorf("notes %q %q", s.Note(addr(t, "B1")), s.Note(addr(t, "D9")))
	}
	if n := s.Book().Lookup("Other").Note(addr(t, "A1")); n != "_x0041_ stays & <so> do these" {
		t.Errorf("escaped note %q", n)
	}
}

func TestXLSXProtectionNote(t *testing.T) {
	x := excelize.NewFile()
	x.SetCellValue("Sheet1", "A1", 1)
	x.ProtectSheet("Sheet1", &excelize.SheetProtectionOptions{})
	name := filepath.Join(t.TempDir(), "protected.xlsx")
	if err := x.SaveAs(name); err != nil {
		t.Fatal(err)
	}
	x.Close()
	got, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Sheet.Protections()) != 0 || !strings.Contains(strings.Join(got.Notes, "; "), "Protection of Sheet1 left out") {
		t.Errorf("protections %+v, notes %q", got.Sheet.Protections(), got.Notes)
	}
}
