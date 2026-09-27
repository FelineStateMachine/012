package jev

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/keyring"
)

func env(vals map[string]string) func(string) string {
	return func(k string) string { return vals[k] }
}

func storeWith(key string) *keyring.Memory {
	m := &keyring.Memory{}
	if key != "" {
		m.Set(context.Background(), key)
	}
	return m
}

// fakeCommand records the argv it's asked to run and prints out.
type fakeCommand struct {
	argv [][]string
	out  string
	err  error
}

func (f *fakeCommand) run(_ context.Context, argv []string) ([]byte, error) {
	f.argv = append(f.argv, argv)
	return []byte(f.out), f.err
}

func TestResolveKeyOrder(t *testing.T) {
	ctx := context.Background()
	cmd := &fakeCommand{out: "from-command\nsecond line\n"}
	s := KeySources{
		Getenv:  env(map[string]string{KeyEnv: " from-env "}),
		Store:   storeWith("from-store"),
		Command: "pass show typesafe",
		run:     cmd.run,
	}
	if k, from, err := ResolveKey(ctx, s); k != "from-env" || from != KeyEnv || err != nil {
		t.Errorf("the environment comes first: %q %q %v", k, from, err)
	}
	s.Getenv = env(nil)
	if k, from, _ := ResolveKey(ctx, s); k != "from-store" || from != "test store" {
		t.Errorf("then the credential store: %q %q", k, from)
	}
	if len(cmd.argv) != 0 {
		t.Error("the command ran although the store had a key")
	}
	s.Store = storeWith("")
	if k, from, _ := ResolveKey(ctx, s); k != "from-command" || from != "jev-api-key-command" {
		t.Errorf("then the command's first line: %q %q", k, from)
	}
	if !slices.Equal(cmd.argv[0], []string{"pass", "show", "typesafe"}) {
		t.Errorf("argv %q", cmd.argv[0])
	}
	s.Command = ""
	if k, from, err := ResolveKey(ctx, s); k != "" || from != "" || err != nil {
		t.Errorf("no key anywhere: %q %q %v", k, from, err)
	}
}

func TestResolveKeyFailures(t *testing.T) {
	ctx := context.Background()
	broken := &keyring.Memory{Err: errors.New("keychain locked")}
	cmd := &fakeCommand{out: "k"}
	k, _, err := ResolveKey(ctx, KeySources{Store: broken, Command: "get-key", run: cmd.run})
	if k != "k" || err != nil {
		t.Errorf("a failing store falls through to the command: %q %v", k, err)
	}
	cmd = &fakeCommand{err: errors.New("exit status 1")}
	_, _, err = ResolveKey(ctx, KeySources{Store: broken, Command: "get-key", run: cmd.run})
	if err == nil || !strings.Contains(err.Error(), "keychain locked") || !strings.Contains(err.Error(), "get-key: exit status 1") {
		t.Errorf("both failures reported: %v", err)
	}
	cmd = &fakeCommand{out: "\n"}
	if _, _, err := ResolveKey(ctx, KeySources{Command: "get-key", run: cmd.run}); err == nil || !strings.Contains(err.Error(), "printed nothing") {
		t.Errorf("empty output: %v", err)
	}
	if _, _, err := ResolveKey(ctx, KeySources{Command: "get 'key"}); err == nil {
		t.Error("an unclosed quote ran")
	}
}

// The command runs without a shell: quotes group words, and nothing is
// expanded or piped.
func TestKeyCommandNoShell(t *testing.T) {
	cmd := &fakeCommand{out: "k"}
	ResolveKey(context.Background(), KeySources{Command: `op read "op://Private/Type Safe/credential" $HOME;rm`, run: cmd.run})
	want := []string{"op", "read", "op://Private/Type Safe/credential", "$HOME;rm"}
	if !slices.Equal(cmd.argv[0], want) {
		t.Errorf("argv %q, want %q", cmd.argv[0], want)
	}
}

func TestKeyCommandRuns(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh")
	}
	k, _, err := ResolveKey(context.Background(), KeySources{Command: `sh -c 'printf "real-key\n"'`})
	if k != "real-key" || err != nil {
		t.Errorf("%q %v", k, err)
	}
}

func TestKeyCommandTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sleep")
	}
	start := time.Now()
	_, _, err := ResolveKey(context.Background(), KeySources{Command: "sleep 5", Timeout: 100 * time.Millisecond})
	if err == nil || !strings.Contains(err.Error(), "didn't finish in 100ms") {
		t.Errorf("timeout: %v", err)
	}
	if d := time.Since(start); d > 3*time.Second {
		t.Errorf("took %s", d)
	}
}

func TestDotEnvHasKey(t *testing.T) {
	dir := t.TempDir()
	if DotEnvHasKey(dir) {
		t.Error("no .env")
	}
	os.WriteFile(filepath.Join(dir, ".env"), []byte("# x\nTYPESAFE_BASE_URL=https://a\n"), 0o600)
	if DotEnvHasKey(dir) {
		t.Error("no key in .env")
	}
	os.WriteFile(filepath.Join(dir, ".env"), []byte("export TYPESAFE_API_KEY=\"x\"\n"), 0o600)
	if !DotEnvHasKey(dir) {
		t.Error("key in .env not noticed")
	}
}

func TestNewClientChecksBaseURL(t *testing.T) {
	if _, err := NewClient(Config{APIKey: "k", BaseURL: "http://evil.example"}); err == nil {
		t.Error("http base URL accepted")
	}
	if _, err := NewClient(Config{APIKey: "k", BaseURL: "http://127.0.0.1:8799"}); err != nil {
		t.Errorf("localhost: %v", err)
	}
}
