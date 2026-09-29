package sheet

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/formula"
)

// A Workbook is an ordered list of sheets, as a Google Sheets
// spreadsheet: each sheet has its own cells, column widths, frozen panes,
// filter and charts, while named ranges, the undo history and
// recalculation are shared, so a formula on one sheet can read another
// (=Sheet2!A1, ='Q3 plan'!B2:C9) and one undo step can span sheets.
//
// Formulas refer to other sheets by name. Renaming a sheet rewrites the
// formulas that name it; deleting one leaves them as written, showing
// #REF! ("Unresolved sheet name"), until a sheet of that name exists
// again, as in Sheets.
type Workbook struct {
	sheets []*Sheet
	byKey  map[string]*Sheet // live sheets by sheetKey of their name

	// names are the named ranges by upper-case name (see names.go), and
	// nameUsers the formula cells that mention each name, defined or not,
	// so defining a name recalculates the formulas waiting for it.
	names     map[string]Name
	nameUsers map[string]map[loc]struct{}

	// crossUsers are the formulas that reference a sheet by name. Like
	// range users, they are scanned when a cell changes, but only for
	// cells on sheets some formula names (crossKeys counts references by
	// sheetKey), so sheets nobody reads by name cost nothing.
	crossUsers map[loc]struct{}
	crossKeys  map[string]int

	// Circular is set when the last recalculation found a cycle.
	Circular bool

	remote RemoteSource // answers JEV functions, see remote.go
	trace  any          // the owner's telemetry trace, see observe.go
	// waiting are the formulas that were shown Loading… by question key,
	// so an answer recalculates only them; evaluating is the formula
	// being evaluated, which a question is asked for (see remote.go).
	waiting    map[string]map[loc]struct{}
	evaluating loc
	// depth counts the cells and operators being evaluated, nested; see
	// evaluate.go.
	depth  int
	hist   history // undo and redo, see history.go
	active int     // the sheet last shown, saved in the file
	settings

	macros      []Macro // see macros.go
	macroOrigin string

	nb notebookState // notebooks' outputs: notebook.go

	// structural is set when sheets were added, deleted or renamed during
	// the open change, which then recalculates everything: references by
	// sheet name may now resolve differently.
	structural bool

	// gen counts recalculations, so what is worked out from values
	// (the looks of rules) is kept until the next one.
	gen uint64

	// pivotDepth counts pivots refreshed because a pivot they read
	// changed, to stop a loop; see pivotlayout.go.
	pivotDepth int
	// spillWork is the arrays an evaluation pass computed, to spill once
	// it's done; see spill.go.
	spillWork spillWork
	// settling is set while regions write their cells; see regionwrite.go.
	settling bool
}

// loc is a cell on a particular sheet.
type loc struct {
	s *Sheet
	a Addr
}

// NewBook returns a workbook with one empty sheet, Sheet1.
func NewBook() *Workbook {
	w := emptyBook()
	w.insert(w.newSheet("Sheet1"), 0)
	return w
}

// emptyBook returns a workbook without sheets, for loaders to fill. A
// workbook given to anyone else always has at least one sheet.
func emptyBook() *Workbook {
	return &Workbook{
		byKey:      map[string]*Sheet{},
		names:      map[string]Name{},
		nameUsers:  map[string]map[loc]struct{}{},
		crossUsers: map[loc]struct{}{},
		crossKeys:  map[string]int{},
	}
}

// newSheet makes an empty sheet belonging to w, not yet in its list.
func (w *Workbook) newSheet(name string) *Sheet {
	return &Sheet{
		wb:         w,
		name:       name,
		cells:      newCellStore(),
		widths:     make(map[int]int),
		dependents: make(map[Addr]map[Addr]struct{}),
		volatile:   make(map[Addr]struct{}),
	}
}

// Sheets returns the sheets in tab order.
func (w *Workbook) Sheets() []*Sheet { return slices.Clone(w.sheets) }

// Len returns the number of sheets.
func (w *Workbook) Len() int { return len(w.sheets) }

// Sheet returns the sheet at index i in tab order.
func (w *Workbook) Sheet(i int) *Sheet { return w.sheets[i] }

// Index returns s's position in tab order, or -1 if it was deleted.
func (w *Workbook) Index(s *Sheet) int { return slices.Index(w.sheets, s) }

// Lookup finds a sheet by name, ignoring case.
func (w *Workbook) Lookup(name string) *Sheet { return w.byKey[formula.SheetKey(name)] }

// Active returns the index of the sheet last shown, as saved in the file.
func (w *Workbook) Active() int { return clampInt(w.active, 0, len(w.sheets)-1) }

// SetActive records which sheet is shown, for the file. It isn't an edit.
func (w *Workbook) SetActive(s *Sheet) {
	if i := w.Index(s); i >= 0 {
		w.active = i
	}
}

// Name returns the sheet's name.
func (s *Sheet) Name() string { return s.name }

// Book returns the workbook the sheet belongs to.
func (s *Sheet) Book() *Workbook { return s.wb }

// Live reports whether the sheet is in its workbook, i.e. not deleted.
func (s *Sheet) Live() bool { return s.live }

// maxSheetName is Excel's limit on a sheet name, kept so every workbook
// can be downloaded as .xlsx with its names intact. Sheets allows 100.
const maxSheetName = 31

// ValidSheetName checks a name for a sheet, following Excel's rules (a
// superset of what Sheets accepts is fine to read, but names written by
// 012 must open in Excel too).
func ValidSheetName(name string) error {
	switch {
	case strings.TrimSpace(name) == "":
		return errors.New("Enter a sheet name")
	case len([]rune(name)) > maxSheetName:
		return fmt.Errorf("A sheet name can be at most %d characters", maxSheetName)
	case strings.ContainsAny(name, `:\/?*[]`):
		return errors.New(`A sheet name can't contain : \ / ? * [ or ]`)
	case strings.HasPrefix(name, "'") || strings.HasSuffix(name, "'"):
		return errors.New("A sheet name can't start or end with '")
	case strings.ContainsFunc(name, func(r rune) bool { return r < ' ' || r == 0x7f }):
		return errors.New("A sheet name can't contain control characters")
	}
	return nil
}

// nextSheetName is the first free name among Sheet1, Sheet2, ... after
// the sheet count, as Sheets numbers new sheets.
func (w *Workbook) nextSheetName() string {
	for n := len(w.sheets) + 1; ; n++ {
		if name := fmt.Sprintf("Sheet%d", n); w.Lookup(name) == nil {
			return name
		}
	}
}

// freeName returns base, or base with a number added ("Copy of Sheet1 2")
// until no sheet has the name, within the length limit.
func (w *Workbook) freeName(base string) string { return freeIn(base, w) }

// checkName validates a new name for s (nil for a new sheet).
func (w *Workbook) checkName(s *Sheet, name string) error {
	if err := ValidSheetName(name); err != nil {
		return err
	}
	if other := w.Lookup(name); other != nil && other != s {
		return fmt.Errorf("There's already a sheet named %s", other.name)
	}
	return nil
}

// AddSheet inserts a new, empty sheet at index at (clamped to the ends),
// as one undo step. An empty name picks the next SheetN.
func (w *Workbook) AddSheet(name string, at int) (*Sheet, error) {
	if name == "" {
		name = w.nextSheetName()
	}
	if err := w.checkName(nil, name); err != nil {
		return nil, err
	}
	s := w.newSheet(name)
	w.change(s, "insert sheet "+name, Rect{}, func() {
		w.recordSheets()
		w.insert(s, at)
	})
	return s, nil
}

// DuplicateSheet copies s, cells, widths, heights, frozen panes, filter,
// merges and charts, to a new sheet right after it named "Copy of ...", as Sheets'
// Duplicate. Formulas are copied as written, so references without a
// sheet name read the copy's own cells.
func (w *Workbook) DuplicateSheet(s *Sheet) (*Sheet, error) {
	if !s.live {
		return nil, errors.New("That sheet was deleted")
	}
	cp := w.newSheet(w.freeName("Copy of " + s.name))
	for a, c := range s.cells.all() {
		if _, ok := s.RegionAt(a); ok && c.spilled {
			c = frozen(c) // a region's table, as values: the copy has no regions
		}
		c := c.clone()
		c.setExpr(c.expr) // fresh reference slices, shared with nothing
		cp.place(a, c)
	}
	for c, width := range s.widths {
		cp.widths[c] = width
	}
	cp.heights = maps.Clone(s.heights)
	cp.lines = lineFormats{cols: maps.Clone(s.lines.cols), rows: maps.Clone(s.lines.rows), sheet: s.lines.sheet}
	cp.view = s.view
	cp.view.filter = s.view.filter.clone()
	cp.charts = slices.Clone(s.charts)
	cp.rules = s.rules // never changed in place
	cp.pivot = pivotState{def: s.pivot.def.clone(), stale: s.pivot.def != nil, out: s.pivot.out, fit: s.pivot.fit}
	w.change(cp, "duplicate "+s.name, Rect{}, func() {
		w.recordSheets()
		w.insert(cp, w.Index(s)+1)
	})
	return cp, nil
}

// DeleteSheet removes s, as one undo step. Formulas on other sheets that
// refer to it show #REF! until it's restored or another sheet takes its
// name. The last sheet can't be deleted.
func (w *Workbook) DeleteSheet(s *Sheet) error {
	i := w.Index(s)
	switch {
	case i < 0:
		return errors.New("That sheet was already deleted")
	case len(w.sheets) == 1:
		return errors.New("A spreadsheet needs at least one sheet")
	case !s.tabHidden && w.visibleCount() == 1:
		return errLastVisible
	}
	w.change(s, "delete sheet "+s.name, Rect{}, func() {
		w.recordSheets()
		w.remove(s)
	})
	return nil
}

// RenameSheet renames s, as one undo step, rewriting every formula that
// refers to it by its old name.
func (w *Workbook) RenameSheet(s *Sheet, name string) error {
	if !s.live {
		return errors.New("That sheet was deleted")
	}
	if err := w.checkName(s, name); err != nil {
		return err
	}
	if name == s.name {
		return nil
	}
	old := formula.SheetKey(s.name)
	w.change(s, "rename "+s.name+" to "+name, Rect{}, func() {
		w.recordSheets()
		delete(w.byKey, old)
		s.name = name
		w.byKey[formula.SheetKey(name)] = s
		if old != formula.SheetKey(name) {
			w.renameRefs(old, name)
		}
		w.renamePivots(old, name)
		w.renameRules(old, name)
	})
	return nil
}

// renameRefs rewrites references written with the sheet key old to name.
func (w *Workbook) renameRefs(old, name string) {
	rw := formula.Rewriter{
		Ref: func(n formula.Ref) Node {
			if n.Sheet != "" && formula.SheetKey(n.Sheet) == old {
				n.Sheet = name
			}
			return n
		},
		Range: func(n formula.Range) Node {
			if n.Sheet != "" && formula.SheetKey(n.Sheet) == old {
				n.Sheet = name
			}
			return n
		},
	}
	for _, l := range w.crossList() {
		if c := l.s.cells.get(l.a); c.readsSheet(old) {
			l.s.place(l.a, c.rewritten(rw))
		}
	}
}

// MoveSheet moves s to index to in tab order, as one undo step.
func (w *Workbook) MoveSheet(s *Sheet, to int) {
	i := w.Index(s)
	to = clampInt(to, 0, len(w.sheets)-1)
	if i < 0 || i == to {
		return
	}
	w.change(s, "move sheet "+s.name, Rect{}, func() {
		w.recordSheets()
		order := slices.Delete(slices.Clone(w.sheets), i, i+1)
		w.sheets = slices.Insert(order, to, s)
	})
}

// insert puts s into the list at index at and indexes its formulas.
func (w *Workbook) insert(s *Sheet, at int) {
	at = clampInt(at, 0, len(w.sheets))
	w.sheets = slices.Insert(slices.Clone(w.sheets), at, s)
	w.attach(s)
}

// remove takes s out of the list and its formulas out of the indexes.
func (w *Workbook) remove(s *Sheet) {
	if i := w.Index(s); i >= 0 {
		w.sheets = slices.Delete(slices.Clone(w.sheets), i, i+1)
		w.detach(s)
	}
}

// attach makes s live: findable by name, its formulas indexed for
// recalculation across sheets.
func (w *Workbook) attach(s *Sheet) {
	if s.live {
		return
	}
	s.live = true
	w.byKey[formula.SheetKey(s.name)] = s
	for a, c := range s.cells.richCells() {
		w.index(loc{s, a}, c)
	}
	w.structural = true
}

// detach is the inverse of attach, for a deleted sheet. The sheet keeps
// its cells, so undo can bring it back as it was.
func (w *Workbook) detach(s *Sheet) {
	if !s.live {
		return
	}
	s.live = false
	if w.byKey[formula.SheetKey(s.name)] == s {
		delete(w.byKey, formula.SheetKey(s.name))
	}
	for a, c := range s.cells.richCells() {
		w.unindex(loc{s, a}, c)
	}
	w.structural = true
}

// index adds a live formula's workbook-wide dependencies: the names it
// uses and its references to other sheets.
func (w *Workbook) index(l loc, c *Cell) {
	for _, k := range c.names {
		if w.nameUsers[k] == nil {
			w.nameUsers[k] = make(map[loc]struct{})
		}
		w.nameUsers[k][l] = struct{}{}
	}
	if len(c.xrefs) > 0 {
		w.crossUsers[l] = struct{}{}
	}
	for _, x := range c.xrefs {
		w.crossKeys[x.key]++
	}
}

func (w *Workbook) unindex(l loc, c *Cell) {
	for _, k := range c.names {
		delete(w.nameUsers[k], l)
		if len(w.nameUsers[k]) == 0 {
			delete(w.nameUsers, k)
		}
	}
	delete(w.crossUsers, l)
	for _, x := range c.xrefs {
		if w.crossKeys[x.key]--; w.crossKeys[x.key] <= 0 {
			delete(w.crossKeys, x.key)
		}
	}
}

// crossList returns the cross-sheet formulas, so callers may change them
// while going through the list.
func (w *Workbook) crossList() []loc {
	out := make([]loc, 0, len(w.crossUsers))
	for l := range w.crossUsers {
		out = append(out, l)
	}
	return out
}

// resolve finds the sheet a reference written with sheet points at, from
// a formula on s: s itself for "", nil when no sheet has the name.
func (w *Workbook) resolve(s *Sheet, sheet string) *Sheet {
	if sheet == "" {
		return s
	}
	return w.byKey[formula.SheetKey(sheet)]
}

// onThis reports, for a formula on from, whether a reference written with
// a sheet name ("" for none) points at s.
func (s *Sheet) onThis(from *Sheet) func(string) bool {
	return func(sheet string) bool { return s.wb.resolve(from, sheet) == s }
}

// xref is a reference to a range on a sheet named in the formula.
type xref struct {
	key string // sheetKey of the name as written
	r   Rect
}

// readsSheet reports whether the cell's formula names the sheet with key k.
func (c *Cell) readsSheet(k string) bool {
	if c == nil {
		return false
	}
	for _, x := range c.xrefs {
		if x.key == k {
			return true
		}
	}
	return false
}

// Batch runs fn as a single undo step across sheets, as Sheet.Batch; the
// step is shown on c.Sheet when undone.
func (w *Workbook) Batch(c Change, fn func() error) error {
	var err error
	w.change(c.Sheet, c.Label, c.Focus, func() { err = fn() })
	return err
}
