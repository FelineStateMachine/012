package ui

import (
	"strings"
	"testing"
)

// sheetsModel has Sheet1 (shown), Summary with 7 in B3, and Q3 plan with
// 5 in A2.
func sheetsModel(t *testing.T) *Model {
	t.Helper()
	m := newModel()
	book := m.book()
	sum, _ := book.AddSheet("Summary", 1)
	sum.Set(addr("B3"), "7")
	plan, _ := book.AddSheet("Q3 plan", 2)
	plan.Set(addr("A2"), "5")
	book.ClearHistory()
	return m
}

func names(list []suggestion) []string {
	var out []string
	for _, s := range list {
		out = append(out, s.name)
	}
	return out
}

func TestAutocompleteSheets(t *testing.T) {
	m := sheetsModel(t)
	press(t, m, "=su")
	list, _ := m.entry.assist.shown(m)
	got := names(list)
	if len(got) < 2 || got[0] != "Summary!" || !strings.HasPrefix(got[1], "SU") {
		t.Fatalf("suggestions %v", got)
	}
	if scr := screen(m); !strings.Contains(scr, "│ Summary!     sheet, A1:B3") {
		t.Errorf("list:\n%s", scr)
	}
	if st := status(m); !strings.Contains(st, "Sheet Summary: A1:B3, 1 cell") {
		t.Errorf("status %q", st)
	}
	press(t, m, "<tab>")
	if m.line.Text() != "=Summary!" || m.mode != modeEnter {
		t.Fatalf("accepted: %q, mode %v", m.line.Text(), m.mode)
	}
	// Typing the address works as ever.
	press(t, m, "B3*2", "<enter>")
	if m.sheet.Value(addr("A1")).Num != 14 {
		t.Errorf("A1 = %v (%q)", m.sheet.Value(addr("A1")), input(m, "A1"))
	}

	// A name that needs quotes is offered quoted, from its start or
	// inside quotes.
	for _, typed := range []string{"=q3", "='q3 p", "='pl"} {
		press(t, m, "<esc>", "<esc>", typed)
		list, start := m.entry.assist.shown(m)
		if len(list) == 0 || list[0].name != "'Q3 plan'!" || start != 1 {
			t.Errorf("%s: %v from %d", typed, names(list), start)
		}
	}
	press(t, m, "<tab>")
	if m.line.Text() != "='Q3 plan'!" {
		t.Fatalf("quoted: %q", m.line.Text())
	}
	// An arrow then points into the sheet, as clicking its tab would.
	press(t, m, "<down>")
	if m.mode != modePoint || m.sheet.Name() != "Q3 plan" || m.pointRef() != "'Q3 plan'!A2" {
		t.Fatalf("pointing: mode %v on %s, %q", m.mode, m.sheet.Name(), m.pointRef())
	}
	press(t, m, "+1", "<enter>")
	if m.sheet.Name() != "Sheet1" || input(m, "A2") != "='Q3 plan'!A2+1" || m.sheet.Value(addr("A2")).Num != 6 {
		t.Errorf("stored on %s: %q", m.sheet.Name(), input(m, "A2"))
	}
}

// The entry's own sheet isn't offered, and pointing after its name typed
// keeps the name; hidden sheets aren't offered, and a quote closing the
// name isn't doubled.
func TestAutocompleteSheetsEdges(t *testing.T) {
	m := sheetsModel(t)
	press(t, m, "=she")
	if list, _ := m.entry.assist.shown(m); len(list) > 0 && list[0].name == "Sheet1!" {
		t.Errorf("the formula's own sheet offered: %v", names(list))
	}
	press(t, m, "et1!", "<right>")
	if m.mode != modePoint || m.sheet.Name() != "Sheet1" || m.pointRef() != "B1" {
		t.Fatalf("own sheet pointing: %v %s %q", m.mode, m.sheet.Name(), m.pointRef())
	}
	press(t, m, "<esc>", "<esc>")

	m.book().HideSheet(m.book().Lookup("Summary"))
	press(t, m, "=summ")
	if list, _ := m.entry.assist.shown(m); len(list) > 0 && list[0].name == "Summary!" {
		t.Errorf("hidden sheet offered: %v", names(list))
	}
	press(t, m, "<esc>")

	// Accepting inside an existing reference replaces the name only.
	m.line.Set("='Q3 p'!A1")
	m.line.Pos = len("='Q3 p")
	m.entry.assist.active = true
	m.mode = modeEnter
	press(t, m, "<tab>")
	if m.line.Text() != "='Q3 plan'!A1" || m.line.Pos != len("='Q3 plan'!") {
		t.Errorf("replaced: %q at %d", m.line.Text(), m.line.Pos)
	}
}
