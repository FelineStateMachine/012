package ui

import "testing"

// ! opens the shell prompt on a notebook sheet and starts an entry on
// any other sheet, so a cell can begin with "!" as in Sheets.
func TestBangOpensPromptOnlyOnNotebookSheets(t *testing.T) {
	m := newModel()
	press(t, m, "!")
	if m.mode != modeEnter || m.line.Text() != "!" {
		t.Fatalf("on a plain sheet, ! should start an entry; mode %v, entry %q", m.mode, m.line.Text())
	}
	press(t, m, "<esc>")
	m.sheet.MakeNotebook()
	press(t, m, "!")
	if m.mode == modeEnter {
		t.Fatalf("on a notebook sheet, ! should open the shell prompt, not an entry")
	}
}

// shell opens the nushell prompt as Data > Shell does, then presses keys.
func shell(t *testing.T, m *Model, keys ...string) {
	t.Helper()
	run(m, m.runCommand("nu.prompt"))
	press(t, m, keys...)
}
