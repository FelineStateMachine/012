package ui

import (
	"time"

	tea "charm.land/bubbletea/v2"
)

// The theme follows the terminal's background: dark on a dark one,
// light on a light one (prefs.go). Until the terminal has said which,
// the screen stays blank rather than draw a frame in the dark theme a
// light terminal would flash, for as long as a question takes to cross
// an SSH connection. The wait ends with the background's answer, with
// the answer to the device attributes asked after it (probes), which a
// terminal that doesn't say its background sends instead, or after
// bgPatience for one that answers neither.

// bgPatience is how long the screen waits for a terminal that answers
// nothing.
const bgPatience = 500 * time.Millisecond

// bgWaitedMsg is bgPatience over.
type bgWaitedMsg struct{}

// after is the clock the model's waits are timed on: a command that
// gives msg once d has passed; tea.Tick's when nil. Unit tests give one
// that holds notebooks' pauses for them to end (pauses_test.go).
var after func(d time.Duration, msg tea.Msg) tea.Cmd

// awaitBackground holds the screen until the terminal says its
// background; probes asks. Its command gives bgWaitedMsg after
// bgPatience, or nothing once the wait is over.
func (t *terminal) awaitBackground() tea.Cmd {
	t.waitBG, t.bgKnown = true, make(chan struct{})
	if after != nil {
		return after(bgPatience, bgWaitedMsg{})
	}
	known := t.bgKnown
	return func() tea.Msg {
		timer := time.NewTimer(bgPatience)
		defer timer.Stop()
		select {
		case <-timer.C:
			return bgWaitedMsg{}
		case <-known:
			return nil
		}
	}
}

// backgroundKnown ends the wait for the background.
func (t *terminal) backgroundKnown() {
	if t.waitBG {
		t.waitBG = false
		close(t.bgKnown)
	}
}

// View implements tea.Model: the screen, once the theme is known.
func (m *Model) View() tea.View {
	if m.term.waitBG {
		v := tea.NewView(" ") // an empty view is drawn again every frame
		v.AltScreen = true
		return v
	}
	return m.frame()
}
