// Package serve runs 012 as an SSH server: each session gets its own
// spreadsheet over its own terminal, confined to one directory.
//
// It is built for one person reaching their own files from elsewhere,
// not for sharing: public-key auth against an authorized_keys file only,
// on the loopback address unless told otherwise, with no port
// forwarding, no subsystems, and no commands: an exec request is only
// ever a file name to open. See docs/terminal/ssh.md.
package serve

import (
	"github.com/FelineStateMachine/012/internal/config"

	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Options are 012 serve's settings, one field per flag. Defaults gives
// their default values; cmd/012 parses flags into them.
type Options struct {
	// Dir is the served directory: sessions open, save, import and
	// download only inside it.
	Dir string
	// Listen is the TCP address to listen on.
	Listen string
	// AuthorizedKeys is the file of public keys allowed to connect, in
	// OpenSSH's authorized_keys format. It is read again on every login.
	AuthorizedKeys string
	// HostKey is the server's private key file, an ed25519 key generated
	// there (0600, in a 0700 directory) when missing.
	HostKey string
	// IdleTimeout ends a session that has had no input for this long; 0
	// never does.
	IdleTimeout time.Duration
	// MaxSessions is how many sessions may run at once; more are turned
	// away.
	MaxSessions int
	// Share is whether sessions opening the same file share its
	// workbook: "edit" (everyone edits), "view" (one writes, the others
	// follow) or "off" (a copy each); "" is off.
	Share string
}

// Default values.
const (
	DefaultListen      = "127.0.0.1:2312"
	DefaultIdleTimeout = 30 * time.Minute
	DefaultMaxSessions = 8
)

// Defaults returns the default options: the current directory, served on
// the loopback address, keys from ~/.ssh/authorized_keys and the host key
// in 012's config directory.
func Defaults() Options {
	o := Options{Dir: ".", Listen: DefaultListen, IdleTimeout: DefaultIdleTimeout, MaxSessions: DefaultMaxSessions, Share: "edit"}
	if home, err := os.UserHomeDir(); err == nil {
		o.AuthorizedKeys = filepath.Join(home, ".ssh", "authorized_keys")
	}
	if dir, err := config.Dir(); err == nil {
		o.HostKey = filepath.Join(dir, "ssh_host_ed25519_key")
	}
	return o
}

func (o Options) validate() error {
	switch {
	case o.Listen == "":
		return errors.New("no address to listen on")
	case o.AuthorizedKeys == "":
		return errors.New("no authorized_keys file")
	case o.HostKey == "":
		return errors.New("no host key file")
	case o.MaxSessions < 1:
		return fmt.Errorf("max sessions is %d, it must be at least 1", o.MaxSessions)
	case o.IdleTimeout < 0:
		return errors.New("the idle timeout can't be negative")
	case o.Share != "" && o.Share != "edit" && o.Share != "view" && o.Share != "off":
		return fmt.Errorf("share is %q: edit, view or off", o.Share)
	}
	return nil
}
