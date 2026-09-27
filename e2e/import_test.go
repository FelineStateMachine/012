package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/xuri/excelize/v2"
)

// writeXLSX writes a workbook with a formatted quarter on its first sheet
// and a second sheet that 012 doesn't import yet.
func writeXLSX(t *testing.T, name string) {
	t.Helper()
	x := excelize.NewFile()
	defer x.Close()
	x.SetSheetName("Sheet1", "Q3")
	x.NewSheet("Q4")
	rows := [][]any{
		{"Region", "July", "August", "September"},
		{"North", 1200.5, 1310, 990},
		{"South", 870, 905.25, 1122},
		{"West", 450, 610, 702.75},
	}
	for r, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, r+1)
		x.SetSheetRow("Q3", cell, &row)
	}
	x.SetCellValue("Q3", "A5", "Total")
	for _, c := range []string{"B", "C", "D"} {
		x.SetCellFormula("Q3", c+"5", fmt.Sprintf("SUM(%s2:%s4)", c, c))
	}
	bold, _ := x.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}})
	x.SetCellStyle("Q3", "A1", "D1", bold)
	x.SetCellStyle("Q3", "A5", "A5", bold)
	code := `\$#,##0.00`
	money, _ := x.NewStyle(&excelize.Style{CustomNumFmt: &code})
	x.SetCellStyle("Q3", "B2", "D5", money)
	x.SetColWidth("Q3", "A", "A", 12)
	x.SetCellValue("Q4", "A1", "later")
	if err := x.SaveAs(name); err != nil {
		t.Fatal(err)
	}
}

func writeText(t *testing.T, name, text string) {
	t.Helper()
	if err := os.WriteFile(name, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestOpenXLSXFromCommandLine(t *testing.T) {
	dir := t.TempDir()
	writeXLSX(t, filepath.Join(dir, "q3.xlsx"))
	s := start(t, dir, "q3.xlsx")
	s.waitFor("Imported q3.xlsx (6 rows)")
	s.waitFor("$1,200.50")
	// Every sheet comes in, each with its tab.
	if !strings.HasPrefix(s.line(29), " Q3   Q4   +") {
		t.Errorf("tabs %q", s.line(29))
	}
	s.keys("<ctrl+pgdown>")
	s.waitForBar("A1", "later")
	s.keys("<ctrl+pgup>")
	s.keys("<f5>", "D5", "<enter>")
	s.waitForBar("D5", "=SUM(D2:D4)")
	s.waitFor("$2,814.75")
	if !strings.Contains(s.line(29), "q3.xlsx") {
		t.Errorf("status line %q", s.line(29))
	}
	// Save asks for a .012 name or a download back to Excel.
	s.keys("<ctrl+s>")
	s.waitFor("q3.xlsx was imported.")
	s.waitFor("Download as XLSX")
	s.keys("<enter>")
	s.waitFor("Save as: q3.012")
	s.keys("<enter>")
	s.eventually("q3.012 saved", func() bool {
		_, err := os.Stat(filepath.Join(dir, "q3.012"))
		return err == nil
	})
}

func TestOpenCSVFromCommandLine(t *testing.T) {
	dir := t.TempDir()
	writeText(t, filepath.Join(dir, "prices.csv"), "\xEF\xBB\xBFItem;Price;Date\nTea;$3.50;9/26/2026\nCake;4;2026-09-27\n")
	s := start(t, dir, "prices.csv")
	s.waitFor("Imported prices.csv (3 rows); separated by semicolons")
	s.waitFor("$3.50")
	s.waitFor("9/26/2026")
	s.keys("<down>", "<down>", "<right>", "<right>")
	s.waitForBar("C3", "2026-09-27")

	// Download it back as TSV from the menus.
	s.keys("<ctrl+k>", "tsv", "<enter>")
	s.waitFor("Download as TSV: prices.tsv")
	s.keys("<enter>")
	s.waitFor("Downloaded prices.tsv (3 rows)")
	data, err := os.ReadFile(filepath.Join(dir, "prices.tsv"))
	if err != nil || string(data) != "Item\tPrice\tDate\nTea\t$3.50\t9/26/2026\nCake\t4\t2026-09-27\n" {
		t.Errorf("prices.tsv %q %v", data, err)
	}
}

func TestOpenMissingImport(t *testing.T) {
	dir := t.TempDir()
	out, err := runBinary(dir, "nope.xlsx")
	if err == nil || !strings.Contains(out, "nope.xlsx: no such file") {
		t.Errorf("missing file: %v %q", err, out)
	}
}

// importDir fills dir with one file of each kind for the import picker.
func importDir(t *testing.T, dir string) {
	t.Helper()
	writeXLSX(t, filepath.Join(dir, "q3.xlsx"))
	writeText(t, filepath.Join(dir, "sales-2026.csv"), strings.Repeat("North,1200,9/26/2026\n", 40))
	writeText(t, filepath.Join(dir, "budget.tsv"), "Rent\t1450\nFood\t612\n")
	writeText(t, filepath.Join(dir, "BUDGET86.WK1"), strings.Repeat("\x00", 2400))
	writeText(t, filepath.Join(dir, "shop.sqlite"), strings.Repeat("s", 8192))
	writeText(t, filepath.Join(dir, "trips.parquet"), strings.Repeat("p", 1_250_000))
	writeText(t, filepath.Join(dir, "notes.txt"), "not listed")
	writeText(t, filepath.Join(dir, "budget.012"), "{}")
}

func TestImportPickerLists(t *testing.T) {
	dir := t.TempDir()
	importDir(t, dir)
	s := start(t, dir)
	s.keys("<alt+f>", "<down>", "<down>", "<enter>")
	s.waitFor("Lotus 1-2-3 worksheet")
	scr := s.screen()
	for _, want := range []string{"BUDGET86.WK1", "budget.tsv", "q3.xlsx", "sales-2026.csv", "shop.sqlite", "trips.parquet", "6 of 6"} {
		if !strings.Contains(scr, want) {
			t.Errorf("picker lacks %q:\n%s", want, scr)
		}
	}
	if strings.Contains(scr, "notes.txt") || strings.Contains(scr, "budget.012") {
		t.Errorf("picker lists other files:\n%s", scr)
	}
	s.keys("tsv", "<enter>")
	s.waitFor("Imported budget.tsv (2 rows)")
}

// slowCSV imports from a named pipe that the test fills in two parts, so
// the import stops half way with a known row count.
func slowCSV(s *session) {
	fifo := filepath.Join(s.dir, "big.csv")
	if err := syscall.Mkfifo(fifo, 0o644); err != nil {
		s.t.Fatal(err)
	}
	done := make(chan struct{})
	s.t.Cleanup(func() { close(done) })
	go func() {
		f, err := os.OpenFile(fifo, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		defer f.Close()
		for i := range 2048 {
			fmt.Fprintf(f, "%d,North,%d.50,9/26/2026,a longer note to fill the pipe\n", i, i*3)
		}
		<-done
	}()
	s.keys("<alt+f>", "<down>", "<down>", "<enter>", "./big.csv", "<enter>")
	s.waitFor("2,048 rows read")
}

// runBinary runs 012 without a terminal, for errors reported before the
// screen starts.
func runBinary(dir string, args ...string) (string, error) {
	cmd := exec.Command(binPath, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestImportProgressAndCancel(t *testing.T) {
	s := start(t, "")
	slowCSV(s)
	s.waitFor("WAIT")
	s.waitFor("Importing big.csv…")
	s.keys("<esc>")
	s.waitFor("Import of big.csv cancelled")
	s.waitFor("READY")
}

// File > Import on a spreadsheet with something in it asks where the
// file goes: new sheets after the one shown (every sheet of an .xlsx),
// undone as one step, or in place of the sheet shown.
func TestImportLocation(t *testing.T) {
	dir := t.TempDir()
	importDir(t, dir)
	s := start(t, dir)
	s.keys("<shift+f11>", "<ctrl+pgup>", "Notes", "<enter>")
	openImportPicker(s)
	s.keys("q3", "<enter>")
	s.waitFor("Import location")
	scr := s.screen()
	if !strings.Contains(scr, "Insert new sheets") || !strings.Contains(scr, "Replace spreadsheet") || strings.Contains(scr, "Replace current sheet") {
		t.Errorf("locations for an .xlsx:\n%s", scr)
	}
	s.keys("<enter>")
	s.waitFor("Imported q3.xlsx as Q3 and Q4")
	if l := s.line(int(s.rows) - 1); !strings.HasPrefix(l, " Sheet1   Q3   Q4   Sheet2   +") {
		t.Errorf("tabs %q", l)
	}
	s.waitFor("$1,200.50")
	s.keys("<ctrl+z>")
	s.waitFor("Undid: import q3.xlsx")
	if l := s.line(int(s.rows) - 1); !strings.HasPrefix(l, " Sheet1   Sheet2   +") {
		t.Errorf("tabs after undo %q", l)
	}

	openImportPicker(s)
	s.keys("tsv", "<enter>")
	s.waitFor("Import location")
	s.keys("current", "<enter>")
	s.waitFor("Imported budget.tsv into Sheet1 (2 rows)")
	s.waitForBar("A1", "Rent")
	s.keys("<ctrl+z>")
	s.waitFor("Undid: import budget.tsv")
	s.keys("<ctrl+home>")
	s.waitForBar("A1", "Notes")
}
