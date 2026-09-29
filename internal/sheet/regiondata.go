package sheet

import "github.com/FelineStateMachine/012/internal/formula"

// What fills a region, and how formulas name one (see region.go).

// RegionData is a region's table: Rows by Cols values, row by row, the
// header row first, with the formats they come in (a file size's Size,
// a date's Date time) by index.
type RegionData struct {
	Rows, Cols int
	Values     []Value
	Formats    map[int]Format
	// Note says what was left out, e.g. rows past max-cells.
	Note string
}

// At returns the value at row r, column c and its format.
func (d *RegionData) At(r, c int) (Value, Format) {
	if d == nil || r < 0 || c < 0 || r >= d.Rows || c >= d.Cols {
		return Value{}, Format{}
	}
	i := r*d.Cols + c
	return d.Values[i], d.Formats[i]
}

// RegionDataOf copies the used range of src, from A1, as a region's
// table: how a table read by an importer becomes one.
func RegionDataOf(src *Sheet) *RegionData {
	used, ok := src.UsedRange()
	if !ok {
		return &RegionData{}
	}
	d := &RegionData{Rows: used.To.Row + 1, Cols: used.To.Col + 1}
	d.Values = make([]Value, d.Rows*d.Cols)
	for a := range src.cells.keysIn(Rect{To: used.To}) {
		i := a.Row*d.Cols + a.Col
		d.Values[i] = src.Value(a)
		if f := src.DisplayFormat(a); !f.IsZero() {
			if d.Formats == nil {
				d.Formats = map[int]Format{}
			}
			d.Formats[i] = f
		}
	}
	return d
}

// boundRegion is a name in a formula that isn't a named range: a
// region's table (nu.r1), #REF! while it has none, or the name as
// written, which shows #NAME?.
func (s *Sheet) boundRegion(nn formula.Name) Node {
	t, r, ok := s.wb.regionName(nameKey(nn.Name))
	switch {
	case !ok:
		return nn
	case r == (Rect{}):
		return formula.RefErr{}
	}
	sheet := ""
	if t != s {
		sheet = t.name
	}
	const fixed = formula.AbsCol | formula.AbsRow
	return formula.Range{Rect: r, Abs: [2]formula.Abs{fixed, fixed}, Sheet: sheet}
}
