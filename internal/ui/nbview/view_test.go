package nbview

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// fakeHost is a notebook of cells and outputs; commands run are noted.
type fakeHost struct {
	th     theme.Theme
	cells  []notebook.Cell
	outs   map[int]*notebook.Output
	states map[int]State
	ran    []string
	edits  []string
	onRun  func(id string)
	kernel Kernel
	grids  []*fakeGrid
}

func newHost(srcs ...string) *fakeHost {
	h := &fakeHost{th: theme.New(true), outs: map[int]*notebook.Output{}, states: map[int]State{}}
	for i, s := range srcs {
		h.cells = append(h.cells, notebook.Cell{ID: i + 1, Source: s})
	}
	return h
}

func (h *fakeHost) Theme() *theme.Theme            { return &h.th }
func (h *fakeHost) Locale() *locale.Locale         { return locale.Canonical }
func (h *fakeHost) Cells() []notebook.Cell         { return h.cells }
func (h *fakeHost) Output(id int) *notebook.Output { return h.outs[id] }
func (h *fakeHost) State(id int) State             { return h.states[id] }
func (h *fakeHost) Kernel() Kernel                 { return h.kernel }
func (h *fakeHost) Run(id string) tea.Cmd {
	h.ran = append(h.ran, id)
	if h.onRun != nil {
		h.onRun(id)
	}
	return nil
}
func (h *fakeHost) Edit(id int, src string) {
	h.edits = append(h.edits, src)
	for i := range h.cells {
		if h.cells[i].ID == id {
			h.cells[i].Source = src
		}
	}
}

// Grid is a fake grid: its column names, then its rows' values, spaced.
func (h *fakeHost) Grid(id int, data []byte, rows int) Grid {
	g := &fakeGrid{}
	g.Append(data, rows, data)
	h.grids = append(h.grids, g)
	return g
}

// Append adds the table's rows.
func (g *fakeGrid) Append(more []byte, _ int, _ []byte) {
	v, err := nuon.Parse(more)
	if err != nil {
		panic(err)
	}
	for _, rec := range v.List {
		var row []string
		for i, f := range rec.Fields {
			if len(g.rows) == 0 && i >= len(g.cols) {
				g.cols = append(g.cols, f.Key)
			}
			row = append(row, cellText(fileio.NUONCell(f.Value), locale.Canonical))
		}
		g.rows = append(g.rows, strings.Join(row, "  "))
	}
}

type fakeGrid struct {
	cols, rows []string
	top, win   int
	in         bool
}

func (g *fakeGrid) Rows() int     { return len(g.rows) }
func (g *fakeGrid) Top() int      { return g.top }
func (g *fakeGrid) Entered() bool { return g.in }
func (g *fakeGrid) Hidden(int) int {
	return 0
}

func (g *fakeGrid) Scroll(d int) bool {
	to := min(max(g.top+d, 0), max(len(g.rows)-g.win, 0))
	moved := to != g.top
	g.top = to
	return moved
}

func (g *fakeGrid) Line(i, from, rows, width int) string {
	g.win = rows
	if i == 0 {
		return strings.Join(g.cols, "  ")
	}
	if from+i-1 < len(g.rows) {
		return fmt.Sprintf("%d  %s", from+i, g.rows[from+i-1])
	}
	return ""
}

func newView(h *fakeHost, w, ht int) *View {
	v := New(h)
	v.Keys = map[string]string{"enter": "nb.edit", "d d": "nb.delete", "o": "nb.toggle_output", "shift+enter": "nb.run_next"}
	v.EditKeys = map[string]string{"esc": "nb.command_mode", "shift+enter": "nb.run_next"}
	v.Providers = Providers{Highlighter: Tokens{}}
	v.Resize(w, ht)
	return v
}

func text(v *View) string { return ansi.Strip(strings.Join(v.Lines(), "\n")) }

func key(s string) tea.KeyPressMsg {
	named := map[string]tea.Key{"enter": {Code: tea.KeyEnter}, "esc": {Code: tea.KeyEscape}, "up": {Code: tea.KeyUp},
		"down": {Code: tea.KeyDown}, "tab": {Code: tea.KeyTab}, "home": {Code: tea.KeyHome}, "end": {Code: tea.KeyEnd}}
	if k, ok := named[s]; ok {
		return tea.KeyPressMsg(k)
	}
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func TestOutputsShow(t *testing.T) {
	h := newHost("files = ls", "{a: 1}", `"x"`, "nope", "big", "[1 2]")
	h.outs[1] = &notebook.Output{NUON: []byte("[[name, size, ok]; [a.txt, 2kb, true], [bb.txt, 10b, false]]"), Count: 1}
	h.outs[2] = &notebook.Output{NUON: []byte("{a: 1, long_key: \"two words\"}"), Count: 2}
	h.outs[3] = &notebook.Output{NUON: []byte(`"line one\nline two"`), Count: 3}
	h.outs[4] = &notebook.Output{Err: "Command `nope` not found", Detail: "help: try ls", Count: 4}
	h.outs[5] = &notebook.Output{Unsaved: true}
	h.outs[6] = &notebook.Output{NUON: []byte("[1, 2]"), Count: 5}
	v := newView(h, 80, 60)
	got := text(v)
	for _, want := range []string{
		"[1]:", "─ files ─", "name  size  ok", "1  a.txt  2.0 kB  TRUE", "2  bb.txt  10 B  FALSE",
		"field  value", "1  a  1", "2  long_key  two words", "line one", "line two",
		"× Command `nope` not found", "help: try ls", "not saved; run to see", "0  1", "1  2",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("lacks %q:\n%s", want, got)
		}
	}
}

func TestLongOutputWindow(t *testing.T) {
	h := newHost("ls")
	var b strings.Builder
	b.WriteString("[[n]; ")
	for i := range 10000 {
		fmt.Fprintf(&b, "[%d], ", i)
	}
	b.WriteString("[x]]")
	h.outs[1] = &notebook.Output{NUON: []byte(b.String()), Count: 1}
	v := newView(h, 60, 30)
	if got := text(v); !strings.Contains(got, "9,991 more rows") || strings.Contains(got, "  10\n") {
		t.Errorf("collapsed:\n%s", got)
	}
	v.ToggleWhole()
	v.Wheel(-1, 100)
	if got := text(v); strings.Contains(got, "more rows") || !strings.Contains(got, " 98\n") {
		t.Errorf("expanded, scrolled:\n%s", got)
	}
}

func TestMovesBetweenCellsAndOutputs(t *testing.T) {
	h := newHost("a", "b", "c")
	h.outs[2] = &notebook.Output{NUON: []byte("1"), Count: 1}
	v := newView(h, 80, 30)
	steps := []struct {
		key string
		sel int
		out bool
	}{{"down", 1, false}, {"j", 1, true}, {"down", 2, false}, {"up", 1, true}, {"k", 1, false}, {"end", 2, false}, {"home", 0, false}}
	for _, s := range steps {
		v.Key(key(s.key))
		if i, out := v.Selected(); i != s.sel || out != s.out {
			t.Fatalf("after %s: %d %v, want %d %v", s.key, i, out, s.sel, s.out)
		}
	}
	// A pair: d d runs Delete; d then another key doesn't.
	v.Key(key("d"))
	if v.Pending() != "d" || len(h.ran) != 0 {
		t.Fatalf("d: pending %q ran %v", v.Pending(), h.ran)
	}
	v.Key(key("d"))
	if len(h.ran) != 1 || h.ran[0] != "nb.delete" {
		t.Errorf("d d ran %v", h.ran)
	}
	if _, ok := v.Key(key("q")); ok {
		t.Error("a key the notebook doesn't know was taken")
	}
}

func TestEditsWithWrapAndCompletes(t *testing.T) {
	h := newHost("ls")
	v := newView(h, 38, 20)
	v.Providers.Completer = Words(func() []Word { return []Word{{Text: "$files", Desc: "cell"}, {Text: "sort-by", Desc: "command"}} })
	h.onRun = func(id string) {
		if id == "nb.command_mode" {
			v.StopEdit()
		}
	}
	v.StartEdit()
	if !v.Editing() {
		t.Fatal("not editing")
	}
	for _, r := range " | where size > 1kb | so" {
		v.Key(key(string(r)))
	}
	msg := v.askCompletions()()
	v.Update(msg)
	if got := v.edit.text(); got != "ls | where size > 1kb | sort-by" {
		t.Errorf("completed %q", got)
	}
	x, y, ok := v.Cursor()
	if !ok || y != 2 || x != textX+2+len("| sort-by") {
		t.Errorf("caret %d,%d %v:\n%s", x, y, ok, text(v))
	}
	lines := strings.Split(text(v), "\n")
	if !strings.Contains(lines[1], "[ ]: ┃ ls | where size > 1kb  ┃ ▶") || !strings.Contains(lines[2], "┃   | sort-by") {
		t.Errorf("wrapped:\n%s", text(v))
	}
	v.Key(key("esc"))
	if v.Editing() || h.cells[0].Source != "ls | where size > 1kb | sort-by" {
		t.Errorf("after Esc: editing %v, source %q", v.Editing(), h.cells[0].Source)
	}
}

func TestHighlights(t *testing.T) {
	spans := Tokens{}.Highlight(context.Background(), `x = ls | where size > 1kb and name =~ "a" # big`)
	var got []string
	src := `x = ls | where size > 1kb and name =~ "a" # big`
	for _, s := range spans {
		got = append(got, fmt.Sprintf("%s:%d", src[s.From:s.To], s.Kind))
	}
	want := fmt.Sprint([]string{"x:2", "=:5", "ls:0", "|:5", "where:0", ">:5", "1kb:3", "and:5", "=~:5", `"a":1`, "# big:6"})
	if fmt.Sprint(got) != want {
		t.Errorf("spans %v\nwant %v", got, want)
	}
}

func TestMarkdown(t *testing.T) {
	th := theme.New(true)
	lines := markdown(&th, "# Title\nSome **bold** and *it* with `code` and [a link](https://x.y).\n- one\n1. first\n> quoted", 40)
	got := ansi.Strip(strings.Join(lines, "\n"))
	want := "Title\nSome bold and it with code and a link.\n• one\n1. first\n│ quoted"
	if got != want {
		t.Errorf("markdown:\n%s\nwant\n%s", got, want)
	}
	if !strings.Contains(strings.Join(lines, ""), "https://x.y") {
		t.Error("the link isn't a hyperlink")
	}
}

// A table's output is the host's grid: its window, then full-screen at
// the body's size; text full-screen scrolls its lines.
func TestGridOutputs(t *testing.T) {
	h := newHost("ls", "text")
	h.outs[1] = &notebook.Output{NUON: []byte("[[name, n]; [b, 2], [a, 10], [c, 1]]"), Count: 1}
	h.outs[2] = &notebook.Output{NUON: []byte(`"one\ntwo"`), Count: 2}
	v := newView(h, 60, 12)
	if len(h.grids) != 1 || !strings.Contains(text(v), "name  n\n") || !strings.Contains(text(v), "1  b  2") {
		t.Fatalf("the grid's window:\n%s", text(v))
	}
	v.Select(0, true)
	if v.SelectedGrid() != Grid(h.grids[0]) || !v.Shows(h.grids[0]) {
		t.Error("the output selected isn't its grid")
	}
	if g, gx, gy, ok := v.GridAt(textX+3, 4); !ok || g != Grid(h.grids[0]) || gx != 3 || gy != 1 {
		t.Errorf("GridAt: %v %d %d %v", g, gx, gy, ok)
	}
	if x, y, ok := v.GridOrigin(h.grids[0]); !ok || x != textX || y != 3 {
		t.Errorf("the grid starts at %d, %d", x, y)
	}
	h.grids[0].in = true
	if bar := v.Lines()[4]; !strings.Contains(bar, "▌") {
		t.Errorf("the grid entered has no bar: %q", bar)
	}
	v.OpenFull()
	if got := text(v); h.grids[0].win != 11 || !strings.HasPrefix(got, "name  n\n1  b  2") {
		t.Errorf("full-screen, %d rows:\n%s", h.grids[0].win, got)
	}
	v.Key(key("esc"))
	v.Select(1, true)
	v.OpenFull()
	v.Key(key("down"))
	if got := text(v); !strings.Contains(got, " one\n▌two") {
		t.Errorf("text full-screen:\n%s", got)
	}
}
