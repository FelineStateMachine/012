package ui

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/filterpick"
)

// Filters follow Sheets' Data > Create a filter: the headers of the
// filtered range get a button (▾, ▼ once the column hides something),
// and clicking it or pressing Alt+Down in the column opens a picker in the
// manner of fzf: a condition on top, then the column's values with
// checkboxes, narrowed by a fuzzy search as you type. Rows that don't
// pass are hidden, not deleted.

func init() {
	register(
		&command{id: "data.filter", title: "Create a filter", desc: "Filter the rows of the selection, or of the data around the active cell; the first row holds the headers",
			run:     (*Model).createFilter,
			enabled: func(m *Model) bool { _, on := m.sheet.FilterRange(); return !on }},
		&command{id: "data.filter_column", title: "Filter by column", desc: "Choose which values of the active column show, or a condition they must meet",
			run: func(m *Model) tea.Cmd {
				m.openFilterPicker(m.cur.Col)
				return nil
			},
			answer: (*Model).answerFilter,
			enabled: func(m *Model) bool {
				r, on := m.sheet.FilterRange()
				return on && m.cur.Col >= r.From.Col && m.cur.Col <= r.To.Col
			}},
		&command{id: "data.filter_remove", title: "Remove filter", desc: "Remove the filter and show every row again",
			run: func(m *Model) tea.Cmd {
				m.sheet.RemoveFilter()
				m.changed = true
				return nil
			},
			enabled: func(m *Model) bool { _, on := m.sheet.FilterRange(); return on }},
	)
}

// createFilter filters the selection, or the data around the active cell.
func (m *Model) createFilter() tea.Cmd {
	r := m.dataRange()
	m.sheet.CreateFilter(r)
	m.changed = true
	m.note = "Created a filter on " + r.String() + "   " + m.th.KeyHints(m.shortcut("data.filter_column"), "filter the active column")
	return nil
}

// dataRange is the range data commands act on: the selection when there
// is one, cut to the data in it, or else the block of data around the
// active cell.
func (g *grid) dataRange() sheet.Rect {
	if !g.hasRange() {
		return g.sheet.Region(g.cur)
	}
	r := g.selection()
	if used, ok := g.sheet.UsedRange(); ok {
		r.To.Row = max(min(r.To.Row, used.To.Row), r.From.Row)
		r.To.Col = max(min(r.To.Col, used.To.Col), r.From.Col)
	}
	return r
}

// filterMark is the filter button in column c's header, if c is in the
// filter's range, and whether the column's filter hides anything.
func (g *grid) filterMark(c int) (string, bool) {
	r, on := g.sheet.FilterRange()
	switch {
	case !on || c < r.From.Col || c > r.To.Col:
		return "", false
	case g.sheet.ColumnFiltered(c):
		return "▼", true
	}
	return "▾", false
}

// filterButtonX is the screen x of column c's filter button, or -1.
func (g *grid) filterButtonX(c int) int {
	name := sheet.ColName(c)
	w := g.sheet.ColWidth(c)
	if mark, _ := g.filterMark(c); mark == "" || w < len(name)+3 {
		return -1
	}
	lw := len(name) + 2
	return g.colStart(c) + (w-lw)/2 + lw - 1
}

// openFilterPicker opens the picker for column col of the filter.
func (m *Model) openFilterPicker(col int) *filterpick.Picker {
	r, on := m.sheet.FilterRange()
	if !on || col < r.From.Col || col > r.To.Col {
		return nil
	}
	title := "Filter " + sheet.ColName(col)
	if h := m.sheet.ShownText(sheet.Addr{Col: col, Row: r.From.Row}); h != "" {
		title += "  " + h
	}
	return m.openValuesPicker(title, m.colStart(col), m.sheet.FilterValues(col), m.sheet.Filter().Cols[col].Cond,
		func(cr sheet.Criteria) {
			m.filterColumn(col, cr)
			m.recordDialog("data.filter_column", filterAnswer(col, cr))
		})
}

// openValuesPicker opens a filter picker titled title at screen column x
// over values, with cond as the condition, calling apply with the
// criteria chosen.
func (m *Model) openValuesPicker(title string, x int, values []sheet.FilterValue, cond sheet.Condition, apply func(sheet.Criteria)) *filterpick.Picker {
	p := filterpick.New(m.host(), title, x, values, cond, apply)
	m.openOverlay(p)
	p.Start()
	return p
}

// filterAnswer is the answer to data.filter_column that sets column
// col's criteria to cr: {"column": "B", "hidden": [...]}, the criteria
// as a filter column's are written in the file.
func filterAnswer(col int, cr sheet.Criteria) string {
	rest := strings.TrimPrefix(cr.JSON(), "{")
	if rest != "}" {
		rest = "," + rest
	}
	return `{"column":"` + sheet.ColName(col) + `"` + rest
}

// answerFilter sets a column's criteria from a macro's answer, as the
// picker does: the column named, or the active one.
func (m *Model) answerFilter(text string) (tea.Cmd, error) {
	var a struct {
		Column string `json:"column"`
	}
	if err := json.Unmarshal([]byte(text), &a); err != nil {
		return nil, err
	}
	col := m.cur.Col
	if a.Column != "" {
		c, ok := sheet.ParseCol(strings.ToUpper(a.Column))
		if !ok {
			return nil, fmt.Errorf("no column %q", a.Column)
		}
		col = c
	}
	r, on := m.sheet.FilterRange()
	switch {
	case !on:
		return nil, errors.New("the sheet has no filter: run(\"data.filter\") creates one")
	case col < r.From.Col || col > r.To.Col:
		return nil, fmt.Errorf("column %s isn't in the filter's range %s", sheet.ColName(col), r)
	}
	return nil, m.openFilterPicker(col).Answer(text)
}

// filterColumn sets the criteria of column col of the sheet's filter.
func (m *Model) filterColumn(col int, cr sheet.Criteria) {
	span := m.spans.Start("filter")
	m.sheet.FilterColumn(col, cr)
	span.End(slog.Int("hidden", m.sheet.HiddenRows()))
	m.changed = true
	if n := m.sheet.HiddenRows(); n > 0 {
		m.note = "Filtered column " + sheet.ColName(col) + ": " + rowCount(n) + " hidden"
	} else {
		m.note = "The filter shows every row"
	}
}

func rowCount(n int) string {
	if n == 1 {
		return "1 row"
	}
	return strconv.Itoa(n) + " rows"
}
