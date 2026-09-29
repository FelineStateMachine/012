package ui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/ui/evalview"
)

// Data > Evaluate formula: the active cell's formula stepped through one
// part at a time in a box over the grid (package evalview), the values
// from the engine's sheet.Steps.

func init() {
	register(&command{id: "data.evaluate", macro: macroView, title: "Evaluate formula",
		desc: "Step through the active cell's formula one part at a time, each part's value in its place",
		enabled: func(m *Model) bool {
			c := m.sheet.Cell(m.cur)
			return c != nil && c.IsFormula()
		},
		run: func(m *Model) tea.Cmd {
			if st := m.sheet.EvaluateSteps(m.cur); st != nil {
				m.clearSelection()
				m.openOverlay(evalview.New(m.host(), st))
			}
			return nil
		}})
	keymap["alt+="] = "data.evaluate"
}
