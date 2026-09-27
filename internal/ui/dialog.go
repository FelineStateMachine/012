package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/ui/choicebar"
)

// question is a choice bar (package choicebar) as the model asks it:
// a small question on the context line answered with a key, e.g.
// "You have unsaved changes.  Enter Save and quit  D Discard  Esc Cancel".
type question struct {
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

// ask opens a choice bar with q's question.
func (m *Model) ask(q question) {
	chs := make([]choicebar.Choice, len(q.choices))
	for i, ch := range q.choices {
		chs[i] = choicebar.Choice{Key: ch.key, Label: ch.label, Run: func() tea.Cmd { return ch.run(m) }}
	}
	m.openOverlay(choicebar.New(m.host(), q.msg, q.warn, q.desc, chs))
}

// quit exits, asking first when there are unsaved changes. Enter saves
// and quits, so the quick path never loses work.
func (m *Model) quit() tea.Cmd {
	if !m.changed {
		return m.exit()
	}
	m.ask(question{
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
