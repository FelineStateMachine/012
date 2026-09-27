package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/cmdline"
	"github.com/FelineStateMachine/012/internal/ui/findbar"
	"github.com/FelineStateMachine/012/internal/ui/lineedit"
	"github.com/FelineStateMachine/012/internal/ui/picker"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// host is how components in packages of their own reach the model. It
// implements each one's Host interface, the little it acts on, so the
// model's exported methods stay what cmd/012 and serve use, and a
// component sees only its own interface.
type host struct{ m *Model }

func (m *Model) host() host { return host{m} }

var (
	_ picker.Host  = host{}
	_ cmdline.Host = host{}
	_ findbar.Host = host{}
)

// What every component with a text field needs.

func (h host) Theme() *theme.Theme       { return &h.m.th }
func (h host) Size() (width, height int) { return h.m.width, h.m.height }
func (h host) Line() *lineedit.Line      { return &h.m.line }
func (h host) Close()                    { h.m.closeOverlay() }

// The picker.

func (h host) RecordAnswer(answer string, cancelled bool) { h.m.recordAnswer(answer, cancelled) }

// newPicker returns a picker of items over the model; the caller opens it.
func newPicker(m *Model, title, placeholder string, maxW int, items []picker.Item) *picker.Picker {
	return picker.New(m.host(), title, placeholder, maxW, items)
}

// The command line.

// Commands are the command line's completions: every registered command
// but the command line itself, by title.
func (h host) Commands() []cmdline.Item {
	ids := commandIDs()
	items := make([]cmdline.Item, 0, len(ids))
	for _, id := range ids {
		if id == "vim.command" {
			continue
		}
		c := commands[id]
		items = append(items, cmdline.Item{Word: id, Title: c.title, Desc: c.desc, Key: h.m.shortcut(id), Off: !c.available(h.m)})
	}
	return items
}

func (h host) Run(text string) (tea.Cmd, bool) { return h.m.runCmdLine(text) }
func (h host) Fail(msg string)                 { h.m.fail(msg) }

// The find bar.

func (h host) Book() *sheet.Workbook             { return h.m.book() }
func (h host) At() (*sheet.Sheet, sheet.Addr)    { return h.m.sheet, h.m.cur }
func (h host) Note(msg string)                   { h.m.note = msg }
func (h host) Edited()                           { h.m.changed = true }
func (h host) Show(s *sheet.Sheet, a sheet.Addr) { h.m.showSheet(s); h.m.cur = a }

// Leave closes the find bar, keeping it for Ctrl+F and find next.
func (h host) Leave() {
	if f, ok := h.m.overlay.(*findbar.Bar); ok {
		h.m.find = f
	}
	h.m.closeOverlay()
}

// Press hands a click outside an overlay to the grid, as if none were
// open.
func (h host) Press(x, y int, button tea.MouseButton) tea.Cmd {
	return h.m.handlePress(tea.Mouse{X: x, Y: y, Button: button})
}
