package ui

import (
	"strings"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/macro"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
)

// Entry follows Google Sheets: typing replaces the cell (ENTER), Enter or
// F2 edits it in place (EDIT). Enter commits and moves down, Tab commits
// and moves right, Esc cancels. While typing a formula, arrow keys after an
// operator point at cells (POINT) and insert their references. The
// selection stays while typing, so Ctrl+Enter can fill it.

// entry is the state of typing into a cell, beyond the text itself (in
// Model.line) and the cell or range pointed at (Model.point).
type entry struct {
	hint string // shown on the context line, e.g. a formula error

	// While pointing, the entry's text before and after the reference
	// being pointed at.
	prefix, suffix string

	// home is the entry's sheet while the pointer is on another one; see
	// tabs.go.
	home *sheet.Sheet

	assist assist // function and name suggestions: assist.go

	// tabStart remembers where a run of Tab-committed entries began, so
	// Enter returns to that column on the next row, as in Sheets.
	tabStart int
	tabbing  bool
}

func (m *Model) startEntry(md mode, text string) {
	if m.refuseEdit(sheet.Rect{From: m.cur, To: m.cur}, false) {
		return
	}
	if m.askProtected(sheet.Rect{From: m.cur, To: m.cur}, func(m *Model) tea.Cmd {
		m.startEntry(md, text)
		return nil
	}) {
		return
	}
	m.mode = md
	m.entry.home = nil
	m.line.Clear()
	m.entry.hint, m.entry.assist = "", assist{}
	m.line.Insert(text)
}

// startEdit edits the active cell's contents with the caret at the end.
func (m *Model) startEdit() tea.Cmd {
	m.startEntry(modeEdit, "")
	if c := m.sheet.Cell(m.cur); c != nil && m.mode == modeEdit {
		m.line.Set(m.shownEntry(c.Input))
	}
	return nil
}

// enterKey handles ENTER mode. Arrow keys accept the entry and move, except
// in a formula where a reference may follow the caret: then they point.
func (m *Model) enterKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if m.entry.assist.key(m, key) || m.commitKey(key) || m.cancelKey(key) || m.sheetKey(key) {
		return nil
	}
	if key == "f2" {
		m.mode = modeEdit
		return nil
	}
	if key == "f4" {
		m.toggleAbsolute()
		return nil
	}
	if m.line.IsFormula() && (m.pointAfterSheet(key) || m.canPoint() && m.startPoint(key)) {
		return nil
	}
	if m.line.IsFormula() && (key == "left" || key == "right") {
		m.typeKey(k) // move the caret within a formula
		return nil
	}
	if isMoveKey(key) {
		if m.commit() {
			m.navigate(key, &m.cur)
		}
		return nil
	}
	m.typeKey(k)
	return nil
}

// editKey handles EDIT mode, where left and right move the caret.
func (m *Model) editKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if m.entry.assist.key(m, key) || m.commitKey(key) || m.cancelKey(key) || m.sheetKey(key) {
		return nil
	}
	if m.line.IsFormula() && strings.HasPrefix(key, "shift+") && (m.pointAfterSheet(key) || m.canPoint() && m.startPoint(key)) {
		return nil
	}
	switch key {
	case "f4":
		m.toggleAbsolute()
	case "up", "down", "pgup", "pgdown":
		if m.commit() {
			m.navigate(key, &m.cur)
		}
	default:
		m.typeKey(k)
	}
	return nil
}

// commitKey handles the keys that accept an entry, moving afterwards the
// way Sheets does. It reports whether key was one of them.
func (m *Model) commitKey(key string) bool {
	switch key {
	case "enter", "shift+enter", "tab", "shift+tab", "ctrl+enter":
	default:
		return false
	}
	if key == "ctrl+enter" && m.hasRange() {
		m.fillEntry()
		return true
	}
	if !m.commit() {
		return true
	}
	switch key {
	case "enter":
		if m.entry.tabbing {
			m.cur.Col = m.entry.tabStart
			m.entry.tabbing = false
		}
		m.cur.Row++
	case "shift+enter":
		m.cur.Row--
	case "tab":
		if !m.entry.tabbing {
			m.entry.tabStart, m.entry.tabbing = m.cur.Col, true
		}
		m.cur.Col++
	case "shift+tab":
		m.cur.Col--
	}
	m.cur = clampAddr(m.cur)
	return true
}

func (m *Model) cancelKey(key string) bool {
	if key != "esc" {
		return false
	}
	m.cancelEntry()
	return true
}

// startPoint enters POINT mode if key moves (Shift extends). The pointer
// starts at the active cell, or on another sheet where it last pointed.
func (m *Model) startPoint(key string) bool {
	from := m.cur
	if m.away() {
		from = m.point.at
	}
	if base, ok := extendKey(key); ok {
		key = base
		m.point = pointer{at: from, anchor: from, anchored: true}
	} else if isMoveKey(key) && key != "tab" && key != "shift+tab" {
		m.point = pointer{at: from}
	} else {
		return false
	}
	m.entry.prefix = m.line.Head()
	m.entry.suffix = m.line.Tail()
	m.mode = modePoint
	m.navigate(key, &m.point.at)
	return true
}

// pointKey handles POINT mode. Arrows move the pointer, Shift+arrows
// extend it to a range, and anything else puts the reference into the
// formula and carries on typing.
func (m *Model) pointKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if m.pointMoveKey(key) || m.sheetKey(key) {
		return nil
	}
	switch key {
	case "esc", "backspace":
		m.resumeEntry("")
		return nil
	case "f4":
		m.resumeEntry(m.pointRef())
		m.toggleAbsolute()
		return nil
	case "enter", "shift+enter", "tab", "shift+tab", "ctrl+enter":
		m.resumeEntry(m.pointRef())
		m.commitKey(key)
		return nil
	case ":":
		if !m.point.anchored {
			m.point.anchor, m.point.anchored = m.point.at, true
			return nil
		}
	}
	if text := typed(k); text != "" {
		m.resumeEntry(m.pointRef())
		m.line.Insert(text)
		m.entry.assist = assist{active: true}
	}
	return nil
}

// resumeEntry leaves POINT mode, inserting ref at the caret.
func (m *Model) resumeEntry(ref string) {
	m.mode = modeEnter
	m.line.Buf = []rune(m.entry.prefix + ref + m.entry.suffix)
	m.line.Pos = utf8.RuneCountInString(m.entry.prefix + ref)
}

// commit stores the edit line in the active cell. An invalid formula stays
// in EDIT mode with the caret at the problem and the reason on line 3.
func (m *Model) commit() bool {
	input := m.storedEntry(m.line.Text())
	warn, ok := m.checkEntry(input) // data validation, rules.go
	if !ok {
		return false
	}
	if err := m.set(m.cur, input); err != nil {
		m.entryError(err, input)
		return false
	}
	m.cancelEntry()
	m.clearSelection()
	m.recordEntry(input, false)
	if warn != "" {
		m.warn = warn
	}
	return true
}

func (m *Model) set(a sheet.Addr, input string) error {
	if err := m.entrySheet().Set(a, input); err != nil {
		return err
	}
	m.changed = true
	return nil
}

// cancelEntry ends the entry, showing its sheet again if the pointer
// went into another.
func (m *Model) cancelEntry() {
	m.returnHome()
	m.mode = modeReady
	m.line.Clear()
	m.entry.hint, m.entry.assist = "", assist{}
}

func (m *Model) handlePaste(content string) {
	switch m.mode {
	case modeReady:
		if m.askProtected(pasteTextRange(m.cur, content), func(m *Model) tea.Cmd {
			m.handlePaste(content)
			return nil
		}) {
			return
		}
		m.recordFlush()
		if m.pasteText(content) {
			m.record(macro.Call("paste_text", content))
			return
		}
		if content = strings.TrimSpace(content); content != "" {
			m.startEntry(modeEnter, content)
		}
	case modeEnter, modeEdit:
		m.line.Insert(content)
	case modePrompt:
		if m.prompt.kind != promptRange {
			m.prompt.paste(m, content)
		}
	case modeMenu:
		if o, ok := m.overlay.(overlay.Text); ok {
			m.line.Insert(content)
			o.Changed()
		}
	}
}

// pointer is a cell or range being selected in POINT mode or a range
// prompt.
type pointer struct {
	at, anchor sheet.Addr
	anchored   bool
}

func (p pointer) rect() sheet.Rect {
	if !p.anchored {
		return sheet.Rect{From: p.at, To: p.at}
	}
	return sheet.NewRect(p.anchor, p.at)
}

// text is the reference as inserted into a formula, e.g. B3 or B3:B5.
func (p pointer) text() string {
	return p.rect().String()
}
