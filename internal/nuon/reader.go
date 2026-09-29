package nuon

import (
	"io"
)

// Reader reads a table's rows from NUON or JSON text as they arrive:
//
//   - the table form, [[a, b]; [1, 2], [3, 4]], whose header is known
//     before the first row;
//   - a list, [{a: 1}, {a: 2, b: 3}]: each record is a row, and anything
//     else a row of one column named "value";
//   - a record, or any other single value, as one row;
//   - several of these one after another, as NDJSON or a stream of
//     records is written, all rows of one table.
//
// Columns are named by the rows: a row may add columns or leave some
// out, as the records of a nushell list may.
type Reader struct {
	s      *scanner
	state  readState
	header []string
}

type readState uint8

const (
	between readState = iota // at the top level, between values
	inList                   // inside a top-level list
	inTable                  // inside a top-level table's rows
)

// NewReader reads rows from r.
func NewReader(r io.Reader) *Reader { return &Reader{s: newScanner(r)} }

// Header is the table form's column names, once its header has been read;
// nil for other shapes of table.
func (r *Reader) Header() []string { return r.header }

// JSON reports whether everything read so far was also JSON.
func (r *Reader) JSON() bool { return !r.s.notJSON }

// Next returns the next row, or io.EOF after the last. A syntax error is
// a *SyntaxError.
func (r *Reader) Next() (Row, error) {
	for {
		row, done, err := r.step()
		if err != nil || done {
			return row, err
		}
	}
}

// step reads up to the next row; done is false when it only moved past
// the end of a list or table.
func (r *Reader) step() (Row, bool, error) {
	switch r.state {
	case inList, inTable:
		c, ok, err := r.s.space(true)
		if err != nil {
			return nil, true, err
		}
		if !ok {
			return nil, true, r.s.fail("unterminated list")
		}
		if c == ']' {
			r.s.take()
			r.state = between
			return nil, false, nil
		}
		if r.state == inTable {
			row, err := r.s.tableRow(r.header)
			return row, true, err
		}
		v, err := r.s.value()
		return rowOf(v), true, err
	}
	c, ok, err := r.s.space(false)
	if err != nil {
		return nil, true, err
	}
	if !ok {
		return nil, true, io.EOF
	}
	if c != '[' {
		v, err := r.s.value()
		return rowOf(v), true, err
	}
	return r.open()
}

// open reads the start of a top-level list: its first item, or the
// header of the table form.
func (r *Reader) open() (Row, bool, error) {
	r.s.take() // [
	r.state = inList
	c, ok, err := r.s.space(true)
	if err != nil || !ok || c != '[' {
		return nil, false, err // an empty list, or items read by step
	}
	first, err := r.s.list()
	if err != nil {
		return nil, true, err
	}
	if c, _, err := r.s.space(true); err != nil || c != ';' {
		return rowOf(first), true, err
	}
	r.s.take() // ;
	r.s.notJSON = true
	if r.header, err = r.s.columns(first.List); err != nil {
		return nil, true, err
	}
	r.state = inTable
	return nil, false, nil
}

// rowOf is a value as a row: a record's fields, or else one column
// named "value".
func rowOf(v Value) Row {
	if v.Kind == Record {
		return v.Fields
	}
	return Row{{Key: "value", Value: v}}
}
