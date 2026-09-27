package fileio

// The XLSX importer 012 used before its own reader, on excelize, kept
// as the reference for TestXLSXDifferential.

import (
	"archive/zip"
	"cmp"
	"context"
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// importXLSXExcelize reads every sheet of a workbook, with its named ranges, and
// returns the sheet that was active in Excel.
func importXLSXExcelize(ctx context.Context, name string, opt Options) (*Result, error) {
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

	b := newBuilder(ctx, opt.MaxCells)
	book := b.s.Book()
	styles := map[int]xlsxStyle{}
	var notes []string
	done := 0
	for i, ws := range names {
		next := b.s
		if i == 0 {
			err = book.RenameSheet(b.s, ws)
		} else {
			next, err = book.AddSheet(ws, i)
		}
		if err != nil {
			return nil, fmt.Errorf("sheet %s: %w", ws, err)
		}
		b.nextSheet(next)
		rows, err := excelizeSheet(ctx, x, b, ws, styles, func(row int) {
			prog.setRows(done + row)
			prog.setFrac(int64(done+row), int64(total))
		})
		if err != nil {
			return nil, err
		}
		done += rows
	}
	filters, protected, heights, err := referenceAutoFilters(name)
	if err != nil {
		return nil, err
	}
	for i, hs := range heights {
		for row, h := range hs {
			book.Sheet(i).LoadRowHeight(row, h)
		}
	}
	notes = append(notes, excelizeNames(x, book)...)
	notes = append(notes, (&xlsxBook{protected: protected}).protectionNote()...)
	active := book.Sheet(clamp(x.GetActiveSheetIndex(), 0, book.Len()-1))
	book.SetActive(active)
	for i, ws := range names {
		if shown, err := x.GetSheetVisible(ws); err == nil && !shown && book.Sheet(i) != active {
			book.HideSheet(book.Sheet(i)) // hidden in Excel, hidden here
		}
	}
	b.s = active
	prog.setRows(done)
	s, notes := b.finish(notes)
	return &Result{Sheet: s, Rows: done, Notes: append(notes, applyFilters(book, filters)...)}, nil
}

// excelizeView freezes s as sheet ws is frozen and loads its notes.
func excelizeView(x *excelize.File, ws string, s *sheet.Sheet) error {
	if p, err := x.GetPanes(ws); err == nil && p.Freeze {
		s.LoadFrozen(p.YSplit, p.XSplit)
	}
	return excelizeNotes(x, ws, s)
}

// excelizeNotes loads sheet ws's notes (Excel's legacy comments) into s.
func excelizeNotes(x *excelize.File, ws string, s *sheet.Sheet) error {
	comments, err := x.GetComments(ws)
	if err != nil {
		return err
	}
	for _, c := range comments {
		text := c.Text
		for _, run := range c.Paragraph {
			text += run.Text
		}
		if a, ok := sheet.ParseAddr(c.Cell); ok {
			s.LoadNote(a, text)
		}
	}
	return nil
}

// refWorksheet is the part of a worksheet referenceAutoFilters reads.
type refWorksheet struct {
	Rows []struct {
		R      int    `xml:"r,attr"`
		Ht     string `xml:"ht,attr"`
		Custom string `xml:"customHeight,attr"`
	} `xml:"sheetData>row"`
	Protection *struct {
		Sheet string `xml:"sheet,attr"`
	} `xml:"sheetProtection"`
	AutoFilter *struct {
		Ref  string `xml:"ref,attr"`
		Cols []struct {
			ColID   int `xml:"colId,attr"`
			Filters *struct {
				Blank  string `xml:"blank,attr"`
				Filter []struct {
					Val string `xml:"val,attr"`
				} `xml:"filter"`
				Dates []struct{} `xml:"dateGroupItem"`
			} `xml:"filters"`
			Custom *struct {
				Filter []struct {
					Op  string `xml:"operator,attr"`
					Val string `xml:"val,attr"`
				} `xml:"customFilter"`
			} `xml:"customFilters"`
			Top10   *struct{} `xml:"top10"`
			Dynamic *struct{} `xml:"dynamicFilter"`
			Color   *struct{} `xml:"colorFilter"`
			Icon    *struct{} `xml:"iconFilter"`
		} `xml:"filterColumn"`
	} `xml:"autoFilter"`
}

// referenceAutoFilters reads each sheet's autoFilter, the names of the
// protected sheets and the heights of rows set by hand (excelize doesn't
// say which those are), by decoding the whole worksheet with
// encoding/xml, as excelize has no API for reading them: the reference
// for the streaming reader's. Only the parts' names come from 012's
// reader.
func referenceAutoFilters(name string) ([]*xlsxAutoFilter, []string, []map[int]int, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, nil, nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, nil, nil, err
	}
	bk, err := openXLSX(f, st.Size(), defaultXLSXLimits)
	if err != nil {
		return nil, nil, nil, err
	}
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return nil, nil, nil, err
	}
	out := make([]*xlsxAutoFilter, len(bk.sheets))
	heights := make([]map[int]int, len(bk.sheets))
	var protected []string
	for i, info := range bk.sheets {
		if info.part == "" || info.kind != "worksheet" {
			continue
		}
		rc, err := zr.Open(info.part)
		if err != nil {
			return nil, nil, nil, err
		}
		var ws refWorksheet
		err = xml.NewDecoder(rc).Decode(&ws)
		rc.Close()
		if err != nil {
			return nil, nil, nil, err
		}
		out[i] = ws.autoFilter()
		if p := ws.Protection; p != nil && (p.Sheet == "1" || p.Sheet == "true") {
			protected = append(protected, info.name)
		}
		heights[i] = ws.heights()
	}
	return out, protected, heights, nil
}

// heights are the rows' heights set by hand, in lines, by row from 0.
func (ws refWorksheet) heights() map[int]int {
	out := map[int]int{}
	for _, r := range ws.Rows {
		se := xml.StartElement{Attr: []xml.Attr{{Name: xml.Name{Local: "ht"}, Value: r.Ht}, {Name: xml.Name{Local: "customHeight"}, Value: r.Custom}}}
		if r.Custom == "" {
			se.Attr = se.Attr[:1]
		}
		if h := rowHeight(se); h > 0 && r.R >= 1 {
			out[r.R-1] = h
		}
	}
	return out
}

func (ws refWorksheet) autoFilter() *xlsxAutoFilter {
	a := ws.AutoFilter
	if a == nil {
		return nil
	}
	af := &xlsxAutoFilter{ref: a.Ref}
	for _, c := range a.Cols {
		fc := xlsxFilterColumn{col: c.ColID}
		if fs := c.Filters; fs != nil {
			fc.filters, fc.blank, fc.dates = true, fs.Blank == "1" || fs.Blank == "true", len(fs.Dates) > 0
			for _, v := range fs.Filter {
				fc.values = append(fc.values, v.Val)
			}
		}
		if c.Custom != nil {
			for _, cf := range c.Custom.Filter {
				op := cf.Op
				if op == "" {
					op = "equal"
				}
				fc.custom = append(fc.custom, xlsxCustomFilter{op: op, val: cf.Val})
			}
		}
		for _, o := range []struct {
			set  bool
			what string
		}{{c.Top10 != nil, "top 10"}, {c.Dynamic != nil, "a dynamic filter"}, {c.Color != nil, "by color"}, {c.Icon != nil, "by icon"}} {
			if o.set {
				fc.other = o.what
			}
		}
		af.cols = append(af.cols, fc)
	}
	return af
}

// excelizeSheet reads one sheet into b.s, reporting rows read, and
// returns how many rows it read.
func excelizeSheet(ctx context.Context, x *excelize.File, b *builder, ws string, styles map[int]xlsxStyle, report func(int)) (int, error) {
	rows, err := x.Rows(ws)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	dimCols := 0
	if dim, err := x.GetSheetDimension(ws); err == nil {
		_, to, _ := strings.Cut(dim, ":")
		dimCols, _, _ = excelize.CellNameToCoordinates(cmp.Or(to, dim))
	}
	row, width := 0, 0
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
		if id := rows.GetRowOpts().StyleID; id != 0 {
			loadRowStyle(b, row, excelizeStyleOf(x, id, styles))
		}
		width = max(width, len(cols))
		for col, raw := range cols {
			a := sheet.Addr{Col: col, Row: row}
			if !b.fits(a) {
				break
			}
			cell, _ := excelize.CoordinatesToCellName(col+1, row+1)
			excelizeCell(x, b, ws, cell, a, raw, styles)
		}
		excelizeBordered(x, b, ws, row, len(cols), dimCols, styles)
	}
	if err := rows.Error(); err != nil {
		return 0, err
	}
	loadColStyles(b, width, func(c int) (xlsxStyle, bool) {
		colName, _ := excelize.ColumnNumberToName(c + 1)
		if id, err := x.GetColStyle(ws, colName); err == nil && id != 0 {
			st := excelizeStyleOf(x, id, styles)
			return st, st != (xlsxStyle{})
		}
		return xlsxStyle{}, false
	})
	for c := range min(max(width, 256), sheet.MaxCols) {
		colName, _ := excelize.ColumnNumberToName(c + 1)
		w, err := x.GetColWidth(ws, colName)
		if err != nil || math.Abs(w-excelDefaultWidth) < 0.01 || math.Abs(w-9.140625) < 0.01 {
			continue
		}
		b.s.LoadColWidth(c, max(int(math.Round(w))+excelPadding, 1))
	}
	if merges, err := x.GetMergeCells(ws, true); err == nil {
		for _, mc := range merges {
			if r, ok := sheet.ParseRange(mc.GetStartAxis() + ":" + mc.GetEndAxis()); ok {
				b.s.LoadMerge(r)
			}
		}
	}
	return row, excelizeView(x, ws, b.s)
}

// excelizeNames defines the workbook's named ranges that are a range on
// one sheet, such as Sales = Q3!$B$2:$B$20, and notes the others (sheet
// scoped names, constants, formulas).
func excelizeNames(x *excelize.File, book *sheet.Workbook) []string {
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

// excelizeBorderStyles are Excel's border styles by excelize's index.
var excelizeBorderStyles = []string{"none", "thin", "medium", "dashed", "dotted", "thick", "double", "hair",
	"mediumDashed", "dashDot", "mediumDashDot", "dashDotDot", "mediumDashDotDot", "slantDashDot"}

func excelizeStyleOf(x *excelize.File, id int, cache map[int]xlsxStyle) xlsxStyle {
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
		for _, br := range st.Border {
			l := sheet.LineNone
			if br.Style > 0 && br.Style < len(excelizeBorderStyles) {
				l = borderLine(excelizeBorderStyles[br.Style])
			}
			switch br.Type {
			case "left":
				out.style.Borders.Left = l
			case "right":
				out.style.Borders.Right = l
			case "top":
				out.style.Borders.Top = l
			case "bottom":
				out.style.Borders.Bottom = l
			}
		}
		if al := st.Alignment; al != nil {
			if al.WrapText {
				out.style.Wrap = sheet.WrapOn
			}
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

// excelizeBordered stores the blank cells of row from column from (from
// 0) up to the last one within dimCols that draws borders, which 012's
// reader keeps though no value follows them.
func excelizeBordered(x *excelize.File, b *builder, ws string, row, from, dimCols int, styles map[int]xlsxStyle) {
	last := -1
	for col := from; col < min(dimCols, sheet.MaxCols); col++ {
		cell, _ := excelize.CoordinatesToCellName(col+1, row+1)
		if id, err := x.GetCellStyle(ws, cell); err == nil && id != 0 && !excelizeStyleOf(x, id, styles).style.Borders.IsZero() {
			last = col
		}
	}
	for col := from; col <= last; col++ {
		cell, _ := excelize.CoordinatesToCellName(col+1, row+1)
		excelizeCell(x, b, ws, cell, sheet.Addr{Col: col, Row: row}, "", styles)
	}
}

func excelizeCell(x *excelize.File, b *builder, ws, cell string, a sheet.Addr, raw string, styles map[int]xlsxStyle) {
	var st xlsxStyle
	if id, err := x.GetCellStyle(ws, cell); err == nil && id != 0 {
		st = excelizeStyleOf(x, id, styles)
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
