package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// No unit test waits on the clock (after). There's no terminal to say
// its background, so the model's wait for it is over at once; notebooks
// ask about the text being edited, and the word at the caret, once
// typing pauses, and the clock hands the message the pause would give
// to run, which keeps it in pauses: a test that wants a pause over ends
// it with endPauses.
func init() {
	after = func(_ time.Duration, msg tea.Msg) tea.Cmd {
		if _, ok := msg.(bgWaitedMsg); ok {
			return func() tea.Msg { return msg }
		}
		return func() tea.Msg { return pausedMsg{msg} }
	}
}

// pausedMsg is a message a notebook asked for once typing pauses.
type pausedMsg struct{ msg tea.Msg }

// pauses are the messages waiting for typing's pauses to end.
var pauses []tea.Msg

// endPauses ends the pauses waiting, and those they start: the
// notebook asks about the text and the word at the caret, and m gets
// the answers.
func endPauses(m *Model) {
	for i := 0; i < 10 && len(pauses) > 0; i++ {
		msgs := pauses
		pauses = nil
		for _, msg := range msgs {
			send(m, msg)
		}
	}
}
