package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recordTotals records a macro with relative references at A1: a label,
// a formula to its right, and bold on both. It's saved as Totals with
// the shortcut Ctrl+Alt+Shift+1.
func recordTotals(s *session) {
	s.keys("<ctrl+k>", "relative references")
	s.waitFor("Record macro with relative references")
	s.keys("<enter>")
	s.waitFor(" REC ")
	s.waitFor("Recording (relative references)")
	s.keys("Total", "<tab>", "=SUM(2,3)", "<enter>")
	s.keys("<up>", "<shift+right>", "<ctrl+b>")
	s.waitFor("Bold on for A1:B1")
	s.keys("<ctrl+k>", "stop and save", "<enter>")
	s.waitFor("Save macro as: Macro 1")
	s.keys("Totals", "<enter>")
	s.waitFor("Shortcut Ctrl+Alt+Shift+ (a digit, or empty for none): 1")
	s.keys("<enter>")
	s.waitFor("Saved macro Totals; Ctrl+Alt+Shift+1 runs it")
}

func TestMacroRecordAndReplay(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := start(t, dir, "book.012")
	recordTotals(s)
	if strings.Contains(s.line(0), "REC") {
		t.Errorf("still shows REC: %q", s.line(0))
	}

	// The shortcut replays it from wherever the active cell is.
	s.keys("<f5>", "D5", "<enter>")
	s.waitForBar("D5", "")
	s.keys("<ctrl+alt+shift+1>")
	s.waitFor("Ran Totals")
	s.keys("<right>")
	s.waitForBar("E5", "=SUM(2,3)")

	// One Ctrl+Z undoes the whole run.
	s.keys("<ctrl+z>")
	s.waitFor("Undid: run macro Totals")
	s.waitForBar("D5", "")

	// Saved in the file, as a script.
	s.keys("<ctrl+s>")
	s.eventually("saved", func() bool {
		data, err := os.ReadFile(filepath.Join(dir, "book.012"))
		return err == nil && strings.Contains(string(data), `"macros": [`) &&
			strings.Contains(string(data), `{"name": "Totals", "key": "1", "api": 1, "source": "# Recorded with relative references`) &&
			strings.Contains(string(data), `enter(\"=SUM(2,3)\", origin=\"B1\")`)
	})
	s.keys("<ctrl+q>")
	s.waitExit()

	// Opening the file runs nothing; on this computer the macro runs
	// without asking.
	s = start(t, dir, "book.012")
	s.waitForBar("A1", "Total")
	s.keys("<f5>", "A3", "<enter>")
	s.waitForBar("A3", "")
	s.keys("<ctrl+k>", "totals")
	s.waitFor("Ctrl+Alt+Shift+1")
	s.keys("<enter>")
	s.waitFor("Ran Totals")
	s.waitForBar("A3:B3", "Total")
}

func TestMacroFromAnotherComputerAsks(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := start(t, dir, "book.012")
	recordTotals(s)
	s.keys("<ctrl+s>")
	s.eventually("saved", func() bool {
		data, err := os.ReadFile(filepath.Join(dir, "book.012"))
		return err == nil && strings.Contains(string(data), "macroOrigin")
	})
	s.keys("<ctrl+q>")
	s.waitExit()

	s = startWith(t, options{dir: dir, configDir: t.TempDir()}, "book.012")
	s.waitForBar("A1", "Total")
	s.keys("<f5>", "C3", "<enter>", "<ctrl+alt+shift+1>")
	s.waitFor("Trust this file's macros?")
	s.keys("<esc>")
	s.waitForBar("C3", "")
	s.keys("<ctrl+alt+shift+1>")
	s.waitFor("made on another computer and can change the file")
	s.keys("<enter>")
	s.waitForBar("C3:D3", "Total") // the macro ends selecting the pair
	// Trusted now: no second question.
	s.keys("<down>", "<ctrl+alt+shift+1>")
	s.waitForBar("C4:D4", "Total")
}

// TestMacroRecordsTheSortBar records the choices made in a dialog and
// replays them.
func TestMacroRecordsTheSortBar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	s := start(t, dir, "book.012")
	s.keys("Month", "<tab>", "Sales", "<enter>", "Jan", "<tab>", "10", "<enter>", "Feb", "<tab>", "30", "<enter>", "Mar", "<tab>", "20", "<enter>")
	s.keys("<ctrl+home>", "<ctrl+k>", "record macro")
	s.waitFor("Record macro")
	s.keys("<enter>")
	s.waitFor("Recording (absolute references)")
	s.keys("<ctrl+k>", "sort range", "<enter>")
	s.waitFor("Sort A2:B4 by  A Month  A→Z")
	s.keys("<right>", "<space>", "<enter>")
	s.waitFor("Sorted A2:B4 by B Z→A")
	s.waitForLine(gridRow1+1, "    2  Feb             30")
	s.keys("<ctrl+k>", "stop and save", "<enter>")
	s.waitFor("Save macro as: Macro 1")
	s.keys("Sort", "<enter>", "<enter>")
	s.waitFor("Saved macro Sort")

	// Sorted back by Sales A to Z, then replayed from the palette.
	s.keys("<right>", "<ctrl+k>", "sort range a to z", "<enter>")
	s.waitForLine(gridRow1+1, "    2  Jan             10")
	s.keys("<left>")
	s.keys("<ctrl+k>", "sort", "<enter>")
	s.waitFor("Ran Sort")
	s.waitForLine(gridRow1+1, "    2  Feb             30")
	s.keys("<ctrl+s>")
	s.eventually("saved", func() bool {
		data, err := os.ReadFile(filepath.Join(dir, "book.012"))
		return err == nil && strings.Contains(string(data), `run(\"data.sort_range\", answer={\"by\": [{\"column\": \"B\", \"order\": \"desc\"}], \"header\": True})`)
	})
}
