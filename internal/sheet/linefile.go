package sheet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Column and row formats in the file: a "lines" field after the widths,
// keyed by columns or rows as a range writes them, runs of lines with the
// same formatting together, each value a formatted blank cell:
//
//	"lines": {"A:XFD": {"italic": true}, "B:B": {"format": "currency", "decimals": 2}, "1:1": {"bold": true}},
//
// A:XFD, every column, is the whole sheet's format.
//
// Earlier builds ignore the field and show those cells unformatted, so it
// needs no new version.

// writeLines writes the "lines" field, if the sheet has line formats.
func (s *Sheet) writeLines(b *bytes.Buffer, indent string) error {
	if s.lines.none() {
		return nil
	}
	b.WriteString(indent + `"lines": {`)
	first := true
	if l := s.lines.sheet; !l.IsZero() {
		raw, err := encodeCell(&Cell{Format: l.Format, Style: l.Style})
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "%q: %s", s.lineRect(false, wholeSheet, wholeSheet).String(), raw)
		first = false
	}
	for _, row := range []bool{false, true} {
		m := s.lines.cols
		if row {
			m = s.lines.rows
		}
		ns := slices.Sorted(maps.Keys(m))
		for i := 0; i < len(ns); {
			j := i + 1
			for j < len(ns) && ns[j] == ns[j-1]+1 && m[ns[j]] == m[ns[i]] {
				j++
			}
			l := m[ns[i]]
			raw, err := encodeCell(&Cell{Format: l.Format, Style: l.Style})
			if err != nil {
				return err
			}
			if !first {
				b.WriteString(", ")
			}
			first = false
			fmt.Fprintf(b, "%q: %s", s.lineRect(row, ns[i], ns[j-1]).String(), raw)
			i = j
		}
	}
	b.WriteString("},\n")
	return nil
}

// readLines sets the line formats of a file's "lines" field.
func (s *Sheet) readLines(lines map[string]json.RawMessage) error {
	for key, raw := range lines {
		from, to, found := strings.Cut(key, ":")
		if !found {
			to = from
		}
		r, _, ok := formula.ParseLines(from, to)
		if !ok {
			return fmt.Errorf("invalid lines %q", key)
		}
		input, f, st, err := decodeCell(raw)
		if err != nil || input != "" {
			return fmt.Errorf("lines %s: invalid format", key)
		}
		if r.AllRows() && r.AllCols() {
			s.setLine(false, wholeSheet, lineFmt{f, st})
			continue
		}
		row := !r.AllRows()
		n0, n1 := r.From.Col, r.To.Col
		if row {
			n0, n1 = r.From.Row, r.To.Row
		}
		for n := n0; n <= n1; n++ {
			s.setLine(row, n, lineFmt{f, st})
		}
	}
	return nil
}
