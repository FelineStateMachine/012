package ui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// View > Freeze, as in Sheets: keep the first rows or columns on screen
// while the rest scrolls. The frozen panes are drawn in panes.go.

func init() {
	freeze := func(id, title, desc string, rows bool, n func(m *Model) int) *command {
		return &command{id: id, title: title, desc: desc,
			run: func(m *Model) tea.Cmd {
				m.freeze(rows, n(m))
				return nil
			},
			enabled: func(m *Model) bool { return n(m) <= sheet.MaxFrozen },
		}
	}
	fixed := func(k int) func(*Model) int { return func(*Model) int { return k } }
	register(
		freeze("view.freeze_rows0", "Unfreeze rows", "Let every row scroll", true, fixed(0)),
		freeze("view.freeze_rows1", "Freeze 1 row", "Keep the first row on screen while the rest scrolls", true, fixed(1)),
		freeze("view.freeze_rows2", "Freeze 2 rows", "Keep the first two rows on screen while the rest scrolls", true, fixed(2)),
		freeze("view.freeze_rows_cur", "Freeze up to current row", "Keep the rows down to the active cell on screen while the rest scrolls", true,
			func(m *Model) int { return m.cur.Row + 1 }),
		freeze("view.freeze_cols0", "Unfreeze columns", "Let every column scroll", false, fixed(0)),
		freeze("view.freeze_cols1", "Freeze 1 column", "Keep the first column on screen while the rest scrolls", false, fixed(1)),
		freeze("view.freeze_cols2", "Freeze 2 columns", "Keep the first two columns on screen while the rest scrolls", false, fixed(2)),
		freeze("view.freeze_cols_cur", "Freeze up to current column", "Keep the columns up to the active cell on screen while the rest scrolls", false,
			func(m *Model) int { return m.cur.Col + 1 }),
	)
}

// freeze sets the frozen rows (or columns) to n and says what happened,
// including when the window is too small to show them all.
func (m *Model) freeze(rows bool, n int) {
	if mg, cut := m.sheet.MergeAcross(rows, n); cut {
		m.fail("Can't freeze through the merged cells " + mg.String() + "; unmerge them first")
		return
	}
	fr, fc := m.sheet.Frozen()
	noun := "column"
	if rows {
		fr, noun = n, "row"
	} else {
		fc = n
	}
	m.sheet.SetFrozen(fr, fc)
	m.changed = m.sheet.StateID() != m.saved
	m.clampView()
	if n == 0 {
		m.note = "Unfroze " + noun + "s"
		return
	}
	m.note = "Froze " + strconv.Itoa(n) + " " + plural(n, noun, noun+"s")
	shownR, shownC := m.frozen()
	if shown := map[bool]int{true: shownR, false: shownC}[rows]; shown < n {
		m.note += " (" + strconv.Itoa(shown) + " fit in this window)"
	}
}
