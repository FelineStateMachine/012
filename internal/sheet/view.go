package sheet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// View state that belongs to the worksheet rather than to a cell: frozen
// rows and columns, the filter and the arithmetic setting. It is saved in
// the file and every change to it is an undo step, as in Sheets.
type viewState struct {
	frozenRows, frozenCols int
	filter                 *Filter // never modified in place; replaced whole
	decimal                bool    // decimal arithmetic, see decimal.go
}

// Frozen returns how many rows and columns are frozen at the top and left.
func (s *Sheet) Frozen() (rows, cols int) {
	return s.view.frozenRows, s.view.frozenCols
}

// MaxFrozen caps frozen rows and columns, as Sheets' View > Freeze does
// in practice: more than a screenful can't stay on screen anyway.
const MaxFrozen = 50

// SetFrozen freezes the first rows rows and cols columns (0 unfreezes).
func (s *Sheet) SetFrozen(rows, cols int) {
	rows, cols = clampInt(rows, 0, MaxFrozen), clampInt(cols, 0, MaxFrozen)
	if rows == s.view.frozenRows && cols == s.view.frozenCols {
		return
	}
	label := "unfreeze"
	switch {
	case rows != s.view.frozenRows:
		label = "freeze " + plural(rows, "row")
		if rows == 0 {
			label = "unfreeze rows"
		}
	case cols != s.view.frozenCols:
		label = "freeze " + plural(cols, "column")
		if cols == 0 {
			label = "unfreeze columns"
		}
	}
	focus := Rect{To: Addr{Col: max(cols, 1) - 1, Row: max(rows, 1) - 1}}
	s.change(label, focus, func() {
		s.recordView()
		s.view.frozenRows, s.view.frozenCols = rows, cols
	})
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// recordView saves the view state before its first change in the open
// step.
func (s *Sheet) recordView() {
	st := s.hist.open
	if st == nil || st.view != nil {
		return
	}
	v := s.view
	st.view = &v
}

func (v viewState) equal(w viewState) bool {
	return v.frozenRows == w.frozenRows && v.frozenCols == w.frozenCols && v.decimal == w.decimal &&
		(v.filter == w.filter || reflect.DeepEqual(v.filter, w.filter))
}

// shiftView keeps frozen lines and the filter in step with inserted or
// deleted rows or columns: inserting inside the frozen area freezes the
// new lines too, and the filter's range grows, shrinks or moves like a
// range reference (and goes away when all of it is deleted).
func (s *Sheet) shiftView(rows bool, sp span) {
	v := s.view
	frozen := &v.frozenCols
	if rows {
		frozen = &v.frozenRows
	}
	if *frozen > 0 {
		if lo, hi, ok := sp.interval(0, *frozen-1); ok && lo == 0 {
			*frozen = hi + 1
		} else if !ok {
			*frozen = 0
		}
		*frozen = min(*frozen, MaxFrozen)
	}
	if f := v.filter; f != nil {
		_, rng := axisRewrite(rows, sp)
		if r, ok := rng(f.Range); ok {
			nf := &Filter{Range: r, Cols: map[int]Criteria{}}
			for c, cr := range f.Cols {
				if !rows {
					var keep bool
					if c, keep = sp.point(c); !keep || c < r.From.Col || c > r.To.Col {
						continue
					}
				}
				nf.Cols[c] = cr
			}
			v.filter = nf
		} else {
			v.filter = nil
		}
	}
	if !v.equal(s.view) {
		s.recordView()
		s.view = v
	}
}

// fileView is the view state in the file (version 3), e.g.
//
//	"freeze": {"rows": 1},
//	"filter": {"range": "A1:C20", "columns": {"B": {"condition": "gt", "value": "100"}}},
//
// and the arithmetic setting, which needs no version bump: earlier
// builds ignore it and compute in binary, as Sheets would.
//
//	"arithmetic": "decimal",
type fileView struct {
	Arithmetic string      `json:"arithmetic,omitempty"`
	Freeze     *fileFreeze `json:"freeze,omitempty"`
	Filter     *fileFilter `json:"filter,omitempty"`
}

type fileFreeze struct {
	Rows int `json:"rows,omitempty"`
	Cols int `json:"cols,omitempty"`
}

type fileFilter struct {
	Range   string                  `json:"range"`
	Columns map[string]fileCriteria `json:"columns,omitempty"`
}

type fileCriteria struct {
	Hidden    []string `json:"hidden,omitempty"`
	Condition string   `json:"condition,omitempty"`
	Value     string   `json:"value,omitempty"`
}

// writeView adds the frozen panes and the filter to a file being written.
func (s *Sheet) writeView(b *bytes.Buffer) error {
	var keys []string
	var parts []any
	if s.view.decimal {
		keys, parts = append(keys, "arithmetic"), append(parts, "decimal")
	}
	if v := s.view; v.frozenRows > 0 || v.frozenCols > 0 {
		keys, parts = append(keys, "freeze"), append(parts, fileFreeze{Rows: v.frozenRows, Cols: v.frozenCols})
	}
	if f := s.view.filter; f != nil {
		ff := fileFilter{Range: f.Range.String(), Columns: map[string]fileCriteria{}}
		for c, cr := range f.Cols {
			ff.Columns[ColName(c)] = fileCriteria{Hidden: cr.Hidden, Condition: cr.Cond.Op.String(), Value: cr.Cond.Arg}
		}
		keys, parts = append(keys, "filter"), append(parts, ff)
	}
	for i, p := range parts {
		raw, err := json.Marshal(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "  %q: %s,\n", keys[i], raw)
	}
	return nil
}

// readView restores the frozen panes and the filter from a file.
func (s *Sheet) readView(fv fileView) error {
	// Anything but "decimal" (say, a mode from a later build) computes
	// in binary, as the file would in a build without the setting.
	s.view.decimal = fv.Arithmetic == "decimal"
	if fz := fv.Freeze; fz != nil {
		s.view.frozenRows, s.view.frozenCols = clampInt(fz.Rows, 0, MaxFrozen), clampInt(fz.Cols, 0, MaxFrozen)
	}
	if ff := fv.Filter; ff != nil {
		r, ok := ParseRange(ff.Range)
		if !ok {
			return fmt.Errorf("invalid filter range %q", ff.Range)
		}
		f := &Filter{Range: r, Cols: map[int]Criteria{}}
		for name, fc := range ff.Columns {
			c, ok := ParseCol(name)
			if !ok {
				return fmt.Errorf("invalid filter column %q", name)
			}
			op, ok := ParseCondOp(fc.Condition)
			if !ok {
				return fmt.Errorf("unknown filter condition %q", fc.Condition)
			}
			f.Cols[c] = Criteria{Hidden: fc.Hidden, Cond: Condition{Op: op, Arg: fc.Value}}
		}
		s.view.filter = f.clone()
	}
	s.hidden.valid = false
	return nil
}
