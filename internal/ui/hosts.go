package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/ui/cmdline"
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
