package ui

import (
	"cmp"
	"errors"
	"log/slog"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Pivot tables follow Sheets: Data > Pivot table summarizes the
// selection, or the table around the active cell (its first row naming
// the fields), on a new sheet named Pivot Table 1, and opens the pivot
// editor (pivoteditor.go) to pick rows, columns, values and filters. The
// engine keeps the results live; editing them is refused with a note on
// the context line, as Sheets refuses. Data > Frequency table counts the
// values of the active column the same way, as VisiData's Shift+F: a
// frequency table is a pivot like any other.

func init() {
	register(
		&command{id: "data.pivot", title: "Pivot table", desc: "Summarize the selection, or the table around the active cell, on a new sheet",
			run: (*Model).createPivot,
			answer: func(m *Model, text string) (tea.Cmd, error) {
				m.createPivot()
				e, ok := m.overlay.(*pivotEditor)
				if !ok {
					return nil, errors.New(cmp.Or(m.errMsg, m.note))
				}
				return nil, e.answer(m, text)
			}},
		&command{id: "data.pivot_edit", title: "Edit pivot table", desc: "Change the pivot table's rows, columns, values and filters",
			enabled: func(m *Model) bool { _, ok := m.sheet.Pivot(); return ok },
			run: func(m *Model) tea.Cmd {
				m.openPivotEditor(m.sheet.StateID(), nil)
				return nil
			},
			answer: func(m *Model, text string) (tea.Cmd, error) {
				if _, ok := m.sheet.Pivot(); !ok {
					return nil, errors.New(m.sheet.Name() + " has no pivot table")
				}
				return nil, m.openPivotEditor(m.sheet.StateID(), nil).answer(m, text)
			}},
		&command{id: "data.frequency", title: "Frequency table (column stats)", desc: "Count each value of the active column, most frequent first, on a new sheet, as Sheets' Column stats do",
			run: (*Model).frequency},
	)
	// VisiData's Shift+F; plain Shift+F types an F, as in Sheets.
	keymap["alt+shift+f"] = "data.frequency"
	keyAliases["alt+F"] = "alt+shift+f"
}

// refuseEdit reports whether r overlaps a pivot table's results or
// cells an array formula spills (unless r holds the formula too, or
// keepsSpills says the edit moves or formats cells rather than writing
// them), which can't be edited, saying so on the context line.
func (m *Model) refuseEdit(r sheet.Rect, keepsSpills bool) bool {
	if m.out != nil {
		m.note = "An output can't be changed: G sends it to a sheet, where its copy can"
		return true
	}
	if m.sheet.InPivot(r) {
		m.note = sheet.ErrPivotEdit.Error()
		return true
	}
	if keepsSpills {
		return false
	}
	if a, reg, ok := m.sheet.InRegion(r); ok {
		if reg.Linked() {
			m.note = sheet.ErrLinkedEdit.Error()
			return true
		}
		m.note = a.String() + " shows " + reg.Name + ", a notebook cell's output: change the cell, or freeze the region"
		return true
	}
	a, ok := m.sheet.InSpill(r)
	if !ok {
		return false
	}
	anchor, _ := m.sheet.SpillAnchor(a)
	m.note = a.String() + " shows part of the array " + anchor.String() + " spills: edit the formula in " + anchor.String()
	return true
}

// Targets of editing commands, for command.edits.

func cellTarget(m *Model) sheet.Rect { return sheet.Rect{From: m.cur, To: m.cur} }

// rowsTarget is the whole rows of the selection and everything below,
// which inserting or deleting rows moves.
func rowsTarget(m *Model) sheet.Rect {
	r := m.selection()
	return sheet.Rect{From: sheet.Addr{Row: r.From.Row}, To: sheet.Addr{Col: sheet.MaxCols - 1, Row: sheet.MaxRows - 1}}
}

func rowsBelowTarget(m *Model) sheet.Rect {
	r := rowsTarget(m)
	r.From.Row = min(m.selection().To.Row+1, sheet.MaxRows-1)
	return r
}

// colsTarget is the whole columns of the selection and everything right
// of them.
func colsTarget(m *Model) sheet.Rect {
	r := m.selection()
	return sheet.Rect{From: sheet.Addr{Col: r.From.Col}, To: sheet.Addr{Col: sheet.MaxCols - 1, Row: sheet.MaxRows - 1}}
}

func colsRightTarget(m *Model) sheet.Rect {
	r := colsTarget(m)
	r.From.Col = min(m.selection().To.Col+1, sheet.MaxCols-1)
	return r
}

// noCells is command.changes for commands that move cells without
// changing any, such as inserting rows, or that only put rules on them
// (a checkbox, a dropdown).
func noCells(*Model) (sheet.Rect, bool) { return sheet.Rect{}, false }

// selectedRowsTarget and selectedColsTarget are the whole rows or
// columns of the selection, which deleting them changes.
func selectedRowsTarget(m *Model) (sheet.Rect, bool) {
	r := m.selection()
	return sheet.Rect{From: sheet.Addr{Row: r.From.Row}, To: sheet.Addr{Col: sheet.MaxCols - 1, Row: r.To.Row}}, true
}

func selectedColsTarget(m *Model) (sheet.Rect, bool) {
	r := m.selection()
	return sheet.Rect{From: sheet.Addr{Col: r.From.Col}, To: sheet.Addr{Col: r.To.Col, Row: sheet.MaxRows - 1}}, true
}

// pasteTarget is where Ctrl+V writes: the selection, and at least the
// clip's size from the active cell.
func (m *Model) pasteTarget() sheet.Rect {
	r := m.selection()
	if c := m.copied.clip; c != nil {
		cols, rows := c.Size()
		r.To.Col = min(max(r.To.Col, r.From.Col+cols-1), sheet.MaxCols-1)
		r.To.Row = min(max(r.To.Row, r.From.Row+rows-1), sheet.MaxRows-1)
	}
	return r
}

// pivotData is the range a new pivot or frequency table reads: the data
// commands' range, cut to the used cells, and whether it has a header row
// and at least one row below it.
func (m *Model) pivotData() (sheet.Rect, bool) {
	r := m.dataRange()
	used, ok := m.sheet.UsedRange()
	if !ok {
		return r, false
	}
	r.To.Row, r.To.Col = min(r.To.Row, used.To.Row), min(r.To.Col, used.To.Col)
	return r, r.To.Row > r.From.Row && r.To.Col >= r.From.Col
}

// createPivot starts a pivot table on a new sheet and opens its editor.
func (m *Model) createPivot() tea.Cmd {
	r, ok := m.pivotData()
	if !ok {
		m.note = "Select the data to summarize first: a row of headers and the rows below it"
		return nil
	}
	src, start := m.sheet, m.sheet.StateID()
	s, err := m.book().CreatePivot(src, r, "", sheet.NewPivot(src, r))
	if err != nil {
		m.fail(err.Error())
		return nil
	}
	m.showSheet(s)
	m.openPivotEditor(start, src)
	return nil
}

// frequency makes a frequency table of the active column on a new sheet.
func (m *Model) frequency() tea.Cmd {
	r, ok := m.pivotData()
	col := m.cur.Col
	if !ok || col < r.From.Col || col > r.To.Col {
		m.note = "Move to a column of data first: a header and the values below it"
		return nil
	}
	span := m.spans.Start("frequency", slog.Int("rows", r.To.Row-r.From.Row))
	defer span.End()
	p := sheet.FrequencyPivot(m.sheet, r, col)
	field := m.book().FieldName(p, col)
	s, err := m.book().CreatePivot(m.sheet, r, "Frequency of "+field, p)
	if err != nil {
		m.fail(err.Error())
		return nil
	}
	m.showSheet(s)
	out, _ := s.PivotRange()
	distinct := max(out.To.Row-1, 0) // less the header and Grand Total rows
	m.note = "Counted " + field + ": " + strconv.Itoa(distinct) + plural(distinct, " distinct value", " distinct values") +
		"   " + m.th.KeyHints("Ctrl+Z", "undo")
	return nil
}
