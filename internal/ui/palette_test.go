package ui

import (
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func openPicker(t *testing.T, m *Model) *picker {
	t.Helper()
	p, ok := m.overlay.(*picker)
	if !ok {
		t.Fatalf("no picker open (mode %v, overlay %T)", m.mode, m.overlay)
	}
	return p
}

func selected(t *testing.T, m *Model) string {
	t.Helper()
	p := openPicker(t, m)
	if p.sel >= len(p.shown) {
		return ""
	}
	return p.shown[p.sel].item.title
}

func TestPaletteKeys(t *testing.T) {
	for _, key := range []string{"<ctrl+k>", "<alt+/>", "<ctrl+shift+p>"} {
		m := newModel()
		press(t, m, key)
		if p := openPicker(t, m); len(p.shown) != len(p.items) || !strings.HasSuffix(line(m, menuLine), "MENU") {
			t.Errorf("%s: %d of %d shown", key, len(p.shown), len(p.items))
		}
	}
}

func TestPaletteSearchRunsCommand(t *testing.T) {
	m := newModel()
	press(t, m, "<ctrl+k>", "colwid")
	p := openPicker(t, m)
	if got := selected(t, m); got != "Column width" {
		t.Fatalf("best match %q", got)
	}
	if len(p.shown[0].inTitle) != len("colwid") {
		t.Errorf("matched %v", p.shown[0].inTitle)
	}
	if !strings.Contains(screen(m), "Column width") || !strings.Contains(screen(m), "Format") {
		t.Errorf("result should show its menu path:\n%s", screen(m))
	}
	x, y, _ := m.cursorPos()
	if b := p.layout(m)[0]; y != b.y+1 || x != b.x+1+3+len("colwid") {
		t.Errorf("cursor at %d,%d, box at %d,%d", x, y, b.x, b.y)
	}
	press(t, m, "<enter>")
	if m.overlay != nil || m.mode != modePrompt || m.prompt.kind != promptWidth {
		t.Errorf("Enter: mode %v overlay %T", m.mode, m.overlay)
	}
}

func TestPaletteMatchesMenuPath(t *testing.T) {
	m := newModel()
	// "file" matches every File menu command through its path.
	press(t, m, "<ctrl+k>", "file")
	var got []string
	for _, pm := range openPicker(t, m).shown {
		got = append(got, pm.item.title)
	}
	slices.Sort(got[:5])
	if s := strings.Join(got[:5], ","); s != "New,Open,Quit,Save,Save as" {
		t.Errorf("file matched %s", strings.Join(got, ","))
	}
	press(t, m, "<down>", "<down>")
	if selected(t, m) != "Save" || !strings.Contains(line(m, m.height-1), "Save the sheet") {
		t.Errorf("selected %q, status %q", selected(t, m), line(m, m.height-1))
	}
	// Up from the top wraps to the bottom.
	press(t, m, "<up>", "<up>", "<up>")
	if p := openPicker(t, m); p.sel != len(p.shown)-1 {
		t.Errorf("wrapped to %d of %d", p.sel, len(p.shown))
	}
}

func TestPaletteNoMatchAndEsc(t *testing.T) {
	m := newModel()
	press(t, m, "<ctrl+k>", "zzqx")
	if !strings.Contains(screen(m), "No matches") || !strings.Contains(screen(m), "0 of ") {
		t.Errorf("no-match screen:\n%s", screen(m))
	}
	press(t, m, "<enter>")
	openPicker(t, m)
	press(t, m, "<backspace>", "<backspace>", "<backspace>", "<backspace>")
	if p := openPicker(t, m); len(p.shown) != len(p.items) {
		t.Error("clearing the search did not show everything")
	}
	send(m, tea.PasteMsg{Content: "goto"})
	if selected(t, m) != "Go to" {
		t.Errorf("paste searched for %q", selected(t, m))
	}
	press(t, m, "<esc>")
	if m.overlay != nil || m.mode != modeReady || len(m.buf) != 0 {
		t.Errorf("Esc: mode %v buf %q", m.mode, string(m.buf))
	}
}

func TestPaletteUnavailableCommand(t *testing.T) {
	runs := fakeCommand(t, "edit.undo", "Undo", func(*Model) bool { return false })
	m := newModel()
	press(t, m, "<ctrl+k>", "undo")
	if selected(t, m) != "Undo" {
		t.Fatalf("selected %q", selected(t, m))
	}
	press(t, m, "<enter>")
	if *runs != 0 || m.overlay == nil {
		t.Error("an unavailable command ran")
	}
	if !strings.Contains(line(m, m.height-1), "not available") {
		t.Errorf("status %q", line(m, m.height-1))
	}
}

func TestPaletteMouse(t *testing.T) {
	m := newModel()
	press(t, m, "<ctrl+k>", "go")
	p := openPicker(t, m)
	b := p.layout(m)[0]
	var row int
	for i, pm := range p.shown {
		if pm.item.title == "Go to" {
			row = i
		}
	}
	mouseAt(m, tea.MouseMotionMsg{X: b.x + 4, Y: b.y + pickerFirstRow + row})
	if selected(t, m) != "Go to" {
		t.Fatalf("hover selected %q", selected(t, m))
	}
	leftClick(m, b.x+4, b.y+pickerFirstRow+row)
	if m.mode != modePrompt || m.prompt.label != "Go to:" {
		t.Errorf("click ran: mode %v", m.mode)
	}
	press(t, m, "<esc>", "<ctrl+k>")
	leftClick(m, 1, m.height-2)
	if m.overlay != nil {
		t.Error("click outside did not close the palette")
	}
}
