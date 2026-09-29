package fileio

import (
	"fmt"
	"html"
	"sort"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/chart"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// maxHTMLCells bounds the cells a page draws, blank ones included: a
// browser shows a table of about this many at once without trouble.
// Past it the page draws the first rows and says so.
const maxHTMLCells = 200_000

// htmlLayout is where the grid's rows and columns go: the columns and
// rows drawn, in order, and the pixel offset of each from the grid's
// top-left corner, headers included.
type htmlLayout struct {
	snap       *Snapshot
	cols, rows []int
	x, y       map[int]int // left edge of a column, top edge of a row
	headW      int         // the row numbers' width
	cut        int         // rows left out past maxHTMLCells
	merged     map[sheet.Addr]sheet.Rect
	covered    map[sheet.Addr]bool
	tableRole  map[sheet.Addr]string
}

// layout places the snapshot's range, grown to hold its charts.
func layout(snap *Snapshot) *htmlLayout {
	r := snap.Range
	colsTo, rowsTo := r.To.Col, r.To.Row
	for _, c := range snap.Charts {
		colsTo = max(colsTo, chartEndCol(snap, c))
		rowsTo = max(rowsTo, chartEndRow(snap, c))
	}
	l := &htmlLayout{snap: snap, x: map[int]int{}, y: map[int]int{}}
	for col := r.From.Col; col <= min(colsTo, sheet.MaxCols-1); col++ {
		l.cols = append(l.cols, col)
	}
	maxRows := max(maxHTMLCells/max(len(l.cols), 1), 100)
	for row := r.From.Row; row <= min(rowsTo, sheet.MaxRows-1); row++ {
		if snap.HiddenRows[row] {
			continue
		}
		if len(l.rows) == maxRows {
			l.cut = rowsTo - row + 1
			break
		}
		l.rows = append(l.rows, row)
	}
	last := r.From.Row + 1
	if len(l.rows) > 0 {
		last = l.rows[len(l.rows)-1] + 1
	}
	l.headW = (max(len(strconv.Itoa(last)), 3) + 2) * htmlCellW
	x := l.headW
	for _, col := range l.cols {
		l.x[col] = x
		x += htmlColWidth(snap, col) * htmlCellW
	}
	y := htmlCellH
	for _, row := range l.rows {
		l.y[row] = y
		y += htmlRowHeight(snap, row) * htmlCellH
	}
	l.mergesAndTables()
	return l
}

func htmlColWidth(snap *Snapshot, col int) int {
	if w, ok := snap.Widths[col]; ok {
		return w
	}
	return sheet.DefaultWidth
}

func htmlRowHeight(snap *Snapshot, row int) int {
	if h, ok := snap.Heights[row]; ok && h > 0 {
		return h
	}
	return 1
}

// chartEndCol is the last column a chart covers.
func chartEndCol(snap *Snapshot, c SnapChart) int {
	col, w := c.At.Col, c.W
	for w > htmlColWidth(snap, col) && col < sheet.MaxCols-1 {
		w -= htmlColWidth(snap, col)
		col++
	}
	return col
}

// chartEndRow is the last row a chart covers.
func chartEndRow(snap *Snapshot, c SnapChart) int {
	row, h := c.At.Row, c.H
	for h > htmlRowHeight(snap, row) && row < sheet.MaxRows-1 {
		h -= htmlRowHeight(snap, row)
		row++
	}
	return row
}

// mergesAndTables notes the merged cells and the cells tables style.
func (l *htmlLayout) mergesAndTables() {
	l.merged, l.covered, l.tableRole = map[sheet.Addr]sheet.Rect{}, map[sheet.Addr]bool{}, map[sheet.Addr]string{}
	for _, m := range l.snap.Merges {
		l.merged[m.From] = m
		for row := m.From.Row; row <= m.To.Row; row++ {
			for col := m.From.Col; col <= m.To.Col; col++ {
				if a := (sheet.Addr{Col: col, Row: row}); a != m.From {
					l.covered[a] = true
				}
			}
		}
	}
	for _, t := range l.snap.Tables {
		for col := t.Range.From.Col; col <= t.Range.To.Col; col++ {
			if t.Header {
				l.tableRole[sheet.Addr{Col: col, Row: t.Range.From.Row}] = "th"
			}
			for row := t.Range.From.Row + 2; t.Banded && row <= t.Range.To.Row; row += 2 {
				l.tableRole[sheet.Addr{Col: col, Row: row}] = "band"
			}
		}
	}
}

// HTMLGrid draws the snapshot as 012's grid: column letters and row
// numbers, the cells as they show with their styles, borders and the
// looks of rules, and the charts over them. It returns the fragment and
// a note when rows were left out.
func HTMLGrid(snap *Snapshot) (string, string) {
	l := layout(snap)
	var b strings.Builder
	b.WriteString(`<div class="sheet"><table class="grid"><colgroup>`)
	fmt.Fprintf(&b, `<col style="width:%dpx">`, l.headW)
	for _, col := range l.cols {
		fmt.Fprintf(&b, `<col style="width:%dpx">`, htmlColWidth(snap, col)*htmlCellW)
	}
	b.WriteString(`</colgroup><thead><tr><th class="corner"></th>`)
	for _, col := range l.cols {
		fmt.Fprintf(&b, `<th data-c="%s">%s</th>`, sheet.ColName(col), sheet.ColName(col))
	}
	b.WriteString("</tr></thead><tbody>")
	for _, row := range l.rows {
		l.writeRow(&b, row)
	}
	b.WriteString("</tbody></table>")
	for _, c := range snap.Charts {
		l.writeChart(&b, c)
	}
	b.WriteString("</div>")
	note := ""
	if l.cut > 0 {
		note = fmt.Sprintf("HTML pages show the first %d rows: %d more are left out", len(l.rows), l.cut)
	}
	return b.String(), note
}

func (l *htmlLayout) writeRow(b *strings.Builder, row int) {
	style := ""
	if h := htmlRowHeight(l.snap, row); h > 1 {
		style = fmt.Sprintf(` style="height:%dpx"`, h*htmlCellH)
	}
	fmt.Fprintf(b, `<tr%s><th data-r="%d">%d</th>`, style, row+1, row+1)
	for _, col := range l.cols {
		a := sheet.Addr{Col: col, Row: row}
		if l.covered[a] {
			continue
		}
		l.writeCell(b, a)
	}
	b.WriteString("</tr>")
}

// writeChart draws a chart in its frame over the cells under it, as
// the screen does: the title in the top border, the range in the
// bottom one.
func (l *htmlLayout) writeChart(b *strings.Builder, c SnapChart) {
	x, y := l.at(c.At)
	fmt.Fprintf(b, `<figure class="chart" style="left:%dpx;top:%dpx;width:%dpx;height:%dpx">`,
		x, y, c.W*htmlCellW, c.H*htmlCellH)
	writeChartBody(b, c)
	b.WriteString("</figure>")
}

// writeChartBody is a chart's frame labels and drawing, inside its
// figure.
func writeChartBody(b *strings.Builder, c SnapChart) {
	fmt.Fprintf(b, `<figcaption>%s</figcaption>`, html.EscapeString(chartTitle(c.Chart)))
	b.WriteString(chart.SVG(c.Type, c.Values, c.W-4, c.H-2, chart.Options{Chart: c.ChartOptions}, chartTitle(c.Chart)))
	fmt.Fprintf(b, `<span class="range">%s</span>`, c.Data.String())
}

// chartTitle is a chart's title, or its type's name when it has none.
func chartTitle(c sheet.Chart) string {
	if c.Title != "" {
		return c.Title
	}
	return c.Type.Title() + " chart"
}

// at is the pixel offset of a cell's top-left corner; a cell in a
// hidden row or past the drawn ones sits where the next drawn one does.
func (l *htmlLayout) at(a sheet.Addr) (int, int) {
	x, ok := l.x[a.Col]
	if !ok {
		x = l.headW
	}
	y, ok := l.y[a.Row]
	if !ok {
		i := sort.SearchInts(l.rows, a.Row)
		switch {
		case i < len(l.rows):
			y = l.y[l.rows[i]]
		case len(l.rows) > 0:
			last := l.rows[len(l.rows)-1]
			y = l.y[last] + htmlRowHeight(l.snap, last)*htmlCellH
		default:
			y = htmlCellH
		}
	}
	return x, y
}

// HTMLChart draws one chart alone, in its frame.
func HTMLChart(c SnapChart) string {
	var b strings.Builder
	fmt.Fprintf(&b, `<figure class="chart solo" style="width:%dpx;height:%dpx">`, c.W*htmlCellW, c.H*htmlCellH)
	writeChartBody(&b, c)
	b.WriteString("</figure>")
	return b.String()
}
