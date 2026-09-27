package ui

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// Saving and opening .012 files. Files are written and read in the
// background; other formats go through transfer.go.
//
// Names are kept as the user typed them and turned into paths on disk
// only to read or write (Model.path), so a session served over SSH stays
// inside its directory.

// Serve prepares m for a session served over SSH: file names resolve
// inside root, and env (KEY=value pairs) is the client's environment,
// used instead of the server's to learn about the client's terminal.
func (m *Model) Serve(root confine.Root, env []string) {
	m.root = root
	m.term = newTerminal(envLookup(env))
}

// envLookup reads a variable from a list of KEY=value pairs, the last
// one winning as in os/exec.
func envLookup(env []string) func(string) string {
	return func(key string) string {
		v := ""
		for _, kv := range env {
			if k, val, ok := strings.Cut(kv, "="); ok && k == key {
				v = val
			}
		}
		return v
	}
}

// path is where name is on disk. It fails for a name outside the served
// directory, saying so with verb ("open", "save") in the note.
func (m *Model) path(verb, name string) (string, bool) {
	p, err := m.root.Resolve(name)
	if err != nil {
		m.fail(fmt.Sprintf("Couldn't %s %s: %v", verb, name, err))
		return "", false
	}
	return p, true
}

func (m *Model) openSave() tea.Cmd {
	name := m.filename
	if name == "" {
		name = m.displayBase() + sheet.FileExt
	}
	m.openText("Save as:", name, (*Model).saveAsFile)
	return nil
}

// saveAsFile saves the sheet under a name typed for Save as or :w,
// asking first when that's another file that exists.
func (m *Model) saveAsFile(text string) tea.Cmd {
	name := withExt(text)
	p, ok := m.path("save", name)
	if !ok {
		m.quitAfterSave = false
		return nil
	}
	return m.confirmReplace(filepath.Base(name)+" exists.", func(m *Model) tea.Cmd {
		return m.saveAs(name, true)
	}, !m.isOpenFile(p) && exists(p))
}

// isOpenFile reports whether path on disk is the open file's, however
// its name was typed. The open file has its own check, for changes
// since it was opened (saveAs).
func (m *Model) isOpenFile(path string) bool {
	if m.filename == "" {
		return false
	}
	open, err := m.root.Resolve(m.filename)
	if err != nil {
		return false
	}
	a, err1 := filepath.Abs(open)
	b, err2 := filepath.Abs(path)
	return err1 == nil && err2 == nil && a == b
}

func (m *Model) openRetrieve() tea.Cmd {
	m.openText("Open file:", "", (*Model).openFile)
	return listFilesCmd(m.root)
}

// openFile opens a sheet, or imports a file of another format, named for
// Open or :e.
func (m *Model) openFile(text string) tea.Cmd {
	if _, ok := fileio.KindOf(text); ok {
		return m.confirmImport(text, fileio.Options{})
	}
	name := withExt(text)
	p, ok := m.path("open", name)
	if !ok {
		return nil
	}
	return loadCmd(name, p, m.spans.Parent())
}

// reset starts over on s, as File > New and Open do. What belongs to the
// session rather than the sheet carries over: the window, theme, terminal
// state, served directory and the JEV connection.
func (m *Model) reset(s *sheet.Sheet, filename string) {
	*m = Model{grid: grid{sheet: s, width: m.width, height: m.height}, filename: filename, th: m.th, term: m.term, jev: m.jev, root: m.root, charts: chartState{last: -1}, prefs: m.prefs,
		macros: macroState{machine: m.macros.machine, editor: m.macros.editor}, spans: m.spans}
	s.Book().SetTrace(m.spans)
	if m.jev != nil {
		s.Book().SetRemote(m.jev.cache)
	}
}

func withExt(name string) string {
	if filepath.Ext(name) == "" {
		return name + sheet.FileExt
	}
	return name
}

// stamp tells versions of a file on disk apart: when it was last written
// and its size. The zero stamp stands for no file.
type stamp struct {
	mod  time.Time
	size int64
}

func stampOf(st fs.FileInfo) stamp { return stamp{st.ModTime(), st.Size()} }

// diskStamp is the stamp of the file at path now, zero if there's none.
func diskStamp(path string) stamp {
	st, err := os.Stat(path)
	if err != nil {
		return stamp{}
	}
	return stampOf(st)
}

func (s stamp) equal(o stamp) bool { return s.mod.Equal(o.mod) && s.size == o.size }

type savedMsg struct {
	name     string
	err      error
	stamp    stamp // the file as written
	conflict bool  // not written: the file changed on disk since it was opened
}

type loadedMsg struct {
	name  string
	sheet *sheet.Sheet
	stamp stamp
	err   error
}

type filesMsg []string

// saveAs saves the sheet as name. When name is the open file and check
// is set, it first makes sure nobody else wrote the file since it was
// opened or last saved here: another program, or another session of a
// served directory.
func (m *Model) saveAs(name string, check bool) tea.Cmd {
	p, ok := m.path("save", name)
	if !ok {
		m.quitAfterSave = false
		return nil
	}
	expect := (*stamp)(nil)
	if check && name == m.filename {
		expect = &m.disk
	}
	return saveCmd(m.sheet, name, p, expect, m.spans)
}

// saveCmd writes the worksheet to path atomically: to a temporary file
// first, then renamed over the target. With expect set, it writes only
// if the file on disk is still that version.
func saveCmd(s *sheet.Sheet, name, path string, expect *stamp, spans *telemetry.Trace) tea.Cmd {
	span := spans.Start("save")
	var buf strings.Builder
	err := s.Write(&buf)
	if telemetry.Enabled() {
		span.End(slog.Int("cells", s.Len()), slog.Int("bytes", buf.Len()))
	}
	return func() tea.Msg {
		if err != nil {
			return savedMsg{name: name, err: err}
		}
		// A file deleted meanwhile is simply written again.
		if now := diskStamp(path); expect != nil && now != (stamp{}) && !now.equal(*expect) {
			return savedMsg{name: name, conflict: true}
		}
		if err := writeAtomic(path, buf.String()); err != nil {
			return savedMsg{name: name, err: err}
		}
		return savedMsg{name: name, stamp: diskStamp(path)}
	}
}

// writeAtomic writes data to path through a temporary file of its own
// beside it, so two writers never share one, then renames it over path.
func writeAtomic(path, data string) error {
	f, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.WriteString(data)
	if err == nil {
		err = f.Chmod(0o644)
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// loadCmd reads the file in the background, the span of it and its
// recalculation under parent.
func loadCmd(name, path string, parent telemetry.Parent) tea.Cmd {
	return func() tea.Msg {
		f, err := os.Open(path)
		if err != nil {
			return loadedMsg{name: name, err: err}
		}
		defer f.Close()
		var st stamp
		if fi, err := f.Stat(); err == nil {
			st = stampOf(fi)
		}
		span := parent.Start("open")
		s, err := sheet.ReadTraced(f, telemetry.NewTrace(span.Parent()))
		if err != nil {
			span.Fail(err)
		} else if telemetry.Enabled() {
			span.End(slog.Int("cells", s.Len()), slog.Int64("bytes", st.size))
		}
		return loadedMsg{name, s, st, err}
	}
}

// listFilesCmd lists the .012 files in the directory names are relative
// to.
func listFilesCmd(root confine.Root) tea.Cmd {
	return func() tea.Msg {
		dir, err := root.Resolve(".")
		if err != nil {
			return filesMsg(nil)
		}
		files, _ := filepath.Glob(filepath.Join(globEscape(dir), "*"+sheet.FileExt))
		for i, f := range files {
			files[i] = filepath.Base(f)
		}
		files = slices.DeleteFunc(files, func(f string) bool { return strings.HasPrefix(f, ".") })
		slices.Sort(files)
		return filesMsg(files)
	}
}

// globEscape escapes the pattern characters in a directory name.
func globEscape(dir string) string {
	if dir == "." || !strings.ContainsAny(dir, `*?[\`) {
		return dir
	}
	var b strings.Builder
	for _, r := range dir {
		if strings.ContainsRune(`*?[\`, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func (m *Model) handleSaved(msg savedMsg) tea.Cmd {
	if msg.conflict {
		m.openOverlay(&choiceBar{
			m:    m,
			msg:  filepath.Base(msg.name) + " changed on disk since it was opened.",
			warn: true,
			choices: []choice{
				{key: "enter", label: "Overwrite", run: func(m *Model) tea.Cmd { return m.saveAs(msg.name, false) }},
				{key: "s", label: "Save as", run: (*Model).openSave},
				{key: "esc", label: "Cancel", run: func(m *Model) tea.Cmd {
					m.quitAfterSave = false
					return nil
				}},
			},
		})
		return nil
	}
	quit := m.quitAfterSave
	m.quitAfterSave = false
	if msg.err != nil {
		m.fail(fmt.Sprintf("Couldn't save %s: %v", msg.name, msg.err))
		return nil
	}
	m.filename, m.disk = msg.name, msg.stamp
	m.changed = false
	if quit {
		return m.exit()
	}
	return nil
}

func (m *Model) handleLoaded(msg loadedMsg) {
	if msg.err != nil {
		m.fail(fmt.Sprintf("Couldn't open %s: %v", msg.name, msg.err))
		return
	}
	m.reset(msg.sheet, msg.name)
	m.disk = msg.stamp
}

// save writes to the current file, asking for a name the first time.
func (m *Model) save() tea.Cmd {
	if m.filename == "" && m.xfer.Source != "" {
		return m.saveImported()
	}
	if m.filename == "" {
		return m.openSave()
	}
	return m.saveAs(m.filename, true)
}
