package e2e

import ghostty "go.mitchellh.com/libghostty"

// Golden screens for notes and protected ranges: a note on the active
// cell and in the box shown on hover, the note being typed, the warning
// before an edit to a protected range, and the list of protections.

// notedBudget is the budget with a two-line note on Rent's amount.
func notedBudget(s *session) {
	budget(s)
	s.keys("<up>", "<up>", "<up>", "<up>", "<shift+f2>")
	s.waitFor("Note on B3:")
	s.keys("Due on the 1st", "<alt+enter>", "Paid by autopay from checking", "<enter>")
	s.waitFor("Note  Due on the 1st")
}

// protectedBudget is the budget with its amounts protected.
func protectedBudget(s *session) {
	budget(s)
	s.keys("<up>", "<up>", "<up>", "<up>", "<shift+down>", "<shift+down>", "<ctrl+k>", "protect range", "<enter>")
	s.waitFor("Describe B3:B5 (optional):")
	s.keys("Monthly amounts", "<enter>")
	s.waitFor("B3:B5 is protected")
}

var noteScreens = []screen{
	{name: "note", setup: notedBudget},
	{name: "note-typing", setup: func(s *session) {
		notedBudget(s)
		s.keys("<shift+f2>")
		s.waitFor("Note on B3:")
	}},
	{name: "note-hover", setup: func(s *session) {
		notedBudget(s)
		s.keys("<down>", "<down>")
		s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonUnknown, 6+10+4, gridRow1+2, 0)
		s.waitFor("┌─ Note ─")
	}},
	{name: "note-hover-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		notedBudget(s)
		s.keys("<left>")
		s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonUnknown, 6+10+4, gridRow1+2, 0)
		s.waitFor("┌─ Note ─")
	}},
	{name: "note-hover-bottom", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		// A note on the last row shown: the box moves up, clear of the
		// status line.
		s.keys("<f5>", "B11", "<enter>", "<shift+f2>")
		s.waitFor("Note on B11:")
		s.keys("Checked against", "<alt+enter>", "the bank statement", "<alt+enter>", "on the 3rd", "<enter>")
		s.waitFor("Note  Checked against")
		s.keys("<ctrl+home>")
		s.mouse(ghostty.MouseActionMotion, ghostty.MouseButtonUnknown, 6+10+4, gridRow1+10, 0)
		s.waitFor("┌─ Note ─")
	}},
	{name: "protect-warning", setup: func(s *session) {
		protectedBudget(s)
		s.keys("<esc>", "<down>", "100")
		s.waitFor("Edit anyway")
	}},
	{name: "protect-warning-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		protectedBudget(s)
		s.keys("<esc>", "<down>", "100")
		s.waitFor("Edit anyway")
	}},
	{name: "protect-picker", setup: func(s *session) {
		protectedBudget(s)
		s.keys("<esc>", "<ctrl+k>", "protect sheets", "<enter>")
		s.waitFor("+ Protect the sheet")
	}},
}

func init() {
	light := map[string]bool{"note": true, "note-hover": true, "note-hover-bottom": true, "protect-warning": true}
	for _, sc := range noteScreens {
		screens = append(screens, sc)
		if light[sc.name] {
			sc.name += "-light"
			sc.opts.light = true
			screens = append(screens, sc)
		}
	}
}
