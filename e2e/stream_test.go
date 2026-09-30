package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A cell run as a stream with the real nu follows a log as it's
// written: its rows reach the output and the sheet it was sent to as
// lines are appended, and Stop ends it, keeping them.
func TestNotebookStreamWithNu(t *testing.T) {
	needNu(t)
	dir := t.TempDir()
	logFile := filepath.Join(dir, "app.log")
	if err := os.WriteFile(logFile, []byte("info started\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := startWith(t, options{dir: dir, startsOn: "EDIT"}, "nu")
	s.keys("log = tail -n +1 -f app.log | lines | parse '{level} {msg}'", "<esc>", "f")
	s.waitFor("● live, 1 row")
	s.waitFor("started")

	s.keys("G", "<enter>")
	s.waitFor("Sent log to")
	appendFile(t, logFile, "warn slow-query\nerror disk-full\n")
	s.waitFor("● live, 3 rows")
	s.waitFor("disk-full")

	s.keys("<ctrl+pgdown>", "<ctrl+home>", "<right>", "<right>", "<right>", "=ROWS(nu.log)", "<enter>")
	s.waitFor("       4")
	appendFile(t, logFile, "info recovered\n")
	s.waitFor("       5")

	s.keys("<ctrl+pgup>", "i", "i")
	s.waitFor("✓")
	s.eventually("the stream stopped", func() bool { return !strings.Contains(s.screen(), "● live") })
	appendFile(t, logFile, "info after-stop\n")
	s.keys("<ctrl+pgdown>")
	s.waitFor("       5")
	time.Sleep(time.Second)
	if strings.Contains(s.screen(), "       6") {
		t.Errorf("rows arrived after Stop:\n%s", s.screen())
	}
	s.keys("<ctrl+q>", "d")
	s.waitExit()
}

func appendFile(t *testing.T, name, text string) {
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
