package fileio

import (
	"iter"
	"strconv"

	"github.com/FelineStateMachine/012/internal/locale"
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
	// Locale is the sheet's, which text formats (CSV) are written in.
	Locale *locale.Locale

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

	// Spills are the cells each formula whose array spills covers, by
	// the formula's cell, for formats that keep formulas (XLSX).
	Spills map[sheet.Addr]sheet.Rect

	// CondFormats and Validations are the sheet's rules, for formats
	// that keep them (XLSX).
	CondFormats []sheet.CondFormat
	Validations []sheet.Validation
	// Notes are the cells' notes in the range, for formats that keep
	// them (XLSX, as comments). A note may be on a cell with no contents.
	Notes map[sheet.Addr]string

	// Heights are the rows' heights set by hand, in lines, and Merges
	// the merged cells in the range, for formats that keep them (XLSX).
	Heights map[int]int
	Merges  []sheet.Rect

	// Tables are the tables wholly in the range, for formats that keep
	// them (XLSX).
	Tables []sheet.Table
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
	Tables  []string // the tables a formula reads, as written
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
	notes, whole := r, r == (sheet.Rect{})
	if whole {
		notes = sheet.Rect{To: sheet.Addr{Col: sheet.MaxCols - 1, Row: sheet.MaxRows - 1}}
		r, _ = s.UsedRange()
	} else if data, ok := s.FilledBounds(r); ok {
		r.To = data.To
	} else {
		r.To = r.From
	}
	snap := &Snapshot{Range: r, Cells: map[sheet.Addr]SnapCell{}, Widths: s.Widths(), Name: name, Locale: s.Locale(),
		ColFormats: snapLines(s, false), RowFormats: snapLines(s, true), Filter: s.Filter(),
		CondFormats: s.CondFormats(), Validations: s.Validations()}
	snap.FrozenRows, snap.FrozenCols = s.Frozen()
	snap.Heights, snap.Merges = heightsMergesIn(s, r)
	for _, t := range s.Tables() {
		if whole {
			// A whole sheet's range holds its tables, blank rows and all.
			snap.Range.To.Col, snap.Range.To.Row = max(snap.Range.To.Col, t.Range.To.Col), max(snap.Range.To.Row, t.Range.To.Row)
		}
		if snap.Range.Contains(t.Range.From) && snap.Range.Contains(t.Range.To) {
			snap.Tables = append(snap.Tables, t)
		}
	}
	for _, a := range s.NotesIn(notes) {
		if snap.Notes == nil {
			snap.Notes = map[sheet.Addr]string{}
		}
		snap.Notes[a] = s.Note(a)
	}
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
		if area, ok := s.SpillArea(a); ok {
			if snap.Spills == nil {
				snap.Spills = map[sheet.Addr]sheet.Rect{}
			}
			snap.Spills[a] = area
		}
		snap.Cells[a] = SnapCell{
			Input:   c.Input,
			Value:   c.Value,
			Format:  s.DisplayFormat(a),
			Own:     s.CellFormat(a),
			Style:   s.CellStyle(a),
			Formula: c.IsFormula(),
			Sheets:  s.NamedSheets(a),
			Tables:  s.NamedTables(a),
		}
	}
	return snap
}

// heightsMergesIn are the heights of the rows of r and the merges inside
// it.
func heightsMergesIn(s *sheet.Sheet, r sheet.Rect) (map[int]int, []sheet.Rect) {
	var heights map[int]int
	for row, h := range s.Heights() {
		if row >= r.From.Row && row <= r.To.Row {
			if heights == nil {
				heights = map[int]int{}
			}
			heights[row] = h
		}
	}
	var merges []sheet.Rect
	for _, m := range s.Merges() {
		if r.Contains(m.From) && r.Contains(m.To) {
			merges = append(merges, m)
		}
	}
	return heights, merges
}

// snapLayout adds to a whole sheet's snapshot what its layout needs in
// a workbook, growing the range to hold it: rows with heights, merged
// cells, and blank cells that draw borders, which count as contents
// there.
func snapLayout(snap *Snapshot, s *sheet.Sheet) {
	snap.Heights, snap.Merges = s.Heights(), s.Merges()
	grow := func(x sheet.Rect) {
		if len(snap.Cells) == 0 && snap.Range.From == snap.Range.To {
			snap.Range = sheet.Rect{To: x.To}
		}
		snap.Range.To.Col, snap.Range.To.Row = max(snap.Range.To.Col, x.To.Col), max(snap.Range.To.Row, x.To.Row)
	}
	for _, m := range snap.Merges {
		grow(m)
	}
	for _, a := range s.BorderedIn(sheet.Rect{To: sheet.Addr{Col: sheet.MaxCols - 1, Row: sheet.MaxRows - 1}}) {
		if _, ok := snap.Cells[a]; !ok {
			snap.Cells[a] = SnapCell{Style: s.CellStyle(a)}
		}
		grow(sheet.Rect{From: a, To: a})
	}
}

// SnapBook copies every sheet of s's workbook, whole, with the named
// ranges, for formats that hold several sheets. The result is s's
// snapshot.
func SnapBook(s *sheet.Sheet) *Snapshot {
	var out *Snapshot
	all := []*Snapshot{}
	for _, t := range s.Book().Sheets() {
		sn := Snap(t, sheet.Rect{}, t.Name())
		snapLayout(sn, t)
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
// range as shown in its locale, for text formats: the rows the filter
// shows, as Sheets copies a filtered range. The slice is reused from row
// to row.
func (s *Snapshot) textRows() iter.Seq[[]string] {
	return func(yield func([]string) bool) {
		r := s.Range
		line := make([]string, r.To.Col-r.From.Col+1)
		for row := r.From.Row; row <= r.To.Row; row++ {
			if s.HiddenRows[row] {
				continue
			}
			for col := r.From.Col; col <= r.To.Col; col++ {
				line[col-r.From.Col] = ""
				if c, ok := s.Cells[sheet.Addr{Col: col, Row: row}]; ok {
					line[col-r.From.Col] = sheet.FormatTextIn(c.Value, c.Format, s.Locale.Or())
				}
			}
			if !yield(line) {
				return
			}
		}
	}
}
