package e2e

import (
	"os"
	"path/filepath"
)

// Golden screens for notebooks: cells with a note and table outputs, a
// cell edited with a long pipeline wrapped (at 80 and 60 columns), a
// failure, a cell running and one waiting, stale outputs, an output
// full-screen, and an output sent to a sheet. Each is recorded on a
// light terminal and in the high-contrast theme too. nu is testdata/nu/nu,
// which answers what the screens run with fixed outputs.

// fakeNu puts the stand-in nu first on the PATH.
var fakeNu = func() []string {
	dir, err := filepath.Abs(filepath.Join("testdata", "nu"))
	if err != nil {
		panic(err)
	}
	return []string{"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")}
}()

// longPipeline is a pipeline too long for one line at 80 columns.
const longPipeline = "$files | where size > 1kb | where type == file | sort-by size --reverse | select name size | first 10 | rename file bytes"

// filesNotebook turns 012 nu's first cell into a note, then writes and
// runs a cell listing files and one reading it, leaving a new cell
// selected.
func filesNotebook(s *session) {
	s.keys("<esc>", "m", "<enter>", "# Files", "<enter>", "What's big in the repo, from `ls`.", "<esc>")
	s.keys("b", "<enter>", "files = ls", "<shift+enter>")
	s.waitFor("[1] files")
	s.keys("<enter>", "$files | where size > 1kb", "<shift+enter>")
	s.waitFor("[2]")
	s.waitFor("NOTEBOOK")
}

var notebookScreens = []screen{
	{name: "notebook-cells", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<up>", "<up>")
		s.waitFor("reads $files")
	}},
	{name: "notebook-wrap", opts: options{cols: 80, rows: 24}, setup: func(s *session) {
		filesNotebook(s)
		s.keys("<enter>", longPipeline)
		s.waitFor("rename file bytes")
	}},
	{name: "notebook-wrap-narrow", opts: options{cols: 60, rows: 24}, setup: func(s *session) {
		filesNotebook(s)
		s.keys("<enter>", longPipeline, "<ctrl+enter>")
		s.waitFor("[3]")
	}},
	{name: "notebook-error", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<enter>", "lss -a", "<ctrl+enter>")
		s.waitFor("× Command `lss` not found")
	}},
	{name: "notebook-running", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<enter>", "sleep 10min", "<esc>", "b", "<enter>", "{name: 1}", "<esc>", "<f9>")
		s.waitFor("waiting")
	}},
	{name: "notebook-stale", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<up>", "<up>", "<up>", "<up>", "<enter>", " | first 4", "<esc>")
		s.waitFor("stale")
	}},
	{name: "notebook-output", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<up>", "<up>", "<up>", "<enter>")
		s.waitFor("full-screen")
	}},
	{name: "notebook-sent", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<up>", "<up>", "<up>", "<up>", "G", "<enter>")
		s.waitFor("Sent files to")
		s.keys("<ctrl+pgdown>", "<down>")
		s.waitFor("Output of files")
	}},
}

func init() {
	for _, sc := range notebookScreens {
		sc.args, sc.opts.startsOn, sc.opts.env = []string{"nu"}, "EDIT", fakeNu
		light, hc := sc, sc
		light.name += "-light"
		light.opts.light = true
		hc.name += "-high-contrast"
		hc.opts.config = highContrastConfig
		screens = append(screens, sc, light, hc)
	}
}
