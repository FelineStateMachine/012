package fileio

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/stress"
)

// TestXLSXDifferential imports a corpus of workbooks with excelize (the
// reader 012 used before its own) and with 012's streaming reader, and
// checks the results are the same: every sheet's cells as saved (input,
// format and style), column widths, named ranges, the sheet shown, the
// rows counted and the notes. The corpus is workbooks written here, the
// stress datasets' XLSX files when fetched (scripts/stress-data.sh), and
// every .xlsx file in the directories listed in XLSX_CORPUS
// (colon-separated), such as excelize's own test files.
//
// Known differences, where excelize's reading is wrong and 012's isn't,
// are left out of the comparison (see compareXLSX).
func TestXLSXDifferential(t *testing.T) {
	for _, path := range xlsxCorpus(t) {
		t.Run(filepath.Base(path), func(t *testing.T) {
			want, errWant := importXLSX(context.Background(), path, Options{})
			got, errGot := importXLSXStream(context.Background(), path, Options{})
			if (errWant == nil) != (errGot == nil) {
				t.Fatalf("excelize: %v; 012: %v", errWant, errGot)
			}
			if errWant != nil {
				t.Logf("both fail: excelize: %v; 012: %v", errWant, errGot)
				return
			}
			compareXLSX(t, path, want, got)
		})
	}
}

func xlsxCorpus(t *testing.T) []string {
	dir := t.TempDir()
	files := []string{writeParts(t, dir, "kitchen.xlsx", kitchenParts())}
	files = append(files, writeExcelizeBook(t, dir), writeExportedBook(t, dir))
	stressData := os.Getenv("STRESS_DIR")
	if stressData == "" {
		stressData = filepath.Join("..", "..", ".deps", "stress")
	}
	files = append(files, globXLSX(stressData)...)
	for _, d := range filepath.SplitList(os.Getenv("XLSX_CORPUS")) {
		files = append(files, globXLSX(d)...)
	}
	return files
}

func globXLSX(dir string) []string {
	if dir == "" {
		return nil
	}
	m, _ := filepath.Glob(filepath.Join(dir, "*.xlsx"))
	return m
}

// writeExcelizeBook writes a workbook with excelize: sheets referring to
// each other, styles, widths and names.
func writeExcelizeBook(t *testing.T, dir string) string {
	x := excelize.NewFile()
	defer x.Close()
	x.SetSheetName("Sheet1", "Q1")
	x.NewSheet("Q2 plan")
	x.SetSheetRow("Q1", "A1", &[]any{"Region", 1234.5, "=text", 45000, true, "12%", "TRUE", -0.5})
	x.SetCellFormula("Q1", "A2", "'Q2 plan'!A1*2")
	x.SetCellFormula("Q1", "B2", "SUM(B1,1)")
	x.SetCellFormula("Q1", "C2", "CUBEVALUE(1)")
	x.SetCellValue("Q1", "E4", "after a gap")
	x.SetCellValue("Q2 plan", "A1", 42)
	x.SetCellValue("Q2 plan", "B300", "far down")
	bold, _ := x.NewStyle(&excelize.Style{NumFmt: 4, Font: &excelize.Font{Bold: true, Underline: "double"}})
	date, _ := x.NewStyle(&excelize.Style{NumFmt: 15, Alignment: &excelize.Alignment{Horizontal: "right"}})
	code := `[$$-409]#,##0.00`
	money, _ := x.NewStyle(&excelize.Style{CustomNumFmt: &code, Font: &excelize.Font{Italic: true, Strike: true}})
	x.SetCellStyle("Q1", "B1", "B1", bold)
	x.SetCellStyle("Q1", "D1", "D1", date)
	x.SetCellStyle("Q1", "A2", "C3", money)
	x.SetColWidth("Q1", "A", "B", 14)
	x.SetColWidth("Q2 plan", "C", "C", 30)
	x.SetDefinedName(&excelize.DefinedName{Name: "Rent", RefersTo: "Q1!$B$1"})
	x.SetDefinedName(&excelize.DefinedName{Name: "Mine", RefersTo: "Q1!$B$1", Scope: "Q2 plan"})
	x.SetActiveSheet(1)
	path := filepath.Join(dir, "excelize.xlsx")
	if err := x.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	return path
}

// writeExportedBook writes a table and a sheet of formulas with 012's
// exporter.
func writeExportedBook(t *testing.T, dir string) string {
	s := stress.Table(60, 10)
	book := s.Book()
	f, _ := book.AddSheet("Formulas", 1)
	for row := range 40 {
		f.Set(sheet.Addr{Col: 0, Row: row}, "=Sheet1!C"+itoa(row+2)+"*2")
		f.Set(sheet.Addr{Col: 1, Row: row}, "=SUM($A$1:A"+itoa(row+1)+")")
	}
	f.SetColWidth(0, 20)
	path := filepath.Join(dir, "exported.xlsx")
	if _, err := Export(context.Background(), path, XLSX, SnapBook(f), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	return path
}

// compareXLSX compares two imports of path.
func compareXLSX(t *testing.T, path string, want, got *Result) {
	t.Helper()
	if want.Rows != got.Rows {
		t.Errorf("rows %d, excelize %d", got.Rows, want.Rows)
	}
	if w, g := strings.Join(want.Notes, "; "), strings.Join(withoutHidden(got.Notes), "; "); w != g {
		t.Errorf("notes %q, excelize %q", g, w)
	}
	if want.Sheet.Name() != got.Sheet.Name() {
		t.Errorf("showing %s, excelize %s", got.Sheet.Name(), want.Sheet.Name())
	}
	skip := mergedCells(t, path)
	w, g := savedLines(t, want.Sheet, skip), savedLines(t, got.Sheet, skip)
	if w == g {
		return
	}
	wl, gl := strings.Split(w, "\n"), strings.Split(g, "\n")
	shown := 0
	for i := 0; i < max(len(wl), len(gl)) && shown < 20; i++ {
		var a, b string
		if i < len(wl) {
			a = wl[i]
		}
		if i < len(gl) {
			b = gl[i]
		}
		if a != b && !sameSharedFormula(a, b) {
			t.Errorf("line %d:\n  012:      %s\n  excelize: %s", i+1, b, a)
			shown++
		}
	}
}

// sameSharedFormula reports whether two saved lines are the same formula
// but for spaces. excelize expands a shared formula by parsing it and
// writing it back, which drops the spaces between its tokens; 012 moves
// the references in the text as written.
func sameSharedFormula(a, b string) bool {
	return strings.Contains(a, `": "=`) && strings.ReplaceAll(a, " ", "") == strings.ReplaceAll(b, " ", "")
}

func withoutHidden(notes []string) []string {
	var out []string
	for _, n := range notes {
		if !strings.Contains(n, "hidden sheet") {
			out = append(out, n)
		}
	}
	return out
}

// savedLines is the workbook as 012 saves it, without the cells in skip
// (by sheet), which excelize reads wrongly.
func savedLines(t *testing.T, s *sheet.Sheet, skip map[string][]sheet.Rect) string {
	t.Helper()
	book := s.Book()
	for i := range book.Len() {
		ws := book.Sheet(i)
		for _, r := range skip[ws.Name()] {
			for _, a := range ws.Addrs() {
				if r.Contains(a) && a != r.From {
					ws.EraseRange(sheet.NewRect(a, a))
				}
			}
		}
	}
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

// mergedCells are the merged ranges of each sheet. excelize reads any
// cell of a merged range as its first cell, so a formula there spreads
// to the others (when a cell after them in the row has a value); 012
// reads each cell as written.
func mergedCells(t *testing.T, path string) map[string][]sheet.Rect {
	x, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer x.Close()
	out := map[string][]sheet.Rect{}
	for _, ws := range x.GetSheetList() {
		merged, _ := x.GetMergeCells(ws)
		for _, m := range merged {
			if r, ok := sheet.ParseRange(m.GetStartAxis() + ":" + m.GetEndAxis()); ok {
				out[ws] = append(out[ws], r)
			}
		}
	}
	return out
}

func itoa(n int) string { return numInput(float64(n)) }
