package nbview

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// testdata/nu holds what nu 0.116 said (the .json and .jsonl files)
// about the text 012 asks it about a cell (the .nu beside each: the
// prelude, then the source masked), so these tests pin both the text
// asked and the answers read.

// fakeRunner answers nu's IDE questions from recorded answers, by the
// flag and the text asked about, and notes each question.
type fakeRunner struct {
	mu      sync.Mutex
	version string
	answers map[string]string // flags, a line, the text asked about → answer
	err     error             // every question fails so
	block   bool              // questions wait until they're stopped
	asked   []string
}

func (f *fakeRunner) Run(ctx context.Context, job nushell.Job, script string, stdout io.Writer) error {
	f.mu.Lock()
	f.asked = append(f.asked, job.IDE[0])
	f.mu.Unlock()
	switch {
	case f.err != nil:
		return f.err
	case job.IDE[0] == "--version":
		_, err := io.WriteString(stdout, f.version)
		return err
	case f.block:
		<-ctx.Done()
		return context.Cause(ctx)
	}
	out, ok := f.answers[asked(job.IDE, script)]
	if !ok {
		return &nushell.Error{Msg: "not recorded: " + script}
	}
	_, err := io.WriteString(stdout, out)
	return err
}

// asked is how fakeRunner files a question.
func asked(flags []string, script string) string { return strings.Join(flags, " ") + "\n" + script }

func (f *fakeRunner) questions() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.asked)
}

// recording is a question to nu and its answer, as testdata/nu keeps
// them.
type recording struct {
	flags          []string
	script, answer string
}

// recordings are testdata/nu's questions and answers.
func recordings(t *testing.T) []recording {
	t.Helper()
	read := func(name string) string {
		data, err := os.ReadFile(filepath.Join("testdata", "nu", name))
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	var out []recording
	for _, r := range []struct{ name, answer, flag string }{
		{"highlight", "highlight.json", "--ide-ast"},
		{"sheet", "sheet.json", "--ide-ast"},
		{"check", "check.jsonl", "--ide-check"},
		{"complete", "complete.json", "--ide-complete"},
	} {
		script := read(r.name + ".nu")
		flags := []string{r.flag}
		switch r.flag {
		case "--ide-check":
			flags = append(flags, fmt.Sprint(maxProblems))
		case "--ide-complete":
			flags = append(flags, fmt.Sprint(len(script))) // the caret at the end
		}
		out = append(out, recording{flags: flags, script: script, answer: read(r.answer)})
	}
	script := read("hover.nu")
	for _, at := range hoverAsked {
		out = append(out, recording{flags: []string{"--ide-hover", fmt.Sprint(at)}, script: script, answer: read(fmt.Sprintf("hover-%d.json", at))})
	}
	return out
}

// hoverAsked are the bytes of hover.nu nu's hover is recorded at: the
// sort-by, -r, into string, --decimals and $n of hoverSrc.
var hoverAsked = []int{83, 96, 101, 113, 135}

// hoverSrc is the cell hover.nu asks about.
const hoverSrc = "let n = 4\n$files | sort-by size -r | into string --decimals 2 | append $n"

// recordedNu is a fake nu answering what testdata/nu records.
func recordedNu(t *testing.T) *fakeRunner {
	t.Helper()
	f := &fakeRunner{version: "0.116.0\n", answers: map[string]string{}}
	for _, r := range recordings(t) {
		f.answers[asked(r.flags, r.script)] = r.answer
	}
	return f
}

// nuFor is a notebook's Nu over r, allowed, knowing the cell $files.
func nuFor(r nushell.Runner) (*NuSession, *Nu) {
	s := NewNuSession(r)
	s.SetOn(true)
	n := s.For()
	n.SetWords([]string{"files"}, []Word{{Text: "$files", Desc: "a cell's output"}, {Text: "sort-by", Desc: "command"}})
	return s, n
}

var roleNames = [...]string{"command", "string", "variable", "number", "keyword", "operator", "comment"}

func describe(src string, spans []Span) []string {
	roles := rolesOf(src, spans)
	var out []string
	rs := []rune(src)
	for i := 0; i < len(rs); {
		j := i + 1
		for j < len(rs) && roles[j] == roles[i] {
			j++
		}
		if roles[i] >= 0 && strings.TrimSpace(string(rs[i:j])) != "" {
			out = append(out, strings.TrimSpace(string(rs[i:j]))+":"+roleNames[roles[i]])
		}
		i = j
	}
	return out
}

func TestNuHighlights(t *testing.T) {
	_, n := nuFor(recordedNu(t))
	src := "big = $files | where size > 1kb | sort-by size --reverse # largest"
	got := describe(src, n.Highlight(context.Background(), src))
	want := []string{"big:variable", "=:operator", "$files:variable", "|:operator", "where:command", "size:variable", ">:operator",
		"1kb:number", "|:operator", "sort-by:command", "size:string", "# largest:comment"}
	if !slices.Equal(got, want) {
		t.Errorf("roles\n got %q\nwant %q", got, want)
	}
	src = "$sheet.A1:C9 | append $selection | first 2"
	got = describe(src, n.Highlight(context.Background(), src))
	want = []string{"$sheet.A1:C9:variable", "|:operator", "append:command", "$selection:variable", "|:operator", "first:command", "2:number"}
	if !slices.Equal(got, want) {
		t.Errorf("a range of a sheet\n got %q\nwant %q", got, want)
	}
}

func TestNuChecks(t *testing.T) {
	_, n := nuFor(recordedNu(t))
	src := "$files | sort-by size --revrse | append $nope"
	var got []string
	for _, d := range n.Check(context.Background(), src) {
		got = append(got, src[d.From:d.To]+": "+d.Msg)
	}
	want := []string{"--revrse: The `sort-by` command doesn't have flag `revrse`.", "$nope: Variable not found."}
	if !slices.Equal(got, want) {
		t.Errorf("problems\n got %q\nwant %q", got, want)
	}
}

func TestNuCompletes(t *testing.T) {
	_, n := nuFor(recordedNu(t))
	src := "$files | so"
	var got []string
	for _, c := range n.Complete(context.Background(), src, len(src)) {
		got = append(got, fmt.Sprintf("%s %d-%d", c.Text, c.From, c.To))
	}
	want := []string{"sort-by 9-11", "sort 9-11", "source 9-11", "source-env 9-11"}
	if !slices.Equal(got, want) {
		t.Errorf("completions\n got %q\nwant %q", got, want)
	}
}

func TestReplaced(t *testing.T) {
	for _, c := range []struct {
		src, w string
		want   int
	}{
		{"ls | str jo", "str join", 5},
		{"$env.PA", "PATH", 5},
		{"ls | sort-by size -", "--reverse", 18},
		{"ls ~/Dev", "~/Developer/", 3},
		{"ls ", "a.txt", 3},
		{"open 'my fi", "`my file.txt`", 5},
	} {
		if got := replaced(c.src, len(c.src), c.w); got != c.want {
			t.Errorf("replaced(%q, %q) = %d, want %d", c.src, c.w, got, c.want)
		}
	}
}

// nu missing, too old, slow, or not allowed: the built-ins answer, and
// nu isn't asked again.
func TestNuFallsBack(t *testing.T) {
	src := "ls | where size > 1kb # big"
	builtin := describe(src, Tokens{}.Highlight(context.Background(), src))
	for name, f := range map[string]*fakeRunner{
		"missing": {err: nushell.ErrMissing},
		"old":     {version: "0.99.0"},
		"slow":    {version: "0.116.0", block: true},
	} {
		s, n := nuFor(f)
		s.Timeout = 20 * time.Millisecond
		for range slowAfter {
			if got := describe(src, n.Highlight(context.Background(), src)); !slices.Equal(got, builtin) {
				t.Errorf("%s: roles %q", name, got)
			}
			if d := n.Check(context.Background(), src); d != nil {
				t.Errorf("%s: problems %v", name, d)
			}
		}
		if !n.Instant() {
			t.Errorf("%s: nu is still asked", name)
		}
		asked := len(f.questions())
		n.Highlight(context.Background(), src)
		if _, ok := n.Hover(context.Background(), src, 6); ok {
			t.Errorf("%s: a hover", name)
		}
		if got := n.Complete(context.Background(), "$fi", 3); len(got) != 1 || got[0].Text != "$files" || len(f.questions()) != asked {
			t.Errorf("%s: completions %v, asked %v", name, got, f.questions())
		}
	}
	f := recordedNu(t)
	s, n := nuFor(f)
	s.SetOn(false) // 012 serve without serve-shell, or a file not trusted
	n.Highlight(context.Background(), src)
	n.Check(context.Background(), src)
	n.Complete(context.Background(), src, 2)
	n.Hover(context.Background(), src, 6)
	if q := f.questions(); len(q) != 0 {
		t.Errorf("asked nu while it isn't allowed: %v", q)
	}
}

// drain runs cmd and what it batches, returning their messages.
func drain(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	switch msg := cmd().(type) {
	case tea.BatchMsg:
		var out []tea.Msg
		for _, c := range msg {
			out = append(out, drain(c)...)
		}
		return out
	case nil:
		return nil
	default:
		return []tea.Msg{msg}
	}
}

// Typing asks nu nothing; once typing pauses, nu is asked about the
// text as it is then, and its answers draw: the roles, the underline
// and, with the caret on it, the problem on the context line.
func TestEditAsksNuOncePaused(t *testing.T) {
	f := recordedNu(t)
	h := newHost("")
	v := newView(h, 80, 20)
	_, n := nuFor(f)
	v.Providers = Providers{Highlighter: n, Completer: n, Checker: n}
	v.StartEdit()
	src := "$files | sort-by size --revrse | append $nope"
	for _, r := range src {
		v.Key(key(string(r)))
	}
	text(v)
	if q := f.questions(); len(q) != 0 {
		t.Fatalf("typing asked nu: %v", q)
	}
	if cmd, _ := v.Update(pausedMsg{view: v, version: v.edit.version - 1}); cmd != nil {
		t.Error("a pause at an older text asked nu")
	}
	cmd, _ := v.Update(pausedMsg{view: v, version: v.edit.version})
	for _, msg := range drain(cmd) {
		v.Update(msg)
	}
	if v.edit.diagFor != src || len(v.edit.diags) != 2 {
		t.Fatalf("problems %v about %q", v.edit.diags, v.edit.diagFor)
	}
	if got := v.Diagnostic(); got != "Variable not found." {
		t.Errorf("the caret after $nope: %q", got)
	}
	v.Key(key("home"))
	if got := v.Diagnostic(); got != "" {
		t.Errorf("the caret on no problem: %q", got)
	}
	line := strings.Split(text(v), "\n")[1]
	if !strings.Contains(line, "--revrse") {
		t.Errorf("line %q", line)
	}
	marks := v.edit.marks()
	if i := strings.Index(src, "--revrse"); !marks[i] || marks[i-1] {
		t.Errorf("the underline: %v", marks)
	}
}

// A question out when the text changes is stopped, and its answer
// dropped.
func TestStaleAnswersDropped(t *testing.T) {
	f := &fakeRunner{version: "0.116.0", block: true}
	h := newHost("ls")
	v := newView(h, 80, 20)
	_, n := nuFor(f)
	v.Providers = Providers{Highlighter: n, Completer: n, Checker: n}
	v.StartEdit()
	cmd, _ := v.Update(pausedMsg{view: v, version: v.edit.version})
	v.Key(key("x")) // stops the questions out
	done := make(chan []tea.Msg)
	go func() { done <- drain(cmd) }()
	select {
	case msgs := <-done:
		if len(msgs) != 0 {
			t.Errorf("stale answers: %v", msgs)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the questions weren't stopped")
	}
}

// Between keys, the text being edited keeps nu's roles where it's
// unchanged, the tokenizer's only where it changed.
func TestEditKeepsRolesBetweenKeys(t *testing.T) {
	h := newHost("big = $files | where size > 1kb | sort-by size --reverse # largest")
	v := newView(h, 80, 20)
	_, n := nuFor(recordedNu(t))
	v.Providers = Providers{Highlighter: n, Completer: n, Checker: n}
	v.StartEdit()
	cmd, _ := v.Update(pausedMsg{view: v, version: v.edit.version})
	for _, msg := range drain(cmd) {
		v.Update(msg)
	}
	answered := v.spansFor(v.edit.text(), h.cells[0].Kind, true)
	v.Key(key("x"))
	now := v.spansFor(v.edit.text(), h.cells[0].Kind, true)
	if !slices.Equal(now[:len(answered)], answered) || now[len(answered)] != int8(theme.SyntaxComment) {
		t.Errorf("roles\n was %v\n now %v", answered, now)
	}
}

// The real nu, when it's the version recorded, says what testdata/nu
// records about the same text.
func TestRecordedAnswersAreNus(t *testing.T) {
	if _, err := exec.LookPath("nu"); err != nil {
		t.Skip("nu isn't installed")
	}
	if v, err := nushell.Version(context.Background(), nushell.Nu{}); err != nil || v != "0.116.0" {
		t.Skipf("nu %s %v isn't the version recorded", v, err)
	}
	for _, r := range recordings(t) {
		var b strings.Builder
		if err := (nushell.Nu{}).Run(context.Background(), nushell.Job{IDE: r.flags}, r.script, &b); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(b.String()) != strings.TrimSpace(r.answer) {
			t.Errorf("nu %v now says\n%s\nrecorded\n%s", r.flags, b.String(), r.answer)
		}
	}
}
