package fileio

import (
	"maps"
	"math"
	"slices"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Column and row styles: Excel keeps a style for a whole column (in
// <cols>) or row, as 012 keeps line formats; both travel both ways.

// LineFormat is the format and style of a whole column or row.
type LineFormat struct {
	Format sheet.Format
	Style  sheet.Style
}

// snapLines copies a sheet's column (row false) or row formats. Excel
// has no format for the whole sheet, so with one every column gets it,
// under its own.
func snapLines(s *sheet.Sheet, row bool) map[int]LineFormat {
	fs, ss := s.ColFormats(), s.ColStyles()
	if row {
		fs, ss = s.RowFormats(), s.RowStyles()
	}
	out := map[int]LineFormat{}
	if f, st := s.SheetFormat(); !row && (!f.IsZero() || !st.IsZero()) {
		for c := range sheet.MaxCols {
			out[c] = LineFormat{f, st}
		}
	}
	for n, f := range fs {
		l := out[n]
		l.Format = f
		out[n] = l
	}
	for n, st := range ss {
		l := out[n]
		l.Style = st
		out[n] = l
	}
	return out
}

// importXLSXColumns reads the sheet's column widths and styles; a style
// on every column is the sheet's.
func importXLSXColumns(x *excelize.File, b *builder, ws string, styles map[int]xlsxStyle) {
	cols := map[int]xlsxStyle{}
	for c := range sheet.MaxCols {
		colName, _ := excelize.ColumnNumberToName(c + 1)
		if w, err := x.GetColWidth(ws, colName); err == nil && math.Abs(w-excelDefaultWidth) >= 0.01 && math.Abs(w-9.140625) >= 0.01 {
			b.s.SetColWidth(c, max(int(math.Round(w))+excelPadding, 1))
		}
		if id, err := x.GetColStyle(ws, colName); err == nil && id != 0 {
			if st := styleOf(x, id, styles); st != (xlsxStyle{}) {
				cols[c] = st
			}
		}
	}
	if st := cols[0]; len(cols) == sheet.MaxCols && !slices.ContainsFunc(slices.Collect(maps.Values(cols)), func(o xlsxStyle) bool { return o != st }) {
		b.s.LoadLineFormat(false, -1, st.format, st.style)
		return
	}
	for c, st := range cols {
		b.s.LoadLineFormat(false, c, st.format, st.style)
	}
}

// importXLSXRowStyle keeps the style of row, if it has one of its own.
func importXLSXRowStyle(x *excelize.File, b *builder, rows *excelize.Rows, row int, styles map[int]xlsxStyle) {
	if id := rows.GetRowOpts().StyleID; id != 0 && row < sheet.MaxRows {
		if st := styleOf(x, id, styles); st != (xlsxStyle{}) {
			b.s.LoadLineFormat(true, row, st.format, st.style)
		}
	}
}

// colStyles sets the snapshot's column styles on a stream, before any
// row: runs of columns with the same style as one range.
func (w *xlsxWriter) colStyles(sw *excelize.StreamWriter, cols map[int]LineFormat) error {
	ns := slices.Sorted(maps.Keys(cols))
	for i := 0; i < len(ns); {
		j := i + 1
		for j < len(ns) && ns[j] == ns[j-1]+1 && cols[ns[j]] == cols[ns[i]] {
			j++
		}
		l := cols[ns[i]]
		id, err := w.styleFor(l.Format, l.Style)
		if err != nil {
			return err
		}
		if err := sw.SetColStyle(ns[i]+1, ns[j-1]+1, id); err != nil {
			return err
		}
		i = j
	}
	return nil
}

// rowOpts is how the stream writes row: with its style, if it has one.
func (w *xlsxWriter) rowOpts(rows map[int]LineFormat, row int) ([]excelize.RowOpts, error) {
	l, ok := rows[row]
	if !ok {
		return nil, nil
	}
	id, err := w.styleFor(l.Format, l.Style)
	return []excelize.RowOpts{{StyleID: id}}, err
}

// styledRowsAfter writes the rows past the exported range that have a
// style of their own, as empty rows.
func (w *xlsxWriter) styledRowsAfter(sw *excelize.StreamWriter, rows map[int]LineFormat, last int) error {
	for _, row := range slices.Sorted(maps.Keys(rows)) {
		if row <= last {
			continue
		}
		opts, err := w.rowOpts(rows, row)
		if err != nil {
			return err
		}
		cell, _ := excelize.CoordinatesToCellName(1, row+1)
		if err := sw.SetRow(cell, nil, opts...); err != nil {
			return err
		}
	}
	return nil
}
