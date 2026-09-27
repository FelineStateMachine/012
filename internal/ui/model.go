// Package ui is the Bubble Tea front end. Inside the grid it behaves like
// Google Sheets (keys, selection, entry); around it, the control panel and
// mode indicator give it a 1-2-3 look.
package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// mode is the state shown in the mode indicator.
type mode int

const (
	modeReady mode = iota
	modeEnter      // typing a new entry, which replaces the cell
	modeEdit       // editing existing contents with a movable caret
	modePoint      // arrowing to a cell or range to insert into a formula
	modeMenu       // a menu, the palette or another overlay is open
	modePrompt
	modeError
)

func (m mode) String() string {
	return [...]string{"READY", "ENTER", "EDIT", "POINT", "MENU", "", "ERROR"}[m]
}

// Layout: three control panel lines (menu bar, formula bar, context
// line), the column header, the grid, and a status line.
const (
	menuLine    = 0 // menu bar on the left, mode indicator on the right
	formulaLine = 1 // name box, then the cell's contents or the entry
	contextLine = 2 // prompts, key hints and formula errors
	panelLines  = 3
	headerLine  = panelLines
	gridTop     = panelLines + 1
	rowHdrW     = 6
	nameBoxW    = 11 // fits most ranges, e.g. "AA100:AB200", without jumping
)

// doubleClick is the longest gap between two clicks that edits a cell.
const doubleClick = 400 * time.Millisecond

// Model is the whole application state.
type Model struct {
	sheet    *sheet.Sheet // the sheet shown; its workbook is the file
	filename string
	changed  bool
	saved    int // the sheet's StateID when last saved or loaded

	copied clipboard // see clipboard.go
	note   string    // feedback on the last action, e.g. "Undid: clear B3"

	quitAfterSave bool // "Save and quit" is waiting for the save to finish

	cur           sheet.Addr // the active cell
	top, left     int        // first visible row and column
	width, height int

	// Selection: see selection.go.
	selecting bool
	ext       sheet.Addr // the moving corner of the selection
	whole     wholeKind

	// Mouse: see mouse.go.
	drag           dragKind
	lastClick      time.Time
	lastHit        hit
	hover          hit        // what's under the mouse, for hover styling
	mouseX, mouseY int        // last mouse position, for autoscroll
	autoscrolling  bool       // an autoscroll tick is pending
	resizeCol      int        // column being resized by its header border
	fillAt         sheet.Addr // where a fill handle drag points, see fill.go
	fillTo         sheet.Rect // the range that drag would fill
	shape          string     // pointer shape last sent to the terminal

	// Sheets: see tabs.go.
	home    *sheet.Sheet           // while pointing into another sheet, the entry's sheet
	places  map[*sheet.Sheet]place // where each sheet's cursor and scroll were left
	tabLeft int                    // the first tab shown when they don't all fit

	// tabStart remembers where a run of Tab-committed entries began, so
	// Enter returns to that column on the next row, as in Sheets.
	tabStart int
	tabbing  bool

	mode mode

	// buf is the edit line used by ENTER, EDIT and text prompts.
	buf    []rune
	bufPos int
	hint   string // shown on the third panel line, e.g. a formula error

	point       pointer // POINT mode and range prompts
	pointPrefix string  // entry text before the reference being pointed at
	pointSuffix string  // entry text after the caret while pointing
	assist      assist  // function and name suggestions while typing, see assist.go

	trace *trace // precedents or dependents being shown, see trace.go

	overlay  overlay    // open menu, palette or dialog, if any (modeMenu)
	lastFind *findBar   // the last search, reopened by Ctrl+F
	jev      *jevRunner // answers JEV functions; nil without an API key
	xfer     transfer   // imports and downloads, see transfer.go
	prompt   *prompt
	files    []string // file list shown by File Open
	errMsg   string

	term      terminal // what the terminal supports, see graphics.go
	lastChart int      // the chart last selected, for chart commands; -1 for none

	keyAt time.Time // when the key the next frame answers was pressed, for telemetry

	th theme
}

// New returns a model editing s. filename may be empty.
func New(s *sheet.Sheet, filename string) *Model {
	return &Model{sheet: s, filename: filename, width: 80, height: 24, th: newTheme(true), term: newTerminal(), lastChart: -1}
}

// Init implements tea.Model. It asks the terminal for its background color
// so the theme can adapt to light terminals.
func (m *Model) Init() tea.Cmd {
	// sendJEV starts any questions queued while loading the file.
	return tea.Batch(tea.RequestBackgroundColor, tea.Raw(shiftEscapeOn), m.probes(), m.sendJEV(), m.startupCmd())
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, key := msg.(tea.KeyPressMsg); key && m.keyAt.IsZero() && telemetry.Enabled() {
		m.keyAt = time.Now()
	}
	if cmd, ok := m.importing(msg); ok {
		return m, cmd
	}
	before, beforeMode := *m.focus(), m.mode
	state := m.beginUpdate(msg)
	var cmd tea.Cmd
	if mouse, ok := msg.(tea.MouseMsg); ok {
		var handled bool
		if cmd, handled = m.shellMouse(mouse); handled {
			msg = nil // taken by the menu bar or an overlay, not the grid
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		beforeMode = -1 // keep the focus visible after a resize
	case tea.BackgroundColorMsg:
		m.th = newTheme(msg.IsDark())
	case tea.KeyPressMsg:
		cmd = m.handleKey(msg)
	case tea.PasteMsg:
		m.handlePaste(msg.Content)
	case tea.MouseClickMsg:
		cmd = m.handlePress(msg.Mouse())
	case tea.MouseMotionMsg:
		cmd = m.handleMotion(msg.Mouse())
	case tea.MouseReleaseMsg:
		cmd = m.handleRelease()
	case autoscrollMsg:
		cmd = m.handleAutoscroll()
	case tea.MouseWheelMsg:
		m.handleWheel(msg.Mouse())
	case savedMsg:
		cmd = m.handleSaved(msg)
		if msg.err == nil {
			m.saved = m.sheet.StateID()
		}
	case loadedMsg:
		m.handleLoaded(msg)
	case filesMsg:
		m.files = msg
	case jevAnswerMsg:
		busy := m.jevBusy() != ""
		m.handleJEVAnswer(msg)
		if busy && m.jevBusy() == "" {
			cmd = m.notifyDone("JEV finished answering in " + m.displayName())
		}
	case importedMsg, importTickMsg, exportedMsg:
		cmd = m.handleTransfer(msg)
	default:
		cmd = m.handleTerminal(msg)
	}
	// Scroll only when the focus moves, so the mouse wheel can look around
	// without the view snapping back, as in Sheets.
	m.endUpdate(state)
	if f := *m.focus(); f != before || m.mode != beforeMode {
		m.scrollTo(f)
	}
	m.clampView()
	// Any edit may have queued JEV questions.
	// Chart images follow any change, see graphics.go.
	return m, tea.Batch(cmd, m.sendJEV(), m.syncImages())
}

// beginUpdate prepares for an input event and returns the sheet's state
// before it. Each user action outside a prompt ends any run of column
// width changes in the undo history, and clears the last action's note.
func (m *Model) beginUpdate(msg tea.Msg) int {
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseClickMsg, tea.PasteMsg:
		m.note = ""
		if _, key := msg.(tea.KeyPressMsg); !key {
			m.trace = nil // keys end a trace in handleKey
		}
		if m.mode != modePrompt {
			m.sheet.Seal()
		}
	}
	return m.sheet.StateID()
}

// endUpdate reacts to edits made while handling an event: the modified
// flag follows the undo history, and any edit but a paste clears the copy
// marker, as in Sheets.
func (m *Model) endUpdate(state int) {
	if m.sheet.StateID() != state {
		m.changed = m.sheet.StateID() != m.saved
		// An edit, a sort or a filter may hide the active cell's row.
		m.cur.Row = m.visibleRow(m.cur.Row)
		if !m.copied.keep {
			m.clearCopyMark()
		}
	}
	m.copied.keep = false
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	if m.drag == dragFill {
		if k.String() == "esc" {
			m.cancelFill()
		}
		return nil // the mouse is busy filling
	}
	if m.traceKey(k) {
		return nil
	}
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
		return m.overlay.key(m, k)
	case modePrompt:
		return m.promptKey(k)
	case modeError:
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
	if i := barMenuFor(key); i >= 0 {
		m.showBarMenu(i)
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
	rows, cols := m.scrollRows(), m.visibleCols(m.left)
	switch key {
	case "up":
		a.Row = m.stepRow(a.Row, -1)
	case "down":
		a.Row = m.stepRow(a.Row, 1)
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
		a.Row = m.stepRow(a.Row, -rows)
		m.top = m.stepRow(m.top, -rows)
	case "pgdown":
		a.Row = m.stepRow(a.Row, rows)
		m.top = m.stepRow(m.top, rows)
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
	a.Row = m.visibleRow(a.Row)
	// Moving into the frozen panes by keyboard scrolls the rest back to
	// the start, as in Sheets (Ctrl+Home shows A1 with row 2 under it).
	if fr, fc := m.frozen(); a.Row < fr || a.Col < fc {
		if a.Row < fr {
			m.top = 0
		}
		if a.Col < fc {
			m.left = 0
		}
	}
	m.clampView()
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
		m.top = m.stepRow(m.top, -step)
	case tea.MouseWheelDown:
		m.top = min(m.stepRow(m.top, step), sheet.MaxRows-m.scrollRows())
	case tea.MouseWheelLeft:
		m.left--
	case tea.MouseWheelRight:
		m.left = min(m.left+1, sheet.MaxCols-1)
	}
	m.clampView()
}

// focus is the cell the viewport follows: the pointer while pointing, the
// moving corner while extending a selection, otherwise the active cell.
func (m *Model) focus() *sheet.Addr {
	switch {
	case m.mode == modePoint || m.pointing() || m.away():
		return &m.point.at
	case m.drag == dragFill:
		return &m.fillAt
	case m.selecting && m.whole == wholeNone:
		return &m.ext
	}
	return &m.cur
}

func (m *Model) visibleRows() int {
	return max(m.height-gridTop-1, 1)
}

// visibleCols returns how many whole scrolling columns fit starting at
// left.
func (m *Model) visibleCols(left int) int {
	n, w := 0, m.scrollX()
	for c := left; c < sheet.MaxCols; c++ {
		w += m.sheet.ColWidth(c)
		if w > m.width {
			break
		}
		n++
	}
	return max(n, 1)
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
