package ui

import (
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Format > Alignment's Top, Middle and Bottom put a value on the first,
// middle or last line of a tall row, and wrapped text starts there.
func TestVerticalAlignment(t *testing.T) {
	m := newModel()
	press(t, m, "x", "<tab>", "one two three", "<enter>")
	m.sheet.SetRowHeight(0, 0, 5)
	m.cur = addr("A1")
	lineOf := func(text string) int {
		for i, l := range gridLines(m)[:5] {
			if strings.Contains(l, text) {
				return i
			}
		}
		return -1
	}
	if got := lineOf("x"); got != 4 {
		t.Errorf("automatic: x on line %d", got)
	}
	for id, want := range map[string]int{"format.valign_top": 0, "format.valign_middle": 2, "format.valign_bottom": 4} {
		run(m, m.runCommand(id))
		if got := lineOf("x"); got != want || !commands[id].checked(m) {
			t.Errorf("%s: x on line %d, want %d", id, got, want)
		}
	}
	m.cur = addr("B1")
	run(m, m.runCommand("format.wrap"))
	run(m, m.runCommand("format.valign_middle"))
	if lineOf("one") != 1 || lineOf("three") != 2 {
		t.Errorf("wrapped in the middle:\n%s", strings.Join(gridLines(m)[:5], "\n"))
	}
	press(t, m, "<ctrl+z>")
	if lineOf("one") != 3 {
		t.Errorf("undo:\n%s", strings.Join(gridLines(m)[:5], "\n"))
	}
	if !menuHas(m, "Format", "Alignment") {
		t.Error("no Format > Alignment")
	}
}

// menuHas reports whether the menu titled top has an item titled title.
func menuHas(m *Model, top, title string) bool {
	for _, mn := range menuBar {
		if mn.title != top {
			continue
		}
		for _, it := range mn.items {
			if it.label() == title {
				return true
			}
		}
	}
	return false
}

// Border color picks the color the next borders draw in, which a
// recording keeps as the command's answer.
func TestBorderColor(t *testing.T) {
	m := newModel()
	press(t, m, "a", "<tab>", "b", "<enter>")
	run(m, m.runCommand("format.border_color"))
	press(t, m, "red", "<enter>")
	if m.borderColor != sheet.ColorRed || !strings.Contains(m.note, "red") {
		t.Fatalf("color %v, note %q", m.borderColor, m.note)
	}
	m.selectRect(rect("A1:B1"))
	run(m, m.runCommand("format.borders_outer"))
	if st := m.sheet.StrokeAbove(addr("A1")); st != (sheet.Stroke{Line: sheet.LineThin, Color: sheet.ColorRed}) {
		t.Errorf("top edge %+v", st)
	}
	if v := m.View().Content; !strings.Contains(v, m.th.RuleText[sheet.ColorRed].Render("│")) {
		t.Error("the lines aren't drawn red")
	}
	m = newModel()
	script(t, m, `run("format.border_color", answer="Blue")
run("format.borders_all")`)
	if st := m.sheet.StrokeLeft(addr("A1")); st.Color != sheet.ColorBlue {
		t.Errorf("script's border %+v, %q", st, m.warn)
	}
}

// A double line meeting a thick outline ends in a thick tee whose arm
// runs on into the double line, as Unicode has no joint of the two.
func TestDoubleMeetsThick(t *testing.T) {
	m := newModel()
	press(t, m, "Day", "<tab>", "Cost", "<enter>", "Fri", "<tab>", "12", "<enter>")
	m.selectRect(rect("A1:B2"))
	run(m, m.runCommand("format.borders_all"))
	run(m, m.runCommand("format.border_thick"))
	run(m, m.runCommand("format.borders_outer"))
	m.selectRect(rect("A1:B1"))
	run(m, m.runCommand("format.border_double"))
	run(m, m.runCommand("format.border_bottom"))
	g := gridLines(m)
	if !strings.Contains(g[2], "┣━════════╪════════━┫") || strings.ContainsAny(g[2], "╠╣") {
		t.Errorf("joint under the headers: %q", g[2])
	}
}

// Typing in a merged cell shows the entry as wide as the merge, and
// stepping off it goes on along the row or column it was entered by.
func TestMergeEntryAndMemory(t *testing.T) {
	m := newModel()
	m.selectRect(rect("B1:C3"))
	run(m, m.runCommand("format.merge_all"))
	m.clearSelection()
	m.cur = addr("B1")
	press(t, m, "a title wider than one column")
	if !strings.Contains(screen(m), "a title wider than one c") {
		t.Errorf("entry not as wide as the merge:\n%s", screen(m))
	}
	press(t, m, "<esc>")
	m.cur = addr("A2")
	press(t, m, "<right>")
	if m.cur != addr("B1") {
		t.Fatalf("into the merge: %v", m.cur)
	}
	press(t, m, "<right>")
	if m.cur != addr("D2") {
		t.Errorf("out of the merge along row 2: %v", m.cur)
	}
	m.cur = addr("C5")
	press(t, m, "<up>", "<up>", "<down>")
	if m.cur != addr("C4") {
		t.Errorf("down out of the merge along column C: %v", m.cur)
	}
	// Freezing through a merge is refused.
	m.cur = addr("B2")
	run(m, m.runCommand("view.freeze_rows2"))
	if m.mode != modeError || !strings.Contains(m.errMsg, "B1:C3") {
		t.Errorf("froze through a merge: %v %q", m.mode, m.errMsg)
	}
	if fr, _ := m.sheet.Frozen(); fr != 0 {
		t.Errorf("froze %d rows", fr)
	}
}

// A merged cell's value sits on the middle one of the lines its rows
// take, rule lines between them included, whatever their heights; Top
// and Bottom put it on the first or last.
func TestMergeCenteredByLines(t *testing.T) {
	m := newModel()
	press(t, m, "mid", "<enter>")
	m.sheet.SetRowHeight(0, 0, 5)
	m.selectRect(rect("A1:A3"))
	run(m, m.runCommand("format.merge_vertical"))
	m.View()
	if row, k := m.mergeText(rect("A1:A3")); row != 0 || k != 3 {
		t.Errorf("middle of 7 lines at row %d line %d", row, k)
	}
	m.cur = addr("A1")
	m.clearSelection()
	run(m, m.runCommand("format.valign_bottom"))
	m.View()
	if row, k := m.mergeText(rect("A1:A3")); row != 2 || k != 0 {
		t.Errorf("bottom at row %d line %d", row, k)
	}
	if !strings.Contains(gridLines(m)[6], "mid") {
		t.Errorf("drawn:\n%s", strings.Join(gridLines(m)[:8], "\n"))
	}
}
