package jev

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/FelineStateMachine/012/internal/config"
	"github.com/FelineStateMachine/012/internal/keyring"
)

// The API key is never in the config file or on a command line. It comes
// from, in order:
//
//  1. TYPESAFE_API_KEY in the environment;
//  2. the OS credential store, where `012 config set-key` and File >
//     Settings > JEV API key put it;
//  3. jev-api-key-command, a command that prints it (a password
//     manager's CLI), run without a shell.

// KeyEnv is the environment variable that holds the key.
const KeyEnv = "TYPESAFE_API_KEY"

// KeyCommandTimeout bounds jev-api-key-command.
const KeyCommandTimeout = 10 * time.Second

// KeySources are the places ResolveKey looks.
type KeySources struct {
	Getenv  func(string) string
	Store   keyring.Store // nil skips the credential store
	Command string        // jev-api-key-command; empty skips it
	Timeout time.Duration // for Command; KeyCommandTimeout when zero
	// run runs the command; tests replace it. It gets the program and
	// its arguments exactly as they'd be executed.
	run func(ctx context.Context, argv []string) ([]byte, error)
}

// ResolveKey finds the API key. from says where it came from:
// "TYPESAFE_API_KEY", the store's name, or "jev-api-key-command". With no
// key anywhere it returns "", "", nil. A store or command that fails is
// skipped, and its error is returned only if no later source has a key.
func ResolveKey(ctx context.Context, s KeySources) (key, from string, err error) {
	if s.Getenv != nil {
		if k := strings.TrimSpace(s.Getenv(KeyEnv)); k != "" {
			return k, KeyEnv, nil
		}
	}
	var errs []error
	if s.Store != nil {
		k, err := s.Store.Get(ctx)
		switch {
		case err == nil && strings.TrimSpace(k) != "":
			return strings.TrimSpace(k), s.Store.Name(), nil
		case err != nil && !errors.Is(err, keyring.ErrNotFound):
			errs = append(errs, err)
		}
	}
	if s.Command != "" {
		k, err := s.runCommand(ctx)
		if err == nil {
			return k, "jev-api-key-command", nil
		}
		errs = append(errs, err)
	}
	return "", "", errors.Join(errs...)
}

// runCommand runs jev-api-key-command and returns the first line it
// prints. The command is split into words (quotes group, nothing is
// expanded) and run directly, not through a shell.
func (s KeySources) runCommand(ctx context.Context) (string, error) {
	argv, err := config.SplitCommand(s.Command)
	if err != nil {
		return "", fmt.Errorf("jev-api-key-command: %w", err)
	}
	timeout := s.Timeout
	if timeout <= 0 {
		timeout = KeyCommandTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	run := s.run
	if run == nil {
		run = execCommand
	}
	out, err := run(ctx, argv)
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return "", fmt.Errorf("jev-api-key-command: %s didn't finish in %s", argv[0], timeout)
	case err != nil:
		return "", fmt.Errorf("jev-api-key-command: %s: %w", argv[0], err)
	}
	line, _, _ := strings.Cut(string(out), "\n")
	if line = strings.TrimSpace(line); line == "" {
		return "", fmt.Errorf("jev-api-key-command: %s printed nothing", argv[0])
	}
	return line, nil
}

// execCommand runs argv with no stdin, returning stdout. What the command
// writes to stderr is reported by its first line only.
func execCommand(ctx context.Context, argv []string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut
	cmd.WaitDelay = time.Second // don't wait on children holding stdout open
	if err := cmd.Run(); err != nil {
		if msg, _, _ := strings.Cut(strings.TrimSpace(errOut.String()), "\n"); msg != "" {
			return nil, fmt.Errorf("%w: %s", err, msg)
		}
		return nil, err
	}
	return out.Bytes(), nil
}

// DotEnvHasKey reports whether dir/.env sets TYPESAFE_API_KEY. It reads
// only the variable names: the key itself is never read into memory
// past the scanner's line, kept or shown. 012 no longer reads .env files
// (a .env beside a downloaded sheet could send the key elsewhere); this
// is only to suggest `012 config set-key` to people who used one.
func DotEnvHasKey(dir string) bool {
	f, err := os.Open(filepath.Join(dir, ".env"))
	if err != nil {
		return false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(sc.Text()), "export "))
		if name, _, ok := strings.Cut(line, "="); ok && strings.TrimSpace(name) == KeyEnv {
			return true
		}
	}
	return false
}
