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
// (items) or runs a registered command (cmd). Descriptions come from the
// command unless the item overrides them.
type slashItem struct {
	name   string
	desc   string
	descFn func(m *Model) string // overrides desc when set
	items  []slashItem
	cmd    string
}

type menuLevel struct {
	items []slashItem
	sel   int
}

// rootMenu is interim: the Sheets-style menu bar replaces it.
var rootMenu = []slashItem{
	{name: "File", items: []slashItem{
		{name: "New", descFn: unsavedWarning("Start a new, empty sheet"), cmd: "file.new"},
		{name: "Open", cmd: "file.open"},
		{name: "Save", cmd: "file.save"},
		{name: "As", desc: "Save the sheet under a new name", cmd: "file.saveas"},
		{name: "Quit", cmd: "quit"},
	}},
	{name: "Edit", items: []slashItem{
		{name: "Clear", cmd: "clear"},
		{name: "Go to", cmd: "goto"},
		{name: "Select all", cmd: "select.all"},
	}},
	{name: "Format", items: []slashItem{
		{name: "Column width", cmd: "column.width"},
		{name: "Reset column width", cmd: "column.reset"},
	}},
}

// quitConfirm is shown by Quit when there are unsaved changes.
var quitConfirm = []slashItem{
	{name: "Cancel", desc: "Keep working"},
	{name: "Quit without saving", descFn: unsavedWarning("Close one23"), cmd: "quit.force"},
}

func init() {
	register(&command{id: "quit.force", title: "Quit without saving", desc: "Close one23, discarding changes", run: func(*Model) tea.Cmd { return exit() }})
}

func unsavedWarning(desc string) func(m *Model) string {
	return func(m *Model) string {
		if m.changed {
			return "You have unsaved changes. " + desc + " anyway?"
		}
		return desc
	}
}

func (it slashItem) description(m *Model) string {
	switch {
	case it.descFn != nil:
		return it.descFn(m)
	case it.items != nil:
		names := make([]string, len(it.items))
		for i, sub := range it.items {
			names[i] = sub.name
		}
		return strings.Join(names, "  ")
	case it.desc != "":
		return it.desc
	case it.cmd != "":
		return commands[it.cmd].desc
	}
	return ""
}

// slashMenuLine shows the open menu level and the highlighted item's
// description.
func (m *Model) slashMenuLine() string {
	lvl := m.menu[len(m.menu)-1]
	parts := make([]string, len(lvl.items))
	for i, it := range lvl.items {
		parts[i] = it.name
		if i == lvl.sel {
			parts[i] = m.th.menuSelected.Render(it.name)
		}
	}
	return strings.Join(parts, "  ") + "   " + m.th.muted.Render(lvl.items[lvl.sel].description(m))
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
		// A letter opens the only item starting with it, or cycles through
		// the items when several do.
		text := strings.ToUpper(typed(k))
		if text == "" {
			return nil
		}
		var matches []int
		for i, it := range lvl.items {
			if strings.HasPrefix(strings.ToUpper(it.name), text) {
				matches = append(matches, i)
			}
		}
		switch len(matches) {
		case 0:
		case 1:
			lvl.sel = matches[0]
			return m.choose(lvl.items[lvl.sel])
		default:
			next := matches[0]
			for _, i := range matches {
				if i > lvl.sel {
					next = i
					break
				}
			}
			lvl.sel = next
		}
	}
	return nil
}

func (m *Model) choose(it slashItem) tea.Cmd {
	if it.items != nil {
		m.menu = append(m.menu, menuLevel{items: it.items})
		return nil
	}
	m.menu = nil
	m.mode = modeReady
	if it.cmd == "" {
		return nil
	}
	return m.runCommand(it.cmd)
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

// openRange asks for a range, starting from the current selection.
func (m *Model) openRange(label string, onRange func(*Model, sheet.Rect) tea.Cmd) {
	r := m.selection()
	m.openPrompt(&prompt{kind: promptRange, label: label, indicator: "POINT", onRange: onRange}, "")
	m.point = pointer{anchor: r.From, at: r.To, anchored: r.From != r.To}
}

func (m *Model) openGoto() {
	m.openText("Go to:", m.cur.String(), func(m *Model, text string) tea.Cmd {
		a, ok := sheet.ParseAddr(strings.TrimSpace(text))
		if !ok {
			m.fail("Not a cell address: " + text)
			return nil
		}
		m.cur = a
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
		name = "SHEET1" + sheet.FileExt
	}
	m.openText("Save as:", name, func(m *Model, text string) tea.Cmd {
		return saveCmd(m.sheet, withExt(text))
	})
	return nil
}

func (m *Model) openRetrieve() tea.Cmd {
	m.files = nil
	m.openText("Open file:", "", func(m *Model, text string) tea.Cmd {
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

func (m *Model) reset(s *sheet.Sheet, filename string) {
	*m = Model{sheet: s, filename: filename, width: m.width, height: m.height, th: m.th}
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
		m.fail(fmt.Sprintf("Couldn't save %s: %v", msg.name, msg.err))
		return
	}
	m.filename = msg.name
	m.changed = false
}

func (m *Model) handleLoaded(msg loadedMsg) {
	if msg.err != nil {
		m.fail(fmt.Sprintf("Couldn't open %s: %v", msg.name, msg.err))
		return
	}
	m.reset(msg.sheet, msg.name)
}
