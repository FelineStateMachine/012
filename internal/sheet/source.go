package sheet

import (
	"errors"
	"slices"
	"strconv"
)

// Linked sources: a Parquet file or a SQLite table or query linked as a
// read-only table too big for any grid. A source is a tab of its own, a
// paged sheet, holding no cells: its table is one region at A1 whose
// definition (a LinkSource with Paged set) names what it reads, so it
// is a table as a linked file's is, sales[amount] and nu.sales, but its
// rows are never written into cells. The UI shows a window of them as
// it scrolls; formulas and pivot tables ask the workbook's SourceHost
// (sourceask.go), which answers them by streaming the source in the
// background. Its table's rows may go past the grid's last row.

// SourceShape is what a source is, as its host finds on opening it:
// its columns' names, the formats their types show in, which hold
// numbers, and how many rows it has under the header.
type SourceShape struct {
	Cols    []string
	Formats []Format
	Numeric []bool
	Rows    int
}

// SourceSort is a column a source's view is sorted by.
type SourceSort struct {
	Col  int
	Desc bool
}

// SourceFilter is a condition a source's view shows the rows meeting,
// on a column.
type SourceFilter struct {
	Col  int
	Cond Condition
}

// SourceOrder is how a source's tab sorts and filters the rows it
// shows. Formulas read the source in its own order, every row, as they
// read a sheet whatever its filter hides.
type SourceOrder struct {
	Sort   []SourceSort
	Filter []SourceFilter
}

// IsZero reports whether the order is the source's own, every row.
func (o *SourceOrder) IsZero() bool { return o == nil || len(o.Sort) == 0 && len(o.Filter) == 0 }

// sourceMeta is what a source's host found of it, never undone.
type sourceMeta struct {
	shape SourceShape
	known bool   // shape holds what the host found
	err   string // why the source can't be read, or ""
}

// Errors of sources.
var (
	// ErrSourceEdit is what typing into a source's tab says.
	ErrSourceEdit = errors.New("A linked source is read-only: its rows are the file's")
	// ErrNoSource is returned for a source the workbook doesn't hold.
	ErrNoSource = errors.New("There's no linked source by that name")
)

// SourceInfo is a linked source as the UI shows it.
type SourceInfo struct {
	// Name names it in formulas (sales[amount], nu.sales) and in the
	// SourceHost's questions.
	Name   string
	Sheet  *Sheet
	Source LinkSource
	Shape  SourceShape
	// Known is set once its host has opened it; Err says why it can't.
	Known bool
	Err   string
}

// Source returns the linked source the sheet is the tab of.
func (s *Sheet) Source() (SourceInfo, bool) {
	if r, ok := s.pagedRegion(); ok {
		return s.sourceInfo(r), true
	}
	return SourceInfo{}, false
}

// IsSource reports whether the sheet is a linked source's tab.
func (s *Sheet) IsSource() bool {
	_, ok := s.pagedRegion()
	return ok
}

func (s *Sheet) sourceInfo(r Region) SourceInfo {
	info := SourceInfo{Name: r.Name, Sheet: s, Source: r.File}
	if sm := s.meta(nameKey(r.Name)).src; sm != nil {
		info.Shape, info.Known, info.Err = sm.shape, sm.known, sm.err
	}
	return info
}

// Sources returns the workbook's linked sources, in the order of their
// tabs.
func (w *Workbook) Sources() []SourceInfo {
	var out []SourceInfo
	for _, s := range w.sheets {
		if info, ok := s.Source(); ok {
			out = append(out, info)
		}
	}
	return out
}

// LookupSource finds a linked source by name, ignoring case.
func (w *Workbook) LookupSource(name string) (SourceInfo, bool) {
	if s, r, ok := w.Region(name); ok && r.File.Paged {
		return s.sourceInfo(r), true
	}
	return SourceInfo{}, false
}

// hasSources reports whether the workbook holds a linked source.
func (w *Workbook) hasSources() bool {
	return slices.ContainsFunc(w.sheets, (*Sheet).IsSource)
}

// AddSource adds a tab linking src as a source named name (a free name
// made from the file's when ""), after the sheet after, as one undo
// step. The tab and the table share the name. Its host opens it.
func (w *Workbook) AddSource(name string, src LinkSource, after *Sheet) (*Sheet, error) {
	if name == "" {
		name = w.sourceName(src.Path)
	}
	if err := w.checkRegionName(name, false); err != nil {
		return nil, err
	}
	if err := w.checkName(nil, name); err != nil {
		return nil, err
	}
	src.Paged = true
	s := w.newSheet(name)
	s.regions.list = []Region{{Name: name, File: src}}
	at := len(w.sheets)
	if i := w.Index(after); i >= 0 {
		at = i + 1
	}
	w.change(s, "link source "+name, Rect{}, func() {
		w.recordSheets()
		w.insert(s, at)
	})
	return s, nil
}

// SetSourceOrder changes how a source's tab sorts and filters its rows,
// as one undo step; o nil shows the source's own order.
func (w *Workbook) SetSourceOrder(name string, o *SourceOrder) error {
	s, r, ok := w.Region(name)
	if !ok || !r.File.Paged {
		return ErrNoSource
	}
	if o.IsZero() {
		o = nil
	}
	if o == nil && r.File.Order == nil {
		return nil
	}
	k := nameKey(r.Name)
	s.change("order "+r.Name, Rect{}, func() {
		st := s.regionsCopy()
		st.list[s.regionIndex(k)].File.Order = o
		s.putRegions(st)
	})
	return nil
}

// SetSourceShape records what the host found on opening a source, or
// why it couldn't (err), and recalculates what reads it. It isn't an
// edit: the file is what it is.
func (w *Workbook) SetSourceShape(name string, shape SourceShape, err string) {
	s, r, ok := w.Region(name)
	if !ok || !r.File.Paged {
		return
	}
	s.meta(nameKey(r.Name)).src = &sourceMeta{shape: shape, known: err == "", err: err}
	w.SourceChanged(name)
}

// SourceChanged recalculates what reads a source, whose file changed:
// the formulas that asked it something or name its table, and the
// pivot tables over it. Its host has forgotten its answers.
func (w *Workbook) SourceChanged(name string) {
	s, r, ok := w.Region(name)
	if !ok || !r.File.Paged {
		return
	}
	k := nameKey(r.Name)
	var changed []loc
	for _, set := range []map[loc]struct{}{w.src.users[k], w.nameUsers[k], w.nameUsers[nameKey(regionPrefix+k)]} {
		for l := range set {
			if l.s.live && l.s.cells.has(l.a) {
				changed = append(changed, l)
			}
		}
	}
	delete(w.src.users, k)
	for _, p := range w.pivotsOn(s) {
		p.pivot.stale = true
	}
	w.recalcFrom(changed, false)
}

// sourceTable is the table of the paged region r on s, header row
// included, and whether it is known: its rows may pass the grid's.
func (s *Sheet) sourceTable(r Region) (Rect, SourceShape, bool) {
	sm := s.meta(nameKey(r.Name)).src
	if sm == nil || !sm.known || len(sm.shape.Cols) == 0 {
		return Rect{}, SourceShape{}, false
	}
	return Rect{To: Addr{Col: len(sm.shape.Cols) - 1, Row: sm.shape.Rows}}, sm.shape, true
}

// sourceErr is why the paged region r can't be read, "" when it can or
// its host hasn't said yet.
func (s *Sheet) sourceErr(r Region) string {
	if sm := s.meta(nameKey(r.Name)).src; sm != nil {
		return sm.err
	}
	return ""
}

// sourceName is a free name for a source reading the file at path, as
// both a table and a tab: the file's name, numbered when taken.
func (w *Workbook) sourceName(path string) string {
	name := w.linkName(path)
	base := name
	for n := 2; w.checkName(nil, name) != nil || w.checkRegionName(name, false) != nil; n++ {
		name = base + "_" + strconv.Itoa(n)
	}
	return name
}
