package ui

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// Saving and opening .012 files. Files are written and read in the
// background; other formats go through transfer.go.

func (m *Model) openSave() tea.Cmd {
	name := m.filename
	if name == "" {
		name = m.displayBase() + sheet.FileExt
	}
	m.openText("Save as:", name, func(m *Model, text string) tea.Cmd {
		return saveCmd(m.sheet, withExt(text))
	})
	return nil
}

func (m *Model) openRetrieve() tea.Cmd {
	m.openText("Open file:", "", func(m *Model, text string) tea.Cmd {
		if _, ok := fileio.KindOf(text); ok {
			return m.confirmImport(text, fileio.Options{})
		}
		return loadCmd(withExt(text))
	})
	return listFilesCmd
}

// reset starts over on s, as File > New and Open do. What belongs to the
// session rather than the sheet carries over: the window, theme, terminal
// state and the JEV connection.
func (m *Model) reset(s *sheet.Sheet, filename string) {
	*m = Model{grid: grid{sheet: s, width: m.width, height: m.height}, filename: filename, th: m.th, term: m.term, jev: m.jev, charts: chartState{last: -1}, prefs: m.prefs}
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

type savedMsg struct {
	name string
	err  error
}

type loadedMsg struct {
	name  string
	sheet *sheet.Sheet
	err   error
}

type filesMsg []string

// saveCmd writes the worksheet atomically: to a temporary file first, then
// renamed over the target.
func saveCmd(s *sheet.Sheet, name string) tea.Cmd {
	span := telemetry.Start("save")
	var buf strings.Builder
	err := s.Write(&buf)
	if telemetry.Enabled() {
		span.End(slog.Int("cells", s.Len()), slog.Int("bytes", buf.Len()))
	}
	return func() tea.Msg {
		if err != nil {
			return savedMsg{name, err}
		}
		tmp := name + ".tmp"
		if err := os.WriteFile(tmp, []byte(buf.String()), 0o644); err != nil {
			return savedMsg{name, err}
		}
		return savedMsg{name, os.Rename(tmp, name)}
	}
}

func loadCmd(name string) tea.Cmd {
	return func() tea.Msg {
		f, err := os.Open(name)
		if err != nil {
			return loadedMsg{name: name, err: err}
		}
		defer f.Close()
		span := telemetry.Start("open")
		s, err := sheet.Read(f)
		if err != nil {
			span.Fail(err)
		} else if telemetry.Enabled() {
			span.End(slog.Int("cells", s.Len()), slog.Int64("bytes", openSize(f)))
		}
		return loadedMsg{name, s, err}
	}
}

// openSize is the size of an open file, for telemetry; -1 if unknown.
func openSize(f *os.File) int64 {
	st, err := f.Stat()
	if err != nil {
		return -1
	}
	return st.Size()
}

func listFilesCmd() tea.Msg {
	files, _ := filepath.Glob("*" + sheet.FileExt)
	slices.Sort(files)
	return filesMsg(files)
}

func (m *Model) handleSaved(msg savedMsg) tea.Cmd {
	quit := m.quitAfterSave
	m.quitAfterSave = false
	if msg.err != nil {
		m.fail(fmt.Sprintf("Couldn't save %s: %v", msg.name, msg.err))
		return nil
	}
	m.filename = msg.name
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
}

// save writes to the current file, asking for a name the first time.
func (m *Model) save() tea.Cmd {
	if m.filename == "" && m.xfer.source != "" {
		return m.saveImported()
	}
	if m.filename == "" {
		return m.openSave()
	}
	return saveCmd(m.sheet, m.filename)
}
