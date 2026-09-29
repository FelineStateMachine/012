package ui

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// fakeNu answers pipelines from a table of outputs, failing those it
// lacks, and keeps the jobs it ran.
type fakeNu struct {
	out  map[string]string
	jobs []nushell.Job
}

func (f *fakeNu) Run(_ context.Context, job nushell.Job, _ string, stdout io.Writer) error {
	if len(job.IDE) > 0 {
		return nushell.ErrMissing // the code editor's questions: the built-ins answer
	}
	if job.Command == "help commands | get name" {
		_, err := io.WriteString(stdout, `["ls", "where", "sort-by", "str join"]`)
		return err
	}
	f.jobs = append(f.jobs, job)
	out, ok := f.out[job.Command]
	if !ok {
		return &nushell.Error{Msg: "Command `" + job.Command + "` not found", Stderr: "  × not found\n  help: check the spelling\n"}
	}
	_, err := io.WriteString(stdout, out)
	return err
}

// ran is the pipelines the fake ran, in order.
func (f *fakeNu) ran() []string {
	var out []string
	for _, j := range f.jobs {
		out = append(out, j.Command)
	}
	return out
}

const lsOut = "[[name, size]; [a.txt, 2kb], [b.txt, 10b], [c.txt, 5kb]]"

// notebookModel is a model on a new notebook, cells running with a fake
// nu answering from out.
func notebookModel(t *testing.T, out map[string]string) (*Model, *fakeNu) {
	t.Helper()
	m := newModel()
	nu := &fakeNu{out: out}
	m.SetShellRunner(nu)
	m.SetMachine("here")
	m.nb.still = true
	run(m, m.runCommand("nb.open"))
	if !m.sheet.IsNotebook() {
		t.Fatal("Open notebook didn't show a notebook")
	}
	return m, nu
}

// write adds a cell below the selected one holding src, leaving edit
// mode.
func write(t *testing.T, m *Model, src string) {
	t.Helper()
	press(t, m, "b", "<enter>")
	send(m, pasteMsg(src))
	press(t, m, "<esc>")
}

func pasteMsg(s string) tea.PasteMsg { return tea.PasteMsg{Content: s} }

func cells(m *Model) []string {
	var out []string
	for _, c := range m.sheet.NotebookCells() {
		out = append(out, c.Source)
	}
	return out
}

func TestNotebookWritesAndRuns(t *testing.T) {
	m, nu := notebookModel(t, map[string]string{"ls": lsOut})
	if got := line(m, menuLine); !strings.Contains(got, "NOTEBOOK") {
		t.Errorf("indicator: %q", got)
	}
	press(t, m, "b", "<enter>")
	if got := line(m, menuLine); !strings.HasSuffix(got, " EDIT") {
		t.Errorf("editing: %q", got)
	}
	press(t, m, "files = ls", "<shift+enter>")
	if got := cells(m); len(got) != 2 || got[0] != "files = ls" || got[1] != "" {
		t.Fatalf("cells %q", got)
	}
	if len(nu.jobs) != 1 {
		t.Fatalf("ran %v", nu.ran())
	}
	s := screen(m)
	for _, want := range []string{"[1] files", "a.txt", "2.0 kB", "c.txt"} {
		if !strings.Contains(s, want) {
			t.Errorf("screen lacks %q:\n%s", want, s)
		}
	}
	// The new cell is selected, and a run count shows on the first.
	if i, _ := m.nbView().Selected(); i != 1 {
		t.Errorf("selected %d after Shift+Enter", i)
	}
	// Grid commands don't act on a notebook tab.
	run(m, m.runCommand("clear"))
	if !strings.Contains(m.note, "notebook") {
		t.Errorf("clear on a notebook: %q", m.note)
	}
}

func TestNotebookReadsNames(t *testing.T) {
	m, nu := notebookModel(t, map[string]string{"ls": lsOut, "$files | where size > 1kb": "[[name]; [a.txt]]"})
	write(t, m, "files = ls")
	write(t, m, "$files | where size > 1kb")
	// Running the second runs the first, which it reads and which hasn't
	// run, first.
	press(t, m, "<ctrl+enter>")
	if got := nu.ran(); len(got) != 2 || got[0] != "ls" {
		t.Fatalf("ran %q", got)
	}
	if got := string(nu.jobs[1].Tables["files"]); got != lsOut {
		t.Errorf("$files reached nu as %q", got)
	}
	// Running files again makes the cell reading it stale; a reactive
	// notebook runs it too.
	press(t, m, "<up>", "<up>", "<up>")
	if i, _ := m.nbView().Selected(); i != 0 {
		t.Fatalf("selected %d", i)
	}
	cs := m.sheet.NotebookCells()
	if st := m.cellState(m.sheet, cs[1].ID); st.Stale {
		t.Error("stale before anything changed")
	}
	press(t, m, "<ctrl+enter>")
	if st := m.cellState(m.sheet, cs[1].ID); !st.Stale {
		t.Error("a cell reading one that ran again isn't stale")
	}
	if !strings.Contains(screen(m), "stale") {
		t.Error("stale isn't on the screen")
	}
	run(m, m.runCommand("nb.reactive"))
	press(t, m, "<ctrl+enter>")
	if got := nu.ran(); len(got) != 5 || got[4] != "$files | where size > 1kb" {
		t.Errorf("reactive ran %q", got)
	}
	if st := m.cellState(m.sheet, cs[1].ID); st.Stale {
		t.Error("stale after the reactive run")
	}
}

func TestNotebookRunOrders(t *testing.T) {
	m, nu := notebookModel(t, map[string]string{"1": "1", "2": "2", "3": "3"})
	for _, s := range []string{"1", "2", "3"} {
		write(t, m, s)
	}
	press(t, m, "<up>")
	if i, _ := m.nbView().Selected(); i != 1 {
		t.Fatalf("selected %d", i)
	}
	run(m, m.runCommand("nb.run_above"))
	run(m, m.runCommand("nb.run_below"))
	run(m, m.runCommand("nb.run_all"))
	if got := strings.Join(nu.ran(), " "); got != "1 2 3 1 2 3" {
		t.Errorf("ran %q", got)
	}
}

func TestNotebookFailureStopsTheQueue(t *testing.T) {
	m, nu := notebookModel(t, map[string]string{"ls": lsOut})
	write(t, m, "nope")
	write(t, m, "ls")
	run(m, m.runCommand("nb.run_all"))
	if got := nu.ran(); len(got) != 1 {
		t.Errorf("ran %q after a failure", got)
	}
	s := screen(m)
	if !strings.Contains(s, "× Command `nope` not found") || !strings.Contains(s, "help: check the spelling") || !strings.Contains(s, "failed") {
		t.Errorf("the error isn't shown:\n%s", s)
	}
}

func TestNotebookCellKeys(t *testing.T) {
	m, _ := notebookModel(t, nil)
	write(t, m, "a")
	write(t, m, "b")
	want := func(step string, srcs ...string) {
		t.Helper()
		if got := cells(m); strings.Join(got, ",") != strings.Join(srcs, ",") {
			t.Fatalf("after %s: %q, want %q", step, got, srcs)
		}
	}
	press(t, m, "d", "d")
	want("dd", "a")
	press(t, m, "z")
	want("z", "a", "b")
	press(t, m, "<end>", "c", "v")
	want("c v", "a", "b", "b")
	press(t, m, "m")
	if m.sheet.NotebookCells()[2].Kind != notebook.Note {
		t.Error("m didn't make a note")
	}
	press(t, m, "y")
	if m.sheet.NotebookCells()[2].Kind != notebook.Code {
		t.Error("y didn't make code")
	}
	press(t, m, "x")
	want("x", "a", "b")
	press(t, m, "a")
	want("a", "a", "", "b")
	press(t, m, "n")
	press(t, m, "<ctrl+a>", "empty", "<enter>")
	want("n", "a", "empty = ", "b")
}

func TestNotebookRestartAndToggle(t *testing.T) {
	long := "[" + strings.Repeat("{n: 1}, ", 40) + "{n: 2}]"
	m, _ := notebookModel(t, map[string]string{"long": long})
	write(t, m, "long")
	press(t, m, "<ctrl+enter>")
	if s := screen(m); !strings.Contains(s, "31 more rows") {
		t.Errorf("a long output isn't cut to its window:\n%s", s)
	}
	press(t, m, "o")
	if s := screen(m); strings.Contains(s, "more rows") {
		t.Errorf("o didn't show all of it:\n%s", s)
	}
	press(t, m, "0", "0")
	if m.book().Output(m.sheet.NotebookCells()[0].ID) != nil || m.nb.count != 0 {
		t.Error("0 0 left outputs")
	}
}

// A 200-character pipeline at 80 columns wraps: every character of it
// is on the screen, in edit mode and out of it.
func TestNotebookLongPipelineWraps(t *testing.T) {
	m, _ := notebookModel(t, nil)
	var b strings.Builder
	for i := 0; b.Len() < 200; i++ {
		b.WriteString("where col" + string(rune('a'+i%26)) + " > " + strings.Repeat("9", i%5+1) + " | ")
	}
	src := "ls | " + b.String() + "first"
	press(t, m, "b", "<enter>")
	send(m, pasteMsg(src))
	check := func(when string) {
		s := strings.ReplaceAll(screen(m), "\n", "")
		var joined strings.Builder
		for _, l := range strings.Split(screen(m), "\n") {
			if rest, ok := strings.CutPrefix(l, "│ "); ok {
				joined.WriteString(strings.TrimSpace(rest) + " ")
			}
		}
		got := strings.Join(strings.Fields(joined.String()), " ")
		if got != strings.Join(strings.Fields(src), " ") {
			t.Errorf("%s: the source on screen is\n%q\nwant\n%q\n%s", when, got, src, s)
		}
	}
	check("editing")
	press(t, m, "<esc>")
	check("in command mode")
	if _, _, ok := m.cursorPos(); ok {
		t.Error("a caret out of edit mode")
	}
}

func TestNotebookSelectionAndRanges(t *testing.T) {
	m, nu := notebookModel(t, map[string]string{"$selection | math sum": "7", "$__sheet1 | length": "2"})
	grid := m.book().Sheet(0)
	grid.Set(addr("A1"), "3")
	grid.Set(addr("A2"), "4")
	m.showSheet(grid)
	m.cur, m.ext, m.selecting = addr("A1"), addr("A2"), true
	run(m, m.runCommand("nb.open"))
	write(t, m, "$selection | math sum")
	write(t, m, "$sheet.Sheet1!A1:A2 | length")
	run(m, m.runCommand("nb.run_all"))
	if len(nu.jobs) != 2 {
		t.Fatalf("ran %q", nu.ran())
	}
	if sel := string(nu.jobs[0].Tables["selection"]); !strings.Contains(sel, "3") || !strings.Contains(sel, "4") {
		t.Errorf("$selection reached nu as %q", sel)
	}
	if r := string(nu.jobs[1].Tables["__sheet1"]); !strings.Contains(r, "4") {
		t.Errorf("$sheet.Sheet1!A1:A2 reached nu as %q", r)
	}
	if o := m.book().Output(m.sheet.NotebookCells()[0].ID); o.Selection != "Sheet1!A1:A2" {
		t.Errorf("the output read %q", o.Selection)
	}
}

func TestNotebookSendsToSheet(t *testing.T) {
	m, nu := notebookModel(t, map[string]string{"ls": lsOut})
	write(t, m, "files = ls")
	press(t, m, "<ctrl+enter>", "G", "<enter>") // the picker's first choice: a new sheet
	t2 := m.book().Lookup("files")
	if t2 == nil {
		t.Fatalf("no sheet files; note %q", m.note)
	}
	if got := t2.Value(addr("A1")).String(); got != "name" {
		t.Errorf("A1 %q", got)
	}
	t2.Set(addr("D1"), "=ROWS(nu.files)")
	if got := t2.Value(addr("D1")).Num; got != 4 {
		t.Errorf("ROWS(nu.files) = %v", got)
	}
	// Running again updates the sheet through the change stream.
	nu.out["ls"] = "[[name]; [x]]"
	press(t, m, "<ctrl+enter>")
	if got := t2.Value(addr("D1")).Num; got != 2 {
		t.Errorf("after running again ROWS = %v", got)
	}
	// Undo the region's removal: it's fed again from the output.
	m.showSheet(t2)
	m.cur = addr("A1")
	run(m, m.runCommand("region.delete"))
	run(m, m.runCommand("edit.undo"))
	send(m, tea.WindowSizeMsg{Width: 80, Height: 20})
	if got := t2.Value(addr("A2")).String(); got != "x" {
		t.Errorf("after undo A2 = %q", got)
	}
}

func TestNotebookTrust(t *testing.T) {
	m, nu := notebookModel(t, map[string]string{"ls": lsOut})
	write(t, m, "ls")
	var b bytes.Buffer
	if err := m.book().Write(&b); err != nil {
		t.Fatal(err)
	}
	text := strings.Replace(b.String(), `"macroOrigin": "here"`, `"macroOrigin": "elsewhere"`, 1)
	s, err := sheet.Read(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	m2 := New(s, "")
	m2.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m2.SetShellRunner(nu)
	m2.SetMachine("here")
	m2.nb.still = true
	run(m2, m2.runCommand("nb.open"))
	run(m2, m2.runCommand("nb.run_all"))
	if len(nu.jobs) != 0 || !strings.Contains(line(m2, contextLine), "Run this file's notebook cells?") {
		t.Fatalf("ran without asking: %q", line(m2, contextLine))
	}
	press(t, m2, "<enter>")
	if len(nu.jobs) != 1 {
		t.Errorf("didn't run after agreeing")
	}
	m2.nb.served = true
	run(m2, m2.runCommand("nb.run_all"))
	if len(nu.jobs) != 1 || !strings.Contains(m2.errMsg, "012 serve") {
		t.Errorf("ran served: %q", m2.errMsg)
	}
}

func TestNotebookBangAndTab(t *testing.T) {
	m := newModel()
	press(t, m, "!")
	if m.mode != modeEnter || m.line.Text() != "!" {
		t.Fatalf("on a sheet, ! starts an entry; mode %v, entry %q", m.mode, m.line.Text())
	}
	press(t, m, "<esc>")
	run(m, m.runCommand("nb.open"))
	press(t, m, "!")
	if !m.nbView().Editing() || len(m.sheet.NotebookCells()) != 1 {
		t.Errorf("! on a notebook adds a cell to edit")
	}
	if got := line(m, m.height-1); !strings.Contains(got, "❯ Notebook") {
		t.Errorf("the tab: %q", got)
	}
}
