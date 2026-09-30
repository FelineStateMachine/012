package sheet

import (
	"bytes"
	"cmp"
	"errors"
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Proposals: a change made on a copy of a workbook, to be made on the
// workbook itself later, whole or cell by cell, or not at all. An
// agent's suggestion in live mode is one (docs/agents/live.md): the
// person sees the cells it would change and accepts some or all of
// them, and what's accepted is one step, its author the workbook's
// author when it's applied, so undo and the others' marks follow as
// for any step.
//
// A change that does more than set cells (inserting rows, adding a
// sheet or a chart, a filter) is made whole only, by running it again
// on the workbook; a change of cells alone is applied as the cells it
// made, so what's accepted is what was shown.

// Proposal is a change made on a copy of a workbook.
type Proposal struct {
	Label string
	// Cells are the cells whose contents, format or note the change
	// sets, by sheet in the copy's order, then row and column.
	Cells []Proposed
	// Other says what the change does besides setting cells, one
	// phrase each ("inserts or deletes rows or columns on Sheet1"); a
	// proposal with any is made whole only.
	Other []string
	// Sheets are the sheets, by name, whose lines, charts, rules,
	// filters or pivot the change changes; Book is set when it changes
	// the sheet list, the settings, the names or the macros.
	Sheets []string
	Book   bool
	// Remote is set when a formula it enters asks a hosted model (JEV),
	// which reaches the network once the change is made.
	Remote bool

	copy *Workbook
	fn   func(*Workbook) error
}

// Proposed is a cell a proposal sets: its sheet, where, its input
// before and after ("" for blank), and whether its format, style or
// note change too.
type Proposed struct {
	Sheet    string
	At       Addr
	Was, Now string
	Look     bool
}

// ErrWhole is returned when part of a proposal that must be made whole
// is picked.
var ErrWhole = errors.New("this change does more than set cells: it can only be accepted whole")

// Propose makes fn, labelled label, on a copy of w and says what it
// changed. w isn't changed; fn may be run again on it when the proposal
// is applied whole.
func Propose(w *Workbook, label string, fn func(*Workbook) error) (*Proposal, error) {
	var data bytes.Buffer
	if err := w.Write(&data); err != nil {
		return nil, err
	}
	s, err := Read(&data)
	if err != nil {
		return nil, err
	}
	c := s.Book()
	before := c.StateID()
	if err := c.Batch(Change{Label: label, Sheet: c.Sheet(c.Active())}, func() error { return fn(c) }); err != nil {
		return nil, err
	}
	p := &Proposal{Label: label, copy: c, fn: fn}
	if st := c.hist.top(); st != nil && st.id != before {
		p.collect(w, st)
	}
	return p, nil
}

// Empty reports whether the proposal changes nothing.
func (p *Proposal) Empty() bool { return len(p.Cells) == 0 && len(p.Other) == 0 }

// Whole reports whether the proposal can only be made whole.
func (p *Proposal) Whole() bool { return len(p.Other) > 0 }

// Result is the copy with the change made, to read what it would look
// like. It must not be changed.
func (p *Proposal) Result() *Workbook { return p.copy }

// collect notes what step st made on the copy, against w.
func (p *Proposal) collect(w *Workbook, st *step) {
	order := map[string]int{}
	for i, s := range p.copy.sheets {
		order[s.name] = i
	}
	for s, img := range st.cells {
		orig := w.Lookup(s.name)
		img.addrs(func(a Addr) bool {
			p.cell(orig, s, a)
			return true
		})
	}
	slices.SortFunc(p.Cells, func(x, y Proposed) int {
		return cmp.Or(cmp.Compare(order[x.Sheet], order[y.Sheet]), cmp.Compare(x.At.Row, y.At.Row), cmp.Compare(x.At.Col, y.At.Col))
	})
	p.others(st)
}

// cell notes a's change on the copy's sheet s, when it changed from
// what the workbook's sheet orig holds (nil: the sheet is new).
func (p *Proposal) cell(orig, s *Sheet, a Addr) {
	now := s.cells.get(a)
	if now.Spilled() || now != nil && now.derived {
		return // an array's or a pivot's: its formula is the change
	}
	var was *Cell
	if orig != nil {
		was = orig.cells.get(a)
	}
	in := func(c *Cell) string {
		if c == nil {
			return ""
		}
		return c.Input
	}
	look := func(c *Cell) (Format, Style, string) {
		if c == nil {
			return Format{}, Style{}, ""
		}
		return c.Format, c.Style, c.Note
	}
	wf, ws, wn := look(was)
	nf, ns, nn := look(now)
	lookChanged := wf != nf || ws != ns || wn != nn
	if in(was) == in(now) && !lookChanged {
		return
	}
	p.Cells = append(p.Cells, Proposed{Sheet: s.name, At: a, Was: in(was), Now: in(now), Look: lookChanged})
	if now != nil && now.expr != nil && callsRemote(now.expr) {
		p.Remote = true
	}
}

// others notes what the step changed besides cells.
func (p *Proposal) others(st *step) {
	sheets := map[*Sheet]bool{}
	add := func(s *Sheet) {
		if !sheets[s] {
			sheets[s] = true
			p.Sheets = append(p.Sheets, s.name)
		}
	}
	for s := range st.shifts {
		add(s)
		p.Other = append(p.Other, "inserts or deletes rows or columns on "+s.name)
	}
	for _, m := range []struct {
		what string
		on   map[*Sheet]bool
	}{{"charts", keysOf(st.charts)}, {"a pivot table", keysOf(st.pivots)}, {"rules", keysOf(st.rules)}, {"regions", keysOf(st.regions)}, {"the view", keysOf(st.views)}} {
		for s := range m.on {
			add(s)
			p.Other = append(p.Other, "changes "+m.what+" on "+s.name)
		}
	}
	lines := map[*Sheet]bool{}
	for k := range st.widths {
		lines[k.s] = true
	}
	for k := range st.heights {
		lines[k.s] = true
	}
	for k := range st.lines {
		lines[k.s] = true
	}
	for s := range lines {
		add(s)
		p.Other = append(p.Other, "changes columns' or rows' sizes or formats on "+s.name)
	}
	for _, b := range []struct {
		what string
		on   bool
	}{{"adds, deletes, renames or moves sheets", st.sheets != nil}, {"changes named ranges", len(st.names) > 0},
		{"changes the settings", st.settings != nil}, {"changes the macros", st.macros != nil}} {
		if b.on {
			p.Book = true
			p.Other = append(p.Other, b.what)
		}
	}
	slices.Sort(p.Other)
}

// callsRemote reports whether a formula calls a function a hosted
// model answers.
func callsRemote(n Node) bool {
	found := false
	var walk func(Node)
	walk = func(n Node) {
		if call, ok := n.(formula.Call); ok && !found {
			if f := funcOf(call); f != nil && f.Remote() {
				found = true
			}
		}
		formula.EachChild(n, walk)
	}
	walk(n)
	return found
}

// Apply makes the proposal on w as one step, in the name of w's author:
// the cells pick keeps (nil keeps them all), or the whole change run
// again on w when it does more than set cells.
func (p *Proposal) Apply(w *Workbook, pick func(i int) bool) error {
	if p.Whole() {
		for i := range p.Cells {
			if pick != nil && !pick(i) {
				return ErrWhole
			}
		}
		return w.Batch(Change{Label: p.Label, Sheet: w.Sheet(w.Active())}, func() error { return p.fn(w) })
	}
	var picked []Proposed
	for i, c := range p.Cells {
		if pick == nil || pick(i) {
			picked = append(picked, c)
		}
	}
	if len(picked) == 0 {
		return nil
	}
	first := w.Lookup(picked[0].Sheet)
	if first == nil {
		return errors.New(picked[0].Sheet + " is gone")
	}
	focus := Rect{From: picked[0].At, To: picked[0].At}
	for _, c := range picked[1:] {
		if c.Sheet == picked[0].Sheet {
			focus = union(focus, Rect{From: c.At, To: c.At})
		}
	}
	return w.Batch(Change{Label: p.Label, Sheet: first, Focus: focus}, func() error {
		for _, c := range picked {
			if err := p.place(w, c); err != nil {
				return err
			}
		}
		return nil
	})
}

// place sets c's cell on w as the copy holds it.
func (p *Proposal) place(w *Workbook, c Proposed) error {
	s := w.Lookup(c.Sheet)
	if s == nil {
		return errors.New(c.Sheet + " is gone")
	}
	r := Rect{From: c.At, To: c.At}
	if s.InPivot(r) {
		return ErrPivotEdit
	}
	if _, ok := s.SpillAnchor(c.At); ok && c.Now != "" {
		return ErrSpillEdit
	}
	src := p.copy.Lookup(c.Sheet).cells.get(c.At)
	if src == nil {
		if s.cells.has(c.At) {
			s.place(c.At, nil)
		}
		return nil
	}
	s.place(c.At, src.clone())
	return nil
}
