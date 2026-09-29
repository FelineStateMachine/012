package sheet

import (
	"slices"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/notebook"
)

// Notebook tabs: a sheet of the workbook that holds a notebook's cells
// rather than a grid (package notebook). The tab sits among the sheets,
// is named, moved, hidden and deleted as a sheet is, and its cells are
// part of the undo state (regionState, region.go), so adding, editing,
// moving and deleting cells undo as any change does. What running a cell
// left, its output, isn't: a run can't be taken back, as a linked file's
// rows can't. Outputs are kept by the cell's ID in the workbook, and
// saved with it up to the caps (notebookfile.go).

// notebookState is the workbook's side of its notebooks: the outputs,
// and the counters that make IDs and Seqs.
type notebookState struct {
	outputs map[int]*notebook.Output
	lastID  int
	lastSeq int
	// changed counts changes to outputs, which the modified flag
	// follows as it follows the undo history.
	changed int
	caps    notebook.Caps
	// notes say what opening the file did that the user should know:
	// the notebook sheets it converted.
	notes []string
}

// IsNotebook reports whether the sheet is a notebook tab.
func (s *Sheet) IsNotebook() bool { return s.regions.notebook }

// MakeNotebook turns an empty sheet into a notebook tab, without an undo
// step, as 012 nu does with a new file's sheet.
func (s *Sheet) MakeNotebook() { s.regions.notebook = true }

// AddNotebook adds a notebook tab named name ("" for Notebook, then
// Notebook 2 and so on) after the sheet after, as one undo step.
func (w *Workbook) AddNotebook(name string, after *Sheet) (*Sheet, error) {
	if name == "" {
		name = w.freeName("Notebook")
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
	w.change(s, "insert notebook "+name, Rect{}, func() {
		w.recordSheets()
		w.insert(s, at)
	})
	return s, nil
}

// Notebook is the workbook's first notebook tab, or nil.
func (w *Workbook) Notebook() *Sheet {
	for _, s := range w.sheets {
		if s.IsNotebook() {
			return s
		}
	}
	return nil
}

// NotebookCells returns a notebook tab's cells.
func (s *Sheet) NotebookCells() []notebook.Cell { return slices.Clone(s.regions.cells) }

// SetNotebookCells replaces a notebook tab's cells as one undo step
// described by label ("add cell"). Cells without an ID are given one.
func (s *Sheet) SetNotebookCells(label string, cells []notebook.Cell) {
	cells = slices.Clone(cells)
	for i := range cells {
		if cells[i].ID == 0 {
			cells[i].ID = s.wb.NewCellID()
		}
	}
	if slices.Equal(cells, s.regions.cells) {
		return
	}
	s.change(label, Rect{}, func() {
		st := s.regionsCopy()
		st.cells = cells
		s.putRegions(st)
	})
}

// NewCellID is an ID no cell of the workbook has had.
func (w *Workbook) NewCellID() int {
	w.nb.lastID++
	return w.nb.lastID
}

// Reactive reports whether the notebook runs the cells reading a cell
// again whenever it runs.
func (s *Sheet) Reactive() bool { return s.regions.reactive }

// SetReactive turns the notebook's reactive setting on or off, as one
// undo step.
func (s *Sheet) SetReactive(on bool) {
	if s.regions.reactive == on {
		return
	}
	s.change("reactive notebook", Rect{}, func() {
		st := s.regionsCopy()
		st.reactive = on
		s.putRegions(st)
	})
}

// Output is what the cell with the ID id left when it last ran, or nil.
func (w *Workbook) Output(id int) *notebook.Output { return w.nb.outputs[id] }

// SetOutput keeps o as what the cell with the ID id left, nil for
// nothing, giving it a Seq of its own. It isn't an undo step.
func (w *Workbook) SetOutput(id int, o *notebook.Output) {
	w.nb.changed++
	if o == nil {
		delete(w.nb.outputs, id)
		return
	}
	cp := *o
	w.nb.lastSeq++
	cp.Seq = w.nb.lastSeq
	if w.nb.outputs == nil {
		w.nb.outputs = map[int]*notebook.Output{}
	}
	w.nb.outputs[id] = &cp
}

// OutputsChanged counts changes to outputs, for the modified flag.
func (w *Workbook) OutputsChanged() int { return w.nb.changed }

// SetOutputCaps sets how much of the outputs a file keeps.
func (w *Workbook) SetOutputCaps(c notebook.Caps) { w.nb.caps = c }

// OutputCaps are how much of the outputs a file keeps.
func (w *Workbook) OutputCaps() notebook.Caps {
	if w.nb.caps == (notebook.Caps{}) {
		return notebook.DefaultCaps
	}
	return w.nb.caps
}

// LoadNotes say what opening the file changed that the user should
// know, such as notebook sheets converted.
func (w *Workbook) LoadNotes() []string { return slices.Clone(w.nb.notes) }

// cellNamed reports whether a notebook's cell is named name, ignoring
// case.
func (w *Workbook) cellNamed(name string) bool {
	for _, s := range w.sheets {
		for _, c := range s.regions.cells {
			if strings.EqualFold(c.Name(), name) {
				return true
			}
		}
	}
	return false
}

// hasNotebook reports whether the workbook has a notebook tab.
func (w *Workbook) hasNotebook() bool { return w.Notebook() != nil }

// nextNotebookCellName is a name for a cell no cell and no region has:
// cell1, cell2 and so on.
func (w *Workbook) nextNotebookCellName() string {
	for n := 1; ; n++ {
		name := "cell" + strconv.Itoa(n)
		if w.checkRegionName(name, false) == nil {
			return name
		}
	}
}

// FreeCellName is a name like base that no cell and no region of the
// workbook has: base, then base_2 and so on; cell1 and on for "".
func (w *Workbook) FreeCellName(base string) string {
	if base == "" || ValidRegionName(base) != nil {
		return w.nextNotebookCellName()
	}
	name := base
	for n := 2; w.checkRegionName(name, false) != nil; n++ {
		name = base + "_" + strconv.Itoa(n)
	}
	return name
}

// StaleOutputs names the outputs sent to sheets that are to be sent
// again: new, put back by undo, moved, or no longer blocked.
func (w *Workbook) StaleOutputs() []string {
	var out []string
	for _, s := range w.sheets {
		for _, r := range s.regions.list {
			if me := s.regionMeta[nameKey(r.Name)]; r.Output && (me == nil || me.stale || me.reread) {
				out = append(out, r.Name)
			}
		}
	}
	return out
}

// RenameRegion gives a sent output its cell's new name, as one undo
// step; it is sent again.
func (w *Workbook) RenameRegion(old, name string) error {
	s, r, ok := w.Region(old)
	if !ok {
		return ErrNoRegion
	}
	if err := w.checkRegionName(name, r.Output); err != nil && nameKey(old) != nameKey(name) {
		return err
	}
	k := nameKey(r.Name)
	s.change("rename "+r.Name, s.covered(r), func() {
		st := s.regionsCopy()
		st.list[s.regionIndex(k)].Name = name
		s.putRegions(st)
	})
	s.meta(nameKey(name)).stale = true
	return nil
}
