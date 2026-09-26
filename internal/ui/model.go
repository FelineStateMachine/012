// Package ui is the Bubble Tea front end. Inside the grid it behaves like
// Google Sheets (keys, selection, entry); around it, the control panel and
// mode indicator give it a 1-2-3 look.
package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"one23/internal/sheet"
)

// mode is the state shown in the mode indicator.
type mode int

const (
	modeReady mode = iota
	modeEnter      // typing a new entry, which replaces the cell
	modeEdit       // editing existing contents with a movable caret
	modePoint      // arrowing to a cell or range to insert into a formula
	modeMenu
	modePrompt
	modeHelp
	modeError
)

func (m mode) String() string {
	return [...]string{"READY", "ENTER", "EDIT", "POINT", "MENU", "", "HELP", "ERROR"}[m]
}

// Layout: three control panel lines, the column header, the grid, and a
// status line.
const (
	panelLines = 3
	headerLine = panelLines
	gridTop    = panelLines + 1
	rowHdrW    = 6
)

// doubleClick is the longest gap between two clicks that edits a cell.
const doubleClick = 400 * time.Millisecond

// Model is the whole application state.
type Model struct {
	sheet    *sheet.Sheet
	filename string
	changed  bool

	cur           sheet.Addr // the active cell
	top, left     int        // first visible row and column
	width, height int

	// Selection: see selection.go.
	selecting bool
	ext       sheet.Addr // the moving corner of the selection
	whole     wholeKind
	drag      dragKind
	lastClick time.Time
	lastAddr  sheet.Addr

	// tabStart remembers where a run of Tab-committed entries began, so
	// Enter returns to that column on the next row, as in Sheets.
	tabStart int
	tabbing  bool

	mode mode

	// buf is the edit line used by ENTER, EDIT and text prompts.
	buf    []rune
	bufPos int
	hint   string // shown on the third panel line, e.g. a formula error
	note   string // what the last command did, until the next key

	point       pointer // POINT mode and range prompts
	pointPrefix string  // entry text before the reference being pointed at
	pointSuffix string  // entry text after the caret while pointing

	menu   []menuLevel
	prompt *prompt
	files  []string // file list shown by File Open
	errMsg string

	th theme
}

// New returns a model editing s. filename may be empty.
func New(s *sheet.Sheet, filename string) *Model {
	return &Model{sheet: s, filename: filename, width: 80, height: 24, th: newTheme(true)}
}

// Init implements tea.Model. It asks the terminal for its background color
// so the theme can adapt to light terminals.
func (m *Model) Init() tea.Cmd { return tea.RequestBackgroundColor }

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	before, beforeMode := *m.focus(), m.mode
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		beforeMode = -1 // keep the focus visible after a resize
	case tea.BackgroundColorMsg:
		m.th = newTheme(msg.IsDark())
	case tea.KeyPressMsg:
		m.note = ""
		cmd = m.handleKey(msg)
	case tea.PasteMsg:
		m.handlePaste(msg.Content)
	case tea.MouseClickMsg:
		m.handleClick(msg.Mouse())
	case tea.MouseMotionMsg:
		m.handleMotion(msg.Mouse())
	case tea.MouseReleaseMsg:
		m.handleRelease()
	case tea.MouseWheelMsg:
		m.handleWheel(msg.Mouse())
	case savedMsg:
		m.handleSaved(msg)
	case loadedMsg:
		m.handleLoaded(msg)
	case filesMsg:
		m.files = msg
	}
	// Scroll only when the focus moves, so the mouse wheel can look around
	// without the view snapping back, as in Sheets.
	if f := *m.focus(); f != before || m.mode != beforeMode {
		m.scrollTo(f)
	}
	return m, cmd
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	switch m.mode {
	case modeReady:
		return m.readyKey(k)
	case modeEnter:
		return m.enterKey(k)
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
	key := k.String()
	if m.moveKey(key) {
		m.tabbing = false
		return nil
	}
	if id, ok := keymap[canonicalKey(key)]; ok {
		return m.runCommand(id)
	}
	if text := typed(k); text != "" {
		m.startEntry(modeEnter, text)
	}
	return nil
}

// navigate applies a movement key to a, returning false if it isn't one.
// Keys follow Google Sheets.
func (m *Model) navigate(key string, a *sheet.Addr) bool {
	rows, cols := m.visibleRows(), m.visibleCols(m.left)
	switch key {
	case "up":
		a.Row--
	case "down":
		a.Row++
	case "left", "shift+tab":
		a.Col--
	case "right", "tab":
		a.Col++
	case "ctrl+up":
		*a = m.sheet.Edge(*a, 0, -1)
	case "ctrl+down":
		*a = m.sheet.Edge(*a, 0, 1)
	case "ctrl+left":
		*a = m.sheet.Edge(*a, -1, 0)
	case "ctrl+right", "end":
		*a = m.sheet.Edge(*a, 1, 0)
	case "pgup":
		a.Row -= rows
		m.top -= rows
	case "pgdown":
		a.Row += rows
		m.top += rows
	case "alt+pgup":
		a.Col -= cols
		m.left -= cols
	case "alt+pgdown":
		a.Col += cols
		m.left += cols
	case "home":
		a.Col = 0
	case "ctrl+home":
		*a = sheet.Addr{}
	case "ctrl+end":
		used, _ := m.sheet.UsedRange()
		*a = used.To
	default:
		return false
	}
	*a = clampAddr(*a)
	m.top = clamp(m.top, 0, sheet.MaxRows-1)
	m.left = clamp(m.left, 0, sheet.MaxCols-1)
	return true
}

func isMoveKey(key string) bool {
	switch key {
	case "up", "down", "left", "right", "tab", "shift+tab", "pgup", "pgdown",
		"alt+pgup", "alt+pgdown", "home", "end", "ctrl+home", "ctrl+end",
		"ctrl+up", "ctrl+down", "ctrl+left", "ctrl+right":
		return true
	}
	return false
}

func (m *Model) handleWheel(mouse tea.Mouse) {
	const step = 3
	button := mouse.Button
	if mouse.Mod.Contains(tea.ModShift) { // Shift+wheel scrolls sideways
		switch button {
		case tea.MouseWheelUp:
			button = tea.MouseWheelLeft
		case tea.MouseWheelDown:
			button = tea.MouseWheelRight
		}
	}
	switch button {
	case tea.MouseWheelUp:
		m.top = max(m.top-step, 0)
	case tea.MouseWheelDown:
		m.top = min(m.top+step, sheet.MaxRows-m.visibleRows())
	case tea.MouseWheelLeft:
		m.left = max(m.left-1, 0)
	case tea.MouseWheelRight:
		m.left = min(m.left+1, sheet.MaxCols-1)
	}
}

// focus is the cell the viewport follows: the pointer while pointing, the
// moving corner while extending a selection, otherwise the active cell.
func (m *Model) focus() *sheet.Addr {
	switch {
	case m.mode == modePoint || m.pointing():
		return &m.point.at
	case m.selecting && m.whole == wholeNone:
		return &m.ext
	}
	return &m.cur
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
