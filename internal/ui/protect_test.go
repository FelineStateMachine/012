package ui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/choicebar"
)

func TestProtectRangeWarns(t *testing.T) {
	m := newModel()
	press(t, m, "<shift+down>", "<shift+right>")
	m.runCommand("data.protect_range")
	if l := line(m, contextLine); !strings.HasPrefix(l, "Describe A1:B2 (optional):") {
		t.Fatalf("prompt %q", l)
	}
	press(t, m, "Inputs", "<enter>")
	if ps := m.sheet.Protections(); len(ps) != 1 || ps[0].Desc != "Inputs" || ps[0].Range != rectOf("A1:B2") {
		t.Fatalf("protections %+v", ps)
	}
	// Typing into a protected cell asks first; Esc backs out.
	press(t, m, "<esc>", "7")
	if l := line(m, contextLine); !strings.Contains(l, "A1:B2 is protected.") || !strings.Contains(l, "Edit anyway") {
		t.Fatalf("no warning: %q", l)
	}
	if !strings.Contains(line(m, m.height-1), "Inputs") {
		t.Errorf("status %q", line(m, m.height-1))
	}
	press(t, m, "<esc>")
	if m.mode != modeReady || m.sheet.Value(addr("A1")).Kind != sheet.Empty {
		t.Fatalf("Esc went on: mode %v, A1 %v", m.mode, m.sheet.Value(addr("A1")))
	}
	// Enter goes on with the keystroke that asked.
	press(t, m, "7", "<enter>")
	if m.mode != modeEnter || m.line.Text() != "7" {
		t.Fatalf("after Enter: mode %v, %q", m.mode, m.line.Text())
	}
	press(t, m, "<enter>")
	if m.sheet.Value(addr("A1")).Num != 7 {
		t.Fatalf("A1 = %v", m.sheet.Value(addr("A1")))
	}
	// Commands ask through what they edit; outside, nothing asks.
	press(t, m, "<up>")
	m.runCommand("clear")
	if _, ok := m.overlay.(*choicebar.Bar); !ok {
		t.Fatal("clear didn't ask")
	}
	press(t, m, "<enter>")
	if m.sheet.Value(addr("A1")).Kind != sheet.Empty {
		t.Errorf("clear after Enter: %v", m.sheet.Value(addr("A1")))
	}
	press(t, m, "<right>", "<right>", "5", "<enter>")
	if m.sheet.Value(addr("C1")).Num != 5 {
		t.Errorf("C1 = %v", m.sheet.Value(addr("C1")))
	}
	// Inserting rows above moves the range without asking.
	press(t, m, "<up>")
	m.runCommand("insert.row_above")
	if m.overlay != nil || m.sheet.Protections()[0].Range != rectOf("A2:B3") {
		t.Errorf("insert: overlay %T, %+v", m.overlay, m.sheet.Protections())
	}
	// Pasted text into the range asks too.
	press(t, m, "<left>", "<left>", "<down>")
	send(m, tea.PasteMsg{Content: "1\t2"})
	if _, ok := m.overlay.(*choicebar.Bar); !ok {
		t.Fatal("paste didn't ask")
	}
	press(t, m, "<enter>")
	if m.sheet.Value(addr("B2")).Num != 2 {
		t.Errorf("B2 = %v", m.sheet.Value(addr("B2")))
	}
	m.runCommand("data.unprotect")
	if len(m.sheet.Protections()) != 0 {
		t.Errorf("unprotect left %+v", m.sheet.Protections())
	}
}

func TestProtectSheetAndPicker(t *testing.T) {
	m := tallModel()
	m.Update(teaSize(100, 30))
	m.runCommand("data.protect")
	s := screen(m)
	if !strings.Contains(s, "Protected sheets and ranges") || !strings.Contains(s, "+ Protect the sheet") {
		t.Fatalf("picker:\n%s", s)
	}
	press(t, m, "<down>", "<enter>", "<enter>")
	if ps := m.sheet.Protections(); len(ps) != 1 || !ps[0].Sheet {
		t.Fatalf("protections %+v", ps)
	}
	// Back in the picker; the sheet's protection is listed.
	if !strings.Contains(screen(m), "whole sheet") {
		t.Fatalf("picker after adding:\n%s", screen(m))
	}
	press(t, m, "<esc>")
	// Anything on a protected sheet asks, inserting rows included.
	m.runCommand("insert.row_above")
	if _, ok := m.overlay.(*choicebar.Bar); !ok || !strings.Contains(line(m, contextLine), "Sheet1 is protected.") {
		t.Fatalf("insert didn't ask: %q", line(m, contextLine))
	}
	press(t, m, "<esc>")
	m.runCommand("data.protect")
	press(t, m, "<down>", "<down>", "<ctrl+d>")
	if len(m.sheet.Protections()) != 0 || !strings.Contains(screen(m), "Removed the protection of the sheet") {
		t.Errorf("remove: %+v\n%s", m.sheet.Protections(), screen(m))
	}
}
