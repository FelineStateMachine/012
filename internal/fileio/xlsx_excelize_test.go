package fileio

// The XLSX importer 012 used before its own reader, on excelize, kept
// as the reference for TestXLSXDifferential.

import (
	"context"
	"fmt"
	"math"
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
		rows, err := excelizeSheet(ctx, x, b, ws, styles, func(row int) {
			prog.setRows(done + row)
			prog.setFrac(int64(done+row), int64(total))
		})
		if err != nil {
			return nil, err
		}
		done += rows
	}
	notes = append(notes, excelizeNames(x, book)...)
	active := book.Sheet(clamp(x.GetActiveSheetIndex(), 0, book.Len()-1))
	book.SetActive(active)
	b.s = active
	prog.setRows(done)
	s, notes := b.finish(notes)
	return &Result{Sheet: s, Rows: done, Notes: notes}, nil
}

// excelizeSheet reads one sheet into b.s, reporting rows read, and
// returns how many rows it read.
func excelizeSheet(ctx context.Context, x *excelize.File, b *builder, ws string, styles map[int]xlsxStyle, report func(int)) (int, error) {
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
			excelizeCell(x, b, ws, cell, a, raw, styles)
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
