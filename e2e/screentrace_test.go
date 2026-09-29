package e2e

// Golden screens for formula tracing: precedents and dependents shown as
// the pointer moves, the list to go to one, and Evaluate formula, on dark
// and light terminals and in the high-contrast theme.

// traced is the named-range budget with the total read by a quarter
// beside it and a yearly figure far below, tracing on at the total.
func traced(s *session) {
	named(s)
	s.keys("<ctrl+g>", "B30", "<enter>", "=B7*12", "<enter>")
	s.keys("<ctrl+g>", "C7", "<enter>", "=B7/4", "<enter>")
	s.keys("<ctrl+home>", "<down>", "<down>", "<down>", "<down>", "<down>", "<down>", "<right>", "<alt+;>")
	s.waitFor("B7 reads Expenses; read by C7, B30↓")
}

// evaluating is Evaluate formula on B8 with two parts computed.
func evaluating(s *session) {
	named(s)
	s.keys("<alt+=>", "<enter>", "<enter>")
	s.waitFor("2 of 3")
}

func init() {
	screens = append(screens,
		screen{name: "trace-view", setup: traced},
		screen{name: "trace-view-light", opts: options{light: true}, setup: traced},
		screen{name: "trace-view-high-contrast", opts: options{config: highContrastConfig}, setup: traced},
		screen{name: "trace-view-high-contrast-light", opts: options{light: true, config: highContrastConfig}, setup: traced},
		screen{name: "trace-list", setup: func(s *session) {
			traced(s)
			s.keys("<alt+'>")
			s.waitFor("Precedents and dependents of B7")
		}},
		screen{name: "evaluate", setup: evaluating},
		screen{name: "evaluate-light", opts: options{light: true}, setup: evaluating},
		screen{name: "evaluate-high-contrast", opts: options{config: highContrastConfig}, setup: evaluating},
		screen{name: "evaluate-into", setup: func(s *session) {
			traced(s)
			s.keys("<right>", "<alt+=>", "<right>")
			s.waitFor("Evaluate C7 › B7")
		}},
		screen{name: "evaluate-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
			named(s)
			s.keys("<alt+=>", "<enter>")
			s.waitFor("1 of 3")
		}},
	)
}
