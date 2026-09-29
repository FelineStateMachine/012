package e2e

import (
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// The notebook as in Jupyter, through libghostty: ▶ runs a cell, a
// click in a box edits it, the toolbar adds a cell, Shift+click selects
// two cells that Alt+Down moves together, and o folds an output.

// boxText is the text inside a code cell's box on screen line l, or "":
// the box's left side is at column 10, after the bar and the prompt.
func boxText(l string) string {
	rs := []rune(l)
	const at = 10
	if len(rs) <= at || rs[at] != '│' && rs[at] != '┃' {
		return ""
	}
	rest := string(rs[at+1:])
	if j := strings.LastIndexAny(rest, "│┃"); j >= 0 {
		rest = rest[:j]
	}
	return strings.TrimSpace(rest)
}

// rowOf is the screen row holding text, and the column it starts at.
func (s *session) rowOf(text string) (col, row int) {
	s.t.Helper()
	for y, l := range strings.Split(s.screen(), "\n") {
		if i := strings.Index(l, text); i >= 0 {
			return len([]rune(l[:i])), y
		}
	}
	s.t.Fatalf("no line holds %q:\n%s", text, s.screen())
	return 0, 0
}

// codes are the code in the cells' boxes, top to bottom.
func (s *session) codes() []string {
	var out []string
	for _, l := range strings.Split(s.screen(), "\n") {
		if t := boxText(l); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// An output is 012's grid: Enter works in it, Shift+arrows select,
// Ctrl+C copies, the palette's sort sorts it, a click moves the active
// cell, Enter shows it full-screen and Esc goes back a level at a time.
func TestNotebookOutputGrid(t *testing.T) {
	s := startWith(t, options{cols: 100, rows: 40, startsOn: "EDIT", env: fakeNu}, "nu")
	clip := s.watchClipboard()
	s.keys("files = ls", "<ctrl+enter>")
	s.waitFor("Out[1]:")
	s.keys("<esc>", "<down>", "<enter>")
	s.waitFor("OUTPUT")
	s.keys("<shift+down>", "<shift+right>", "<ctrl+c>")
	s.eventually("the rows copied", func() bool { return clip.get() == "012\tfile\nCLAUDE.md\tfile" })
	s.waitFor("name to type, rows 1 to 2")
	s.keys("<esc>", "<right>", "<right>", "<ctrl+k>", "Sort sheet Z to A", "<enter>")
	s.eventually("sorted by size", func() bool {
		_, first := s.rowOf("README.md")
		_, last := s.rowOf("go.mod")
		return first < last && strings.Contains(s.line(first), "  1  README.md")
	})
	col, row := s.rowOf("demos")
	s.leftClick(col+1, row, 0)
	s.keys("<ctrl+c>")
	s.eventually("the cell clicked copied", func() bool { return clip.get() == "demos" })
	s.keys("<enter>")
	s.waitFor("files [1], full-screen")
	s.keys("<esc>")
	s.eventually("back in the window", func() bool {
		return !strings.Contains(s.screen(), "full-screen") || strings.Contains(s.screen(), "[1]:")
	})
	s.keys("<esc>")
	s.waitFor("NOTEBOOK")
}

func TestNotebookMouseAndKeys(t *testing.T) {
	s := startWith(t, options{cols: 100, rows: 40, startsOn: "EDIT", env: fakeNu}, "nu")
	s.keys("files = ls", "<esc>")
	s.waitFor("NOTEBOOK")
	_, row := s.rowOf("files = ls")
	s.leftClick(99, row, 0) // ▶
	s.waitFor("Out[1]:")

	col, _ := s.rowOf("+ Add")
	s.leftClick(col, 1, 0) // the toolbar
	s.waitFor("[ ]:")
	s.keys("<enter>", "$files | get name | str join \", \"", "<esc>")
	s.waitFor("NOTEBOOK")
	_, row = s.rowOf("$files | get")
	s.leftClick(30, row, 0) // into the box: edit, the caret there
	s.waitFor("EDIT")
	s.keys("<esc>")
	s.waitFor("NOTEBOOK")

	_, row = s.rowOf("files = ls")
	s.leftClick(5, row, ghostty.ModShift) // both cells
	s.waitFor("2 cells selected")
	s.keys("<alt+down>") // can't: the last cell is under them
	s.keys("<up>")
	s.keys("<alt+down>")
	s.eventually("the first cell moved down", func() bool {
		c := s.codes()
		return len(c) == 2 && strings.HasPrefix(c[0], "$files") && c[1] == "files = ls"
	})
	s.keys("<alt+up>", "<down>", "o")
	s.waitFor("output hidden")
	s.keys("o")
	s.eventually("the output shown again", func() bool { return !strings.Contains(s.screen(), "output hidden") })
}
