package ui

import (
	"encoding/base64"
	"slices"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// ownersCell is a cell of two statements: files is its own variable,
// read by its second line.
const ownersCell = "files = ls | where type == file\n$files | where size > 1kb | sort-by size --reverse"

// varsOut is what nu prints for a run handing back variables: a line
// for the output and each variable, its NUON as base64.
func varsOut(out string, vars ...string) string {
	enc := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	lines := []string{"__out " + enc(out)}
	for i := 0; i+1 < len(vars); i += 2 {
		lines = append(lines, vars[i]+" "+enc(vars[i+1]))
	}
	return strings.Join(lines, "\n") + "\n"
}

// nuCommand is what nu runs for src.
func nuCommand(src string) string {
	cmd, _, _ := notebook.Parse(src).Command()
	return cmd
}

// The owner's cell runs as a script: files is the cell's own, not a cell
// it reads, its output is its last line's, it has no name, and a later
// cell reads $files from its run.
func TestNotebookStatements(t *testing.T) {
	big := "[[name, size]; [c.txt, 5kb], [a.txt, 2kb]]"
	m, nu := notebookModel(t, map[string]string{
		nuCommand(ownersCell): varsOut(big, "files", lsOut),
		"$files | length":     "3",
	})
	write(t, m, ownersCell)
	write(t, m, "$files | length")
	press(t, m, "<ctrl+enter>") // the second runs the first first
	if got := nu.ran(); len(got) != 2 || !strings.HasPrefix(got[0], "let files = ls") {
		t.Fatalf("ran %q", got)
	}
	if !slices.Equal(nu.jobs[0].Vars, []string{"files"}) || len(nu.jobs[0].Tables) != 0 {
		t.Errorf("the first job %+v", nu.jobs[0])
	}
	if got := string(nu.jobs[1].Tables["files"]); got != lsOut {
		t.Errorf("$files reached the second cell as %q", got)
	}
	s := screen(m)
	if strings.Contains(s, "failed") || strings.Contains(s, "─ files ─") || !strings.Contains(s, "c.txt") {
		t.Errorf("screen:\n%s", s)
	}
	cs := m.sheet.NotebookCells()
	if o := m.book().Output(cs[0].ID); o == nil || string(o.NUON) != big || string(o.Vars["files"]) != lsOut {
		t.Errorf("output %+v", o)
	}
	press(t, m, "<up>", "<up>", "<up>")
	if i, _ := m.nbView().Selected(); i != 0 {
		t.Fatalf("selected %d", i)
	}
	if got := line(m, contextLine); !strings.Contains(got, "sets $files") || strings.Contains(got, "reads") {
		t.Errorf("context line %q", got)
	}
	// Running it again makes the reader stale.
	press(t, m, "<ctrl+enter>")
	if st := m.cellState(m.sheet, cs[1].ID); !st.Stale {
		t.Error("the reader isn't stale")
	}
	// Naming the cell names its last statement.
	press(t, m, "n")
	send(m, pasteMsg("big"))
	press(t, m, "<enter>")
	if got := m.sheet.NotebookCells()[0].Source; got != "files = ls | where type == file\nbig = $files | where size > 1kb | sort-by size --reverse" {
		t.Errorf("named %q", got)
	}
}

// Comments are nu's: a commented-out assignment names nothing and a
// commented-out $name reads nothing.
func TestNotebookComments(t *testing.T) {
	src := "# files = ls\nls # | where name == $nope"
	m, nu := notebookModel(t, map[string]string{nuCommand(src): lsOut})
	write(t, m, src)
	press(t, m, "<ctrl+enter>")
	if len(nu.jobs) != 1 || len(nu.jobs[0].Tables) != 0 || strings.Contains(nu.jobs[0].Command, "#") {
		t.Fatalf("ran %+v", nu.jobs)
	}
	if strings.Contains(screen(m), "failed") || m.sheet.NotebookCells()[0].Name() != "" {
		t.Errorf("screen:\n%s", screen(m))
	}
}

// A cell of several statements streams its last one's values, its own
// variables bound first and none handed back.
func TestNotebookStreamStatements(t *testing.T) {
	streamPolls = nil
	m, _ := notebookModel(t, nil)
	nu := newStreamNu()
	m.SetShellRunner(nu)
	write(t, m, "log = open app.log\n$log | lines")
	press(t, m, "f")
	defer m.stopCells()
	script := <-nu.scripts
	if !strings.Contains(script, "let log = open app.log\n$log | lines") || strings.Contains(script, "__out") {
		t.Errorf("stream script %s", script)
	}
}
