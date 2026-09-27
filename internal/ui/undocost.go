package ui

import (
	"fmt"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A change whose undo step would hold more than sheet.MaxStepBytes
// (millions of formulas, or of distinct long texts, cleared or rewritten
// at once) asks first on the context line rather than holding gigabytes
// for undo: Enter runs it without undo, forgetting the history, and Esc
// backs out. Commands are checked through what they edit, as protected
// ranges are, and so is sorting. Macros that run don't ask.

// maxStepBytes is sheet.MaxStepBytes; tests lower it.
var maxStepBytes int64 = sheet.MaxStepBytes

// askUndoCost reports whether changing r would take more undo history
// than maxStepBytes, and if so asks whether to go on: Enter runs retry
// without undo, and retry isn't asked again.
func (m *Model) askUndoCost(r sheet.Rect, retry func(*Model) tea.Cmd) bool {
	if m.undoOK || m.macros.run != nil {
		return false
	}
	cost := m.sheet.UndoCost(r)
	if cost <= maxStepBytes {
		return false
	}
	protectOK := m.protectOK // agreed to before this asked
	m.ask(question{
		msg: "This can't be undone: it would take " + byteSize(cost) + " of undo history.", warn: true,
		desc: "Going on runs it without undo and forgets the changes before it",
		choices: []choice{
			{key: "enter", label: "Go on without undo", run: func(m *Model) tea.Cmd {
				m.undoOK, m.protectOK = true, protectOK
				defer func() { m.undoOK, m.protectOK = false, false }()
				var cmd tea.Cmd
				m.book().WithoutUndo(func() { cmd = retry(m) })
				m.syncChanged()
				return cmd
			}},
			{key: "esc", label: "Cancel", run: func(*Model) tea.Cmd { return nil }},
		},
	})
	return true
}

// byteSize is n bytes in MB, or in GB from a thousand MB.
func byteSize(n int64) string {
	if mb := n >> 20; mb < 1000 {
		return fmt.Sprintf("%d MB", mb)
	}
	return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
}
