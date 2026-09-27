// Package shortcuts is the keyboard shortcuts view: a scrollable box
// over the grid listing every key by what it does, in two columns when
// the screen is wide enough. The rows come from the host, which builds
// them from the key bindings and the command registry. It knows the UI
// only through Host.
package shortcuts

import (
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// Host is what the shortcuts view needs of the UI.
type Host interface {
	Theme() *theme.Theme
	Size() (width, height int)
	// Close closes the open overlay.
	Close()
	// Shortcuts are the rows to list, in order.
	Shortcuts() []Row
}

// Row is a line of the view: a group heading, or keys and what they do.
type Row struct {
	Heading string
	Keys    []string // as shown, e.g. "Ctrl+G"
	Action  string
}

// View is the open shortcuts view.
type View struct {
	h   Host
	top int
}

// ID identifies the view's box in mouse events.
const ID = "shortcuts"

// New returns the view scrolled to the top.
func New(h Host) *View { return &View{h: h} }

// Top is the first line shown.
func (s *View) Top() int { return s.top }

func (s *View) Indicator() string { return "HELP" }

// Lines renders the rows, as one or two columns, and returns their
// width.
func (s *View) Lines() ([]string, int) {
	th := s.h.Theme()
	width, _ := s.h.Size()
	rows := s.h.Shortcuts()
	keyW, actW := 0, 0
	for _, r := range rows {
		keyW = max(keyW, ansi.StringWidth(th.Chips(r.Keys)))
		actW = max(actW, ansi.StringWidth(r.Action))
	}
	// On narrow screens actions give way (truncated) so the keys fit.
	room := min(width-4, 160)
	if 1+keyW+2+actW > room {
		actW = max(room-3-keyW, 16)
	}
	colW := 1 + keyW + 2 + actW
	render := func(rows []Row) []string {
		var out []string
		for i, r := range rows {
			switch {
			case r.Heading != "" && i > 0:
				out = append(out, "", th.Title.Render(" "+r.Heading))
			case r.Heading != "":
				out = append(out, th.Title.Render(" "+r.Heading))
			default:
				out = append(out, " "+theme.PadRight(ansi.Truncate(r.Action, actW, "…"), actW)+"  "+th.Chips(r.Keys))
			}
		}
		return out
	}
	if 2*colW+3 > room {
		return render(rows), min(colW, room)
	}
	// Two columns, split at the group boundary nearest the middle.
	split, best := 0, len(rows)
	for i, r := range rows {
		if d := abs(len(rows) - 2*i); r.Heading != "" && d < best {
			split, best = i, d
		}
	}
	left, right := render(rows[:split]), render(rows[split:])
	out := make([]string, max(len(left), len(right)))
	for i := range out {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out[i] = theme.PadRight(l, colW) + "   " + r
	}
	return out, 2*colW + 3
}

func abs(x int) int { return max(x, -x) }

// visible is how many lines fit between the menu bar and the status line.
func (s *View) visible(total int) int {
	_, height := s.h.Size()
	return max(min(total, height-2-2-1), 1)
}

func (s *View) Layout() []overlay.Box {
	th := s.h.Theme()
	width, height := s.h.Size()
	lines, w := s.Lines()
	inner := w + 1
	n := s.visible(len(lines) + 1)
	s.top = min(max(s.top, 0), max(len(lines)+1-n, 0))
	lines = append([]string{""}, lines...) // breathing room under the title
	rows := make([]string, n)
	for i := range rows {
		if j := s.top + i; j < len(lines) {
			rows[i] = theme.Cells(th.MenuBar, lines[j], inner)
		} else {
			rows[i] = strings.Repeat(" ", inner)
		}
	}
	footer := ""
	if n < len(lines) {
		footer = strconv.Itoa(s.top+1) + "-" + strconv.Itoa(s.top+n) + " of " + strconv.Itoa(len(lines))
	}
	b := th.Frame(inner, "Keyboard shortcuts", footer, rows)
	bw, bh := ansi.StringWidth(b[0]), len(b)
	return []overlay.Box{{ID: ID, X: (width - bw) / 2, Y: max(overlay.MenuLine+1, (height-1-bh)/2), Lines: b}}
}

func (s *View) Key(k tea.KeyPressMsg) tea.Cmd {
	lines, _ := s.Lines()
	page := s.visible(len(lines) + 1)
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
		s.top = len(lines)
	case "esc", "enter", "q", "f1", "ctrl+/":
		s.h.Close()
	}
	s.top = min(max(s.top, 0), max(len(lines)+1-page, 0))
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
	return "", s.h.Theme().KeyHints("Up/Down", "scroll", "Esc", "close")
}
