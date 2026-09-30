package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// Golden screens for a workbook shared over 012 serve: two others in it
// (their pointers in their colors, their initials on the row headers,
// the cells they just changed marked, their names on the status line),
// the list of who's here, an undo refused because another changed the
// cell since, and a follower in a one-writer room. The first is also
// recorded on a light terminal and in the high-contrast theme.

// householdBudget is a small budget in budget.012.
func householdBudget(t *testing.T, dir string) {
	book := `{"version": 2, "cells": {"A1": "Household budget", "A3": "Rent", "B3": "1200", "A4": "Food", "B4": "450",
		"A5": "Power", "B5": "90", "A6": "Total", "B6": "=SUM(B3:B5)"}}`
	if err := os.WriteFile(filepath.Join(dir, "budget.012"), []byte(book), 0o644); err != nil {
		t.Fatal(err)
	}
}

// bobAndCy are two others in the budget: bob changes the food line and
// moves to D4, cy adds a line and stays under it.
var bobAndCy = []peer{
	{user: "bob", keys: []string{"<down>", "<down>", "<down>", "<right>", "480", "<enter>", "<up>", "<right>", "<right>"}, sees: "1770"},
	{user: "cy", keys: []string{"<down>", "<down>", "<down>", "<down>", "<down>", "<down>", "Gym", "<enter>"}, sees: "Gym"},
}

var shareScreens = []screen{
	{name: "share-who", user: "ann", ssh: []string{"budget.012"}, files: householdBudget, peers: bobAndCy, setup: func(s *session) {
		s.waitFor("Sharing budget.012 with bob and cy")
		s.keys("<ctrl+k>", "who's here", "<enter>")
		s.waitFor("Sheet1!D4")
	}},
	{name: "share-undo-refused", user: "ann", ssh: []string{"budget.012"}, files: householdBudget, peers: bobAndCy[:1], setup: func(s *session) {
		s.waitFor("Sharing budget.012 with bob")
		s.keys("<down>", "<down>", "<right>", "1300", "<enter>")
		s.waitFor("1870")
		bob := s.peers[0]
		bob.waitFor("1870")
		bob.keys("<left>", "<left>", "<up>", "1250", "<enter>")
		s.waitFor("1820")
		s.keys("<ctrl+z>")
		s.waitFor("Can't undo")
	}},
	{name: "share-follow", serve: []string{"--share", "view"}, user: "ann", ssh: []string{"budget.012"}, files: householdBudget, peers: bobAndCy[:1],
		setup: func(s *session) {
			s.waitFor("bob writes")
			s.keys("x")
			s.waitFor("bob writes here and you follow")
		}},
}

func init() {
	presence := screen{name: "share-presence", user: "ann", ssh: []string{"budget.012"}, files: householdBudget, peers: bobAndCy, setup: func(s *session) {
		s.waitFor("Sharing budget.012 with bob and cy")
	}}
	light, hc := presence, presence
	light.name += "-light"
	light.opts.light = true
	hc.name += "-high-contrast"
	hc.opts.config = highContrastConfig
	screens = append(screens, presence, light, hc)
	screens = append(screens, shareScreens...)
}
