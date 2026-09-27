package fileio

import (
	"bufio"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Frozen panes and filters in XLSX: written here, read in xlsxviewread.go.
//
// A filter is an <autoFilter> over its range, after the cells, with a
// <filterColumn> for each column with criteria (colId counts from the
// range's first column). Hidden values become the list of values shown
// (<filters>, blank="1" for blanks), as Excel keeps them; a condition
// alone becomes a custom filter where Excel has one (text conditions as
// wildcard patterns), else the values it lets through. The rows the
// filter hides are written hidden, since Excel doesn't filter again on
// opening.

// excelRect writes r as Excel's A1:G4.
func excelRect(r sheet.Rect) string {
	a := func(x sheet.Addr) string { return excelColName(x.Col+1) + strconv.Itoa(x.Row+1) }
	if r.From == r.To {
		return a(r.From)
	}
	return a(r.From) + ":" + a(r.To)
}

// writeSheetView writes the sheet's view: whether it is the tab shown,
// and its frozen panes.
func writeSheetView(bw *bufio.Writer, snap *Snapshot, active bool) {
	bw.WriteString(`<sheetViews><sheetView workbookViewId="0"`)
	if active {
		bw.WriteString(` tabSelected="1"`)
	}
	rows, cols := snap.FrozenRows, snap.FrozenCols
	if rows == 0 && cols == 0 {
		bw.WriteString(`/></sheetViews>`)
		return
	}
	bw.WriteString(`><pane`)
	if cols > 0 {
		fmt.Fprintf(bw, ` xSplit="%d"`, cols)
	}
	if rows > 0 {
		fmt.Fprintf(bw, ` ySplit="%d"`, rows)
	}
	pane := "bottomRight"
	switch {
	case cols == 0:
		pane = "bottomLeft"
	case rows == 0:
		pane = "topRight"
	}
	fmt.Fprintf(bw, ` topLeftCell="%s" activePane="%s" state="frozen"/><selection pane="%s"/></sheetView></sheetViews>`,
		excelColName(cols+1)+strconv.Itoa(rows+1), pane, pane)
}

// writeAutoFilter writes the sheet's filter, if it has one.
func writeAutoFilter(bw *bufio.Writer, snap *Snapshot) {
	f := snap.Filter
	if f == nil {
		return
	}
	fmt.Fprintf(bw, `<autoFilter ref="%s"`, excelRect(f.Range))
	cols := slices.Sorted(maps.Keys(f.Cols))
	if len(cols) == 0 {
		bw.WriteString(`/>`)
		return
	}
	bw.WriteString(`>`)
	for _, c := range cols {
		if c < f.Range.From.Col || c > f.Range.To.Col || f.Cols[c].IsZero() {
			continue
		}
		fmt.Fprintf(bw, `<filterColumn colId="%d">%s</filterColumn>`, c-f.Range.From.Col, filterColumnXML(snap, c, f.Cols[c]))
	}
	bw.WriteString(`</autoFilter>`)
}

// filterColumnXML is the criteria of column c as Excel keeps them.
func filterColumnXML(snap *Snapshot, c int, cr sheet.Criteria) string {
	if len(cr.Hidden) == 0 {
		if x, ok := customFilterXML(cr.Cond); ok {
			return x
		}
	}
	hidden := map[string]bool{}
	for _, h := range cr.Hidden {
		hidden[h] = true
	}
	shown, blanks := filterColumnValues(snap, c)
	var b strings.Builder
	b.WriteString(`<filters`)
	if blanks && !hidden[""] && cr.Cond.Matches(sheet.Value{}, "") {
		b.WriteString(` blank="1"`)
	}
	b.WriteString(`>`)
	for _, t := range slices.Sorted(maps.Keys(shown)) {
		if !hidden[t] && cr.Cond.Matches(shown[t], t) {
			fmt.Fprintf(&b, `<filter val="%s"/>`, escapeXML(t, true))
		}
	}
	b.WriteString(`</filters>`)
	return b.String()
}

// filterColumnValues is the distinct values shown in column c of the
// filter's data rows, with a value for each, and whether any is blank.
func filterColumnValues(snap *Snapshot, c int) (map[string]sheet.Value, bool) {
	r := snap.Filter.Range
	last := min(r.To.Row, snap.Range.To.Row)
	shown := map[string]sheet.Value{}
	blanks := r.To.Row > last
	for row := r.From.Row + 1; row <= last; row++ {
		cell, ok := snap.Cells[sheet.Addr{Col: c, Row: row}]
		t := ""
		if ok {
			t = cell.Text()
		}
		if t == "" {
			blanks = true
			continue
		}
		if _, seen := shown[t]; !seen {
			shown[t] = cell.Value
		}
	}
	return shown, blanks
}

// excelOps are the custom filter operators for 012's comparisons.
var excelOps = map[sheet.CondOp]string{
	sheet.CondGreater: "greaterThan", sheet.CondGreaterEq: "greaterThanOrEqual",
	sheet.CondLess: "lessThan", sheet.CondLessEq: "lessThanOrEqual",
	sheet.CondEqual: "equal", sheet.CondNotEqual: "notEqual",
}

// customFilterXML is a condition as one of Excel's custom filters, or
// false when Excel has none like it.
func customFilterXML(c sheet.Condition) (string, bool) {
	custom := func(op, val string) (string, bool) {
		attr := ""
		if op != "equal" {
			attr = ` operator="` + op + `"`
		}
		return fmt.Sprintf(`<customFilters><customFilter%s val="%s"/></customFilters>`, attr, escapeXML(val, true)), true
	}
	arg := escapeWildcards(c.Arg)
	switch c.Op {
	case sheet.CondEmpty:
		return `<filters blank="1"/>`, true
	case sheet.CondNotEmpty:
		return custom("notEqual", " ")
	case sheet.CondContains:
		return custom("equal", "*"+arg+"*")
	case sheet.CondNotContains:
		return custom("notEqual", "*"+arg+"*")
	case sheet.CondStartsWith:
		return custom("equal", arg+"*")
	case sheet.CondEndsWith:
		return custom("equal", "*"+arg)
	case sheet.CondExactly:
		return custom("equal", arg)
	}
	op, ok := excelOps[c.Op]
	if !ok {
		return "", false
	}
	if n, f, isNum := sheet.ParseValue(strings.TrimSpace(c.Arg)); isNum {
		return custom(op, numInput(toExcelSerial(n, f)))
	}
	return custom(op, arg)
}

// escapeWildcards escapes Excel's wildcards, so text matches as is.
func escapeWildcards(s string) string {
	return strings.NewReplacer("~", "~~", "*", "~*", "?", "~?").Replace(s)
}
