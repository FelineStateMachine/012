package headless

import (
	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Description is what a workbook holds, for 012 describe and the MCP
// describe tool: enough for agents (or people) to know which
// references to read and write without reading every cell. Its JSON
// form is a stable schema (docs/reference/json.md): fields are only
// added, never renamed or removed.
type Description struct {
	Sheets []SheetDescription `json:"sheets"`
	Names  []NameDescription  `json:"names"`
}

// SheetDescription is one tab: a sheet or a notebook.
type SheetDescription struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"` // "sheet", "notebook" or "pivot"
	Shown  bool   `json:"shown"`
	Hidden bool   `json:"hidden"`
	// Used is the range from A1 to the last cell with contents, "" when
	// the sheet is empty; Rows and Cols are its size.
	Used     string `json:"used"`
	Rows     int    `json:"rows"`
	Cols     int    `json:"cols"`
	Cells    int    `json:"cells"`
	Formulas int    `json:"formulas"` // -1 on sheets too large to count them
	Errors   int    `json:"errors"`   // formulas showing errors; -1 when not counted
	// Header is the row guessed to name the used range's columns, and
	// Columns its names; 0 and none when no row looks like one.
	Header   int                 `json:"header_row"`
	Columns  []string            `json:"columns"`
	Frozen   [2]int              `json:"frozen"` // rows, columns
	Filter   string              `json:"filter"` // the filtered range, or ""
	Tables   []TableDescription  `json:"tables"`
	Regions  []RegionDescription `json:"regions"`
	Charts   []ChartDescription  `json:"charts"`
	Pivot    *PivotDescription   `json:"pivot,omitempty"`
	Notebook []NotebookCell      `json:"notebook_cells,omitempty"`
}

// NameDescription is a named range.
type NameDescription struct {
	Name  string `json:"name"`
	Range string `json:"range"` // with its sheet: Q3!B2:B9, or #REF!
}

// TableDescription is a named table.
type TableDescription struct {
	Name    string   `json:"name"`
	Range   string   `json:"range"` // header row included
	Columns []string `json:"columns"`
}

// RegionDescription is a linked file or a notebook cell's output sent to
// the sheet.
type RegionDescription struct {
	Name  string `json:"name"`
	Kind  string `json:"kind"` // "output" or "linked"
	Range string `json:"range"`
	File  string `json:"file,omitempty"` // a linked region's file
}

// ChartDescription is a chart: Number is what 012 export --chart and
// the MCP tools call it.
type ChartDescription struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Type   string `json:"type"`
	Data   string `json:"data"`
	At     string `json:"at"`
}

// PivotDescription is where a pivot table's data comes from.
type PivotDescription struct {
	Source string `json:"source"` // Sheet!A1:D99
}

// NotebookCell is one cell of a notebook tab.
type NotebookCell struct {
	Number int    `json:"number"`
	Kind   string `json:"kind"` // "code" or "note"
	Name   string `json:"name,omitempty"`
	Source string `json:"source"`
	// State is "ran", "failed" or "not run"; Error says why it failed.
	State string `json:"state,omitempty"`
	Error string `json:"error,omitempty"`
}

// maxCounted is the most cells Describe counts formulas and errors on.
const maxCounted = 1_000_000

// Describe describes the workbook as it is.
func Describe(w *sheet.Workbook) Description {
	d := Description{Sheets: []SheetDescription{}, Names: []NameDescription{}}
	for i, s := range w.Sheets() {
		d.Sheets = append(d.Sheets, describeSheet(s, i == w.Active()))
	}
	for _, n := range w.Names() {
		r := "#REF!"
		if !n.Gone() {
			r = sheet.Qualified(n.Sheet.Name(), n.Range)
		}
		d.Names = append(d.Names, NameDescription{Name: n.Name, Range: r})
	}
	return d
}

func describeSheet(s *sheet.Sheet, shown bool) SheetDescription {
	d := SheetDescription{Name: s.Name(), Kind: "sheet", Shown: shown, Hidden: s.Hidden(), Cells: s.Len(),
		Tables: []TableDescription{}, Regions: []RegionDescription{}, Charts: []ChartDescription{}, Columns: []string{}}
	if used, ok := s.UsedRange(); ok {
		d.Used, d.Rows, d.Cols = used.String(), used.To.Row+1, used.To.Col+1
		d.Header, d.Columns = guessHeader(s, used)
	}
	d.Formulas, d.Errors = countFormulas(s)
	d.Frozen[0], d.Frozen[1] = s.Frozen()
	if f := s.Filter(); f != nil {
		d.Filter = f.Range.String()
	}
	for _, t := range s.Tables() {
		d.Tables = append(d.Tables, TableDescription{Name: t.Name, Range: t.Range.String(), Columns: append([]string{}, t.Cols...)})
	}
	for _, r := range s.Regions() {
		d.Regions = append(d.Regions, describeRegion(s, r))
	}
	for i, c := range s.Charts() {
		d.Charts = append(d.Charts, ChartDescription{Number: i + 1, Title: ChartTitle(c), Type: c.Type.String(),
			Data: c.Data.String(), At: c.At.String()})
	}
	if p, ok := s.Pivot(); ok {
		d.Kind, d.Pivot = "pivot", &PivotDescription{Source: sheet.Qualified(p.Source, p.Range)}
	}
	if s.IsNotebook() {
		d.Kind, d.Notebook = "notebook", describeNotebook(s)
	}
	return d
}

// countFormulas counts the sheet's formulas and those showing errors,
// or returns -1 for both past maxCounted cells.
func countFormulas(s *sheet.Sheet) (formulas, errors int) {
	if s.Len() > maxCounted {
		return -1, -1
	}
	for _, a := range s.Addrs() {
		if c := s.Cell(a); c != nil && c.IsFormula() {
			formulas++
			if s.Value(a).Kind == sheet.Error {
				errors++
			}
		}
	}
	return formulas, errors
}

func describeRegion(s *sheet.Sheet, r sheet.Region) RegionDescription {
	d := RegionDescription{Name: r.Name, Kind: "output", Range: r.At.String()}
	if rng, ok := s.RegionTable(r.Name); ok {
		d.Range = rng.String()
	}
	if r.Linked() {
		d.Kind, d.File = "linked", r.File.Path
	}
	return d
}

func describeNotebook(s *sheet.Sheet) []NotebookCell {
	out := []NotebookCell{}
	for i, c := range s.NotebookCells() {
		nc := NotebookCell{Number: i + 1, Kind: "code", Name: c.Name(), Source: c.Source}
		if c.Kind != notebook.Code {
			nc.Kind = "note"
			out = append(out, nc)
			continue
		}
		switch o := s.Book().Output(c.ID); {
		case o == nil || o.Unsaved:
			nc.State = "not run"
		case o.Failed():
			nc.State, nc.Error = "failed", o.Err
		default:
			nc.State = "ran"
		}
		out = append(out, nc)
	}
	return out
}

// headerLook is how many rows below a header guessHeader reads.
const headerLook = 5

// guessHeader finds the row naming the used range's columns: its first
// row with contents, when every cell there is text and the rows below
// hold something other than text, or the row is bold. It returns the
// row's number (1 for the first) and the names, or 0.
func guessHeader(s *sheet.Sheet, used sheet.Rect) (int, []string) {
	first, ok := s.FilledBounds(used)
	if !ok {
		return 0, []string{}
	}
	row := first.From.Row
	names := make([]string, 0, used.To.Col-used.From.Col+1)
	bold, n := true, 0
	for col := used.From.Col; col <= used.To.Col; col++ {
		a := sheet.Addr{Col: col, Row: row}
		v := s.Value(a)
		switch v.Kind {
		case sheet.Empty:
			names = append(names, "")
			continue
		case sheet.Text:
		default:
			return 0, []string{}
		}
		names = append(names, v.Str)
		bold = bold && s.CellStyle(a).Bold
		n++
	}
	if n == 0 || row == used.To.Row || !bold && !typedBelow(s, used, row) {
		return 0, []string{}
	}
	return row + 1, names
}

// typedBelow reports whether the rows below row hold a number, a date,
// a boolean or a formula.
func typedBelow(s *sheet.Sheet, used sheet.Rect, row int) bool {
	for r := row + 1; r <= min(row+headerLook, used.To.Row); r++ {
		for col := used.From.Col; col <= used.To.Col; col++ {
			a := sheet.Addr{Col: col, Row: r}
			if k := s.Value(a).Kind; k != sheet.Empty && k != sheet.Text {
				return true
			}
		}
	}
	return false
}
