package e2e

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// appendText adds text to the end of a file, as a program logging to it
// does.
func appendText(t *testing.T, name, text string) {
	t.Helper()
	f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

// linkTable links name at the active cell through the palette, keeping
// every row.
func (s *session) linkTable(name string) {
	s.t.Helper()
	s.linkTableAsk(name)
	s.keys("<enter>")
}

func TestFollowFileAsItGrows(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "app.csv")
	writeText(t, log, "level,ms\ninfo,12\n")
	s := start(t, dir)
	s.keys("<f5>", "D1", "<enter>", "=SUM(B:B)", "<enter>")
	s.keys("<f5>", "A1", "<enter>")
	s.linkTable("app.csv")
	s.waitFor("info")
	s.waitFor("Sheet1 ●")

	// Rows appended while 012 follows the file come in, a partial line
	// once it's whole, and the formula over them recalculates.
	for i := range 5 {
		appendText(t, log, fmt.Sprintf("warn,%d\n", 100+i))
	}
	appendText(t, log, "error,")
	s.waitFor("104")
	s.eventually("the sum of the rows", func() bool { return strings.Contains(s.line(4), "522") })
	if strings.Contains(s.screen(), "error") {
		t.Fatalf("a partial line was read:\n%s", s.screen())
	}
	appendText(t, log, "7\n")
	s.waitFor("error")
	s.eventually("the sum with the last row", func() bool { return strings.Contains(s.line(4), "529") })

	// The context line says what the region is doing.
	s.keys("<f5>", "A3", "<enter>")
	s.waitFor("● Following app.csv  7 rows")

	// Truncated and written again, it's read from the start.
	writeText(t, log, "level,ms\nreset,1\n")
	s.waitFor("reset")
	s.eventually("the old rows gone", func() bool { return !strings.Contains(s.screen(), "104") })

	// Paused, it waits; unlinked, the values stay as they are.
	s.keys("<ctrl+k>", "Follow", "<enter>")
	s.waitFor("Paused following app.csv")
	appendText(t, log, "late,2\n")
	s.keys("<up>") // A2, in the region
	s.waitFor("‖ Paused app.csv")
	if strings.Contains(s.screen(), "late") {
		t.Fatalf("a paused region read on:\n%s", s.screen())
	}
	s.keys("<ctrl+k>", "Unlink", "<enter>")
	s.waitFor("Unlinked app.csv")
	s.waitFor("Sheet1 ")
	if strings.Contains(s.line(29), "●") {
		t.Errorf("tab still marked: %q", s.line(29))
	}
}
