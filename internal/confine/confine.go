// Package confine maps the file names a user types to paths on disk,
// keeping them inside one directory when 012 serves sessions over SSH.
//
// A Root's zero value confines nothing: names are used as typed,
// relative to the working directory, as the local app always has. A Root
// made with New resolves names inside its directory and rejects absolute
// paths, names that climb out with "..", hidden files and directories,
// and symbolic links that lead outside or nowhere.
package confine

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ErrOutside is wrapped by every name a confined Root refuses.
var ErrOutside = errors.New("outside the served directory")

// Root is a directory names are confined to; the zero value confines
// nothing.
type Root struct {
	dir string // absolute, with symbolic links resolved; "" confines nothing
}

// New confines names to dir, which must exist.
func New(dir string) (Root, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Root{}, err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Root{}, err
	}
	st, err := os.Stat(real)
	if err != nil {
		return Root{}, err
	}
	if !st.IsDir() {
		return Root{}, fmt.Errorf("%s is not a directory", dir)
	}
	return Root{dir: real}, nil
}

// Confined reports whether r keeps names inside a directory.
func (r Root) Confined() bool { return r.dir != "" }

// Dir is the directory names are confined to, "" when unconfined.
func (r Root) Dir() string { return r.dir }

// Resolve returns the path to use on disk for name, as the user typed
// it. Unconfined, that's name itself. Confined, it's a path inside the
// root with every existing symbolic link along it followed, or an error
// wrapping ErrOutside.
func (r Root) Resolve(name string) (string, error) {
	if r.dir == "" {
		return name, nil
	}
	if name == "" {
		return "", errors.New("no file name")
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") || strings.HasPrefix(name, `\`) {
		return "", refuse("absolute path")
	}
	clean := filepath.Clean(name)
	if !filepath.IsLocal(clean) {
		return "", ErrOutside
	}
	parts := strings.Split(clean, string(filepath.Separator))
	if clean == "." {
		parts = nil
	}
	if slices.ContainsFunc(parts, hidden) {
		return "", refuse("hidden file")
	}
	cur := r.dir
	for i, p := range parts {
		next := filepath.Join(cur, p)
		st, err := os.Lstat(next)
		if errors.Is(err, fs.ErrNotExist) {
			// The rest doesn't exist yet (a file about to be saved):
			// nothing along it can lead elsewhere.
			return filepath.Join(append([]string{cur}, parts[i:]...)...), nil
		}
		if err != nil {
			return "", err
		}
		if st.Mode()&fs.ModeSymlink != 0 {
			target, err := filepath.EvalSymlinks(next)
			if err != nil {
				return "", refuse("broken link")
			}
			if !r.contains(target) {
				return "", refuse("link")
			}
			next = target
		}
		cur = next
	}
	return cur, nil
}

// Scrub removes the root's location from msg, such as an error naming a
// resolved path, so it names files relative to the root.
func (r Root) Scrub(msg string) string {
	if r.dir == "" {
		return msg
	}
	msg = strings.ReplaceAll(msg, r.dir+string(filepath.Separator), "")
	return strings.ReplaceAll(msg, r.dir, ".")
}

// contains reports whether the resolved path p is the root or a visible
// file or directory inside it.
func (r Root) contains(p string) bool {
	rel, err := filepath.Rel(r.dir, p)
	if err != nil || !filepath.IsLocal(rel) {
		return false
	}
	return rel == "." || !slices.ContainsFunc(strings.Split(rel, string(filepath.Separator)), hidden)
}

func hidden(part string) bool { return strings.HasPrefix(part, ".") }

// refuse is ErrOutside with the reason. The name is left out, as callers
// show it already.
func refuse(why string) error {
	return fmt.Errorf("%w (%s)", ErrOutside, why)
}
