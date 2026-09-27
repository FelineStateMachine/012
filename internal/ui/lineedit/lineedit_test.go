package lineedit

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestEditing(t *testing.T) {
	var l Line
	l.Set("=SUM(A1")
	l.Key(tea.KeyPressMsg{Code: tea.KeyLeft})
	l.Key(tea.KeyPressMsg{Code: tea.KeyBackspace})
	l.Insert("B\tC")
	if got := l.Text(); got != "=SUM(B C1" {
		t.Fatalf("text %q", got)
	}
	if l.Head() != "=SUM(B C" || l.Tail() != "1" {
		t.Errorf("head %q tail %q", l.Head(), l.Tail())
	}
	if !l.IsFormula() {
		t.Error("not a formula")
	}
	l.Key(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if l.Pos != 0 || l.CanPoint() {
		t.Errorf("home: pos %d", l.Pos)
	}
	l.SetCaret(5)
	if !l.CanPoint() {
		t.Errorf("after ( a reference may follow: pos %d", l.Pos)
	}
	l.Clear()
	if l.Text() != "" || l.Pos != 0 {
		t.Errorf("clear left %q", l.Text())
	}
}

func TestTyped(t *testing.T) {
	if Typed(tea.KeyPressMsg{Code: 'x', Text: "x", Mod: tea.ModCtrl}) != "" {
		t.Error("ctrl+x typed text")
	}
	if Typed(tea.KeyPressMsg{Code: 'x', Text: "x"}) != "x" {
		t.Error("x typed nothing")
	}
}
