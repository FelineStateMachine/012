package ui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

func TestLinksRenderAsHyperlinks(t *testing.T) {
	m := newModel()
	press(t, m, "https://example.com", "<enter>", `=HYPERLINK("example.org", "Docs")`, "<enter>", "plain text", "<enter>")
	v := m.View().Content
	for _, want := range []string{"\x1b]8;;https://example.com\x07", "\x1b]8;;https://example.org\x07"} {
		if !strings.Contains(v, want) {
			t.Errorf("view lacks hyperlink %q", want)
		}
	}
	if strings.Contains(ansi.Strip(strings.Split(v, "\n")[gridTop+2]), "plain") && strings.Contains(strings.Split(v, "\n")[gridTop+2], "\x1b]8") {
		t.Error("plain text drawn as a link")
	}
	// The link survives the pointer and a composited overlay. The window is
	// wide enough that the palette doesn't cover column A.
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 30})
	press(t, m, "<ctrl+home>", "<ctrl+k>")
	if !strings.Contains(m.View().Content, "https://example.com") {
		t.Error("hyperlink lost under the pointer with an overlay open")
	}
}

func TestErrorCellsAreMarked(t *testing.T) {
	m := newModel()
	press(t, m, "10", "<enter>", "=A1/0", "<enter>", "=A2+1", "<enter>")
	v := m.View().Content
	// Curly underline (4:3) under the error's text only.
	if !strings.Contains(v, "4:3m#") || !strings.Contains(ansi.Strip(v), "#DIV/0!") {
		t.Errorf("no curly underline on errors: %q", v)
	}
	press(t, m, "<up>")
	if got := line(m, contextLine); got != "#DIV/0!  From A2: division by zero in A1/0" {
		t.Errorf("context line %q", got)
	}
	press(t, m, "<up>")
	if got := line(m, contextLine); got != "#DIV/0!  Division by zero in A1/0" {
		t.Errorf("context line %q", got)
	}
	press(t, m, "<up>")
	if got := line(m, contextLine); got != "" {
		t.Errorf("context line on a number %q", got)
	}
}

func TestCursorOnlyWhileTyping(t *testing.T) {
	m := newModel()
	cases := []struct {
		keys  []string
		shape tea.CursorShape
		shown bool
	}{
		{nil, 0, false},
		{[]string{"1"}, tea.CursorBar, true},
		{[]string{"<esc>", "<f5>"}, tea.CursorBar, true},
		{[]string{"<esc>", "=", "<up>"}, 0, false}, // pointing: the pointer shows where
		{[]string{"<esc>", "<esc>", "<ctrl+k>"}, tea.CursorBar, true},
		{[]string{"<esc>", "<alt+f>"}, 0, false},
	}
	for i, c := range cases {
		press(t, m, c.keys...)
		cur := m.View().Cursor
		if (cur != nil) != c.shown || cur != nil && cur.Shape != c.shape {
			t.Errorf("step %d (%v): cursor %+v", i, c.keys, cur)
		}
		if cur != nil && cur.Y >= gridTop && cur.Y < m.height-1 {
			t.Errorf("step %d: cursor in the grid at %d", i, cur.Y)
		}
	}
}

func TestLiveLightDark(t *testing.T) {
	m := newModel()
	for _, ev := range []tea.Msg{uv.LightColorSchemeEvent{}, uv.DarkColorSchemeEvent{}} {
		_, cmd := m.Update(ev)
		asked := false
		for _, msg := range flatten(cmd) {
			asked = asked || strings.Contains(fmt.Sprintf("%T", msg), "ackgroundColor")
		}
		if !asked {
			t.Errorf("%T didn't ask for the background color", ev)
		}
	}
	send(m, tea.BackgroundColorMsg{Color: color.White})
	if m.th.link.GetForeground() != newTheme(false).link.GetForeground() {
		t.Error("theme didn't follow a light background")
	}
}

func TestNotifyOnlyWhenBlurred(t *testing.T) {
	m := newModel()
	if m.notifyDone("done") != nil {
		t.Error("notified while focused")
	}
	send(m, tea.BlurMsg{})
	msgs := flatten(m.notifyDone("Import done\x1b]0;x\x07"))
	if len(msgs) != 1 || msgs[0].(tea.RawMsg).Msg != "\x1b]9;012: Import done]0;x\x07" {
		t.Errorf("notification %q", msgs)
	}
	send(m, tea.FocusMsg{})
	if m.notifyDone("done") != nil {
		t.Error("notified after focus came back")
	}
	if !m.View().ReportFocus {
		t.Error("focus reporting off")
	}
}
