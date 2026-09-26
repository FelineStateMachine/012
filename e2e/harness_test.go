// Package e2e runs the real one23 binary on a pseudo-terminal and renders
// its output with libghostty-vt, Ghostty's terminal emulator core. Tests
// assert on what a user would actually see: screen text, cursor position,
// window title and screen mode. Keystrokes are encoded by libghostty from
// the terminal's current modes, so they match what Ghostty would send.
package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
	ghostty "go.mitchellh.com/libghostty"
)

var binPath string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "one23-e2e")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "one23")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/one23")
	build.Dir = ".."
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building one23: %v\n%s", err, out)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

const waitTimeout = 5 * time.Second

// session is one running instance of one23 attached to a virtual terminal.
type session struct {
	t   *testing.T
	dir string
	cmd *exec.Cmd
	pty *os.File

	mu   sync.Mutex // guards vt, enc, ev and raw
	raw  []byte     // everything the program wrote, for debugging
	vt   *ghostty.Terminal
	enc  *ghostty.KeyEncoder
	ev   *ghostty.KeyEvent
	cols uint16
	rows uint16

	exited chan struct{} // closed when the pty reader stops
}

// options configures a session.
type options struct {
	dir        string // working directory; a fresh temp dir if empty
	cols, rows uint16 // defaults to 100x30
	light      bool   // use the light reference palette
}

// start launches one23 in dir (a fresh temp dir if empty) with args.
func start(t *testing.T, dir string, args ...string) *session {
	t.Helper()
	return startWith(t, options{dir: dir}, args...)
}

func startWith(t *testing.T, o options, args ...string) *session {
	t.Helper()
	if o.dir == "" {
		o.dir = t.TempDir()
	}
	if o.cols == 0 {
		o.cols, o.rows = 100, 30
	}
	s := &session{t: t, dir: o.dir, cols: o.cols, rows: o.rows, exited: make(chan struct{})}

	var err error
	if s.vt, err = ghostty.NewTerminal(ghostty.WithSize(s.cols, s.rows)); err != nil {
		t.Fatal(err)
	}
	applyPalette(t, s.vt, o.light)
	if s.enc, err = ghostty.NewKeyEncoder(); err != nil {
		t.Fatal(err)
	}
	if s.ev, err = ghostty.NewKeyEvent(); err != nil {
		t.Fatal(err)
	}

	s.cmd = exec.Command(binPath, args...)
	s.cmd.Dir = o.dir
	s.cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLORTERM=truecolor")
	s.pty, err = pty.StartWithSize(s.cmd, &pty.Winsize{Cols: s.cols, Rows: s.rows})
	if err != nil {
		t.Fatal(err)
	}

	// Replies to the program's queries (device attributes, sizes, mode
	// reports) go back through the pty, as in a real terminal.
	s.vt.SetEffectWritePty(func(_ *ghostty.Terminal, data []byte) {
		s.pty.Write(append([]byte(nil), data...))
	})
	s.vt.SetEffectSize(func(*ghostty.Terminal) (ghostty.SizeReportSize, bool) {
		return ghostty.SizeReportSize{Columns: s.cols, Rows: s.rows, CellWidth: 8, CellHeight: 16}, true
	})

	go func() {
		defer close(s.exited)
		buf := make([]byte, 32*1024)
		for {
			n, err := s.pty.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.vt.Write(buf[:n])
				s.raw = append(s.raw, buf[:n]...)
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()

	t.Cleanup(func() {
		s.cmd.Process.Kill()
		s.cmd.Wait()
		s.pty.Close()
		<-s.exited
		s.ev.Close()
		s.enc.Close()
		s.vt.Close()
	})
	s.waitFor("READY")
	return s
}

// screen returns the visible screen as plain text.
func (s *session) screen() string {
	return s.format(ghostty.FormatterFormatPlain)
}

// html returns the visible screen as HTML (see renderHTML).
func (s *session) html() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out, err := renderHTML(s.vt)
	if err != nil {
		s.t.Fatal(err)
	}
	return out
}

func (s *session) format(format ghostty.FormatterFormat) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := ghostty.NewFormatter(s.vt,
		ghostty.WithFormatterFormat(format),
		ghostty.WithFormatterTrim(true))
	if err != nil {
		s.t.Fatal(err)
	}
	defer f.Close()
	out, err := f.FormatString()
	if err != nil {
		s.t.Fatal(err)
	}
	return out
}

// line returns screen line i (0-based), trimmed on the right.
func (s *session) line(i int) string {
	lines := strings.Split(s.screen(), "\n")
	if i >= len(lines) {
		return ""
	}
	return strings.TrimRight(lines[i], " ")
}

// eventually polls cond until it holds or the timeout expires.
func (s *session) eventually(what string, cond func() bool) {
	s.t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for !cond() {
		if time.Now().After(deadline) {
			s.t.Fatalf("timed out waiting for %s; screen:\n%s", what, s.screen())
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func (s *session) waitFor(text string) {
	s.t.Helper()
	s.eventually(fmt.Sprintf("%q", text), func() bool { return strings.Contains(s.screen(), text) })
}

func (s *session) waitForLine(i int, want string) {
	s.t.Helper()
	s.eventually(fmt.Sprintf("line %d == %q", i, want), func() bool { return s.line(i) == want })
}

// waitExit waits for the program to end.
func (s *session) waitExit() {
	s.t.Helper()
	select {
	case <-s.exited:
	case <-time.After(waitTimeout):
		s.t.Fatalf("program did not exit; screen:\n%s", s.screen())
	}
}

// keys sends key presses. Named keys are written in angle brackets, e.g.
// "<down>"; anything else is typed character by character.
func (s *session) keys(keys ...string) {
	s.t.Helper()
	for _, k := range keys {
		if name, ok := strings.CutPrefix(k, "<"); ok {
			parts := strings.Split(strings.TrimSuffix(name, ">"), "+")
			var mods ghostty.Mods
			for _, p := range parts[:len(parts)-1] {
				switch p {
				case "shift":
					mods |= ghostty.ModShift
				case "ctrl":
					mods |= ghostty.ModCtrl
				case "alt":
					mods |= ghostty.ModAlt
				}
			}
			base := parts[len(parts)-1]
			if key, ok := namedKeys[base]; ok {
				var cp rune
				if base == "space" {
					cp = ' ' // the encoder needs the key's codepoint
				}
				s.press(key, mods, "", cp)
				continue
			}
			if ck, ok := charKeys[rune(base[0])]; ok && len(base) == 1 {
				s.press(ck.key, mods|ck.mods, "", rune(base[0]))
				continue
			}
			s.t.Fatalf("unknown key %s", k)
		}
		for _, r := range k {
			key, ok := charKeys[r]
			if !ok {
				s.t.Fatalf("no key mapping for %q", r)
			}
			unshifted := key.base
			if key.mods&ghostty.ModShift == 0 {
				unshifted = r
			}
			s.press(key.key, key.mods, string(r), unshifted)
		}
	}
}

func (s *session) press(key ghostty.Key, mods ghostty.Mods, text string, unshifted rune) {
	s.mu.Lock()
	s.enc.SetOptFromTerminal(s.vt)
	s.ev.SetAction(ghostty.KeyActionPress)
	s.ev.SetKey(key)
	s.ev.SetMods(mods)
	// Shift that produced the text is consumed by the layout, as a real
	// terminal reports it; otherwise "+" would encode as shift+"=".
	var consumed ghostty.Mods
	if text != "" {
		consumed = mods & ghostty.ModShift
	}
	s.ev.SetConsumedMods(consumed)
	s.ev.SetUTF8(text)
	s.ev.SetUnshiftedCodepoint(unshifted)
	data, err := s.enc.Encode(s.ev)
	s.mu.Unlock()
	if err != nil {
		s.t.Fatalf("encoding key %v: %v", key, err)
	}
	if _, err := s.pty.Write(data); err != nil {
		s.t.Fatal(err)
	}
	// Give the program a moment so rapid keys aren't coalesced into one
	// read that it could misparse as a paste or an escape sequence.
	time.Sleep(5 * time.Millisecond)
}

// resize changes both the virtual terminal and the pty, which sends the
// program SIGWINCH.
func (s *session) resize(cols, rows uint16) {
	s.mu.Lock()
	s.cols, s.rows = cols, rows
	err := s.vt.Resize(cols, rows, 8, 16)
	s.mu.Unlock()
	if err != nil {
		s.t.Fatal(err)
	}
	if err := pty.Setsize(s.pty, &pty.Winsize{Cols: cols, Rows: rows}); err != nil {
		s.t.Fatal(err)
	}
}

func (s *session) cursor() (x, y int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cx, _ := s.vt.CursorX()
	cy, _ := s.vt.CursorY()
	return int(cx), int(cy)
}

func (s *session) title() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, _ := s.vt.Title()
	return t
}

func (s *session) activeScreen() ghostty.TerminalScreen {
	s.mu.Lock()
	defer s.mu.Unlock()
	scr, _ := s.vt.ActiveScreen()
	return scr
}

var namedKeys = map[string]ghostty.Key{
	"enter": ghostty.KeyEnter, "esc": ghostty.KeyEscape, "backspace": ghostty.KeyBackspace,
	"tab": ghostty.KeyTab, "delete": ghostty.KeyDelete, "home": ghostty.KeyHome,
	"up": ghostty.KeyArrowUp, "down": ghostty.KeyArrowDown,
	"left": ghostty.KeyArrowLeft, "right": ghostty.KeyArrowRight,
	"pgup": ghostty.KeyPageUp, "pgdown": ghostty.KeyPageDown,
	"f1": ghostty.KeyF1, "f2": ghostty.KeyF2, "f5": ghostty.KeyF5, "f10": ghostty.KeyF10,
	"space": ghostty.KeySpace,
}

type physKey struct {
	key  ghostty.Key
	mods ghostty.Mods
	base rune // unshifted character, for shifted symbols
}

// charKeys maps typed characters to physical keys on a US layout.
var charKeys = func() map[rune]physKey {
	m := map[rune]physKey{' ': {key: ghostty.KeySpace}}
	for i := range 26 {
		k := ghostty.KeyA + ghostty.Key(i)
		m[rune('a'+i)] = physKey{key: k}
		m[rune('A'+i)] = physKey{key: k, mods: ghostty.ModShift, base: rune('a' + i)}
	}
	digits := ")!@#$%^&*("
	for i := range 10 {
		k := ghostty.KeyDigit0 + ghostty.Key(i)
		m[rune('0'+i)] = physKey{key: k}
		m[rune(digits[i])] = physKey{key: k, mods: ghostty.ModShift, base: rune('0' + i)}
	}
	for _, p := range []struct {
		plain, shifted rune
		key            ghostty.Key
	}{
		{'-', '_', ghostty.KeyMinus}, {'=', '+', ghostty.KeyEqual},
		{',', '<', ghostty.KeyComma}, {'.', '>', ghostty.KeyPeriod},
		{'/', '?', ghostty.KeySlash}, {';', ':', ghostty.KeySemicolon},
		{'\'', '"', ghostty.KeyQuote},
	} {
		m[p.plain] = physKey{key: p.key}
		m[p.shifted] = physKey{key: p.key, mods: ghostty.ModShift, base: p.plain}
	}
	return m
}()
