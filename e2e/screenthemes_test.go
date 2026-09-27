package e2e

// Golden screens for the high-contrast theme (WCAG AAA text), on a dark
// and a light terminal, with a selection and an error, and an open menu.

// highContrast is the budget with an error and a selection.
func highContrast(s *session) {
	budget(s)
	s.keys("<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>")
	s.waitFor("Sum 2158.4")
}

const highContrastConfig = "theme = high-contrast\n"

func init() {
	screens = append(screens,
		screen{name: "theme-high-contrast", opts: options{config: highContrastConfig}, setup: highContrast},
		screen{name: "theme-high-contrast-light", opts: options{light: true, config: highContrastConfig}, setup: highContrast},
		screen{name: "theme-high-contrast-menu", opts: options{config: highContrastConfig}, setup: func(s *session) {
			budget(s)
			s.keys("<alt+f>", "<down>", "<down>", "<down>")
			s.waitFor("Save the sheet")
		}},
	)
}
