package e2e

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Golden screens for notebooks: cells with a note and table outputs, a
// cell edited with a long pipeline wrapped (at 80 and 60 columns), a
// failure, nu's highlighting and a problem nu found, a cell running and
// one waiting, a cell running as a stream, stale outputs, an output's
// grid entered with a range selected, its filter open, and full-screen,
// and an output sent to a sheet. Each is recorded on a light terminal
// and in the high-contrast theme too. nu is testdata/nu/nu, which
// answers what the screens run, and what the code editor asks, with
// fixed outputs.

// fakeNu puts the stand-in nu first on the PATH.
var fakeNu = func() []string {
	dir, err := filepath.Abs(filepath.Join("testdata", "nu"))
	if err != nil {
		panic(err)
	}
	return []string{"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")}
}()

// nuHighlighted reports whether the line of screen html holding mark
// draws word twice in two styles, as nu's shapes of `where size > 1kb
// | sort-by size` do (a column, then a string) where the tokenizer
// leaves both plain.
func nuHighlighted(html, mark, word string) bool {
	for line := range strings.SplitSeq(html, "\n") {
		if !strings.Contains(line, mark) {
			continue
		}
		var styles []string
		for _, m := range regexp.MustCompile(`<span style="([^"]*)">`+regexp.QuoteMeta(word)+`</span>`).FindAllStringSubmatch(line, -1) {
			styles = append(styles, m[1])
		}
		return len(styles) == 2 && styles[0] != styles[1]
	}
	return false
}

// longPipeline is a pipeline too long for one line at 80 columns.
const longPipeline = "$files | where size > 1kb | where type == file | sort-by size --reverse | select name size | first 10 | rename file bytes"

// filesNotebook turns 012 nu's first cell into a note, then writes and
// runs a cell listing files and one reading it, leaving a new cell
// selected.
func filesNotebook(s *session) {
	s.keys("<esc>", "m", "<enter>", "# Files", "<enter>", "What's big in the repo, from `ls`.", "<esc>")
	s.keys("b", "<enter>", "files = ls", "<shift+enter>")
	s.waitFor("Out[1]:")
	s.keys("<enter>", "$files | where size > 1kb", "<shift+enter>")
	s.waitFor("Out[2]:")
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
		s.waitFor("[3]:")
	}},
	{name: "notebook-error", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<enter>", "lss -a", "<ctrl+enter>")
		s.waitFor("× Command `lss` not found")
	}},
	{name: "notebook-running", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<enter>", "sleep 10min", "<esc>", "b", "<enter>", "{name: 1}", "<esc>", "<f9>")
		// F9 queues every cell; the first two finish, then sleep runs.
		s.waitFor("busy, 1 waiting")
	}},
	{name: "notebook-stale", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<up>", "<up>", "<up>", "<up>", "<enter>", " | first 4", "<esc>")
		s.waitFor("stale")
	}},
	{name: "notebook-output", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<up>", "<up>", "<up>", "<enter>", "<enter>")
		s.waitFor("◀ Back")
	}},
	{name: "notebook-grid", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<up>", "<up>", "<up>", "<enter>", "<shift+down>", "<shift+right>", "<shift+right>")
		s.waitFor("Count 6")
	}},
	{name: "notebook-grid-filter", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<up>", "<up>", "<up>", "<enter>", "<ctrl+k>", "Create a filter", "<enter>")
		s.waitFor("▾")
		s.keys("<right>", "<alt+down>")
		s.waitFor("Select all")
	}},
	{name: "notebook-nu-highlight", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<enter>", "big = $files | where size > 1kb | sort-by size --reverse # largest")
		s.eventually("nu's highlighting", func() bool { return nuHighlighted(s.html(), "largest", "size") })
	}},
	{name: "notebook-nu-error", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<enter>", "$files | sort-by size --revrse")
		s.waitFor("doesn't have flag `revrse`")
	}},
	{name: "notebook-select", opts: options{cols: 120, rows: 30}, setup: func(s *session) {
		filesNotebook(s)
		s.keys("<up>", "<up>", "<shift+up>")
		s.waitFor("2 cells selected")
	}},
	{name: "notebook-scroll", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<enter>", "1..30 | each {|i| {n: $i, square: ($i * $i)}}", "<ctrl+enter>")
		s.waitFor("Out[3]:")
		s.keys("<esc>", "<down>", "<down>", "<down>", "<down>")
		s.waitFor("rows 4 to 13 of 30")
	}},
	{name: "notebook-stream", setup: func(s *session) {
		filesNotebook(s)
		s.keys("<enter>", "log = tail -f app.log | lines | parse '{time} {level} {msg}'", "<esc>", "f")
		// The head counts the rows the output has, so the grid is fitted
		// to all three: only the first shows.
		s.waitFor("● live, 3 rows")
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
