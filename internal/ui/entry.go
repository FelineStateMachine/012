package ui

import (
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Entry follows Google Sheets: typing replaces the cell (ENTER), Enter or
// F2 edits it in place (EDIT). Enter commits and moves down, Tab commits
// and moves right, Esc cancels. While typing a formula, arrow keys after an
// operator point at cells (POINT) and insert their references. The
// selection stays while typing, so Ctrl+Enter can fill it.

func (m *Model) startEntry(md mode, text string) {
	m.mode = md
	m.buf, m.bufPos, m.hint, m.assist = nil, 0, "", assist{}
	m.insert(text)
}

// startEdit edits the active cell's contents with the caret at the end.
func (m *Model) startEdit() tea.Cmd {
	m.startEntry(modeEdit, "")
	if c := m.sheet.Cell(m.cur); c != nil {
		m.buf = []rune(c.Input)
		m.bufPos = len(m.buf)
	}
	return nil
}

// isFormula reports whether the edit line holds a formula.
func (m *Model) isFormula() bool {
	return len(m.buf) > 0 && (m.buf[0] == '=' || m.buf[0] == '+' || m.buf[0] == '-')
}

// enterKey handles ENTER mode. Arrow keys accept the entry and move, except
// in a formula where a reference may follow the caret: then they point.
func (m *Model) enterKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if m.assistKey(key) || m.commitKey(key) || m.cancelKey(key) {
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
	if m.isFormula() && m.canPoint() && m.startPoint(key) {
		return nil
	}
	if m.isFormula() && (key == "left" || key == "right") {
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
	if m.assistKey(key) || m.commitKey(key) || m.cancelKey(key) {
		return nil
	}
	if m.isFormula() && m.canPoint() && strings.HasPrefix(key, "shift+") && m.startPoint(key) {
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
		if m.tabbing {
			m.cur.Col = m.tabStart
			m.tabbing = false
		}
		m.cur.Row++
	case "shift+enter":
		m.cur.Row--
	case "tab":
		if !m.tabbing {
			m.tabStart, m.tabbing = m.cur.Col, true
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

// lineKey applies line-editing keys to buf.
func (m *Model) lineKey(k tea.KeyPressMsg) {
	switch k.String() {
	case "left":
		m.bufPos = max(m.bufPos-1, 0)
	case "right":
		m.bufPos = min(m.bufPos+1, len(m.buf))
	case "home", "ctrl+a":
		m.bufPos = 0
	case "end", "ctrl+e":
		m.bufPos = len(m.buf)
	case "backspace":
		if m.bufPos > 0 {
			m.buf = slices.Delete(m.buf, m.bufPos-1, m.bufPos)
			m.bufPos--
		}
	case "delete":
		if m.bufPos < len(m.buf) {
			m.buf = slices.Delete(m.buf, m.bufPos, m.bufPos+1)
		}
	default:
		m.insert(typed(k))
	}
}

// canPoint reports whether the text before the caret ends where a cell
// reference may follow.
func (m *Model) canPoint() bool {
	if m.bufPos == 0 {
		return false
	}
	return strings.ContainsRune("=+-*/^(,;<>&:", m.buf[m.bufPos-1])
}

// startPoint enters POINT mode if key moves (Shift extends). The pointer
// starts at the active cell.
func (m *Model) startPoint(key string) bool {
	if base, ok := extendKey(key); ok {
		key = base
		m.point = pointer{at: m.cur, anchor: m.cur, anchored: true}
	} else if isMoveKey(key) && key != "tab" && key != "shift+tab" {
		m.point = pointer{at: m.cur}
	} else {
		return false
	}
	m.pointPrefix = string(m.buf[:m.bufPos])
	m.pointSuffix = string(m.buf[m.bufPos:])
	m.mode = modePoint
	m.navigate(key, &m.point.at)
	return true
}

// pointKey handles POINT mode. Arrows move the pointer, Shift+arrows
// extend it to a range, and anything else puts the reference into the
// formula and carries on typing.
func (m *Model) pointKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if m.pointMoveKey(key) {
		return nil
	}
	switch key {
	case "esc", "backspace":
		m.resumeEntry("")
		return nil
	case "f4":
		m.resumeEntry(m.point.text())
		m.toggleAbsolute()
		return nil
	case "enter", "shift+enter", "tab", "shift+tab", "ctrl+enter":
		m.resumeEntry(m.point.text())
		m.commitKey(key)
		return nil
	case ":":
		if !m.point.anchored {
			m.point.anchor, m.point.anchored = m.point.at, true
			return nil
		}
	}
	if text := typed(k); text != "" {
		m.resumeEntry(m.point.text())
		m.insert(text)
		m.assist = assist{active: true}
	}
	return nil
}

// resumeEntry leaves POINT mode, inserting ref at the caret.
func (m *Model) resumeEntry(ref string) {
	m.mode = modeEnter
	m.buf = []rune(m.pointPrefix + ref + m.pointSuffix)
	m.bufPos = utf8.RuneCountInString(m.pointPrefix + ref)
}

// commit stores the edit line in the active cell. An invalid formula stays
// in EDIT mode with the caret at the problem and the reason on line 3.
func (m *Model) commit() bool {
	input := string(m.buf)
	if err := m.set(m.cur, input); err != nil {
		m.entryError(err, input)
		return false
	}
	m.cancelEntry()
	m.clearSelection()
	return true
}

func (m *Model) set(a sheet.Addr, input string) error {
	if err := m.sheet.Set(a, input); err != nil {
		return err
	}
	m.changed = true
	return nil
}

func (m *Model) cancelEntry() {
	m.mode = modeReady
	m.buf, m.bufPos, m.hint, m.assist = nil, 0, "", assist{}
}

func (m *Model) insert(text string) {
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' {
			r = ' '
		}
		if !unicode.IsPrint(r) {
			continue
		}
		m.buf = slices.Insert(m.buf, m.bufPos, r)
		m.bufPos++
	}
}

func (m *Model) handlePaste(content string) {
	switch m.mode {
	case modeReady:
		if m.pasteText(content) {
			return
		}
		if content = strings.TrimSpace(content); content != "" {
			m.startEntry(modeEnter, content)
		}
	case modeEnter, modeEdit:
		m.insert(content)
	case modePrompt:
		if m.prompt.kind != promptRange {
			m.promptType(content)
		}
	case modeMenu:
		if o, ok := m.overlay.(textOverlay); ok {
			m.insert(content)
			o.changed(m)
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
