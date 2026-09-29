package sheet

import "github.com/FelineStateMachine/012/internal/functions"

// How the engine's reader reads a linked source's tab (source.go),
// which holds no cells: its header row from what the host found, and
// every other cell from the host (sourceask.go). A function given a
// source's range is asked whole (functions.StreamCall); what reads a
// range some other way (an operator over it, a function whose other
// arguments are arrays) is given its values read whole, as long as the
// host's budget allows.

// pagedRegion is the paged region of t, if t is a source's tab.
func (t *Sheet) pagedRegion() (Region, bool) {
	for _, r := range t.regions.list {
		if r.File.Paged {
			return r, true
		}
	}
	return Region{}, false
}

// notOpen is what a source's cells read before its host has opened it:
// Loading…, or #REF! when it can't.
func (t *Sheet) notOpen(r Region) Value {
	if t.sourceErr(r) != "" {
		return ErrRef
	}
	return Pending
}

// pagedCell is the value of the cell at a of t, the tab of the source
// r: a column's name in the header row, or what the host answers.
func (rd *reader) pagedCell(t *Sheet, r Region, a Addr) Value {
	tbl, shape, ok := t.sourceTable(r)
	switch {
	case !ok:
		if l := rd.w.evaluating; l.s != nil {
			addLoc(&rd.w.src.users, nameKey(r.Name), l)
		}
		return t.notOpen(r)
	case !tbl.Contains(a):
		return Value{}
	case a.Row == 0:
		return Value{Kind: Text, Str: shape.Cols[a.Col]}
	}
	ans, ok := rd.w.ask(SourceQuestion{Kind: AskCell, Source: r.Name, At: a})
	if !ok {
		return Pending
	}
	return ans.V
}

// pagedRange is the part of r on t, the tab of the source reg, that its
// table holds, and its values as an array: nil with an error when they
// can't be had (yet), nil without one when the part is empty.
func (rd *reader) pagedRange(t *Sheet, reg Region, r Rect) (Rect, *functions.Array, *Value) {
	tbl, _, ok := t.sourceTable(reg)
	if !ok {
		v := t.notOpen(reg)
		return Rect{}, nil, &v
	}
	part, ok := intersectRect(tbl, r)
	if !ok {
		return Rect{}, nil, nil
	}
	ans, known := rd.w.ask(SourceQuestion{Kind: AskRange, Source: reg.Name, R: part})
	switch {
	case !known:
		return Rect{}, nil, &Pending
	case ans.A == nil:
		v := ans.V
		if v.Kind != Error {
			v = ErrValue
		}
		return Rect{}, nil, &v
	}
	return part, ans.A, nil
}

// pagedScan is Scan over a source's tab: the cells of r holding
// something, from from on, read from the range's values.
func (rd *reader) pagedScan(t *Sheet, reg Region, r Rect, from Addr, addrs []Addr, vals []Value) int {
	part, arr, err := rd.pagedRange(t, reg, r)
	if err != nil {
		if len(addrs) == 0 || from != r.From {
			return 0 // the error was the range's first cell
		}
		addrs[0] = r.From
		if vals != nil {
			vals[0] = *err
		}
		return 1
	}
	if arr == nil {
		return 0
	}
	if from.Row < part.From.Row {
		from = part.From
	}
	n := 0
	for row := from.Row; row <= part.To.Row && n < len(addrs); row++ {
		col := part.From.Col
		if row == from.Row {
			col = max(col, from.Col)
		}
		for ; col <= part.To.Col && n < len(addrs); col++ {
			v := arr.At(row-part.From.Row, col-part.From.Col)
			if v.Kind == Empty {
				continue
			}
			addrs[n] = Addr{Col: col, Row: row}
			if vals != nil {
				vals[n] = v
				if v.Kind == Error {
					return n + 1
				}
			}
			n++
		}
	}
	return n
}

// pagedFold is Fold over a source's tab.
func (rd *reader) pagedFold(t *Sheet, reg Region, r Rect, s functions.Agg) (functions.Agg, *Value) {
	part, arr, err := rd.pagedRange(t, reg, r)
	if err != nil {
		return s, s.Add(*err, false)
	}
	if arr == nil {
		return s, nil
	}
	for i := range part.To.Row - part.From.Row + 1 {
		for j := range part.To.Col - part.From.Col + 1 {
			if v := arr.At(i, j); v.Kind != Empty {
				if e := s.Add(v, false); e != nil {
					return s, e
				}
			}
		}
	}
	return s, nil
}

// pagedBounds is Bounds over a source's tab: the part of r its table
// holds.
func (t *Sheet) pagedBounds(reg Region, r Rect) (Rect, bool) {
	tbl, _, ok := t.sourceTable(reg)
	if !ok {
		return Rect{}, false
	}
	return intersectRect(tbl, r)
}
