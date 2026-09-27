package keyring

import (
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// runFunc runs a program with args, writing stdin to it, and returns its
// stdout. Tests replace it to see exactly what would run.
type runFunc func(ctx context.Context, name string, args []string, stdin string) (string, error)

// exitError is a program that ran and failed.
type exitError struct {
	code   int
	stderr string
}

func (e *exitError) Error() string {
	if e.stderr != "" {
		return fmt.Sprintf("exit status %d: %s", e.code, e.stderr)
	}
	return fmt.Sprintf("exit status %d", e.code)
}

func run(ctx context.Context, name string, args []string, stdin string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	err := cmd.Run()
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return out.String(), &exitError{code: ee.ExitCode(), stderr: firstLine(errOut.String())}
	}
	return out.String(), err
}

func firstLine(s string) string {
	s, _, _ = strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(s)
}

func exitCode(err error) int {
	var ee *exitError
	if errors.As(err, &ee) {
		return ee.code
	}
	return -1
}

// keychain is the macOS Keychain, through /usr/bin/security. The secret
// is written with "security -i", which reads its command from stdin, as
// hex (-X), so it's never an argument of any process.
type keychain struct{ run runFunc }

// errItemNotFound is security's exit status for a missing item.
const errItemNotFound = 44

const securityPath = "/usr/bin/security"

func (k keychain) Name() string { return "macOS Keychain" }

func (k keychain) Get(ctx context.Context) (string, error) {
	out, err := k.run(ctx, securityPath, []string{"find-generic-password", "-s", Service, "-a", Account, "-w"}, "")
	if exitCode(err) == errItemNotFound {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("keychain: %w", err)
	}
	return strings.TrimRight(out, "\r\n"), nil
}

func (k keychain) Set(ctx context.Context, secret string) error {
	cmd := fmt.Sprintf("add-generic-password -U -s %s -a %s -l %q -X %s\n", Service, Account, Label, hex.EncodeToString([]byte(secret)))
	if _, err := k.run(ctx, securityPath, []string{"-i"}, cmd); err != nil {
		return fmt.Errorf("keychain: %w", err)
	}
	return nil
}

func (k keychain) Delete(ctx context.Context) error {
	_, err := k.run(ctx, securityPath, []string{"delete-generic-password", "-s", Service, "-a", Account}, "")
	if exitCode(err) == errItemNotFound {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("keychain: %w", err)
	}
	return nil
}

// secretService is the freedesktop Secret Service (GNOME Keyring,
// KWallet, KeePassXC), through libsecret's secret-tool, which reads the
// secret to store from stdin.
type secretService struct{ run runFunc }

func (s secretService) Name() string { return "Secret Service" }

func attrs(verb string, extra ...string) []string {
	return append(append([]string{verb}, extra...), "service", Service, "account", Account)
}

func (s secretService) Get(ctx context.Context) (string, error) {
	out, err := s.run(ctx, "secret-tool", attrs("lookup"), "")
	var ee *exitError
	switch {
	case err == nil && out != "":
		return strings.TrimRight(out, "\r\n"), nil
	case err == nil, errors.As(err, &ee) && ee.code == 1 && ee.stderr == "":
		return "", ErrNotFound // secret-tool exits 1, silently, when nothing matches
	}
	return "", s.wrap(err)
}

func (s secretService) Set(ctx context.Context, secret string) error {
	_, err := s.run(ctx, "secret-tool", attrs("store", "--label="+Label), secret)
	return s.wrap(err)
}

func (s secretService) Delete(ctx context.Context) error {
	if _, err := s.Get(ctx); err != nil {
		return err
	}
	_, err := s.run(ctx, "secret-tool", attrs("clear"), "")
	return s.wrap(err)
}

func (s secretService) wrap(err error) error {
	if errors.Is(err, exec.ErrNotFound) {
		return errors.New("secret-tool isn't installed (libsecret-tools on Debian and Ubuntu, libsecret on Fedora and Arch)")
	}
	if err != nil {
		return fmt.Errorf("secret service: %w", err)
	}
	return nil
}
