package headless

import (
	"fmt"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Operation is one change of an Apply: what the MCP apply_operations
// tool takes, each the same engine call a menu command makes. Op says
// which, and the other fields are its arguments:
//
//	set             Ref (one cell) and Input or Value, and Format, as Set takes an Entry
//	write_table     Rows under a header at Ref, formatted as WriteTable formats them
//	clear           Ref: the range's contents, keeping formats and notes
//	insert_rows     before Ref's first row, Count rows (Ref's rows when 0)
//	delete_rows     Ref's rows
//	insert_columns  before Ref's first column, Count columns
//	delete_columns  Ref's columns
//	add_sheet       Name, after the last sheet
//	rename_sheet    Ref (a sheet) to Name
//	delete_sheet    Ref (a sheet)
//	define_name     Name for the range Ref
//	sort            Ref's rows by Keys, the first row left in place with Header
type Operation struct {
	Op      string            `json:"op" jsonschema:"set, write_table, clear, insert_rows, delete_rows, insert_columns, delete_columns, add_sheet, rename_sheet, delete_sheet, define_name or sort"`
	Ref     string            `json:"ref,omitempty" jsonschema:"the cell, range or sheet the operation acts on, as formulas write it: B7, Q3!A1:C9, 'Q3 plan'!A:A, Q3; for write_table its top-left cell"`
	Input   string            `json:"input,omitempty" jsonschema:"for set: what to type, as a person types it in en-US form (=SUM(A1:A6), $3.50, 12%, 2026-09-29); empty clears the cell"`
	Value   Value             `json:"value,omitempty" jsonschema:"for set, instead of input: the value with its type, as write_cells takes it: {\"currency\": 3.5}, {\"date\": \"2026-09-29\"}, \"00123\""`
	Format  string            `json:"format,omitempty" jsonschema:"for set: a number format code for the cell, or alone for every cell of ref: $#,##0.00, 0.0%, yyyy-mm-dd"`
	Columns []string          `json:"columns,omitempty" jsonschema:"for write_table: the columns' names in order"`
	Rows    []Row             `json:"rows,omitempty" jsonschema:"for write_table: the rows, each a record of typed values or a list of them in columns' order, as write_table takes them"`
	Formats map[string]string `json:"formats,omitempty" jsonschema:"for write_table: number format codes by column name"`
	Count   int               `json:"count,omitempty" jsonschema:"for insert_rows and insert_columns: how many; 0 inserts as many as ref spans"`
	Name    string            `json:"name,omitempty" jsonschema:"for add_sheet and rename_sheet the sheet's name, for define_name the range's"`
	Keys    []SortKey         `json:"keys,omitempty" jsonschema:"for sort: the columns to sort by, first first"`
	Header  bool              `json:"header,omitempty" jsonschema:"for sort: the range's first row is a header and stays in place"`
}

// SortKey is a column to sort by: a letter (B) or, with a header, the
// header's text.
type SortKey struct {
	Column     string `json:"column" jsonschema:"a column letter (B) or the header's text"`
	Descending bool   `json:"descending,omitempty"`
}

// Apply makes ops as one change, in order, stopping at the first that
// fails with an error naming it; the workbook should then be thrown
// away rather than saved, as after Set.
func Apply(w *sheet.Workbook, ops []Operation, o SetOptions) (warnings []string, err error) {
	err = w.Batch(sheet.Change{Label: "apply operations", Sheet: w.Sheet(w.Active())}, func() error {
		for i, op := range ops {
			warn, err := applyOne(w, op, o)
			if err != nil {
				return fmt.Errorf("operation %d (%s): %w", i+1, op.Op, err)
			}
			if warn != "" {
				warnings = append(warnings, warn)
			}
		}
		return nil
	})
	return warnings, err
}

func applyOne(w *sheet.Workbook, op Operation, o SetOptions) (string, error) {
	switch op.Op {
	case "set":
		return setOne(w, Entry{Ref: op.Ref, Input: op.Input, Value: op.Value, Format: op.Format}, o)
	case "write_table":
		warnings, err := writeTable(w, TableSpec{At: op.Ref, Columns: op.Columns, Rows: op.Rows, Formats: op.Formats}, o)
		return strings.Join(warnings, "; "), err
	case "add_sheet":
		_, err := w.AddSheet(op.Name, w.Len())
		return "", err
	case "define_name":
		t, err := Resolve(w, op.Ref)
		if err != nil {
			return "", err
		}
		return "", w.DefineName(op.Name, t.Sheet, targetRect(t))
	case "rename_sheet", "delete_sheet":
		s := w.Lookup(strings.TrimSuffix(op.Ref, "!"))
		if s == nil {
			return "", noSheet(w, op.Ref)
		}
		if op.Op == "rename_sheet" {
			return "", w.RenameSheet(s, op.Name)
		}
		return "", w.DeleteSheet(s)
	case "sort":
		return "", Sort(w, op.Ref, op.Keys, op.Header)
	}
	return "", rangeOp(w, op, o)
}

// rangeOp makes the operations on a range: clearing it, and inserting
// and deleting its rows or columns.
func rangeOp(w *sheet.Workbook, op Operation, o SetOptions) error {
	t, err := Resolve(w, op.Ref)
	if err != nil {
		return err
	}
	s, r := t.Sheet, targetRect(t)
	rows, cols := r.To.Row-r.From.Row+1, r.To.Col-r.From.Col+1
	if op.Count > 0 {
		rows, cols = op.Count, op.Count
	}
	switch op.Op {
	case "clear":
		if p, ok := s.Protecting(r); ok && !o.Force {
			return fmt.Errorf("%s is protected (%s): force clears it anyway", t, p.Label())
		}
		s.EraseRange(r)
		return nil
	case "insert_rows":
		return s.InsertRows(r.From.Row, rows)
	case "delete_rows":
		s.DeleteRows(r.From.Row, r.To.Row-r.From.Row+1)
		return nil
	case "insert_columns":
		return s.InsertCols(r.From.Col, cols)
	case "delete_columns":
		s.DeleteCols(r.From.Col, r.To.Col-r.From.Col+1)
		return nil
	}
	return fmt.Errorf("no operation %q: set, write_table, clear, insert_rows, delete_rows, insert_columns, delete_columns, add_sheet, rename_sheet, delete_sheet, define_name or sort", op.Op)
}

// targetRect is the target's range: a whole sheet's is A1 to its last
// cell with contents.
func targetRect(t Target) sheet.Rect {
	if t.Whole {
		r, _ := t.Sheet.UsedRange()
		return r
	}
	return t.Range
}

// Sort sorts the rows of the range ref names by keys, as Data > Sort
// range does, leaving its first row in place when header is set.
func Sort(w *sheet.Workbook, ref string, keys []SortKey, header bool) error {
	t, err := Resolve(w, ref)
	if err != nil {
		return err
	}
	r := targetRect(t)
	if len(keys) == 0 {
		return fmt.Errorf("name a column to sort %s by", t)
	}
	var sk []sheet.SortKey
	for _, k := range keys {
		col, err := columnOf(t.Sheet, r, k.Column, header)
		if err != nil {
			return err
		}
		sk = append(sk, sheet.SortKey{Col: col, Desc: k.Descending})
	}
	if header {
		r.From.Row++
	}
	if len(t.Sheet.MergesIn(r)) > 0 {
		return fmt.Errorf("%s has merged cells, which can't be sorted", t)
	}
	t.Sheet.SortRange(r, sk)
	return nil
}

// columnOf finds a column of r by its letter, or with a header row by
// the header's text, ignoring case.
func columnOf(s *sheet.Sheet, r sheet.Rect, name string, header bool) (int, error) {
	if header {
		for col := r.From.Col; col <= r.To.Col; col++ {
			if v := s.Value(sheet.Addr{Col: col, Row: r.From.Row}); v.Kind == sheet.Text && strings.EqualFold(v.Str, name) {
				return col, nil
			}
		}
	}
	if a, ok := sheet.ParseAddr(strings.ToUpper(name) + "1"); ok && a.Row == 0 && a.Col >= r.From.Col && a.Col <= r.To.Col {
		return a.Col, nil
	}
	return 0, fmt.Errorf("%q isn't a column of %s: name it by its letter%s", name, r, headerHint(header))
}

func headerHint(header bool) string {
	if header {
		return " or its header"
	}
	return ""
}
