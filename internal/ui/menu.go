package ui

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

type promptKind int

const (
	promptText promptKind = iota
	promptRange
	promptWidth
)

// prompt is a question on the context line, such as a file name or a
// range to act on.
type prompt struct {
	kind      promptKind
	label     string
	indicator string
	fresh     bool // the default is still showing; typing replaces it
	typing    bool // range prompts: the user is typing instead of pointing
	onText    func(m *Model, text string) tea.Cmd
	onRange   func(m *Model, r sheet.Rect) tea.Cmd
	onCancel  func(m *Model)
}

func (m *Model) pointing() bool {
	return m.mode == modePrompt && m.prompt.kind == promptRange && !m.prompt.typing
}

func (m *Model) openPrompt(p *prompt, initial string) {
	m.mode = modePrompt
	m.prompt = p
	m.buf, m.bufPos = nil, 0
	m.hint = ""
	m.insert(initial)
}

func (m *Model) openText(label, initial string, onText func(*Model, string) tea.Cmd) {
	m.openPrompt(&prompt{kind: promptText, label: label, indicator: "EDIT", fresh: true, onText: onText}, initial)
}

// openRange asks for a range, starting from the current selection.
func (m *Model) openRange(label string, onRange func(*Model, sheet.Rect) tea.Cmd) {
	r := m.selection()
	m.openPrompt(&prompt{kind: promptRange, label: label, indicator: "POINT", onRange: onRange}, "")
	m.point = pointer{anchor: r.From, at: r.To, anchored: r.From != r.To}
}

func (m *Model) openGoto() {
	m.openText("Go to:", m.cur.String(), func(m *Model, text string) tea.Cmd {
		text = strings.TrimSpace(text)
		// A sheet may lead: Sheet2!A1, 'Q3 plan'!B2:C9, or just Sheet2!.
		target := m.sheet
		if name, rest := sheet.SplitSheet(text); name != "" {
			if target = m.book().Lookup(name); target == nil {
				m.fail("There's no sheet named " + name)
				return nil
			}
			if text = rest; text == "" {
				m.showSheet(target)
				return nil
			}
		}
		r, ok := sheet.ParseRange(text)
		if n, named := m.sheet.LookupName(text); named && !n.Gone() && target == m.sheet {
			r, ok, target = n.Range, true, n.Sheet
		}
		if !ok {
			m.fail("Not a cell, range or named range: " + text)
			return nil
		}
		m.showSheet(target)
		if r.From == r.To {
			m.clearSelection()
			m.cur = r.From
			return nil
		}
		m.selectRect(r)
		return nil
	})
	m.prompt.indicator = "POINT"
}

// openWidth asks for the width of the selected columns, previewing it
// live as the arrows change it. Esc restores the original widths.
func (m *Model) openWidth() tea.Cmd {
	r := m.selection()
	orig := map[int]int{}
	for c := r.From.Col; c <= r.To.Col; c++ {
		orig[c] = m.sheet.ColWidth(c)
	}
	restore := func(m *Model) {
		for c, w := range orig {
			m.sheet.SetColWidth(c, w)
		}
	}
	m.openPrompt(&prompt{
		kind:      promptWidth,
		label:     "Column width (1-240):",
		indicator: "WIDTH",
		fresh:     true,
		onText: func(m *Model, text string) tea.Cmd {
			w, err := strconv.Atoi(text)
			if err != nil || w < 1 || w > 240 {
				restore(m)
				m.fail("Column width must be between 1 and 240")
				return nil
			}
			m.setWidths(w)
			m.changed = true
			return nil
		},
		onCancel: restore,
	}, strconv.Itoa(m.sheet.ColWidth(m.cur.Col)))
	return nil
}

// setWidths sets the width of every selected column.
func (m *Model) setWidths(w int) {
	r := m.selection()
	for c := r.From.Col; c <= r.To.Col; c++ {
		m.sheet.SetColWidth(c, w)
	}
}

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
	m.files = nil
	m.openText("Open file:", "", func(m *Model, text string) tea.Cmd {
		if _, ok := fileio.KindOf(text); ok {
			return m.confirmImport(text, fileio.Options{})
		}
		return loadCmd(withExt(text))
	})
	return listFilesCmd
}

// promptKey handles keys while a prompt is open.
func (m *Model) promptKey(k tea.KeyPressMsg) tea.Cmd {
	p := m.prompt
	key := k.String()
	switch key {
	case "esc":
		if m.pointing() && m.point.anchored {
			m.point.anchored = false
			return nil
		}
		if p.onCancel != nil {
			p.onCancel(m)
		}
		m.quitAfterSave = false // cancelling Save as cancels Save and quit
		m.closePrompt()
		return nil
	case "enter":
		text := strings.TrimSpace(string(m.buf))
		m.closePrompt()
		if p.kind != promptRange {
			return p.onText(m, text)
		}
		r := m.point.rect()
		if p.typing {
			var ok bool
			if r, ok = sheet.ParseRange(text); !ok {
				m.fail("Invalid range: " + text)
				return nil
			}
		}
		return p.onRange(m, r)
	}

	switch {
	case m.pointing():
		if m.pointMoveKey(key) {
			return nil
		}
		if key == ":" {
			m.point.anchor, m.point.anchored = m.point.at, true
			return nil
		}
		if text := typed(k); text != "" {
			p.typing = true
			m.insert(text)
		}
	case p.kind == promptWidth && (key == "left" || key == "right"):
		w, _ := strconv.Atoi(string(m.buf))
		if key == "left" {
			w--
		} else {
			w++
		}
		w = clamp(w, 1, 240)
		m.buf, m.bufPos, p.fresh = nil, 0, false
		m.insert(strconv.Itoa(w))
		m.setWidths(w) // live preview
	default:
		if p.fresh && typed(k) != "" {
			m.buf, m.bufPos = nil, 0
		}
		p.fresh = false
		if p.kind == promptWidth && !isDigits(typed(k)) {
			return nil
		}
		m.lineKey(k)
	}
	return nil
}

func (m *Model) promptType(text string) {
	if m.prompt.fresh {
		m.buf, m.bufPos = nil, 0
		m.prompt.fresh = false
	}
	m.insert(text)
}

func (m *Model) closePrompt() {
	m.prompt = nil
	m.files = nil
	m.cancelEntry()
}

func (m *Model) fail(msg string) {
	m.mode = modeError
	m.errMsg = msg
}

// reset starts over on s, as File > New and Open do. What belongs to the
// session rather than the sheet carries over: the window, theme, terminal
// state and the JEV connection.
func (m *Model) reset(s *sheet.Sheet, filename string) {
	*m = Model{grid: grid{sheet: s, width: m.width, height: m.height}, filename: filename, th: m.th, term: m.term, jev: m.jev, lastChart: -1}
}

func isDigits(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return !unicode.IsDigit(r) })
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
