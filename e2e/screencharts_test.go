package e2e

// Golden screens for the chart types and options past column, bar, line
// and pie: area, scatter, stacking and the axis bar.

var chartScreens = []screen{
	{name: "chart-area", setup: func(s *session) { chartOfType(s, 4, "Area chart") }},
	{name: "chart-scatter", setup: func(s *session) { chartWith(s, "Scatter chart", "6", "e") }},
	{name: "chart-stacked", setup: func(s *session) { chartWith(s, "Column chart", "k") }},
	{name: "chart-bar-percent", setup: func(s *session) { chartWith(s, "Bar chart", "2", "k", "k") }},
	{name: "chart-axes", setup: func(s *session) {
		spending(s)
		insertChart(s)
		s.keys("a", "p")
		s.waitFor("Legend right")
	}},
	{name: "chart-axes-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		spending(s)
		insertChart(s)
		s.keys("a")
		s.waitFor("Legend bottom")
	}},
}

func init() {
	light := map[string]bool{"chart-area": true, "chart-scatter": true, "chart-stacked": true, "chart-axes": true}
	for _, sc := range chartScreens {
		screens = append(screens, sc)
		if light[sc.name] {
			sc.name += "-light"
			sc.opts.light = true
			screens = append(screens, sc)
		}
	}
}
