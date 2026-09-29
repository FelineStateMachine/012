package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/nushell/module"
)

func TestNuModulePrints(t *testing.T) {
	e, out, _ := testEnv(t, nil)
	if err := run([]string{"nu", "--module"}, e); err != nil {
		t.Fatal(err)
	}
	if out.String() != module.Source || !strings.Contains(out.String(), "export def sheet [") {
		t.Errorf("printed:\n%s", out)
	}
}

// fakeNuConfig points 012 nu --install-module at a temporary nu config
// directory.
func fakeNuConfig(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "nushell")
	old := nuConfigDir
	nuConfigDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { nuConfigDir = old })
	return dir
}

func TestNuInstallModule(t *testing.T) {
	e, out, _ := testEnv(t, nil)
	dir := fakeNuConfig(t)
	path := filepath.Join(dir, "scripts", "012.nu")
	if err := run([]string{"nu", "--install-module"}, e); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != module.Source {
		t.Fatalf("installed %q, %v", got, err)
	}
	if want := "Wrote " + path + "\nAdd this line to config.nu (config nu opens it):\nuse scripts/012.nu *\n"; out.String() != want {
		t.Errorf("printed %q, want %q", out, want)
	}

	// Again: the same module is left as it is.
	out.Reset()
	if err := run([]string{"nu", "--install-module"}, e); err != nil || !strings.HasPrefix(out.String(), path+" is up to date\n") {
		t.Errorf("reinstalling: %v, %q", err, out)
	}

	// A different file there stays unless --force.
	os.WriteFile(path, []byte("# mine\n"), 0o644)
	err := run([]string{"nu", "--install-module"}, e)
	if err == nil || !strings.Contains(err.Error(), "--force replaces it") {
		t.Errorf("overwrote without --force: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "# mine\n" {
		t.Errorf("file changed: %q", got)
	}
	if err := run([]string{"nu", "--install-module", "--force"}, e); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != module.Source {
		t.Errorf("--force didn't replace it: %q", got)
	}
}

// An explicit path is used as given, or as the folder to put 012.nu in,
// and config.nu names it in full.
func TestNuInstallModuleAt(t *testing.T) {
	e, out, _ := testEnv(t, nil)
	old := nuConfigDir
	nuConfigDir = func() (string, error) { t.Fatal("asked nu"); return "", nil }
	t.Cleanup(func() { nuConfigDir = old })
	dir := t.TempDir()
	file := filepath.Join(dir, "lib", "sheet.nu")
	if err := run([]string{"nu", "--install-module", file}, e); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(file); string(got) != module.Source {
		t.Errorf("installed %q", got)
	}
	if !strings.HasSuffix(out.String(), "use \""+file+"\" *\n") {
		t.Errorf("printed %q", out)
	}
	if err := run([]string{"nu", "--install-module", dir}, e); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "012.nu")); err != nil {
		t.Error(err)
	}
	if err := run([]string{"nu", "--module", "extra"}, e); err == nil {
		t.Error("--module took an argument")
	}
}
