package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Pastes and fills meet the validation of the cells they write, as in
// Sheets: a rule that rejects takes the whole change back, one that warns
// keeps it and says so.
func TestValidationOnPastesAndFills(t *testing.T) {
	setup := func(t *testing.T) *Model {
		m := newModel()
		mustValidate(t, m, `{"ranges":"B1:B9","criteria":"number","condition":"between","values":["1","10"],"reject":true}`)
		mustValidate(t, m, `{"ranges":"C1:C9","criteria":"number","condition":"between","values":["1","10"]}`)
		m.sheet.Set(addr("A1"), "5")
		m.sheet.Set(addr("A2"), "50")
		return m
	}

	t.Run("paste rejected", func(t *testing.T) {
		m := setup(t)
		press(t, m, "<shift+down>", "<ctrl+c>", "<right>", "<ctrl+v>")
		if m.mode != modeError || input(m, "B1") != "" || input(m, "B2") != "" || m.sheet.CanRedo() {
			t.Fatalf("mode %v B1 %q B2 %q", m.mode, input(m, "B1"), input(m, "B2"))
		}
		if m.errMsg != "Paste undone: Invalid entry in B2: Input must be a number between 1 and 10" {
			t.Errorf("reason %q", m.errMsg)
		}
	})

	t.Run("paste warned", func(t *testing.T) {
		m := setup(t)
		press(t, m, "<shift+down>", "<ctrl+c>", "<right>", "<right>", "<ctrl+v>")
		if input(m, "C2") != "50" || !m.sheet.Look(addr("C2")).Invalid {
			t.Fatalf("C2 %q", input(m, "C2"))
		}
		if l := line(m, contextLine); !strings.Contains(l, "1 cell invalid, first C2: Input must be a number between 1 and 10") {
			t.Errorf("context line %q", l)
		}
	})

	t.Run("text paste rejected", func(t *testing.T) {
		m := setup(t)
		m.cur = addr("B1")
		send(m, tea.PasteMsg{Content: "3\n30"})
		if m.mode != modeError || input(m, "B1") != "" {
			t.Errorf("mode %v B1 %q", m.mode, input(m, "B1"))
		}
	})

	t.Run("fill down rejected", func(t *testing.T) {
		m := setup(t)
		m.sheet.Set(addr("B1"), "7")
		m.sheet.Set(addr("B1"), "=A1*2") // 10, valid; filled down it's 100
		press(t, m, "<right>", "<shift+down>", "<ctrl+d>")
		if m.mode != modeError || input(m, "B2") != "" {
			t.Errorf("mode %v B2 %q", m.mode, input(m, "B2"))
		}
	})

	t.Run("ctrl+enter rejected keeps the entry", func(t *testing.T) {
		m := setup(t)
		press(t, m, "<right>", "<shift+down>", "=A1*2", "<ctrl+enter>")
		if m.mode != modeEdit || input(m, "B1") != "" || !strings.Contains(line(m, contextLine), "Invalid entry in B2") {
			t.Errorf("mode %v B1 %q %q", m.mode, input(m, "B1"), line(m, contextLine))
		}
	})
}

// Cut and paste takes the cells' rules along.
func TestRulesMoveWithCutAndPaste(t *testing.T) {
	m := newModel()
	mustValidate(t, m, `{"ranges":"A1:A2","criteria":"list","items":["Yes","No"]}`)
	m.sheet.Set(addr("A1"), "Yes")
	press(t, m, "<shift+down>", "<ctrl+x>", "<right>", "<right>", "<ctrl+v>")
	vs := m.sheet.Validations()
	if len(vs) != 1 || sheet.RangesText(vs[0].Ranges) != "C1:C2" || !m.sheet.Look(addr("C1")).Dropdown {
		t.Fatalf("validations %+v", vs)
	}
	if m.mode == modeError {
		t.Errorf("a move was refused: %s", screen(m))
	}
}
