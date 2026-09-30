package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"
)

var (
	ctrlQ = tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl}
	down  = tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift}
	enter = tea.KeyPressMsg{Code: tea.KeyEnter}
	keyD  = tea.KeyPressMsg{Code: 'd', Text: "d"}
)

// fakeTerminal stands in for the terminal and Bubble Tea: it checks the
// UI was given a terminal of its own (when tty is set), carries out the
// model's commands as the program would, feeding their messages back,
// answers the device attributes as a terminal does, which the UI waits
// for before it draws, then presses keys until the model quits.
type fakeTerminal struct {
	t      *testing.T
	keys   []tea.KeyPressMsg
	opened bool // openTTY was called
	given  int  // the options the program was given
}

func (f *fakeTerminal) install(e *env) {
	e.openTTY = func() (io.ReadCloser, io.WriteCloser, error) {
		f.opened = true
		r, w := io.Pipe()
		return r, w, nil
	}
	e.runTUI = func(m tea.Model, opts ...tea.ProgramOption) error {
		f.given = len(opts)
		if f.drive(m, m.Init()) {
			f.t.Fatal("quit on starting")
		}
		f.drive(m, func() tea.Msg { return tea.WindowSizeMsg{Width: 100, Height: 30} })
		for _, k := range f.keys {
			if f.drive(m, func() tea.Msg { return k }) {
				return nil
			}
		}
		f.t.Fatal("the keys didn't quit")
		return nil
	}
}

// drive runs cmd and what its messages lead to, reporting whether the
// model quit. A command that takes more than a few seconds (waiting on
// the terminal) is dropped.
func (f *fakeTerminal) drive(m tea.Model, cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(3 * time.Second):
		return false
	}
	if _, ok := msg.(tea.QuitMsg); ok {
		return true
	}
	// Batches and sequences are slices of commands.
	if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == reflect.TypeFor[tea.Cmd]() {
		for i := range v.Len() {
			if c, ok := v.Index(i).Interface().(tea.Cmd); ok && f.drive(m, c) {
				return true
			}
		}
		return false
	}
	if msg == nil {
		return false
	}
	_, next := m.Update(msg)
	if raw, ok := msg.(tea.RawMsg); ok && strings.Contains(fmt.Sprint(raw.Msg), ansi.RequestPrimaryDeviceAttributes) {
		_, answer := m.Update(uv.PrimaryDeviceAttributesEvent{62, 22})
		next = tea.Batch(next, answer)
	}
	return f.drive(m, next)
}

const lsNUON = "[[name, size]; [CLAUDE.md, 1646b], [go.mod, 1500b]]\n"

func pipeEnv(t *testing.T, stdin string, keys ...tea.KeyPressMsg) (env, *strings.Builder, *fakeTerminal) {
	t.Helper()
	e, _, _ := testEnv(t, nil)
	out := &strings.Builder{}
	e.stdin, e.stdout, e.stderr = strings.NewReader(stdin), out, io.Discard
	f := &fakeTerminal{t: t, keys: keys}
	f.install(&e)
	return e, out, f
}

func TestPipeSendsTheSheet(t *testing.T) {
	e, out, f := pipeEnv(t, lsNUON, ctrlQ, enter)
	if err := run([]string{"--pipe"}, e); err != nil {
		t.Fatal(err)
	}
	want := "[[name, size]; [CLAUDE.md, 1646b],\n[go.mod, 1500b]]\n"
	if out.String() != want {
		t.Errorf("stdout %q, want %q", out, want)
	}
	if !f.opened || f.given != 2 {
		t.Errorf("the UI wasn't given the terminal: opened %v, %d options", f.opened, f.given)
	}
}

func TestPipeTo(t *testing.T) {
	e, out, _ := pipeEnv(t, lsNUON, ctrlQ, enter)
	if err := run([]string{"--pipe", "--to", "csv"}, e); err != nil {
		t.Fatal(err)
	}
	if out.String() != "name,size\nCLAUDE.md,1.6 kB\ngo.mod,1.5 kB\n" {
		t.Errorf("stdout %q", out)
	}
	e, out, _ = pipeEnv(t, "a\tb\n1\t2\n", ctrlQ, enter)
	if err := run([]string{"--pipe", "--to=json"}, e); err != nil {
		t.Fatal(err)
	}
	if out.String() != "[\n{\"a\":1,\"b\":2}\n]\n" {
		t.Errorf("stdout %q", out)
	}
}

// TestPipeSend: with --send, Ctrl+Q sends without asking: the
// selection, or the sheet whatever is selected.
func TestPipeSend(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--pipe", "--send", "selection", "--to", "csv"}, "name\nCLAUDE.md\n"},
		{[]string{"--pipe", "--send=sheet", "--to", "csv"}, "name,size\nCLAUDE.md,1.6 kB\ngo.mod,1.5 kB\n"},
	} {
		e, out, _ := pipeEnv(t, lsNUON, down, ctrlQ)
		if err := run(tc.args, e); err != nil {
			t.Fatal(err)
		}
		if out.String() != tc.want {
			t.Errorf("%q: stdout %q, want %q", tc.args, out, tc.want)
		}
	}
	// Only one cell selected: --send selection sends the sheet.
	e, out, _ := pipeEnv(t, lsNUON, ctrlQ)
	if err := run([]string{"--pipe", "--send", "selection", "--to", "tsv"}, e); err != nil || out.String() != "name\tsize\nCLAUDE.md\t1.6 kB\ngo.mod\t1.5 kB\n" {
		t.Errorf("one cell: %v, stdout %q", err, out)
	}
}

func TestPipeQuitWithoutSending(t *testing.T) {
	e, out, _ := pipeEnv(t, lsNUON, ctrlQ, keyD)
	if err := run([]string{"--pipe"}, e); !errors.Is(err, errNotSent) {
		t.Errorf("error %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout %q", out)
	}
}

// TestStdinWithoutPipe: 012 - reads standard input, writes nothing to
// standard output, and asks before quitting with the data unsaved.
func TestStdinWithoutPipe(t *testing.T) {
	e, out, f := pipeEnv(t, lsNUON, ctrlQ, keyD)
	if err := run([]string{"-"}, e); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || !f.opened {
		t.Errorf("stdout %q, terminal opened %v", out, f.opened)
	}
}

func TestPipeAFile(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	os.WriteFile(filepath.Join(dir, "data.tsv"), []byte("a\tb\n1\t2\n"), 0o600)
	e, out, _ := pipeEnv(t, "", ctrlQ, enter)
	e.isTTY = true
	if err := run([]string{"--pipe", "data.tsv"}, e); err != nil {
		t.Fatal(err)
	}
	if out.String() != "a\tb\n1\t2\n" {
		t.Errorf("stdout %q", out)
	}
}

func TestPipeArguments(t *testing.T) {
	for _, tc := range []struct {
		args []string
		tty  bool
		want string
	}{
		{[]string{"-"}, true, "nothing piped in"},
		{[]string{"--pipe"}, true, "nothing piped in"},
		{[]string{"--to", "csv"}, false, "--to goes with --pipe"},
		{[]string{"--pipe", "--to", "xlsx"}, false, "012 writes nuon, json, csv or tsv"},
		{[]string{"--pipe", "--to"}, false, "--to needs a value: nuon, json, csv or tsv"},
		{[]string{"-", "a.csv"}, false, "not both"},
		{[]string{"-", "--send", "sheet"}, false, "--send goes with --pipe: 012 --pipe --send sheet"},
		{[]string{"--send=ask", "a.csv"}, false, "--send goes with --pipe"},
		{[]string{"--pipe", "--send", "all"}, false, "--send all: 012 sends ask, selection or sheet"},
		{[]string{"--pipe", "--send"}, false, "--send needs a value: ask, selection or sheet"},
	} {
		e, _, _ := pipeEnv(t, lsNUON)
		e.isTTY = tc.tty
		if err := run(tc.args, e); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%q: %v, want %q", tc.args, err, tc.want)
		}
	}
}
