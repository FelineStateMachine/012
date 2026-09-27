package ui

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// importInto runs File > Import on name and picks place in the location
// picker.
func importInto(t *testing.T, m *Model, name, place string) {
	t.Helper()
	m.runCommand("file.import")
	press(t, m, name, "<enter>")
	p, ok := m.overlay.(*picker)
	if !ok || !strings.HasPrefix(p.title, "Import ") {
		t.Fatalf("no location picker: %T", m.overlay)
	}
	press(t, m, place, "<enter>")
}

func TestImportLocation(t *testing.T) {
	t.Chdir(t.TempDir())
	writeFile(t, "sales.csv", "Region,Total\nNorth,12\n")
	m := newModel()
	press(t, m, "=sales!B2*2", "<enter>")
	m.changed = false

	// The picker offers Sheets' choices, what the current sheet is, and
	// says what each does.
	m.runCommand("file.import")
	press(t, m, "sales.csv", "<enter>")
	got := strings.Join(pickerTitles(t, m), "\n")
	want := "Insert new sheet | after the others\nReplace current sheet | Sheet1\nReplace spreadsheet | open it instead"
	if got != want {
		t.Errorf("locations:\n%s\nwant\n%s", got, want)
	}
	if st := status(m); !strings.Contains(st, "Add sales.csv as new sheets") {
		t.Errorf("status %q", st)
	}
	press(t, m, "<esc>")
	if m.overlay != nil || m.book().Len() != 1 {
		t.Fatalf("Esc imported: %d sheets", m.book().Len())
	}

	// Insert new sheet: one undo step, the file stays this one.
	importInto(t, m, "sales.csv", "Insert new sheet")
	if sheetNames(m) != "Sheet1,sales" || m.sheet.Name() != "sales" || input(m, "A2") != "North" {
		t.Fatalf("inserted: %s on %s", sheetNames(m), m.sheet.Name())
	}
	if line(m, contextLine) != "Imported sales.csv as sales (2 rows)" || !m.changed || m.xfer.source != "" {
		t.Errorf("context %q changed %v source %q", line(m, contextLine), m.changed, m.xfer.source)
	}
	if v := m.book().Sheet(0).Value(addr("A1")); v.Num != 24 {
		t.Errorf("Sheet1!A1 reading the import = %v", v)
	}
	// Again: the name is taken, so the new sheet gets a number.
	importInto(t, m, "sales.csv", "Insert new sheet")
	if sheetNames(m) != "Sheet1,sales,sales 2" || m.sheet.Name() != "sales 2" {
		t.Errorf("second insert: %s", sheetNames(m))
	}
	press(t, m, "<ctrl+z>")
	if sheetNames(m) != "Sheet1,sales" || !strings.Contains(line(m, contextLine), "Undid: import sales.csv") {
		t.Errorf("undo: %s, %q", sheetNames(m), line(m, contextLine))
	}

	// Replace current sheet keeps the sheet's name and place.
	press(t, m, "<ctrl+pgup>")
	writeFile(t, "sales.csv", "Region,Total\nSouth,50\n")
	importInto(t, m, "sales.csv", "Replace current sheet")
	if sheetNames(m) != "Sheet1,sales" || m.sheet.Name() != "Sheet1" || input(m, "A2") != "South" || input(m, "A1") != "Region" {
		t.Fatalf("replaced: %s on %s, A2 %q", sheetNames(m), m.sheet.Name(), input(m, "A2"))
	}
	if line(m, contextLine) != "Imported sales.csv into Sheet1 (2 rows)" {
		t.Errorf("context %q", line(m, contextLine))
	}
	press(t, m, "<ctrl+z>")
	if m.sheet.Name() != "Sheet1" || input(m, "A1") != "=sales!B2*2" {
		t.Errorf("undo replace: on %s, A1 %q", m.sheet.Name(), input(m, "A1"))
	}
}

// An XLSX workbook inserts every sheet, names made unique, and offers no
// Replace current sheet.
func TestImportLocationWorkbook(t *testing.T) {
	t.Chdir(t.TempDir())
	src := sheet.New()
	book := src.Book()
	book.RenameSheet(src, "Sheet1")
	src.Set(addr("A1"), "1")
	q3, _ := book.AddSheet("Q3", 1)
	q3.Set(addr("A1"), "=Sheet1!A1+1")
	if _, err := fileio.Export(context.Background(), filepath.Join(".", "book.xlsx"), fileio.XLSX, fileio.SnapBook(src), fileio.ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	m := newModel()
	press(t, m, "x", "<enter>")
	m.runCommand("file.import")
	press(t, m, "book", "<enter>")
	if got := strings.Join(pickerTitles(t, m), "\n"); got != "Insert new sheets | after the others\nReplace spreadsheet | unsaved changes" {
		t.Errorf("locations:\n%s", got)
	}
	press(t, m, "insert", "<enter>")
	if sheetNames(m) != "Sheet1,Sheet1 2,Q3" || m.sheet.Name() != "Sheet1 2" {
		t.Fatalf("sheets %s on %s (%q)", sheetNames(m), m.sheet.Name(), m.errMsg)
	}
	if got := m.book().Lookup("Q3"); got.Cell(addr("A1")).Input != "='Sheet1 2'!A1+1" || got.Value(addr("A1")).Num != 2 {
		t.Errorf("Q3!A1 %q = %v", got.Cell(addr("A1")).Input, got.Value(addr("A1")))
	}
	if l := line(m, contextLine); l != "Imported book.xlsx as Sheet1 2 and Q3 (2 rows)" {
		t.Errorf("context %q", l)
	}
}
