package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func traceModel(t *testing.T) *Model {
	m := newModel()
	for a, in := range map[string]string{"A1": "1", "A2": "2", "A3": "3", "B1": "=A1+SUM(A2:A3)", "C1": "=B1*2", "C2": "=B1"} {
		m.sheet.Set(addr(a), in)
	}
	return m
}

func TestTracePrecedents(t *testing.T) {
	m := traceModel(t)
	m.cur = addr("B1")
	press(t, m, "<alt+,>")
	if m.cur != addr("A1") || !m.trace.covers(m.sheet, addr("A3")) || m.trace.covers(m.sheet, addr("B1")) {
		t.Fatalf("cur %v", m.cur)
	}
	if l := line(m, contextLine); !strings.HasPrefix(l, "2 precedents of B1: A1, A2:A3") || !strings.Contains(l, "Alt+,  next") {
		t.Errorf("context %q", l)
	}
	press(t, m, "<alt+,>")
	if m.cur != addr("A2") {
		t.Errorf("next: %v", m.cur)
	}
	press(t, m, "<alt+,>")
	if m.cur != addr("A1") {
		t.Errorf("wrap: %v", m.cur)
	}
	// Esc returns to the traced cell and ends the trace.
	press(t, m, "<esc>")
	if m.cur != addr("B1") || m.trace != nil {
		t.Errorf("esc: %v %v", m.cur, m.trace)
	}
}

func TestTraceDependents(t *testing.T) {
	m := traceModel(t)
	m.cur = addr("B1")
	press(t, m, "<alt+.>")
	if m.cur != addr("C1") || !m.trace.covers(m.sheet, addr("C2")) {
		t.Fatalf("cur %v", m.cur)
	}
	// Any other key ends the trace and does its own thing.
	press(t, m, "<down>")
	if m.trace != nil || m.cur != addr("C2") {
		t.Errorf("down: %v %v", m.trace, m.cur)
	}
	// A click ends it too.
	m.cur = addr("B1")
	press(t, m, "<alt+.>")
	send(m, tea.MouseClickMsg{X: minRowHdrW + 1, Y: gridTop + 4, Button: tea.MouseLeft})
	if m.trace != nil {
		t.Error("click kept the trace")
	}
	m.cur = addr("C1")
	press(t, m, "<alt+.>")
	if m.trace != nil || !strings.Contains(line(m, contextLine), "No formulas read C1") {
		t.Errorf("no dependents: %q", line(m, contextLine))
	}
	press(t, m, "<alt+,>", "<alt+,>")
	if m.cur != addr("B1") || m.trace == nil {
		t.Errorf("from C1 back: %v", m.cur)
	}
}
