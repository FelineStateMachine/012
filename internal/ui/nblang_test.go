package ui

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// ideNu is fakeNu answering the code editor's questions too, as nu
// 0.116 would about any pipeline, and noting them.
type ideNu struct {
	*fakeNu
	ide []string
}

func (f *ideNu) Run(ctx context.Context, job nushell.Job, script string, stdout io.Writer) error {
	if len(job.IDE) == 0 {
		return f.fakeNu.Run(ctx, job, script, stdout)
	}
	f.ide = append(f.ide, job.IDE[0])
	answer := map[string]string{"--version": "0.116.0", "--ide-ast": "[]", "--ide-complete": `{"completions": []}`}[job.IDE[0]]
	_, err := io.WriteString(stdout, answer)
	return err
}

// asked are the questions nu was asked since the last call.
func (f *ideNu) asked() []string {
	out := f.ide
	f.ide = nil
	return out
}

// Writing a cell asks nu about it once typing pauses; a file from
// another computer isn't asked about until its cells are trusted, and
// 012 serve asks nu only as serve-shell allows.
func TestNotebookAsksNuAsTrusted(t *testing.T) {
	nu := &ideNu{fakeNu: &fakeNu{out: map[string]string{"ls": lsOut}}}
	m := newModel()
	m.SetShellRunner(nu)
	m.SetMachine("here")
	m.nb.still = true
	run(m, m.runCommand("nb.open"))
	press(t, m, "b", "<enter>", "l")
	if got := nu.asked(); !slices.Contains(got, "--ide-ast") || !slices.Contains(got, "--ide-check") {
		t.Fatalf("a notebook made here: asked %v", got)
	}
	press(t, m, "<esc>")

	var b bytes.Buffer
	if err := m.book().Write(&b); err != nil {
		t.Fatal(err)
	}
	s, err := sheet.Read(strings.NewReader(strings.Replace(b.String(), `"macroOrigin": "here"`, `"macroOrigin": "elsewhere"`, 1)))
	if err != nil {
		t.Fatal(err)
	}
	m2 := New(s, "")
	m2.Update(tea.WindowSizeMsg{Width: 80, Height: 20})
	m2.SetShellRunner(nu)
	m2.SetMachine("here")
	m2.nb.still = true
	run(m2, m2.runCommand("nb.open"))
	press(t, m2, "<enter>", "s")
	if got := nu.asked(); len(got) != 0 {
		t.Errorf("a file from elsewhere, not trusted: asked %v", got)
	}
	press(t, m2, "<esc>")
	run(m2, m2.runCommand("nb.run_all"))
	press(t, m2, "<enter>") // Run this file's notebook cells? Run
	press(t, m2, "<enter>", "s")
	if got := nu.asked(); !slices.Contains(got, "--ide-ast") {
		t.Errorf("trusted: asked %v", got)
	}
	press(t, m2, "<esc>")
	m2.nb.served = true
	press(t, m2, "<enter>", "s")
	if got := nu.asked(); len(got) != 0 {
		t.Errorf("served without serve-shell: asked %v", got)
	}
}
