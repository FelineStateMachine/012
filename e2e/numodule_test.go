package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The nushell module (012 nu --install-module, then `sheet` in nu): nu
// runs as a pipeline stage in the terminal, with 012 on its PATH, the
// module installed where 012 puts it and used by the line 012 prints
// for config.nu.

// startSheet installs the module in a temporary nu config directory and
// runs script in nu with it, standard output captured.
func startSheet(t *testing.T, startsOn, script string) *session {
	t.Helper()
	needNu(t)
	dir := t.TempDir()
	xdg := filepath.Join(dir, ".config") // the session's own (configEnv)
	install := exec.Command(binPath, "nu", "--install-module")
	install.Env = append(os.Environ(), "XDG_CONFIG_HOME="+xdg)
	out, err := install.CombinedOutput()
	if err != nil {
		t.Fatalf("installing the module: %v\n%s", err, out)
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	config := filepath.Join(xdg, "nushell", "config.nu")
	if err := os.WriteFile(config, []byte(lines[len(lines)-1]+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := "PATH=" + filepath.Dir(binPath) + string(os.PathListSeparator) + os.Getenv("PATH")
	return startWith(t, options{dir: dir, piped: true, program: "nu", startsOn: startsOn, env: []string{path}},
		"--config", config, "-c", script)
}

const sizesNu = "[[name size]; [a 2kb] [b 10b] [c 5mb]]"

// A table goes through sheet: a selection is sent back, and nu gets it
// with its types, file sizes as file sizes.
func TestSheetSendsSelection(t *testing.T) {
	s := startSheet(t, "5.0 MB", "let t = "+sizesNu+" | sheet; print ($t | get size | describe); $t | to nuon")
	s.keys("<shift+down>", "<shift+down>", "<shift+right>")
	s.waitFor("send A1:B3 as NUON")
	s.keys("<ctrl+q>")
	s.waitFor("Send A1:B3 as NUON?")
	s.keys("<enter>")
	out, code := s.finish()
	if want := "list<filesize>\n[[name, size]; [a, 2000b], [b, 10b]]\n"; code != 0 || out != want {
		t.Errorf("exit %d, stdout %q, want %q", code, out, want)
	}
}

// Quitting without sending raises sheet's error, with 012's reason.
func TestSheetNotSent(t *testing.T) {
	s := startSheet(t, "5.0 MB", "try { "+sizesNu+" | sheet } catch {|e| print $'caught: ($e.msg)' }")
	s.keys("<ctrl+q>", "d")
	out, code := s.finish()
	if want := "caught: quit without sending anything to the pipeline\n"; code != 0 || out != want {
		t.Errorf("exit %d, stdout %q, want %q", code, out, want)
	}
}

// sheet view shows the table and returns nothing.
func TestSheetView(t *testing.T) {
	s := startSheet(t, "5.0 MB", sizesNu+" | sheet view | describe")
	s.keys("<ctrl+q>")
	s.waitFor("You have unsaved changes.")
	s.keys("d")
	out, code := s.finish()
	if code != 0 || out != "nothing\n" {
		t.Errorf("exit %d, stdout %q", code, out)
	}
}
