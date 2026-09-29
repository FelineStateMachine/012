package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Notebooks: cells written and run with the real nu, one reading
// another's output, saved with their outputs and opened again without
// running; an output sent to a sheet and kept up to date; the selection
// read as $selection; and a long pipeline wrapped, never cut.

func needNu(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("nu"); err != nil {
		t.Skip("nu isn't installed")
	}
}

func TestNotebookWithNu(t *testing.T) {
	needNu(t)
	dir := t.TempDir()
	s := startWith(t, options{dir: dir, startsOn: "EDIT"}, "nu", "book.012")
	s.keys("sizes = [[name size]; [a 2kb] [b 10b] [c 5mb]]", "<shift+enter>")
	s.waitFor("[1] sizes")
	s.waitFor("5.0 MB")
	s.keys("<enter>", "$sizes | where size > 1kb | sort-by size --reverse", "<shift+enter>")
	s.waitFor("[2]")
	s.keys("<enter>", "nope-not-a-command", "<ctrl+enter>")
	s.waitFor("failed")
	s.keys("<ctrl+s>")
	s.eventually("saved notebook", func() bool {
		data, err := os.ReadFile(filepath.Join(dir, "book.012"))
		return err == nil && strings.Contains(string(data), `"notebookCells"`) && strings.Contains(string(data), "5000000b")
	})
	s.keys("<ctrl+q>")
	s.waitExit()

	s = startWith(t, options{dir: dir, startsOn: "NOTEBOOK"}, "nu", "book.012")
	s.waitFor("5.0 MB") // the outputs, saved, show without running
	s.waitFor("saved")
	s.keys("<f9>")
	s.waitFor("[1] sizes")
}

func TestNotebookSendsToSheetWithNu(t *testing.T) {
	needNu(t)
	s := startWith(t, options{startsOn: "EDIT"}, "nu")
	s.keys("n = [[k]; [1] [2] [3]]", "<ctrl+enter>")
	s.waitFor("[1] n")
	s.keys("G", "<enter>")
	s.waitFor("Sent n to")
	s.keys("<ctrl+pgdown>", "<ctrl+home>", "<right>", "<right>", "=SUM(nu.n)", "<enter>")
	s.waitFor("       6")
	s.keys("<ctrl+pgup>", "<enter>", " | append {k: 10}", "<ctrl+enter>")
	s.waitFor("[2] n")
	s.keys("<ctrl+pgdown>")
	s.waitFor("      16")
}

// Data > Shell from a workbook makes a Notebook tab; $selection is
// the range selected on the sheet shown before it.
func TestNotebookReadsSelection(t *testing.T) {
	needNu(t)
	s := start(t, "")
	s.keys("n", "<enter>", "3", "<enter>", "4", "<enter>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>", "<shift+down>")
	s.waitFor(" A1:A4 ")
	s.keys("<ctrl+k>", "Open notebook", "<enter>")
	s.waitFor("NOTEBOOK")
	s.keys("!", "$selection | get n | math sum", "<ctrl+enter>")
	s.waitFor("[1]")
	s.waitFor("  7")
	if !strings.Contains(s.screen(), "Notebook") {
		t.Errorf("no Notebook tab:\n%s", s.screen())
	}
}

// A 200-character pipeline at 80 columns is on the screen whole, wrapped
// before its pipes, while it's edited and once it isn't.
func TestNotebookLongPipeline(t *testing.T) {
	s := startWith(t, options{cols: 80, rows: 24, startsOn: "EDIT", env: fakeNu}, "nu")
	var b strings.Builder
	b.WriteString("ls")
	for i := 0; b.Len() < 200; i++ {
		b.WriteString(" | where size > " + strings.Repeat("9", i%4+1) + "kb")
	}
	src := b.String()
	s.keys(src)
	whole := func() bool {
		var got []string
		for _, l := range strings.Split(s.screen(), "\n") {
			if rest, ok := strings.CutPrefix(l, "│ "); ok {
				got = append(got, strings.Fields(rest)...)
			}
		}
		return strings.Join(got, " ") == strings.Join(strings.Fields(src), " ")
	}
	s.eventually("the whole pipeline, edited", whole)
	s.keys("<esc>")
	s.waitFor("NOTEBOOK")
	if !whole() {
		t.Errorf("the pipeline isn't whole after editing:\n%s", s.screen())
	}
}
