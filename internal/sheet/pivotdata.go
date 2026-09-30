package sheet

import (
	"errors"
	"fmt"
)

// Where a pivot table reads its source's rows: a sheet's cells, or a
// linked source's rows as they stream past (source.go), which a pivot
// over a source gathers in the background (GatherPivot) and lays out
// once the host answers.

// pivotData is a pivot's source, read a row at a time.
type pivotData interface {
	value(a Addr) Value
	// format is what the value at a shows in.
	format(a Addr) Format
	// colFormat is the number format of column col's data in r.
	colFormat(col int, r Rect) Format
	// lastRow is r's last row that may hold data.
	lastRow(r Rect) int
	// blankRow reports whether row is blank across r's columns.
	blankRow(row int, r Rect) bool
	// passes reports whether row meets the filters' tests.
	passes(row int, tests []colTest) bool
}

// sheetData is a sheet's cells as a pivot's source.
type sheetData struct{ s *Sheet }

func (d sheetData) value(a Addr) Value                   { return d.s.Value(a) }
func (d sheetData) format(a Addr) Format                 { return d.s.DisplayFormat(a) }
func (d sheetData) lastRow(r Rect) int                   { return d.s.filterData(r).To.Row }
func (d sheetData) blankRow(row int, r Rect) bool        { return d.s.rowBlank(row, r) }
func (d sheetData) passes(row int, tests []colTest) bool { return d.s.rowPasses(row, tests) }

// colFormat is the column's own format when it has one, else the one
// most of its numbers show in, ties going to the one met first. So one
// cell typed differently ("$9,000" among "$2.50"s) doesn't restyle
// every result, as in Sheets.
func (d sheetData) colFormat(col int, r Rect) Format {
	s := d.s
	if f := s.lines.cols[col].Format; !f.IsZero() {
		return f
	}
	counts := map[Format]int{}
	var seen []Format // in the order first met
	for row, ok := s.cells.filled.nextRow(col, r.From.Row+1, 1); ok && row <= r.To.Row; row, ok = s.cells.filled.nextRow(col, row+1, 1) {
		a := Addr{Col: col, Row: row}
		if s.Value(a).Kind != Number {
			continue
		}
		f := s.DisplayFormat(a)
		if counts[f] == 0 {
			seen = append(seen, f)
		}
		counts[f]++
	}
	var best Format
	most := 0
	for _, f := range seen {
		if counts[f] > most {
			best, most = f, counts[f]
		}
	}
	return best
}

// rowData is a source's rows as a pivot's source: the row streaming
// past, its values by column.
type rowData struct {
	shape SourceShape
	row   []LiveCell
}

func (d *rowData) cell(col int) LiveCell {
	if col < len(d.row) {
		return d.row[col]
	}
	return LiveCell{}
}

func (d *rowData) value(a Addr) Value { return d.cell(a.Col).V }

func (d *rowData) format(a Addr) Format {
	if c := d.cell(a.Col); !c.F.IsZero() {
		return c.F
	}
	return d.colFormat(a.Col, Rect{})
}

func (d *rowData) colFormat(col int, _ Rect) Format {
	if col < len(d.shape.Formats) {
		return d.shape.Formats[col]
	}
	return Format{}
}

func (d *rowData) lastRow(Rect) int { return d.shape.Rows }

func (d *rowData) blankRow(int, Rect) bool {
	for _, c := range d.row {
		if c.V.Kind != Empty {
			return false
		}
	}
	return true
}

func (d *rowData) passes(_ int, tests []colTest) bool {
	for _, t := range tests {
		c := d.cell(t.col)
		f := c.F
		if f.IsZero() {
			f = d.colFormat(t.col, Rect{})
		}
		shown := ""
		if c.V.Kind != Empty {
			shown = FormatText(c.V, f)
		}
		if t.hidden[shown] || !t.cond(c.V, shown) {
			return false
		}
	}
	return true
}

// PivotGroups is a pivot table's source rows gathered into groups, as
// GatherPivot leaves them for the layout.
type PivotGroups struct {
	calc *pivotCalc
}

// Records counts the source rows the groups summarize.
func (g *PivotGroups) Records() int { return g.calc.records }

// PivotCols are the source columns a pivot reads, for a source to
// stream no others.
func PivotCols(p Pivot) []int { return p.fieldCols() }

// GatherPivot gathers a pivot's groups from a linked source of shape
// shape, whose rows scan streams: it calls its argument with each row
// (counting from 0, under the header) and its values by column (those
// PivotCols names, the rest blank), stopping when told. It touches no
// workbook, so it runs in the background.
func GatherPivot(p Pivot, shape SourceShape, scan func(each func(row int, vals []LiveCell) bool) error) (*PivotGroups, error) {
	d := &rowData{shape: shape}
	c := newPivotCalc(nil, &p, d)
	tests := pivotTests(p.Filters)
	err := scan(func(row int, vals []LiveCell) bool {
		d.row = vals
		c.gatherRow(row+1, tests)
		return true
	})
	if err != nil {
		return nil, err
	}
	d.row = nil
	c.sortRows(c.root, 0)
	c.sortCols()
	return &PivotGroups{calc: c}, nil
}

// pivotKey identifies a pivot's definition, for the answers a source's
// host keeps.
func pivotKey(p Pivot) string { return fmt.Sprintf("%+v", p) }

// sourcePivot is s's pivot over the linked source reg, as its host has
// gathered it: nil while it's on its way, s then waiting for it.
func (w *Workbook) sourcePivot(s *Sheet, reg Region, p *Pivot) (*pivotCalc, error) {
	if w.sources == nil {
		return nil, errors.New("Nothing reads linked sources here")
	}
	q := SourceQuestion{Kind: AskPivot, Source: reg.Name, Pivot: *p}
	ans, ok := w.sources.Answer(q)
	switch {
	case !ok:
		k := q.Key()
		if w.src.pivots == nil {
			w.src.pivots = map[string][]*Sheet{}
		}
		w.src.pivots[k] = append(w.src.pivots[k], s)
		return nil, nil
	case ans.Pivot == nil:
		why := ans.Why
		if why == "" {
			why = reg.Name + " can't be read"
		}
		return nil, errors.New(why)
	}
	c := *ans.Pivot.calc // the host keeps its own for the next layout
	c.w, c.p = w, p
	return &c, nil
}
