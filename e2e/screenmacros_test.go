package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// Golden screens for macros: the Data > Macros menu, recording, saving,
// the manager, an error from a script and the question before running
// macros from another computer.

// recordOnBudget records, with relative references, a total row below
// the budget: a label, a formula and bold, saved as name with shortcut
// key (empty for none).
func recordOnBudget(s *session, name, key string) {
	s.keys("<ctrl+k>", "relative references", "<enter>")
	s.waitFor(" REC ")
	s.keys("<down>", "<left>", "Yearly", "<tab>", "=B7*12", "<enter>")
	s.keys("<up>", "<shift+right>", "<ctrl+b>")
	s.waitFor("Bold on")
	s.keys("<ctrl+k>", "stop and save", "<enter>")
	s.waitFor("Save macro as:")
	s.keys(name, "<enter>")
	s.waitFor("Shortcut Ctrl+Alt+Shift+")
	s.keys("<backspace>", key, "<enter>")
	s.waitFor("Saved macro " + name)
}

// foreignBook is a saved budget with a macro made on another computer.
const foreignBook = `{
  "version": 2,
  "macroOrigin": "0123456789abcdef0123456789abcdef",
  "macros": [
    {"name": "Yearly", "key": "1", "api": 1, "source": "enter(\"Yearly\")\nmove(1, 0)\nenter(\"=B7*12\", origin=\"B8\")\n"}
  ],
  "cells": {
    "A1": "Household budget 2026",
    "A3": "Rent",
    "B3": "1450",
    "A7": "Total",
    "B7": "=SUM(B3:B5)"
  }
}
`

var macroScreens = []screen{
	{name: "macro-menu", setup: func(s *session) {
		budget(s)
		s.keys("<alt+d>", "m")
		s.waitFor("Record macro with relative references")
	}},
	{name: "macro-recording", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+k>", "relative references", "<enter>")
		s.waitFor(" REC ")
		s.keys("<down>", "<left>", "Yearly", "<tab>")
		s.waitForBar("B8", "")
	}},
	{name: "macro-shortcut", setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+k>", "relative references", "<enter>")
		s.waitFor(" REC ")
		s.keys("<down>", "<left>", "Yearly", "<enter>")
		s.keys("<ctrl+k>", "stop and save", "<enter>")
		s.waitFor("Save macro as:")
		s.keys("Yearly total", "<enter>")
		s.waitFor("Shortcut Ctrl+Alt+Shift+")
	}},
	{name: "macro-manager", setup: func(s *session) {
		budget(s)
		recordOnBudget(s, "Yearly total", "1")
		s.keys("<ctrl+home>")
		recordOnBudget(s, "Header row", "")
		s.keys("<ctrl+k>", "manage macros", "<enter>")
		s.waitFor("Write a macro")
		s.keys("<down>")
	}},
	{name: "macro-error", setup: func(s *session) {
		budget(s)
		recordOnBudget(s, "Yearly", "1")
		s.keys("<ctrl+end>", "<down>", "<down>", "<down>", "<home>", "<ctrl+alt+shift+1>")
		s.waitFor("goes off the sheet")
	}},
	{name: "macro-trust", files: func(t *testing.T, dir string) {
		if err := os.WriteFile(filepath.Join(dir, "budget.012"), []byte(foreignBook), 0o644); err != nil {
			t.Fatal(err)
		}
	}, setup: func(s *session) {
		s.keys("<ctrl+o>")
		s.waitFor("Open file:")
		s.keys("budget", "<enter>")
		s.waitFor("Household budget 2026")
		s.keys("<f5>", "A8", "<enter>", "<ctrl+alt+shift+1>")
		s.waitFor("Trust this file's macros?")
	}},
	{name: "macro-recording-narrow", opts: options{cols: 60, rows: 16}, setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+k>", "relative references", "<enter>")
		s.waitFor(" REC ")
	}},
}

func init() {
	light := map[string]bool{"macro-menu": true, "macro-recording": true, "macro-manager": true, "macro-error": true, "macro-trust": true}
	for _, sc := range macroScreens {
		screens = append(screens, sc)
		if light[sc.name] {
			sc.name += "-light"
			sc.opts.light = true
			screens = append(screens, sc)
		}
	}
}
