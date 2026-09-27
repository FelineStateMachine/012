package fileio

import (
	"context"
	"fmt"
	"math"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// importXLSX reads every sheet of a workbook, with its named
// ranges, and returns the sheet that was active in Excel. Worksheets are
// streamed a row at a time (see xlsxpkg.go).
func importXLSX(ctx context.Context, name string, opt Options) (*Result, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	bk, err := openXLSX(f, st.Size(), defaultXLSXLimits)
	if err != nil {
		return nil, err
	}
	return bk.importBook(ctx, opt)
}

func (bk *xlsxBook) importBook(ctx context.Context, opt Options) (*Result, error) {
	prog := opt.Progress
	if len(bk.sheets) == 0 {
		return nil, fmt.Errorf("the workbook has no sheets")
	}
	// Rows across all sheets, for the progress bar.
	total := 0
	for i := range bk.sheets {
		total += bk.dimension(i)
	}
	b := newBuilder(opt.MaxCells)
	book := b.s.Book()
	done := 0
	var err error
	for i, info := range bk.sheets {
		next := b.s
		if i == 0 {
			err = book.RenameSheet(b.s, info.name)
		} else {
			next, err = book.AddSheet(info.name, i)
		}
		if err != nil {
			return nil, fmt.Errorf("sheet %s: %w", info.name, err)
		}
		b.nextSheet(next)
		rows, err := bk.importSheet(ctx, b, i, func(row int) {
			prog.setRows(done + row)
			prog.setFrac(int64(done+row), int64(total))
		})
		if err != nil {
			return nil, err
		}
		done += rows
	}
	notes := bk.importNames(book)
	active := book.Sheet(clamp(bk.active, 0, book.Len()-1))
	book.SetActive(active)
	for i, info := range bk.sheets {
		if info.hidden && book.Sheet(i) != active {
			book.HideSheet(book.Sheet(i)) // hidden in Excel, hidden here
		}
	}
	b.s = active
	prog.setRows(done)
	s, notes := b.finish(notes)
	return &Result{Sheet: s, Rows: done, Notes: notes}, nil
}

// importSheet reads sheet i into b.s, reporting rows read, and returns
// the number of its last row.
func (bk *xlsxBook) importSheet(ctx context.Context, b *builder, i int, report func(int)) (int, error) {
	r, err := bk.openSheet(i)
	if err != nil {
		return 0, err
	}
	defer r.close()
	last, width := 0, 0
	for n := 0; ; n++ {
		ok, err := r.next()
		if err != nil {
			return 0, fmt.Errorf("sheet %s: %w", bk.sheets[i].name, err)
		}
		if !ok {
			break
		}
		if n%128 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			report(r.row.num)
		}
		if err := bk.importRow(b, r); err != nil {
			return 0, fmt.Errorf("sheet %s: %w", bk.sheets[i].name, err)
		}
		last = max(last, r.row.num)
		if n := len(r.row.cells); n > 0 {
			width = max(width, r.row.cells[n-1].col)
		}
	}
	loadColStyles(b, width, func(c int) (xlsxStyle, bool) {
		st := bk.styles.style(r.lineStyle(c + 1))
		return st, st != (xlsxStyle{})
	})
	for c := range min(max(width, 256), sheet.MaxCols) { // the data's columns, and 1-2-3's
		w := r.colWidth(c + 1)
		if math.Abs(w-excelDefaultWidth) < 0.01 || math.Abs(w-9.140625) < 0.01 || !(w > 0 && w < 1000) {
			continue
		}
		b.s.LoadColWidth(c, max(int(math.Round(w))+excelPadding, 1))
	}
	return last, nil
}

// importRow stores a row's cells. As Excel's cells are read, a cell
// with neither a value nor a formula only counts for its format, and
// only up to the row's last cell with one: formatted blanks after it are
// left out.
func (bk *xlsxBook) importRow(b *builder, r *xlsxSheetReader) error {
	row := &r.row
	if row.style != 0 {
		loadRowStyle(b, row.num-1, bk.styles.style(row.style))
	}
	cells := row.cells
	if !slices.IsSortedFunc(cells, func(a, b xlsxCell) int { return a.col - b.col }) {
		slices.SortStableFunc(cells, func(a, b xlsxCell) int { return a.col - b.col })
	}
	lastCol := 0
	for i := range cells {
		if cells[i].kept() {
			lastCol = cells[i].col
		}
	}
	if lastCol == 0 || !b.fits(sheet.Addr{Col: lastCol - 1, Row: row.num - 1}) && row.num > sheet.MaxRows {
		return nil
	}
	j := 0
	for col := 1; col <= min(lastCol, sheet.MaxCols); col++ {
		a := sheet.Addr{Col: col - 1, Row: row.num - 1}
		for j < len(cells) && cells[j].col < col {
			j++
		}
		var c *xlsxCell
		id := 0
		if j < len(cells) && cells[j].col == col {
			c = &cells[j]
			id = c.style
		}
		if id == 0 {
			id = row.style
		}
		if id == 0 {
			id = r.colStyle(col)
		}
		st := bk.styles.style(id)
		if c == nil || !c.kept() {
			b.put(a, "", st.format, st.style)
			continue
		}
		f, err := r.formula(c)
		if err != nil {
			return err
		}
		bk.importCell(b, a, c, f, st)
	}
	return nil
}

func (bk *xlsxBook) importCell(b *builder, a sheet.Addr, c *xlsxCell, formula string, st xlsxStyle) {
	if formula != "" {
		b.formula(a, fromExcelFormula(formula), st.format, st.style, func() { bk.keep(b, a, c, st) })
		return
	}
	if c.value == "" {
		b.put(a, "", st.format, st.style)
		return
	}
	bk.keep(b, a, c, st)
}

// keep stores a cell's value: its type says whether it is a boolean or
// text; anything else that reads as a number is one.
func (bk *xlsxBook) keep(b *builder, a sheet.Addr, c *xlsxCell, st xlsxStyle) {
	raw := c.value
	switch c.typ {
	case "b":
		b.boolean(a, raw == "1" || strings.EqualFold(raw, "TRUE"), st.style)
		return
	case "s", "inlineStr", "str", "e":
		b.text(a, raw, st.format, st.style)
		return
	case "d":
		if v, f, ok := isoDate(raw); ok {
			if st.format.IsZero() {
				st.format = f
			}
			b.number(a, v, st.format, st.style)
			return
		}
	}
	if v, err := strconv.ParseFloat(raw, 64); err == nil {
		b.number(a, bk.serial(v, st.format), st.format, st.style)
		return
	}
	b.text(a, raw, st.format, st.style)
}

// serial converts an Excel serial number to 012's: in the 1904 date
// system, day 0 is January 1, 1904.
func (bk *xlsxBook) serial(v float64, f sheet.Format) float64 {
	if bk.date1904 && isDateFormat(f) {
		return v + date1904Offset
	}
	return fromExcelSerial(v, f)
}

// date1904Offset is the serial of January 1, 1904 in 012's (and the 1900
// system's) count.
const date1904Offset = 1462

// isoDate reads a t="d" cell's ISO 8601 date, time or both as 012's
// serial number, with the format that shows it.
func isoDate(s string) (float64, sheet.Format, bool) {
	s = strings.TrimSpace(s)
	for _, l := range []struct {
		layout string
		kind   sheet.FormatKind
	}{
		{"2006-01-02T15:04:05.999999999Z07:00", sheet.FmtDateTime},
		{"2006-01-02T15:04:05.999999999", sheet.FmtDateTime},
		{"2006-01-02", sheet.FmtDate},
		{"T15:04:05.999999999", sheet.FmtTime},
		{"15:04:05.999999999", sheet.FmtTime},
	} {
		t, err := time.Parse(l.layout, s)
		if err != nil {
			continue
		}
		wall := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
		base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
		if l.kind == sheet.FmtTime {
			base = time.Date(0, 1, 1, 0, 0, 0, 0, time.UTC)
		}
		return wall.Sub(base).Hours() / 24, sheet.Format{Kind: l.kind}, true
	}
	return 0, sheet.Format{}, false
}

// importNames defines the workbook's named ranges that are a range on
// one sheet, such as Sales = Q3!$B$2:$B$20, and notes the others (sheet
// scoped names, constants, formulas).
func (bk *xlsxBook) importNames(book *sheet.Workbook) []string {
	skipped := 0
	example := ""
	for _, dn := range bk.names {
		if strings.HasPrefix(dn.name, "_xlnm.") {
			continue // print areas and the like
		}
		ws, rest := sheet.SplitSheet(strings.TrimPrefix(dn.refersTo, "="))
		r, ok := sheet.ParseRange(rest)
		s := book.Lookup(ws)
		if bk.scoped(dn) || !ok || s == nil || book.DefineName(dn.name, s, r) != nil {
			skipped++
			if example == "" {
				example = dn.name
			}
		}
	}
	if skipped > 0 {
		return []string{fmt.Sprintf("%s left out, e.g. %s", count(skipped, "named range", "named ranges"), example)}
	}
	return nil
}

// scoped reports whether a name belongs to one sheet. As excelize has
// it, a name scoped to a sheet that doesn't exist, or to one called
// Workbook, is the workbook's.
func (bk *xlsxBook) scoped(dn xlsxName) bool {
	return dn.sheet >= 0 && dn.sheet < len(bk.sheets) && bk.sheets[dn.sheet].name != "Workbook"
}
