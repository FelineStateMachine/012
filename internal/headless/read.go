package headless

import (
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Range is a range's values as a grid, for agents: what ReadRange
// returns and the MCP read_range tool gives back. Its JSON form is a
// stable schema (docs/reference/json.md).
type Range struct {
	// Range is what was read, with its sheet: Q3!A1:D9; a whole sheet is
	// read from A1 to its last cell with contents.
	Range string `json:"range"`
	Rows  int    `json:"rows"`
	Cols  int    `json:"cols"`
	// Values are the cells row by row, each a Value typed by its format:
	// numbers, strings, true or false, null for blank, {"currency": 3.5},
	// {"date": "2026-09-29"} and the rest, errors as their text (#DIV/0!).
	Values [][]Value `json:"values"`
	// Text is each cell as the sheet shows it, unless left out.
	Text [][]string `json:"text,omitempty"`
	// Formulas are the formulas in the range by cell (B7: =SUM(B1:B6)).
	Formulas map[string]string `json:"formulas"`
	// Truncated is set when the range held more than the most cells a
	// read returns; Rows says how many rows came back.
	Truncated bool `json:"truncated"`
}

// ReadOptions tune ReadRange.
type ReadOptions struct {
	MaxCells int  // the most cells to return, whole rows at a time; 0 is 10,000
	NoText   bool // leave out each cell as shown
}

// ReadRange reads t's cells as a grid.
func ReadRange(t Target, o ReadOptions) Range {
	if o.MaxCells <= 0 {
		o.MaxCells = 10_000
	}
	s, r := t.Sheet, t.Range
	if t.Whole {
		r, _ = s.UsedRange()
	} else if data, ok := s.FilledBounds(r); ok {
		r.To.Row, r.To.Col = min(r.To.Row, data.To.Row), min(r.To.Col, data.To.Col)
	} else {
		r.To = r.From
	}
	cols := r.To.Col - r.From.Col + 1
	rows := r.To.Row - r.From.Row + 1
	out := Range{Range: sheet.Qualified(s.Name(), r), Cols: cols, Formulas: map[string]string{}}
	if rows*cols > o.MaxCells {
		rows, out.Truncated = max(o.MaxCells/cols, 1), true
	}
	out.Rows = rows
	for row := r.From.Row; row < r.From.Row+rows; row++ {
		vals := make([]Value, cols)
		var text []string
		if !o.NoText {
			text = make([]string, cols)
		}
		for i := range cols {
			a := sheet.Addr{Col: r.From.Col + i, Row: row}
			vals[i] = Value("null")
			if c := s.Cell(a); c != nil && c.IsFormula() {
				out.Formulas[a.String()] = c.Input
			}
			if s.Value(a).Kind == sheet.Empty {
				continue
			}
			vals[i] = CellValue(s, a)
			if text != nil {
				text[i] = s.LocalText(a)
			}
		}
		out.Values = append(out.Values, vals)
		if text != nil {
			out.Text = append(out.Text, text)
		}
	}
	return out
}
