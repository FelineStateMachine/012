package sheet

import (
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Keeping tables (table.go) in step with their cells. At the end of
// every change, before recalculating, a table grows over the rows just
// below it that the change filled, and its columns' names are read
// again from its header row when the change touched it: a name changed
// in place renames the column in the formulas that read it, and a
// blank or repeated header is given a name of its own. Undo puts back
// the tables with the view, and the formulas and cells with theirs, so
// it runs none of this.

// settleTables brings the tables on the sheets the open step changed
// in step with their cells, within the step.
func (w *Workbook) settleTables() {
	var touched map[*Sheet][]Addr
	for _, l := range w.hist.dirty {
		if len(l.s.view.tables) == 0 || !l.s.live {
			continue
		}
		if touched == nil {
			touched = map[*Sheet][]Addr{}
		}
		touched[l.s] = append(touched[l.s], l.a)
	}
	for s, at := range touched {
		for _, t := range s.Tables() {
			s.growTable(t, at)
		}
		for _, t := range s.Tables() {
			if headerTouched(t, at) {
				s.syncHeader(t)
			}
		}
	}
}

// headerTouched reports whether a cell of t's header row is among at,
// or t's columns don't match its width.
func headerTouched(t Table, at []Addr) bool {
	if len(t.Cols) != t.Range.To.Col-t.Range.From.Col+1 {
		return true
	}
	return slices.ContainsFunc(at, func(a Addr) bool {
		return a.Row == t.Range.From.Row && a.Col >= t.Range.From.Col && a.Col <= t.Range.To.Col
	})
}

// growTable extends t over the rows just below it that cells among at,
// filled in t's columns, make a run from its last row.
func (s *Sheet) growTable(t Table, at []Addr) {
	filled := map[int]bool{}
	for _, a := range at {
		if a.Row > t.Range.To.Row && a.Col >= t.Range.From.Col && a.Col <= t.Range.To.Col && s.cells.filledAt(a) {
			filled[a.Row] = true
		}
	}
	end := t.Range.To.Row
	for filled[end+1] {
		end++
	}
	if end == t.Range.To.Row {
		return
	}
	r := t.Range
	r.To.Row = end
	if s.checkTableRange(r, nameKey(t.Name)) != nil {
		return
	}
	s.editTable(nameKey(t.Name), func(t *Table) { t.Range = r })
}

// syncHeader reads t's columns' names again from its header row,
// renaming in formulas each column whose name changed in place.
func (s *Sheet) syncHeader(t Table) {
	names := s.headerNames(t)
	if slices.Equal(names, t.Cols) {
		return
	}
	k := nameKey(t.Name)
	for old, name := range renamedColumns(t.Cols, names) {
		s.wb.renameColumnInFormulas(k, old, name)
	}
	s.editTable(k, func(t *Table) { t.Cols = names })
}

// renamedColumns pairs the names in before that are gone from after
// with the names in their place that are new, by position: a column
// renamed in its header. Names that moved, as when columns are inserted,
// deleted or moved, rename nothing.
func renamedColumns(before, after []string) map[string]string {
	out := map[string]string{}
	has := func(list []string, name string) bool {
		return slices.ContainsFunc(list, func(n string) bool { return strings.EqualFold(n, name) })
	}
	for i := range min(len(before), len(after)) {
		old, name := before[i], after[i]
		if old == "" || old == name || has(after, old) || has(before, name) && !strings.EqualFold(old, name) {
			continue
		}
		out[old] = name
	}
	return out
}

// headerNames reads the names of t's columns from its header row (see
// columnNames), writing the names it gives blank or repeated headers
// into their cells, so the header shows what formulas use.
func (s *Sheet) headerNames(t Table) []string {
	texts := s.rowTexts(Rect{From: t.Range.From, To: Addr{Col: t.Range.To.Col, Row: t.Range.From.Row}})
	names := columnNames(texts, t.Cols)
	for i, name := range names {
		a := Addr{Col: t.Range.From.Col + i, Row: t.Range.From.Row}
		if name == texts[i] {
			continue
		}
		if _, spilled := s.SpillAnchor(a); !spilled {
			s.put(a, name)
		}
	}
	return names
}

// columnNames are the names of a table's columns whose header cells
// show texts: each text, with a blank one named after its place
// (Column3) and a repeat numbered (Amount2). Of two headers the same, the
// one already a column's name in prev, the names before, keeps it.
func columnNames(texts, prev []string) []string {
	names := make([]string, len(texts))
	seen := map[string]bool{}
	for i, name := range texts {
		if k := nameKey(name); name != "" && i < len(prev) && nameKey(prev[i]) == k && !seen[k] {
			seen[k], names[i] = true, name
		}
	}
	for i, name := range texts {
		if k := nameKey(name); name != "" && names[i] == "" && !seen[k] {
			seen[k], names[i] = true, name
		}
	}
	for i, name := range names {
		if name != "" {
			continue
		}
		base := texts[i]
		if base == "" {
			base = "Column"
		}
		name = uniqueColumn(base, i+1, seen)
		seen[nameKey(name)], names[i] = true, name
	}
	return names
}

// uniqueColumn is base numbered so no name in seen has it: base with
// the column's place for a blank header (Column3), else from 2 up.
func uniqueColumn(base string, place int, seen map[string]bool) string {
	from := 2
	if base == "Column" {
		from = place
	}
	for i := from; ; i++ {
		if name := base + strconv.Itoa(i); !seen[nameKey(name)] {
			return name
		}
	}
}

// shiftTables moves tables with inserted or deleted rows or columns, as
// a range reference moves. A table whose header row or every column is
// deleted goes; one left without data rows keeps an empty one. Columns
// inserted into a table come in unnamed, to be named from their header
// cells when the change ends.
func shiftTables(ts []Table, rows bool, sp formula.Span) []Table {
	if len(ts) == 0 {
		return ts
	}
	_, rng := formula.AxisMaps(rows, sp)
	var out []Table
	for _, t := range ts {
		if rows && sp.N < 0 && t.Range.From.Row >= sp.At && t.Range.From.Row < sp.At-sp.N {
			continue // the header row went
		}
		r, ok := rng(t.Range)
		if !ok {
			continue
		}
		t = t.clone()
		if !rows {
			t.Cols = shiftColumns(t, r, sp)
		}
		t.Range = tableRange(r)
		out = append(out, t)
	}
	return out
}

// shiftColumns are t's columns' names once its range is r, after
// columns were inserted or deleted: each kept column's name where the
// column went, and "" for columns inserted.
func shiftColumns(t Table, r Rect, sp formula.Span) []string {
	out := make([]string, r.To.Col-r.From.Col+1)
	for i, name := range t.Cols {
		if c, ok := sp.Point(t.Range.From.Col + i); ok && c >= r.From.Col && c <= r.To.Col {
			out[c-r.From.Col] = name
		}
	}
	return out
}

// remapTables moves the tables wholly inside the cells moved with them,
// as cut and paste moves them in Sheets; rng maps a range that moved.
func (s *Sheet) remapTables(src Rect, rng func(Rect) (Rect, bool)) {
	if len(s.view.tables) == 0 {
		return
	}
	st := s.Tables()
	for i, t := range st {
		if src.Contains(t.Range.From) && src.Contains(t.Range.To) {
			if r, ok := rng(t.Range); ok {
				st[i].Range = r
			}
		}
	}
	s.putTables(st)
}

// moveTablesTo moves the tables wholly inside src on s to dst, shifted
// by shift, as cutting them from one sheet and pasting on another does.
func (s *Sheet) moveTablesTo(dst *Sheet, src Rect, shift func(Addr) Addr) {
	var moved, kept []Table
	for _, t := range s.Tables() {
		if src.Contains(t.Range.From) && src.Contains(t.Range.To) {
			t.Range = Rect{From: shift(t.Range.From), To: shift(t.Range.To)}
			moved = append(moved, t)
		} else {
			kept = append(kept, t)
		}
	}
	if len(moved) == 0 {
		return
	}
	s.putTables(kept)
	dst.putTables(append(dst.Tables(), moved...))
}
