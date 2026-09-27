package ui

import (
	"bytes"
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
// docs/ssh.md.

// RecoveryDir is the directory, inside the served one, recovery files
// are kept in.
const RecoveryDir = ".012-recovery"

const (
	recoveryKeep     = 3            // recovery files kept per file name; older ones are removed
	recoveryUntitled = "(untitled)" // the key for a book never saved; "(" is escaped in file names
	recoveryStamp    = "20060102-150405"
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

// startOpenCmd opens what OpenOnStart set, once, when the program starts.
func (m *Model) startOpenCmd() tea.Cmd {
	if m.start == nil {
		return nil
	}
	name := *m.start
	m.start = nil
	if name == "" {
		m.offerRecovery("")
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
// removing the oldest beyond recoveryKeep, and returns the file's name
// relative to the served directory. It's for 012 serve, when the program
// has stopped; a model that isn't served has nowhere to put one.
func (m *Model) Recover(now time.Time) (string, error) {
	if !m.root.Confined() {
		return "", errors.New("recovery files are only kept for served sessions")
	}
	var buf bytes.Buffer
	if err := m.sheet.Write(&buf); err != nil {
		return "", err
	}
	dir, err := recoveryDir(m.root.Dir(), true)
	if err != nil {
		return "", err
	}
	key := recoveryKey(m.filename)
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
		return filepath.Join(RecoveryDir, name), nil
	}
	return "", errors.New("too many recovery files this second")
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
// file name, or recoveryUntitled for a book never saved.
func recoveryKey(name string) string {
	if name == "" {
		return recoveryUntitled
	}
	name = filepath.ToSlash(filepath.Clean(name))
	return url.PathEscape(strings.TrimSuffix(name, filepath.Ext(name)))
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
// newest recovery file kept for name, when a served session opens it.
func (m *Model) offerRecovery(name string) {
	if !m.root.Confined() {
		return
	}
	dir, err := recoveryDir(m.root.Dir(), false)
	if err != nil {
		return
	}
	files := recoveries(dir, recoveryKey(name))
	if len(files) == 0 {
		return
	}
	f := files[len(files)-1]
	what := "an untitled sheet"
	if name != "" {
		what = filepath.Base(name)
	}
	m.openOverlay(&choiceBar{
		m:    m,
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
