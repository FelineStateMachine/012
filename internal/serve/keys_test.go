package serve

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

func TestAuthorizedKeys(t *testing.T) {
	keys := []gossh.Signer{newKey(t), newKey(t), newKey(t), newKey(t), newKey(t), newKey(t)}
	line := func(i int, opts string) string {
		s := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(keys[i].PublicKey())))
		if opts != "" {
			s = opts + " " + s
		}
		return s + " user@host\n"
	}
	content := "# comment\n\n" +
		line(0, "") +
		line(1, `command="rrsync /backup"`) + // a restricted backup key: not for 012
		line(2, `from="10.0.0.0/8"`) + // 012 can't check where it comes from
		line(3, "restrict") + // no terminal
		line(4, "restrict,pty") +
		line(5, "no-port-forwarding,no-agent-forwarding")
	path := filepath.Join(t.TempDir(), "authorized_keys")
	os.WriteFile(path, []byte(content), 0o600)
	got, skipped, err := authorizedKeys(path)
	if err != nil {
		t.Fatal(err)
	}
	want := []int{0, 4, 5}
	if len(got) != len(want) || skipped != 3 {
		t.Fatalf("got %d keys, skipped %d; want %d and 3", len(got), skipped, len(want))
	}
	for i, k := range want {
		if fingerprint(got[i]) != fingerprint(keys[k].PublicKey()) {
			t.Errorf("key %d is not key %d", i, k)
		}
	}

	// Writable by others: refused, as sshd's StrictModes does.
	os.Chmod(path, 0o620)
	if _, _, err := authorizedKeys(path); err == nil || !strings.Contains(err.Error(), "written by others") {
		t.Errorf("group-writable file: %v", err)
	}
	os.Chmod(path, 0o644)
	if _, _, err := authorizedKeys(path); err != nil {
		t.Errorf("0644 file: %v", err)
	}

	// A line that doesn't parse names its line.
	os.WriteFile(path, []byte(line(0, "")+"ssh-ed25519 notbase64!\n"), 0o600)
	if _, _, err := authorizedKeys(path); err == nil || !strings.Contains(err.Error(), ":2:") {
		t.Errorf("bad line: %v", err)
	}
	if _, _, err := authorizedKeys(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("missing file accepted")
	}
	if _, _, err := authorizedKeys(t.TempDir()); err == nil {
		t.Error("a directory accepted")
	}
}

func TestHostKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config", "012")
	path := filepath.Join(dir, "ssh_host_ed25519_key")
	s1, err := hostKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if s1.PublicKey().Type() != gossh.KeyAlgoED25519 {
		t.Errorf("host key type %s", s1.PublicKey().Type())
	}
	for p, want := range map[string]os.FileMode{path: 0o600, dir: 0o700 | os.ModeDir, path + ".pub": 0o644} {
		if st, err := os.Stat(p); err != nil || st.Mode() != want {
			t.Errorf("%s: mode %v, %v; want %v", filepath.Base(p), st.Mode(), err, want)
		}
	}
	// Loaded again, not regenerated.
	s2, err := hostKey(path)
	if err != nil || fingerprint(s2.PublicKey()) != fingerprint(s1.PublicKey()) {
		t.Errorf("reloaded host key differs: %v", err)
	}
	os.Chmod(path, 0o644)
	if _, err := hostKey(path); err == nil || !strings.Contains(err.Error(), "chmod 600") {
		t.Errorf("readable host key: %v", err)
	}
}

func TestNewChecks(t *testing.T) {
	key := newKey(t)
	top := t.TempDir()
	ak := filepath.Join(top, "authorized_keys")
	os.WriteFile(ak, gossh.MarshalAuthorizedKey(key.PublicKey()), 0o600)
	empty := filepath.Join(top, "empty")
	os.WriteFile(empty, []byte("# nobody\n"), 0o600)
	base := Options{Dir: top, Listen: "127.0.0.1:0", AuthorizedKeys: ak, HostKey: filepath.Join(top, "hk"), MaxSessions: 1}
	cases := map[string]func(*Options){
		"missing dir":        func(o *Options) { o.Dir = filepath.Join(top, "nope") },
		"no keys":            func(o *Options) { o.AuthorizedKeys = empty },
		"missing keys":       func(o *Options) { o.AuthorizedKeys = filepath.Join(top, "nope") },
		"no listen":          func(o *Options) { o.Listen = "" },
		"no sessions":        func(o *Options) { o.MaxSessions = 0 },
		"negative idle":      func(o *Options) { o.IdleTimeout = -1 },
		"no host key":        func(o *Options) { o.HostKey = "" },
		"no authorized keys": func(o *Options) { o.AuthorizedKeys = "" },
	}
	for name, change := range cases {
		o := base
		change(&o)
		if _, err := New(o, Deps{}); err == nil {
			t.Errorf("%s: New succeeded", name)
		}
	}
	if _, err := New(base, Deps{}); err != nil {
		t.Errorf("valid options: %v", err)
	}
	d := Defaults()
	if d.Listen != "127.0.0.1:2312" || !strings.HasSuffix(d.AuthorizedKeys, filepath.Join(".ssh", "authorized_keys")) ||
		!strings.HasSuffix(d.HostKey, filepath.Join("012", "ssh_host_ed25519_key")) || d.IdleTimeout <= 0 || d.MaxSessions <= 0 {
		t.Errorf("Defaults() = %+v", d)
	}
}
