package e2e

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/creack/pty"
)

// lockedBuffer is a piped session's standard output, written by the
// command's copying goroutine and read by the test.
type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

// startPiped starts the session's command as a pipeline would, with
// stdin on its standard input and its standard output captured, and a
// new pseudo-terminal as its controlling terminal (what /dev/tty opens)
// on standard error: a shell's `... | 012 --pipe | ...` in a terminal.
func (s *session) startPiped(stdin string) (*os.File, error) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		return nil, err
	}
	defer tty.Close()
	if err := pty.Setsize(ptmx, &pty.Winsize{Cols: s.cols, Rows: s.rows}); err != nil {
		ptmx.Close()
		return nil, err
	}
	s.stdout = &lockedBuffer{}
	s.cmd.Stdin = strings.NewReader(stdin)
	s.cmd.Stdout = s.stdout
	s.cmd.Stderr = tty
	s.cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 2}
	if err := s.cmd.Start(); err != nil {
		ptmx.Close()
		return nil, err
	}
	return ptmx, nil
}

// finish waits for a piped session to end, returning its standard
// output and exit code.
func (s *session) finish() (string, int) {
	s.t.Helper()
	s.waitExit()
	s.cmd.Wait()
	return s.stdout.String(), s.cmd.ProcessState.ExitCode()
}

const lsNUON = "[[name, size, modified]; [CLAUDE.md, 1646b, 2026-09-27T11:27:31-06:00], [go.mod, 812b, 2026-09-20T08:00:00-06:00], [e2e, 4.2kb, 2026-09-28T17:45:10-06:00]]\n"

// TestPipeEditAndSend is ls | to nuon | 012 --pipe | ...: the table
// arrives on standard input and shows with its types on the terminal,
// an edit is made, and quitting sends it on standard output, which gets
// nothing else.
func TestPipeEditAndSend(t *testing.T) {
	s := startWith(t, options{piped: true, stdin: lsNUON, env: []string{"TZ=America/Denver"}}, "--pipe")
	s.waitFor("Read 4 rows of NUON from standard input")
	s.waitFor("4.2 kB")
	s.waitFor("Ctrl+Q  send the sheet as NUON")
	s.keys("<down>", "<down>", "<down>", "<down>", "notes.txt", "<tab>", "2048", "<enter>")
	s.waitFor("2048") // typed, a plain number: it goes out as an int
	s.keys("<ctrl+q>")
	s.waitFor("Send the sheet as NUON?")
	s.keys("<enter>")
	out, code := s.finish()
	want := "[[name, size, modified]; [CLAUDE.md, 1646b, 2026-09-27T11:27:31-06:00],\n" +
		"[go.mod, 812b, 2026-09-20T08:00:00-06:00],\n[e2e, 4200b, 2026-09-28T17:45:10-06:00],\n[notes.txt, 2048, null]]\n"
	if code != 0 || out != want {
		t.Errorf("exit %d, stdout:\n%s\nwant\n%s", code, out, want)
	}
}

// TestPipeSendSelectionAsCSV sends a selection, in the format --to
// asks for.
func TestPipeSendSelectionAsCSV(t *testing.T) {
	s := startWith(t, options{piped: true, stdin: lsNUON}, "--pipe", "--to", "csv")
	s.waitFor("4.2 kB")
	s.keys("<shift+down>", "<shift+down>", "<shift+right>")
	s.waitFor("send A1:B3 as CSV")
	s.keys("<ctrl+q>")
	s.waitFor("Send A1:B3 as CSV?")
	s.keys("<enter>")
	out, code := s.finish()
	if code != 0 || out != "name,size\nCLAUDE.md,1.6 kB\ngo.mod,812 B\n" {
		t.Errorf("exit %d, stdout %q", code, out)
	}
}

// TestPipeSendSelectionWithoutAsking: --send selection makes Ctrl+Q
// send the selection at once.
func TestPipeSendSelectionWithoutAsking(t *testing.T) {
	s := startWith(t, options{piped: true, stdin: lsNUON}, "--pipe", "--send", "selection", "--to", "csv")
	s.waitFor("Ctrl+Q  send the sheet as CSV")
	s.keys("<shift+down>", "<shift+right>")
	s.waitFor("Ctrl+Q  send A1:B2 as CSV")
	s.keys("<ctrl+q>")
	out, code := s.finish()
	if code != 0 || out != "name,size\nCLAUDE.md,1.6 kB\n" {
		t.Errorf("exit %d, stdout %q", code, out)
	}
}

// TestPipeSendSheetWithoutAsking: --send sheet makes Ctrl+Q send the
// whole sheet, whatever is selected; File > Quit without sending still
// sends nothing.
func TestPipeSendSheetWithoutAsking(t *testing.T) {
	s := startWith(t, options{piped: true, stdin: "a,b\n1,2\n3,4\n"}, "--pipe", "--send", "sheet")
	s.waitFor("Read 3 rows of CSV from standard input")
	s.keys("<shift+down>")
	s.waitFor("Ctrl+Q  send the sheet as CSV")
	s.keys("<ctrl+q>")
	out, code := s.finish()
	if code != 0 || out != "a,b\n1,2\n3,4\n" {
		t.Errorf("exit %d, stdout %q", code, out)
	}

	s = startWith(t, options{piped: true, stdin: "a,b\n1,2\n"}, "--pipe", "--send", "sheet")
	s.waitFor("Read 2 rows of CSV from standard input")
	s.keys("<ctrl+k>", "without sending")
	s.waitFor("Quit without sending")
	s.keys("<enter>")
	out, code = s.finish()
	if code != 1 || out != "" {
		t.Errorf("quit without sending: exit %d, stdout %q", code, out)
	}
}

// TestPipeQuitWithoutSending exits with status 1 and writes nothing.
func TestPipeQuitWithoutSending(t *testing.T) {
	s := startWith(t, options{piped: true, stdin: "a,b\n1,2\n"}, "--pipe")
	s.waitFor("Read 2 rows of CSV from standard input")
	s.keys("<ctrl+q>", "d")
	out, code := s.finish()
	if code != 1 || out != "" {
		t.Errorf("exit %d, stdout %q", code, out)
	}
}

// TestStdinOnly: 012 - reads standard input and writes nothing to
// standard output.
func TestStdinOnly(t *testing.T) {
	s := startWith(t, options{piped: true, stdin: `[{"a": 1, "b": "x"}]`}, "-")
	s.waitFor("Read 2 rows of JSON from standard input")
	s.keys("<ctrl+q>")
	s.waitFor("You have unsaved changes.")
	s.keys("d")
	out, code := s.finish()
	if code != 0 || out != "" {
		t.Errorf("exit %d, stdout %q", code, out)
	}
}
