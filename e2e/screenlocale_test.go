package e2e

// Golden screens for locales: File > Settings > Locale's picker narrowed
// to a search, and a budget shown in German (Germany), with a formula in
// its syntax in the formula bar and a function's hint while typing one.

var localeScreens = []screen{
	{name: "locale-picker", setup: func(s *session) {
		s.keys("<ctrl+k>", "locale", "<enter>")
		s.waitFor("Type a language, country or tag")
		s.keys("de")
		s.waitFor("German (Switzerland)")
	}},
	{name: "locale-de", setup: func(s *session) {
		germanBudget(s)
		s.keys("<up>", "<right>")
		s.waitForBar("B5", "=ROUND(B4*0,19; 2)")
	}},
	{name: "locale-de-signature", setup: func(s *session) {
		germanBudget(s)
		s.keys("Rate", "<tab>", "=ROUND(B5/B4; ")
		s.waitFor("ROUND(value; [places])")
	}},
}

func init() {
	for _, sc := range localeScreens {
		screens = append(screens, sc)
		sc.name += "-light"
		sc.opts.light = true
		screens = append(screens, sc)
	}
}
