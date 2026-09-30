package ui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/cmdhelp"
	"github.com/FelineStateMachine/012/internal/ui/shortcuts"
)

// ideNu is fakeNu answering the code editor's questions too, as nu
// 0.116 would about any pipeline, and noting them. Its check finds
// problem wherever the pipeline has it.
type ideNu struct {
	*fakeNu
	ide     []string
	problem string
	// help is the hover of every command, a command's help as nu
	// writes it.
	help string
}

func (f *ideNu) Run(ctx context.Context, job nushell.Job, script string, stdout io.Writer) error {
	if len(job.IDE) == 0 {
		return f.fakeNu.Run(ctx, job, script, stdout)
	}
	f.ide = append(f.ide, job.IDE[0])
	answer := map[string]string{"--version": "0.116.0", "--ide-ast": "[]", "--ide-complete": `{"completions": []}`}[job.IDE[0]]
	if i := strings.Index(script, f.problem); job.IDE[0] == "--ide-check" && f.problem != "" && i >= 0 {
		answer = fmt.Sprintf(`{"type":"diagnostic","severity":"Error","message":"Not a flag.","span":{"start":%d,"end":%d}}`, i, i+len(f.problem))
	}
	if job.IDE[0] == "--ide-hover" && f.help != "" {
		answer = f.hover(script, job.IDE[1])
	}
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

// hover answers as nu does about the word at byte at of script: a
// flag's "flag", a command's help.
func (f *ideNu) hover(script, at string) string {
	i, _ := strconv.Atoi(at)
	from, to := i, i
	for from > 0 && script[from-1] != ' ' && script[from-1] != '\n' {
		from--
	}
	for to < len(script) && script[to] != ' ' {
		to++
	}
	text := f.help
	switch {
	case from == to:
		return ""
	case script[from] == '-':
		text = "flag"
	case script[from] == '$' || script[from] == '|':
		return ""
	}
	b, _ := json.Marshal(map[string]any{"hover": text, "span": map[string]int{"start": from, "end": to}})
	return string(b)
}

// sortByHelp is sort-by's hover, as nu 0.116 writes it.
const sortByHelp = "Sort by the given cell path or closure.\n### Usage\n```\n  sort-by {flags} <...comparator>\n```\n\n### Flags\n\n" +
	"  `-h`, `--help` - Display the help message for this command\\\n  `-r`, `--reverse` - Sort in reverse order.\\\n" +
	"  `-i`, `--ignore-case` - Sort string-based data case-insensitively.\n\n### Parameters\n\n" +
	" `...comparator: oneof<cell-path, closure>` - The cell path(s) or closure(s) to compare elements by.\n\n\n" +
	"### Input/output types\n\n```\n  list<any> | list<any>\n  table | table\n\n```\n### Example(s)\n```\n```\n" +
	"  Sort files by modified date.\n```\n  ls | sort-by modified\n\n"

// The caret resting on a command says its signature on the context
// line; F1 opens its help, and Esc goes back to the cell being edited.
// F1 where there's no command opens the shortcuts.
func TestNotebookWordHelp(t *testing.T) {
	nu := &ideNu{fakeNu: &fakeNu{out: map[string]string{}}, help: sortByHelp}
	m := newModel()
	m.SetShellRunner(nu)
	m.SetMachine("here")
	m.nb.still = true
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	run(m, m.runCommand("nb.open"))
	press(t, m, "b", "<enter>", "ls | sort-by name -r", "<home>", "<right>", "<right>", "<right>", "<right>", "<right>", "<right>")
	if got := line(m, contextLine); !strings.Contains(got, "sort-by <...comparator: cell-path|closure> --reverse") ||
		!strings.Contains(got, "Sort by the given cell path or closure.") || !strings.Contains(got, "F1") {
		t.Fatalf("context line %q", got)
	}
	press(t, m, "<end>")
	if got := line(m, contextLine); !strings.Contains(got, "sort-by --reverse, -r") || !strings.Contains(got, "Sort in reverse order.") {
		t.Errorf("on the flag: %q", got)
	}
	press(t, m, "<f1>")
	if _, ok := m.overlay.(*cmdhelp.View); !ok {
		t.Fatalf("F1 opened %T", m.overlay)
	}
	if s := screen(m); !strings.Contains(s, "https://www.nushell.sh/commands/docs/sort-by.html") || !strings.Contains(s, "-r, --reverse") ||
		!strings.Contains(s, "ls | sort-by modified") || !strings.Contains(line(m, 0), "HELP") {
		t.Errorf("help\n%s", s)
	}
	press(t, m, "<esc>")
	if m.overlay != nil || !m.nbView().Editing() {
		t.Errorf("Esc: overlay %T, editing %v", m.overlay, m.nbView().Editing())
	}
	press(t, m, " ", "<f1>")
	if _, ok := m.overlay.(*shortcuts.View); !ok {
		t.Errorf("F1 on no word opened %T", m.overlay)
	}
}
