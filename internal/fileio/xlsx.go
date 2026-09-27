package fileio

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Column widths: Excel measures in characters of its default font, 012
// in terminal columns including one of padding.
const (
	excelDefaultWidth = 8.43
	excelPadding      = 1
)

// importXLSX reads every sheet of a workbook, with its named ranges, and
// returns the sheet that was active in Excel.
func importXLSX(ctx context.Context, name string, opt Options) (*Result, error) {
	prog := opt.Progress
	x, err := excelize.OpenFile(name)
	if err != nil {
		return nil, err
	}
	defer x.Close()
	names := x.GetSheetList()
	if len(names) == 0 {
		return nil, fmt.Errorf("the workbook has no sheets")
	}
	// Rows across all sheets, for the progress bar.
	dims := make([]int, len(names))
	total := 0
	for i, ws := range names {
		if dim, err := x.GetSheetDimension(ws); err == nil {
			if _, to, ok := strings.Cut(dim, ":"); ok {
				if _, r, err := excelize.CellNameToCoordinates(to); err == nil {
					dims[i] = r
				}
			}
		}
		total += dims[i]
	}

	b := newBuilder()
	book := b.s.Book()
	styles := map[int]xlsxStyle{}
	var notes []string
	done := 0
	for i, ws := range names {
		if i == 0 {
			err = book.RenameSheet(b.s, ws)
		} else {
			b.s, err = book.AddSheet(ws, i)
		}
		if err != nil {
			return nil, fmt.Errorf("sheet %s: %w", ws, err)
		}
		rows, err := importXLSXSheet(ctx, x, b, ws, styles, func(row int) {
			prog.setRows(done + row)
			prog.setFrac(int64(done+row), int64(total))
		})
		if err != nil {
			return nil, err
		}
		done += rows
	}
	notes = append(notes, importXLSXNames(x, book)...)
	active := book.Sheet(clamp(x.GetActiveSheetIndex(), 0, book.Len()-1))
	book.SetActive(active)
	b.s = active
	prog.setRows(done)
	s, notes := b.finish(notes)
	return &Result{Sheet: s, Rows: done, Notes: notes}, nil
}

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

// importXLSXSheet reads one sheet into b.s, reporting rows read, and
// returns how many rows it read.
func importXLSXSheet(ctx context.Context, x *excelize.File, b *builder, ws string, styles map[int]xlsxStyle, report func(int)) (int, error) {
	rows, err := x.Rows(ws)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	row := 0
	for ; rows.Next(); row++ {
		if row%128 == 0 {
			if err := ctx.Err(); err != nil {
				return 0, err
			}
			report(row)
		}
		cols, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return 0, err
		}
		for col, raw := range cols {
			a := sheet.Addr{Col: col, Row: row}
			if !b.fits(a) {
				break
			}
			cell, _ := excelize.CoordinatesToCellName(col+1, row+1)
			importXLSXCell(x, b, ws, cell, a, raw, styles)
		}
	}
	if err := rows.Error(); err != nil {
		return 0, err
	}
	for c := range sheet.MaxCols {
		colName, _ := excelize.ColumnNumberToName(c + 1)
		w, err := x.GetColWidth(ws, colName)
		if err != nil || math.Abs(w-excelDefaultWidth) < 0.01 || math.Abs(w-9.140625) < 0.01 {
			continue
		}
		b.s.SetColWidth(c, max(int(math.Round(w))+excelPadding, 1))
	}
	return row, nil
}

// importXLSXNames defines the workbook's named ranges that are a range on
// one sheet, such as Sales = Q3!$B$2:$B$20, and notes the others (sheet
// scoped names, constants, formulas).
func importXLSXNames(x *excelize.File, book *sheet.Workbook) []string {
	skipped := 0
	example := ""
	for _, dn := range x.GetDefinedName() {
		if strings.HasPrefix(dn.Name, "_xlnm.") {
			continue // print areas and the like
		}
		ref := strings.TrimPrefix(dn.RefersTo, "=")
		ws, rest := sheet.SplitSheet(ref)
		r, ok := sheet.ParseRange(rest)
		s := book.Lookup(ws)
		if dn.Scope != "" && dn.Scope != "Workbook" || !ok || s == nil || book.DefineName(dn.Name, s, r) != nil {
			skipped++
			if example == "" {
				example = dn.Name
			}
		}
	}
	if skipped > 0 {
		return []string{fmt.Sprintf("%s left out, e.g. %s", count(skipped, "named range", "named ranges"), example)}
	}
	return nil
}

// xlsxStyle is what 012 keeps of an Excel cell style.
type xlsxStyle struct {
	format sheet.Format
	style  sheet.Style
}

func styleOf(x *excelize.File, id int, cache map[int]xlsxStyle) xlsxStyle {
	if st, ok := cache[id]; ok {
		return st
	}
	var out xlsxStyle
	if st, err := x.GetStyle(id); err == nil && st != nil {
		code := ""
		if st.CustomNumFmt != nil {
			code = *st.CustomNumFmt
		}
		out.format = formatOf(st.NumFmt, code)
		if f := st.Font; f != nil {
			out.style.Bold, out.style.Italic, out.style.Strikethrough = f.Bold, f.Italic, f.Strike
			out.style.Underline = f.Underline != "" && f.Underline != "none"
		}
		if al := st.Alignment; al != nil {
			switch al.Horizontal {
			case "left":
				out.style.Align = sheet.AlignLeft
			case "center", "centerContinuous":
				out.style.Align = sheet.AlignCenter
			case "right":
				out.style.Align = sheet.AlignRight
			}
		}
	}
	cache[id] = out
	return out
}

func importXLSXCell(x *excelize.File, b *builder, ws, cell string, a sheet.Addr, raw string, styles map[int]xlsxStyle) {
	var st xlsxStyle
	if id, err := x.GetCellStyle(ws, cell); err == nil && id != 0 {
		st = styleOf(x, id, styles)
	}
	typ, _ := x.GetCellType(ws, cell)
	keep := func() {
		switch typ {
		case excelize.CellTypeBool:
			b.boolean(a, raw == "1" || strings.EqualFold(raw, "TRUE"), st.style)
			return
		case excelize.CellTypeSharedString, excelize.CellTypeInlineString, excelize.CellTypeFormula, excelize.CellTypeError:
			b.text(a, raw, st.format, st.style)
			return
		}
		if v, err := strconv.ParseFloat(raw, 64); err == nil {
			b.number(a, fromExcelSerial(v, st.format), st.format, st.style)
			return
		}
		b.text(a, raw, st.format, st.style)
	}
	if f, err := x.GetCellFormula(ws, cell); err == nil && f != "" {
		b.formula(a, fromExcelFormula(f), st.format, st.style, keep)
		return
	}
	if raw == "" {
		b.put(a, "", st.format, st.style)
		return
	}
	keep()
}

// exportXLSX writes a workbook: the snapshot's sheets (or just the
// snapshot), each with values, formulas in Excel's syntax with their
// results cached, number formats, text styles, alignment and column
// widths, and the named ranges.
func exportXLSX(_ context.Context, name string, snap *Snapshot, _ ExportOptions) (*ExportResult, error) {
	x := excelize.NewFile()
	defer x.Close()
	sheets := snap.Sheets
	if len(sheets) == 0 {
		sheets = []*Snapshot{snap}
	}
	w := &xlsxWriter{x: x, styleIDs: map[xlsxStyle]int{}, multi: len(sheets) > 1}
	res := &ExportResult{}
	for i, sn := range sheets {
		ws := sheetName(sn.Name)
		var err error
		if i == 0 {
			err = x.SetSheetName("Sheet1", ws)
		} else {
			_, err = x.NewSheet(ws)
		}
		if err != nil {
			return nil, err
		}
		rows, err := w.sheet(ws, sn)
		if err != nil {
			return nil, err
		}
		res.Rows += rows
		if sn == snap || snap.Sheets == nil {
			x.SetActiveSheet(i)
		}
	}
	for _, n := range snap.Names {
		if err := x.SetDefinedName(&excelize.DefinedName{Name: n[0], RefersTo: n[1]}); err != nil {
			return nil, err
		}
	}
	yes := true
	if err := x.SetCalcProps(&excelize.CalcPropsOptions{FullCalcOnLoad: &yes}); err != nil {
		return nil, err
	}
	if w.values > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%s with no Excel equivalent saved as values, e.g. %s",
			count(w.values, "formula", "formulas"), w.example))
	}
	return res, writeFile(name, func(w io.Writer) error { return x.Write(w) })
}

// xlsxWriter writes sheets into a workbook, sharing cell styles.
type xlsxWriter struct {
	x        *excelize.File
	styleIDs map[xlsxStyle]int
	values   int    // formulas written as values
	example  string // the first of them
	multi    bool   // examples name their sheet
}

func (w *xlsxWriter) styleFor(f sheet.Format, st sheet.Style) (int, error) {
	key := xlsxStyle{f, st}
	if id, ok := w.styleIDs[key]; ok {
		return id, nil
	}
	xs := &excelize.Style{}
	if code := excelCode(f); code != "" {
		xs.CustomNumFmt = &code
	}
	if st.Bold || st.Italic || st.Underline || st.Strikethrough {
		xs.Font = &excelize.Font{Bold: st.Bold, Italic: st.Italic, Strike: st.Strikethrough}
		if st.Underline {
			xs.Font.Underline = "single"
		}
	}
	if st.Align != sheet.AlignAuto {
		xs.Alignment = &excelize.Alignment{Horizontal: st.Align.String()}
	}
	id, err := w.x.NewStyle(xs)
	w.styleIDs[key] = id
	return id, err
}

// sheet writes a snapshot to sheet ws and returns the rows written.
func (w *xlsxWriter) sheet(ws string, snap *Snapshot) (int, error) {
	sw, err := w.x.NewStreamWriter(ws)
	if err != nil {
		return 0, err
	}
	for c, width := range snap.Widths {
		if err := sw.SetColWidth(c+1, c+1, float64(max(width-excelPadding, 1))); err != nil {
			return 0, err
		}
	}
	r := snap.Range
	line := make([]any, r.To.Col-r.From.Col+1)
	for row := r.From.Row; row <= r.To.Row; row++ {
		for col := r.From.Col; col <= r.To.Col; col++ {
			a := sheet.Addr{Col: col, Row: row}
			line[col-r.From.Col] = nil
			if c, ok := snap.Cells[a]; ok {
				if line[col-r.From.Col], err = w.cell(ws, a, c); err != nil {
					return 0, err
				}
			}
		}
		cell, _ := excelize.CoordinatesToCellName(r.From.Col+1, row+1)
		if err := sw.SetRow(cell, line); err != nil {
			return 0, err
		}
	}
	if err := sw.Flush(); err != nil {
		return 0, err
	}
	return r.To.Row - r.From.Row + 1, nil
}

// cell is the snapshot cell c at a as excelize writes it. A formula with
// no Excel equivalent is written as its value, and counted.
func (w *xlsxWriter) cell(ws string, a sheet.Addr, c SnapCell) (excelize.Cell, error) {
	// A formula's inferred format is written too, so its result shows
	// the same in Excel.
	id := 0
	if !c.Format.IsZero() || !c.Style.IsZero() {
		var err error
		if id, err = w.styleFor(c.Format, c.Style); err != nil {
			return excelize.Cell{}, err
		}
	}
	xc := excelize.Cell{StyleID: id, Value: xlsxValue(c.Value, c.Format)}
	if !c.Formula {
		return xc, nil
	}
	if fx, ok := toExcelFormula(c.Input); ok {
		xc.Formula = fx
		return xc, nil
	}
	w.values++
	if w.example == "" {
		w.example = a.String()
		if w.multi {
			w.example = sheet.QuoteSheet(ws) + "!" + a.String()
		}
	}
	return xc, nil
}

// xlsxValue is a computed value as excelize writes it.
func xlsxValue(v sheet.Value, f sheet.Format) any {
	switch v.Kind {
	case sheet.Number:
		return toExcelSerial(v.Num, f)
	case sheet.Bool:
		return v.Num != 0
	case sheet.Text, sheet.Error:
		return v.Str
	}
	return nil
}

// sheetName makes a valid Excel sheet name: at most 31 characters, none
// of : \ / ? * [ ].
func sheetName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`:\/?*[]`, r) {
			return '_'
		}
		return r
	}, s)
	s = strings.Trim(strings.TrimSpace(s), "'")
	if r := []rune(s); len(r) > 31 {
		s = string(r[:31])
	}
	if s == "" {
		return "Sheet1"
	}
	return s
}
