package sheet

import (
	"fmt"
)

// Tables in the file (version 6): a sheet's tables are a list in its
// view, each with its name, its range, header row included, its
// columns' names and its style:
//
//	"tables": [{"name":"Sales","range":"A1:C20","columns":["Region","Month","Amount"],"banded":true,"header":true}],
//
// They need a version: a build without tables can't read the formulas
// that read them.
type fileTable struct {
	Name    string   `json:"name"`
	Range   string   `json:"range"`
	Columns []string `json:"columns,omitempty"`
	Banded  bool     `json:"banded,omitempty"`
	Header  bool     `json:"header,omitempty"`
}

// hasTables reports whether any sheet has a table.
func (w *Workbook) hasTables() bool {
	for _, s := range w.sheets {
		if s.HasTables() {
			return true
		}
	}
	return false
}

// encodeTables is the sheet's tables as the file holds them.
func encodeTables(ts []Table) []fileTable {
	out := make([]fileTable, len(ts))
	for i, t := range ts {
		out[i] = fileTable{Name: t.Name, Range: t.Range.String(), Columns: t.Cols, Banded: t.Banded, Header: t.Header}
	}
	return out
}

// readTables restores a sheet's tables from a file, its cells already
// read. Columns the file doesn't name, or names in a number other than
// the range's width, are named from the header row.
func (s *Sheet) readTables(fts []fileTable) error {
	for _, ft := range fts {
		if err := s.wb.checkTableName(ft.Name, ""); err != nil {
			return fmt.Errorf("table %q: %w", ft.Name, err)
		}
		r, ok := ParseRange(ft.Range)
		if !ok || r.To.Row == r.From.Row {
			return fmt.Errorf("table %q: invalid range %q", ft.Name, ft.Range)
		}
		if err := s.checkTableRange(r, ""); err != nil {
			return fmt.Errorf("table %q: %w", ft.Name, err)
		}
		t := Table{Name: ft.Name, Range: r, Cols: columnNames(ft.Columns, nil), Banded: ft.Banded, Header: ft.Header}
		if len(t.Cols) != r.To.Col-r.From.Col+1 {
			t.Cols = columnNames(s.rowTexts(Rect{From: r.From, To: Addr{Col: r.To.Col, Row: r.From.Row}}), nil)
		}
		s.view.tables = append(s.view.tables, t)
	}
	return nil
}
