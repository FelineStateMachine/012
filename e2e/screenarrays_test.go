package e2e

// Golden screens for arrays: a formula's array spilled beside the budget,
// with the active cell on a spilled value, and an array blocked by a cell
// in its way, its #REF! explained.

// sortedBudget is the budget with its amounts sorted, largest first,
// spilling from D2, and the active cell on a spilled amount.
func sortedBudget(s *session) {
	budget(s)
	s.keys("<f5>", "D2", "<enter>", "=SORT(A3:B5, 2, FALSE)", "<enter>", "<right>")
	s.waitFor("Spilled from D2")
}

var arrayScreens = []screen{
	{name: "spill", setup: sortedBudget},
	{name: "spill-blocked", setup: func(s *session) {
		budget(s)
		s.keys("<f5>", "D4", "<enter>", "paid", "<enter>", "<f5>", "D2", "<enter>", "=SORT(A3:B5, 2, FALSE)", "<enter>", "<up>")
		s.waitFor("would overwrite data in D4")
	}},
}

func init() {
	for _, sc := range arrayScreens {
		screens = append(screens, sc)
		sc.name += "-light"
		sc.opts.light = true
		screens = append(screens, sc)
	}
}
