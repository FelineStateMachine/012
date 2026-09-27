package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// An array formula spills: the spilled cells say where they come from,
// show its formula dimmed, and refuse edits as Sheets' array results do,
// while the formula's own cell and a range holding it stay editable.
func TestSpillInGrid(t *testing.T) {
	m := newModel()
	press(t, m, "=SEQUENCE(3, 2)", "<enter>", "<up>")
	if shows(m, "B3") != "6" {
		t.Fatalf("B3 shows %q", shows(m, "B3"))
	}
	if !strings.Contains(line(m, contextLine), "Spills into A1:B3") {
		t.Errorf("anchor's context line %q", line(m, contextLine))
	}
	press(t, m, "<down>", "<right>")
	if got := line(m, contextLine); !strings.Contains(got, "Spilled from A1") {
		t.Errorf("spilled cell's context line %q", got)
	}
	if got := bar(m); got != "=SEQUENCE(3, 2)" {
		t.Errorf("formula bar %q, want the anchor's formula", got)
	}
	for _, keys := range [][]string{{"x"}, {"<delete>"}, {"<enter>"}, {"<ctrl+d>"}} {
		press(t, m, keys...)
		if m.mode != modeReady || !strings.Contains(line(m, contextLine), "B2 shows part of the array A1 spills") {
			t.Errorf("%v: mode %v, %q", keys, m.mode, line(m, contextLine))
		}
		press(t, m, "<esc>")
	}
	// Formatting a spilled cell keeps its value.
	press(t, m, "<ctrl+b>")
	if c := m.sheet.Cell(addr("B2")); shows(m, "B2") != "4" || c == nil || !c.Style.Bold {
		t.Errorf("B2 after bold: %q %+v", shows(m, "B2"), c)
	}
	// Inserting a row through the spill recomputes it.
	press(t, m, "<ctrl+alt+=>")
	if strings.Contains(m.note, "spills") || shows(m, "B3") != "6" || shows(m, "B4") != "" {
		t.Errorf("after inserting a row: %q, B3 %q, B4 %q", m.note, shows(m, "B3"), shows(m, "B4"))
	}
	// Copying pastes values; clearing the whole spill clears the formula.
	press(t, m, "<ctrl+g>", "A1:B4", "<enter>", "<delete>")
	if shows(m, "A1") != "" || shows(m, "B3") != "" {
		t.Errorf("after clearing the spill: A1 %q B3 %q", shows(m, "A1"), shows(m, "B3"))
	}
}

// Cells in the way of a spill make the formula #REF!, with Sheets'
// explanation, until they're cleared.
func TestSpillBlockedInGrid(t *testing.T) {
	m := newModel()
	press(t, m, "<down>", "<down>", "x", "<enter>", "<ctrl+home>", "=SEQUENCE(3)", "<enter>", "<up>")
	if shows(m, "A1") != "#REF!" {
		t.Fatalf("A1 shows %q", shows(m, "A1"))
	}
	if got := line(m, contextLine); !strings.Contains(got, "would overwrite data in A3") {
		t.Errorf("context line %q", got)
	}
	press(t, m, "<down>", "<down>", "<delete>")
	if shows(m, "A1") != "1" || shows(m, "A3") != "3" {
		t.Errorf("after clearing A3: A1 %q A3 %q", shows(m, "A1"), shows(m, "A3"))
	}
}

// A protected range over a spill doesn't make the spill ask: refusing to
// edit a spilled cell comes first, and the formula's own cell asks as any
// protected cell does.
func TestSpillProtected(t *testing.T) {
	m := newModel()
	press(t, m, "=SEQUENCE(3)", "<enter>")
	m.sheet.Protect(sheet.Protection{Range: sheet.Rect{From: addr("A1"), To: addr("A5")}})
	press(t, m, "<down>", "<up>", "x") // into A2, a spilled cell
	if m.mode != modeReady || !strings.Contains(line(m, contextLine), "A2 shows part of the array A1 spills") {
		t.Errorf("typing into a protected spilled cell: mode %v, %q", m.mode, line(m, contextLine))
	}
	press(t, m, "<esc>", "<shift+f2>")
	if !strings.Contains(line(m, contextLine), "A2 shows part of the array A1 spills") {
		t.Errorf("a note on a spilled cell: %q", line(m, contextLine))
	}
}
