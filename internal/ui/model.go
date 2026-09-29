// Package ui is the Bubble Tea front end. Inside the grid it behaves like
// Google Sheets (keys, selection, entry); around it, the control panel and
// mode indicator give it a 1-2-3 look.
package ui

import (
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/FelineStateMachine/012/internal/confine"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
	"github.com/FelineStateMachine/012/internal/ui/findbar"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/rowtext"
	"github.com/FelineStateMachine/012/internal/ui/tabstrip"
	"github.com/FelineStateMachine/012/internal/ui/theme"
	"github.com/FelineStateMachine/012/internal/ui/transfer"
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
	menuLine    = overlay.MenuLine    // menu bar on the left, mode indicator on the right
	formulaLine = overlay.FormulaLine // name box, then the cell's contents or the entry
	contextLine = overlay.ContextLine // prompts, key hints and formula errors
	panelLines  = 3
	headerLine  = panelLines
	gridTop     = overlay.GridTop
	minRowHdrW  = 6                       // the row numbers up to 9999; see grid.hdrW
	nameBoxW    = overlay.FormulaBarX - 1 // fits most ranges, e.g. "AA100:AB200", without jumping
)

// doubleClick is the longest gap between two clicks that edits a cell.
const doubleClick = 400 * time.Millisecond

const (
	// FrameRate is how many frames a second Bubble Tea may draw, its
	// maximum (tea.WithFPS). A key's answer waits for the next frame, so
	// this halves the wait at the default 60. The renderer writes a frame
	// only when the view changed, so an idle screen costs no more.
	FrameRate = 120
	// FrameInterval is the time between frames.
	FrameInterval = time.Second / FrameRate
)

// Model is the root of the UI, the tea.Model Bubble Tea runs. It holds
// the file, the mode and the grid, owns a component for everything that
// takes input or draws a part of the screen, routes each message to the
// one it's for, and composes the screen from what they draw (panel.go,
// view.go, overlay.go).
type Model struct {
	// grid is the sheet shown, the active cell, the scroll position, the
	// window size and the selection; see grid.go.
	grid

	// The file.
	filename      string
	changed       bool
	saved         int          // the sheet's StateID when last saved or loaded
	quitAfterSave bool         // "Save and quit" is waiting for the save to finish
	disk          stamp        // the file on disk as last opened or saved here, to notice others' saves
	root          confine.Root // where file names resolve; confined when served over SSH
	start         *string      // the file a served session opens first (OpenOnStart); nil once opened
	recovered     string       // the recovery file restored into this book, removed once it's saved: recovery.go
	offerKept     bool         // offer the recovery file kept for the file once started (OfferKept)

	mode      mode
	protectOK bool   // an edit to a protected range was agreed to: protect.go
	undoOK    bool   // a change too large to undo was agreed to: undocost.go
	note      string // feedback on the last action, e.g. "Undid: clear B3"
	warn      string // like note, for something that went wrong, e.g. a macro's error
	errMsg    string // the message ERROR mode shows
	// borderLine is the line Format > Borders draws with: layoutfmt.go.
	borderLine sheet.Line
	// borderColor is the color it draws in: alignfmt.go.
	borderColor sheet.Color
	// painted is what borders drew this frame, by role: gridlines.go.
	painted map[paintKey]string
	// shaded are roles with their escape codes, for glyphs drawn one at a
	// time (bars.go); kept for a frame as painted is.
	shaded map[*lipgloss.Style]theme.Shade
	// mergeLines are the lines merges show their values on, worked out
	// once a frame (see mergeText).
	mergeLines map[sheet.Rect][2]int
	// rowScratch holds the spans of the line of cells being drawn, which
	// each line lays out in again (see layoutLine).
	rowScratch rowtext.Scratch

	// Components. Each owns its state and the handling of the input it
	// takes; Model routes messages to them and composes what they draw.
	line    lineedit.Line     // the edit line of entries, prompts and search fields: package lineedit
	entry   entry             // typing into a cell: entry.go
	point   pointer           // the cell or range pointed at in POINT mode and range prompts
	prompt  *prompt           // a question on the context line: prompt.go
	overlay overlay.Overlay   // the open menu, picker or bar, if any (modeMenu): overlay.go
	mouse   mouseState        // drags, hover and double clicks: mouse.go
	tabs    tabstrip.Strip    // the sheet tabs and where each sheet was left: package tabstrip
	find    *findbar.Bar      // the last search, reopened by Ctrl+F: package findbar
	charts  chartState        // chart commands' target: charts.go
	copied  clipboard         // what Ctrl+V pastes: clipboard.go
	trace   *trace            // precedents or dependents being shown: trace.go
	xfer    transfer.Transfer // the import running and the file imported: package transfer
	pipe    pipeState         // standard input and output, for 012 - and --pipe: pipe.go
	jev     *jevRunner        // answers JEV functions; nil without an API key: jev.go
	term    terminal          // what the terminal supports: graphics.go
	prefs   prefs             // the settings in effect and the theme chosen: prefs.go
	session session           // what outlasts the file open: the : history, the keyboard: keyboard.go

	vim    vimState    // a vim key sequence in progress: vim.go
	rec    *recorder   // a macro being recorded: macrorec.go
	macros macroState  // a macro running, and trust in the file's macros: macrorun.go
	follow followState // the linked regions followed, and trust in them: follow.go
	nb     nbState     // notebooks' views and the cells running: notebook.go, nbrun.go

	keyAt time.Time        // when the key the next frame answers was pressed, for telemetry
	spans *telemetry.Trace // the spans open, which what the model starts nests in: trace.go

	th theme.Theme
}

// New returns a model editing s. filename may be empty.
func New(s *sheet.Sheet, filename string) *Model {
	m := &Model{grid: grid{sheet: s, width: 80, height: 24}, filename: filename, th: theme.New(true), term: newTerminal(os.Getenv), charts: chartState{last: -1},
		spans: telemetry.NewTrace(telemetry.Parent{})}
	s.Book().SetTrace(m.spans)
	if filename != "" {
		m.disk = diskStamp(filename)
	}
	m.bookOpened()
	return m
}

// TraceUnder nests every span the model starts under p for good: 012
// serve's session span holds each session's commands.
func (m *Model) TraceUnder(p telemetry.Parent) { m.spans.Enter(p) }

// Init implements tea.Model. It asks the terminal for its background color
// so the theme can adapt to light terminals.
func (m *Model) Init() tea.Cmd {
	// jev.send starts any questions queued while loading the file.
	return tea.Batch(tea.RequestBackgroundColor, tea.Raw(shiftEscapeOn), m.term.probes(), m.jev.send(m.spans.Parent()), m.startupCmd(), m.startStdin(), m.startOpenCmd(), m.startNotebook(), m.syncFollowers())
}

// Update implements tea.Model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if _, key := msg.(tea.KeyPressMsg); key && m.keyAt.IsZero() && telemetry.Enabled() {
		m.keyAt = time.Now()
	}
	if _, release := msg.(tea.KeyReleaseMsg); release && !m.held() {
		return m, nil // a release matters only to a key held: keyboard.go
	}
	if cmd, ok := m.importing(msg); ok {
		return m, cmd
	}
	if m.macroBusy(msg) {
		return m, nil
	}
	if m.rec != nil {
		m.rec.acted = false
	}
	before, beforeMode := *m.focus(), m.mode
	state := m.beginUpdate(msg)
	m.vimCapture(msg)
	var cmd tea.Cmd
	if mouse, ok := msg.(tea.MouseMsg); ok {
		var handled bool
		if cmd, handled = m.shellMouse(mouse); handled {
			msg = nil // taken by the menu bar, an overlay or a notebook, not the grid
		}
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		beforeMode = -1 // keep the focus visible after a resize
	case tea.KeyPressMsg:
		cmd = m.handleKey(msg)
	case tea.PasteMsg:
		cmd = m.pasteMsg(msg.Content)
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
		if msg.err == nil && !msg.conflict {
			m.saved = m.sheet.StateID()
		}
		cmd = m.handleSaved(msg)
	case loadedMsg:
		m.handleLoaded(msg)
	case restoredMsg:
		m.restored(msg)
	case filesMsg:
		if m.prompt != nil {
			m.prompt.files = msg
		}
	case jevAnswerMsg:
		cmd = m.answerJEV(msg)
	case jevRecalcMsg:
		cmd = m.recalcAnswered()
	case transfer.ImportedMsg, transfer.TickMsg, exportedMsg:
		cmd = m.handleTransfer(msg)
	case followTickMsg, followMsg:
		cmd = m.handleFollow(msg)
	case macroCallMsg:
		cmd = m.serveMacro(msg)
	case macroDoneMsg:
		cmd = m.finishMacro(msg.r, msg.done)
	case macroEditedMsg:
		m.macroEdited(msg)
	case tea.KeyReleaseMsg:
		cmd = m.keyReleased(msg)
	case tea.KeyboardEnhancementsMsg:
		m.keyboard(msg)
	case tea.ClipboardMsg:
		if m.vim.clipPaste {
			m.pasteClipboard(msg.Content)
		}
	default:
		var ok bool
		if cmd, ok = m.notebookMsg(msg); !ok && !m.handlePrefs(msg) {
			cmd = m.term.handle(msg)
		}
	}
	// Scroll only when the focus moves, so the mouse wheel can look around
	// without the view snapping back, as in Sheets.
	m.vimSettle()
	m.endUpdate(state)
	if f := *m.focus(); f != before || m.mode != beforeMode {
		m.scrollTo(f)
	}
	m.clampView()
	// Any edit may have queued JEV questions.
	// Chart images follow any change, see graphics.go.
	// Linked regions may have come, gone or changed: follow.go.
	return m, tea.Batch(cmd, m.jev.send(m.spans.Parent()), m.term.syncImages(m.sheet, m.displayCharts, &m.th, m.spans), m.syncSixel(msg), m.syncFollowers(), m.notebookSync())
}

// beginUpdate prepares for an input event and returns the sheet's state
// before it. Each user action outside a prompt ends any run of column
// width changes in the undo history, and clears the last action's note.
func (m *Model) beginUpdate(msg tea.Msg) int {
	switch msg.(type) {
	case tea.KeyPressMsg, tea.MouseClickMsg, tea.PasteMsg:
		m.note, m.warn = "", ""
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
	m.observeRecording(state)
	if m.sheet.StateID() != state {
		m.changed = m.sheet.StateID() != m.saved
		// An edit, a sort or a filter may hide the active cell's row.
		m.cur.Row = m.visibleRow(m.cur.Row)
		if !m.copied.keep {
			m.copied.clearMark()
		}
	}
	m.copied.keep = false
}

func (m *Model) handleKey(k tea.KeyPressMsg) tea.Cmd {
	if m.mouse.drag == dragFill {
		if k.String() == "esc" {
			m.cancelFill()
		}
		return nil // the mouse is busy filling
	}
	if m.macroKey(k) || m.traceKey(k) {
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
		return m.overlay.Key(k)
	case modePrompt:
		return m.prompt.key(m, k)
	case modeError:
		m.errMsg = ""
		m.mode = modeReady
	}
	return nil
}

func (m *Model) readyKey(k tea.KeyPressMsg) tea.Cmd {
	if m.sheet.IsNotebook() {
		return m.notebookReadyKey(k)
	}
	if m.prefs.vim {
		if cmd, ok := m.vimKeyPress(k); ok {
			return cmd
		}
	}
	key := k.String()
	if m.recordMove(key) {
		m.entry.tabbing = false
		return nil
	}
	if i := barMenuFor(key); i >= 0 {
		m.showBarMenu(i)
		return nil
	}
	if id, ok := keymap[canonicalKey(key)]; ok && (!commands[id].typed || commands[id].available(m)) {
		return m.runCommand(id)
	}
	if cmd, ok := m.runShortcut(key); ok {
		return cmd
	}
	if text := typed(k); text != "" {
		m.startEntry(modeEnter, text)
	}
	return nil
}

// notebookReadyKey is a key on a notebook tab: the notebook's, or else
// the UI's (menus, other sheets, Save), which never types into a cell.
func (m *Model) notebookReadyKey(k tea.KeyPressMsg) tea.Cmd {
	if cmd, ok := m.notebookKey(k); ok {
		return cmd
	}
	key := k.String()
	if i := barMenuFor(key); i >= 0 {
		m.showBarMenu(i)
		return nil
	}
	if id, ok := keymap[canonicalKey(key)]; ok && commands[id].available(m) {
		return m.runCommand(id)
	}
	cmd, _ := m.runShortcut(key)
	return cmd
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
	case m.mouse.drag == dragFill:
		return &m.mouse.fillAt
	case m.selecting && (m.whole == wholeNone || m.vim.visual == visualRows):
		return &m.ext
	}
	return &m.cur
}

// typed returns the printable text of a key press, if any.
func typed(k tea.KeyPressMsg) string { return lineedit.Typed(k) }

func clamp(v, lo, hi int) int {
	return max(lo, min(v, hi))
}

func clampAddr(a sheet.Addr) sheet.Addr {
	return sheet.Addr{
		Col: clamp(a.Col, 0, sheet.MaxCols-1),
		Row: clamp(a.Row, 0, sheet.MaxRows-1),
	}
}

// fail shows msg in ERROR mode. A served session isn't told where its
// directory is on the server: errors name its files relative to it.
func (m *Model) fail(msg string) {
	m.mode = modeError
	m.errMsg = m.root.Scrub(msg)
}
