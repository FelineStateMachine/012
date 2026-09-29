// Package install holds the install scripts served at the docs site's
// root (install.sh, install.ps1) and the tests that run install.sh
// against a local server standing in for the site's /releases/.
package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// release is a fake release: a SHA256SUMS and an archive per Unix
// platform whose 012 is a shell script printing its version.
type release struct {
	files map[string][]byte
}

func newRelease(t *testing.T, version string) release {
	t.Helper()
	r := release{files: map[string][]byte{}}
	var sums strings.Builder
	for _, goos := range []string{"darwin", "linux"} {
		for _, arch := range []string{"amd64", "arm64"} {
			name := fmt.Sprintf("012_%s_%s_%s", strings.TrimPrefix(version, "v"), goos, arch)
			bin := fmt.Sprintf("#!/bin/sh\necho 012 %s %s/%s\n", version, goos, arch)
			data := tarball(t, name, bin)
			r.files[name+".tar.gz"] = data
			sum := sha256.Sum256(data)
			fmt.Fprintf(&sums, "%s  %s.tar.gz\n", hex.EncodeToString(sum[:]), name)
		}
	}
	r.files["SHA256SUMS"] = []byte(sums.String())
	return r
}

func tarball(t *testing.T, dir, bin string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, f := range []struct {
		name string
		body string
		mode int64
	}{{dir + "/012", bin, 0o755}, {dir + "/LICENSE", "license\n", 0o644}} {
		if err := tw.WriteHeader(&tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(f.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// serve stands in for the site: /releases/<version>/<file>, with latest
// as one of the versions.
func serve(t *testing.T, releases map[string]release) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		version, file, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/releases/"), "/")
		data, found := releases[version].files[file]
		if !ok || !found {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// shells are the POSIX shells install.sh runs under: sh, and dash when
// it's installed, as Debian's /bin/sh is.
func shells(t *testing.T) []string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("install.sh is for macOS and Linux")
	}
	out := []string{"sh"}
	if _, err := exec.LookPath("dash"); err == nil {
		out = append(out, "dash")
	}
	return out
}

type run struct {
	shell string
	env   []string
	args  []string
	stdin bool // the script piped to the shell, as curl | sh does
}

func (r run) do(t *testing.T, home, base string) (string, error) {
	t.Helper()
	script, err := filepath.Abs("install.sh")
	if err != nil {
		t.Fatal(err)
	}
	var cmd *exec.Cmd
	if r.stdin {
		cmd = exec.Command(r.shell, append([]string{"-s", "--"}, r.args...)...)
		f, err := os.Open(script)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		cmd.Stdin = f
	} else {
		cmd = exec.Command(r.shell, append([]string{script}, r.args...)...)
	}
	cmd.Dir = home
	cmd.Env = append([]string{"HOME=" + home, "PATH=" + os.Getenv("PATH"), "O12_BASE_URL=" + base}, r.env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestInstallsTheLatestIntoLocalBin(t *testing.T) {
	srv := serve(t, map[string]release{"latest": newRelease(t, "v9.9.9")})
	for _, sh := range shells(t) {
		for _, stdin := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stdin=%v", sh, stdin), func(t *testing.T) {
				home := t.TempDir()
				out, err := run{shell: sh, stdin: stdin}.do(t, home, srv.URL)
				if err != nil {
					t.Fatalf("install.sh: %v\n%s", err, out)
				}
				bin := filepath.Join(home, ".local", "bin", "012")
				got, err := exec.Command(bin).Output()
				if err != nil {
					t.Fatalf("running the installed 012: %v\n%s", err, out)
				}
				if !strings.HasPrefix(string(got), "012 v9.9.9 "+runtime.GOOS+"/") {
					t.Errorf("installed 012 prints %q", got)
				}
				for _, want := range []string{"SHA256 checked", "installed 012 v9.9.9", bin, "is not on your PATH"} {
					if !strings.Contains(out, want) {
						t.Errorf("output lacks %q:\n%s", want, out)
					}
				}
			})
		}
	}
}

func TestInstallsAVersionUnderPrefix(t *testing.T) {
	srv := serve(t, map[string]release{
		"latest": newRelease(t, "v9.9.9"),
		"v1.2.3": newRelease(t, "v1.2.3"),
	})
	for _, sh := range shells(t) {
		t.Run(sh, func(t *testing.T) {
			home := t.TempDir()
			prefix := filepath.Join(home, "opt")
			out, err := run{shell: sh, env: []string{"PREFIX=" + prefix}, args: []string{"--version", "1.2.3"}}.do(t, home, srv.URL)
			if err != nil {
				t.Fatalf("install.sh: %v\n%s", err, out)
			}
			got, err := exec.Command(filepath.Join(prefix, "bin", "012")).Output()
			if err != nil || !strings.HasPrefix(string(got), "012 v1.2.3 ") {
				t.Fatalf("installed 012 prints %q (%v)\n%s", got, err, out)
			}
			if _, err := os.Stat(filepath.Join(home, ".local")); !os.IsNotExist(err) {
				t.Errorf("PREFIX set, but ~/.local was touched: %v", err)
			}
		})
	}
}

func TestRefusesAnArchiveThatFailsItsChecksum(t *testing.T) {
	rel := newRelease(t, "v9.9.9")
	for name, data := range rel.files {
		if strings.HasSuffix(name, ".tar.gz") {
			rel.files[name] = append(append([]byte{}, data...), 0)
		}
	}
	srv := serve(t, map[string]release{"latest": rel})
	for _, sh := range shells(t) {
		t.Run(sh, func(t *testing.T) {
			home := t.TempDir()
			out, err := run{shell: sh}.do(t, home, srv.URL)
			if err == nil {
				t.Fatalf("install.sh succeeded with a bad archive:\n%s", out)
			}
			if !strings.Contains(out, "nothing installed") {
				t.Errorf("output doesn't say why:\n%s", out)
			}
			if _, err := os.Stat(filepath.Join(home, ".local", "bin", "012")); !os.IsNotExist(err) {
				t.Errorf("012 was installed anyway: %v", err)
			}
		})
	}
}

func TestFailsOnAMissingReleaseOrOption(t *testing.T) {
	srv := serve(t, map[string]release{"latest": newRelease(t, "v9.9.9")})
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--version", "v0.0.1"}, "couldn't download"},
		{[]string{"--version", "next"}, "must be a release"},
		{[]string{"--frobnicate"}, "unknown option"},
	} {
		home := t.TempDir()
		out, err := run{shell: "sh", args: tc.args}.do(t, home, srv.URL)
		if err == nil || !strings.Contains(out, tc.want) {
			t.Errorf("install.sh %v: err %v, output lacks %q:\n%s", tc.args, err, tc.want, out)
		}
	}
}
