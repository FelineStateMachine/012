package fileio

// The XLSX importer 012 used before its own reader, on excelize, kept
// as the reference for TestXLSXDifferential.

import (
	"archive/zip"
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
		excelizePanes(x, ws, next)
		done += rows
	}
	filters, err := referenceAutoFilters(name)
	if err != nil {
		return nil, err
	}
	notes = append(notes, excelizeNames(x, book)...)
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

// excelizePanes freezes s as sheet ws is frozen.
func excelizePanes(x *excelize.File, ws string, s *sheet.Sheet) {
	if p, err := x.GetPanes(ws); err == nil && p.Freeze {
		s.LoadFrozen(p.YSplit, p.XSplit)
	}
}

// refWorksheet is the part of a worksheet referenceAutoFilters reads.
type refWorksheet struct {
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

// referenceAutoFilters reads each sheet's autoFilter by decoding the
// whole worksheet with encoding/xml, as excelize has no API for reading
// one: the reference for the streaming reader's. Only the parts' names
// come from 012's reader.
func referenceAutoFilters(name string) ([]*xlsxAutoFilter, error) {
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
	zr, err := zip.NewReader(f, st.Size())
	if err != nil {
		return nil, err
	}
	out := make([]*xlsxAutoFilter, len(bk.sheets))
	for i, info := range bk.sheets {
		if info.part == "" || info.kind != "worksheet" {
			continue
		}
		rc, err := zr.Open(info.part)
		if err != nil {
			return nil, err
		}
		var ws refWorksheet
		err = xml.NewDecoder(rc).Decode(&ws)
		rc.Close()
		if err != nil {
			return nil, err
		}
		out[i] = ws.autoFilter()
	}
	return out, nil
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
	return row, nil
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
