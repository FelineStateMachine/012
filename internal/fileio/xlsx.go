package fileio

import (
	"context"
	"fmt"
	"io"
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

func clamp(v, lo, hi int) int { return max(lo, min(v, hi)) }

// xlsxStyle is what 012 keeps of an Excel cell style.
type xlsxStyle struct {
	format sheet.Format
	style  sheet.Style
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
