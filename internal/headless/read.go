package headless

import (
	"encoding/json"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/nuon"
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
	// Values are the cells row by row, typed as 012 get --format json
	// types one cell: numbers, strings, true or false, null for blank,
	// dates as ISO 8601 strings, errors as their text (#DIV/0!).
	Values [][]json.RawMessage `json:"values"`
	// Text is each cell as the sheet shows it, when asked for.
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
	Text     bool // also return each cell as shown
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
	null := json.RawMessage("null")
	for row := r.From.Row; row < r.From.Row+rows; row++ {
		vals := make([]json.RawMessage, cols)
		var text []string
		if o.Text {
			text = make([]string, cols)
		}
		for i := range cols {
			a := sheet.Addr{Col: r.From.Col + i, Row: row}
			vals[i] = null
			if c := s.Cell(a); c != nil && c.IsFormula() {
				out.Formulas[a.String()] = c.Input
			}
			v := s.Value(a)
			if v.Kind == sheet.Empty {
				continue
			}
			vals[i] = nuon.AppendJSON(nil, fileio.CellValue(fileio.SnapCell{Value: v, Format: s.DisplayFormat(a)}))
			if o.Text {
				text[i] = s.LocalText(a)
			}
		}
		out.Values = append(out.Values, vals)
		if o.Text {
			out.Text = append(out.Text, text)
		}
	}
	return out
}
