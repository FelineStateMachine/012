package ui

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/transfer"
)

// Recovery files: when 012 serve stops, or a session idles out, with
// unsaved changes, the workbook is kept in a hidden directory of the
// served directory, and the next session opening that file offers it
// back. The directory is hidden, so names typed in a session never reach
// it (internal/confine); only this file reads and writes it. See
// docs/terminal/ssh.md.
//
// A local session keeps them too when it stops on an internal error
// (crash.go), in Settings.RecoveryDir, keyed by the file's absolute path,
// and offers them back when that file is opened again. See
// docs/files/saving.md.

// RecoveryDir is the directory, inside the served one, recovery files
// are kept in.
const RecoveryDir = ".012-recovery"

const (
	recoveryKeep     = 3            // recovery files kept per file name; older ones are removed
	recoveryUntitled = "(untitled)" // the key for a book never saved; "(" is escaped in file names
	recoveryStamp    = "20060102-150405"
	// recoveryKeyMax keeps a recovery file's name (the key, the time and
	// .012) under the 255 bytes file systems allow.
	recoveryKeyMax = 200
)

// recoveryName matches a recovery file's name after its key and "-":
// the time it was written, a count when two share a second, and .012.
var recoveryName = regexp.MustCompile(`^(\d{8}-\d{6})(-\d+)?\.012$`)

// OpenOnStart makes a served session start on name, as `ssh -t host
// name` asks: a sheet to open (or a new one to save as name, when there's
// no such file) or a file to import. Either way, or with name empty, the
// session then offers unsaved changes kept for that name.
func (m *Model) OpenOnStart(name string) {
	m.start = &name
}

// OfferKept makes a local session offer, once it starts, the changes
// kept for its file (or for an untitled book) when 012 last stopped on
// an internal error. cmd/012 asks for it when started on a file or on
// nothing, not on an import or standard input.
func (m *Model) OfferKept() { m.offerKept = true }

// startOpenCmd opens what OpenOnStart set, once, when the program starts.
func (m *Model) startOpenCmd() tea.Cmd {
	if m.start == nil {
		if m.offerKept {
			m.offerKept = false
			m.offerRecovery(m.filename)
		}
		return nil
	}
	name := *m.start
	m.start = nil
	if name == "" {
		m.offerRecovery("")
		return nil
	}
	if m.share.reg != nil && strings.HasPrefix(name, "@") {
		m.joinNamed(name)
		return nil
	}
	if _, ok := fileio.KindOf(name); ok {
		return m.startImport(name, fileio.Options{}, transfer.Book)
	}
	name = withExt(name)
	p, ok := m.path("open", name)
	if !ok {
		return nil
	}
	if m.share.reg != nil {
		return m.openShared(name, p)
	}
	if _, err := os.Stat(p); errors.Is(err, fs.ErrNotExist) {
		// A new sheet, saved under this name.
		m.filename, m.disk = name, stamp{}
		m.offerRecovery(name)
		return nil
	}
	return loadCmd(name, p, m.spans.Parent())
}

// Unsaved reports whether the workbook has changes a recovery file
// would keep: it was changed since it was last saved or opened, and
// isn't empty.
func (m *Model) Unsaved() bool {
	if !m.changed {
		return false
	}
	wb := m.sheet.Book()
	if len(wb.Macros()) > 0 {
		return true
	}
	for _, s := range slices.Concat(wb.Visible(), wb.HiddenSheets()) {
		if _, ok := s.UsedRange(); ok {
			return true
		}
	}
	return false
}

// Filename is the open file's name as typed, "" for a book never saved.
func (m *Model) Filename() string { return m.filename }

// Recover writes the workbook to a new recovery file for its name,
// removing the oldest beyond recoveryKeep, and returns the file's name:
// relative to the served directory in 012 serve, its full path in a
// local session. It's for when the program has stopped; a local model
// without Settings.RecoveryDir has nowhere to put one.
func (m *Model) Recover(now time.Time) (string, error) {
	dir, key, err := m.recoveryPlace(m.filename, true)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := m.sheet.Write(&buf); err != nil {
		return "", err
	}
	base := key + "-" + now.Format(recoveryStamp)
	for i := 1; i < 100; i++ {
		name := base + sheet.FileExt
		if i > 1 {
			name = fmt.Sprintf("%s-%d%s", base, i, sheet.FileExt)
		}
		f, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, err = f.Write(buf.Bytes())
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(f.Name())
			return "", err
		}
		prune(dir, key)
		if !m.root.Confined() {
			return filepath.Join(dir, name), nil
		}
		return filepath.Join(RecoveryDir, name), nil
	}
	return "", errors.New("too many recovery files this second")
}

// recoveryPlace is the directory recovery files for name are kept in,
// made when create is set, and the start of their names there.
func (m *Model) recoveryPlace(name string, create bool) (dir, key string, err error) {
	if m.root.Confined() {
		dir, err = recoveryDir(m.root.Dir(), create)
		return dir, recoveryKey(name), err
	}
	dir = m.prefs.RecoveryDir
	if dir == "" {
		return "", "", errors.New("this session keeps no recovery files")
	}
	if create {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", "", err
		}
	}
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return "", "", fmt.Errorf("no recovery directory at %s", dir)
	}
	if name != "" {
		if abs, err := filepath.Abs(name); err == nil {
			name = abs
		}
	}
	return dir, recoveryKey(name), nil
}

// recoveryDir is the recovery directory in root, made 0700 when create
// is set. It must be a directory of its own, not a link elsewhere.
func recoveryDir(root string, create bool) (string, error) {
	dir := filepath.Join(root, RecoveryDir)
	if create {
		if err := os.Mkdir(dir, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
	}
	st, err := os.Lstat(dir)
	if err != nil {
		return "", err
	}
	if !st.IsDir() {
		return "", fmt.Errorf("%s isn't a directory", RecoveryDir)
	}
	return dir, nil
}

// recoveryKey is the start of the recovery files' names for a file name
// as typed: the name without its extension, escaped so any path is one
// file name, or recoveryUntitled for a book never saved. A key too long
// for a file name keeps its end after a hash of the whole.
func recoveryKey(name string) string {
	if name == "" {
		return recoveryUntitled
	}
	name = filepath.ToSlash(filepath.Clean(name))
	key := url.PathEscape(strings.TrimSuffix(name, filepath.Ext(name)))
	if len(key) > recoveryKeyMax {
		sum := sha256.Sum256([]byte(key))
		key = hex.EncodeToString(sum[:8]) + "-" + key[len(key)-(recoveryKeyMax-17):]
	}
	return key
}

// recoveryFile is a recovery file found for a name.
type recoveryFile struct {
	path string
	when time.Time
	n    int // which of the files written in that second
}

// recoveries lists the recovery files kept for key in dir, oldest first.
func recoveries(dir, key string) []recoveryFile {
	entries, _ := os.ReadDir(dir)
	var out []recoveryFile
	for _, e := range entries {
		rest, ok := strings.CutPrefix(e.Name(), key+"-")
		if !ok || !e.Type().IsRegular() {
			continue
		}
		mm := recoveryName.FindStringSubmatch(rest)
		if mm == nil {
			continue
		}
		when, err := time.ParseInLocation(recoveryStamp, mm[1], time.Local)
		if err != nil {
			continue
		}
		n, _ := strconv.Atoi(strings.TrimPrefix(mm[2], "-")) // 0 for the first in its second
		out = append(out, recoveryFile{path: filepath.Join(dir, e.Name()), when: when, n: n})
	}
	slices.SortFunc(out, func(a, b recoveryFile) int {
		if c := a.when.Compare(b.when); c != 0 {
			return c
		}
		return a.n - b.n
	})
	return out
}

// prune removes the oldest recovery files for key beyond recoveryKeep.
func prune(dir, key string) {
	files := recoveries(dir, key)
	for _, f := range files[:max(0, len(files)-recoveryKeep)] {
		os.Remove(f.path)
	}
}

// offerRecovery asks, on the context line, whether to restore the
// newest recovery file kept for name, when a session opens it.
func (m *Model) offerRecovery(name string) {
	dir, key, err := m.recoveryPlace(name, false)
	if err != nil {
		return
	}
	files := recoveries(dir, key)
	if len(files) == 0 {
		return
	}
	f := files[len(files)-1]
	what := "an untitled sheet"
	if name != "" {
		what = filepath.Base(name)
	}
	m.ask(question{
		msg:  "Unsaved changes to " + what + " were kept.",
		warn: true,
		desc: "Kept " + f.when.Format("Jan 2 15:04") + ", when a session ended unsaved. Later asks again next time",
		choices: []choice{
			{key: "enter", label: "Restore", run: func(m *Model) tea.Cmd { return restoreCmd(name, f.path) }},
			{key: "d", label: "Discard", run: func(m *Model) tea.Cmd {
				if err := os.Remove(f.path); err != nil {
					m.fail("Couldn't delete the kept changes: " + err.Error())
					return nil
				}
				m.note = "Deleted the kept changes to " + what
				return nil
			}},
			{key: "esc", label: "Later", run: func(*Model) tea.Cmd { return nil }},
		},
	})
}

// restoredMsg carries a recovery file read back for name.
type restoredMsg struct {
	name, file string
	sheet      *sheet.Sheet
	err        error
}

func restoreCmd(name, file string) tea.Cmd {
	return func() tea.Msg {
		f, err := os.Open(file)
		if err != nil {
			return restoredMsg{name: name, file: file, err: err}
		}
		defer f.Close()
		s, err := sheet.Read(f)
		return restoredMsg{name: name, file: file, sheet: s, err: err}
	}
}

// restored puts the kept changes in place of the opened file, as unsaved
// changes to it: saving still checks the file on disk hasn't changed
// since it was opened. The recovery file goes once they're saved.
func (m *Model) restored(msg restoredMsg) {
	if msg.err != nil {
		m.fail("Couldn't restore the kept changes: " + msg.err.Error())
		return
	}
	disk := m.disk
	m.reset(msg.sheet, msg.name)
	m.disk, m.saved, m.changed = disk, -1, true
	m.recovered = msg.file
	m.note = "Restored the kept changes; save to keep them"
}

// savedRecovered removes the recovery file restored into this book, once
// the book is saved.
func (m *Model) savedRecovered() {
	if m.recovered != "" {
		os.Remove(m.recovered)
		m.recovered = ""
	}
}
