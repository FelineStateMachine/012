// Package cmdline is the command line (: with vim keys, or from the
// palette): what's typed on the context line, with completions from the
// command registry in a box under it. It knows the UI only through Host;
// what a line means (vim's file commands, a cell to go to, a command by
// name) is the host's to carry out.
package cmdline

import (
	"slices"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the command line needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Line is the shared edit line the command is typed in.
	Line() *lineedit.Line
	// Close closes the open overlay.
	Close()
	// Commands are the registered commands to complete, by title.
	Commands() []Item
	// Run carries out a command line and reports whether it was one.
	Run(text string) (tea.Cmd, bool)
	// Fail shows an error.
	Fail(msg string)
}

// Item is a completion: a file command or a registered command.
type Item struct {
	Word  string // what Tab puts on the line: "w", or a command's ID
	Title string
	Desc  string
	Key   string // the command's shortcut
	Off   bool   // unavailable right now
}

// FileWords are vim's file commands.
var FileWords = []Item{
	{Word: "w", Title: "Write", Desc: "Save the sheet; :w name saves it as name, :w name.csv downloads it"},
	{Word: "q", Title: "Quit", Desc: "Close 012, asking about unsaved changes"},
	{Word: "q!", Title: "Quit without saving", Desc: "Close 012, discarding unsaved changes"},
	{Word: "wq", Title: "Write and quit", Desc: "Save the sheet, then close 012"},
	{Word: "x", Title: "Write if changed and quit", Desc: "Save the sheet if it changed, then close 012"},
	{Word: "e", Title: "Edit a file", Desc: "Open a sheet or import a file: :e name"},
}

// ID identifies the completions' box in mouse events.
const ID = "cmdline"

// Line is the open command line.
type Line struct {
	h Host
	overlay.List
	shown  []Item
	tabbed bool // the text is a completion Tab put there; Tab again moves on
}

// New returns a command line with the edit line cleared.
func New(h Host) *Line {
	c := &Line{h: h}
	h.Line().Clear()
	c.Changed()
	return c
}

// Shown are the completions of what's typed.
func (c *Line) Shown() []Item { return c.shown }

func (c *Line) Indicator() string { return "COMMAND" }

// Changed completes what's typed: file commands, then commands whose ID
// or title starts with it, then those that contain it. Nothing is
// completed once a file command's argument follows.
func (c *Line) Changed() {
	c.Sel, c.Top, c.tabbed = 0, 0, false
	c.shown = c.shown[:0]
	q := strings.ToLower(strings.TrimPrefix(strings.TrimLeft(c.h.Line().Text(), " "), ":"))
	if word, _, arg := strings.Cut(q, " "); arg && isFileWord(word) {
		return
	}
	for _, w := range FileWords {
		if strings.HasPrefix(w.Word, q) {
			c.shown = append(c.shown, w)
		}
	}
	var starts, contains []Item
	for _, it := range c.h.Commands() {
		title := strings.ToLower(it.Title)
		switch {
		case strings.HasPrefix(it.Word, q) || strings.HasPrefix(title, q):
			starts = append(starts, it)
		case strings.Contains(it.Word, q) || strings.Contains(title, q):
			contains = append(contains, it)
		}
	}
	c.shown = append(append(c.shown, starts...), contains...)
}

// isFileWord reports whether word is one of vim's file commands, which
// take an argument.
func isFileWord(word string) bool {
	return slices.ContainsFunc(FileWords, func(it Item) bool { return it.Word == strings.TrimSuffix(word, "!") })
}

func (c *Line) Key(k tea.KeyPressMsg) tea.Cmd {
	line := c.h.Line()
	switch key := k.String(); {
	case key == "esc", key == "backspace" && len(line.Buf) == 0:
		c.h.Close()
	case key == "enter":
		return c.run()
	case key == "tab":
		c.complete(1)
	case key == "shift+tab":
		c.complete(-1)
	case key == "up", key == "ctrl+p":
		c.Move(-1, len(c.shown))
	case key == "down", key == "ctrl+n":
		c.Move(1, len(c.shown))
	default:
		before := line.Text()
		line.Key(k)
		if line.Text() != before {
			c.Changed()
		}
	}
	return nil
}

// complete puts the highlighted completion on the line; pressed again,
// it moves on to the next one (d = 1) or the previous one (d = -1).
func (c *Line) complete(d int) {
	if len(c.shown) == 0 {
		return
	}
	if c.tabbed {
		c.Move(d, len(c.shown))
	}
	c.h.Line().Set(c.shown[c.Sel].Word)
	c.tabbed = true
}

// run closes the line and carries it out. Text that isn't a command,
// cell or range runs the highlighted completion, so ":fill d" Enter
// fills down.
func (c *Line) run() tea.Cmd {
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(c.h.Line().Text()), ":"))
	fallback := ""
	if c.Sel < len(c.shown) {
		fallback = c.shown[c.Sel].Word
	}
	c.h.Close()
	if text == "" {
		return nil
	}
	if cmd, ok := c.h.Run(text); ok {
		return cmd
	}
	if fallback != "" {
		if cmd, ok := c.h.Run(fallback); ok {
			return cmd
		}
	}
	c.h.Fail("Not a command, cell or range: " + text)
	return nil
}

func (c *Line) ContextLine() (string, string) {
	return c.h.Theme().Title.Render(":") + c.h.Line().Text(), ""
}

func (c *Line) Cursor() (int, int) {
	return 1 + ansi.StringWidth(c.h.Line().Head()), overlay.ContextLine
}

func (c *Line) Status() (string, string) {
	keys := c.h.Theme().KeyHints("Tab", "complete", "Enter", "run", "Esc", "cancel")
	if c.Sel >= len(c.shown) {
		return "", keys
	}
	it := c.shown[c.Sel]
	if it.Off {
		return it.Desc + " (not available now)", keys
	}
	return it.Desc, keys
}

// rows is how many completions show.
func (c *Line) rows() int {
	_, height := c.h.Size()
	return max(min(len(c.shown), 8, height-overlay.ContextLine-4), 0)
}

// Layout draws the completions in a box under the context line: title,
// what Tab types, and the shortcut.
func (c *Line) Layout() []overlay.Box {
	rows := c.rows()
	if rows == 0 {
		return nil
	}
	c.Show(rows)
	th := c.h.Theme()
	width, _ := c.h.Size()
	tw, ww, kw := 0, 0, 0
	for _, it := range c.shown {
		tw = max(tw, ansi.StringWidth(it.Title))
		ww = max(ww, ansi.StringWidth(it.Word))
		if it.Key != "" {
			kw = max(kw, ansi.StringWidth(it.Key)+2)
		}
	}
	// As wide as the completions, within the screen and a line's reach.
	inner := min(max(1+tw+2+ww+2+kw+1, 40), 72, width-2)
	tw = min(tw, inner/2)
	ww = max(min(ww, inner-1-tw-2-kw-2), 0)
	lines := make([]string, rows)
	for r := range rows {
		i := c.Top + r
		it := c.shown[i]
		base, dim := th.MenuBar, th.Muted
		switch {
		case i == c.Sel:
			base, dim = th.MenuSelected, th.MenuSelected
		case it.Off:
			base, dim = th.Disabled, th.Disabled
		}
		row := base.Render(" "+theme.PadRight(ansi.Truncate(it.Title, tw, "…"), tw)+"  ") +
			dim.Render(theme.PadRight(ansi.Truncate(it.Word, ww, "…"), ww))
		k := ""
		switch {
		case it.Key == "":
		case i == c.Sel || it.Off:
			k = base.Render(" " + it.Key + " ")
		default:
			k = th.Chip(it.Key)
		}
		row += base.Render(strings.Repeat(" ", max(inner-ansi.StringWidth(row)-ansi.StringWidth(k)-1, 0))) + k + base.Render(" ")
		lines[r] = ansi.Truncate(row, inner, "")
	}
	footer := strconv.Itoa(c.Sel+1) + " of " + strconv.Itoa(len(c.shown))
	return []overlay.Box{{ID: ID, X: 0, Y: overlay.ContextLine + 1, Lines: th.Frame(inner, "Commands", footer, lines)}}
}

func (c *Line) Mouse(e overlay.MouseEvent) tea.Cmd {
	if e.Box != ID {
		if e.Kind == overlay.MousePress {
			c.h.Close()
		}
		return nil
	}
	i := c.Top + e.Row - 1 // under the top border
	switch {
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelUp:
		c.Move(-1, len(c.shown))
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelDown:
		c.Move(1, len(c.shown))
	case e.Row < 1 || i >= len(c.shown) || i >= c.Top+c.rows():
	case e.Kind == overlay.MouseMotion:
		c.Sel = i
	case e.Kind == overlay.MousePress && e.Button == tea.MouseLeft:
		c.h.Line().Set(c.shown[i].Word)
		c.Sel = i
		return c.run()
	}
	return nil
}
