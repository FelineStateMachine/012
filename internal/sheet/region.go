package sheet

import (
	"errors"
	"fmt"
	"slices"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// Regions are blocks of cells whose values come from outside the
// engine, of two kinds: a linked file (a file followed as it grows or is
// rewritten, linked.go) and a notebook cell's output sent to a sheet
// (Output). A region is an anchor (At, its table's first cell) and a
// definition saying where its table comes from. Like a spill, its cells
// are derived: written by the engine outside the undo history, refused
// by Set, values to formulas, copies and exports, formatting kept, never
// saved; the file keeps the definitions only.
//
// Both kinds are one indexed set, the sheet's regionState, and their
// cells have one write path: rows arrive as live operations (LiveOp,
// live.go), written by writeTable (regionwrite.go). Their rows are never
// part of the undo state: the source is where they come from, and undo
// can't bring back what it held. When undo puts a region back, or it
// moves, it is emptied and marked stale, and the UI sends its rows
// again: the file read again, or the cell's output as it stands.
//
// The definitions are undo steps. So are a notebook tab's cells, which
// the same state holds (notebook.go).
//
// In formulas a region is a name: nu. and its own (=SUM(nu.sales)),
// which stands for its table, header row included.

// Region is a region's definition.
type Region struct {
	// Name identifies the region in the workbook, ignoring case: a
	// linked file's is made from the file's name, a sent output's is its
	// cell's. Notebook cells read linked files as $name.
	Name string
	// At is the table's first cell.
	At Addr
	// File is the file a linked file follows; zero for a sent output.
	File LinkSource
	// Output is set on a notebook cell's output sent to the sheet: the
	// output of the cell of the region's name.
	Output bool
}

// Linked reports whether the region is a linked file.
func (r Region) Linked() bool { return r.File.Path != "" }

// regionState is what a sheet keeps of its regions in the undo history:
// the definitions in the order they were made and, on a notebook tab,
// its cells. It is replaced whole, never changed in place.
type regionState struct {
	notebook bool
	list     []Region
	cells    []notebook.Cell
	reactive bool
}

// regionMeta is what isn't undone: the cells written last and what the
// rows written made of the region.
type regionMeta struct {
	written Rect
	has     bool   // written holds something
	why     string // why the table isn't shown, or ""
	need    Rect   // the cells the table needs while it isn't shown
	// rows and cols are the table's size as written, header included.
	rows, cols int
	liveMeta   // how its rows arrive: see linked.go
}

// Errors of regions.
var (
	// ErrOutputEdit is what typing into a sent output's cell says.
	ErrOutputEdit = errors.New("That cell shows a notebook cell's output: change the cell, or freeze the region to edit its values")
	// ErrNoRegion is returned for a region the workbook doesn't hold:
	// deleted, unlinked, undone, or on a sheet deleted.
	ErrNoRegion = errors.New("There's no region by that name")
)

// regionPrefix starts a region's name in formulas.
const regionPrefix = "nu."

// FormulaName is how formulas name the region: nu.sales.
func (r Region) FormulaName() string { return regionPrefix + r.Name }

// ValidRegionName checks that name can name a region: a nushell
// variable's name of letters, digits and _, not starting with a digit,
// as a notebook cell's name is.
func ValidRegionName(name string) error { return notebook.ValidName(name) }

// HasRegions reports whether the sheet has regions, so what draws it can
// skip looking for them.
func (s *Sheet) HasRegions() bool { return len(s.regions.list) > 0 }

// Regions returns the sheet's regions in the order they were made.
func (s *Sheet) Regions() []Region { return slices.Clone(s.regions.list) }

// Region finds a region by name, ignoring case, on any sheet.
func (w *Workbook) Region(name string) (*Sheet, Region, bool) {
	k := nameKey(name)
	for _, s := range w.sheets {
		if i := s.regionIndex(k); i >= 0 {
			return s, s.regions.list[i], true
		}
	}
	return nil, Region{}, false
}

func (s *Sheet) regionIndex(k string) int {
	return slices.IndexFunc(s.regions.list, func(r Region) bool { return nameKey(r.Name) == k })
}

// allRegions returns every region of the workbook's sheets, sheet by
// sheet.
func (w *Workbook) allRegions() []Region {
	var out []Region
	for _, s := range w.sheets {
		out = append(out, s.regions.list...)
	}
	return out
}

// checkRegionName reports why name can't name a new region. A sent
// output (output set) takes its cell's name, which a notebook's cell
// has; any other region's name is one no cell has.
func (w *Workbook) checkRegionName(name string, output bool) error {
	if err := ValidRegionName(name); err != nil {
		return err
	}
	if _, r, ok := w.Region(name); ok {
		return fmt.Errorf("There's already a region named %s", r.Name)
	}
	if n, ok := w.LookupName(regionPrefix + name); ok {
		return fmt.Errorf("%s already names %s", n.Name, n.Ref())
	}
	if !output && w.cellNamed(name) {
		return fmt.Errorf("A notebook cell is named %s", name)
	}
	return nil
}

// AddRegion adds a region to the sheet as one undo step. It starts
// empty and stale: the UI sends its rows.
func (s *Sheet) AddRegion(r Region) error {
	if err := s.wb.checkRegionName(r.Name, r.Output); err != nil {
		return err
	}
	if !r.At.Valid() || s.cells.filledAt(r.At) || s.InPivot(Rect{From: r.At, To: r.At}) {
		return errLinkOver
	}
	if _, ok := s.RegionAt(r.At); ok {
		return errLinkOver
	}
	s.change("add "+r.Name, Rect{From: r.At, To: r.At}, func() {
		st := s.regionsCopy()
		st.list = append(st.list, r)
		s.putRegions(st)
	})
	s.meta(nameKey(r.Name)).stale = true
	return nil
}

// DeleteRegion removes a region and its cells, as one undo step.
func (s *Sheet) DeleteRegion(name string) error {
	k := nameKey(name)
	i := s.regionIndex(k)
	if i < 0 {
		return ErrNoRegion
	}
	r := s.regions.list[i]
	s.change("delete "+r.Name, s.covered(r), func() {
		st := s.regionsCopy()
		st.list = slices.Delete(st.list, i, i+1)
		s.putRegions(st)
	})
	return nil
}

// FreezeRegion turns a region's table into plain values, each in the
// format it showed, and removes the region, as one undo step.
func (s *Sheet) FreezeRegion(name string) error {
	k := nameKey(name)
	i := s.regionIndex(k)
	if i < 0 {
		return ErrNoRegion
	}
	r := s.regions.list[i]
	table, ok := s.RegionTable(r.Name)
	var cells []*Cell
	var at []Addr
	if ok {
		for a := range s.cells.anyKeysIn(table) {
			if c := s.cells.get(a); c.Spilled() {
				cells, at = append(cells, frozen(c)), append(at, a)
			}
		}
	}
	s.change("freeze "+r.Name, s.covered(r), func() {
		// The region goes first, so its cells take the values placed.
		st := s.regionsCopy()
		st.list = slices.Delete(st.list, i, i+1)
		s.putRegions(st)
		for j, a := range at {
			s.place(a, cells[j])
		}
	})
	return nil
}

// frozen is a region's cell as a plain one: its value typed in, in the
// format it shows.
func frozen(c *Cell) *Cell {
	p := &Cell{Value: c.Value, Format: c.Format, Style: c.Style, spilled: true}
	if p.Format.IsZero() {
		p.Format = c.auto
	}
	return p.plain().withNote(c.Note)
}

// regionsCopy is a copy of the sheet's regions to change and put back.
func (s *Sheet) regionsCopy() regionState {
	st := s.regions
	st.list = slices.Clone(st.list)
	return st
}

// putRegions replaces the sheet's regions, recording them for undo, and
// has their cells written again.
func (s *Sheet) putRegions(st regionState) {
	s.recordRegions()
	s.regions = st
	s.regionsStale = true
}

// recordRegions saves the sheet's regions before their first change in
// the open step.
func (s *Sheet) recordRegions() {
	if st := s.wb.hist.open; st != nil {
		if _, seen := st.regions[s]; !seen {
			st.regions[s] = s.regions
		}
	}
}

// sameRegions reports whether two states of a sheet's regions are the
// same.
func sameRegions(a, b regionState) bool {
	return a.notebook == b.notebook && a.reactive == b.reactive && slices.Equal(a.list, b.list) && slices.Equal(a.cells, b.cells)
}

// putRegionsBack puts back a sheet's regions from an undo step. A
// region the step brings back, or moves, is emptied and fed again once
// the change ends: its rows aren't part of the undo state, so the cells
// undo put there may not be its source's.
func (s *Sheet) putRegionsBack(st regionState) {
	for _, r := range st.list {
		i := s.regionIndex(nameKey(r.Name))
		if i < 0 || s.regions.list[i] != r {
			s.meta(nameKey(r.Name)).reread = true
		}
	}
	s.regions, s.regionsStale = st, true
}

// emptyUnder empties the regions holding cells that undo puts contents
// back in (what a region's table grew over once they left), so the
// contents come back and the region, sent again, finds them in its way,
// as before they left.
func (s *Sheet) emptyUnder(img *image) []loc {
	if len(s.regions.list) == 0 {
		return nil
	}
	var changed []loc
	img.each(func(a Addr, c *Cell) {
		if c == nil || c.Blank() || c.Spilled() {
			return
		}
		if _, me, ok := s.ownerOf(a); ok {
			changed = append(changed, s.emptyRegion(me)...)
		}
	})
	return changed
}

// emptyMoved empties the regions that putting st back moves or takes
// away, outside the undo history, as moving them does, before undo puts
// cells back: a cell a region holds keeps its value when placed, so
// what undo brings back where a region moved to would be lost.
func (s *Sheet) emptyMoved(st regionState) []loc {
	var changed []loc
	for _, r := range s.regions.list {
		i := slices.IndexFunc(st.list, func(x Region) bool { return nameKey(x.Name) == nameKey(r.Name) })
		if me := s.regionMeta[nameKey(r.Name)]; me != nil && (i < 0 || st.list[i] != r) {
			changed = append(changed, s.emptyRegion(me)...)
		}
	}
	return changed
}
