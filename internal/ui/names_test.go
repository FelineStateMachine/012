package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

func rectOf(s string) sheet.Rect {
	r, _ := sheet.ParseRange(s)
	return r
}

func TestDefineNameFromSelection(t *testing.T) {
	m := newModel()
	press(t, m, "<right>", "10", "<enter>", "20", "<enter>", "<up>", "<shift+up>")
	m.runCommand("data.define_name")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Name for B1:B2:") {
		t.Fatalf("prompt %q", l)
	}
	press(t, m, "Sales", "<enter>")
	if n, ok := m.sheet.LookupName("sales"); !ok || n.Range != rectOf("B1:B2") {
		t.Fatalf("name %v %v", n, ok)
	}
	if l := line(m, contextLine); !strings.Contains(l, "=SUM(Sales)") {
		t.Errorf("note %q", l)
	}
	// The name box shows the name of the selected range.
	if l := line(m, formulaLine); !strings.HasPrefix(l, " Sales") {
		t.Errorf("name box %q", l)
	}
	press(t, m, "<esc>", "<down>", "<down>", "=SUM(Sales)", "<enter>")
	if m.sheet.Value(addr("B3")).Num != 30 {
		t.Errorf("B3 = %v", m.sheet.Value(addr("B3")))
	}
	press(t, m, "<ctrl+z>", "<ctrl+z>")
	if _, ok := m.sheet.LookupName("Sales"); ok {
		t.Error("undo kept the name")
	}

	// A bad name says why.
	m.runCommand("data.define_name")
	press(t, m, "A1", "<enter>")
	if m.mode != modeError || !strings.Contains(m.errMsg, "cell reference") {
		t.Errorf("mode %v %q", m.mode, m.errMsg)
	}
}

func TestNamedRangesPicker(t *testing.T) {
	m := tallModel()
	m.Update(teaSize(120, 40))
	m.sheet.Set(addr("A1"), "=SUM(Costs)+Rate")
	m.sheet.DefineName("Costs", rectOf("C2:C9"))
	m.sheet.DefineName("Rate", rectOf("E1"))
	press(t, m, "<alt+d>")
	if !strings.Contains(screen(m), "Named ranges") || !strings.Contains(screen(m), "Trace precedents") {
		t.Fatalf("Data menu:\n%s", screen(m))
	}
	press(t, m, "<esc>")
	m.runCommand("data.named_ranges")
	scr := screen(m)
	for _, want := range []string{"+ Add a range", "Costs", "C2:C9", "Rate", "E1"} {
		if !strings.Contains(scr, want) {
			t.Errorf("missing %q in\n%s", want, scr)
		}
	}
	press(t, m, "cos")
	if st := line(m, m.height-1); !strings.Contains(st, "used in 1 formula") || !strings.Contains(st, "F2  edit") {
		t.Errorf("status %q", st)
	}
	press(t, m, "<enter>")
	if m.mode != modeReady || m.selection() != rectOf("C2:C9") {
		t.Fatalf("go to: %v %v", m.mode, m.selection())
	}

	// F2 renames and repoints; formulas follow the new name.
	m.runCommand("data.named_ranges")
	press(t, m, "rate", "<f2>")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Rename Rate: Rate") {
		t.Fatalf("rename prompt %q", l)
	}
	press(t, m, "Tax", "<enter>")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Range for Tax: E1") {
		t.Fatalf("range prompt %q", l)
	}
	press(t, m, "<shift+down>", "<enter>")
	if n, _ := m.sheet.LookupName("tax"); n.Range != rectOf("E1:E2") || input(m, "A1") != "=SUM(Costs)+Tax" {
		t.Fatalf("edited: %v %q", n, input(m, "A1"))
	}
	if _, ok := m.overlay.(*namesPicker); !ok {
		t.Fatal("picker didn't come back")
	}

	// Ctrl+D deletes; the picker stays open and says how to undo.
	press(t, m, "tax", "<ctrl+d>")
	if _, ok := m.sheet.LookupName("Tax"); ok {
		t.Fatal("not deleted")
	}
	if st := line(m, m.height-1); !strings.Contains(st, "Deleted Tax") {
		t.Errorf("status %q", st)
	}
	press(t, m, "<esc>", "<ctrl+z>")
	if _, ok := m.sheet.LookupName("Tax"); !ok {
		t.Error("undo didn't restore")
	}

	// Add a range names the selection the picker opened with.
	press(t, m, "<ctrl+home>", "<shift+right>")
	m.runCommand("data.named_ranges")
	press(t, m, "<enter>", "Top", "<enter>")
	if n, ok := m.sheet.LookupName("Top"); !ok || n.Range != rectOf("A1:B1") {
		t.Errorf("added %v %v", n, ok)
	}
}

func TestGotoAcceptsNamesAndRanges(t *testing.T) {
	m := newModel()
	m.sheet.DefineName("Totals", rectOf("D4:E6"))
	press(t, m, "<f5>", "totals", "<enter>")
	if m.selection() != rectOf("D4:E6") {
		t.Errorf("name: %v", m.selection())
	}
	press(t, m, "<f5>", "B2:C3", "<enter>")
	if m.selection() != rectOf("B2:C3") {
		t.Errorf("range: %v", m.selection())
	}
	press(t, m, "<f5>", "nope", "<enter>")
	if m.mode != modeError || m.errMsg != "Not a cell, range or named range: nope" {
		t.Errorf("%v %q", m.mode, m.errMsg)
	}
}
