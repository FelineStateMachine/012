package fileio

import (
	"bufio"
	"fmt"
	"maps"
	"slices"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Column and row styles: Excel keeps a style for whole columns (in
// <cols>) and rows (a <row>'s s, with customFormat), as 012 keeps line
// formats; both travel both ways. Excel has no style for the whole
// sheet: 012's goes out on every column, and a style on both A and XFD
// comes back as the sheet's.

// LineFormat is the format and style of a whole column or row.
type LineFormat struct {
	Format sheet.Format
	Style  sheet.Style
}

// snapLines copies a sheet's column (row false) or row formats, the
// sheet's own format on every column.
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

// loadColStyles gives the builder's sheet the styles of its columns, as
// styleOf (by column from 0) reports them: the sheet's when A and XFD
// have the same one, and those of the first width columns (at least
// 256) that differ from it.
func loadColStyles(b *builder, width int, styleOf func(col int) (xlsxStyle, bool)) {
	var whole xlsxStyle
	if first, ok := styleOf(0); ok {
		if last, _ := styleOf(sheet.MaxCols - 1); last == first {
			whole = first
			b.s.LoadLineFormat(false, -1, first.format, first.style)
		}
	}
	for c := range min(max(width, 256), sheet.MaxCols) {
		if st, ok := styleOf(c); ok && st != whole {
			b.s.LoadLineFormat(false, c, st.format, st.style)
		}
	}
}

// loadRowStyle gives row (from 0) of the builder's sheet its style.
func loadRowStyle(b *builder, row int, st xlsxStyle) {
	if st != (xlsxStyle{}) && row >= 0 && row < sheet.MaxRows {
		b.s.LoadLineFormat(true, row, st.format, st.style)
	}
}

// colSpec is what <cols> says of a column: its width (0 for the
// default) and style.
type colSpec struct {
	width int
	style int
}

// writeCols writes <cols>: the columns with a width or a format, runs
// of columns alike as one <col>.
func (w *xlsxWriter) writeCols(bw *bufio.Writer, snap *Snapshot) {
	specs := map[int]colSpec{}
	for c, width := range snap.Widths {
		specs[c] = colSpec{width: max(width-excelPadding, 1)}
	}
	for c, l := range snap.ColFormats {
		sp := specs[c]
		sp.style = w.styles.id(l.Format, l.Style)
		specs[c] = sp
	}
	if len(specs) == 0 {
		return
	}
	bw.WriteString(`<cols>`)
	cs := slices.Sorted(maps.Keys(specs))
	for i := 0; i < len(cs); {
		j := i + 1
		for j < len(cs) && cs[j] == cs[j-1]+1 && specs[cs[j]] == specs[cs[i]] {
			j++
		}
		sp := specs[cs[i]]
		fmt.Fprintf(bw, `<col min="%d" max="%d"`, cs[i]+1, cs[j-1]+1)
		if sp.width > 0 {
			fmt.Fprintf(bw, ` width="%d" customWidth="1"`, sp.width)
		}
		if sp.style > 0 {
			fmt.Fprintf(bw, ` style="%d"`, sp.style)
		}
		bw.WriteString(`/>`)
		i = j
	}
	bw.WriteString(`</cols>`)
}

// rowStart writes a <row> start tag, with the row's style if it has one.
func (w *xlsxWriter) rowStart(bw *bufio.Writer, snap *Snapshot, row int) {
	fmt.Fprintf(bw, `<row r="%d"`, row+1)
	if l, ok := snap.RowFormats[row]; ok {
		fmt.Fprintf(bw, ` s="%d" customFormat="1"`, w.styles.id(l.Format, l.Style))
	}
	bw.WriteString(`>`)
}

// styledRows writes, as empty rows, the styled rows (ascending) before
// row to, returning those left.
func (w *xlsxWriter) styledRows(bw *bufio.Writer, snap *Snapshot, rows []int, to int) []int {
	for len(rows) > 0 && rows[0] < to {
		w.rowStart(bw, snap, rows[0])
		bw.WriteString(`</row>`)
		rows = rows[1:]
	}
	return rows
}
