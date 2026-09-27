package keyring

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
)

// These tests never run security or secret-tool: a fake runner records
// what would run, so the real keychain is never touched.

const secret = "ts-secret-key-123"

type call struct {
	name  string
	args  []string
	stdin string
}

type fakeRunner struct {
	calls []call
	out   string
	err   error
}

func (f *fakeRunner) run(_ context.Context, name string, args []string, stdin string) (string, error) {
	f.calls = append(f.calls, call{name, args, stdin})
	return f.out, f.err
}

// noSecretInArgs fails if the secret is in any argument, where other
// users could see it in the process list.
func noSecretInArgs(t *testing.T, calls []call) {
	t.Helper()
	for _, c := range calls {
		for _, a := range c.args {
			if strings.Contains(a, secret) || strings.Contains(a, hex.EncodeToString([]byte(secret))) {
				t.Errorf("secret in argv: %s %q", c.name, c.args)
			}
		}
	}
}

func TestKeychain(t *testing.T) {
	ctx := context.Background()
	f := &fakeRunner{}
	k := keychain{run: f.run}
	if err := k.Set(ctx, secret); err != nil {
		t.Fatal(err)
	}
	noSecretInArgs(t, f.calls)
	c := f.calls[0]
	if c.name != "/usr/bin/security" || len(c.args) != 1 || c.args[0] != "-i" {
		t.Errorf("set ran %s %q", c.name, c.args)
	}
	if !strings.Contains(c.stdin, "add-generic-password -U -s 012 -a jev-api-key") || !strings.Contains(c.stdin, "-X "+hex.EncodeToString([]byte(secret))) {
		t.Errorf("set wrote %q", c.stdin)
	}

	f.out = secret + "\n"
	if got, err := k.Get(ctx); err != nil || got != secret {
		t.Errorf("get: %q %v", got, err)
	}
	f.out, f.err = "", &exitError{code: 44, stderr: "The specified item could not be found in the keychain."}
	if _, err := k.Get(ctx); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing item: %v", err)
	}
	if err := k.Delete(ctx); !errors.Is(err, ErrNotFound) {
		t.Errorf("delete missing: %v", err)
	}
	f.err = &exitError{code: 51, stderr: "user interaction is not allowed"}
	if _, err := k.Get(ctx); err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), "interaction") {
		t.Errorf("other failure: %v", err)
	}
}

func TestSecretService(t *testing.T) {
	ctx := context.Background()
	f := &fakeRunner{}
	s := secretService{run: f.run}
	if err := s.Set(ctx, secret); err != nil {
		t.Fatal(err)
	}
	noSecretInArgs(t, f.calls)
	c := f.calls[0]
	if c.name != "secret-tool" || c.args[0] != "store" || c.stdin != secret {
		t.Errorf("set ran %s %q with stdin %q", c.name, c.args, c.stdin)
	}
	if strings.Join(c.args[2:], " ") != "service 012 account jev-api-key" {
		t.Errorf("attributes %q", c.args)
	}

	f.out = secret
	if got, err := s.Get(ctx); err != nil || got != secret {
		t.Errorf("get: %q %v", got, err)
	}
	f.out, f.err = "", &exitError{code: 1}
	if _, err := s.Get(ctx); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	f.err = &exitError{code: 1, stderr: "Cannot autolaunch D-Bus without X11 $DISPLAY"}
	if _, err := s.Get(ctx); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("no D-Bus should be an error, not a missing key: %v", err)
	}
}

func TestMemory(t *testing.T) {
	ctx := context.Background()
	m := &Memory{}
	if _, err := m.Get(ctx); !errors.Is(err, ErrNotFound) {
		t.Error(err)
	}
	m.Set(ctx, "k")
	if got, _ := m.Get(ctx); got != "k" {
		t.Error(got)
	}
	if err := m.Delete(ctx); err != nil {
		t.Error(err)
	}
}
