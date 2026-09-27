package e2e

import (
	"os"
	"path/filepath"
	"testing"
)

// Golden screens for files and settings: questions about replacing a
// file, restoring kept changes over ssh, and checking an API key.
var fileScreens = []screen{
	{name: "save-as-replace", files: func(t *testing.T, dir string) {
		if err := os.WriteFile(filepath.Join(dir, "budget.012"), []byte("A1 old\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}, setup: func(s *session) {
		budget(s)
		s.keys("<ctrl+s>", "budget", "<enter>")
		s.waitFor("budget.012 exists.")
	}},
	// Over ssh, a file whose changes a session kept when the server
	// stopped.
	{name: "recovery-offer", opts: options{startsOn: "were kept."}, ssh: []string{"budget.012"}, files: func(t *testing.T, dir string) {
		os.Mkdir(filepath.Join(dir, ".012-recovery"), 0o700)
		for path, a1 := range map[string]string{"budget.012": "Household budget 2025", ".012-recovery/budget-20260927-140203.012": "Household budget 2026"} {
			book := `{"version": 2, "cells": {"A1": "` + a1 + `", "A3": "Rent", "B3": "1200"}}`
			if err := os.WriteFile(filepath.Join(dir, path), []byte(book), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}, setup: func(s *session) {
		s.waitFor("Unsaved changes to budget.012 were kept.")
	}},
	// Nothing listens on port 1: the key is stored, and its check fails.
	{name: "api-key-check-failed", opts: options{env: []string{"TYPESAFE_BASE_URL=http://127.0.0.1:1"}}, setup: func(s *session) {
		s.keys("<alt+f>", "<up>", "<up>", "<right>", "<down>", "<down>", "<down>", "<down>")
		s.waitFor("Store the TypeSafe API key")
		s.keys("<enter>", "test-key", "<enter>")
		s.waitFor("Key saved, but the check failed")
	}},
}

func init() { screens = append(screens, fileScreens...) }
