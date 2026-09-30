// Package cmdhelp is a nushell command's help, F1 on a command in a
// notebook's cell: a scrollable box over the notebook with what the
// command does, a link to its page in nushell's docs, its usage, flags,
// parameters, input and output types and examples, as nu's help has
// them. It knows the UI only through Host.
package cmdhelp

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/nushell"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the help needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Close closes the open overlay.
	Close()
	// Code draws a line of nushell highlighted.
	Code(src string) string
}

// View is the open help.
type View struct {
	h    Host
	help nushell.Help
	top  int
}

// ID identifies the view's box in mouse events.
const ID = "cmdhelp"

// maxWidth is the widest the text runs, for reading.
const maxWidth = 92

// New returns the help of a command, scrolled to the top.
func New(h Host, help nushell.Help) *View { return &View{h: h, help: help} }

// Top is the first line shown.
func (s *View) Top() int { return s.top }

func (s *View) Indicator() string { return "HELP" }

// width is the text's width inside the box.
func (s *View) width() int {
	w, _ := s.h.Size()
	return max(min(w-6, maxWidth), 20)
}

// Lines are the help's lines at the box's width.
func (s *View) Lines() []string {
	th := s.h.Theme()
	w := s.width()
	hp := s.help
	var out []string
	wrap := func(text, indent string) {
		for line := range strings.SplitSeq(ansi.Wordwrap(text, w-len(indent), ""), "\n") {
			out = append(out, indent+line)
		}
	}
	heading := func(title string) { out = append(out, "", th.Title.Render(title)) }
	for _, para := range strings.Split(hp.Desc, "\n\n") {
		if len(out) > 0 {
			out = append(out, "")
		}
		wrap(strings.Join(strings.Fields(para), " "), "")
	}
	if hp.Desc != "" { // a command of the cell's own has no page
		url := nushell.DocsURL(hp.Name)
		out = append(out, "", th.Muted.Render("Docs: ")+th.Link.Hyperlink(url).Render(url))
	}
	heading("Usage")
	out = append(out, "  "+s.h.Code(hp.Usage))
	if len(hp.Flags) > 0 {
		heading("Flags")
		out = append(out, s.table(flagRows(hp.Flags))...)
	}
	if len(hp.Params) > 0 {
		heading("Parameters")
		out = append(out, s.table(paramRows(hp.Params))...)
	}
	if len(hp.Types) > 0 {
		heading("Input and output")
		for _, t := range hp.Types {
			out = append(out, "  "+t)
		}
	}
	if len(hp.Examples) > 0 {
		heading("Examples")
		for i, ex := range hp.Examples {
			if i > 0 {
				out = append(out, "")
			}
			wrap(ex.Desc, "  ")
			for line := range strings.SplitSeq(ex.Code, "\n") {
				out = append(out, "    "+s.h.Code(line))
			}
		}
	}
	return out
}

// flagRows are the flags as names and what they do: -r, --reverse.
func flagRows(flags []nushell.Flag) [][2]string {
	rows := make([][2]string, len(flags))
	for i, f := range flags {
		name := f.Long
		if f.Short != "" {
			name = strings.TrimSuffix(f.Short+", "+f.Long, ", ")
		}
		if f.Arg != "" {
			name += " <" + f.Arg + ">"
		}
		rows[i] = [2]string{name, f.Desc}
	}
	return rows
}

func paramRows(params []nushell.Param) [][2]string {
	rows := make([][2]string, len(params))
	for i, p := range params {
		rows[i] = [2]string{strings.TrimSuffix(p.Name+": "+p.Type, ": "), p.Desc}
	}
	return rows
}

// table draws names and what they are in two columns, the second
// wrapped beside the first.
func (s *View) table(rows [][2]string) []string {
	th := s.h.Theme()
	w := s.width()
	nameW := 0
	for _, r := range rows {
		nameW = max(nameW, ansi.StringWidth(r[0]))
	}
	nameW = min(nameW, w/2)
	var out []string
	for _, r := range rows {
		name := ansi.Truncate(r[0], nameW, "…")
		lead := "  " + th.Key.Render(name) + strings.Repeat(" ", nameW-ansi.StringWidth(name)+2)
		pad := strings.Repeat(" ", nameW+4)
		for i, line := range strings.Split(ansi.Wordwrap(r[1], max(w-nameW-4, 10), ""), "\n") {
			if i == 0 {
				out = append(out, lead+line)
			} else {
				out = append(out, pad+line)
			}
		}
	}
	return out
}

// visible is how many lines fit between the menu bar and the status
// line.
func (s *View) visible(total int) int {
	_, height := s.h.Size()
	return max(min(total, height-2-2-1), 1)
}

func (s *View) Layout() []overlay.Box {
	th := s.h.Theme()
	width, height := s.h.Size()
	lines := append([]string{""}, s.Lines()...) // breathing room under the title
	inner := s.width() + 2
	n := s.visible(len(lines))
	s.top = min(max(s.top, 0), max(len(lines)-n, 0))
	rows := make([]string, n)
	for i := range rows {
		line := ""
		if j := s.top + i; j < len(lines) {
			line = lines[j]
		}
		rows[i] = theme.Cells(th.MenuBar, " "+line, inner)
	}
	footer := ""
	if n < len(lines) {
		footer = strconv.Itoa(s.top+1) + "-" + strconv.Itoa(s.top+n) + " of " + strconv.Itoa(len(lines))
	}
	b := th.Frame(inner, s.help.Name, footer, rows)
	bw, bh := ansi.StringWidth(b[0]), len(b)
	return []overlay.Box{{ID: ID, X: (width - bw) / 2, Y: max(overlay.MenuLine+1, (height-1-bh)/2), Lines: b}}
}

func (s *View) Key(k tea.KeyPressMsg) tea.Cmd {
	total := len(s.Lines()) + 1
	page := s.visible(total)
	switch k.String() {
	case "up", "k":
		s.top--
	case "down", "j":
		s.top++
	case "pgup":
		s.top -= page
	case "pgdown", "space":
		s.top += page
	case "home":
		s.top = 0
	case "end":
		s.top = total
	case "esc", "enter", "q", "f1":
		s.h.Close()
	}
	s.top = min(max(s.top, 0), max(total-page, 0))
	return nil
}

func (s *View) Mouse(e overlay.MouseEvent) tea.Cmd {
	switch {
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelUp:
		s.top = max(s.top-3, 0)
	case e.Kind == overlay.MouseWheel && e.Button == tea.MouseWheelDown:
		s.top += 3 // layout clamps
	case e.Kind == overlay.MousePress && e.Box != ID:
		s.h.Close()
	}
	return nil
}

func (s *View) Status() (string, string) {
	return s.help.Summary(), s.h.Theme().KeyHints("Up/Down", "scroll", "Esc", "close")
}
