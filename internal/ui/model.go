// Package ui is the Bubble Tea front end: a 1-2-3 style control panel,
// worksheet grid, slash menu and modal key handling.
package ui

import (
	"errors"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"

	"one23/internal/sheet"
)

// mode is the state shown in the mode indicator, as in 1-2-3.
type mode int

const (
	modeReady mode = iota
	modeLabel
	modeValue
	modeEdit
	modePoint
	modeMenu
	modePrompt
	modeHelp
	modeError
)

func (m mode) String() string {
	return [...]string{"READY", "LABEL", "VALUE", "EDIT", "POINT", "MENU", "", "HELP", "ERROR"}[m]
}

// Layout: three control panel lines, the column header, the grid, and a
// status line.
const (
	panelLines = 3
	headerLine = panelLines
	gridTop    = panelLines + 1
	rowHdrW    = 6
)

// Model is the whole application state.
type Model struct {
	sheet    *sheet.Sheet
	filename string
	changed  bool

	cur           sheet.Addr // cell pointer
	top, left     int        // first visible row and column
	width, height int

	mode mode

	// buf is the edit line used by LABEL, VALUE, EDIT and text prompts.
	buf    []rune
	bufPos int
	hint   string // shown on the third panel line, e.g. a parse error

	point       pointer // POINT mode and range prompts
	pointPrefix string  // entry text before the address being pointed at

	menu   []menuLevel
	prompt *prompt
	files  []string // file list shown by File Retrieve
	errMsg string
}

// New returns a model editing s. filename may be empty.
func New(s *sheet.Sheet, filename string) *Model {
	return &Model{sheet: s, filename: filename, width: 80, height: 24}
}

// Init implements tea.Model.
func (m *Model) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		cmd = m.handleKey(msg)
	case tea.PasteMsg:
		m.handlePaste(msg.Content)
	case tea.MouseClickMsg:
		m.handleClick(msg.Mouse())
	case tea.MouseWheelMsg:
		m.handleWheel(msg.Mouse())
	case savedMsg:
		m.handleSaved(msg)
	case loadedMsg:
		m.handleLoaded(msg)
	case filesMsg:
		m.files = msg
	}
	m.scrollTo(*m.focus())
	return m, cmd
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	switch m.mode {
	case modeReady:
		return m.readyKey(k)
	case modeLabel, modeValue:
		return m.entryKey(k)
	case modeEdit:
		return m.editKey(k)
	case modePoint:
		return m.pointKey(k)
	case modeMenu:
		return m.menuKey(k)
	case modePrompt:
		return m.promptKey(k)
	case modeHelp, modeError:
		m.errMsg = ""
		m.mode = modeReady
	}
	return nil
}

func (m *Model) readyKey(k tea.KeyPressMsg) tea.Cmd {
	if m.navigate(k.String(), &m.cur) {
		return nil
	}
	switch k.String() {
	case "/", "<":
		m.openMenu()
	case "f1":
		m.mode = modeHelp
	case "f2":
		m.startEntry(modeEdit, "")
		if c := m.sheet.Cell(m.cur); c != nil {
			m.buf = []rune(c.Input)
			m.bufPos = len(m.buf)
		}
	case "f5":
		m.openGoto()
	case "delete":
		m.set(m.cur, "")
	default:
		if text := typed(k); text != "" {
			m.startEntry(entryMode(text), text)
		}
	}
	return nil
}

// navigate applies a movement key to a, returning false if key isn't one.
func (m *Model) navigate(key string, a *sheet.Addr) bool {
	rows, cols := m.visibleRows(), m.visibleCols(m.left)
	switch key {
	case "up":
		a.Row--
	case "down":
		a.Row++
	case "left", "shift+tab":
		if key == "shift+tab" {
			a.Col -= cols
			m.left -= cols
		} else {
			a.Col--
		}
	case "right", "tab":
		if key == "tab" {
			a.Col += cols
			m.left += cols
		} else {
			a.Col++
		}
	case "ctrl+left":
		a.Col -= cols
		m.left -= cols
	case "ctrl+right":
		a.Col += cols
		m.left += cols
	case "pgup":
		a.Row -= rows
		m.top -= rows
	case "pgdown":
		a.Row += rows
		m.top += rows
	case "home":
		*a = sheet.Addr{}
		m.top, m.left = 0, 0
	default:
		return false
	}
	*a = clampAddr(*a)
	m.top = clamp(m.top, 0, sheet.MaxRows-1)
	m.left = clamp(m.left, 0, sheet.MaxCols-1)
	return true
}

func (m *Model) startEntry(md mode, text string) {
	m.mode = md
	m.buf = nil
	m.bufPos = 0
	m.hint = ""
	m.insert(text)
}

func entryMode(text string) mode {
	if sheet.IsValueEntry(text) {
		return modeValue
	}
	return modeLabel
}

// entryKey handles LABEL and VALUE modes: typing appends, and a movement
// key accepts the entry and then moves, as in 1-2-3.
func (m *Model) entryKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	switch key {
	case "enter":
		m.commit()
		return nil
	case "esc":
		m.cancelEntry()
		return nil
	case "f2":
		m.mode = modeEdit
		return nil
	case "backspace":
		if m.bufPos > 0 {
			m.buf = slices.Delete(m.buf, m.bufPos-1, m.bufPos)
			m.bufPos--
		}
		if len(m.buf) == 0 {
			m.cancelEntry()
		}
		return nil
	}
	if isMoveKey(key) {
		if m.mode == modeValue && canPoint(m.buf) {
			m.mode = modePoint
			m.pointPrefix = string(m.buf)
			m.point = pointer{at: m.cur}
			m.navigate(key, &m.point.at)
			return nil
		}
		if m.commit() {
			m.navigate(key, &m.cur)
		}
		return nil
	}
	m.insert(typed(k))
	return nil
}

// editKey handles EDIT mode, where arrows move within the entry.
func (m *Model) editKey(k tea.KeyPressMsg) tea.Cmd {
	switch key := k.String(); key {
	case "enter":
		m.commit()
	case "esc":
		m.cancelEntry()
	case "up", "down", "pgup", "pgdown":
		if m.commit() {
			m.navigate(key, &m.cur)
		}
	default:
		m.lineKey(k)
	}
	return nil
}

// lineKey applies line-editing keys to buf. It reports whether the key was
// consumed.
func (m *Model) lineKey(k tea.KeyPressMsg) bool {
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
		text := typed(k)
		m.insert(text)
		return text != ""
	}
	return true
}

// pointKey handles POINT mode: arrows move a pointer whose address (or
// range, once anchored with '.') is spliced into the formula.
func (m *Model) pointKey(k tea.KeyPressMsg) tea.Cmd {
	key := k.String()
	if m.navigate(key, &m.point.at) {
		return nil
	}
	switch key {
	case ".":
		if !m.point.anchored {
			m.point.anchor, m.point.anchored = m.point.at, true
		}
	case "esc":
		if m.point.anchored {
			m.point.anchored = false
			return nil
		}
		m.resumeValue(m.pointPrefix)
	case "backspace":
		m.resumeValue(m.pointPrefix)
	case "enter":
		m.resumeValue(m.pointPrefix + m.point.text())
		m.commit()
	default:
		if text := typed(k); text != "" {
			m.resumeValue(m.pointPrefix + m.point.text() + text)
		}
	}
	return nil
}

func (m *Model) resumeValue(text string) {
	m.startEntry(modeValue, text)
}

// commit stores the edit line in the current cell. An invalid formula puts
// the user in EDIT mode with the cursor at the problem, like 1-2-3 does.
func (m *Model) commit() bool {
	input := string(m.buf)
	if err := m.set(m.cur, input); err != nil {
		var pe *sheet.ParseError
		m.mode = modeEdit
		m.hint = err.Error()
		if errors.As(err, &pe) {
			m.bufPos = utf8.RuneCountInString(input[:min(pe.Pos, len(input))])
		}
		return false
	}
	m.cancelEntry()
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
	m.buf, m.bufPos, m.hint = nil, 0, ""
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
		content = strings.TrimSpace(content)
		if content != "" {
			m.startEntry(entryMode(content), content)
		}
	case modeLabel, modeValue, modeEdit:
		m.insert(content)
	case modePrompt:
		if m.prompt.kind != promptRange {
			m.promptType(content)
		}
	}
}

func (m *Model) handleClick(mouse tea.Mouse) {
	if mouse.Button != tea.MouseLeft {
		return
	}
	a, ok := m.cellAt(mouse.X, mouse.Y)
	if !ok {
		return
	}
	switch {
	case m.mode == modeReady:
		m.cur = a
	case m.mode == modePoint, m.pointing():
		m.point.at = a
	}
}

func (m *Model) handleWheel(mouse tea.Mouse) {
	const step = 3
	switch mouse.Button {
	case tea.MouseWheelUp:
		m.top = max(m.top-step, 0)
	case tea.MouseWheelDown:
		m.top = min(m.top+step, sheet.MaxRows-m.visibleRows())
	case tea.MouseWheelLeft:
		m.left = max(m.left-1, 0)
	case tea.MouseWheelRight:
		m.left = min(m.left+1, sheet.MaxCols-1)
	default:
		return
	}
	// Keep the focused cell on screen rather than snapping back to it.
	f := m.focus()
	f.Row = clamp(f.Row, m.top, m.top+m.visibleRows()-1)
	f.Col = clamp(f.Col, m.left, m.left+m.visibleCols(m.left)-1)
}

// focus is the cell the viewport follows: the pointer while pointing,
// otherwise the cell pointer.
func (m *Model) focus() *sheet.Addr {
	if m.mode == modePoint || m.pointing() {
		return &m.point.at
	}
	return &m.cur
}

// cellAt maps a screen position to a cell.
func (m *Model) cellAt(x, y int) (sheet.Addr, bool) {
	row := y - gridTop
	if row < 0 || row >= m.visibleRows() || x < rowHdrW {
		return sheet.Addr{}, false
	}
	cx := rowHdrW
	for c := m.left; c < sheet.MaxCols; c++ {
		cx += m.sheet.ColWidth(c)
		if x < cx {
			return sheet.Addr{Col: c, Row: m.top + row}, true
		}
		if cx >= m.width {
			break
		}
	}
	return sheet.Addr{}, false
}

func (m *Model) visibleRows() int {
	return max(m.height-gridTop-1, 1)
}

// visibleCols returns how many whole columns fit starting at left.
func (m *Model) visibleCols(left int) int {
	n, w := 0, rowHdrW
	for c := left; c < sheet.MaxCols; c++ {
		w += m.sheet.ColWidth(c)
		if w > m.width {
			break
		}
		n++
	}
	return max(n, 1)
}

// scrollTo moves the viewport the minimum amount needed to show a.
func (m *Model) scrollTo(a sheet.Addr) {
	rows := m.visibleRows()
	m.top = clamp(m.top, a.Row-rows+1, a.Row)
	if a.Col < m.left {
		m.left = a.Col
	}
	for a.Col >= m.left+m.visibleCols(m.left) {
		m.left++
	}
}

func isMoveKey(key string) bool {
	switch key {
	case "up", "down", "left", "right", "pgup", "pgdown", "tab", "shift+tab", "ctrl+left", "ctrl+right", "home":
		return true
	}
	return false
}

// canPoint reports whether the entry so far ends where a cell address may
// follow, so movement keys start POINT mode instead of accepting the entry.
func canPoint(buf []rune) bool {
	if len(buf) == 0 {
		return false
	}
	return strings.ContainsRune("+-*/^(,;=<>&#", buf[len(buf)-1])
}

// typed returns the printable text of a key press, if any.
func typed(k tea.KeyPressMsg) string {
	if k.Mod.Contains(tea.ModCtrl) || k.Mod.Contains(tea.ModAlt) {
		return ""
	}
	return k.Text
}

func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

func clampAddr(a sheet.Addr) sheet.Addr {
	return sheet.Addr{
		Col: clamp(a.Col, 0, sheet.MaxCols-1),
		Row: clamp(a.Row, 0, sheet.MaxRows-1),
	}
}

// pointer is a cell or range being selected with the arrow keys.
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

func (p pointer) text() string {
	if !p.anchored {
		return p.at.String()
	}
	return p.anchor.String() + ".." + p.at.String()
}
