package e2e

import (
	"os"
	"path/filepath"
)

// Golden screens for notebooks: two regions, the second reading the
// first; the prompt completing a command; a command that failed. Each is
// recorded on a light terminal and in the high-contrast theme too. nu is
// testdata/nu/nu, which answers what the screens type with fixed tables.

// fakeNu puts the stand-in nu first on the PATH.
var fakeNu = func() []string {
	dir, err := filepath.Abs(filepath.Join("testdata", "nu"))
	if err != nil {
		panic(err)
	}
	return []string{"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH")}
}()

// twoRegions runs ls and a region reading it, then goes back to the grid.
func twoRegions(s *session) {
	s.keys("ls", "<enter>")
	s.waitFor("r1: 5 rows")
	s.keys("big = $r1 | where size > 1kb", "<enter>")
	s.waitFor("big: 3 rows")
	s.keys("<esc>")
	s.waitFor("READY")
}

var notebookScreens = []screen{
	{name: "notebook-regions", setup: func(s *session) {
		twoRegions(s)
		s.keys("<down>", "<down>", "<right>", "<right>")
		s.waitFor("Region big")
	}},
	{name: "notebook-completion", setup: func(s *session) {
		s.keys("ls", "<enter>")
		s.waitFor("r1: 5 rows")
		s.keys("$r1 | so")
		s.waitFor("sort-by")
	}},
	{name: "notebook-failed", setup: func(s *session) {
		s.keys("ls", "<enter>")
		s.waitFor("r1: 5 rows")
		s.keys("lss -a", "<enter>")
		s.waitFor("r2 failed: Command `lss` not found")
	}},
}

func init() {
	for _, sc := range notebookScreens {
		sc.args, sc.opts.startsOn, sc.opts.env = []string{"nu"}, "nu❯", fakeNu
		light, hc := sc, sc
		light.name += "-light"
		light.opts.light = true
		hc.name += "-high-contrast"
		hc.opts.config = highContrastConfig
		screens = append(screens, sc, light, hc)
	}
}
