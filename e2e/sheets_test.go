package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// summary adds a second sheet to the budget, renames it and fills it with
// formulas that read the budget, pointing into it the way a Sheets user
// clicks the other tab while typing.
func summary(s *session) {
	budget(s)
	s.keys("<shift+f11>")
	s.waitFor("Added Sheet2")
	s.keys("<ctrl+k>", "rename sheet", "<enter>")
	s.waitFor("Rename sheet: Sheet2")
	s.keys("Summary", "<enter>")
	s.waitFor("Renamed Sheet2 to Summary")
	s.keys("Yearly total", "<tab>", "=", "<ctrl+pgup>")
	s.waitForBar("Summary!B1", "=Sheet1!B7")
	s.keys("*12", "<enter>")
	s.keys("Monthly average", "<tab>", "=AVERAGE(", "<ctrl+pgup>", "<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>", ")", "<enter>")
	s.waitFor("719.46667")
	s.keys("<up>", "<up>", "<right>")
	s.waitForBar("B1", "=Sheet1!B7*12")
}

// tabX is the screen column of the middle of a tab on the status line.
func tabX(t *testing.T, s *session, name string) int {
	t.Helper()
	l := s.line(int(s.rows) - 1)
	i := strings.Index(l, " "+name+" ")
	if i < 0 {
		t.Fatalf("no tab %q in %q", name, l)
	}
	return len([]rune(l[:i])) + len([]rune(name))/2 + 1
}

func TestSheets(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir, "book.012")
	summary(s)
	if l := s.line(int(s.rows) - 1); !strings.HasPrefix(l, " Sheet1   Summary   +") {
		t.Errorf("tabs %q", l)
	}
	s.waitFor("25900.8")

	// A change on one sheet reaches formulas on the other.
	s.keys("<ctrl+pgup>")
	s.waitForBar("B5", "96") // where the pointer last was
	s.keys("0", "<enter>")
	s.keys("<alt+right>")
	s.waitFor("24748.8")

	// Renaming by double-clicking the tab rewrites the formulas.
	x := tabX(t, s, "Sheet1")
	s.click(ghostty.MouseButtonLeft, x, int(s.rows)-1)
	s.click(ghostty.MouseButtonLeft, x, int(s.rows)-1)
	s.waitFor("Rename sheet: Sheet1")
	s.keys("Budget 2026", "<enter>")
	s.keys("<ctrl+pgdown>")
	s.waitForBar("B1", "='Budget 2026'!B7*12")

	// The tab's menu deletes a sheet; its formulas show #REF! until undo.
	x = tabX(t, s, "Budget 2026")
	s.click(ghostty.MouseButtonRight, x, int(s.rows)-1)
	s.waitFor("Duplicate")
	s.keys("<down>", "<down>", "<enter>")
	s.waitFor("Delete Budget 2026 and its")
	s.keys("<enter>")
	s.waitFor("#REF!")
	s.keys("<ctrl+z>")
	s.waitForName("B6") // back on the restored sheet, where it was
	s.waitFor("2062.4")

	// Both sheets are saved, and come back.
	s.keys("<ctrl+s>")
	s.eventually("saved", func() bool {
		data, err := os.ReadFile(filepath.Join(dir, "book.012"))
		return err == nil && strings.Contains(string(data), `"version": 4`) && strings.Contains(string(data), `"name": "Summary"`)
	})
	s.keys("<ctrl+q>")
	s.waitExit()

	// The sheet shown when saved opens first.
	s = start(t, dir, "book.012")
	s.waitFor("2062.4")
	if l := s.line(int(s.rows) - 1); !strings.HasPrefix(l, " Budget 2026   Summary   +") {
		t.Errorf("tabs after opening %q", l)
	}
	s.keys("<alt+shift+k>")
	s.waitFor("Go to sheet")
	s.keys("summ", "<enter>")
	s.waitForBar("A1", "Yearly total")
	s.waitFor("24748.8")
}
