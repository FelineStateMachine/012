package fileio

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/telemetry"
)

// builder fills a new workbook for an importer, one sheet at a time (s
// is the sheet being filled). It keeps text as text (an imported "12%"
// string must not turn into a number, nor "=A1" into a formula), falls
// back to a formula's cached value when the formula can't be translated,
// and counts what didn't fit so the notes can say.
type builder struct {
	s *sheet.Sheet

	rowsOut, colsOut int // how far past the sheet's edges the data went
	values           int // formulas kept as their values
	valueExample     string

	// The cell budget (max-cells): rows are kept whole while their cells
	// fit, and none after the first that doesn't.
	maxCells int
	cells    int          // cells stored
	row      int          // the row being filled
	rowCells []sheet.Addr // its cells stored so far
	full     bool         // the budget ran out at row fullRow
	fullRow  int
	lastRow  int // the last row the data reached
}

// newBuilder starts a workbook keeping at most maxCells cells; 0 means
// the max-cells setting, and a negative number no budget (the format's
// own limits apply). The workbook's recalculation nests in the span ctx
// carries: the import's.
func newBuilder(ctx context.Context, maxCells int) *builder {
	if maxCells == 0 {
		maxCells = sheet.MaxCells()
	}
	s := sheet.New()
	s.Book().SetTrace(telemetry.NewTrace(telemetry.ParentFrom(ctx)))
	return &builder{s: s, maxCells: maxCells}
}

// fits reports whether a is inside the worksheet and the budget,
// recording by how much data overflowed when it isn't.
func (b *builder) fits(a sheet.Addr) bool {
	b.lastRow = max(b.lastRow, a.Row)
	if a.Row >= sheet.MaxRows {
		b.rowsOut = max(b.rowsOut, a.Row-sheet.MaxRows+1)
	}
	if a.Col >= sheet.MaxCols {
		b.colsOut = max(b.colsOut, a.Col-sheet.MaxCols+1)
	}
	return a.Valid() && !b.full
}

// nextSheet starts filling another sheet, whose rows start again at 1.
func (b *builder) nextSheet(s *sheet.Sheet) {
	b.s, b.row, b.rowCells = s, 0, b.rowCells[:0]
}

// take counts a cell about to be stored against the budget, reporting
// false when it doesn't fit: then the row it's on is taken out again, so
// the sheet ends with whole rows.
func (b *builder) take(a sheet.Addr) bool {
	if b.maxCells < 0 {
		return true
	}
	if a.Row != b.row {
		b.row, b.rowCells = a.Row, b.rowCells[:0]
	}
	if b.cells < b.maxCells {
		b.cells++
		b.rowCells = append(b.rowCells, a)
		return true
	}
	for _, c := range b.rowCells {
		b.s.Unload(c)
	}
	b.cells -= len(b.rowCells)
	b.rowCells = b.rowCells[:0]
	b.full, b.fullRow = true, a.Row
	return false
}

// isFull reports whether the budget ran out, so an importer can stop
// reading rows.
func (b *builder) isFull() bool { return b.full }

// flatten replaces line breaks and tabs, which a cell can't show, with
// spaces.
func flatten(s string) string {
	if !strings.ContainsAny(s, "\r\n\t") {
		return s
	}
	s = strings.ReplaceAll(s, "\r\n", " ")
	return strings.Map(func(r rune) rune {
		switch r {
		case '\r', '\n', '\t':
			return ' '
		}
		return r
	}, s)
}

// textInput is the entry that shows s as text: s itself, or s behind a
// ' when typing it would make a number, a boolean or a formula.
func textInput(s string) string {
	s = flatten(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "'") || sheet.IsFormulaEntry(s) {
		return "'" + s
	}
	if _, _, ok := sheet.ParseValue(s); ok {
		return "'" + s
	}
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "TRUE", "FALSE":
		return "'" + s
	}
	return s
}

// entryInput is s as if typed into Sheets, for text formats: numbers,
// dates, currency and percentages become values with formats. Formulas
// stay text: a data file shouldn't run formulas (or ask JEV questions)
// just by being opened.
func entryInput(s string) string {
	s = flatten(s)
	if strings.HasPrefix(s, "'") || sheet.IsFormulaEntry(s) {
		return "'" + s
	}
	return s
}

// numInput writes v so it parses back to the same number.
func numInput(v float64) string {
	if a := math.Abs(v); a == 0 || (a >= 1e-6 && a < 1e21) {
		return strconv.FormatFloat(v, 'f', -1, 64)
	}
	return strconv.FormatFloat(v, 'E', -1, 64)
}

func (b *builder) put(a sheet.Addr, input string, f sheet.Format, st sheet.Style) {
	if input == "" && f.IsZero() && st.IsZero() || !b.fits(a) || !b.take(a) {
		return
	}
	if err := b.s.Load(a, input, f, st); err != nil {
		// Only formulas fail; keep what was written as text.
		b.s.Load(a, "'"+input, f, st)
	}
}

// text stores s as text.
func (b *builder) text(a sheet.Addr, s string, f sheet.Format, st sheet.Style) {
	b.put(a, textInput(s), f, st)
}

// entry stores s as if typed (see entryInput).
func (b *builder) entry(a sheet.Addr, s string) {
	b.put(a, entryInput(s), sheet.Format{}, sheet.Style{})
}

// number stores v with format f. Plain text formats keep the digits.
func (b *builder) number(a sheet.Addr, v float64, f sheet.Format, st sheet.Style) {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		b.put(a, "'#NUM!", f, st)
		return
	}
	b.put(a, numInput(v), f, st)
}

func (b *builder) boolean(a sheet.Addr, v bool, st sheet.Style) {
	b.put(a, strings.ToUpper(strconv.FormatBool(v)), sheet.Format{}, st)
}

// formula stores a formula in 012's syntax (with the leading =), or,
// when it doesn't parse, the value it had in the file, stored by keep.
func (b *builder) formula(a sheet.Addr, text string, f sheet.Format, st sheet.Style, keep func()) {
	if !b.fits(a) {
		return
	}
	if _, err := sheet.Parse(text); err == nil && b.take(a) {
		if b.s.Load(a, text, f, st) == nil {
			return
		}
		b.cells--
		b.rowCells = b.rowCells[:len(b.rowCells)-1]
	}
	b.kept(a, text, keep)
}

// kept stores a formula's value with keep, counting it for the notes.
func (b *builder) kept(a sheet.Addr, text string, keep func()) {
	if !b.fits(a) {
		return
	}
	keep()
	b.values++
	if b.valueExample == "" {
		b.valueExample = strings.TrimSpace(a.String() + " " + text)
	}
}

// finish recalculates the workbook and returns the sheet being filled
// with notes on what was lost.
func (b *builder) finish(notes []string) (*sheet.Sheet, []string) {
	b.s.RecalcAll()
	if b.values > 0 {
		notes = append(notes, fmt.Sprintf("%s kept as values, e.g. %s", count(b.values, "formula", "formulas"), b.valueExample))
	}
	switch {
	case b.full:
		notes = append(notes, fmt.Sprintf("only the first %s rows fit in max-cells (%s cells); %s left out",
			thousands(b.fullRow), thousands(b.maxCells), count(max(b.lastRow, b.fullRow)-b.fullRow+1+b.rowsOut, "row", "rows")))
	case b.rowsOut > 0:
		notes = append(notes, fmt.Sprintf("only the first %s rows fit; %s left out", thousands(sheet.MaxRows), count(b.rowsOut, "row", "rows")))
	}
	if b.colsOut > 0 {
		notes = append(notes, fmt.Sprintf("only columns A to %s fit; %s left out", sheet.ColName(sheet.MaxCols-1), count(b.colsOut, "column", "columns")))
	}
	return b.s, notes
}

// count writes n with a singular or plural noun: "1 row", "1,200 rows".
func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return thousands(n) + " " + many
}

// thousands writes n with thousands separators.
func thousands(n int) string {
	s := strconv.Itoa(n)
	if n < 0 {
		return "-" + thousands(-n)
	}
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
