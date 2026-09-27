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
	if p.Sel >= len(p.shown) {
		return ""
	}
	return p.shown[p.Sel].item.title
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
	if b := p.Layout()[0]; y != b.Y+1 || x != b.X+1+3+len("colwid") {
		t.Errorf("cursor at %d,%d, box at %d,%d", x, y, b.X, b.Y)
	}
	press(t, m, "<enter>")
	if m.overlay != nil || m.mode != modePrompt || m.prompt.kind != promptWidth {
		t.Errorf("Enter: mode %v overlay %T", m.mode, m.overlay)
	}
}

func TestPaletteMatchesMenuPath(t *testing.T) {
	m := newModel()
	// "file" matches "Open config file" by its title first, then every
	// File menu command through its path.
	press(t, m, "<ctrl+k>", "file")
	var got []string
	for _, pm := range openPicker(t, m).shown {
		got = append(got, pm.item.title)
	}
	if got[0] != "Open config file" {
		t.Errorf("first match for file: %q", got[0])
	}
	slices.Sort(got[1:7])
	if s := strings.Join(got[1:7], ","); s != "Import,New,Open,Quit,Save,Save as" {
		t.Errorf("file matched %s", strings.Join(got, ","))
	}
	press(t, m, "<down>", "<down>", "<down>")
	if selected(t, m) != "Save" || !strings.Contains(line(m, m.height-1), "Save the sheet") {
		t.Errorf("selected %q, status %q", selected(t, m), line(m, m.height-1))
	}
	// Up from the top wraps to the bottom.
	press(t, m, "<up>", "<up>", "<up>", "<up>")
	if p := openPicker(t, m); p.Sel != len(p.shown)-1 {
		t.Errorf("wrapped to %d of %d", p.Sel, len(p.shown))
	}
}

func TestPaletteTitleMatchesFirst(t *testing.T) {
	m := newModel()
	// "sel" also matches "Save" through the l of its File path, but
	// titles that match on their own rank first.
	press(t, m, "<ctrl+k>", "sel")
	shown := openPicker(t, m).shown
	seenPathOnly := false
	for _, pm := range shown {
		inTitle := strings.Contains(strings.ToLower(pm.item.title), "sel")
		// Titles matching letter by letter ("Move sheet left") are
		// title matches too, just not literal ones.
		pathOnly := len(pm.inTitle) < len("sel")
		if pathOnly && pm.item.title != "Select all" {
			seenPathOnly = true
		} else if seenPathOnly && inTitle {
			t.Errorf("%q ranked after a path-only match", pm.item.title)
		}
	}
}

func TestPaletteWordPrefixFirst(t *testing.T) {
	m := newModel()
	// "col" starts a word in the column commands; "Command line" only
	// spells it across two words, so it ranks after them.
	press(t, m, "<ctrl+k>", "col")
	shown := openPicker(t, m).shown
	seenOther := false
	for _, pm := range shown {
		prefix := wordPrefix(pm.item.title[:pm.item.name], "col")
		if !prefix {
			seenOther = true
		} else if seenOther {
			t.Errorf("%q, a word-prefix match, ranked after other matches", pm.item.title)
		}
	}
	if len(shown) == 0 || !wordPrefix(shown[0].item.title, "col") {
		t.Errorf("first match for col: %+v", shown[0].item.title)
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
	if m.overlay != nil || m.mode != modeReady || len(m.line.Buf) != 0 {
		t.Errorf("Esc: mode %v buf %q", m.mode, m.line.Text())
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
	b := p.Layout()[0]
	var row int
	for i, pm := range p.shown {
		if pm.item.title == "Go to" {
			row = i
		}
	}
	mouseAt(m, tea.MouseMotionMsg{X: b.X + 4, Y: b.Y + pickerFirstRow + row})
	if selected(t, m) != "Go to" {
		t.Fatalf("hover selected %q", selected(t, m))
	}
	leftClick(m, b.X+4, b.Y+pickerFirstRow+row)
	if m.mode != modePrompt || m.prompt.label != "Go to:" {
		t.Errorf("click ran: mode %v", m.mode)
	}
	press(t, m, "<esc>", "<ctrl+k>")
	leftClick(m, 1, m.height-2)
	if m.overlay != nil {
		t.Error("click outside did not close the palette")
	}
}
