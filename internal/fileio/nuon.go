package fileio

import (
	"context"
	"errors"
	"io"
	"math"
	"os"
	"time"

	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/value"
)

// NUON and JSON tables become cells with their types (see
// docs/nushell/types.md for the mapping): the header row holds the
// column names, numbers stay numbers, file sizes are bytes in the Size
// format, durations are elapsed time in the Duration format, dates are
// date-time serials in the local time zone, and nested records and
// lists are their NUON text. Rows are read and stored one at a time
// (nuon.Reader), so a table streaming in is stored as it arrives.

// zone is the time zone dates are shown in: the local one, which tests
// replace.
var zone = func() *time.Location { return time.Local }

func importNUON(ctx context.Context, name string, opt Options) (*Result, error) {
	return importTable(ctx, name, opt)
}

func importJSON(ctx context.Context, name string, opt Options) (*Result, error) {
	return importTable(ctx, name, opt)
}

func importTable(ctx context.Context, name string, opt Options) (*Result, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer context.AfterFunc(ctx, func() { f.Close() })()
	var size int64
	if st, err := f.Stat(); err == nil && st.Mode().IsRegular() {
		size = st.Size()
	}
	counter := &countingReader{r: f}
	res, _, err := readTable(ctx, counter, opt.MaxCells, func(rows int) {
		opt.Progress.setRows(rows)
		opt.Progress.setFrac(counter.n, size)
	})
	return res, err
}

// readTable reads a NUON or JSON table into a new sheet a row at a time,
// keeping up to maxCells cells (see newBuilder), and reports whether the
// text was all JSON. progress is called every few hundred rows.
func readTable(ctx context.Context, in io.Reader, maxCells int, progress func(rows int)) (*Result, bool, error) {
	r := nuon.NewReader(in)
	b := newBuilder(ctx, maxCells)
	t := tableCells{b: b, cols: map[string]int{}, zone: zone(), dated: map[int]bool{}}
	row := 0
	for ; ; row++ {
		if row%256 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, false, err
			}
			progress(row)
		}
		fields, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, false, err
		}
		if b.isFull() {
			b.fits(sheet.Addr{Row: row + 1}) // counted for the note
			continue
		}
		t.header(r.Header())
		t.row(row+1, fields)
	}
	progress(row)
	s, notes := b.finish(nil)
	return &Result{Sheet: s, Rows: row + 1, Notes: notes}, r.JSON(), nil
}

// tableCells stores a table's rows: each column's name in the header
// row once it's first seen, and each value under its column.
type tableCells struct {
	b     *builder
	cols  map[string]int
	zone  *time.Location
	dated map[int]bool // columns widened for dates
	fixed bool         // widths are the sheet's own: dates widen nothing
}

// dateTimeWidth fits a date and time in the Date time format, in any
// locale's order: 9/27/2026 11:27:31, 27.09.2026 11:27:31.
const dateTimeWidth = 20

// header adds the table form's columns, in their order, before any row
// names them.
func (t *tableCells) header(cols []string) {
	for _, c := range cols {
		t.col(c)
	}
}

// col is the column named c, added after the others if it's new.
func (t *tableCells) col(c string) int {
	i, ok := t.cols[c]
	if !ok {
		i = len(t.cols)
		t.cols[c] = i
		t.b.text(sheet.Addr{Col: i}, c, sheet.Format{}, sheet.Style{})
	}
	return i
}

func (t *tableCells) row(r int, fields nuon.Row) {
	for _, f := range fields {
		t.value(sheet.Addr{Col: t.col(f.Key), Row: r}, f.Value)
	}
}

// Durations are elapsed time: a count of days, shown as hours, minutes
// and seconds, with milliseconds when there's less than a second in it.
var (
	durationFormat     = sheet.Format{Kind: sheet.FmtDuration}
	durationFormatFrac = sheet.Format{Kind: sheet.FmtDuration, Pattern: "[h]:mm:ss.000"}
)

// nsPerDay is a day in nanoseconds, a date serial's unit.
const nsPerDay = 86400e9

// value stores one value as a cell.
func (t *tableCells) value(a sheet.Addr, v nuon.Value) {
	b, none := t.b, sheet.Style{}
	c := nuonCell(v, t.zone)
	switch c.V.Kind {
	case sheet.Empty:
	case sheet.Bool:
		b.boolean(a, c.V.Num != 0, none)
	case sheet.Number:
		b.number(a, c.V.Num, c.F, none)
	default:
		b.text(a, c.V.Str, c.F, none)
	}
	if v.Kind == nuon.Date && !t.fixed && !t.dated[a.Col] && a.Valid() {
		t.dated[a.Col] = true
		b.s.LoadColWidth(a.Col, dateTimeWidth)
	}
}

// nuonCell is what a cell shows for a NUON value: its value, text as
// the value's own, and its format.
func nuonCell(v nuon.Value, zone *time.Location) sheet.LiveCell {
	num := func(n float64, f sheet.Format) sheet.LiveCell { return sheet.LiveCell{V: value.Num(n), F: f} }
	switch v.Kind {
	case nuon.Null:
		return sheet.LiveCell{}
	case nuon.Bool:
		return sheet.LiveCell{V: value.Boolean(v.Bool)}
	case nuon.Int:
		return num(float64(v.Int), sheet.Format{})
	case nuon.Float:
		if math.IsNaN(v.Float) || math.IsInf(v.Float, 0) {
			return sheet.LiveCell{V: value.Str(v.String())}
		}
		return num(v.Float, sheet.Format{})
	case nuon.String:
		return sheet.LiveCell{V: value.Str(v.Str)}
	case nuon.Filesize:
		return num(float64(v.Int), sheet.Preset(sheet.FmtSize))
	case nuon.Duration:
		f := durationFormat
		if v.Int%1e9 != 0 {
			f = durationFormatFrac
		}
		return num(float64(v.Int)/nsPerDay, f)
	case nuon.Date:
		return num(numfmt.SerialOf(v.Time.In(zone)), sheet.Preset(sheet.FmtDateTime))
	}
	return sheet.LiveCell{V: value.Str(v.String())} // binary, lists and records
}
