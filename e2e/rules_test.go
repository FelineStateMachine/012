package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	ghostty "go.mitchellh.com/libghostty"
)

// Rules as a user meets them: a dropdown picked from with Alt+Down and
// the mouse, a checkbox toggled with Space and a click, an entry a rule
// rejects, a conditional format added from the panel, and all of it
// saved and opened again.
func TestRules(t *testing.T) {
	dir := t.TempDir()
	tasksFile(t, dir)
	s := start(t, dir, "tasks.012")
	s.waitFor("Write spec")

	// B3's dropdown, from the keyboard.
	s.keys("<down>", "<down>", "<right>", "<alt+down>")
	s.waitFor("Search the items")
	s.keys("cy", "<enter>")
	s.waitForBar("B3", "Cy")

	// D3's checkbox, with Space, then B5's dropdown and D5's checkbox
	// with the mouse (row 5 is screen line gridRow1+4).
	s.keys("<right>", "<right>", "<space>")
	s.waitForBar("D3", "TRUE")
	s.waitForLine(gridRow1+2, "    3  Build UI      Cy      ▾     40   [✓]")
	s.click(ghostty.MouseButtonLeft, 6+14+10-1, gridRow1+4) // B's last column
	s.waitFor("Search the items")
	s.keys("ann", "<enter>")
	s.waitForBar("B5", "Ann")
	s.click(ghostty.MouseButtonLeft, 6+14+10+8+3, gridRow1+4) // D's glyph
	s.waitForBar("D5", "TRUE")

	// Hours reject anything over 80, with Sheets' words.
	s.keys("<left>", "120", "<enter>")
	s.waitFor("Invalid entry in C5: Input must be a number between 0 and 80")
	s.keys("<esc>")
	s.waitForBar("C5", "6")

	// Format > Conditional formatting: a new rule over the selection.
	s.keys("<ctrl+k>", "conditional formatting", "<enter>")
	s.waitFor("3 rules")
	s.keys("<enter>")
	s.waitFor("Add conditional format")
	s.keys("<enter>")
	s.waitFor("4 rules")
	s.keys("<esc>", "<ctrl+s>")
	s.eventually("saved", func() bool { return !strings.Contains(s.line(29), "modified") })
	body, err := os.ReadFile(filepath.Join(dir, "tasks.012"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`{"ranges":"C5","condition":"not_empty","fill":"green"}`, `"B3": "Cy"`, `"D5": "TRUE"`, `"validations": [`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("file lacks %s:\n%s", want, body)
		}
	}

	// Opened again, the rules are there.
	s2 := start(t, dir, "tasks.012")
	s2.waitForLine(gridRow1+2, "    3  Build UI      Cy      ▾     40   [✓]")
	s2.keys("<ctrl+k>", "conditional formatting", "<enter>")
	s2.waitFor("4 rules")
}
