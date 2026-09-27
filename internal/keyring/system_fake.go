//go:build fakekeyring

package keyring

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// System, in binaries built with -tags fakekeyring (the e2e tests), is a
// file in $XDG_CONFIG_HOME/012, which the tests point at a temporary
// directory, so they never reach a real credential store.
func System() Store { return fileStore{} }

type fileStore struct{}

func (fileStore) path() (string, error) {
	x := os.Getenv("XDG_CONFIG_HOME")
	if x == "" {
		return "", errors.New("the test credential store needs XDG_CONFIG_HOME")
	}
	return filepath.Join(x, "012", "test-credential-store"), nil
}

func (fileStore) Name() string { return "test credential store" }

func (f fileStore) Get(context.Context) (string, error) {
	p, err := f.path()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrNotFound
	}
	return strings.TrimSpace(string(b)), err
}

func (f fileStore) Set(_ context.Context, secret string) error {
	p, err := f.path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(secret), 0o600)
}

func (f fileStore) Delete(context.Context) error {
	p, err := f.path()
	if err != nil {
		return err
	}
	if err := os.Remove(p); errors.Is(err, os.ErrNotExist) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return nil
}
