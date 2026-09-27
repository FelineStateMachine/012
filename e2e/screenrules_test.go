package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// tasksBook is a task list with rules on every column: owners from a
// dropdown (one of them not on its list), hours on a color scale, done
// tasks struck through in green, and checkboxes.
const tasksBook = `{
  "version": 2,
  "widths": {"A": 14, "B": 10, "C": 8, "D": 7},
  "cells": {
    "A1": {"input":"Task","bold":true},
    "B1": {"input":"Owner","bold":true},
    "C1": {"input":"Hours","bold":true,"align":"right"},
    "D1": {"input":"Done","bold":true,"align":"center"},
    "A2": "Write spec", "B2": "Ann", "C2": "12", "D2": "TRUE",
    "A3": "Build UI", "B3": "Bo", "C3": "40", "D3": "FALSE",
    "A4": "Tests", "B4": "Cy", "C4": "26", "D4": "TRUE",
    "A5": "Docs", "B5": "Dee", "C5": "6",
    "A6": "Release", "B6": "Ann", "C6": "3", "D6": "FALSE",
    "A7": "Total", "C7": "=SUM(C2:C6)"
  },
  "conditionalFormats": [
    {"ranges":"A2:A6","condition":"formula","values":["=$D2"],"text":"green","strikethrough":true},
    {"ranges":"C2:C6","scale":[{"type":"min","color":"green"},{"type":"percentile","value":"50","color":"yellow"},{"type":"max","color":"red"}]},
    {"ranges":"C7","condition":"gt","values":["80"],"fill":"magenta","bold":true}
  ],
  "validations": [
    {"ranges":"B2:B6","criteria":"list","items":["Ann","Bo","Cy"],"help":"Pick who does it"},
    {"ranges":"D2:D6","criteria":"checkbox"},
    {"ranges":"C2:C6","criteria":"number","condition":"between","values":["0","80"],"reject":true}
  ]
}
`

func tasksFile(t *testing.T, dir string) {
	if err := os.WriteFile(filepath.Join(dir, "tasks.012"), []byte(tasksBook), 0o644); err != nil {
		t.Fatal(err)
	}
}

// openTasks opens the task list.
func openTasks(s *session) {
	s.keys("<ctrl+o>")
	s.waitFor("Open file:")
	s.keys("tasks", "<enter>")
	s.waitFor("Write spec")
}

var ruleScreens = []screen{
	{name: "rules-sheet", files: tasksFile, setup: func(s *session) {
		openTasks(s)
		s.keys("<down>", "<down>", "<down>", "<down>", "<right>")
		s.waitFor("Invalid: Pick who does it")
	}},
	{name: "rules-sheet-dracula", opts: options{config: "theme = Dracula\n"}, files: tasksFile, setup: func(s *session) {
		openTasks(s)
		s.keys("<down>", "<down>", "<right>", "<right>", "<right>")
		s.waitFor("check or uncheck")
	}},
	{name: "rules-sheet-latte", opts: options{light: true, config: "theme = Catppuccin Latte\n"}, files: tasksFile, setup: func(s *session) {
		openTasks(s)
		s.keys("<down>", "<down>", "<right>", "<right>", "<right>")
		s.waitFor("check or uncheck")
	}},
	{name: "rules-panel", files: tasksFile, setup: func(s *session) {
		openTasks(s)
		s.keys("<ctrl+k>", "conditional formatting", "<enter>")
		s.waitFor("Conditional format rules")
		s.keys("<down>", "<down>")
		s.waitFor("first rule that matches")
	}},
	{name: "rules-editor", files: tasksFile, setup: func(s *session) {
		openTasks(s)
		s.keys("<ctrl+k>", "conditional formatting", "<enter>")
		s.waitFor("Conditional format rules")
		s.keys("<down>", "<enter>")
		s.waitFor("Edit conditional format")
	}},
	{name: "rules-editor-narrow", opts: options{cols: 60, rows: 16}, files: tasksFile, setup: func(s *session) {
		openTasks(s)
		s.keys("<ctrl+k>", "conditional formatting", "<enter>")
		s.waitFor("Conditional format rules")
		s.keys("<down>", "<down>", "<enter>")
		s.waitFor("Minpoint")
	}},
	{name: "validation-editor", files: tasksFile, setup: func(s *session) {
		openTasks(s)
		s.keys("<ctrl+k>", "data validation", "<enter>")
		s.waitFor("Data validation rules")
		s.keys("<down>", "<enter>")
		s.waitFor("Edit data validation")
	}},
	{name: "dropdown-open", files: tasksFile, setup: func(s *session) {
		openTasks(s)
		s.keys("<down>", "<down>", "<right>", "<alt+down>")
		s.waitFor("Search the items")
	}},
	{name: "validation-reject", files: tasksFile, setup: func(s *session) {
		openTasks(s)
		s.keys("<down>", "<right>", "<right>", "120", "<enter>")
		s.waitFor("Invalid entry in C2")
	}},
}

func init() {
	light := map[string]bool{"rules-sheet": true, "rules-panel": true, "rules-editor": true, "validation-editor": true, "dropdown-open": true}
	for _, sc := range ruleScreens {
		screens = append(screens, sc)
		if light[sc.name] {
			sc.name += "-light"
			sc.opts.light = true
			screens = append(screens, sc)
		}
	}
}
