package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	ghostty "go.mitchellh.com/libghostty"
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
	s.waitFor("Out[1]:")
	s.waitFor("5.0 MB")
	s.keys("<enter>", "$sizes | where size > 1kb | sort-by size --reverse", "<shift+enter>")
	s.waitFor("Out[2]:")
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
	s.waitFor("[1]:")
}

// A cell of two statements, the second reading the variable the first
// assigns, runs with the real nu: its output is the second's, and a
// later cell reads the variable.
func TestNotebookStatementsWithNu(t *testing.T) {
	needNu(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "big.txt"), []byte(strings.Repeat("x", 3000)), 0o644)
	os.WriteFile(filepath.Join(dir, "small.txt"), []byte("x"), 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	s := startWith(t, options{dir: dir, startsOn: "EDIT"}, "nu")
	s.keys("files = ls | where type == file # not sub", "<enter>", "$files | where size > 1kb | sort-by size --reverse", "<shift+enter>")
	s.waitFor("Out[1]:")
	s.waitFor("big.txt")
	s.keys("<enter>", "$files | length", "<shift+enter>")
	s.waitFor("Out[2]:")
	scr := s.screen()
	if strings.Contains(scr, "failed") || strings.Contains(scr, "small.txt") {
		t.Errorf("screen:\n%s", scr)
	}
	s.eventually("the second cell reads 2 files", func() bool {
		for _, l := range strings.Split(s.screen(), "\n") {
			if strings.Contains(l, "Out[2]:") && strings.HasSuffix(strings.TrimSpace(l), " 2") {
				return true
			}
		}
		return false
	})
}

func TestNotebookSendsToSheetWithNu(t *testing.T) {
	needNu(t)
	s := startWith(t, options{startsOn: "EDIT"}, "nu")
	s.keys("n = [[k]; [1] [2] [3]]", "<ctrl+enter>")
	s.waitFor("Out[1]:")
	s.keys("G", "<enter>")
	s.waitFor("Sent n to")
	s.keys("<ctrl+pgdown>", "<ctrl+home>", "<right>", "<right>", "=SUM(nu.n)", "<enter>")
	s.waitFor("       6")
	s.keys("<ctrl+pgup>", "<enter>", " | append {k: 10}", "<ctrl+enter>")
	s.waitFor("Out[2]:")
	s.keys("<ctrl+pgdown>")
	s.waitFor("      16")
}

// With the real nu, a cell being written is highlighted by nu's shapes
// once typing pauses, and Tab completes what only nu knows: a flag.
func TestNotebookHighlightsAndCompletesWithNu(t *testing.T) {
	needNu(t)
	s := startWith(t, options{startsOn: "EDIT"}, "nu")
	s.keys("ls | where size > 1kb | sort-by size")
	s.eventually("nu's highlighting", func() bool { return nuHighlighted(s.html(), "sort-by", "size") })
	s.keys(" --rev", "<tab>")
	s.waitFor("sort-by size --reverse")
}

// With the real nu, the caret resting on a command says its signature
// on the context line, and on a flag what the flag does; F1 opens the
// command's help, with its page in nushell's docs as a link. On a
// cell's $name, the line says the cell and its output's shape.
func TestNotebookHoverWithNu(t *testing.T) {
	needNu(t)
	s := startWith(t, options{cols: 120, rows: 30, startsOn: "EDIT"}, "nu")
	s.keys("n = [[k v]; [1 a] [2 b] [3 c]]", "<shift+enter>")
	s.waitFor("Out[1]:")
	s.keys("<enter>", "$n | sort-by k --reverse", "<home>")
	s.waitFor("$n: table, 3 rows × 2 columns from cell 1")
	s.keys("<right>", "<right>", "<right>", "<right>", "<right>", "<right>")
	s.waitFor("sort-by <...comparator: cell-path|closure>")
	s.waitFor("Sort by the given cell path or closure.")
	s.keys("<end>")
	s.waitFor("sort-by --reverse, -r")
	s.keys("<f1>")
	s.waitFor("Docs: https://www.nushell.sh/commands/docs/sort-by.html")
	s.waitFor("HELP")
	if got := s.linkAt("https://www.nushell.sh"); got != "https://www.nushell.sh/commands/docs/sort-by.html" {
		t.Errorf("the docs link to %q", got)
	}
	s.keys("<esc>")
	s.waitFor("EDIT")
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
			got = append(got, strings.Fields(boxText(l))...)
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

// linkAt is the hyperlink under the first cell of text on the screen.
func (s *session) linkAt(text string) string {
	for y, line := range strings.Split(s.screen(), "\n") {
		if i := strings.Index(line, text); i >= 0 {
			s.mu.Lock()
			defer s.mu.Unlock()
			ref, err := s.vt.GridRef(ghostty.Point{Tag: ghostty.PointTagActive, X: uint16(utf8.RuneCountInString(line[:i])), Y: uint32(y)})
			if err != nil {
				return ""
			}
			u, _ := ref.HyperlinkURI()
			return u
		}
	}
	return ""
}
