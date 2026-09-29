// Package headless works on workbook files without the screen, for
// 012 get, set, recalc and export (see docs/files/scripts.md): it opens
// a .012 file, finds the cells a reference names, reads and writes
// them, lists the errors formulas show and saves atomically. Nothing
// here runs a program or reaches the network unless its caller asks:
// notebook cells run through RunNotebooks and JEV functions through
// AnswerJEV, which the command line calls only behind flags.
package headless

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// File is a workbook opened from a .012 file.
type File struct {
	Path string
	Book *sheet.Workbook
	// New is set when there was no file at Path and Book is empty.
	New  bool
	mode fs.FileMode
}

// Open reads the workbook at path, with the notebook outputs it kept
// sent on to their sheets, as the screen opens it. With create, a path
// with no file opens as an empty workbook that Save writes there, as
// opening a new name in 012 does.
func Open(path string, create bool) (*File, error) {
	f := &File{Path: path, mode: 0o644}
	file, err := os.Open(path)
	switch {
	case errors.Is(err, fs.ErrNotExist) && create:
		f.Book, f.New = sheet.NewBook(), true
		return f, nil
	case err != nil:
		return nil, err
	}
	defer file.Close()
	if st, err := file.Stat(); err == nil {
		if st.IsDir() {
			return nil, fmt.Errorf("%s is a folder, not a %s workbook", path, sheet.FileExt)
		}
		f.mode = st.Mode().Perm()
	}
	if f.Book, err = sheet.ReadBook(file); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	SyncOutputs(f.Book)
	return f, nil
}

// Save writes the workbook back to its file atomically: into a
// temporary file beside it, renamed over it once complete, so a failed
// write never leaves half a workbook. A file that would come out the
// same isn't touched, so its modification time says when it last
// changed.
func (f *File) Save() error {
	if !f.New {
		if old, err := os.ReadFile(f.Path); err == nil && sameAs(f.Book, old) {
			return nil
		}
	}
	return WriteAtomic(f.Path, f.mode, f.Book.Write)
}

// sameAs reports whether w writes exactly old.
func sameAs(w *sheet.Workbook, old []byte) bool {
	c := &compareWriter{want: old}
	return w.Write(c) == nil && c.same && c.n == len(old)
}

// compareWriter checks what's written against want without keeping it.
type compareWriter struct {
	want []byte
	n    int
	same bool
}

func (c *compareWriter) Write(p []byte) (int, error) {
	if c.n == 0 {
		c.same = true
	}
	if c.same && (c.n+len(p) > len(c.want) || string(c.want[c.n:c.n+len(p)]) != string(p)) {
		c.same = false
	}
	c.n += len(p)
	return len(p), nil
}

// WriteAtomic writes a file with write into a temporary file of its own
// beside path, then renames it over path with mode.
func WriteAtomic(path string, mode fs.FileMode, write func(io.Writer) error) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	err = write(tmp)
	if err == nil {
		err = tmp.Chmod(mode)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}

// Trusted reports whether the workbook's commands may run here without
// asking, by the rule macros follow: they were made or trusted on this
// computer, whose id is machine.
func Trusted(w *sheet.Workbook, machine string) bool {
	o := w.MacroOrigin()
	return o != "" && o == machine
}
