package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/FelineStateMachine/012/internal/nushell/module"
)

// 012 nu --module and 012 nu --install-module: the nushell module that
// gives 012 the command `sheet` (see docs/nushell/README.md).

// nuConfigDir asks nu for its config directory, where config.nu is and
// whose scripts folder is on nushell's library path. Tests replace it.
var nuConfigDir = func() (string, error) {
	out, err := exec.Command("nu", "--no-config-file", "-c", "$nu.default-config-dir").Output()
	if err != nil {
		return "", fmt.Errorf("asking nu for its config directory: %w; name the file instead: 012 nu --install-module path/to/%s", err, module.Name)
	}
	return strings.TrimSpace(string(out)), nil
}

// isModuleCommand reports whether 012 nu's arguments ask for the module
// rather than a notebook.
func isModuleCommand(args []string) bool {
	return len(args) > 0 && (args[0] == "--module" || args[0] == "--install-module")
}

// runNuModule is 012 nu --module (print the module) or 012 nu
// --install-module [--force] [path].
func runNuModule(args []string, e env) error {
	if args[0] == "--module" {
		if len(args) > 1 {
			return usage()
		}
		_, err := fmt.Fprint(e.stdout, module.Source)
		return err
	}
	force := false
	var paths []string
	for _, a := range args[1:] {
		if a == "--force" {
			force = true
		} else {
			paths = append(paths, a)
		}
	}
	if len(paths) > 1 {
		return usage()
	}
	path, useLine, err := modulePath(paths)
	if err != nil {
		return err
	}
	wrote, err := installModule(path, force)
	if err != nil {
		return err
	}
	if wrote {
		fmt.Fprintln(e.stdout, "Wrote", path)
	} else {
		fmt.Fprintln(e.stdout, path, "is up to date")
	}
	fmt.Fprintln(e.stdout, "Add this line to config.nu (config nu opens it):")
	fmt.Fprintln(e.stdout, useLine)
	return nil
}

// modulePath is where the module goes, and the line config.nu needs to
// use it: the path given, or scripts/012.nu in nu's config directory,
// which config.nu names relative to itself.
func modulePath(given []string) (path, useLine string, err error) {
	if len(given) == 1 {
		path, err := filepath.Abs(given[0])
		if err != nil {
			return "", "", err
		}
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			path = filepath.Join(path, module.Name)
		}
		return path, fmt.Sprintf("use %q *", path), nil
	}
	dir, err := nuConfigDir()
	if err != nil {
		return "", "", err
	}
	if dir == "" {
		return "", "", errors.New("nu named no config directory")
	}
	return filepath.Join(dir, "scripts", module.Name), "use scripts/" + module.Name + " *", nil
}

// installModule writes the module to path, creating its directory. A
// file already there is replaced only with force, unless it's the same
// module; wrote is false then.
func installModule(path string, force bool) (wrote bool, err error) {
	old, err := os.ReadFile(path)
	switch {
	case err == nil && bytes.Equal(old, []byte(module.Source)):
		return false, nil
	case err == nil && !force:
		return false, fmt.Errorf("%s exists and isn't this 012's module; --force replaces it", path)
	case err != nil && !errors.Is(err, fs.ErrNotExist):
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	return true, os.WriteFile(path, []byte(module.Source), 0o644)
}
