package ui

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/nuprompt"
)

// fakeNu answers commands from a table of outputs, failing those it
// lacks, and keeps the jobs it ran.
type fakeNu struct {
	out  map[string]string
	jobs []nushell.Job
}

func (f *fakeNu) Run(_ context.Context, job nushell.Job, _ string, stdout io.Writer) error {
	if job.Command == "help commands | get name" {
		_, err := io.WriteString(stdout, `["ls", "where", "sort-by", "str join"]`)
		return err
	}
	f.jobs = append(f.jobs, job)
	out, ok := f.out[job.Command]
	if !ok {
		return &nushell.Error{Msg: "Command `" + job.Command + "` not found"}
	}
	_, err := io.WriteString(stdout, out)
	return err
}

func notebookModel(t *testing.T, out map[string]string) (*Model, *fakeNu) {
	t.Helper()
	m := newModel()
	nu := &fakeNu{out: out}
	m.SetShellRunner(nu)
	m.SetMachine("here")
	return m, nu
}

const lsOut = "[[name, size]; [a.txt, 2kb], [b.txt, 10b], [c.txt, 5kb]]"

func TestShellMakesRegions(t *testing.T) {
	m, nu := notebookModel(t, map[string]string{
		"ls":                     lsOut,
		"$r1 | where size > 1kb": "[[name, size]; [a.txt, 2kb], [c.txt, 5kb]]",
	})
	shell(t, m)
	if _, ok := m.overlay.(*nuprompt.Prompt); !ok || m.indicator() != "NU" {
		t.Fatalf("no prompt: %T %q", m.overlay, m.indicator())
	}
	if got := bar(m); got != nuprompt.Mark[:len(nuprompt.Mark)-1] {
		t.Errorf("formula bar %q", got)
	}
	press(t, m, "ls", "<enter>")
	if m.sheet.Name() != "Shell 1" || !m.sheet.Notebook() {
		t.Fatalf("on %q, notebook %v", m.sheet.Name(), m.sheet.Notebook())
	}
	if got := input(m, "A1"); got != "r1  ls" {
		t.Errorf("label %q", got)
	}
	if got := m.sheet.ShownText(addr("B3")); got != "2.0 kB" {
		t.Errorf("B3 shows %q", got)
	}
	if !strings.Contains(m.shell.said, "r1: 3 rows") {
		t.Errorf("said %q", m.shell.said)
	}
	press(t, m, "$r1 | where size > 1kb", "<enter>")
	if got := input(m, "A7"); got != "r2  $r1 | where size > 1kb" {
		t.Errorf("r2's label %q", got)
	}
	if len(nu.jobs) != 2 || !strings.Contains(string(nu.jobs[1].Tables["r1"]), "a.txt") {
		t.Fatalf("r2 read r1 as %q", nu.jobs[len(nu.jobs)-1].Tables)
	}
	// Refreshing r1 runs r2 after it.
	press(t, m, "<esc>")
	m.cur = addr("A1")
	nu.out["ls"] = "[[name, size]; [d.txt, 9kb]]"
	press(t, m, "<enter>")
	if got := input(m, "A3"); got != "d.txt" {
		t.Errorf("r1 refreshed to %q", got)
	}
	if got := input(m, "A5"); !strings.HasPrefix(got, "r2  ") {
		t.Errorf("r2 didn't move up: A5 %q", got)
	}
	if last := nu.jobs[len(nu.jobs)-1]; last.Command != "$r1 | where size > 1kb" || !strings.Contains(string(last.Tables["r1"]), "d.txt") {
		t.Errorf("r2 wasn't run after r1: %+v", last)
	}
	// Undo takes the refresh back, one region at a time.
	press(t, m, "<ctrl+z>")
	press(t, m, "<ctrl+z>")
	if got := input(m, "A3"); got != "a.txt" {
		t.Errorf("after undo A3 %q", got)
	}
}

func TestShellFails(t *testing.T) {
	m, _ := notebookModel(t, map[string]string{})
	shell(t, m, "lss", "<enter>")
	if got := input(m, "A1"); got != "r1  lss   failed" {
		t.Errorf("label %q", got)
	}
	if !strings.Contains(line(m, contextLine), "r1 failed: Command `lss` not found") {
		t.Errorf("context line %q", line(m, contextLine))
	}
	press(t, m, "<esc>")
	m.cur = addr("A1")
	if !strings.Contains(line(m, contextLine), "r1 failed") {
		t.Errorf("on the label: %q", line(m, contextLine))
	}
	// Named: name = pipeline, and redefining it.
	m.shell.runner.(*fakeNu).out["ls"] = lsOut
	shell(t, m, "files = ls", "<enter>")
	if _, r, ok := m.book().Region("files"); !ok || r.Command != "ls" {
		t.Fatalf("no region files: %+v", r)
	}
	press(t, m, "r1 = ls", "<enter>")
	if got := input(m, "A1"); got != "r1  ls" {
		t.Errorf("r1 redefined: %q", got)
	}
	if got := m.book().Sheet(1).Value(addr("A3")).String(); got != "a.txt" {
		t.Errorf("r1's table %q", got)
	}
}

func TestShellCompletes(t *testing.T) {
	m, _ := notebookModel(t, map[string]string{"ls": lsOut})
	shell(t, m, "ls", "<enter>", "$")
	p := m.overlay.(*nuprompt.Prompt)
	if got := p.Shown(); len(got) != 1 || got[0].Text != "$r1" {
		t.Fatalf("completions of $: %+v", got)
	}
	press(t, m, "<tab>")
	if m.line.Text() != "$r1" {
		t.Errorf("tab put %q", m.line.Text())
	}
	press(t, m, " | so")
	if got := p.Shown(); len(got) != 1 || got[0].Text != "sort-by" {
		t.Errorf("completions of so: %+v", got)
	}
	if !strings.Contains(screen(m), "Completions") {
		t.Error("no completions box")
	}
	// History: Up recalls the lines run.
	m.line.Clear()
	p.Changed()
	press(t, m, "<up>")
	if m.line.Text() != "ls" {
		t.Errorf("up recalled %q", m.line.Text())
	}
	if got := m.book().ShellHistory(); len(got) != 1 || got[0] != "ls" {
		t.Errorf("history %q", got)
	}
}

func TestShellTrust(t *testing.T) {
	m, nu := notebookModel(t, map[string]string{"ls": lsOut})
	shell(t, m, "ls", "<enter>", "<esc>")
	if m.book().MacroOrigin() != "here" {
		t.Errorf("origin %q", m.book().MacroOrigin())
	}
	// A file from elsewhere asks before its commands run.
	m.book().SetMacroOrigin("elsewhere")
	m.macros.trusted = false
	m.cur = addr("A1")
	n := len(nu.jobs)
	press(t, m, "<enter>")
	if len(nu.jobs) != n || !strings.Contains(line(m, contextLine), "Run this file's shell commands?") {
		t.Fatalf("didn't ask: %q", line(m, contextLine))
	}
	press(t, m, "<esc>")
	if len(nu.jobs) != n {
		t.Error("ran after Esc")
	}
	press(t, m, "<f9>", "<enter>")
	if len(nu.jobs) != n+1 || m.book().MacroOrigin() != "here" {
		t.Errorf("run all after trusting: %d jobs, origin %q", len(nu.jobs)-n, m.book().MacroOrigin())
	}
	// Served, commands are off unless the server allows them.
	m.shell.served = true
	press(t, m, "<f9>")
	if !strings.Contains(m.errMsg, "off in 012 serve") {
		t.Errorf("served: %q", m.errMsg)
	}
}

func TestShellGuardsAndFreeze(t *testing.T) {
	m, _ := notebookModel(t, map[string]string{"ls": lsOut})
	shell(t, m, "ls", "<enter>", "<esc>")
	m.cur = addr("A3")
	press(t, m, "x", "<enter>")
	if got := input(m, "A3"); got != "a.txt" {
		t.Errorf("typed over a region: %q", got)
	}
	if !strings.Contains(m.note, "region r1") {
		t.Errorf("note %q", m.note)
	}
	if got := bar(m); got != "nu❯ ls" {
		t.Errorf("formula bar on a region %q", got)
	}
	run(m, m.runCommand("nu.freeze"))
	press(t, m, "x", "<enter>")
	if got := input(m, "A3"); got != "x" {
		t.Errorf("frozen A3 %q", got)
	}
	if got := input(m, "B4"); got != "10" || m.sheet.ShownText(addr("B4")) != "10 B" {
		t.Errorf("frozen B4 %q shows %q", got, m.sheet.ShownText(addr("B4")))
	}
}

// Sorting a region's table keeps the order through the next run.
func TestShellSortsRegion(t *testing.T) {
	m, _ := notebookModel(t, map[string]string{"ls": lsOut})
	shell(t, m, "ls", "<enter>", "<esc>")
	m.cur = addr("B3")
	run(m, m.runCommand("data.sort_range_za"))
	if got := input(m, "A3"); got != "c.txt" {
		t.Fatalf("sorted A3 %q, note %q", got, m.note)
	}
	m.cur = addr("A1")
	press(t, m, "<enter>")
	if got := input(m, "A3"); got != "c.txt" {
		t.Errorf("after refresh A3 %q", got)
	}
}

// Opening a saved notebook shows its regions not run, and F9 runs them.
func TestShellReopened(t *testing.T) {
	m, _ := notebookModel(t, map[string]string{"ls": lsOut, "$r1 | first": "{name: a.txt, size: 2kb}"})
	shell(t, m, "ls", "<enter>", "$r1 | first", "<enter>", "<esc>")
	s := roundTripSheet(t, m.sheet)
	m2, nu := notebookModel(t, map[string]string{"ls": lsOut, "$r1 | first": "{name: a.txt, size: 2kb}"})
	m2.reset(s, "nb.012")
	if got := input(m2, "A1"); got != "r1  ls   not run" {
		t.Errorf("label %q", got)
	}
	if !strings.Contains(line(m2, contextLine), "run all") {
		t.Errorf("context line %q", line(m2, contextLine))
	}
	press(t, m2, "<f9>")
	if len(nu.jobs) != 2 || input(m2, "A1") != "r1  ls" {
		t.Errorf("run all: %d jobs, A1 %q", len(nu.jobs), input(m2, "A1"))
	}
}

func roundTripSheet(t *testing.T, s *sheet.Sheet) *sheet.Sheet {
	t.Helper()
	var b strings.Builder
	if err := s.Write(&b); err != nil {
		t.Fatal(err)
	}
	got, err := sheet.Read(strings.NewReader(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	return got
}
