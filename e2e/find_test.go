package e2e

import (
	"strings"
	"testing"
)

func TestFindAndReplace(t *testing.T) {
	t.Parallel()
	s := start(t, "")
	s.keys("Rent", "<tab>", "1450", "<enter>", "rental car", "<enter>", "Total rent", "<enter>", "<ctrl+home>")
	s.keys("<ctrl+f>", "rent")
	s.eventually("find bar", func() bool {
		l := s.line(2)
		return strings.HasPrefix(l, "Find › rent") && strings.HasSuffix(l, "1 of 3")
	})
	s.waitFor("FIND")
	s.keys("<enter>", "<enter>")
	s.waitForName("A3")

	s.keys("<ctrl+h>", "lease", "<ctrl+enter>")
	s.waitFor("No matches")
	s.keys("<esc>")
	s.waitForBar("A3", "Total lease")
	s.keys("<ctrl+z>")
	s.waitForLine(gridRow1+2, "    3  Total rent")
}
