package confine

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// served builds a served directory next to a private one:
//
//	served/
//	  a.012, sub/b.csv, .env, .hidden/c.012
//	  in -> a.012            (link inside)
//	  subln -> sub           (link to a directory inside)
//	  out -> ../private/secret.012
//	  outdir -> ../private
//	  abs -> /etc/passwd style absolute link to private
//	  dangling -> ../private/nothing
//	  toenv -> .env          (link to a hidden file inside)
//	private/secret.012
func served(t *testing.T) (Root, string) {
	t.Helper()
	top := t.TempDir()
	dir, priv := filepath.Join(top, "served"), filepath.Join(top, "private")
	for _, d := range []string{dir, priv, filepath.Join(dir, "sub"), filepath.Join(dir, ".hidden")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"served/a.012", "served/sub/b.csv", "served/.env", "served/.hidden/c.012", "private/secret.012"} {
		if err := os.WriteFile(filepath.Join(top, f), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	links := map[string]string{
		"in":       "a.012",
		"subln":    "sub",
		"out":      "../private/secret.012",
		"outdir":   "../private",
		"abs":      filepath.Join(priv, "secret.012"),
		"dangling": "../private/nothing",
		"toenv":    ".env",
		"loop":     "loop",
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	r, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r, r.Dir()
}

func TestResolve(t *testing.T) {
	r, dir := served(t)
	ok := []struct{ name, want string }{
		{"a.012", "a.012"},
		{"./a.012", "a.012"},
		{"sub/b.csv", "sub/b.csv"},
		{"sub/../a.012", "a.012"},
		{"new.012", "new.012"},
		{"sub/new.csv", "sub/new.csv"},
		{"missing/dir/new.012", "missing/dir/new.012"},
		{"in", "a.012"},
		{"subln/b.csv", "sub/b.csv"},
		{".", "."},
		{"a b.012", "a b.012"},
	}
	for _, c := range ok {
		got, err := r.Resolve(c.name)
		want := filepath.Join(dir, c.want)
		if err != nil || got != want {
			t.Errorf("Resolve(%q) = %q, %v; want %q", c.name, got, err, want)
		}
	}
	escapes := []string{
		"/etc/passwd",
		filepath.Join(dir, "a.012"), // absolute, even inside
		"..",
		"../private/secret.012",
		"sub/../../private/secret.012",
		"sub/../..",
		"./../served/a.012",
		".env",
		".hidden/c.012",
		"sub/.x.csv",
		"new/.env",
		"out",
		"outdir/secret.012",
		"abs",
		"dangling",
		"toenv",
		"loop",
		`\\server\share`,
	}
	for _, name := range escapes {
		got, err := r.Resolve(name)
		if !errors.Is(err, ErrOutside) {
			t.Errorf("Resolve(%q) = %q, %v; want ErrOutside", name, got, err)
		}
	}
	if _, err := r.Resolve(""); err == nil {
		t.Error("Resolve(\"\") succeeded")
	}
}

func TestUnconfined(t *testing.T) {
	var r Root
	for _, name := range []string{"/etc/passwd", "../x", ".env", "a.012"} {
		if got, err := r.Resolve(name); err != nil || got != name {
			t.Errorf("Resolve(%q) = %q, %v; want it unchanged", name, got, err)
		}
	}
	if r.Confined() {
		t.Error("the zero Root is confined")
	}
}

func TestNew(t *testing.T) {
	if _, err := New(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Error("New of a missing directory succeeded")
	}
	f := filepath.Join(t.TempDir(), "file")
	os.WriteFile(f, nil, 0o644)
	if _, err := New(f); err == nil {
		t.Error("New of a file succeeded")
	}
	// A root reached through a link is resolved, so paths under it compare.
	top := t.TempDir()
	os.Mkdir(filepath.Join(top, "real"), 0o755)
	os.Symlink("real", filepath.Join(top, "link"))
	r, err := New(filepath.Join(top, "link"))
	if err != nil {
		t.Fatal(err)
	}
	real, _ := filepath.EvalSymlinks(filepath.Join(top, "real"))
	if r.Dir() != real {
		t.Errorf("Dir = %q, want %q", r.Dir(), real)
	}
}
