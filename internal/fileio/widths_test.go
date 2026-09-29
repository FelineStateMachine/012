package fileio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Formats without widths of their own fit their columns to the header
// and the rows below it, capped, and leave narrow columns at the
// default; read from a stream too.
func TestImportFitsWidths(t *testing.T) {
	long := strings.Repeat("x", 60)
	files := map[string]string{
		"t.csv":   "Item,Qty,Note\nStapler with a long name,3," + long + "\n",
		"t.tsv":   "Item\tQty\tNote\nStapler with a long name\t3\t" + long + "\n",
		"t.json":  `[{"Item":"Stapler with a long name","Qty":3,"Note":"` + long + `"}]`,
		"t.nuon":  `[[Item, Qty, Note]; ["Stapler with a long name", 3, "` + long + `"]]`,
		"t.jsonl": `{"Item":"Stapler with a long name","Qty":3,"Note":"` + long + `"}`,
	}
	check := func(t *testing.T, s *sheet.Sheet) {
		t.Helper()
		if got := s.ColWidth(0); got != len("Stapler with a long name")+2 {
			t.Errorf("Item is %d wide", got)
		}
		if got := s.ColWidth(1); got != sheet.DefaultWidth {
			t.Errorf("Qty is %d wide, want the default", got)
		}
		if got := s.ColWidth(2); got != 30 {
			t.Errorf("Note is %d wide, want the cap", got)
		}
	}
	dir := t.TempDir()
	for name, text := range files {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name)
			os.WriteFile(path, []byte(text), 0o600)
			r, err := Import(context.Background(), path, Options{})
			if err != nil {
				t.Fatal(err)
			}
			check(t, r.Sheet)
			r, err = ImportReader(context.Background(), "stdin", strings.NewReader(text), Options{})
			if err != nil {
				t.Fatal(err)
			}
			check(t, r.Sheet)
		})
	}
}

// A format that keeps widths keeps them, and columns it leaves at the
// default stay there.
func TestImportKeepsFileWidths(t *testing.T) {
	s := sheet.New()
	s.Set(sheet.Addr{}, strings.Repeat("x", 40))
	path := filepath.Join(t.TempDir(), "t.xlsx")
	if _, err := Export(context.Background(), path, XLSX, SnapBook(s), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	r, err := Import(context.Background(), path, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if got := r.Sheet.ColWidth(0); got != sheet.DefaultWidth {
		t.Errorf("A is %d wide, want the file's default", got)
	}
}
