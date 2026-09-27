package fileio

import (
	"iter"
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

	// ColFormats and RowFormats are the formats of whole columns and
	// rows, for formats that keep them (XLSX).
	ColFormats, RowFormats map[int]LineFormat

	// Sheets are every sheet of the workbook, in order, for formats that
	// hold several (XLSX); the snapshot itself is one of them, the one
	// shown. Nil exports just this snapshot. Names are the workbook's
	// named ranges.
	Sheets []*Snapshot
	Names  []SnapName
	Hidden bool // a hidden sheet of Sheets, written hidden

	// FrozenRows and FrozenCols are the frozen panes; Filter is the
	// sheet's filter, if any, and HiddenRows the rows of Range it hides,
	// for formats that keep them (XLSX).
	FrozenRows, FrozenCols int
	Filter                 *sheet.Filter
	HiddenRows             map[int]bool
}

// SnapName is a named range: its name, and the range on the sheet
// named Sheet.
type SnapName struct {
	Name, Sheet string
	Range       sheet.Rect
}

// SnapCell is one non-blank cell of a snapshot.
type SnapCell struct {
	Input   string
	Value   sheet.Value
	Format  sheet.Format // as displayed, including one inferred from a formula
	Own     sheet.Format // the cell's own format
	Style   sheet.Style
	Formula bool
	Sheets  []string // the sheets a formula names, as written
}

// Text is the cell as displayed, without a width limit.
func (c SnapCell) Text() string {
	return sheet.FormatText(c.Value, c.Format)
}

// Snap copies the cells of s in r. A zero r means the whole sheet, from
// A1 to the last cell with contents; any other r is trimmed to its last
// row and column with contents, so exporting whole columns writes their
// data rather than a million blank lines.
func Snap(s *sheet.Sheet, r sheet.Rect, name string) *Snapshot {
	if r == (sheet.Rect{}) {
		r, _ = s.UsedRange()
	} else if data, ok := s.FilledBounds(r); ok {
		r.To = data.To
	} else {
		r.To = r.From
	}
	snap := &Snapshot{Range: r, Cells: map[sheet.Addr]SnapCell{}, Widths: s.Widths(), Name: name,
		ColFormats: snapLines(s, false), RowFormats: snapLines(s, true), Filter: s.Filter()}
	snap.FrozenRows, snap.FrozenCols = s.Frozen()
	if f := snap.Filter; f != nil {
		snap.HiddenRows = map[int]bool{}
		for row := max(f.Range.From.Row, r.From.Row); row <= min(f.Range.To.Row, r.To.Row); row++ {
			if s.RowHidden(row) {
				snap.HiddenRows[row] = true
			}
		}
	}
	for _, a := range s.Addrs() {
		if !r.Contains(a) {
			continue
		}
		c := s.Cell(a)
		snap.Cells[a] = SnapCell{
			Input:   c.Input,
			Value:   c.Value,
			Format:  s.DisplayFormat(a),
			Own:     s.CellFormat(a),
			Style:   s.CellStyle(a),
			Formula: c.IsFormula(),
			Sheets:  s.NamedSheets(a),
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
		sn.Hidden = t.Hidden() && t != s
		if t == s {
			out = sn
		}
		all = append(all, sn)
	}
	out.Sheets = all
	for _, n := range s.Book().Names() {
		if !n.Gone() {
			out.Names = append(out.Names, SnapName{Name: n.Name, Sheet: n.Sheet.Name(), Range: n.Range})
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

// textRows yields the snapshot's displayed text row by row across its
// range, for text formats. The slice is reused from row to row.
func (s *Snapshot) textRows() iter.Seq[[]string] {
	return func(yield func([]string) bool) {
		r := s.Range
		line := make([]string, r.To.Col-r.From.Col+1)
		for row := r.From.Row; row <= r.To.Row; row++ {
			for col := r.From.Col; col <= r.To.Col; col++ {
				line[col-r.From.Col] = ""
				if c, ok := s.Cells[sheet.Addr{Col: col, Row: row}]; ok {
					line[col-r.From.Col] = c.Text()
				}
			}
			if !yield(line) {
				return
			}
		}
	}
}
