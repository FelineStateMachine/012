package live

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/FelineStateMachine/012/internal/fileio"
)

// File follows a file with the standard library's stat alone (no
// inotify or kqueue, which differ by OS and need cgo or a module), which
// polling every Interval makes cheap enough: one stat a poll while
// nothing changes.
//
// A text table (CSV, TSV, JSON lines, NUON; fileio.Kind.Grows) is read
// as it grows: each poll reads what was appended, and a record cut off
// at the end waits for the rest (fileio.Tail). The file is read again
// whole when it shrinks (truncated), is another file (rotated: the path
// names a new inode), or its bytes before where reading stopped differ
// (rewritten in place). Any other format is read again whole when its
// size or modification time changes, once they have held still for
// Debounce, so a file being written isn't read half done.
type File struct {
	// Name is how the file is named to the user, in errors.
	Name   string
	path   string
	kind   fileio.Kind
	opt    fileio.Options
	loaded bool
	err    string

	// A growing file: what identifies it, how far it's read, and the
	// bytes before that, which a rewrite changes.
	info   os.FileInfo
	offset int64
	mark   []byte
	tail   *fileio.Tail

	// A file read whole: its stamp when last read, and the one last
	// seen with when it was first seen, for the debounce.
	read, seen stamp
	seenAt     time.Time

	// Debounce is how long a file read whole must hold still.
	Debounce time.Duration
	// MaxRead bounds what one poll reads of a growing file, so a large
	// file comes in over several polls, each a short step for the UI.
	MaxRead int64
	// Now is the clock; tests replace it.
	Now func() time.Time
}

// stamp is what tells a file changed: its size and modification time.
type stamp struct {
	size int64
	mod  time.Time
}

func stampOf(st os.FileInfo) stamp { return stamp{st.Size(), st.ModTime()} }

// markLen is how many bytes before the read offset tell an append from
// a rewrite.
const markLen = 64

// NewFile follows the file at path (on disk) in format k, with opt's
// table, query and locale (fileio.Options); name is how the user named it.
func NewFile(name, path string, k fileio.Kind, opt fileio.Options) *File {
	return &File{Name: name, path: path, kind: k, opt: opt, Debounce: 300 * time.Millisecond, MaxRead: 1 << 20, Now: time.Now}
}

// Poll reads what changed (see File).
func (f *File) Poll(ctx context.Context) (Update, bool) {
	st, err := os.Stat(f.path)
	if err != nil {
		return f.fail(err)
	}
	var u Update
	var ok bool
	if f.kind.Grows() {
		u, ok = f.pollGrowing(st)
	} else {
		u, ok = f.pollWhole(ctx, st)
	}
	u.At = f.Now()
	if ok && u.Err == "" {
		f.err = ""
	}
	return u, ok
}

// fail reports err once, until the file can be read again.
func (f *File) fail(err error) (Update, bool) {
	msg := err.Error()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		msg = "the file isn't there"
	case errors.Is(err, fs.ErrPermission):
		msg = "permission denied"
	default:
		// Errors name the path on disk; the user named the file.
		msg = strings.ReplaceAll(msg, f.path, f.Name)
	}
	if msg == f.err {
		return Update{}, false
	}
	f.err = msg
	f.Reload() // it's read whole once it's back
	return Update{Err: msg, At: f.Now()}, true
}

// Reload has the next poll read the file again, whole.
func (f *File) Reload() {
	f.loaded = false
	f.read, f.seen = stamp{}, stamp{}
	if f.tail != nil {
		f.tail.Close()
		f.tail = nil
	}
}

// Close lets the file go.
func (f *File) Close() { f.Reload() }

// pollWhole reads a file that is rewritten, not appended to, once it
// has held still.
func (f *File) pollWhole(ctx context.Context, st os.FileInfo) (Update, bool) {
	s := stampOf(st)
	if f.loaded && s == f.read {
		return Update{}, false
	}
	now := f.Now()
	if s != f.seen {
		f.seen, f.seenAt = s, now
		if f.loaded {
			return Update{}, false // changing: wait for it to settle
		}
	}
	if f.loaded && now.Sub(f.seenAt) < f.Debounce {
		return Update{}, false
	}
	res, err := fileio.Import(ctx, f.path, f.opt)
	if err != nil {
		return f.fail(err)
	}
	f.loaded, f.read = true, s
	rows := fileio.Rows(res.Sheet)
	return Update{Reset: true, Header: rows.Header, Rows: rows.Rows, Note: strings.Join(res.Notes, "; ")}, true
}

// pollGrowing reads what a growing file gained, or the whole file again
// when it was truncated, rotated or rewritten.
func (f *File) pollGrowing(st os.FileInfo) (Update, bool) {
	reset := !f.loaded || !os.SameFile(f.info, st) || st.Size() < f.offset
	if !reset && st.Size() == f.offset && st.ModTime().Equal(f.info.ModTime()) {
		return Update{}, false
	}
	file, err := os.Open(f.path)
	if err != nil {
		return f.fail(err)
	}
	defer file.Close()
	if !reset && !f.sameMark(file) {
		reset = true
	}
	if reset {
		if err := f.restart(file); err != nil {
			return f.fail(err)
		}
	}
	f.info = st
	u := Update{Reset: reset}
	end := min(st.Size(), f.offset+f.MaxRead)
	if end > f.offset {
		piece := make([]byte, end-f.offset)
		n, err := file.ReadAt(piece, f.offset)
		if err != nil && !errors.Is(err, io.EOF) {
			return f.fail(err)
		}
		piece = piece[:n]
		rows, err := f.tail.Feed(piece)
		if err != nil {
			f.Reload()
			return Update{Err: err.Error(), At: f.Now()}, true
		}
		f.offset += int64(n)
		f.remember(piece)
		u.Header, u.Rows, u.More = rows.Header, rows.Rows, f.offset < st.Size()
	}
	return u, reset || u.Header != nil || len(u.Rows) > 0
}

// restart reads the file from its start: a new tail, sniffing its head.
func (f *File) restart(file *os.File) error {
	if f.tail != nil {
		f.tail.Close()
	}
	head := make([]byte, 64<<10)
	n, err := file.ReadAt(head, 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	tail, err := fileio.NewTail(f.kind, head[:n], f.opt.Locale)
	if err != nil {
		return err
	}
	f.tail, f.offset, f.mark, f.loaded = tail, 0, nil, true
	return nil
}

// remember keeps the last bytes read, which a rewrite changes.
func (f *File) remember(piece []byte) {
	f.mark = append(f.mark, piece[max(len(piece)-markLen, 0):]...)
	f.mark = f.mark[max(len(f.mark)-markLen, 0):]
}

// sameMark reports whether the bytes before the offset are as read.
func (f *File) sameMark(file *os.File) bool {
	if len(f.mark) == 0 {
		return true
	}
	b := make([]byte, len(f.mark))
	n, _ := file.ReadAt(b, f.offset-int64(len(b)))
	return bytes.Equal(b[:n], f.mark)
}

// KindOf is the format a linked file is read in: format, named as
// fileio.Kind.String names it, or else the one its name tells.
func KindOf(name, format string) (fileio.Kind, bool) {
	for _, k := range fileio.Kinds() {
		if strings.EqualFold(k.String(), format) {
			return k, true
		}
	}
	return fileio.KindOf(filepath.Base(name))
}
