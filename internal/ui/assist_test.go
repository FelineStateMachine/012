package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestScanCaret(t *testing.T) {
	for _, tc := range []struct {
		text string // | marks the caret
		word string
		fn   string
		arg  int
	}{
		{"=SU|", "SU", "", 0},
		{"=@su|", "su", "", 0},
		{"=SUM(|", "", "SUM", 0},
		{"=SUM(A1, B|", "B", "SUM", 1},
		{"=IF(A1>2, SUM(B1; B2), |", "", "IF", 2},
		{"=IF(A1, (1+|", "", "IF", 1},
		{`=IF(A1="a,b(", |`, "", "IF", 1},
		{`=CONCAT("SU|`, "", "", 0},
		{"=SUM (1, |", "", "SUM", 1},
		{"=jev.te|", "jev.te", "", 0},
		{"=A1+1|", "", "", 0},
		{"=SU|M(1)", "", "", 0},
		{"=Sales|", "Sales", "", 0},
	} {
		i := strings.Index(tc.text, "|")
		buf := []rune(strings.Replace(tc.text, "|", "", 1))
		c := scanCaret(buf, len([]rune(tc.text[:i])))
		if c.word != tc.word || c.fn != tc.fn || c.arg != tc.arg {
			t.Errorf("%s: word %q fn %q arg %d", tc.text, c.word, c.fn, c.arg)
		}
	}
}

func TestArgPart(t *testing.T) {
	for _, tc := range []struct {
		args     string
		arg      int
		variadic bool
		want     string
	}{
		{"value1, [value2, ...]", 0, true, "value1"},
		{"value1, [value2, ...]", 4, true, "[value2, ...]"},
		{"condition, value_if_true, [value_if_false]", 2, false, "[value_if_false]"},
		{"condition, value_if_true, [value_if_false]", 3, false, ""},
		{"sum_range, criteria_range1, criterion1, [criteria_range2, criterion2, ...]", 5, true, "[criteria_range2, criterion2, ...]"},
	} {
		parts := splitArgs(tc.args)
		got := ""
		if i := argPart(parts, tc.arg, tc.variadic); i >= 0 {
			got = parts[i]
		}
		if got != tc.want {
			t.Errorf("%s arg %d: %q, want %q", tc.args, tc.arg, got, tc.want)
		}
	}
}

func TestAutocompleteFunctions(t *testing.T) {
	m := newModel()
	press(t, m, "=su")
	list, _ := m.entry.assist.shown(m)
	if len(list) == 0 || list[0].name != "SUBSTITUTE" && list[0].name != "SUM" {
		t.Fatalf("suggestions %v", list)
	}
	for _, s := range list {
		if !strings.HasPrefix(s.name, "SU") && !strings.Contains(s.name, "SU") {
			t.Errorf("unexpected %s", s.name)
		}
	}
	scr := screen(m)
	if !strings.Contains(scr, "│ SUM ") || !strings.Contains(scr, "(value1, [value2, ...])") {
		t.Errorf("list not drawn:\n%s", scr)
	}
	if st := line(m, m.height-1); !strings.Contains(st, "Tab  insert") {
		t.Errorf("status %q", st)
	}
	// Down moves the highlight rather than committing; Tab inserts.
	for list[m.entry.assist.sel].name != "SUM" {
		press(t, m, "<down>")
	}
	press(t, m, "<tab>")
	if m.mode != modeEnter || m.line.text() != "=SUM(" {
		t.Fatalf("mode %v buf %q", m.mode, m.line.text())
	}
	if ctx := line(m, contextLine); !strings.HasPrefix(ctx, "SUM(value1, [value2, ...])") || !strings.Contains(ctx, "Sum of numbers") {
		t.Errorf("signature %q", ctx)
	}
	press(t, m, "1,")
	sig, _ := signature(&m.th, m.line.buf, m.line.pos)
	if !strings.Contains(sig, m.th.Argument.Render("[value2, ...]")) {
		t.Errorf("second argument not marked: %q", sig)
	}
	press(t, m, "2)", "<enter>")
	if input(m, "A1") != "=SUM(1,2)" || m.sheet.Value(addr("A1")).Num != 3 {
		t.Errorf("A1 %q", input(m, "A1"))
	}
}

func TestAutocompleteKeys(t *testing.T) {
	m := newModel()
	// Enter inserts while the list shows, then commits when it doesn't.
	press(t, m, "=ab", "<enter>")
	if m.line.text() != "=ABS(" {
		t.Fatalf("enter: %q", m.line.text())
	}
	press(t, m, "-4)", "<enter>")
	if m.sheet.Value(addr("A1")).Num != 4 {
		t.Fatalf("A1 = %v", m.sheet.Value(addr("A1")))
	}
	// Esc hides the list, a second Esc cancels the entry.
	press(t, m, "=ro")
	press(t, m, "<esc>")
	if list, _ := m.entry.assist.shown(m); list != nil || m.mode != modeEnter {
		t.Fatalf("esc: %v %v", list, m.mode)
	}
	press(t, m, "<esc>")
	if m.mode != modeReady {
		t.Fatalf("second esc: %v", m.mode)
	}
	// Arrows after an operator still point.
	press(t, m, "=1+", "<up>")
	if m.mode != modePoint {
		t.Errorf("mode %v", m.mode)
	}
	press(t, m, "<esc>", "<esc>")
	// Moving the caret hides the list; a cell reference gets no list.
	press(t, m, "=co", "<left>")
	if list, _ := m.entry.assist.shown(m); list != nil {
		t.Error("list after moving the caret")
	}
	press(t, m, "<esc>", "=B2")
	if list, _ := m.entry.assist.shown(m); list != nil {
		t.Errorf("list for a reference: %v", list)
	}
	// Plain text gets no list.
	press(t, m, "<esc>", "su")
	if list, _ := m.entry.assist.shown(m); list != nil {
		t.Error("list for text")
	}
}

func TestAutocompleteNamesAndMouse(t *testing.T) {
	m := tallModel()
	m.sheet.DefineName("Sales", rectOf("B1:B3"))
	press(t, m, "=SUM(sa")
	list, start := m.entry.assist.shown(m)
	if list[0].name != "Sales" || list[0].fn {
		t.Fatalf("names first: %v", list)
	}
	// Clicking a suggestion inserts it.
	b, _ := m.entry.assist.box(m)
	send(m, tea.MouseClickMsg{X: b.x + 3, Y: b.y + 1, Button: tea.MouseLeft})
	if m.line.text() != "=SUM(Sales" || m.line.pos != start+5 {
		t.Fatalf("click: %q", m.line.text())
	}
	// Editing inside existing parentheses doesn't double them.
	m = newModel()
	press(t, m, "=AB(1)", "<f2>", "<left>", "<left>", "<left>", "<backspace>", "B")
	press(t, m, "<tab>")
	if m.line.text() != "=ABS(1)" || m.line.pos != 5 {
		t.Errorf("into parens: %q at %d", m.line.text(), m.line.pos)
	}
}

func TestSignatureWhilePointing(t *testing.T) {
	m := newModel()
	press(t, m, "=JEV.TEST(", "<down>")
	if m.mode != modePoint {
		t.Fatalf("mode %v", m.mode)
	}
	if ctx := line(m, contextLine); !strings.HasPrefix(ctx, "JEV.TEST(value, question") {
		t.Errorf("context %q", ctx)
	}
}
