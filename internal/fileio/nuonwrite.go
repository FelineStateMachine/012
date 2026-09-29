package fileio

import (
	"bufio"
	"context"
	"io"
	"math"
	"strings"
	"time"

	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// A table written as NUON or JSON takes its column names from the first
// row of the range and has a record for each row after it. Cells keep
// their types by their formats: numbers in the Size format are file
// sizes, in the Duration format durations, in a date or date-time
// format dates in the local time zone (a time of day alone is a
// duration since midnight); other numbers are ints when whole and
// floats otherwise. Text that is a list, a record or binary as NUON
// writes it goes back as that value, so nested values read from NUON
// return as they came.

func exportNUON(_ context.Context, name string, snap *Snapshot, _ ExportOptions) (*ExportResult, error) {
	return exportText(name, snap, encodeNUON)
}

func exportJSON(_ context.Context, name string, snap *Snapshot, _ ExportOptions) (*ExportResult, error) {
	return exportText(name, snap, encodeJSON)
}

// exportText writes a snapshot to a file with encode.
func exportText(name string, snap *Snapshot, encode func(io.Writer, *Snapshot) (int, error)) (*ExportResult, error) {
	rows := 0
	err := writeFile(name, func(w io.Writer) error {
		n, err := encode(w, snap)
		rows = n
		return err
	})
	if err != nil {
		return nil, err
	}
	return &ExportResult{Rows: rows, Notes: formulaNotes(snap)}, nil
}

// formulaNotes says how many formulas were written as their values.
func formulaNotes(snap *Snapshot) []string {
	formulas := 0
	for _, c := range snap.Cells {
		if c.Formula {
			formulas++
		}
	}
	if formulas == 0 {
		return nil
	}
	return []string{count(formulas, "formula", "formulas") + " saved as values"}
}

// encodeNUON writes the snapshot as a NUON table, returning the rows
// written, the header among them.
func encodeNUON(w io.Writer, snap *Snapshot) (int, error) {
	cols := snapColumns(snap)
	tw := nuon.NewTableWriter(w, cols)
	n := 1
	for row := range snapRecords(snap, len(cols)) {
		if err := tw.Write(row); err != nil {
			return 0, err
		}
		n++
	}
	return n, tw.Close()
}

// encodeJSON writes the snapshot as a JSON list of records, one to a
// line.
func encodeJSON(w io.Writer, snap *Snapshot) (int, error) {
	bw := bufio.NewWriter(w)
	cols := snapColumns(snap)
	fields := make([]nuon.Field, len(cols))
	for i, c := range cols {
		fields[i].Key = c
	}
	n := 1
	var buf []byte
	bw.WriteString("[")
	for row := range snapRecords(snap, len(cols)) {
		for i, v := range row {
			fields[i].Value = v
		}
		buf = append(buf[:0], "\n"...)
		if n > 1 {
			buf = append(buf[:0], ",\n"...)
		}
		buf = nuon.AppendJSONRecord(buf, fields)
		if _, err := bw.Write(buf); err != nil {
			return 0, err
		}
		n++
	}
	if n > 1 {
		bw.WriteString("\n")
	}
	bw.WriteString("]\n")
	return n, bw.Flush()
}

// snapColumns are the names in the first row of the range, as shown; a
// blank one is named after its column letter.
func snapColumns(snap *Snapshot) []string {
	r := snap.Range
	cols := make([]string, r.To.Col-r.From.Col+1)
	for col := r.From.Col; col <= r.To.Col; col++ {
		name := ""
		if c, ok := snap.Cells[sheet.Addr{Col: col, Row: r.From.Row}]; ok {
			name = strings.TrimSpace(c.Text())
		}
		if name == "" {
			name = sheet.ColName(col)
		}
		cols[col-r.From.Col] = name
	}
	return cols
}

// snapRecords yields the values of each row after the first. The slice
// is reused from row to row.
func snapRecords(snap *Snapshot, n int) func(yield func([]nuon.Value) bool) {
	return func(yield func([]nuon.Value) bool) {
		r, loc := snap.Range, zone()
		row := make([]nuon.Value, n)
		for y := r.From.Row + 1; y <= r.To.Row; y++ {
			for x := r.From.Col; x <= r.To.Col; x++ {
				c, ok := snap.Cells[sheet.Addr{Col: x, Row: y}]
				row[x-r.From.Col] = nuon.NullValue()
				if ok {
					row[x-r.From.Col] = nuonValue(c, loc)
				}
			}
			if !yield(row) {
				return
			}
		}
	}
}

// nuonValue is a cell as a NUON value, typed by its format.
func nuonValue(c SnapCell, loc *time.Location) nuon.Value {
	switch v := c.Value; v.Kind {
	case sheet.Empty:
		return nuon.NullValue()
	case sheet.Bool:
		return nuon.BoolValue(v.Num != 0)
	case sheet.Text:
		return textValue(v.Str)
	case sheet.Number:
		return numberValue(v.Num, c.Format, loc)
	}
	return nuon.StringValue(c.Value.String()) // an error, as it shows
}

// textValue is text as a string, or as the list, record or binary it
// is written as.
func textValue(s string) nuon.Value {
	if strings.HasPrefix(s, "[") || strings.HasPrefix(s, "{") || strings.HasPrefix(s, "0x[") {
		v, err := nuon.Parse([]byte(s))
		if err == nil && (v.Kind == nuon.List || v.Kind == nuon.Record || v.Kind == nuon.Binary) && v.String() == s {
			return v
		}
	}
	return nuon.StringValue(s)
}

// maxExact is the largest magnitude below which every whole float64 is
// exact, so it's written as an int.
const maxExact = 1 << 53

func numberValue(n float64, f sheet.Format, loc *time.Location) nuon.Value {
	switch f.Kind {
	case sheet.FmtSize:
		if b, ok := wholeInt(n); ok {
			return nuon.FilesizeValue(b)
		}
	case sheet.FmtDuration:
		if ns, ok := wholeInt(n * nsPerDay); ok {
			return nuon.DurationValue(time.Duration(ns))
		}
	case sheet.FmtTime:
		if ns, ok := wholeInt(n * nsPerDay); ok && n >= 0 && n < 1 {
			return nuon.DurationValue(time.Duration(ns))
		}
		return nuon.DateValue(serialTime(n, loc))
	case sheet.FmtDate, sheet.FmtDateTime:
		return nuon.DateValue(serialTime(n, loc))
	}
	if n == math.Trunc(n) && math.Abs(n) < maxExact {
		return nuon.IntValue(int64(n))
	}
	return nuon.FloatValue(n)
}

// wholeInt rounds f to an int64, reporting false when it doesn't fit.
func wholeInt(f float64) (int64, bool) {
	f = math.Round(f)
	if math.IsNaN(f) || f >= math.MaxInt64 || f <= math.MinInt64 {
		return 0, false
	}
	return int64(f), true
}

// serialTime is the time a date serial stands for in loc, to the
// microsecond: about as fine as a serial's float holds a time today.
func serialTime(v float64, loc *time.Location) time.Time {
	days := math.Floor(v)
	us := math.Round((v - days) * 86400e6)
	if math.Abs(days) > 1e7 {
		days = math.Copysign(1e7, days) // past the year 29,000: as far as a sheet shows
	}
	return time.Date(1899, 12, 30+int(days), 0, 0, 0, int(us)*1e3, loc)
}
