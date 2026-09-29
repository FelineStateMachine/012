package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

func TestTraceView(t *testing.T) {
	m := traceModel(t)
	m.sheet.Set(addr("A40"), "=B1")
	m.cur = addr("B1")
	press(t, m, "<alt+;>")
	if m.tview == nil {
		t.Fatal("tracing isn't on")
	}
	for a, want := range map[string]string{"A1": "precedent", "A3": "precedent", "C1": "dependent", "C2": "dependent", "A4": "", "D1": ""} {
		got := ""
		switch m.traceRole(addr(a)) {
		case &m.th.Precedent:
			got = "precedent"
		case &m.th.Dependent:
			got = "dependent"
		}
		if got != want {
			t.Errorf("%s: %q, want %q", a, got, want)
		}
	}
	// Off-screen dependents are listed with an arrow toward them.
	if l := line(m, contextLine); !strings.HasPrefix(l, "B1 reads A1, A2:A3; read by C1, C2, A40↓") || !strings.Contains(l, "Alt+'  list") {
		t.Errorf("context %q", l)
	}
	// It follows the pointer.
	press(t, m, "<right>")
	if l := line(m, contextLine); !strings.HasPrefix(l, "C1 reads B1") || m.traceRole(addr("B1")) != &m.th.Precedent {
		t.Errorf("after moving: %q", l)
	}
	press(t, m, "<left>", "<left>")
	if l := line(m, contextLine); !strings.HasPrefix(l, "A1 is read by B1") {
		t.Errorf("a number: %q", l)
	}
	press(t, m, "<down>", "<down>", "<down>")
	if l := line(m, contextLine); !strings.Contains(l, "A4 reads no cells and no formula reads it") {
		t.Errorf("nothing: %q", l)
	}
	// Stepping through a trace shows that trace, and Esc comes back.
	m.cur = addr("B1")
	press(t, m, "<alt+.>")
	if m.cur != addr("C1") || !strings.HasPrefix(line(m, contextLine), "3 dependents of B1") {
		t.Errorf("stepping: %v %q", m.cur, line(m, contextLine))
	}
	press(t, m, "<esc>", "<alt+;>")
	if m.tview != nil || m.traceRole(addr("A1")) != nil {
		t.Error("tracing stayed on")
	}
}

func TestTraceViewEdits(t *testing.T) {
	m := traceModel(t)
	m.cur = addr("A1")
	run(m, m.runCommand("view.trace"))
	m.cur = addr("D1")
	press(t, m, "=A1*3", "<enter>", "<up>")
	if m.traceRole(addr("A1")) != &m.th.Precedent {
		t.Error("a new formula's precedent isn't marked")
	}
	m.cur = addr("A1")
	press(t, m, "<right>", "<left>")
	if m.traceRole(addr("D1")) != &m.th.Dependent {
		t.Error("a new dependent isn't marked")
	}
}

func TestTraceList(t *testing.T) {
	m := traceModel(t)
	other, _ := m.book().AddSheet("Data", 1)
	other.Set(addr("A1"), "=Sheet1!B1")
	m.sheet.DefineName("Pair", rectOf("A2:A3"))
	m.cur = addr("B1")
	press(t, m, "<alt+'>")
	scr := screen(m)
	for _, want := range []string{"Precedents and dependents of B1", "A1", "Pair", "C2", "Data!A1", "dependent, on Data"} {
		if !strings.Contains(scr, want) {
			t.Errorf("missing %q in\n%s", want, scr)
		}
	}
	press(t, m, "Data", "<enter>")
	if m.sheet != other || m.cur != addr("A1") {
		t.Errorf("went to %s!%v", m.sheet.Name(), m.cur)
	}
	m.cur = addr("B5")
	press(t, m, "<alt+'>")
	if !strings.Contains(line(m, contextLine), "B5 reads no cells") {
		t.Errorf("nothing to list: %q", line(m, contextLine))
	}
}

func TestTraceViewSpill(t *testing.T) {
	m := newModel()
	m.sheet.Set(addr("A1"), "=SEQUENCE(3)")
	m.sheet.Set(addr("B1"), "=SUM(A2:A3)")
	m.cur = addr("A2")
	run(m, m.runCommand("view.trace"))
	if l := line(m, contextLine); !strings.HasPrefix(l, "A2 reads A1; read by B1") {
		t.Errorf("a spilled cell: %q", l)
	}
	m.cur = addr("A1")
	send(m, teaSize(100, 30))
	if m.traceRole(addr("A3")) != &m.th.Dependent || m.traceRole(addr("B1")) != &m.th.Dependent {
		t.Error("the anchor's spill and what reads it aren't dependents")
	}
	if got := m.sheet.Value(addr("A3")); got.Kind != sheet.Number {
		t.Errorf("A3 = %v", got)
	}
}
