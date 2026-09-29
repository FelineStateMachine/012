package notebook

import (
	"fmt"
	"slices"
	"strings"
)

// The cells' dependency graph: a cell reading $sales depends on the cell
// named sales. A name belongs to the first code cell giving it; a later
// one giving the same name can't run until it's renamed. Cells run in
// the order asked, each after the cells it reads among those running,
// and a cell reading itself, directly or through others, doesn't run.

// Names maps each name to the index of the first code cell giving it.
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

// Reads returns the indices of the cells cell i reads, in the order it
// names them.
func Reads(cells []Cell, names map[string]int, i int) []int {
	var out []int
	for _, n := range Refs(cells[i].Pipeline()) {
		if j, ok := names[n]; ok && j != i && !slices.Contains(out, j) {
			out = append(out, j)
		}
	}
	return out
}

// Order returns the code cells of want, each after the cells it reads
// among them, in want's order otherwise, and the cells left out for
// reading themselves, directly or through others.
func Order(cells []Cell, want []int) (order, cycle []int) {
	names := Names(cells)
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
			ready := !slices.ContainsFunc(Reads(cells, names, i), func(j int) bool { return in[j] && !done[j] })
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
	names := Names(cells)
	want := map[int]bool{i: true}
	for changed := true; changed; {
		changed = false
		for j := range cells {
			if !want[j] && cells[j].Kind == Code && slices.ContainsFunc(Reads(cells, names, j), func(k int) bool { return want[k] }) {
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

// Inputs returns the cells cell i reads, directly or through others,
// that have no output, in the notebook's order: what has to run before
// it can.
func Inputs(cells []Cell, i int, output func(id int) *Output) []int {
	names := Names(cells)
	need := map[int]bool{}
	var visit func(j int)
	visit = func(j int) {
		for _, k := range Reads(cells, names, j) {
			if need[k] || k == i {
				continue
			}
			if o := output(cells[k].ID); o == nil || o.NUON == nil {
				need[k] = true
				visit(k)
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
	names := Names(cells)
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
		for _, name := range Refs(c.Pipeline()) {
			j, ok := names[name]
			seq := 0
			if ok {
				if d := output(cells[j].ID); d != nil {
					seq = d.Seq
				}
				s = s || stale[cells[j].ID]
			}
			if read, had := o.Reads[name]; had || ok {
				s = s || read != seq
			}
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
	names := Names(cells)
	for _, c := range cells {
		o := outputs[c.ID]
		if o == nil {
			continue
		}
		o.Source, o.Reads = c.Source, map[string]int{}
		for _, name := range Refs(c.Pipeline()) {
			if j, ok := names[name]; ok {
				if d := outputs[cells[j].ID]; d != nil {
					o.Reads[name] = d.Seq
				} else {
					o.Reads[name] = 0
				}
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
