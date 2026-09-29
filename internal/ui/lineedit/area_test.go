package lineedit

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func rowsText(buf []rune, rows []Row) []string {
	var out []string
	for _, r := range rows {
		s := string(buf[r.Start:r.End])
		if r.Cont {
			s = strings.Repeat(" ", Indent) + s
		}
		out = append(out, s)
	}
	return out
}

func TestWrapAtPipes(t *testing.T) {
	buf := []rune("ls | where size > 1kb | sort-by size | first 3\n$x")
	got := rowsText(buf, Wrap(buf, 24))
	want := []string{"ls | where size > 1kb ", "  | sort-by size ", "  | first 3", "$x"}
	if strings.Join(got, "/") != strings.Join(want, "/") {
		t.Errorf("rows %q", got)
	}
	// Every character is on a row, once.
	long := []rune(strings.Repeat("abcdefghij", 20))
	n := 0
	for _, r := range Wrap(long, 30) {
		n += r.End - r.Start
		if w := RowWidth(long, r.Start, r.End); w > 30 {
			t.Errorf("a row %d wide", w)
		}
	}
	if n != len(long) {
		t.Errorf("%d of %d characters on rows", n, len(long))
	}
}

func key(s string) tea.KeyPressMsg {
	switch s {
	case "up":
		return tea.KeyPressMsg{Code: tea.KeyUp}
	case "down":
		return tea.KeyPressMsg{Code: tea.KeyDown}
	case "home":
		return tea.KeyPressMsg{Code: tea.KeyHome}
	case "end":
		return tea.KeyPressMsg{Code: tea.KeyEnd}
	case "enter":
		return tea.KeyPressMsg{Code: tea.KeyEnter}
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func TestAreaMovesByRows(t *testing.T) {
	var a Area
	a.SetText("ls | where size > 1kb | sort-by size")
	const w = 24
	a.Key(key("home"), w)
	if a.Pos != 22 {
		t.Errorf("Home on the second row: %d", a.Pos)
	}
	a.Key(key("up"), w)
	if row, _ := Caret(Wrap(a.Buf, w), a.Buf, a.Pos); row != 0 {
		t.Errorf("Up: row %d", row)
	}
	a.Key(key("end"), w)
	if a.Pos != 21 {
		t.Errorf("End on a wrapped row stays on it: %d", a.Pos)
	}
	a.Key(key("down"), w)
	a.Key(key("end"), w)
	if a.Pos != len(a.Buf) {
		t.Errorf("End on the last row: %d", a.Pos)
	}
	a.SetText("  $x")
	a.Key(key("enter"), w)
	a.Key(key("y"), w)
	if a.Text() != "  $x\n  y" {
		t.Errorf("a new line keeps the indent: %q", a.Text())
	}
	if row, col := Caret(Wrap(a.Buf, w), a.Buf, a.Pos); row != 1 || col != 3 {
		t.Errorf("caret %d,%d", row, col)
	}
	a.InsertText("\ta\x01")
	if a.Text() != "  $x\n  y  a" || a.Offset() != len("  $x\n  y  a") {
		t.Errorf("inserted %q", a.Text())
	}
}
