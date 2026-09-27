// Package keyring keeps the JEV API key in the operating system's
// credential store: the macOS Keychain, the Windows Credential Manager,
// or the freedesktop Secret Service on Linux and the BSDs (GNOME
// Keyring, KWallet, KeePassXC). It needs nothing beyond the standard
// library and golang.org/x/sys: on macOS and Linux it runs the system's
// own command line tools (security, secret-tool), handing them the
// secret on stdin so it never appears in a process list; on Windows it
// calls the Credential Manager API directly. See docs/contributing/architecture.md.
package keyring

import (
	"context"
	"errors"
	"sync"
)

// Service and Account name the stored item.
const (
	Service = "012"
	Account = "jev-api-key"
	Label   = "012 JEV API key"
)

// ErrNotFound means no key is stored.
var ErrNotFound = errors.New("no key in the credential store")

// Store keeps one secret.
type Store interface {
	Get(ctx context.Context) (string, error)
	Set(ctx context.Context, secret string) error
	Delete(ctx context.Context) error
	// Name is what the store is called, e.g. "macOS Keychain".
	Name() string
}

// Memory is a Store in memory, for tests.
type Memory struct {
	mu     sync.Mutex
	secret string
	Err    error // returned by every call when set
}

func (m *Memory) Get(context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return "", m.Err
	}
	if m.secret == "" {
		return "", ErrNotFound
	}
	return m.secret, nil
}

func (m *Memory) Set(_ context.Context, s string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	m.secret = s
	return nil
}

func (m *Memory) Delete(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Err != nil {
		return m.Err
	}
	if m.secret == "" {
		return ErrNotFound
	}
	m.secret = ""
	return nil
}

func (m *Memory) Name() string { return "test store" }
