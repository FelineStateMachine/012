package fileio

import (
	"context"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"

	"012/internal/sheet"
)

// Column widths: Excel measures in characters of its default font, 012
// in terminal columns including one of padding.
const (
	excelDefaultWidth = 8.43
	excelPadding      = 1
)

func importXLSX(ctx context.Context, name string, prog *Progress) (*Result, error) {
	x, err := excelize.OpenFile(name)
	if err != nil {
		return nil, err
	}
	defer x.Close()
	sheets := x.GetSheetList()
	if len(sheets) == 0 {
		return nil, fmt.Errorf("the workbook has no sheets")
	}
	ws := sheets[0]
	var notes []string
	if n := len(sheets) - 1; n > 0 {
		notes = append(notes, fmt.Sprintf("imported the first sheet, %s; %s not imported (%s)",
			ws, count(n, "other sheet", "other sheets"), strings.Join(sheets[1:], ", ")))
	}

	total := 0
	if dim, err := x.GetSheetDimension(ws); err == nil {
		if _, to, ok := strings.Cut(dim, ":"); ok {
			if _, r, err := excelize.CellNameToCoordinates(to); err == nil {
				total = r
			}
		}
	}

	b := newBuilder()
	styles := map[int]xlsxStyle{}
	rows, err := x.Rows(ws)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	row := 0
	for ; rows.Next(); row++ {
		if row%128 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			prog.setRows(row)
			prog.setFrac(int64(row), int64(total))
		}
		cols, err := rows.Columns(excelize.Options{RawCellValue: true})
		if err != nil {
			return nil, err
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
		return nil, err
	}
	prog.setRows(row)

	for c := range sheet.MaxCols {
		colName, _ := excelize.ColumnNumberToName(c + 1)
		w, err := x.GetColWidth(ws, colName)
		if err != nil || math.Abs(w-excelDefaultWidth) < 0.01 || math.Abs(w-9.140625) < 0.01 {
			continue
		}
		b.s.SetColWidth(c, max(int(math.Round(w))+excelPadding, 1))
	}
	s, notes := b.finish(notes)
	return &Result{Sheet: s, Rows: row, Notes: notes}, nil
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

// exportXLSX writes a workbook with one sheet: values, formulas in
// Excel's syntax with their results cached, number formats, text styles,
// alignment and column widths.
func exportXLSX(name string, snap *Snapshot) (*ExportResult, error) {
	x := excelize.NewFile()
	defer x.Close()
	ws := sheetName(snap.Name)
	if err := x.SetSheetName("Sheet1", ws); err != nil {
		return nil, err
	}
	sw, err := x.NewStreamWriter(ws)
	if err != nil {
		return nil, err
	}
	for c, w := range snap.Widths {
		if err := sw.SetColWidth(c+1, c+1, float64(max(w-excelPadding, 1))); err != nil {
			return nil, err
		}
	}
	styleIDs := map[xlsxStyle]int{}
	styleFor := func(f sheet.Format, st sheet.Style) (int, error) {
		key := xlsxStyle{f, st}
		if id, ok := styleIDs[key]; ok {
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
		id, err := x.NewStyle(xs)
		styleIDs[key] = id
		return id, err
	}

	r := snap.Range
	values := 0
	example := ""
	for row := r.From.Row; row <= r.To.Row; row++ {
		var line []any
		for col := r.From.Col; col <= r.To.Col; col++ {
			a := sheet.Addr{Col: col, Row: row}
			c, ok := snap.Cells[a]
			if !ok {
				line = append(line, nil)
				continue
			}
			// A formula's inferred format is written too, so its result
			// shows the same in Excel.
			f := c.Format
			id := 0
			if !f.IsZero() || !c.Style.IsZero() {
				if id, err = styleFor(f, c.Style); err != nil {
					return nil, err
				}
			}
			xc := excelize.Cell{StyleID: id, Value: xlsxValue(c.Value, f)}
			if c.Formula {
				if fx, ok := toExcelFormula(c.Input); ok {
					xc.Formula = fx
				} else {
					values++
					if example == "" {
						example = a.String()
					}
				}
			}
			line = append(line, xc)
		}
		cell, _ := excelize.CoordinatesToCellName(r.From.Col+1, row+1)
		if err := sw.SetRow(cell, line); err != nil {
			return nil, err
		}
	}
	if err := sw.Flush(); err != nil {
		return nil, err
	}
	yes := true
	if err := x.SetCalcProps(&excelize.CalcPropsOptions{FullCalcOnLoad: &yes}); err != nil {
		return nil, err
	}
	buf, err := x.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	res := &ExportResult{Rows: r.To.Row - r.From.Row + 1}
	if values > 0 {
		res.Notes = append(res.Notes, fmt.Sprintf("%s with no Excel equivalent saved as values, e.g. %s",
			count(values, "formula", "formulas"), example))
	}
	return res, writeFile(name, buf.Bytes())
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
