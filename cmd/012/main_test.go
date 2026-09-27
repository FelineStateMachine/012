package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/keyring"
)

// testEnv points the config at a temporary directory and the credential
// store at memory, so tests never touch the user's config or keychain.
func testEnv(t *testing.T, vars map[string]string) (env, *bytes.Buffer, *keyring.Memory) {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	out := &bytes.Buffer{}
	store := &keyring.Memory{}
	return env{
		getenv: func(k string) string { return vars[k] },
		keys:   store, stdin: strings.NewReader(""), stdout: out, stderr: out,
		readKey: func(string) (string, error) { t.Fatal("read from a terminal"); return "", nil },
		runTUI:  func(tea.Model) error { return nil },
	}, out, store
}

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path, _ := config.DefaultPath()
	os.MkdirAll(filepath.Dir(path), 0o700)
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigCommands(t *testing.T) {
	e, out, _ := testEnv(t, map[string]string{"O12_THEME": "Dracula"})
	path := writeConfig(t, "theme = Nord\nlog-level = warn\nwhat = 1\n")
	if err := run([]string{"config", "path"}, e); err != nil || strings.TrimSpace(out.String()) != path {
		t.Errorf("path: %q %v", out, err)
	}
	out.Reset()
	if err := run([]string{"config"}, e); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"theme = Dracula", "# $O12_THEME", "log-level = warn", path + ":2", "chart-images = true", "# default",
		`warning: ` + path + `:3: unknown key "what"`, "# JEV API key: not set; run 012 config set-key"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("012 config lacks %q:\n%s", want, out)
		}
	}
	out.Reset()
	if err := run([]string{"config", "default"}, e); err != nil || !strings.Contains(out.String(), "# theme = terminal") {
		t.Errorf("default: %v\n%s", err, out)
	}
	out.Reset()
	if err := run([]string{"config", "themes"}, e); err != nil || !strings.Contains(out.String(), "* Dracula") || !strings.Contains(out.String(), "  terminal") {
		t.Errorf("themes: %v\n%s", err, out)
	}
	if err := run([]string{"config", "bogus"}, e); err == nil {
		t.Error("unknown subcommand")
	}
}

func TestSetAndDeleteKey(t *testing.T) {
	const key = "ts-live-abc123"
	e, out, store := testEnv(t, nil)
	e.stdin = strings.NewReader(key + "\n")
	if err := run([]string{"config", "set-key"}, e); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(context.Background()); got != key {
		t.Errorf("stored %q", got)
	}
	if strings.Contains(out.String(), key) {
		t.Errorf("the key was printed: %s", out)
	}
	out.Reset()
	run([]string{"config"}, e)
	if !strings.Contains(out.String(), "JEV API key: stored in the test store") || strings.Contains(out.String(), key) {
		t.Errorf("status: %s", out)
	}
	out.Reset()
	if err := run([]string{"config", "delete-key"}, e); err != nil || !strings.Contains(out.String(), "Deleted") {
		t.Errorf("delete: %v %s", err, out)
	}
	e.stdin = strings.NewReader("")
	if err := run([]string{"config", "set-key"}, e); err == nil {
		t.Error("empty key accepted")
	}

	// A terminal reads the key without echo.
	e.isTTY = true
	e.readKey = func(string) (string, error) { return "from-tty", nil }
	if err := run([]string{"config", "set-key"}, e); err != nil {
		t.Fatal(err)
	}
	if got, _ := store.Get(context.Background()); got != "from-tty" {
		t.Errorf("stored %q", got)
	}

	writeConfig(t, "jev-credential-store = false\n")
	if err := run([]string{"config", "set-key"}, e); err == nil || !strings.Contains(err.Error(), "jev-credential-store is off") {
		t.Errorf("store off: %v", err)
	}
}

func TestStartJEV(t *testing.T) {
	e, _, store := testEnv(t, nil)
	c, _ := loadConfig(e, nil)
	if client, notes := startJEV(c, e, store); client != nil || len(notes) != 0 {
		t.Errorf("no key: %v %v", client, notes)
	}
	store.Set(context.Background(), "k")
	if client, notes := startJEV(c, e, store); client == nil || len(notes) != 0 {
		t.Errorf("stored key: %v %v", client, notes)
	}

	// A .env with a key is never read, but noticed.
	store.Delete(context.Background())
	dir := t.TempDir()
	t.Chdir(dir)
	os.WriteFile(".env", []byte("TYPESAFE_API_KEY=should-not-be-read\nTYPESAFE_BASE_URL=http://evil.example\n"), 0o600)
	client, notes := startJEV(c, e, store)
	if client != nil || len(notes) != 1 || !strings.Contains(notes[0], "012 config set-key") || strings.Contains(notes[0], "should-not") {
		t.Errorf(".env: %v %v", client, notes)
	}

	// The base URL only comes from the config or environment, and must
	// be https.
	e.getenv = func(k string) string {
		return map[string]string{"TYPESAFE_API_KEY": "k", "TYPESAFE_BASE_URL": "http://evil.example"}[k]
	}
	c, _ = loadConfig(e, nil)
	if c.String("jev-base-url") != "" || len(c.Warnings) == 0 {
		t.Errorf("http base URL accepted: %q", c.String("jev-base-url"))
	}
}

func TestTelemetryConfig(t *testing.T) {
	e, _, _ := testEnv(t, map[string]string{"O12_LOG": "env.jsonl", "OTEL_EXPORTER_OTLP_ENDPOINT": "http://env:4318"})
	writeConfig(t, "log-file = file.jsonl\nlog-level = debug\n")
	flags, rest, err := config.ParseFlags([]string{"--log", "flag.jsonl", "b.csv"})
	if err != nil || len(rest) != 1 {
		t.Fatal(rest, err)
	}
	c, _ := loadConfig(e, flags)
	tc := telemetryConfig(c)
	if tc.LogPath != "flag.jsonl" || tc.OTLP.Endpoint != "http://env:4318" || tc.Level.String() != "DEBUG" {
		t.Errorf("got %+v", tc)
	}
}

func TestRunWarnsOnConfig(t *testing.T) {
	e, _, _ := testEnv(t, nil)
	writeConfig(t, "bogus = 1\ntheme = No Such Theme\n")
	if err := run(nil, e); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"a", "b"}, e); err == nil {
		t.Error("two files")
	}
}
