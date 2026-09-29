package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// A panic stops 012 with the terminal restored (the primary screen, the
// cursor shown), the unsaved work kept, and a report written, their
// paths printed. Opening the file again offers the work back. The
// binary is built with -tags crashtest, where F12 panics where
// O12_CRASH_TEST says (internal/ui/crashtest.go).
func TestPanicKeepsWork(t *testing.T) {
	for _, where := range []string{"update", "command", "view"} {
		t.Run(where, func(t *testing.T) {
			dir := t.TempDir()
			o := options{dir: dir, env: []string{"O12_CRASH_TEST=" + where}}
			s := startWith(t, o, "budget.012")
			s.keys("kept", "<enter>")
			s.waitFor("kept")
			s.keys("<f12>")
			s.waitExit()
			if scr := s.activeScreen(); scr != ghostty.ScreenPrimary {
				t.Errorf("active screen after the crash = %v, want primary", scr)
			}
			s.mu.Lock()
			visible, _ := s.vt.CursorVisible()
			s.mu.Unlock()
			if !visible {
				t.Error("the cursor is hidden after the crash")
			}
			// The lines wrap at the terminal's width, dropping the space a
			// line ends on, so compare without whitespace.
			flat := func(s string) string { return strings.Join(strings.Fields(s), "") }
			text := flat(s.screen())
			recovery, _ := filepath.Glob(filepath.Join(dir, ".config", "012", "recovery", "*budget-*.012"))
			reports, _ := filepath.Glob(filepath.Join(dir, ".config", "012", "crashes", "crash-*.txt"))
			if len(recovery) != 1 || len(reports) != 1 {
				t.Fatalf("recovery files %v, reports %v; screen:\n%s", recovery, reports, s.screen())
			}
			for _, want := range []string{"012: stopped on an internal error (a panic in " + strings.Replace(where, "command", "a command", 1), "unsaved changes kept in " + recovery[0] + "; open budget.012 again to restore them",
				"a report is in " + reports[0]} {
				if !strings.Contains(text, flat(want)) {
					t.Errorf("the screen lacks %q:\n%s", want, s.screen())
				}
			}
			report, _ := os.ReadFile(reports[0])
			if !strings.Contains(string(report), "Panic: crash test in ") {
				t.Errorf("report:\n%s", report)
			}

			r := startWith(t, options{dir: dir, startsOn: "Unsaved changes to budget.012 were kept."}, "budget.012")
			r.keys("<enter>")
			r.waitFor("Restored the kept changes")
			if !strings.Contains(r.line(gridRow1), "kept") {
				t.Errorf("A1 after restoring: %q", r.line(gridRow1))
			}
		})
	}
}
