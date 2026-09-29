package fileio

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Tables travel as Excel's tables: a table part per table, related to
// its worksheet, which lists them in <tableParts>. A filter on a table's
// range goes in the table part, as Excel keeps a table's filter, rather
// than on the sheet. A banded table has row stripes, and a styled header
// Excel's default table style; a table with neither has no style.
// Excel needs a table's header cells to hold text, so a number there is
// written as the text it shows.

const (
	tableType   = mlType + "table+xml"
	tableRelID  = "rIdTable"
	tableStyle  = "TableStyleMedium2"
	maxTableCol = 16384 // tableColumns Excel reads, a sheet's width
)

// tableFilter is the index of the table whose range the sheet's filter
// has, or -1.
func tableFilter(snap *Snapshot) int {
	if snap.Filter == nil {
		return -1
	}
	for i, t := range snap.Tables {
		if t.Range == snap.Filter.Range {
			return i
		}
	}
	return -1
}

// headerCells are the cells of the header rows of the snapshot's
// tables.
func headerCells(snap *Snapshot) map[sheet.Addr]bool {
	if len(snap.Tables) == 0 {
		return nil
	}
	out := map[sheet.Addr]bool{}
	for _, t := range snap.Tables {
		for c := t.Range.From.Col; c <= t.Range.To.Col; c++ {
			out[sheet.Addr{Col: c, Row: t.Range.From.Row}] = true
		}
	}
	return out
}

// writeTableParts lists the snapshot's tables in its worksheet, by
// their relationships.
func writeTableParts(bw *bufio.Writer, snap *Snapshot) {
	if len(snap.Tables) == 0 {
		return
	}
	fmt.Fprintf(bw, `<tableParts count="%d">`, len(snap.Tables))
	for i := range snap.Tables {
		fmt.Fprintf(bw, `<tablePart r:id="%s%d"/>`, tableRelID, i+1)
	}
	bw.WriteString(`</tableParts>`)
}

// writeTables writes the table parts of a worksheet's tables, numbered
// on from the tables written before, and returns their relationships
// from the worksheet.
func (w *xlsxWriter) writeTables(zw *zip.Writer, snap *Snapshot) ([]string, error) {
	var rels []string
	onFilter := tableFilter(snap)
	for i, t := range snap.Tables {
		w.tableParts++
		n := strconv.Itoa(w.tableParts)
		var body strings.Builder
		bw := bufio.NewWriter(&body)
		fmt.Fprintf(bw, xmlHead+`<table xmlns="%s" id="%s" name="%s" displayName="%s" ref="%s" totalsRowShown="0">`,
			sheetMain, n, escapeXML(t.Name, true), escapeXML(t.Name, true), excelRect(t.Range))
		if i == onFilter {
			writeAutoFilter(bw, snap)
		}
		fmt.Fprintf(bw, `<tableColumns count="%d">`, len(t.Cols))
		for j, col := range t.Cols {
			fmt.Fprintf(bw, `<tableColumn id="%d" name="%s"/>`, j+1, escapeXML(col, true))
		}
		bw.WriteString(`</tableColumns><tableStyleInfo`)
		if t.Header {
			bw.WriteString(` name="` + tableStyle + `"`)
		}
		fmt.Fprintf(bw, ` showFirstColumn="0" showLastColumn="0" showRowStripes="%d" showColumnStripes="0"/></table>`, b2i(t.Banded))
		if err := bw.Flush(); err != nil {
			return nil, err
		}
		if err := writePart(zw, "xl/tables/table"+n+".xml", body.String()); err != nil {
			return nil, err
		}
		w.tableTypes = append(w.tableTypes, `<Override PartName="/xl/tables/table`+n+`.xml" ContentType="`+tableType+`"/>`)
		rels = append(rels, fmt.Sprintf(`<Relationship Id="%s%d" Type="%s/table" Target="../tables/table%s.xml"/>`, tableRelID, i+1, officeRel, n))
	}
	return rels, nil
}

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

// tableKeys are the keys of the tables the snapshots hold, which
// formulas may read in the file.
func tableKeys(sheets []*Snapshot) map[string]bool {
	out := map[string]bool{}
	for _, sn := range sheets {
		for _, t := range sn.Tables {
			out[strings.ToUpper(t.Name)] = true
		}
	}
	return out
}

// unknownTable reports whether c's formula reads a table the file
// doesn't hold, such as a notebook output's, which Excel would refuse.
func (w *xlsxWriter) unknownTable(c SnapCell) bool {
	for _, name := range c.Tables {
		if !w.tables[strings.ToUpper(name)] {
			return true
		}
	}
	return false
}

// tablesNote says how many tables an import left out, or nothing.
func (bk *xlsxBook) tablesNote() []string {
	if bk.tablesLeft == 0 {
		return nil
	}
	return []string{count(bk.tablesLeft, "table", "tables") + " left out: without a header row, or named as a range or another table is"}
}

// readTables loads the tables of worksheet part, from the table parts
// its relationships name, into s, whose cells are read. A table's
// filter goes in af when the sheet has none of its own. Tables 012
// can't hold (no header row, a name taken) are left out, and counted.
func (bk *xlsxBook) readTables(s *sheet.Sheet, part string, af **xlsxAutoFilter) error {
	rels, err := bk.pkg.rels(part)
	if err != nil {
		return err
	}
	for _, rel := range rels {
		if rel.typ != "table" || !bk.pkg.has(rel.target) {
			continue
		}
		t, tf, err := bk.readTable(rel.target)
		if err != nil {
			return fmt.Errorf("table: %w", err)
		}
		if t == nil || s.LoadTable(*t) != nil {
			bk.tablesLeft++
			continue
		}
		if *af == nil && tf != nil {
			*af = tf
		}
	}
	return nil
}

// readTable reads a table part: its name, range, columns and style,
// and its filter; nil for a table without a header row. A totals row
// is left as cells below the table.
func (bk *xlsxBook) readTable(part string) (*sheet.Table, *xlsxAutoFilter, error) {
	x, err := bk.pkg.open(part)
	if x == nil || err != nil {
		return nil, nil, err
	}
	defer x.close()
	var t sheet.Table
	var af *xlsxAutoFilter
	for {
		tok, err := x.next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, nil, err
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "table":
			if !tableHead(&t, se) {
				return nil, nil, x.skip()
			}
		case "autoFilter":
			if af, err = readAutoFilter(x, se); err != nil {
				return nil, nil, err
			}
		case "tableColumn":
			if len(t.Cols) < maxTableCol {
				t.Cols = append(t.Cols, attrOr(se, "name", ""))
			}
		case "tableStyleInfo":
			t.Header = attrOr(se, "name", "") != ""
			t.Banded = boolAttr(se, "showRowStripes", false)
		}
	}
	if t.Range.To.Row == t.Range.From.Row {
		return nil, nil, nil
	}
	return &t, af, nil
}

// tableHead reads a <table>'s name and range into t, the range less a
// totals row, and reports false for a table 012 can't hold: no header
// row, or no range. LoadTable checks the name.
func tableHead(t *sheet.Table, se xml.StartElement) bool {
	t.Name = attrOr(se, "displayName", attrOr(se, "name", ""))
	r, ok := sheet.ParseRange(strings.ReplaceAll(attrOr(se, "ref", ""), "$", ""))
	if !ok || attrOr(se, "headerRowCount", "1") == "0" {
		return false
	}
	if n, err := strconv.Atoi(attrOr(se, "totalsRowCount", "0")); err == nil && n > 0 {
		r.To.Row = max(r.To.Row-n, r.From.Row)
	}
	t.Range = r
	return true
}
