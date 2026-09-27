// Package overlay is the contract between the UI's root model and the
// components that take over input while open: menus, pickers, bars on
// the context line and dialogs. It holds what they share (the boxes they
// draw, the mouse events they get, a scrolling list) and nothing of the
// model, so a component can live in a package of its own and be tested
// alone. Each component is built with the small host interface it acts
// on; the model implements those.
package overlay

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// The control panel's rows that overlays anchor to.
const (
	MenuLine    = 0 // the menu bar; dropdowns and pickers open under it
	ContextLine = 2 // prompts and bars; completions open under it
)

// SearchPrompt starts a search field, in pickers and bars.
const SearchPrompt = " › "

// KeyFor makes a key press from a keystroke like "alt+c", for chips that
// stand for a key when clicked.
func KeyFor(s string) tea.KeyPressMsg {
	k := tea.KeyPressMsg{}
	parts := strings.Split(s, "+")
	for _, p := range parts[:len(parts)-1] {
		if p == "alt" {
			k.Mod |= tea.ModAlt
		}
	}
	k.Code = rune(parts[len(parts)-1][0])
	return k
}

// Overlay is a floating box drawn over the grid: a dropdown or context
// menu, the command palette or a dialog. While one is open it takes every
// key and mouse event, and Esc closes one level. Overlays are composited
// on top of the finished screen, so the grid underneath never moves.
type Overlay interface {
	// Indicator is the mode shown at the top right, e.g. "MENU".
	Indicator() string
	// Layout places the overlay's boxes on the screen, bottom first.
	Layout() []Box
	Key(k tea.KeyPressMsg) tea.Cmd
	Mouse(e MouseEvent) tea.Cmd
	// Status is what the status line says while the overlay is open: what
	// the highlighted item does, and the keys that apply.
	Status() (desc, keys string)
}

// Text is an overlay with a text input, which gets the terminal cursor.
// The text is edited in the model's shared edit line, like any other
// entry.
type Text interface {
	Overlay
	// Cursor is where the caret is; a negative x hides it, while no
	// field of the overlay is being typed in.
	Cursor() (x, y int)
	Changed() // the text changed
}

// Liner is an overlay drawn on the context line, such as the find bar.
type Liner interface {
	ContextLine() (left, right string)
}

// Box is a rectangle of styled lines at a screen position. Every line has
// the same display width.
type Box struct {
	ID    string // identifies the box in mouse hit tests
	X, Y  int
	Lines []string
}

func (b Box) Width() int  { return ansi.StringWidth(b.Lines[0]) }
func (b Box) Height() int { return len(b.Lines) }

// MouseKind is what a mouse event is.
type MouseKind int

const (
	MousePress MouseKind = iota
	MouseMotion
	MouseRelease
	MouseWheel
)

// MouseEvent is a mouse event with what's under it already worked out.
type MouseEvent struct {
	Kind     MouseKind
	Button   tea.MouseButton
	X, Y     int
	Box      string // ID of the topmost box under the mouse, "" for none
	Col, Row int    // position inside that box
}

// List is the highlighted row and scroll position of a list in a box.
type List struct {
	Sel, Top int
}

// Move highlights the row d steps away, wrapping around the ends.
func (l *List) Move(d, n int) {
	if n > 0 {
		l.Sel = ((l.Sel+d)%n + n) % n
	}
}

// Show scrolls so the highlighted row is among the visible rows.
func (l *List) Show(rows int) {
	l.Top = max(l.Sel-rows+1, min(l.Top, l.Sel))
	l.Top = max(l.Top, 0)
}
