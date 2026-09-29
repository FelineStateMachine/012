package sheet

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Regions are blocks of cells whose values come from outside the
// engine: a nushell command's output on a notebook sheet, and in time a
// file followed as it grows. A region is an anchor (its label line, At)
// and a definition saying where its table comes from (Command, opaque to
// the engine); what fills it arrives as a RegionData the UI hands over
// (ShowRegion). Like a spill, its cells are derived: written by the
// engine outside the undo history, refused by Set, values to formulas,
// copies and exports, formatting kept, never saved. The definitions and
// the data shown are undo steps, so undoing a run shows the table it
// replaced; the file keeps the definitions only.
//
// On a notebook sheet regions stack in run order, a gap row between
// them: a new one goes below everything on the sheet, and a table that
// grows or shrinks inserts or deletes rows under it, so what's below
// moves as rows inserted by hand would move it.
//
// In formulas a region is a name: nu. and its own (=SUM(nu.r1)), which
// stands for its table, header row included.

// Region is a region's definition.
type Region struct {
	// Name identifies the region in the workbook, ignoring case: r1, or
	// a name the user gave. Commands read other regions as $name.
	Name string
	// Command is what fills the region: for a notebook, a nushell
	// pipeline.
	Command string
	// At is the label line's first cell; the table starts below it.
	At Addr
	// Rows and Cols are the table's size as last shown, header row
	// included, so a region not run yet keeps its place.
	Rows, Cols int
	// Deps are the regions the command reads, in the order it names
	// them.
	Deps []string
	// Input is the range the command reads as $in, qualified with its
	// sheet ("Sheet1!A1:C9"), or "".
	Input string
	// Sort orders the table's rows below its header, by columns counted
	// from the region's first.
	Sort []SortKey
}

// regionState is what a sheet keeps of its regions in the undo history:
// the definitions in run order and the data each shows, by name key.
// Both are replaced whole, never changed in place.
type regionState struct {
	notebook bool
	list     []Region
	data     map[string]*RegionData
}

// regionMeta is what isn't undone: what the region's label says it's
// doing, and the cells written last.
type regionMeta struct {
	status  string
	written Rect
	has     bool   // written holds something
	why     string // why the table isn't shown, or ""
}

// RegionData is a region's table: Rows by Cols values, row by row, the
// header row first, with the formats they come in (a file size's Size,
// a date's Date time) by index.
type RegionData struct {
	Rows, Cols int
	Values     []Value
	Formats    map[int]Format
	// Note says what was left out, e.g. rows past max-cells.
	Note string
}

// At returns the value at row r, column c and its format.
func (d *RegionData) At(r, c int) (Value, Format) {
	if d == nil || r < 0 || c < 0 || r >= d.Rows || c >= d.Cols {
		return Value{}, Format{}
	}
	i := r*d.Cols + c
	return d.Values[i], d.Formats[i]
}

// RegionDataOf copies the used range of src, from A1, as a region's
// table: how a table read by an importer becomes one.
func RegionDataOf(src *Sheet) *RegionData {
	used, ok := src.UsedRange()
	if !ok {
		return &RegionData{}
	}
	d := &RegionData{Rows: used.To.Row + 1, Cols: used.To.Col + 1}
	d.Values = make([]Value, d.Rows*d.Cols)
	for a := range src.cells.keysIn(Rect{To: used.To}) {
		i := a.Row*d.Cols + a.Col
		d.Values[i] = src.Value(a)
		if f := src.DisplayFormat(a); !f.IsZero() {
			if d.Formats == nil {
				d.Formats = map[int]Format{}
			}
			d.Formats[i] = f
		}
	}
	return d
}

// Errors of regions.
var (
	// ErrRegionEdit is what typing into a region's cell says.
	ErrRegionEdit = errors.New("That cell is part of a shell region: change its command, or Freeze it to edit its values")
	errNoRegion   = errors.New("There's no region by that name")
)

// regionPrefix starts a region's name in formulas.
const regionPrefix = "nu."

// FormulaName is how formulas name the region: nu.r1.
func (r Region) FormulaName() string { return regionPrefix + r.Name }

// reservedRegionNames are nushell's own variables.
var reservedRegionNames = []string{"in", "env", "nu", "it"}

// ValidRegionName checks that name can name a region: a nushell
// variable name of letters, digits and _, not starting with a digit.
func ValidRegionName(name string) error {
	switch {
	case name == "":
		return errors.New("Enter a name")
	case len(name) > 64:
		return errors.New("A region's name can be at most 64 characters")
	case isDigit(name[0]):
		return errors.New("A region's name can't start with a digit")
	case slices.Contains(reservedRegionNames, strings.ToLower(name)):
		return fmt.Errorf("$%s is nushell's own", name)
	}
	for i := range len(name) {
		if c := name[i]; !isLetter(c) && !isDigit(c) && c != '_' {
			return errors.New("A region's name can only have letters, digits and _")
		}
	}
	return nil
}

// Notebook reports whether the sheet is a notebook: its regions stack
// in run order.
func (s *Sheet) Notebook() bool { return s.regions.notebook }

// MakeNotebook turns an empty sheet into a notebook, as a loader does,
// without an undo step.
func (s *Sheet) MakeNotebook() { s.regions.notebook = true }

// AddNotebook adds a notebook sheet named name ("" for the next Shell
// N) after the active one, as one undo step.
func (w *Workbook) AddNotebook(name string, after *Sheet) (*Sheet, error) {
	if name == "" {
		name = w.NextNotebookName()
	}
	if err := w.checkName(nil, name); err != nil {
		return nil, err
	}
	s := w.newSheet(name)
	s.regions.notebook = true
	at := len(w.sheets)
	if i := w.Index(after); i >= 0 {
		at = i + 1
	}
	w.change(s, "insert sheet "+name, Rect{}, func() {
		w.recordSheets()
		w.insert(s, at)
	})
	return s, nil
}

// NextNotebookName is Shell 1, then 2 and so on.
func (w *Workbook) NextNotebookName() string {
	for n := 1; ; n++ {
		if name := "Shell " + strconv.Itoa(n); w.Lookup(name) == nil {
			return name
		}
	}
}

// HasRegions reports whether the sheet has regions, so what draws it can
// skip looking for them.
func (s *Sheet) HasRegions() bool { return len(s.regions.list) > 0 }

// Regions returns the sheet's regions in run order.
func (s *Sheet) Regions() []Region {
	out := slices.Clone(s.regions.list)
	for i := range out {
		out[i].Deps, out[i].Sort = slices.Clone(out[i].Deps), slices.Clone(out[i].Sort)
	}
	return out
}

// Region finds a region by name, ignoring case, on any sheet.
func (w *Workbook) Region(name string) (*Sheet, Region, bool) {
	k := nameKey(name)
	for _, s := range w.sheets {
		if i := s.regionIndex(k); i >= 0 {
			return s, s.Regions()[i], true
		}
	}
	return nil, Region{}, false
}

func (s *Sheet) regionIndex(k string) int {
	return slices.IndexFunc(s.regions.list, func(r Region) bool { return nameKey(r.Name) == k })
}

// NextRegionName is the first of r1, r2 and so on that no region or
// named range has.
func (w *Workbook) NextRegionName() string {
	for n := 1; ; n++ {
		name := "r" + strconv.Itoa(n)
		if w.checkRegionName(name) == nil {
			return name
		}
	}
}

// checkRegionName reports why name can't name a new region.
func (w *Workbook) checkRegionName(name string) error {
	if err := ValidRegionName(name); err != nil {
		return err
	}
	if _, r, ok := w.Region(name); ok {
		return fmt.Errorf("There's already a region named %s", r.Name)
	}
	if n, ok := w.LookupName(regionPrefix + name); ok {
		return fmt.Errorf("%s already names %s", n.Name, n.Ref())
	}
	return nil
}

// RegionShown reports whether the region has data to show: it has run
// since the file was opened.
func (s *Sheet) RegionShown(name string) bool {
	return s.regions.data[nameKey(name)] != nil
}

// RegionStatus is what the region's label says it's doing: "Running…",
// "not run", why its table can't be shown, or "".
func (s *Sheet) RegionStatus(name string) string {
	k := nameKey(name)
	if me := s.regionMeta[k]; me != nil {
		switch {
		case me.status != "":
			return me.status
		case me.why != "":
			return me.why
		}
	}
	switch d := s.regions.data[k]; {
	case d == nil:
		return "not run"
	case d.Rows == 0:
		return "no rows"
	}
	return ""
}

// RegionNote is what the data shown left out, or "".
func (s *Sheet) RegionNote(name string) string {
	if d := s.regions.data[nameKey(name)]; d != nil {
		return d.Note
	}
	return ""
}

// AddRegion adds a region to the sheet as one undo step. On a notebook
// its label goes below everything on the sheet, a row left free, and r.At
// is ignored.
func (s *Sheet) AddRegion(r Region) error {
	if err := s.wb.checkRegionName(r.Name); err != nil {
		return err
	}
	if err := s.wb.checkDeps(r.Name, r.Deps); err != nil {
		return err
	}
	if s.regions.notebook {
		r.At = Addr{Row: s.nextRegionRow()}
	}
	r.Deps, r.Sort = slices.Clone(r.Deps), slices.Clone(r.Sort)
	s.change("add "+r.Name, Rect{From: r.At, To: r.At}, func() {
		st := s.regionsCopy()
		st.list = append(st.list, r)
		s.putRegions(st)
	})
	return nil
}

// nextRegionRow is where a new region's label goes on a notebook: below
// the last cell and region, a row left free, or row 1 on an empty sheet.
func (s *Sheet) nextRegionRow() int {
	next := 0
	if used, ok := s.UsedRange(); ok {
		next = used.To.Row + 2
	}
	for _, r := range s.regions.list {
		next = max(next, r.At.Row+r.Rows+2)
	}
	return min(next, MaxRows-1)
}

// EditRegion changes a region's command, what it reads and its input,
// as one undo step. The table stays until the command runs again.
func (s *Sheet) EditRegion(name, command string, deps []string, input string) error {
	i := s.regionIndex(nameKey(name))
	if i < 0 {
		return errNoRegion
	}
	if err := s.wb.checkDeps(s.regions.list[i].Name, deps); err != nil {
		return err
	}
	s.change("edit "+s.regions.list[i].Name, Rect{From: s.regions.list[i].At, To: s.regions.list[i].At}, func() {
		st := s.regionsCopy()
		st.list[i].Command, st.list[i].Deps, st.list[i].Input = command, slices.Clone(deps), input
		s.putRegions(st)
	})
	return nil
}

// DeleteRegion removes a region and its cells, as one undo step.
func (s *Sheet) DeleteRegion(name string) error {
	k := nameKey(name)
	i := s.regionIndex(k)
	if i < 0 {
		return errNoRegion
	}
	r := s.regions.list[i]
	s.change("delete "+r.Name, s.regionArea(r), func() {
		st := s.regionsCopy()
		st.list = slices.Delete(st.list, i, i+1)
		delete(st.data, k)
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
		return errNoRegion
	}
	r := s.regions.list[i]
	table, ok := s.RegionTable(r.Name)
	s.change("freeze "+r.Name, s.regionArea(r), func() {
		if ok {
			for a := range s.cells.anyKeysIn(table) {
				if c := s.cells.get(a); c.Spilled() {
					s.place(a, frozen(c))
				}
			}
		}
		st := s.regionsCopy()
		st.list = slices.Delete(st.list, i, i+1)
		delete(st.data, k)
		s.putRegions(st)
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

// SortRegion sorts a region's table below its header by keys, columns of
// the sheet, as one undo step: the order is the region's, so the table
// stays sorted when it runs again.
func (s *Sheet) SortRegion(name string, keys []SortKey) error {
	i := s.regionIndex(nameKey(name))
	if i < 0 {
		return errNoRegion
	}
	r := s.regions.list[i]
	rel := make([]SortKey, len(keys))
	for j, key := range keys {
		rel[j] = SortKey{Col: key.Col - r.At.Col, Desc: key.Desc}
	}
	s.change("sort "+r.Name, s.regionArea(r), func() {
		st := s.regionsCopy()
		st.list[i].Sort = rel
		s.putRegions(st)
	})
	return nil
}

// ShowRegion shows d in the region, as one undo step: the table grows or
// shrinks, and on a notebook the rows under it move with it.
func (s *Sheet) ShowRegion(name string, d *RegionData) error {
	k := nameKey(name)
	i := s.regionIndex(k)
	if i < 0 {
		return errNoRegion
	}
	if d == nil {
		d = &RegionData{}
	}
	r := s.regions.list[i]
	s.change("run "+r.Name, s.regionArea(r), func() {
		if s.regions.notebook {
			s.fitRegion(k, d.Rows)
		}
		st := s.regionsCopy()
		i := s.regionIndex(k) // fitting may have moved it
		if i < 0 {
			return
		}
		st.list[i].Rows, st.list[i].Cols = d.Rows, d.Cols
		st.data[k] = d
		s.putRegions(st)
	})
	return nil
}

// fitRegion makes room under the region with key k for a table of rows
// rows: inserting rows below its table, or deleting those it no longer
// needs when nothing else is on them.
func (s *Sheet) fitRegion(k string, rows int) {
	r := s.regions.list[s.regionIndex(k)]
	end := r.At.Row + r.Rows // the table's last row
	switch {
	case rows > r.Rows:
		below := rowRect(end+1, MaxRows-1)
		for range s.cells.anyKeysIn(below) {
			_ = s.insert(true, end+1, rows-r.Rows) // rows pushed past the edge: the table is cut instead
			return
		}
	case rows < r.Rows:
		from := r.At.Row + rows + 1
		spare := rowRect(from, end)
		area := s.regionArea(r)
		for a := range s.cells.anyKeysIn(spare) {
			if !area.Contains(a) {
				return // something else is on those rows
			}
		}
		s.restructure(true, spanOf(from, -(end-from+1)))
	}
}

// regionsCopy is a copy of the sheet's regions to change and put back.
func (s *Sheet) regionsCopy() regionState {
	return regionState{notebook: s.regions.notebook, list: slices.Clone(s.regions.list), data: maps.Clone(s.regions.data)}
}

// putRegions replaces the sheet's regions, recording them for undo, and
// has their cells written again.
func (s *Sheet) putRegions(st regionState) {
	s.recordRegions()
	if st.data == nil {
		st.data = map[string]*RegionData{}
	}
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
	if a.notebook != b.notebook || len(a.list) != len(b.list) || !maps.Equal(a.data, b.data) {
		return false
	}
	for i := range a.list {
		x, y := a.list[i], b.list[i]
		if x.Name != y.Name || x.Command != y.Command || x.At != y.At || x.Rows != y.Rows || x.Cols != y.Cols ||
			x.Input != y.Input || !slices.Equal(x.Deps, y.Deps) || !slices.Equal(x.Sort, y.Sort) {
			return false
		}
	}
	return true
}

// boundRegion is a name in a formula that isn't a named range: a
// region's table (nu.r1), #REF! while it has none, or the name as
// written, which shows #NAME?.
func (s *Sheet) boundRegion(nn formula.Name) Node {
	t, r, ok := s.wb.regionName(nameKey(nn.Name))
	switch {
	case !ok:
		return nn
	case r == (Rect{}):
		return formula.RefErr{}
	}
	sheet := ""
	if t != s {
		sheet = t.name
	}
	const fixed = formula.AbsCol | formula.AbsRow
	return formula.Range{Rect: r, Abs: [2]formula.Abs{fixed, fixed}, Sheet: sheet}
}
