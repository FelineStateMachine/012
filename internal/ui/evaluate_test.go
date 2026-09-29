package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestEvaluateFormula(t *testing.T) {
	m := traceModel(t)
	m.cur = addr("A1")
	if commands["data.evaluate"].available(m) {
		t.Error("Evaluate formula on a number")
	}
	m.cur = addr("C1")
	press(t, m, "<alt+=>")
	if m.mode != modeMenu || !strings.Contains(line(m, menuLine), "EVAL") {
		t.Fatalf("mode %v:\n%s", m.mode, screen(m))
	}
	if scr := screen(m); !strings.Contains(scr, "Evaluate C1") || !strings.Contains(scr, "Next  B1 = 6") {
		t.Errorf("box:\n%s", scr)
	}
	press(t, m, "<right>")
	if scr := screen(m); !strings.Contains(scr, "Evaluate C1 › B1") || !strings.Contains(scr, "=A1+SUM(A2:A3)") {
		t.Errorf("stepped in:\n%s", scr)
	}
	press(t, m, "<left>", "<enter>")
	if scr := screen(m); !strings.Contains(scr, "Value  12") {
		t.Errorf("done:\n%s", scr)
	}
	// A click outside closes it, as Esc does.
	send(m, tea.MouseClickMsg{X: 2, Y: m.height - 3, Button: tea.MouseLeft})
	if m.mode != modeReady || m.overlay != nil {
		t.Errorf("mode %v after a click outside", m.mode)
	}
	press(t, m, "<alt+=>", "<esc>")
	if m.mode != modeReady {
		t.Errorf("mode %v after Esc", m.mode)
	}
}
