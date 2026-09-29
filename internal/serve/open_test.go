package serve

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	gossh "golang.org/x/crypto/ssh"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui"
)

// openExec starts a session with a terminal and an exec request for
// command, as ssh -t host command does.
func openExec(t testing.TB, c *gossh.Client, command string) (*term, error) {
	t.Helper()
	sess, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.RequestPty("xterm-256color", 24, 90, gossh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	tm := &term{sess: sess, done: make(chan error, 1), wrote: make(chan struct{}, 1)}
	tm.stdin, _ = sess.StdinPipe()
	sess.Stdout = tm
	sess.Stderr = tm
	if err := sess.Start(command); err != nil {
		sess.Close()
		return nil, err
	}
	go func() { tm.done <- sess.Wait() }()
	return tm, nil
}

// saveSheet writes a one-cell sheet at path.
func saveSheet(t *testing.T, path, a1 string) {
	t.Helper()
	s := sheet.New()
	s.Set(sheet.Addr{}, a1)
	var b bytes.Buffer
	if err := s.Write(&b); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func a1(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	s, err := sheet.Read(f)
	if err != nil || s.Cell(sheet.Addr{}) == nil {
		return ""
	}
	return s.Cell(sheet.Addr{}).Input
}

// ssh -t host book.012 opens book.012 from the served directory; a name
// with no such file starts a new sheet to be saved under it.
func TestOpenFromCommandLine(t *testing.T) {
	key := newKey(t)
	srv, addr, dir := testServer(t, nil, key)
	saveSheet(t, filepath.Join(dir, "book.012"), "from disk")
	c, err := dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	for _, name := range []string{"book.012", "book"} {
		tm, err := openExec(t, c, name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		tm.waitFor(t, "012 - book.012", "from disk")
		io.WriteString(tm.stdin, "\x11")
		tm.waitClosed(t)
	}
	tm, err := openExec(t, c, "fresh.012")
	if err != nil {
		t.Fatal(err)
	}
	tm.waitFor(t, "012 - fresh.012", "READY")
	io.WriteString(tm.stdin, "new\r\x13")
	deadline := time.Now().Add(5 * time.Second)
	for a1(t, filepath.Join(dir, "fresh.012")) != "new" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := a1(t, filepath.Join(dir, "fresh.012")); got != "new" {
		t.Errorf("Ctrl+S saved %q", got)
	}
	io.WriteString(tm.stdin, "\x11")
	tm.waitClosed(t)
}

// An exec request is only ever a file name inside the served directory:
// commands, names outside it or hidden, and requests without a terminal
// are refused, and nothing runs.
func TestExecConfined(t *testing.T) {
	key := newKey(t)
	srv, addr, dir := testServer(t, nil, key)
	outside := filepath.Join(filepath.Dir(dir), "outside.012")
	saveSheet(t, outside, "secret")
	os.Symlink(outside, filepath.Join(dir, "link.012"))
	os.Mkdir(filepath.Join(dir, ".012-recovery"), 0o700)
	saveSheet(t, filepath.Join(dir, ".012-recovery", "book-20260101-120000.012"), "kept")
	saveSheet(t, filepath.Join(dir, "book.012"), "ok")
	c, err := dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	for _, name := range []string{"../outside.012", "../outside", outside, "link.012", "link", ".env", ".012-recovery/book-20260101-120000.012", "sub/../../outside.012"} {
		tm, err := openExec(t, c, name)
		if err != nil {
			t.Fatalf("%s: request refused before the name was checked: %v", name, err)
		}
		tm.waitClosed(t)
		out := tm.output()
		if !strings.Contains(out, "outside the served directory") || strings.Contains(out, "secret") || strings.Contains(out, "kept") {
			t.Errorf("%s: %q", name, out)
		}
		if strings.Contains(out, dir) {
			t.Errorf("%s: the message names the server's directory: %q", name, out)
		}
	}

	// Anything but one word is refused outright, as are exec requests
	// without a terminal and names with control characters.
	for _, cmd := range []string{"cat /etc/passwd", "book.012 book.012", "sh -c id", "", "'book\x1b[2J.012'"} {
		tm, err := openExec(t, c, cmd)
		if err == nil {
			tm.waitClosed(t)
			if out := tm.output(); strings.Contains(out, "READY") {
				t.Errorf("%q started 012: %q", cmd, out)
			}
		}
	}
	sess, _ := c.NewSession()
	if out, err := sess.CombinedOutput("book.012"); err == nil || len(out) > 0 {
		t.Errorf("exec without a terminal: %q, %v", out, err)
	}
}

// A session that idles out with unsaved changes keeps them in a
// recovery file, 0600 in a 0700 directory, and says where; one without
// changes keeps nothing.
func TestRecoveryOnIdle(t *testing.T) {
	key := newKey(t)
	srv, addr, dir := testServer(t, func(o *Options) { o.IdleTimeout = 500 * time.Millisecond }, key)
	c, err := dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	tm := openTerm(t, c, 80, 24)
	tm.waitFor(t, "READY")
	tm.waitClosed(t)
	if _, err := os.Stat(filepath.Join(dir, ".012-recovery")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an unchanged session kept something: %v", err)
	}

	tm, err = openExec(t, c, "book.012")
	if err != nil {
		t.Fatal(err)
	}
	tm.waitFor(t, "READY")
	io.WriteString(tm.stdin, "42\r")
	tm.waitClosed(t)
	out := tm.output()
	if !strings.Contains(out, "closed after 500ms without input") ||
		!strings.Contains(out, "unsaved changes kept in .012-recovery/book-") ||
		!strings.Contains(out, "open book.012 again to restore them") {
		t.Fatalf("no recovery note in %q", out[max(0, len(out)-300):])
	}
	rdir := filepath.Join(dir, ".012-recovery")
	st, err := os.Stat(rdir)
	if err != nil || st.Mode().Perm() != 0o700 {
		t.Fatalf("recovery directory: %v %v", st, err)
	}
	files, _ := filepath.Glob(filepath.Join(rdir, "book-*.012"))
	if len(files) != 1 {
		t.Fatalf("recovery files %v", files)
	}
	if st, _ := os.Stat(files[0]); st.Mode().Perm() != 0o600 || a1(t, files[0]) != "42" {
		t.Errorf("recovery file %v, A1 %q", st.Mode(), a1(t, files[0]))
	}
}

// Stopping the server keeps each session's unsaved changes and says so;
// the next session opening the file offers them back, and saving them
// removes the recovery file.
func TestRecoveryOnShutdown(t *testing.T) {
	key := newKey(t)
	srv, addr, dir := testServer(t, nil, key)
	saveSheet(t, filepath.Join(dir, "book.012"), "saved")
	c, err := dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	tm, err := openExec(t, c, "book.012")
	if err != nil {
		t.Fatal(err)
	}
	tm.waitFor(t, "saved")
	io.WriteString(tm.stdin, "unsaved\r")
	tm.waitFor(t, "modified")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	tm.waitClosed(t)
	if out := tm.output(); !strings.Contains(out, "the server is stopping") || !strings.Contains(out, "open book.012 again") {
		t.Fatalf("no note: %q", out[max(0, len(out)-300):])
	}
	c.Close()

	srv, addr, _ = testServer(t, func(o *Options) { o.Dir = dir }, key)
	c, err = dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	tm = openTerm(t, c, 90, 24)
	tm.waitFor(t, "READY")
	io.WriteString(tm.stdin, "\x0f") // Ctrl+O
	tm.waitFor(t, "Open file")
	io.WriteString(tm.stdin, "book\r")
	tm.waitFor(t, "Unsaved changes to book.012 were kept.", "Restore")
	io.WriteString(tm.stdin, "\r")
	tm.waitFor(t, "unsaved", "Restored the kept changes")
	if got := a1(t, filepath.Join(dir, "book.012")); got != "saved" {
		t.Fatalf("restoring wrote the file: %q", got)
	}
	io.WriteString(tm.stdin, "\x13")
	deadline := time.Now().Add(5 * time.Second)
	for a1(t, filepath.Join(dir, "book.012")) != "unsaved" && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	files, _ := filepath.Glob(filepath.Join(dir, ".012-recovery", "*"))
	if a1(t, filepath.Join(dir, "book.012")) != "unsaved" || len(files) != 0 {
		t.Errorf("after saving: A1 %q, recovery files %v", a1(t, filepath.Join(dir, "book.012")), files)
	}
	io.WriteString(tm.stdin, "\x11")
	tm.waitClosed(t)
}

// A session that stopped on an internal error keeps its unsaved work,
// however it ended, and tells the client the report is in the log.
func TestRecoveryOnCrash(t *testing.T) {
	dir := t.TempDir()
	root, err := confine.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	m := ui.New(sheet.New(), "book.012")
	m.Serve(root, nil)
	m.Update(tea.KeyPressMsg{Code: '7', Text: "7"})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	e := ending{crash: &ui.Crash{Where: "update", Value: "boom"}}
	e.keep(ui.Guard(m))
	var out strings.Builder
	e.tell(&out, time.Minute, root)
	if e.err != nil || !strings.HasPrefix(e.kept, filepath.Join(".012-recovery", "book-")) {
		t.Fatalf("kept %q, %v", e.kept, e.err)
	}
	if !strings.Contains(out.String(), "stopped on an internal error; the server's log has a report") ||
		!strings.Contains(out.String(), "open book.012 again to restore them") {
		t.Errorf("told %q", out.String())
	}
}
