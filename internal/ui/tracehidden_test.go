package ui

import "testing"

// Tracing a formula whose every precedent is on a hidden sheet says so,
// and where to show it; the same for dependents.
func TestTraceIntoHiddenSheet(t *testing.T) {
	m := hiddenBook(t)
	run(m, m.runCommand("sheet.hide"))
	press(t, m, "<ctrl+pgup>")
	m.cur = addr("A1")
	press(t, m, "<alt+,>")
	if got := line(m, contextLine); m.trace != nil || got != "Reads only Data, a hidden sheet; View > Hidden sheets shows it" {
		t.Errorf("trace %v, context %q", m.trace, got)
	}
	m.book().Lookup("Sheet3").Set(addr("B2"), "5")
	m.book().HideSheet(m.book().Lookup("Sheet3"))
	m.sheet.Set(addr("A2"), "=Data!A1+Sheet3!B2")
	m.cur = addr("A2")
	press(t, m, "<alt+,>")
	if got := line(m, contextLine); got != "Reads only Data and Sheet3, hidden sheets; View > Hidden sheets shows them" {
		t.Errorf("context %q", got)
	}
	// Precedents on a sheet shown are traced as ever.
	m.sheet.Set(addr("A3"), "=Data!A1+A2")
	m.cur = addr("A3")
	press(t, m, "<alt+,>")
	if m.trace == nil || m.cur != addr("A2") {
		t.Errorf("trace %v, on %v", m.trace, m.cur)
	}
	if got := noTrace(true, addr("B2"), []string{"Data"}); got != "Only formulas on Data, a hidden sheet, read B2; View > Hidden sheets shows it" {
		t.Errorf("dependents: %q", got)
	}
}
