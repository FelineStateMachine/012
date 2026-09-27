package theme

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestKeyLabel(t *testing.T) {
	for in, want := range map[string]string{
		"ctrl+s":       "Ctrl+S",
		"f10":          "F10",
		"delete":       "Del",
		"esc":          "Esc",
		"alt+pgdown":   "Alt+PgDn",
		"ctrl+shift+p": "Ctrl+Shift+P",
	} {
		if got := KeyLabel(in); got != want {
			t.Errorf("KeyLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFrameKeepsEveryLineTheSameWidth(t *testing.T) {
	th := New(true)
	lines := th.Frame(12, "A long title that won't fit", "3 of 9", []string{PadRight("row", 12), SepRow, Cells(th.MenuBar, "a row that is too long", 12)})
	if len(lines) != 5 {
		t.Fatalf("%d lines, want 5", len(lines))
	}
	for i, l := range lines {
		if w := ansi.StringWidth(l); w != 14 {
			t.Errorf("line %d is %d wide: %q", i, w, ansi.Strip(l))
		}
	}
	if got := ansi.Strip(lines[4]); got != "└─── 3 of 9 ─┘" {
		t.Errorf("footer %q", got)
	}
}

func TestPadding(t *testing.T) {
	if got := PadLeft("7", 3) + "|" + PadRight("7", 3) + "|" + Center("ab", 5); got != "  7|7  | ab  " {
		t.Errorf("got %q", got)
	}
}
