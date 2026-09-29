package sheet

import (
	"bufio"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
)

// View state that belongs to the worksheet rather than to a cell: frozen
// rows and columns, the filter, the protected ranges and the merged
// cells. It is saved in the file and every change to it is an undo step,
// as in Sheets.
type viewState struct {
	frozenRows, frozenCols int
	filter                 *Filter      // never modified in place; replaced whole
	protected              []Protection // see protect.go; replaced whole too
	merges                 []Rect       // see merge.go; replaced whole too
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
	st := s.wb.hist.open
	if st == nil {
		return
	}
	if _, seen := st.views[s]; seen {
		return
	}
	v := s.view
	st.views[s] = &v
}

func (v viewState) equal(w viewState) bool {
	return v.frozenRows == w.frozenRows && v.frozenCols == w.frozenCols &&
		(v.filter == w.filter || reflect.DeepEqual(v.filter, w.filter)) &&
		slices.Equal(v.protected, w.protected) && slices.Equal(v.merges, w.merges)
}

// shiftView keeps frozen lines, the filter and protected ranges in step
// with inserted or deleted rows or columns: inserting inside the frozen
// area freezes the new lines too, and the filter's and protections'
// ranges grow, shrink or move like range references (and go away when
// all of one is deleted).
func (s *Sheet) shiftView(rows bool, sp formula.Span) {
	v := s.view
	v.protected = shiftProtected(v.protected, rows, sp)
	v.merges = shiftMerges(v.merges, rows, sp)
	frozen := &v.frozenCols
	if rows {
		frozen = &v.frozenRows
	}
	if *frozen > 0 {
		if lo, hi, ok := sp.Interval(0, *frozen-1); ok && lo == 0 {
			*frozen = hi + 1
		} else if !ok {
			*frozen = 0
		}
		*frozen = min(*frozen, MaxFrozen)
	}
	if v.filter != nil {
		v.filter = shiftFilter(v.filter, rows, sp)
	}
	if !v.equal(s.view) {
		s.recordView()
		merged := !slices.Equal(v.merges, s.view.merges)
		s.view = v
		if merged { // a merge gone may free an array to spill
			s.respill(Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}})
		}
	}
}

// shiftFilter is f with rows or columns inserted or deleted, nil once
// its range is all deleted.
func shiftFilter(f *Filter, rows bool, sp formula.Span) *Filter {
	_, rng := formula.AxisMaps(rows, sp)
	r, ok := rng(f.Range)
	if !ok {
		return nil
	}
	nf := &Filter{Range: r, Cols: map[int]Criteria{}}
	for c, cr := range f.Cols {
		if !rows {
			var keep bool
			if c, keep = sp.Point(c); !keep || c < r.From.Col || c > r.To.Col {
				continue
			}
		}
		nf.Cols[c] = cr
	}
	return nf
}

// fileView is the view state in the file (version 3), e.g.
//
//	"freeze": {"rows": 1},
//	"filter": {"range": "A1:C20", "columns": {"B": {"condition": "gt", "value": "100"}}},
type fileView struct {
	Freeze    *fileFreeze      `json:"freeze,omitempty"`
	Filter    *fileFilter      `json:"filter,omitempty"`
	Protected []fileProtection `json:"protected,omitempty"`
	// Merges need no version: earlier builds ignore them and show the
	// cells unmerged.
	Merges []string `json:"merges,omitempty"`
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

// writeView adds the frozen panes, the filter and the protected ranges to
// a file being written.
func (s *Sheet) writeView(b *bufio.Writer, indent string) error {
	var keys []string
	var parts []any
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
	if ps := s.view.protected; len(ps) > 0 {
		keys, parts = append(keys, "protected"), append(parts, encodeProtections(ps))
	}
	if ms := s.view.merges; len(ms) > 0 {
		names := make([]string, len(ms))
		for i, m := range ms {
			names[i] = m.String()
		}
		keys, parts = append(keys, "merges"), append(parts, names)
	}
	for i, p := range parts {
		raw, err := json.Marshal(p)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "%s%q: %s,\n", indent, keys[i], raw)
	}
	return nil
}

// readView restores the frozen panes, the filter and the protected
// ranges from a file.
func (s *Sheet) readView(fv fileView) error {
	ps, err := decodeProtections(fv.Protected)
	if err != nil {
		return err
	}
	s.view.protected = ps
	for _, name := range fv.Merges {
		r, ok := ParseRange(name)
		if !ok {
			return fmt.Errorf("invalid merged range %q", name)
		}
		s.LoadMerge(r)
	}
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
