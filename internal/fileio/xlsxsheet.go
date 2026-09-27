package fileio

import (
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Excel's own limits on a worksheet. A reference past them is an error,
// as it is in Excel.
const (
	excelRows = 1 << 20
	excelCols = 1 << 14
)

// xlsxCell is a cell of a worksheet row, as the reader passes it on.
type xlsxCell struct {
	col   int         // from 1
	style int         // the cell format index (s), 0 when not given
	typ   string      // the type (t): s, str, inlineStr, b, e, n, d or ""
	value string      // the value, a shared string looked up
	f     cellFormula // its <f>, see xlsxSheetReader.formula
	hasF  bool        // the cell has a formula element, even an empty one
}

// kept reports whether excelize's row reader would return the cell: it
// has a value or a formula element. The others are formatted blanks.
func (c *xlsxCell) kept() bool { return c.value != "" || c.hasF }

// xlsxCol is a <col> element: widths and styles for columns min to max.
type xlsxCol struct {
	min, max int
	width    float64
	hasWidth bool
	style    int
}

// xlsxSheetReader streams a worksheet's rows. Opening it reads what
// comes before the cells: column widths and styles, the default width.
type xlsxSheetReader struct {
	bk   *xlsxBook
	x    *xmlStream
	cols []xlsxCol
	// sheetFormatPr's defaultColWidth and baseColWidth.
	defaultWidth float64
	baseWidth    int

	colStyles []int // by column, see colStyle

	// The first sheet view's frozen panes (see readPane).
	paneRead               bool
	frozenRows, frozenCols int

	// The rules after the rows, see xlsxrulesread.go, and how many
	// conditional formats Excel 2010's extension holds.
	cfs      []xlsxCF
	dvs      []xlsxDV
	extRules int

	inData bool // inside <sheetData>
	row    xlsxRowData
	shared map[int]sharedFormula
	buf    []byte
}

// xlsxRowData is the row just read.
type xlsxRowData struct {
	num   int // from 1
	style int // the row's cell format index, for cells not written
	cells []xlsxCell
}

// sharedFormula is the master cell of a shared formula.
type sharedFormula struct {
	text     string
	col, row int
}

// openSheet opens sheet i. A sheet with no worksheet part (a chart
// sheet) reads as empty.
func (bk *xlsxBook) openSheet(i int) (*xlsxSheetReader, error) {
	info := bk.sheets[i]
	if info.part == "" {
		return nil, fmt.Errorf("sheet %s: its part is missing", info.name)
	}
	x, err := bk.pkg.open(info.part)
	if err != nil {
		return nil, err
	}
	r := &xlsxSheetReader{bk: bk, x: x, shared: map[int]sharedFormula{}}
	if info.kind != "worksheet" {
		return r, nil
	}
	return r, r.readHead()
}

func (r *xlsxSheetReader) close() error { return r.x.close() }

// readHead reads up to the start of <sheetData>.
func (r *xlsxSheetReader) readHead() error {
	for {
		t, err := r.x.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		se, ok := t.(xml.StartElement)
		if !ok || r.x.depth != 2 && se.Name.Local != "col" && se.Name.Local != "pane" {
			continue
		}
		switch se.Name.Local {
		case "pane":
			if r.x.depth == 4 {
				r.readPane(se)
			}
		case "sheetData":
			r.inData = true
			return nil
		case "sheetFormatPr":
			r.defaultWidth, _ = strconv.ParseFloat(attrOr(se, "defaultColWidth", "0"), 64)
			r.baseWidth = intAttr(se, "baseColWidth", 0)
		case "col":
			if r.x.depth == 3 && len(r.cols) < excelCols {
				w, err := strconv.ParseFloat(attrOr(se, "width", ""), 64)
				r.cols = append(r.cols, xlsxCol{
					min: intAttr(se, "min", 0), max: intAttr(se, "max", 0),
					width: w, hasWidth: err == nil, style: intAttr(se, "style", 0),
				})
			}
		}
	}
}

// colStyle is the cell format of column col (from 1): the first <col>
// covering it with a style. Columns up to sheet.MaxCols, the ones asked
// for once per cell, are looked up once.
func (r *xlsxSheetReader) colStyle(col int) int {
	if r.colStyles == nil {
		r.colStyles = make([]int, sheet.MaxCols)
		for _, c := range r.cols {
			for i := max(c.min, 1); i <= min(c.max, sheet.MaxCols); i++ {
				if r.colStyles[i-1] == 0 {
					r.colStyles[i-1] = c.style
				}
			}
		}
	}
	if col >= 1 && col <= len(r.colStyles) {
		return r.colStyles[col-1]
	}
	for _, c := range r.cols {
		if c.min <= col && col <= c.max && c.style != 0 {
			return c.style
		}
	}
	return 0
}

// lineStyle is the style of column col (from 1) as a whole: that of the
// last <col> covering it, as for its width (and as excelize reads it).
func (r *xlsxSheetReader) lineStyle(col int) int {
	style := 0
	for _, c := range r.cols {
		if c.min <= col && col <= c.max {
			style = c.style
		}
	}
	return style
}

// colWidth is column col's width in Excel's characters: the last <col>
// covering it with a width, else the sheet's default.
func (r *xlsxSheetReader) colWidth(col int) float64 {
	w := 0.0
	for _, c := range r.cols {
		if c.min <= col && col <= c.max && c.hasWidth {
			w = c.width
		}
	}
	switch {
	case w != 0:
		return w
	case r.defaultWidth > 0:
		return r.defaultWidth
	case r.baseWidth > 0:
		return float64(r.baseWidth)
	}
	return excelDefaultWidth
}

// next reads the next row into r.row, returning false after the last.
func (r *xlsxSheetReader) next() (bool, error) {
	for r.inData {
		t, err := r.x.next()
		if err != nil {
			return false, eofAsUnexpected(err)
		}
		switch t := t.(type) {
		case xml.StartElement:
			if t.Name.Local == "row" && r.x.depth == 3 {
				return true, r.readRow(t)
			}
		case xml.EndElement:
			if r.x.depth < 2 {
				r.inData = false
			}
		}
	}
	return false, nil
}

// readRow reads a <row>. A row without a number follows the one before.
func (r *xlsxSheetReader) readRow(se xml.StartElement) error {
	num := intAttr(se, "r", 0)
	if num <= 0 {
		num = r.row.num + 1
	}
	if num > excelRows {
		return fmt.Errorf("row %s is past Excel's last row, %s", thousands(num), thousands(excelRows))
	}
	r.row.num, r.row.style = num, intAttr(se, "s", 0)
	r.row.cells = r.row.cells[:0]
	col := 0
	for {
		t, err := r.x.next()
		if err != nil {
			return eofAsUnexpected(err)
		}
		switch t := t.(type) {
		case xml.StartElement:
			if t.Name.Local != "c" || r.x.depth != 4 {
				if err := r.x.skip(); err != nil {
					return err
				}
				continue
			}
			if len(r.row.cells) >= excelCols {
				return fmt.Errorf("row %s has more than %s cells", thousands(num), thousands(excelCols))
			}
			if col, err = r.readCell(t, col); err != nil {
				return err
			}
		case xml.EndElement:
			return nil
		}
	}
}

// readCell reads a <c> following column prev and appends it to the row,
// returning its column.
func (r *xlsxSheetReader) readCell(se xml.StartElement, prev int) (int, error) {
	c := xlsxCell{col: prev + 1, style: intAttr(se, "s", 0)}
	c.typ, _ = attr(se, "t")
	if ref, ok := attr(se, "r"); ok && ref != "" {
		col, _, ok := parseCellRef(ref)
		if !ok {
			return 0, fmt.Errorf("row %s: %q isn't a cell reference", thousands(r.row.num), ref)
		}
		c.col = col
	}
	if c.col > excelCols {
		return 0, fmt.Errorf("row %s: a cell past Excel's last column, XFD", thousands(r.row.num))
	}
	var v, is string
	var hasIS bool
	var f cellFormula
	depth := r.x.depth
	for {
		t, err := r.x.next()
		if err != nil {
			return 0, eofAsUnexpected(err)
		}
		if _, ok := t.(xml.EndElement); ok && r.x.depth < depth {
			break
		}
		se, ok := t.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "v":
			r.buf, err = r.x.text(r.buf[:0])
			v = string(r.buf)
		case "f":
			c.hasF = true
			f, err = r.readFormula(se)
		case "is":
			r.buf, err = inlineText(r.x, r.buf[:0])
			is, hasIS = string(unescapeOOXML(r.buf)), true
		default:
			err = r.x.skip()
		}
		if err != nil {
			return 0, err
		}
	}
	c.value = r.value(c.typ, v, is, hasIS)
	if c.hasF {
		c.f = f
		if _, ok := r.shared[f.si]; !ok && f.typ == "shared" && f.si >= 0 && f.ref != "" {
			r.shared[f.si] = sharedFormula{text: f.text, col: c.col, row: r.row.num}
		}
	}
	r.row.cells = append(r.row.cells, c)
	return c.col, nil
}

// value is a cell's value: its shared string, its inline string, or the
// text of its <v>.
func (r *xlsxSheetReader) value(typ, v, is string, hasIS bool) string {
	switch typ {
	case "s":
		if v == "" {
			return ""
		}
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return ""
		}
		s, _ := r.bk.sst.get(i)
		return s
	case "inlineStr":
		if hasIS {
			return is
		}
	}
	return v
}

// cellFormula is an <f> element.
type cellFormula struct {
	text, typ, ref string
	si             int // -1 when not given
}

func (r *xlsxSheetReader) readFormula(se xml.StartElement) (cellFormula, error) {
	f := cellFormula{si: intAttr(se, "si", -1)}
	f.typ, _ = attr(se, "t")
	f.ref, _ = attr(se, "ref")
	var err error
	r.buf, err = r.x.text(r.buf[:0])
	f.text = string(r.buf)
	return f, err
}

// formula is the formula of cell c of the current row: its own text,
// or for a cell sharing a formula, the first cell's with its relative
// references moved by the distance between the cells ("" when the first
// is missing). Shared formulas are expanded only on request, and only
// up to the limit on their text: a small file of shared formulas could
// otherwise expand to gigabytes.
func (r *xlsxSheetReader) formula(c *xlsxCell) (string, error) {
	f := c.f
	if !c.hasF || f.typ != "shared" || f.si < 0 {
		return f.text, nil
	}
	m, ok := r.shared[f.si]
	if !ok {
		return "", nil
	}
	out := shiftFormula(m.text, c.col-m.col, r.row.num-m.row)
	if r.bk.expanded += int64(len(out)); r.bk.expanded > r.bk.pkg.lim.shared {
		return "", fmt.Errorf("shared formulas expand to more than %s: %w", mb(r.bk.pkg.lim.shared), errXLSXLimit)
	}
	return out, nil
}

// parseCellRef parses an A1 reference (without $) into a column and row
// from 1, within Excel's limits.
func parseCellRef(ref string) (col, row int, ok bool) {
	i := 0
	for i < len(ref) && i < 4 && isLetter(ref[i]) {
		col = col*26 + int(upper(ref[i])-'A'+1)
		i++
	}
	if i == 0 || i == len(ref) || col > excelCols {
		return 0, 0, false
	}
	for _, c := range []byte(ref[i:]) {
		if !isDigitByte(c) {
			return 0, 0, false
		}
		row = row*10 + int(c-'0')
		if row > excelRows {
			return 0, 0, false
		}
	}
	return col, row, row >= 1
}

func isLetter(c byte) bool { return c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' }

func upper(c byte) byte {
	if c >= 'a' && c <= 'z' {
		return c - 'a' + 'A'
	}
	return c
}

// dimension is the last row of sheet i by its <dimension> element, for
// the progress bar; 0 when it doesn't say.
func (bk *xlsxBook) dimension(i int) int {
	if bk.sheets[i].part == "" {
		return 0
	}
	x, err := bk.pkg.open(bk.sheets[i].part)
	if x == nil || err != nil {
		return 0
	}
	defer x.close()
	for {
		t, err := x.next()
		if err != nil {
			return 0
		}
		se, ok := t.(xml.StartElement)
		switch {
		case !ok:
		case se.Name.Local == "sheetData":
			return 0
		case se.Name.Local == "dimension":
			ref, _ := attr(se, "ref")
			if _, to, ok := strings.Cut(ref, ":"); ok {
				_, row, _ := parseCellRef(strings.ReplaceAll(to, "$", ""))
				return row
			}
			return 0
		}
	}
}
