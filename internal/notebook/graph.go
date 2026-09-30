package notebook

import (
	"fmt"
	"slices"
	"strings"
)

// The cells' dependency graph: a cell reading $sales depends on the cell
// giving sales (Vars). A cell's name belongs to the first code cell
// giving it; a later one giving the same name can't run until it's
// renamed. Cells run in the order asked, each after the cells it reads
// among those running, and a cell reading itself, directly or through
// others, doesn't run.

// Names maps each cell's name to the index of the first code cell
// giving it.
func Names(cells []Cell) map[string]int {
	out := map[string]int{}
	for i, c := range cells {
		if n := c.Name(); n != "" {
			if _, dup := out[n]; !dup {
				out[n] = i
			}
		}
	}
	return out
}

// Vars maps each variable the cells give others to the index of the
// cell giving it: a cell's name to that cell (Names), and any other
// name a statement assigns to the first code cell assigning it.
func Vars(cells []Cell) map[string]int {
	out := Names(cells)
	for i, c := range cells {
		for _, n := range c.Parse().Assigned() {
			if _, ok := out[n]; !ok {
				out[n] = i
			}
		}
	}
	return out
}

// Taken says why cell i can't use its name: an earlier cell gives it
// (names ignore case there, as formulas' nu.name do), or "".
func Taken(cells []Cell, i int) string {
	name := cells[i].Name()
	if name == "" {
		return ""
	}
	for j, c := range cells[:i] {
		if strings.EqualFold(c.Name(), name) {
			return fmt.Sprintf("cell %d is already named %s: rename one", j+1, c.Name())
		}
	}
	return ""
}

// ref is a variable a cell reads from another: its name, and the index
// of the cell giving it.
type ref struct {
	name string
	cell int
}

// refs are the variables cell i reads from other cells, in the order it
// names them; vars is Vars(cells). A name only it assigns, read before
// it does, is none.
func refs(cells []Cell, vars map[string]int, i int) []ref {
	var out []ref
	for _, n := range cells[i].Parse().Refs() {
		if j, ok := vars[n]; ok && j != i {
			out = append(out, ref{n, j})
		}
	}
	return out
}

// Reads returns the indices of the cells cell i reads, in the order it
// names them; vars is Vars(cells).
func Reads(cells []Cell, vars map[string]int, i int) []int {
	var out []int
	for _, r := range refs(cells, vars, i) {
		if !slices.Contains(out, r.cell) {
			out = append(out, r.cell)
		}
	}
	return out
}

// Order returns the code cells of want, each after the cells it reads
// among them, in want's order otherwise, and the cells left out for
// reading themselves, directly or through others.
func Order(cells []Cell, want []int) (order, cycle []int) {
	vars := Vars(cells)
	in := map[int]bool{}
	for _, i := range want {
		if cells[i].Kind == Code {
			in[i] = true
		}
	}
	done := map[int]bool{}
	for len(order) < len(in) {
		progress := false
		for _, i := range want {
			if !in[i] || done[i] {
				continue
			}
			ready := !slices.ContainsFunc(Reads(cells, vars, i), func(j int) bool { return in[j] && !done[j] })
			if ready {
				order, done[i], progress = append(order, i), true, true
			}
		}
		if !progress {
			break
		}
	}
	for _, i := range want {
		if in[i] && !done[i] && !slices.Contains(cycle, i) {
			cycle = append(cycle, i)
		}
	}
	return order, cycle
}

// Dependents returns the cells that read cell i, directly or through
// others, in the notebook's order.
func Dependents(cells []Cell, i int) []int {
	vars := Vars(cells)
	want := map[int]bool{i: true}
	for changed := true; changed; {
		changed = false
		for j := range cells {
			if !want[j] && cells[j].Kind == Code && slices.ContainsFunc(Reads(cells, vars, j), func(k int) bool { return want[k] }) {
				want[j], changed = true, true
			}
		}
	}
	var out []int
	for j := range cells {
		if want[j] && j != i {
			out = append(out, j)
		}
	}
	return out
}

// Readable reports whether o, the output of cell c, holds the variable
// name: c's output when c is named so, else a variable it assigns,
// which a file doesn't keep.
func Readable(c Cell, name string, o *Output) bool {
	switch {
	case o == nil || o.NUON == nil:
		return false
	case c.Name() == name:
		return true
	}
	_, ok := o.Vars[name]
	return ok
}

// Inputs returns the cells cell i reads, directly or through others,
// whose outputs don't hold what it reads (Readable), in the notebook's
// order: what has to run before it can.
func Inputs(cells []Cell, i int, output func(id int) *Output) []int {
	vars := Vars(cells)
	need := map[int]bool{}
	var visit func(j int)
	visit = func(j int) {
		for _, r := range refs(cells, vars, j) {
			if need[r.cell] || r.cell == i {
				continue
			}
			if c := cells[r.cell]; !Readable(c, r.name, output(c.ID)) {
				need[r.cell] = true
				visit(r.cell)
			}
		}
	}
	visit(i)
	var out []int
	for j := range cells {
		if need[j] {
			out = append(out, j)
		}
	}
	return out
}

// Stale returns the cells, by ID, whose output may not be what running
// them now would give: their source changed since, or an output they
// read changed or is stale itself.
func Stale(cells []Cell, output func(id int) *Output) map[int]bool {
	vars := Vars(cells)
	all := make([]int, len(cells))
	for i := range all {
		all[i] = i
	}
	order, _ := Order(cells, all)
	stale := map[int]bool{}
	for _, i := range order {
		c := cells[i]
		o := output(c.ID)
		if o == nil {
			continue
		}
		s := o.Source != c.Source
		read := map[string]bool{}
		for _, r := range refs(cells, vars, i) {
			read[r.name] = true
			seq := 0
			if d := output(cells[r.cell].ID); d != nil {
				seq = d.Seq
			}
			s = s || stale[cells[r.cell].ID] || o.Reads[r.name] != seq
		}
		for name := range o.Reads {
			s = s || !read[name] // it read a cell it no longer does
		}
		if s {
			stale[c.ID] = true
		}
	}
	return stale
}

// Settle makes outputs read from a file what their cells' runs would
// have left: made from the source as it is, having read the outputs as
// they are, so none is stale until something changes.
func Settle(cells []Cell, outputs map[int]*Output) {
	vars := Vars(cells)
	for i, c := range cells {
		o := outputs[c.ID]
		if o == nil {
			continue
		}
		o.Source, o.Reads = c.Source, map[string]int{}
		for _, r := range refs(cells, vars, i) {
			o.Reads[r.name] = 0
			if d := outputs[cells[r.cell].ID]; d != nil {
				o.Reads[r.name] = d.Seq
			}
		}
	}
}

// Caps are how much of the outputs a file keeps, in bytes of NUON: at
// most Cell of one cell's, Total of all of them.
type Caps struct{ Cell, Total int }

// DefaultCaps keep 1 MB of a cell's output and 8 MB of a notebook's.
var DefaultCaps = Caps{Cell: 1 << 20, Total: 8 << 20}

// Kept returns the cells, by ID, whose outputs a file keeps within caps:
// in the notebook's order, each that fits in what's left.
func Kept(cells []Cell, output func(id int) *Output, caps Caps) map[int]bool {
	out := map[int]bool{}
	left := caps.Total
	for _, c := range cells {
		o := output(c.ID)
		if o == nil || o.NUON == nil {
			continue
		}
		if n := len(o.NUON); n <= caps.Cell && n <= left {
			out[c.ID] = true
			left -= n
		}
	}
	return out
}
