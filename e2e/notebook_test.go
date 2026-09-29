package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Notebooks with the real nu: a region, one reading it, saving, and the
// file reopened with its regions not run until F9.

func needNu(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("nu"); err != nil {
		t.Skip("nu isn't installed")
	}
}

func TestNotebookWithNu(t *testing.T) {
	needNu(t)
	dir := t.TempDir()
	s := startWith(t, options{dir: dir, startsOn: "nu❯"}, "nu", "book.012")
	s.keys("[[name size]; [a 2kb] [b 10b] [c 5mb]]", "<enter>")
	s.waitFor("r1: 3 rows")
	s.waitFor("5.0 MB")
	s.keys("big = $r1 | where size > 1kb | sort-by size --reverse", "<enter>")
	s.waitFor("big: 2 rows")
	s.keys("nope-not-a-command", "<enter>")
	s.waitFor("r2 failed:")
	s.keys("<esc>", "<ctrl+s>")
	s.eventually("saved notebook", func() bool {
		data, err := os.ReadFile(filepath.Join(dir, "book.012"))
		return err == nil && strings.Contains(string(data), `"reads":["r1"]`)
	})
	s.keys("<ctrl+q>")
	s.waitExit()

	s = startWith(t, options{dir: dir, startsOn: "nu❯"}, "nu", "book.012")
	s.waitFor("r1  [[name size]; [a 2kb] [b 10b] [c 5mb]]   not run")
	s.keys("<esc>", "<f9>")
	s.waitFor("big  $r1 | where size > 1kb | sort-by size --reverse\n")
	s.waitFor("5.0 MB")
}

// ! in an ordinary workbook makes a Shell 1 sheet, and $in is the
// selection.
func TestShellFromWorkbook(t *testing.T) {
	needNu(t)
	s := start(t, "")
	s.keys("n", "<enter>", "3", "<enter>", "4", "<enter>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>", "<shift+down>")
	s.waitFor(" A1:A4 ")
	s.keys("!", "$in | math sum", "<enter>")
	s.waitFor("r1: 1 row")
	s.waitFor("Shell 1")
	s.waitFor("       7")
}
