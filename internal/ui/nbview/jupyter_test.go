package nbview

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// at is the body line holding text and the column it starts at.
func at(t *testing.T, v *View, want string) (x, y int) {
	t.Helper()
	for y, l := range v.Lines() {
		if i := strings.Index(ansi.Strip(l), want); i >= 0 {
			return ansi.StringWidth(ansi.Strip(l)[:i]), y
		}
	}
	t.Fatalf("no line holds %q:\n%s", want, text(v))
	return 0, 0
}

func TestToolbarFitsAndClicks(t *testing.T) {
	h := newHost("ls")
	v := newView(h, 120, 20)
	wide := ansi.Strip(v.Toolbar(120))
	for _, want := range []string{"▶ Run", "Shift+Enter", "■ Stop", "▶▶ Run all", "+ Add", "✂ Cut", "Code ▾", "nu ○ idle"} {
		if !strings.Contains(wide, want) {
			t.Errorf("at 120 the toolbar lacks %q: %q", want, wide)
		}
	}
	narrow := ansi.Strip(v.Toolbar(60))
	if ansi.StringWidth(narrow) > 60 || !strings.Contains(narrow, "▶ Run") || !strings.Contains(narrow, "nu ○ idle") || strings.Contains(narrow, "Shift+Enter") {
		t.Errorf("at 60: %q", narrow)
	}
	x := strings.Index(wide, "▶ Run")
	if got := v.ToolbarAt(x+2, 120); got != "nb.run_next" {
		t.Errorf("Run: %q", got)
	}
	stop := ansi.StringWidth(wide[:strings.Index(wide, "■ Stop")])
	if got := v.ToolbarAt(stop, 120); got != "" {
		t.Errorf("Stop with nothing running: %q", got)
	}
	h.kernel = Kernel{Busy: true, Waiting: 2}
	if got := v.ToolbarAt(stop, 120); got != "nb.stop" || !strings.Contains(ansi.Strip(v.Toolbar(120)), "nu ● busy, 2 waiting") {
		t.Errorf("Stop while busy: %q", got)
	}
	h.cells[0].Kind = notebook.Note
	if !strings.Contains(ansi.Strip(v.Toolbar(120)), "Markdown ▾") {
		t.Error("the kind isn't the cell's")
	}
}

func TestClicksSelectEditRunAndFold(t *testing.T) {
	h := newHost("files = ls", "$files | first", "# Title")
	h.cells[2].Kind = notebook.Note
	h.outs[1] = &notebook.Output{NUON: []byte("[[name]; [a], [b]]"), Count: 1}
	v := newView(h, 80, 40)
	x, y := at(t, v, "first")
	v.Click(x+2, y, false) // into the second cell's code
	if i, _ := v.Selected(); i != 1 || !v.Editing() {
		t.Fatalf("a click in the box: cell %d, editing %v", i, v.Editing())
	}
	if got := v.edit.area.Pos; got != len([]rune("$files | fi")) {
		t.Errorf("the caret went to %d", got)
	}
	_, y = at(t, v, "files = ls")
	v.Click(v.width-1, y, false) // ▶
	if v.Editing() || !slices.Contains(h.ran, "nb.run") {
		t.Errorf("▶: editing %v, ran %v", v.Editing(), h.ran)
	}
	_, y = at(t, v, "Out[1]:")
	v.Click(2, y, false) // left of the output
	if !v.Hidden(1) || !strings.Contains(text(v), "output hidden") {
		t.Errorf("folding:\n%s", text(v))
	}
	v.Click(2, y, false)
	if v.Hidden(1) {
		t.Error("unfolding")
	}
	_, y = at(t, v, "Title")
	v.Click(20, y, true)
	if from, to := v.Range(); from != 0 || to != 2 {
		t.Errorf("Shift+click: %d to %d", from, to)
	}
	v.lastTap = tap{} // a while later
	v.Click(20, y, false)
	v.Click(20, y, false)
	if i, _ := v.Selected(); i != 2 || !v.Editing() || !strings.Contains(text(v), "# Title") {
		t.Errorf("a double click on a note edits its Markdown:\n%s", text(v))
	}
}

func TestOutputScrollsInItsWindow(t *testing.T) {
	h := newHost("ls", "after")
	var b strings.Builder
	b.WriteString("[[n]; ")
	for i := range 45 {
		fmt.Fprintf(&b, "[%d], ", i)
	}
	b.WriteString("[x]]")
	h.outs[1] = &notebook.Output{NUON: []byte(b.String()), Count: 1}
	v := newView(h, 80, 40)
	_, afterY := at(t, v, "after")
	if !strings.Contains(text(v), "36 more rows") {
		t.Fatalf("the window:\n%s", text(v))
	}
	v.Key(key("down")) // onto the output
	v.Key(key("down")) // scrolls it
	v.Key(key("down"))
	if got := text(v); !strings.Contains(got, "rows 3 to 12 of 46") || !strings.Contains(got, "  11\n") && !strings.Contains(got, " 11 ") {
		t.Errorf("scrolled by keys:\n%s", got)
	}
	if _, y := at(t, v, "after"); y != afterY {
		t.Errorf("the cell under moved from %d to %d", afterY, y)
	}
	_, y := at(t, v, "rows 3 to 12")
	v.Wheel(y-2, 3)
	if !strings.Contains(text(v), "rows 6 to 15 of 46") {
		t.Errorf("scrolled by the wheel:\n%s", text(v))
	}
	v.Key(key("O"))
	v.ToggleWhole()
	if strings.Contains(text(v), "more rows") || strings.Contains(text(v), "rows 6") {
		t.Errorf("whole:\n%s", text(v))
	}
	v.ToggleHidden()
	if !strings.Contains(text(v), "output hidden") {
		t.Errorf("hidden:\n%s", text(v))
	}
}

func TestSelectsSeveralCells(t *testing.T) {
	h := newHost("a", "b", "c", "d")
	v := newView(h, 80, 40)
	v.Key(key("down"))
	v.Key(key("J"))
	v.Key(key("J"))
	if from, to := v.Range(); from != 1 || to != 3 {
		t.Errorf("J J: %d to %d", from, to)
	}
	v.Key(key("K"))
	if from, to := v.Range(); from != 1 || to != 2 {
		t.Errorf("K: %d to %d", from, to)
	}
	v.Key(key("up"))
	if from, to := v.Range(); from != 1 || to != 1 {
		t.Errorf("up selects one: %d to %d", from, to)
	}
}

func TestOutline(t *testing.T) {
	h := newHost("# Intro\ntext\n## Part", "files = ls", "no heading")
	h.cells[0].Kind = notebook.Note
	h.cells[2].Kind = notebook.Note
	v := newView(h, 80, 40)
	var got []string
	for _, e := range v.Outline(false) {
		got = append(got, fmt.Sprintf("%d %d %s", e.Cell, e.Level, e.Title))
	}
	if !slices.Equal(got, []string{"0 1 Intro", "0 2 Part"}) {
		t.Errorf("contents %q", got)
	}
	got = nil
	for _, e := range v.Outline(true) {
		got = append(got, fmt.Sprintf("%d %d %s", e.Cell, e.Level, e.Title))
	}
	if !slices.Equal(got, []string{"0 1 Intro", "0 2 Part", "1 0 files = ls", "2 0 no heading"}) {
		t.Errorf("cells %q", got)
	}
}
