package config

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Editor returns the command that opens path in the user's editor:
// $VISUAL, then $EDITOR (either may carry arguments, e.g. "code -w"),
// else vi, or notepad on Windows.
func Editor(path string, getenv func(string) string) (*exec.Cmd, error) {
	ed := getenv("VISUAL")
	if ed == "" {
		ed = getenv("EDITOR")
	}
	if ed == "" {
		ed = "vi"
		if runtime.GOOS == "windows" {
			ed = "notepad"
		}
	}
	argv, err := SplitCommand(ed)
	if err != nil {
		return nil, err
	}
	return exec.Command(argv[0], append(argv[1:], path)...), nil
}

// EnsureFile creates the config file at path, with every option
// commented out at its default, when it doesn't exist yet.
func EnsureFile(path string) error {
	if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(DefaultFile()), 0o600)
}
