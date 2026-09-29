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
// read.
func (s *Sheet) readTables(fts []fileTable) error {
	for _, ft := range fts {
		r, ok := ParseRange(ft.Range)
		if !ok {
			return fmt.Errorf("table %q: invalid range %q", ft.Name, ft.Range)
		}
		if err := s.LoadTable(Table{Name: ft.Name, Range: r, Cols: ft.Columns, Banded: ft.Banded, Header: ft.Header}); err != nil {
			return fmt.Errorf("table %q: %w", ft.Name, err)
		}
	}
	return nil
}

// LoadTable adds t to the sheet as a loader does, without recording
// undo, its cells loaded. Columns t doesn't name, or names in a number
// other than its range's width, are named from its header row, and the
// names are made unique.
func (s *Sheet) LoadTable(t Table) error {
	if err := s.wb.checkTableName(t.Name, ""); err != nil {
		return err
	}
	if t.Range.To.Row == t.Range.From.Row {
		return fmt.Errorf("the range %s has no row below its header", t.Range)
	}
	if err := s.checkTableRange(t.Range, ""); err != nil {
		return err
	}
	t.Cols = columnNames(t.Cols, nil)
	if len(t.Cols) != t.Range.To.Col-t.Range.From.Col+1 {
		t.Cols = columnNames(s.rowTexts(Rect{From: t.Range.From, To: Addr{Col: t.Range.To.Col, Row: t.Range.From.Row}}), nil)
	}
	s.view.tables = append(s.view.tables, t)
	return nil
}
