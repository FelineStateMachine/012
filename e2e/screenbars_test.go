package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// scoresBook is a class's scores with the rules that draw their own:
// data bars on the points, arrows on the change, circles on the grade,
// the top two in green, duplicate names marked, a dropdown shown as
// chips and a checkbox of its own values.
const scoresBook = `{
  "version": 2,
  "widths": {"A": 10, "B": 9, "C": 9, "D": 8, "E": 10, "F": 8},
  "cells": {
    "A1": {"input":"Name","bold":true},
    "B1": {"input":"Points","bold":true,"align":"right"},
    "C1": {"input":"Change","bold":true,"align":"right"},
    "D1": {"input":"Grade","bold":true,"align":"right"},
    "E1": {"input":"Track","bold":true},
    "F1": {"input":"Passed","bold":true},
    "A2": "Ada", "B2": "92", "C2": "12", "D2": "4", "E2": "Math", "F2": "Yes",
    "A3": "Bo", "B3": "48", "C3": "-8", "D3": "2", "E3": "Art", "F3": "No",
    "A4": "Cy", "B4": "75", "C4": "3", "D4": "3", "E4": "Math", "F4": "Yes",
    "A5": "Bo", "B5": "30", "C5": "-2", "D5": "1", "F5": "No",
    "A6": "Dee", "B6": "100", "C6": "15", "D6": "5", "E6": "Music", "F6": "Yes"
  },
  "conditionalFormats": [
    {"ranges":"B2:B6","dataBar":{"color":"blue","min":{"type":"min"},"max":{"type":"max"}}},
    {"ranges":"C2:C6","iconSet":{"icons":"arrows","points":[{"type":"percent","value":"33"},{"type":"percent","value":"67"}]}},
    {"ranges":"D2:D6","iconSet":{"icons":"circles","points":[{"type":"percent","value":"20"},{"type":"percent","value":"40"},{"type":"percent","value":"60"},{"type":"percent","value":"80"}],"iconOnly":true}},
    {"ranges":"A2:A6","condition":"duplicate","text":"magenta","italic":true}
  ],
  "validations": [
    {"ranges":"E2:E6","criteria":"list","items":["Math","Art","Music"],"display":"chip"},
    {"ranges":"F2:F6","criteria":"checkbox","items":["Yes","No"]}
  ]
}
`

func scoresFile(t *testing.T, dir string) {
	if err := os.WriteFile(filepath.Join(dir, "scores.012"), []byte(scoresBook), 0o644); err != nil {
		t.Fatal(err)
	}
}

// openScores opens the scores.
func openScores(s *session) {
	s.keys("<ctrl+o>")
	s.waitFor("Open file:")
	s.keys("scores", "<enter>")
	s.waitFor("Dee")
}

var barScreens = []screen{
	{name: "rules-bars", files: scoresFile, setup: func(s *session) {
		openScores(s)
		s.keys("<down>", "<down>", "<right>", "<right>", "<right>", "<right>")
		s.waitForBar("E3", "Art")
	}},
	{name: "rules-bars-high-contrast", opts: options{config: highContrastConfig}, files: scoresFile, setup: func(s *session) {
		openScores(s)
		s.keys("<down>", "<down>", "<right>", "<right>", "<right>", "<right>")
		s.waitForBar("E3", "Art")
	}},
	{name: "rules-editor-bar", files: scoresFile, setup: func(s *session) {
		openScores(s)
		s.keys("<ctrl+k>", "conditional formatting", "<enter>")
		s.waitFor("Conditional format rules")
		s.keys("<down>", "<enter>")
		s.waitFor("Show bar only")
	}},
	{name: "rules-editor-icons", files: scoresFile, setup: func(s *session) {
		openScores(s)
		s.keys("<ctrl+k>", "conditional formatting", "<enter>")
		s.waitFor("Conditional format rules")
		s.keys("<down>", "<down>", "<enter>")
		s.waitFor("Reverse icons")
	}},
}

func init() {
	for _, sc := range barScreens {
		screens = append(screens, sc)
		if sc.name == "rules-bars" || sc.name == "rules-editor-icons" {
			sc.name += "-light"
			sc.opts.light = true
			screens = append(screens, sc)
		}
	}
}
