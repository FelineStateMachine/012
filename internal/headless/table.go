package headless

import (
	"encoding/json"
	"fmt"

	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// TableSpec is a table to write: rows of records, each field a Value,
// under a header row naming their fields, as importing NUON stores a
// table. Columns orders them: a JSON object's keys may arrive in
// another order than written.
type TableSpec struct {
	At      string            `json:"at" jsonschema:"the table's top-left cell, where its header goes: A1, Q3!B2"`
	Columns []string          `json:"columns,omitempty" jsonschema:"the columns' names in order, left to right; rows' other fields follow them. Give it: a record's fields may arrive in any order"`
	Rows    []Row             `json:"rows" jsonschema:"the rows, each a record of the columns' values or a list of them in columns' order, typed as a cell's value is: {\"Item\": \"Tea\", \"Price\": {\"currency\": 3.5}, \"Bought\": {\"date\": \"2026-09-29\"}} or [\"Tea\", {\"currency\": 3.5}, {\"date\": \"2026-09-29\"}]"`
	Formats map[string]string `json:"formats,omitempty" jsonschema:"number format codes by column name, for every cell of the column under the header: {\"Price\": \"$#,##0.00\"}"`
}

// Row is a table's row, in JSON or NUON: a record of its columns'
// Values by name, or a list of them in the columns' order.
type Row json.RawMessage

// MarshalJSON writes r as it is.
func (r Row) MarshalJSON() ([]byte, error) { return Value(r).MarshalJSON() }

// UnmarshalJSON keeps the JSON as it is.
func (r *Row) UnmarshalJSON(b []byte) error { return (*Value)(r).UnmarshalJSON(b) }

// WriteTable writes spec's rows under a header naming their columns,
// in the order they're first seen. Each column takes the format of its
// first value that has one (currency, a date, a size), or the one
// Formats gives it, so its plain numbers and blanks show as the rest
// do; a cell whose own value has another type keeps its own. Each cell
// is checked as Set checks it.
func WriteTable(w *sheet.Workbook, spec TableSpec, o SetOptions) (warnings []string, err error) {
	err = w.Batch(sheet.Change{Label: "write table"}, func() (err error) {
		warnings, err = writeTable(w, spec, o)
		return err
	})
	return warnings, err
}

func writeTable(w *sheet.Workbook, spec TableSpec, o SetOptions) (warnings []string, err error) {
	s, at, err := ResolveCell(w, spec.At)
	if err != nil {
		return nil, err
	}
	cols, rows, err := tableRows(spec.Rows, spec.Columns)
	if err != nil {
		return nil, err
	}
	if at.Col+len(cols) > sheet.MaxCols || at.Row+len(rows) >= sheet.MaxRows {
		return nil, fmt.Errorf("a table of %d rows and %d columns doesn't fit at %s", len(rows), len(cols), spec.At)
	}
	formats := columnFormats(cols, rows, spec.Formats)
	for i, name := range cols {
		t := typed{input: textInputOf(name)}
		if warn, err := putTyped(s, sheet.Addr{Col: at.Col + i, Row: at.Row}, t, o); err != nil {
			return warnings, err
		} else if warn != "" {
			warnings = append(warnings, warn)
		}
	}
	for r, row := range rows {
		for i, t := range row {
			if f, ok := formats[i]; ok && (!t.formatted || spec.Formats[cols[i]] != "") {
				t.format, t.formatted = f, true
			}
			warn, err := putTyped(s, sheet.Addr{Col: at.Col + i, Row: at.Row + 1 + r}, t, o)
			if err != nil {
				return warnings, err
			}
			if warn != "" {
				warnings = append(warnings, warn)
			}
		}
	}
	return warnings, nil
}

// tableRows reads rows as the columns they name, columns first and the
// others in the order first seen, and each row's cells in those
// columns. A row is a record, or a list of values in columns' order.
func tableRows(values []Row, columns []string) ([]string, [][]typed, error) {
	t := tableReader{index: map[string]int{}}
	for _, c := range columns {
		t.col(c)
	}
	for n, v := range values {
		val, err := nuon.Parse(v)
		if err == nil && val.Kind == nuon.List {
			val, err = t.record(val.List, len(columns))
		}
		if err != nil || val.Kind != nuon.Record {
			return nil, nil, fmt.Errorf("row %d isn't a record of the columns' values, or a list of them in columns' order: %s", n+1, v)
		}
		row := make([]typed, len(t.cols))
		for _, f := range val.Fields {
			i := t.col(f.Key)
			c, err := typedOf(f.Value)
			if err != nil {
				return nil, nil, fmt.Errorf("row %d, %s: %w", n+1, f.Key, err)
			}
			for len(row) <= i {
				row = append(row, typed{})
			}
			row[i] = c
		}
		t.rows = append(t.rows, row)
	}
	return t.cols, t.rows, nil
}

// tableReader collects a table's columns and rows.
type tableReader struct {
	cols  []string
	index map[string]int
	rows  [][]typed
}

// col is the column named name, added after the others when new.
func (t *tableReader) col(name string) int {
	i, ok := t.index[name]
	if !ok {
		i = len(t.cols)
		t.index[name] = i
		t.cols = append(t.cols, name)
	}
	return i
}

// record is a list of values as a record of the first n columns.
func (t *tableReader) record(vs []nuon.Value, n int) (nuon.Value, error) {
	if len(vs) > n {
		return nuon.Value{}, fmt.Errorf("%d values for %d columns", len(vs), n)
	}
	fs := make([]nuon.Field, len(vs))
	for i, v := range vs {
		fs[i] = nuon.Field{Key: t.cols[i], Value: v}
	}
	return nuon.RecordValue(fs...), nil
}

// columnFormats are the formats of the table's columns, by index: the
// one formats names, or the first a value sets.
func columnFormats(cols []string, rows [][]typed, formats map[string]string) map[int]sheet.Format {
	out := map[int]sheet.Format{}
	for i, name := range cols {
		if code := formats[name]; code != "" {
			out[i] = FormatOfCode(code)
			continue
		}
		for _, row := range rows {
			if i < len(row) && row[i].formatted {
				out[i] = row[i].format
				break
			}
		}
	}
	return out
}

// textInputOf is a column's name as an entry: text, whatever it looks
// like.
func textInputOf(name string) string {
	t, _ := typedOf(nuon.StringValue(name))
	return t.input
}
