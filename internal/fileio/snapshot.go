package fileio

import (
	"012/internal/sheet"
)

// Snapshot is a copy of the part of a sheet being exported, taken on the
// UI goroutine so the file can be written in the background while the
// sheet keeps changing.
type Snapshot struct {
	// Range is what's exported. Whole-sheet exports start at A1, as
	// Sheets' downloads do, so cells keep their addresses.
	Range  sheet.Rect
	Cells  map[sheet.Addr]SnapCell
	Widths map[int]int // non-default column widths
	Name   string      // what to call the data: a sheet or table name
}

// SnapCell is one non-blank cell of a snapshot.
type SnapCell struct {
	Input   string
	Value   sheet.Value
	Format  sheet.Format // as displayed, including one inferred from a formula
	Own     sheet.Format // the cell's own format
	Style   sheet.Style
	Formula bool
}

// Text is the cell as displayed, without a width limit.
func (c SnapCell) Text() string {
	return sheet.FormatText(c.Value, c.Format)
}

// Snap copies the cells of s in r. A zero r means the whole sheet, from
// A1 to the last cell with contents.
func Snap(s *sheet.Sheet, r sheet.Rect, name string) *Snapshot {
	if r == (sheet.Rect{}) {
		r, _ = s.UsedRange()
	}
	snap := &Snapshot{Range: r, Cells: map[sheet.Addr]SnapCell{}, Widths: s.Widths(), Name: name}
	for _, a := range s.Addrs() {
		if !r.Contains(a) {
			continue
		}
		c := s.Cell(a)
		snap.Cells[a] = SnapCell{
			Input:   c.Input,
			Value:   c.Value,
			Format:  s.DisplayFormat(a),
			Own:     c.Format,
			Style:   c.Style,
			Formula: c.IsFormula(),
		}
	}
	return snap
}

// rows returns the snapshot's displayed text row by row across its
// range, for text formats.
func (s *Snapshot) rows() [][]string {
	r := s.Range
	out := make([][]string, 0, r.To.Row-r.From.Row+1)
	for row := r.From.Row; row <= r.To.Row; row++ {
		line := make([]string, r.To.Col-r.From.Col+1)
		for col := r.From.Col; col <= r.To.Col; col++ {
			if c, ok := s.Cells[sheet.Addr{Col: col, Row: row}]; ok {
				line[col-r.From.Col] = c.Text()
			}
		}
		out = append(out, line)
	}
	return out
}
