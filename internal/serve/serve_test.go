package serve

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	typesafe "github.com/FelineStateMachine/typesafe-go"
	gossh "golang.org/x/crypto/ssh"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// newKey makes a client key.
func newKey(t testing.TB) gossh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// testServer serves a fresh directory on a random loopback port to the
// given keys, returning the server, its address and the served
// directory.
func testServer(t testing.TB, change func(*Options), keys ...gossh.Signer) (*Server, string, string) {
	t.Helper()
	top := t.TempDir()
	dir := filepath.Join(top, "served")
	os.Mkdir(dir, 0o755)
	var ak bytes.Buffer
	for _, k := range keys {
		ak.Write(gossh.MarshalAuthorizedKey(k.PublicKey()))
	}
	akPath := filepath.Join(top, "authorized_keys")
	if err := os.WriteFile(akPath, ak.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	o := Defaults()
	o.Dir, o.AuthorizedKeys, o.HostKey, o.Listen = dir, akPath, filepath.Join(top, "config", "host_key"), "127.0.0.1:0"
	if change != nil {
		change(&o)
	}
	srv, err := New(o, Deps{})
	if err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("tcp", o.Listen)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(l) }()
	t.Cleanup(func() {
		srv.Close()
		if err := <-done; err != nil {
			t.Errorf("Serve: %v", err)
		}
	})
	return srv, l.Addr().String(), srv.Dir()
}

func dial(addr string, key gossh.Signer, host gossh.PublicKey) (*gossh.Client, error) {
	return gossh.Dial("tcp", addr, &gossh.ClientConfig{
		User:            "dami",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(key)},
		HostKeyCallback: gossh.FixedHostKey(host),
		Timeout:         5 * time.Second,
	})
}

// term is a client session with a terminal, collecting what it's sent.
type term struct {
	sess  *gossh.Session
	stdin io.WriteCloser
	mu    sync.Mutex
	out   bytes.Buffer
	done  chan error
	wrote chan struct{} // signalled after output arrives
}

func openTerm(t testing.TB, c *gossh.Client, w, h int) *term {
	t.Helper()
	sess, err := c.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.RequestPty("xterm-256color", h, w, gossh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}
	tm := &term{sess: sess, done: make(chan error, 1), wrote: make(chan struct{}, 1)}
	tm.stdin, _ = sess.StdinPipe()
	sess.Stdout = tm
	if err := sess.Shell(); err != nil {
		t.Fatal(err)
	}
	go func() { tm.done <- sess.Wait() }()
	return tm
}

func (tm *term) Write(p []byte) (int, error) {
	tm.mu.Lock()
	n, err := tm.out.Write(p)
	tm.mu.Unlock()
	select {
	case tm.wrote <- struct{}{}:
	default:
	}
	return n, err
}

func (tm *term) output() string {
	tm.mu.Lock()
	defer tm.mu.Unlock()
	return tm.out.String()
}

// waitFor waits until the output holds each of want.
func (tm *term) waitFor(t testing.TB, want ...string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		out := tm.output()
		ok := true
		for _, w := range want {
			ok = ok && strings.Contains(out, w)
		}
		if ok {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("output never showed %q; got %q", want, tm.output())
}

// waitClosed waits for the session to end.
func (tm *term) waitClosed(t testing.TB) {
	t.Helper()
	select {
	case <-tm.done:
	case <-time.After(10 * time.Second):
		t.Fatal("the session didn't close")
	}
}

// A session gets its own 012: the first screen renders at the client's
// size, and quitting ends the session.
func TestSession(t *testing.T) {
	key := newKey(t)
	srv, addr, dir := testServer(t, nil, key)
	c, err := dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	tm := openTerm(t, c, 90, 20)
	tm.waitFor(t, "READY", "012 - untitled", "Sheet1")
	if n := srv.Sessions(); n != 1 {
		t.Errorf("Sessions() = %d", n)
	}

	// Type a number and save it: the file lands in the served directory.
	io.WriteString(tm.stdin, "42\r")
	// Copying goes to the client's clipboard, through its terminal
	// (OSC 52), not the server's.
	io.WriteString(tm.stdin, "\x1b[A\x03")
	tm.waitFor(t, "\x1b]52;c;"+base64.StdEncoding.EncodeToString([]byte("42")))
	io.WriteString(tm.stdin, "\x13") // Ctrl+S asks for a name
	tm.waitFor(t, "Save as")
	io.WriteString(tm.stdin, "kept\r")
	waitFile(t, filepath.Join(dir, "kept.012"))

	io.WriteString(tm.stdin, "\x11") // Ctrl+Q: saved, so it quits at once
	tm.waitClosed(t)
	deadline := time.Now().Add(5 * time.Second)
	for srv.Sessions() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := srv.Sessions(); n != 0 {
		t.Errorf("Sessions() = %d after quitting", n)
	}
}

func waitFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if f, err := os.Open(path); err == nil {
			s, err := sheet.Read(f)
			f.Close()
			if err == nil && s.Cell(sheet.Addr{}) != nil && s.Cell(sheet.Addr{}).Input == "42" {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s wasn't saved", path)
}

// A key missing from authorized_keys can't log in, and neither can a
// password.
func TestRejected(t *testing.T) {
	good, bad := newKey(t), newKey(t)
	srv, addr, _ := testServer(t, nil, good)
	if c, err := dial(addr, bad, srv.HostKey()); err == nil {
		c.Close()
		t.Fatal("a key not in authorized_keys logged in")
	}
	c, err := gossh.Dial("tcp", addr, &gossh.ClientConfig{
		User: "dami",
		Auth: []gossh.AuthMethod{gossh.Password("hunter2"), gossh.KeyboardInteractive(
			func(string, string, []string, []bool) ([]string, error) { return nil, nil })},
		HostKeyCallback: gossh.FixedHostKey(srv.HostKey()),
		Timeout:         5 * time.Second,
	})
	if err == nil {
		c.Close()
		t.Fatal("password auth logged in")
	}
}

// Logged in, a client can run 012 and nothing else: no commands, no
// subsystems, no forwarding, no session without a terminal.
func TestOnlyShell(t *testing.T) {
	key := newKey(t)
	srv, addr, _ := testServer(t, nil, key)
	c, err := dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	sess, _ := c.NewSession()
	if out, err := sess.CombinedOutput("cat /etc/passwd"); err == nil || len(out) > 0 {
		t.Errorf("exec ran: %q, %v", out, err)
	}
	sess, _ = c.NewSession()
	if err := sess.RequestSubsystem("sftp"); err == nil {
		t.Error("the sftp subsystem started")
	}
	if conn, err := c.Dial("tcp", "127.0.0.1:22"); err == nil {
		conn.Close()
		t.Error("local port forwarding was allowed")
	}
	if l, err := c.Listen("tcp", "127.0.0.1:0"); err == nil {
		l.Close()
		t.Error("remote port forwarding was allowed")
	}
	// A shell without a terminal is told to ask for one and closed.
	sess, _ = c.NewSession()
	var stderr bytes.Buffer
	sess.Stderr = &stderr
	sess.Stdout = io.Discard
	if err := sess.Shell(); err != nil {
		t.Fatal(err)
	}
	var exit *gossh.ExitError
	if err := sess.Wait(); !errors.As(err, &exit) || exit.ExitStatus() != 1 || !strings.Contains(stderr.String(), "needs a terminal") {
		t.Errorf("shell without a terminal: %v, %q", err, stderr.String())
	}
}

// Sessions past the limit are turned away; a place frees when one ends.
func TestMaxSessions(t *testing.T) {
	key := newKey(t)
	srv, addr, _ := testServer(t, func(o *Options) { o.MaxSessions = 1 }, key)
	c, err := dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	first := openTerm(t, c, 80, 24)
	first.waitFor(t, "READY")
	second := openTerm(t, c, 80, 24)
	second.waitClosed(t)
	io.WriteString(first.stdin, "\x11")
	first.waitClosed(t)
	third := openTerm(t, c, 80, 24)
	third.waitFor(t, "READY")
}

// A session without input for the idle timeout is closed with a note.
func TestIdleTimeout(t *testing.T) {
	key := newKey(t)
	srv, addr, _ := testServer(t, func(o *Options) { o.IdleTimeout = 300 * time.Millisecond }, key)
	c, err := dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	tm := openTerm(t, c, 80, 24)
	tm.waitFor(t, "READY")
	tm.waitClosed(t)
	if !strings.Contains(tm.output(), "closed after 300ms without input") {
		t.Errorf("no idle note in %q", tm.output()[max(0, len(tm.output())-200):])
	}
}

// fakeJEV answers every question with 0.9 (90%), slowly enough
// that two sessions' questions are in flight together.
type fakeJEV struct{ calls atomic.Int64 }

func (f *fakeJEV) SystemOne(context.Context, typesafe.SystemOneRequest) (*typesafe.SystemOneResponse, error) {
	f.calls.Add(1)
	time.Sleep(50 * time.Millisecond)
	return &typesafe.SystemOneResponse{Answers: map[string]typesafe.Answer{"answer": typesafe.NoulAnswer{Noul: 0.9}}}, nil
}

// Two sessions are two separate spreadsheets: what one types, the other
// doesn't see, and each gets the answers to its own JEV questions.
func TestSessionsAreSeparate(t *testing.T) {
	key := newKey(t)
	fake := &fakeJEV{}
	srv, addr, _ := testServer(t, nil, key)
	srv.jev = fake
	c, err := dial(addr, key, srv.HostKey())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	a, b := openTerm(t, c, 80, 24), openTerm(t, c, 100, 30)
	a.waitFor(t, "READY")
	b.waitFor(t, "READY")
	io.WriteString(a.stdin, "only in a\r")
	a.waitFor(t, "only in a")
	for _, tm := range []*term{a, b} {
		io.WriteString(tm.stdin, `=JEV.PROB("the box was crushed", "Is this a complaint?")`+"\r")
	}
	a.waitFor(t, "90%")
	b.waitFor(t, "90%")
	if strings.Contains(b.output(), "only in a") {
		t.Error("session b shows what session a typed")
	}
	if n := fake.calls.Load(); n != 2 {
		t.Errorf("%d JEV requests; want one per session", n)
	}
	for _, tm := range []*term{a, b} {
		io.WriteString(tm.stdin, "\x11")
		tm.waitFor(t, "unsaved changes")
		io.WriteString(tm.stdin, "d")
		tm.waitClosed(t)
	}
}
