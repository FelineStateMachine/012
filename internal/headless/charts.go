package headless

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// FindChart finds a chart of s by its number, 1 for the first in the
// order the sheet keeps them, or by its title, ignoring case.
func FindChart(s *sheet.Sheet, which string) (int, error) {
	charts := s.Charts()
	if n, err := strconv.Atoi(strings.TrimSpace(which)); err == nil {
		if n < 1 || n > len(charts) {
			return 0, fmt.Errorf("%s has %d %s: chart %d isn't one", s.Name(), len(charts), plural(len(charts), "chart", "charts"), n)
		}
		return n - 1, nil
	}
	var titles []string
	for i, c := range charts {
		if strings.EqualFold(ChartTitle(c), which) {
			return i, nil
		}
		titles = append(titles, ChartTitle(c))
	}
	if len(titles) == 0 {
		return 0, fmt.Errorf("%s has no charts", s.Name())
	}
	return 0, fmt.Errorf("%s has no chart titled %q (charts: %s)", s.Name(), which, strings.Join(titles, ", "))
}

// ChartTitle is a chart's title, or its type's name when it has none.
func ChartTitle(c sheet.Chart) string {
	if c.Title != "" {
		return c.Title
	}
	return c.Type.Title() + " chart"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
