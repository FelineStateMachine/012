package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Problems in the config file show on the context line; 012 starts
// anyway, with defaults for what's wrong.
func TestConfigWarnings(t *testing.T) {
	s := startWith(t, options{config: "theme = Dracula\nbogus = 1\nchart-images = maybe\n"})
	s.waitFor(`Config: config:2: unknown key "bogus" (and 1 more; run 012 config)`)
	if !strings.Contains(s.html(), "background:#1e1f29") {
		t.Error("the valid theme line was not applied")
	}
}

// openTheme opens File > Settings > Theme.
func openTheme(s *session) {
	s.keys("<alt+f>", "<up>", "<up>", "<right>", "<down>", "<down>", "<down>")
	s.waitFor("Pick a color theme")
	s.keys("<enter>")
	s.waitFor("─ Theme ─")
}

// The theme picker previews the highlighted theme, Esc restores the one
// there was, and Enter keeps and saves it, so the next start has it.
func TestThemePicker(t *testing.T) {
	cfg := t.TempDir()
	s := startWith(t, options{configDir: cfg})
	openTheme(s)
	s.keys("Dracula")
	s.eventually("Dracula previewed", func() bool { return strings.Contains(s.html(), "background:#1e1f29") })
	s.keys("<esc>")
	s.eventually("the terminal theme restored", func() bool { return !strings.Contains(s.html(), "#1e1f29") })
	s.waitFor("READY")

	openTheme(s)
	s.keys("Dracula", "<enter>")
	s.waitFor("Theme Dracula, saved in")
	data, err := os.ReadFile(filepath.Join(cfg, "012", "config"))
	if err != nil || string(data) != "theme = Dracula\n" {
		t.Errorf("config file %q %v", data, err)
	}
	s.keys("<ctrl+q>")
	s.waitExit()

	r := startWith(t, options{configDir: cfg})
	r.eventually("Dracula at start", func() bool { return strings.Contains(r.html(), "background:#1e1f29") })
}

// A .env with the key next to the sheet is never read: JEV stays off,
// and the context line says how to store the key instead.
func TestDotEnvNotRead(t *testing.T) {
	srv, requests := fakeTypeSafe(t)
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, ".env"), []byte("TYPESAFE_API_KEY=test-key\nTYPESAFE_BASE_URL="+srv.URL+"\n"), 0o600)
	s := startWith(t, options{dir: dir})
	s.waitFor("012 no longer reads TYPESAFE_API_KEY from .env files")
	s.keys(`=JEV.TEST("x", "Q")`, "<enter>", "<up>")
	s.waitFor("JEV functions need an API key")
	if n := requests.Load(); n != 0 {
		t.Errorf("%d requests with a key from .env", n)
	}
}

// 012 config set-key stores the key (here in the tests' stand-in for the
// credential store); the next start finds it there and JEV works.
func TestSetKeyThenJEV(t *testing.T) {
	srv, requests := fakeTypeSafe(t)
	cfg := t.TempDir()
	cmd := exec.Command(binPath, "config", "set-key")
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+cfg, "TYPESAFE_API_KEY=")
	cmd.Stdin = strings.NewReader("test-key\n")
	out, err := cmd.CombinedOutput()
	if err != nil || !strings.Contains(string(out), "Saved the JEV API key in the test credential store") || strings.Contains(string(out), "test-key") {
		t.Fatalf("set-key: %v\n%s", err, out)
	}
	s := startWith(t, options{configDir: cfg, env: []string{"TYPESAFE_BASE_URL=" + srv.URL}})
	s.keys(`=JEV.TEST("The box was crushed", "Is this a complaint?")`, "<enter>")
	s.waitForLine(gridRow1, "    1    TRUE")
	if requests.Load() != 1 {
		t.Errorf("%d requests", requests.Load())
	}
}

// File > Settings > JEV API key takes the key masked, stores it and turns
// JEV on at once, after one test call with it.
func TestAPIKeyPrompt(t *testing.T) {
	srv, _ := fakeTypeSafe(t)
	cfg := t.TempDir()
	s := startWith(t, options{configDir: cfg, env: []string{"TYPESAFE_BASE_URL=" + srv.URL}})
	s.keys(`=JEV.TEST("The box was crushed", "Is this a complaint?")`, "<enter>")
	s.keys("<alt+f>", "<up>", "<up>", "<right>", "<down>", "<down>", "<down>", "<down>")
	s.waitFor("Store the TypeSafe API key")
	s.keys("<enter>", "test-key")
	s.waitFor("TypeSafe API key: ••••••••")
	if strings.Contains(s.screen(), "test-key") {
		t.Fatal("the key is on screen")
	}
	s.keys("<enter>")
	s.waitFor("Key saved and checked; it's in the test credential store")
	s.waitForLine(gridRow1, "    1    TRUE")
	data, err := os.ReadFile(filepath.Join(cfg, "012", "test-credential-store"))
	if err != nil || string(data) != "test-key" {
		t.Errorf("stored %q %v", data, err)
	}
}
