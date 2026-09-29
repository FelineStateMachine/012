package lineedit

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

var (
	ctrlLeft      = tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModCtrl}
	altRight      = tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt}
	ctrlBackspace = tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModCtrl}
	legacyCtrlBS  = tea.KeyPressMsg{Code: 'h', Mod: tea.ModCtrl}
	optBackspace  = tea.KeyPressMsg{Code: tea.KeyBackspace, Mod: tea.ModAlt}
	altD          = tea.KeyPressMsg{Code: 'd', Mod: tea.ModAlt}
	ctrlU         = tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl}
)

func TestWordMoves(t *testing.T) {
	var l Line
	l.Set("=SUM(A1:B2) + total")
	for _, want := range []int{14, 12, 10, 8, 7, 5, 4, 1, 0, 0} {
		l.Key(ctrlLeft)
		if l.Pos != want {
			t.Fatalf("ctrl+left to %d, want %d", l.Pos, want)
		}
	}
	for _, want := range []int{1, 4, 5, 7, 8, 10, 11, 13, 19, 19} {
		l.Key(altRight)
		if l.Pos != want {
			t.Fatalf("alt+right to %d, want %d", l.Pos, want)
		}
	}
}

func TestWordDeletes(t *testing.T) {
	var l Line
	l.Set("=SUM(A1:B2")
	for _, step := range []struct {
		k    tea.KeyPressMsg
		want string
	}{
		{ctrlBackspace, "=SUM(A1:"},
		{legacyCtrlBS, "=SUM(A1"},
		{optBackspace, "=SUM("},
	} {
		l.Key(step.k)
		if l.Text() != step.want {
			t.Fatalf("%s left %q, want %q", step.k, l.Text(), step.want)
		}
	}
	l.Set("hello big world")
	l.Pos = 6
	l.Key(altD)
	if l.Text() != "hello  world" || l.Pos != 6 {
		t.Errorf("alt+d left %q at %d", l.Text(), l.Pos)
	}
	l.Key(ctrlU)
	if l.Text() != " world" || l.Pos != 0 {
		t.Errorf("ctrl+u left %q at %d", l.Text(), l.Pos)
	}
}

func TestAreaWordKeys(t *testing.T) {
	a := &Area{}
	a.Set("ls\n| where size > 1kb")
	a.Key(ctrlBackspace, 40)
	if a.Text() != "ls\n| where size > " {
		t.Fatalf("ctrl+backspace left %q", a.Text())
	}
	a.Key(ctrlU, 40)
	if a.Text() != "ls\n" {
		t.Errorf("ctrl+u took more than its line: %q", a.Text())
	}
}
