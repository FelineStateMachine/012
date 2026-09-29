package mcp

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/FelineStateMachine/012/internal/diff"
	"github.com/FelineStateMachine/012/internal/headless"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Backend is where the tools' workbook lives. The tools only read it,
// work on a copy thrown away, or change it through the workbook's one
// mutation path (Batch), so the file backend here and a live session's
// (a workbook open on the screen, changed as a participant's operations)
// can stand behind the same tools.
type Backend interface {
	// Name is what the workbook is called, for titles: its file's name.
	Name() string
	// View runs fn on the workbook as it is, which fn must not change.
	View(ctx context.Context, fn func(*sheet.Workbook) error) error
	// Scratch runs fn on a copy of the workbook that is thrown away.
	Scratch(ctx context.Context, fn func(*sheet.Workbook) error) error
	// Change runs fn on the workbook inside one Batch, labelled label,
	// and returns what it changed. The change is kept unless fn fails or
	// dryRun is set; a failed change leaves the workbook as it was.
	Change(ctx context.Context, label string, dryRun bool, fn func(*sheet.Workbook) error) ([]diff.Change, error)
}

// FileBackend is a workbook file, opened afresh for every call so edits
// made meanwhile by others (the screen, git, 012 set) are seen, and
// saved atomically after each change, as 012 set saves it.
type FileBackend struct {
	Path string
	// Prepare, when set, runs on the workbook once opened, before fn:
	// what the server's flags allow (answering JEV functions).
	Prepare func(context.Context, *headless.File) error
	// Save writes a changed workbook; nil is File.Save.
	Save func(*headless.File) error

	mu sync.Mutex // one call at a time, so changes don't overlap
}

// Name is the file's name without its folder.
func (b *FileBackend) Name() string {
	return strings.TrimSuffix(filepath.Base(b.Path), filepath.Ext(b.Path))
}

// open opens the file, a missing one as an empty workbook saved on the
// first change, as 012 set creates it.
func (b *FileBackend) open(ctx context.Context) (*headless.File, error) {
	f, err := headless.Open(b.Path, true)
	if err != nil {
		return nil, err
	}
	if b.Prepare != nil {
		if err := b.Prepare(ctx, f); err != nil {
			return nil, err
		}
	}
	return f, nil
}

func (b *FileBackend) View(ctx context.Context, fn func(*sheet.Workbook) error) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	f, err := b.open(ctx)
	if err != nil {
		return err
	}
	return fn(f.Book)
}

// Scratch is View: every call opens its own copy of the file.
func (b *FileBackend) Scratch(ctx context.Context, fn func(*sheet.Workbook) error) error {
	return b.View(ctx, fn)
}

func (b *FileBackend) Change(ctx context.Context, label string, dryRun bool, fn func(*sheet.Workbook) error) ([]diff.Change, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	f, err := b.open(ctx)
	if err != nil {
		return nil, err
	}
	w := f.Book
	if err := w.Batch(sheet.Change{Label: label, Sheet: w.Sheet(w.Active())}, func() error { return fn(w) }); err != nil {
		return nil, err
	}
	changes, err := f.Changes()
	if err != nil || dryRun || len(changes) == 0 {
		return changes, err
	}
	if b.Save != nil {
		return changes, b.Save(f)
	}
	return changes, f.Save()
}
