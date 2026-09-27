package ui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/FelineStateMachine/012/internal/ui/overlay"
	"github.com/FelineStateMachine/012/internal/ui/theme"
)

// choiceBar is a small question asked on the context line and answered
// with a key, the way terminal programs confirm things, e.g.
// "You have unsaved changes.  Enter Save and quit  D Discard  Esc Cancel".
// It takes the keyboard like an overlay but draws no box.
type choiceBar struct {
	m       *Model // the model it acts on
	msg     string
	warn    bool   // the message is a warning, e.g. about losing work
	desc    string // more on the status line, when the question needs it
	choices []choice
}

type choice struct {
	key   string // as tea reports it, e.g. "enter" or "d"
	label string
	run   func(m *Model) tea.Cmd
}

func (c *choiceBar) Indicator() string        { return "MENU" }
func (c *choiceBar) Layout() []overlay.Box    { return nil }
func (c *choiceBar) Status() (string, string) { return c.desc, "" }

func (c *choiceBar) Key(k tea.KeyPressMsg) tea.Cmd {
	m := c.m
	key := strings.ToLower(k.String())
	for _, ch := range c.choices {
		if key == ch.key {
			return c.choose(m, ch)
		}
	}
	return nil
}

func (c *choiceBar) choose(m *Model, ch choice) tea.Cmd {
	m.closeOverlay()
	cmd := ch.run(m)
	m.recordAnswer(ch.key, ch.key == "esc")
	return cmd
}

// Mouse runs a choice when its key chip or label is clicked. A click
// anywhere else cancels, like Esc.
func (c *choiceBar) Mouse(e overlay.MouseEvent) tea.Cmd {
	m := c.m
	if e.Kind != overlay.MousePress {
		return nil
	}
	if e.Y == contextLine {
		x := ansi.StringWidth(c.prefix(m))
		for _, ch := range c.choices {
			w := ansi.StringWidth(c.item(m, ch))
			if e.X >= x && e.X < x+w {
				return c.choose(m, ch)
			}
			x += w + len(choiceGap)
		}
	}
	m.closeOverlay()
	return nil
}

const choiceGap = "   "

func (c *choiceBar) prefix(m *Model) string {
	if c.warn {
		return m.th.Warning.Render(c.msg) + choiceGap
	}
	return c.msg + choiceGap
}

func (c *choiceBar) item(m *Model, ch choice) string {
	return m.th.Chip(theme.KeyLabel(ch.key)) + " " + ch.label
}

// ContextLine renders the bar for the context line.
func (c *choiceBar) ContextLine() (string, string) {
	m := c.m
	items := make([]string, len(c.choices))
	for i, ch := range c.choices {
		items[i] = c.item(m, ch)
	}
	return c.prefix(m) + strings.Join(items, choiceGap), ""
}

// quit exits, asking first when there are unsaved changes. Enter saves
// and quits, so the quick path never loses work.
func (m *Model) quit() tea.Cmd {
	if !m.changed {
		return m.exit()
	}
	m.openOverlay(&choiceBar{
		m:    m,
		msg:  "You have unsaved changes.",
		warn: true,
		choices: []choice{
			{key: "enter", label: "Save and quit", run: func(m *Model) tea.Cmd {
				m.quitAfterSave = true
				return m.save()
			}},
			{key: "d", label: "Discard", run: (*Model).exit},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		},
	})
	return nil
}
