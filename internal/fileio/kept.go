package fileio

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/notebook"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// NUON has no currency or percentage, so a cell showing $3.50 reaches
// nushell as the number 3.5, and a date as a datetime, whatever its
// pattern. When a notebook cell's output comes back to a sheet, a
// column takes again the format its namesake had in the ranges the cell
// read ($sheet.A1:C9, $selection), when its values are still of that
// type: a price column read as currency and sent back is currency
// again, a date column shows its dates as it did (see
// docs/nushell/types.md).

// TextInput is s as an entry that stays text, whatever it looks like:
// '00123 for 00123, as importing stores text.
func TextInput(s string) string { return textInput(s) }

// NumInput is v as an entry: 3.5, 1E+21.
func NumInput(v float64) string { return numInput(v) }

// CodeKind says whether a number format code shows a date, a time,
// both or a duration; FmtCustom for anything else.
func CodeKind(code string) sheet.FormatKind { return dateKind(code) }

// SnapColumns are the names of snap's columns, from its range's first
// row, as a table written as NUON or JSON names them.
func SnapColumns(snap *Snapshot) []string { return snapColumns(snap) }

// ColumnFormats are the formats of snap's columns, by the columns'
// names: the format of each column's first number under the header,
// when it isn't Automatic.
func ColumnFormats(snap *Snapshot) map[string]sheet.Format {
	r := snap.Range
	out := map[string]sheet.Format{}
	for i, name := range snapColumns(snap) {
		col := r.From.Col + i
		for row := r.From.Row + 1; row <= r.To.Row; row++ {
			c, ok := snap.Cells[sheet.Addr{Col: col, Row: row}]
			if !ok || c.Value.Kind != sheet.Number {
				continue
			}
			if !c.Format.IsZero() {
				out[name] = c.Format
			}
			break
		}
	}
	return out
}

// nuonKind is the NUON type a number shown in f is written as: a size,
// a duration (a time of day among them), a date, or a plain number.
func nuonKind(f sheet.Format) sheet.FormatKind {
	switch k := f.Kind; k {
	case sheet.FmtSize, sheet.FmtDuration:
		return k
	case sheet.FmtTime:
		return sheet.FmtDuration
	case sheet.FmtDate, sheet.FmtDateTime:
		return sheet.FmtDate
	case sheet.FmtCustom:
		return nuonKind(sheet.Format{Kind: dateKind(f.Pattern)})
	}
	return sheet.FmtNumber
}

// KeepFormats gives each number under a header named in formats that
// format, when NUON writes both as the same type: a plain number takes
// its column's currency, a date its column's pattern, a size its
// column's decimals.
func KeepFormats(rows *TailRows, formats map[string]sheet.Format) {
	if len(formats) == 0 {
		return
	}
	for i, h := range rows.Header {
		f, ok := formats[strings.TrimSpace(h.V.String())]
		if !ok {
			continue
		}
		for _, row := range rows.Rows {
			if i < len(row) && row[i].V.Kind == sheet.Number && nuonKind(row[i].F) == nuonKind(f) {
				row[i].F = f
			}
		}
	}
}

// RangeResolver finds the sheet and range a $sheet reference names.
type RangeResolver func(ref string) (*sheet.Sheet, sheet.Rect, error)

// OutputFormats are the formats KeepFormats keeps for output o of w:
// those of the columns of the ranges its run read, found by resolve,
// and of its selection.
func OutputFormats(w *sheet.Workbook, o *notebook.Output, resolve RangeResolver) map[string]sheet.Format {
	out := map[string]sheet.Format{}
	add := func(t *sheet.Sheet, r sheet.Rect) {
		for name, f := range ColumnFormats(Snap(t, r, t.Name())) {
			if _, ok := out[name]; !ok {
				out[name] = f
			}
		}
	}
	_, _, refs := notebook.Parse(o.Source).Command()
	for _, ref := range refs {
		if t, r, err := resolve(ref.Ref); err == nil {
			add(t, r)
		}
	}
	if o.Selection != "" {
		name, cells := sheet.SplitSheet(o.Selection)
		r, ok := sheet.ParseRange(cells)
		if t := w.Lookup(name); ok && t != nil {
			add(t, r)
		}
	}
	return out
}
