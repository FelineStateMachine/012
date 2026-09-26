package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"

	"one23/internal/sheet"
)

// menuItem is one entry in the slash menu. An item either opens a submenu
// (items) or runs an action.
type menuItem struct {
	name   string
	desc   string
	descFn func(m *Model) string // overrides desc when set
	items  []menuItem
	action func(m *Model) tea.Cmd
}

type menuLevel struct {
	items []menuItem
	sel   int
}

var rootMenu = []menuItem{
	{name: "Worksheet", items: []menuItem{
		{name: "Column", items: []menuItem{
			{name: "Set-Width", desc: "Specify a width for the current column", action: (*Model).openWidth},
			{name: "Reset-Width", desc: "Return the current column to the default width", action: func(m *Model) tea.Cmd {
				m.sheet.SetColWidth(m.cur.Col, 0)
				m.changed = true
				return nil
			}},
		}},
		{name: "Erase", items: []menuItem{
			{name: "No", desc: "Do not erase the worksheet; return to READY mode", action: noop},
			{name: "Yes", descFn: unsavedWarning("Erase the entire worksheet from memory"), action: func(m *Model) tea.Cmd {
				m.reset(sheet.New(), "")
				return nil
			}},
		}},
	}},
	{name: "Range", items: []menuItem{
		{name: "Erase", desc: "Erase the cell or range", action: func(m *Model) tea.Cmd {
			m.openRange("Enter range to erase:", func(m *Model, r sheet.Rect) tea.Cmd {
				m.sheet.EraseRange(r)
				m.changed = true
				return nil
			})
			return nil
		}},
	}},
	{name: "File", items: []menuItem{
		{name: "Save", desc: "Store the entire worksheet in a file", action: (*Model).openSave},
		{name: "Retrieve", desc: "Erase the current worksheet and display the selected file", action: (*Model).openRetrieve},
	}},
	{name: "Quit", items: []menuItem{
		{name: "No", desc: "Do not end the session; return to READY mode", action: noop},
		{name: "Yes", descFn: unsavedWarning("End the session"), action: func(*Model) tea.Cmd { return tea.Quit }},
	}},
}

func noop(*Model) tea.Cmd { return nil }

func unsavedWarning(desc string) func(m *Model) string {
	return func(m *Model) string {
		if m.changed {
			return "WORKSHEET CHANGES NOT SAVED! " + desc + " anyway?"
		}
		return desc
	}
}

func (it menuItem) description(m *Model) string {
	switch {
	case it.descFn != nil:
		return it.descFn(m)
	case it.items != nil:
		names := make([]string, len(it.items))
		for i, sub := range it.items {
			names[i] = sub.name
		}
		return strings.Join(names, "  ")
	}
	return it.desc
}

func (m *Model) openMenu() {
	m.mode = modeMenu
	m.menu = []menuLevel{{items: rootMenu}}
}

// menuKey handles MENU mode: arrows highlight, Enter or an item's first
// letter selects, Esc backs out one level.
func (m *Model) menuKey(k tea.KeyPressMsg) tea.Cmd {
	lvl := &m.menu[len(m.menu)-1]
	n := len(lvl.items)
	switch k.String() {
	case "left", "shift+tab":
		lvl.sel = (lvl.sel + n - 1) % n
	case "right", "tab", "space":
		lvl.sel = (lvl.sel + 1) % n
	case "home":
		lvl.sel = 0
	case "end":
		lvl.sel = n - 1
	case "enter":
		return m.choose(lvl.items[lvl.sel])
	case "esc":
		m.menu = m.menu[:len(m.menu)-1]
		if len(m.menu) == 0 {
			m.mode = modeReady
		}
	default:
		text := strings.ToUpper(typed(k))
		for i, it := range lvl.items {
			if text != "" && strings.HasPrefix(strings.ToUpper(it.name), text) {
				lvl.sel = i
				return m.choose(it)
			}
		}
	}
	return nil
}

func (m *Model) choose(it menuItem) tea.Cmd {
	if it.items != nil {
		m.menu = append(m.menu, menuLevel{items: it.items})
		return nil
	}
	m.menu = nil
	m.mode = modeReady
	return it.action(m)
}

type promptKind int

const (
	promptText promptKind = iota
	promptRange
	promptWidth
)

// prompt is a question on the second control panel line, such as a file
// name or a range to act on.
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

// openRange asks for a range, pre-anchored on the current cell as in 1-2-3.
func (m *Model) openRange(label string, onRange func(*Model, sheet.Rect) tea.Cmd) {
	m.openPrompt(&prompt{kind: promptRange, label: label, indicator: "POINT", onRange: onRange}, "")
	m.point = pointer{at: m.cur, anchor: m.cur, anchored: true}
}

func (m *Model) openGoto() {
	m.openText("Enter address to go to:", m.cur.String(), func(m *Model, text string) tea.Cmd {
		a, ok := sheet.ParseAddr(strings.TrimSpace(text))
		if !ok {
			m.fail("Invalid cell address: " + text)
			return nil
		}
		m.cur = a
		return nil
	})
	m.prompt.indicator = "POINT"
}

func (m *Model) openWidth() tea.Cmd {
	col, orig := m.cur.Col, m.sheet.ColWidth(m.cur.Col)
	m.openPrompt(&prompt{
		kind:      promptWidth,
		label:     "Enter column width (1..240):",
		indicator: "POINT",
		fresh:     true,
		onText: func(m *Model, text string) tea.Cmd {
			w, err := strconv.Atoi(text)
			if err != nil || w < 1 || w > 240 {
				m.sheet.SetColWidth(col, orig)
				m.fail("Column width must be between 1 and 240")
				return nil
			}
			m.sheet.SetColWidth(col, w)
			m.changed = true
			return nil
		},
		onCancel: func(m *Model) { m.sheet.SetColWidth(col, orig) },
	}, strconv.Itoa(orig))
	return nil
}

func (m *Model) openSave() tea.Cmd {
	name := m.filename
	if name == "" {
		name = "SHEET1" + sheet.FileExt
	}
	m.openText("Enter save file name:", name, func(m *Model, text string) tea.Cmd {
		return saveCmd(m.sheet, withExt(text))
	})
	return nil
}

func (m *Model) openRetrieve() tea.Cmd {
	m.files = nil
	m.openText("Name of file to retrieve:", "", func(m *Model, text string) tea.Cmd {
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
		if m.navigate(key, &m.point.at) {
			return nil
		}
		if key == "." {
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
		m.sheet.SetColWidth(m.cur.Col, w) // live preview
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

func (m *Model) reset(s *sheet.Sheet, filename string) {
	*m = Model{sheet: s, filename: filename, width: m.width, height: m.height}
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
	var buf strings.Builder
	err := s.Write(&buf)
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
		s, err := sheet.Read(f)
		return loadedMsg{name, s, err}
	}
}

func listFilesCmd() tea.Msg {
	files, _ := filepath.Glob("*" + sheet.FileExt)
	slices.Sort(files)
	return filesMsg(files)
}

func (m *Model) handleSaved(msg savedMsg) {
	if msg.err != nil {
		m.fail(fmt.Sprintf("Cannot save %s: %v", msg.name, msg.err))
		return
	}
	m.filename = msg.name
	m.changed = false
}

func (m *Model) handleLoaded(msg loadedMsg) {
	if msg.err != nil {
		m.fail(fmt.Sprintf("Cannot retrieve %s: %v", msg.name, msg.err))
		return
	}
	m.reset(msg.sheet, msg.name)
}
