package serve

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// hostKey loads the server's private key from path, generating an
// ed25519 key there first if there's none. A key others can read or
// write is refused, as sshd refuses one.
func hostKey(path string) (gossh.Signer, error) {
	st, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		if err := generateHostKey(path); err != nil {
			return nil, fmt.Errorf("generating the host key: %w", err)
		}
		st, err = os.Stat(path)
	}
	if err != nil {
		return nil, err
	}
	if st.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("host key %s is accessible by others (%v); chmod 600 it", path, st.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	s, err := gossh.ParsePrivateKey(data)
	if err != nil {
		return nil, fmt.Errorf("host key %s: %w", path, err)
	}
	return s, nil
}

// generateHostKey writes a new ed25519 key to path (0600) and its public
// half beside it, creating the directory (0700) if needed.
func generateHostKey(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return err
	}
	block, err := gossh.MarshalPrivateKey(priv, "012 host key")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(pem.EncodeToMemory(block))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return err
	}
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		return err
	}
	return os.WriteFile(path+".pub", gossh.MarshalAuthorizedKey(sshPub), 0o644)
}

// safeOptions are the authorized_keys options a key may carry and still
// be accepted: they only turn off things 012 never offers. Keys with any
// other option (command=, from=, no-pty, expiry-time= and so on) are
// skipped, since 012 can't honor the restriction.
var safeOptions = map[string]bool{
	"no-port-forwarding":  true,
	"no-agent-forwarding": true,
	"no-x11-forwarding":   true,
	"no-user-rc":          true,
	"restrict":            true,
	"pty":                 true,
}

// authorizedKeys reads the keys allowed to log in from path, in
// OpenSSH's format. It fails if the file can be written by others, as
// sshd's StrictModes does, or if a line doesn't parse. skipped counts
// keys left out for their options.
func authorizedKeys(path string) (keys []gossh.PublicKey, skipped int, err error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, 0, err
	}
	if !st.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("%s is not a regular file", path)
	}
	if st.Mode().Perm()&0o022 != 0 {
		return nil, 0, fmt.Errorf("%s can be written by others (%v); chmod go-w it", path, st.Mode().Perm())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, err
	}
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(nil, 1<<20)
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 || line[0] == '#' {
			continue
		}
		key, _, opts, _, err := gossh.ParseAuthorizedKey(line)
		if err != nil {
			return nil, 0, fmt.Errorf("%s:%d: %w", path, n, err)
		}
		if !acceptable(opts) {
			skipped++
			continue
		}
		keys = append(keys, key)
	}
	return keys, skipped, sc.Err()
}

// acceptable reports whether a key with these authorized_keys options
// may log in to 012.
func acceptable(opts []string) bool {
	restricted, pty := false, false
	for _, o := range opts {
		o = strings.ToLower(o)
		if !safeOptions[o] {
			return false
		}
		restricted = restricted || o == "restrict"
		pty = pty || o == "pty"
	}
	// restrict takes the terminal away unless pty gives it back, and 012
	// needs one.
	return !restricted || pty
}

// fingerprint is a key's SHA256 fingerprint, as ssh-keygen -l shows it.
func fingerprint(k gossh.PublicKey) string {
	if k == nil {
		return ""
	}
	return gossh.FingerprintSHA256(k)
}
