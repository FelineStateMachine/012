package fileio

import (
	"strconv"

	"github.com/FelineStateMachine/012/internal/sheet"
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

	// Sheets are every sheet of the workbook, in order, for formats that
	// hold several (XLSX); the snapshot itself is one of them, the one
	// shown. Nil exports just this snapshot. Names are the workbook's
	// named ranges as name and Excel reference, e.g. Q3!$B$2:$B$9.
	Sheets []*Snapshot
	Names  [][2]string
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

// SnapBook copies every sheet of s's workbook, whole, with the named
// ranges, for formats that hold several sheets. The result is s's
// snapshot.
func SnapBook(s *sheet.Sheet) *Snapshot {
	var out *Snapshot
	all := []*Snapshot{}
	for _, t := range s.Book().Sheets() {
		sn := Snap(t, sheet.Rect{}, t.Name())
		if t == s {
			out = sn
		}
		all = append(all, sn)
	}
	out.Sheets = all
	for _, n := range s.Book().Names() {
		if !n.Gone() {
			out.Names = append(out.Names, [2]string{n.Name, excelRange(n.Sheet.Name(), n.Range)})
		}
	}
	return out
}

// excelRange writes a range as Excel's defined names hold it: absolute,
// with its sheet, Q3!$B$2:$B$9.
func excelRange(ws string, r sheet.Rect) string {
	abs := func(a sheet.Addr) string {
		return "$" + sheet.ColName(a.Col) + "$" + strconv.Itoa(a.Row+1)
	}
	ref := abs(r.From)
	if r.From != r.To {
		ref += ":" + abs(r.To)
	}
	return sheet.QuoteSheet(sheetName(ws)) + "!" + ref
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
