package sheet

import (
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Tracing, as Excel's Trace Precedents and Trace Dependents: which cells
// a formula reads, and which formulas read a cell, on any sheet. Only
// direct links are followed; tracing again from a found cell goes a level
// further. The dependency indexes recalculation keeps answer both ways,
// so tracing costs what it finds, not the sheet: a cell's precedents are
// its formula's references, and its dependents come from the indexes by
// cell, by range and by name (tracedeps.go).

// Target is a traced cell or range and the sheet it is on.
type Target struct {
	Sheet *Sheet
	Range Rect
}

// LinkKind is how a traced cell is linked to the one traced.
type LinkKind uint8

const (
	// LinkRef is a reference or range the formula writes out, or a
	// formula reading the cell through one.
	LinkRef LinkKind = iota
	// LinkName is a named range the formula uses: Link.Name is its name.
	LinkName
	// LinkRegion is a region's table the formula names, nu.sales:
	// Link.Name is how the formula names it.
	LinkRegion
	// LinkSpill is the array formula whose values the cell shows, among
	// precedents, or the cells a formula spills into, among dependents.
	LinkSpill
	// LinkFile is the linked file a region's cells come from: Link.Name
	// is its path, and Target the region's cells.
	LinkFile
	// LinkTable is a table's cells a formula reads by a structured
	// reference or the table's name alone: Link.Name is the reference as
	// written, Sales[Amount].
	LinkTable
	// LinkOutput is the notebook cell whose output a region shows:
	// Link.Name is the cell's name, Target.Sheet the notebook tab and
	// Target.Range.From.Row the cell's index there.
	LinkOutput
)

// Link is a precedent or dependent: where it is, and how it's linked.
type Link struct {
	Target
	Kind LinkKind
	Name string // see LinkKind
}

// OnGrid reports whether the link is cells of a sheet's grid rather than
// a notebook cell.
func (l Link) OnGrid() bool { return !l.Sheet.IsNotebook() }

// Precedents returns the cells and ranges the formula at a reads, in the
// order the formula mentions them; see PrecedentLinks.
func (s *Sheet) Precedents(a Addr) []Target { return targetsOf(s.PrecedentLinks(a)) }

// Dependents returns the formula cells that read a directly; see
// DependentLinks.
func (s *Sheet) Dependents(a Addr) []Target {
	links, _ := s.DependentLinks(a, 0)
	return targetsOf(links)
}

func targetsOf(links []Link) []Target {
	var out []Target
	for _, l := range links {
		if l.OnGrid() {
			out = append(out, l.Target)
		}
	}
	return out
}

// PrecedentLinks returns what the cell at a comes from. For a formula,
// the cells and ranges it reads in the order it mentions them, named
// ranges and regions as their cells, with repeats dropped and references
// to sheets that don't exist left out. For a cell an array spilled into,
// the formula that spilled it; for a region's cell, the linked file or
// notebook cell the region shows.
func (s *Sheet) PrecedentLinks(a Addr) []Link {
	var out []Link
	add := func(l Link) {
		if l.Sheet != nil && !slices.ContainsFunc(out, func(o Link) bool { return o.Target == l.Target && o.Kind == l.Kind }) {
			out = append(out, l)
		}
	}
	if c, _, _ := s.cells.peek(a); c != nil && c.IsFormula() {
		s.walkPrecedents(c.expr, a, add)
	}
	if anchor, ok := s.SpillAnchor(a); ok {
		add(Link{Target: Target{s, Rect{From: anchor, To: anchor}}, Kind: LinkSpill})
	}
	if r, ok := s.RegionAt(a); ok {
		add(s.regionSource(r))
	}
	return out
}

// walkPrecedents calls add with each reference, range, name and table
// in n, the formula at a, in the order they're written.
func (s *Sheet) walkPrecedents(n Node, a Addr, add func(Link)) {
	w := s.wb
	switch n := n.(type) {
	case formula.Ref:
		add(Link{Target: Target{w.resolve(s, n.Sheet), Rect{From: n.Addr, To: n.Addr}}})
	case formula.Range:
		add(Link{Target: Target{w.resolve(s, n.Sheet), n.Rect}})
	case formula.Name:
		if nm, ok := w.names[nameKey(n.Name)]; ok {
			if !nm.Gone() {
				add(Link{Target: Target{nm.Sheet, nm.Range}, Kind: LinkName, Name: nm.Name})
			}
			return
		}
		if _, found := w.findTable(nameKey(n.Name)); found {
			s.tableLink(a, formula.TableRef{Table: n.Name}, n.Name, add)
			return
		}
		if t, r, ok := w.regionName(nameKey(n.Name)); ok && r != (Rect{}) {
			add(Link{Target: Target{t, r}, Kind: LinkRegion, Name: s.regionSpelling(n.Name)})
		}
	case formula.TableRef:
		s.tableLink(a, n, n.String(), add)
	default:
		formula.EachChild(n, func(k Node) { s.walkPrecedents(k, a, add) })
	}
}

// tableLink adds the cells the structured reference t, written as name
// in the formula at a, reads, when it reads any.
func (s *Sheet) tableLink(a Addr, t formula.TableRef, name string, add func(Link)) {
	var r Target
	switch n := s.bindTable(a, t, 0).(type) {
	case formula.Ref:
		r = Target{s.wb.resolve(s, n.Sheet), Rect{From: n.Addr, To: n.Addr}}
	case formula.Range:
		r = Target{s.wb.resolve(s, n.Sheet), n.Rect}
	default:
		return
	}
	add(Link{Target: r, Kind: LinkTable, Name: name})
}

// regionSpelling is how formulas name the region a name like NU.SALES
// finds: nu. and the region's own name.
func (s *Sheet) regionSpelling(name string) string {
	if _, r, ok := s.wb.Region(name[len(regionPrefix):]); ok {
		return r.FormulaName()
	}
	return strings.ToLower(name)
}

// regionSource is where a region's cells come from: its linked file, or
// the notebook cell whose output it shows (on the notebook's tab, when
// there is one; otherwise the region itself).
func (s *Sheet) regionSource(r Region) Link {
	area := Target{s, s.covered(r)}
	if r.Linked() {
		return Link{Target: area, Kind: LinkFile, Name: r.File.Path}
	}
	for _, nb := range s.wb.sheets {
		for i, c := range nb.regions.cells {
			if strings.EqualFold(c.Name(), r.Name) {
				return Link{Target: Target{nb, Rect{From: Addr{Row: i}, To: Addr{Row: i}}}, Kind: LinkOutput, Name: c.Name()}
			}
		}
	}
	return Link{Target: area, Kind: LinkOutput, Name: r.Name}
}
