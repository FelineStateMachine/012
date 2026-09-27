package ui

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// The command line (: with vim keys, or from the palette) takes vim's
// file commands (:w, :q, :wq, :x, :e file), a cell, range, named range or
// row number to go to (:B12, :Sheet2!A1, :40), or any registered command
// by its ID or title (:edit.fill_down, :fill down). Completions come from
// the command registry as you type, in a box under the context line, so
// there's no second list of commands to keep up.

type cmdLine struct {
	m *Model // the model it acts on
	overlay.List
	shown  []cmdItem
	tabbed bool // the text is a completion Tab put there; Tab again moves on
}

// cmdItem is a completion: a file command or a registered command.
type cmdItem struct {
	word  string // what Tab puts on the line: "w", or a command's ID
	title string
	desc  string
	key   string // the command's shortcut
	off   bool   // unavailable right now
}

// fileWords are vim's file commands.
var fileWords = []cmdItem{
	{word: "w", title: "Write", desc: "Save the sheet; :w name saves it as name, :w name.csv downloads it"},
	{word: "q", title: "Quit", desc: "Close 012, asking about unsaved changes"},
	{word: "q!", title: "Quit without saving", desc: "Close 012, discarding unsaved changes"},
	{word: "wq", title: "Write and quit", desc: "Save the sheet, then close 012"},
	{word: "x", title: "Write if changed and quit", desc: "Save the sheet if it changed, then close 012"},
	{word: "e", title: "Edit a file", desc: "Open a sheet or import a file: :e name"},
}

const cmdLineID = "cmdline"

func init() {
	register(&command{id: "vim.command", macro: macroNever, title: "Command line",
		desc: "Type a command: a cell to go to (B12), w, q, wq, e file, or any command by name",
		run: func(m *Model) tea.Cmd {
			c := &cmdLine{m: m}
			m.line.Clear()
			m.openOverlay(c)
			c.Changed()
			return nil
		}})
}

func (c *cmdLine) Indicator() string { return "COMMAND" }

// Changed completes what's typed: file commands, then commands whose ID
// or title starts with it, then those that contain it. Nothing is
// completed once a file command's argument follows.
func (c *cmdLine) Changed() {
	m := c.m
	c.Sel, c.Top, c.tabbed = 0, 0, false
	c.shown = c.shown[:0]
	q := strings.ToLower(strings.TrimPrefix(strings.TrimLeft(m.line.Text(), " "), ":"))
	if word, _, arg := strings.Cut(q, " "); arg && isFileWord(word) {
		return
	}
	for _, w := range fileWords {
		if strings.HasPrefix(w.word, q) {
			c.shown = append(c.shown, w)
		}
	}
	var starts, contains []cmdItem
	for _, id := range commandIDs() {
		cm := commands[id]
		title := strings.ToLower(cm.title)
		it := cmdItem{word: id, title: cm.title, desc: cm.desc, key: m.shortcut(id), off: !cm.available(m)}
		switch {
		case id == "vim.command":
		case strings.HasPrefix(id, q) || strings.HasPrefix(title, q):
			starts = append(starts, it)
		case strings.Contains(id, q) || strings.Contains(title, q):
			contains = append(contains, it)
		}
	}
	c.shown = append(append(c.shown, starts...), contains...)
}

// isFileWord reports whether word is one of vim's file commands, which
// take an argument.
func isFileWord(word string) bool {
	return slices.ContainsFunc(fileWords, func(it cmdItem) bool { return it.word == strings.TrimSuffix(word, "!") })
}

// commandIDs are the registered commands' IDs, by title.
func commandIDs() []string {
	ids := make([]string, 0, len(commands))
	for id := range commands {
		ids = append(ids, id)
	}
	slices.SortFunc(ids, func(a, b string) int {
		return cmp.Or(cmp.Compare(commands[a].title, commands[b].title), cmp.Compare(a, b))
	})
	return ids
}

func (c *cmdLine) Key(k tea.KeyPressMsg) tea.Cmd {
	m := c.m
	switch key := k.String(); {
	case key == "esc", key == "backspace" && len(m.line.Buf) == 0:
		m.closeOverlay()
	case key == "enter":
		return c.run(m)
	case key == "tab":
		c.complete(m, 1)
	case key == "shift+tab":
		c.complete(m, -1)
	case key == "up", key == "ctrl+p":
		c.Move(-1, len(c.shown))
	case key == "down", key == "ctrl+n":
		c.Move(1, len(c.shown))
	default:
		before := m.line.Text()
		m.line.Key(k)
		if m.line.Text() != before {
			c.Changed()
		}
	}
	return nil
}

// complete puts the highlighted completion on the line; pressed again,
// it moves on to the next one (d = 1) or the previous one (d = -1).
func (c *cmdLine) complete(m *Model, d int) {
	if len(c.shown) == 0 {
		return
	}
	if c.tabbed {
		c.Move(d, len(c.shown))
	}
	m.line.Set(c.shown[c.Sel].word)
	c.tabbed = true
}

// run closes the line and carries it out. Text that isn't a command,
// cell or range runs the highlighted completion, so ":fill d" Enter
// fills down.
func (c *cmdLine) run(m *Model) tea.Cmd {
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(m.line.Text()), ":"))
	fallback := ""
	if c.Sel < len(c.shown) {
		fallback = c.shown[c.Sel].word
	}
	m.closeOverlay()
	if text == "" {
		return nil
	}
	if cmd, ok := m.runCmdLine(text); ok {
		return cmd
	}
	if fallback != "" {
		if cmd, ok := m.runCmdLine(fallback); ok {
			return cmd
		}
	}
	m.fail("Not a command, cell or range: " + text)
	return nil
}

// runCmdLine carries out a command line and reports whether it was one.
func (m *Model) runCmdLine(text string) (tea.Cmd, bool) {
	word, arg, _ := strings.Cut(text, " ")
	arg = strings.TrimSpace(arg)
	bang := strings.HasSuffix(word, "!")
	switch strings.TrimSuffix(word, "!") {
	case "w":
		return m.writeTo(arg), true
	case "q":
		if bang {
			return m.exit(), true
		}
		return m.runCommand("quit"), true
	case "wq", "x":
		if word == "x" && !m.changed && arg == "" {
			return m.exit(), true
		}
		m.quitAfterSave = true
		return m.writeTo(arg), true
	case "e":
		return m.editFile(arg, bang), true
	}
	if n, err := strconv.Atoi(text); err == nil {
		m.clearSelection()
		m.cur.Row = m.visibleRow(clamp(n-1, 0, sheet.MaxRows-1))
		return nil, true
	}
	if m.gotoText(text) {
		return nil, true
	}
	id, ok := commandNamed(text)
	if !ok {
		return nil, false
	}
	if c := commands[id]; !c.available(m) {
		m.note = c.title + " isn't available now"
		return nil, true
	}
	return m.runCommand(id), true
}

// commandNamed finds a command by its ID or title, in any case.
func commandNamed(text string) (string, bool) {
	ids := commandIDs()
	for _, id := range ids {
		if strings.EqualFold(id, text) {
			return id, true
		}
	}
	for _, id := range ids {
		if strings.EqualFold(commands[id].title, text) {
			return id, true
		}
	}
	return "", false
}

// writeTo is :w. Without a name it saves, as File > Save; with one it
// saves as that name, as File > Save as, or downloads when the name is
// another format's (:w out.csv), as File > Download. It goes the menus'
// ways, so files are named, checked and replaced the same.
func (m *Model) writeTo(name string) tea.Cmd {
	if name == "" {
		return m.runCommand("file.save")
	}
	k, ok := fileio.KindOf(name)
	switch {
	case !ok:
		return m.saveAsFile(name)
	case !k.CanExport():
		m.quitAfterSave = false
		m.fail("012 can't write " + k.String() + " files")
		return nil
	}
	m.quitAfterSave = false // a download isn't the sheet's file
	return m.downloadFile(k, sheet.Rect{}, name)
}

// editFile is :e name: open a sheet or import a file, refusing to drop
// unsaved changes unless forced with :e!.
func (m *Model) editFile(name string, force bool) tea.Cmd {
	switch {
	case name == "":
		return m.runCommand("file.open")
	case m.changed && !force:
		m.fail("Unsaved changes: :w saves them, :e! " + name + " discards them")
		return nil
	}
	return m.openFile(name)
}

func (c *cmdLine) ContextLine() (string, string) {
	m := c.m
	return m.th.Title.Render(":") + m.line.Text(), ""
}

func (c *cmdLine) Cursor() (int, int) {
	m := c.m
	return 1 + ansi.StringWidth(m.line.Head()), contextLine
}

func (c *cmdLine) Status() (string, string) {
	m := c.m
	keys := m.th.KeyHints("Tab", "complete", "Enter", "run", "Esc", "cancel")
	if c.Sel >= len(c.shown) {
		return "", keys
	}
	it := c.shown[c.Sel]
	if it.off {
		return it.desc + " (not available now)", keys
	}
	return it.desc, keys
}

// rows is how many completions show.
func (c *cmdLine) rows(m *Model) int {
	return max(min(len(c.shown), 8, m.height-contextLine-4), 0)
}

// Layout draws the completions in a box under the context line: title,
// what Tab types, and the shortcut.
func (c *cmdLine) Layout() []overlay.Box {
	m := c.m
	rows := c.rows(m)
	if rows == 0 {
		return nil
	}
	c.Show(rows)
	tw, ww, kw := 0, 0, 0
	for _, it := range c.shown {
		tw = max(tw, ansi.StringWidth(it.title))
		ww = max(ww, ansi.StringWidth(it.word))
		if it.key != "" {
			kw = max(kw, ansi.StringWidth(it.key)+2)
		}
	}
	// As wide as the completions, within the screen and a line's reach.
	inner := min(max(1+tw+2+ww+2+kw+1, 40), 72, m.width-2)
	tw = min(tw, inner/2)
	ww = max(min(ww, inner-1-tw-2-kw-2), 0)
	lines := make([]string, rows)
	for r := range rows {
		i := c.Top + r
		it := c.shown[i]
		base, dim := m.th.MenuBar, m.th.Muted
		switch {
		case i == c.Sel:
			base, dim = m.th.MenuSelected, m.th.MenuSelected
		case it.off:
			base, dim = m.th.Disabled, m.th.Disabled
		}
		row := base.Render(" "+theme.PadRight(ansi.Truncate(it.title, tw, "…"), tw)+"  ") +
			dim.Render(theme.PadRight(ansi.Truncate(it.word, ww, "…"), ww))
		k := ""
		switch {
		case it.key == "":
		case i == c.Sel || it.off:
			k = base.Render(" " + it.key + " ")
		default:
			k = m.th.Chip(it.key)
		}
		row += base.Render(strings.Repeat(" ", max(inner-ansi.StringWidth(row)-ansi.StringWidth(k)-1, 0))) + k + base.Render(" ")
		lines[r] = ansi.Truncate(row, inner, "")
	}
	footer := strconv.Itoa(c.Sel+1) + " of " + strconv.Itoa(len(c.shown))
	return []overlay.Box{{ID: cmdLineID, X: 0, Y: contextLine + 1, Lines: m.th.Frame(inner, "Commands", footer, lines)}}
}

func (c *cmdLine) Mouse(e overlay.MouseEvent) tea.Cmd {
	m := c.m
	if e.Box != cmdLineID {
		if e.Kind == overlay.MousePress {
			m.closeOverlay()
		}
		return nil
	}
	i := c.Top + e.Row - 1 // under the top border
	switch {
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelUp:
		c.Move(-1, len(c.shown))
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelDown:
		c.Move(1, len(c.shown))
	case e.Row < 1 || i >= len(c.shown) || i >= c.Top+c.rows(m):
	case e.Kind == overlay.MouseMotion:
		c.Sel = i
	case e.Kind == overlay.MousePress && e.Button == tea.MouseLeft:
		m.line.Set(c.shown[i].word)
		c.Sel = i
		return c.run(m)
	}
	return nil
}
