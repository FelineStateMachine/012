package sheet

import (
	"slices"

	"github.com/FelineStateMachine/012/internal/formula"
)

// Cycles are found from the formulas as written, as Sheets and Excel
// find them, before a recalculation evaluates anything: a formula is
// part of a cycle when what it references (cells, ranges, names, tables,
// regions, other sheets) leads back to it, whether or not evaluating it
// reads that far: an IF branch not taken, a range read that stops at an
// error, INDEX reading one cell of its range. Evaluation meeting a cell
// it is evaluating finds the cycles it walks (evaluate.go), but only
// those, so what the cells of a cycle it didn't walk showed depended on
// the order they were evaluated in. Every cell of a cycle is #REF!.
//
// Only the formulas to recalculate are looked at: a cycle through one of
// them runs through formulas that read it, which recalculate too.
//
// The graph's nodes are those formulas, each leading to the formulas it
// references. A range leads to the formulas in it through a segment tree
// over each column's formulas, whose nodes lead to their halves, so a
// range costs the logarithm of the formulas in its columns rather than
// their number, and a column of running totals over computed cells
// stays linear. Tarjan's algorithm finds the strongly connected
// components; the formulas of a component with more than one node, or
// that reference themselves, are a cycle's. The graph's buffers are kept
// for the next recalculation, up to maxCycleKeep formulas.

// maxCycleKeep is the most formulas whose graph's buffers are kept.
const maxCycleKeep = 1 << 16

// cycleGraph is the graph of the formulas to recalculate: formulas are
// nodes 0 to formulas-1, in sheet order and by column and row within a
// sheet, and segment nodes follow. The edges leaving node v are
// edges[start[v]:start[v+1]].
type cycleGraph struct {
	w        *Workbook
	formulas int32
	sheets   []cycleSheet
	keys     []uint64   // each sheet's formulas' keys, sorted
	cols     []cycleCol // each sheet's columns holding formulas, in order
	last     *cycleSheet
	start    []int32
	edges    []int32
	tarjan
}

// cycleSheet is a sheet's formulas in the graph: keys[from:to] (column in
// the high half, row in the low), which are nodes from to to, and its
// columns, cols[col0:col1].
type cycleSheet struct {
	s          *Sheet
	from, to   int32
	col0, col1 int32
}

// cycleCol is a column's formulas: keys[from:from+m], and the node of its
// segment tree's first inner node (the tree has m-1).
type cycleCol struct {
	col     int
	from, m int32
	seg     int32
}

func cycleKey(a Addr) uint64 { return uint64(a.Col)<<32 | uint64(uint32(a.Row)) }

func keyAddr(k uint64) Addr { return Addr{Col: int(k >> 32), Row: int(uint32(k))} }

// findCycles returns the formulas to recalculate, dirty in each sheet's
// calc, that are part of a cycle, or nil when none is.
func (w *Workbook) findCycles() map[loc]bool {
	g := w.cycles
	if g == nil {
		g = &cycleGraph{w: w}
	}
	var cycle map[loc]bool
	if g.collect() {
		g.link()
		cycle = g.components()
	}
	g.reset()
	w.cycles = nil
	if cap(g.keys) <= maxCycleKeep {
		w.cycles = g
	}
	return cycle
}

// reset empties the graph for the next recalculation, keeping its
// buffers.
func (g *cycleGraph) reset() {
	clear(g.sheets)
	g.sheets, g.keys, g.cols, g.start, g.edges = g.sheets[:0], g.keys[:0], g.cols[:0], g.start[:0], g.edges[:0]
	g.formulas, g.last = 0, nil
	g.tarjan = tarjan{index: g.index[:0], low: g.low[:0], on: g.on[:0], stack: g.stack[:0], path: g.path[:0], next: g.next[:0]}
}

// collect gathers the formulas to recalculate, reporting whether a
// cycle may run through them: not when there are none, or when each
// references only cells before it, row by row, as a column of running
// totals or a chain down a column does, since then no reference leads
// back.
func (g *cycleGraph) collect() bool {
	back := true
	for _, s := range g.w.sheets {
		from := len(g.keys)
		for a, st := range s.calc {
			if c := s.cells.richAt(a); st == dirty && c != nil && c.expr != nil && !c.derived && !c.spilled && readsCells(c) {
				g.keys = append(g.keys, cycleKey(a))
				back = back && readsBefore(c, a)
			}
		}
		if len(g.keys) > from {
			g.sheets = append(g.sheets, cycleSheet{s: s, from: int32(from), to: int32(len(g.keys))})
		}
	}
	g.formulas = int32(len(g.keys))
	if back {
		return false
	}
	for _, cs := range g.sheets {
		slices.Sort(g.keys[cs.from:cs.to])
	}
	seg := g.formulas
	for i := range g.sheets {
		cs := &g.sheets[i]
		cs.col0 = int32(len(g.cols))
		for j := cs.from; j < cs.to; j++ {
			col := int(g.keys[j] >> 32)
			if len(g.cols) == int(cs.col0) || g.cols[len(g.cols)-1].col != col {
				g.cols = append(g.cols, cycleCol{col: col, from: j})
			}
			g.cols[len(g.cols)-1].m++
		}
		cs.col1 = int32(len(g.cols))
		for c := cs.col0; c < cs.col1; c++ {
			g.cols[c].seg = seg
			seg += max(g.cols[c].m-1, 0)
		}
	}
	g.start = slices.Grow(g.start, int(seg)+1)[:seg+1]
	return true
}

// readsBefore reports whether the formula c, at a, references only
// cells of its own sheet before a, row by row.
func readsBefore(c *Cell, a Addr) bool {
	if len(c.xrefs) > 0 || len(c.names) > 0 {
		return false
	}
	for _, r := range c.refs {
		if !before(r, a) {
			return false
		}
	}
	for _, r := range c.ranges {
		if !before(r.To, a) {
			return false
		}
	}
	return true
}

// readsCells reports whether the formula c references anything.
func readsCells(c *Cell) bool {
	return len(c.refs) > 0 || len(c.ranges) > 0 || len(c.xrefs) > 0 || len(c.names) > 0
}

// link writes the edges: each formula's to what it references, then
// each segment node's to its halves.
func (g *cycleGraph) link() {
	for _, cs := range g.sheets {
		for j := cs.from; j < cs.to; j++ {
			g.start[j] = int32(len(g.edges))
			g.references(cs.s, keyAddr(g.keys[j]))
		}
	}
	v := g.formulas
	for _, col := range g.cols {
		for p := int32(1); p < col.m; p++ {
			g.start[v] = int32(len(g.edges))
			g.edges = append(g.edges, col.node(2*p), col.node(2*p+1))
			v++
		}
	}
	g.start[v] = int32(len(g.edges))
}

// node is the node at position p of the column's segment tree: an inner
// node below m, a formula from m on.
func (col cycleCol) node(p int32) int32 {
	if p >= col.m {
		return col.from + p - col.m
	}
	return col.seg + p - 1
}

// references adds the edges from the formula at a on s to the formulas
// it references. One naming a name, a table or a region is bound first,
// as evaluating it is, so a table's row (Sales[@Price]) leads to that
// row alone.
func (g *cycleGraph) references(s *Sheet, a Addr) {
	c := s.cells.richAt(a)
	if len(c.names) > 0 {
		formula.WalkRefs(s.bound(a, c),
			func(sheet string, r Addr) { g.cell(g.w.resolve(s, sheet), r) },
			func(sheet string, r Rect) { g.rect(g.w.resolve(s, sheet), r) })
		return
	}
	for _, r := range c.refs {
		g.cell(s, r)
	}
	for _, r := range c.ranges {
		g.rect(s, r)
	}
	for _, x := range c.xrefs {
		g.rect(g.w.byKey[x.key], x.r)
	}
}

// sheet is t's formulas in the graph, if it has any.
func (g *cycleGraph) sheet(t *Sheet) (*cycleSheet, bool) {
	if g.last != nil && g.last.s == t {
		return g.last, true
	}
	for i := range g.sheets {
		if g.sheets[i].s == t {
			g.last = &g.sheets[i]
			return g.last, true
		}
	}
	return nil, false
}

// cell adds an edge to the formula at a on t, if it is one.
func (g *cycleGraph) cell(t *Sheet, a Addr) {
	if cs, ok := g.sheet(t); ok {
		if i, found := slices.BinarySearch(g.keys[cs.from:cs.to], cycleKey(a)); found {
			g.edges = append(g.edges, cs.from+int32(i))
		}
	}
}

// rect adds edges to the formulas of r on t: in each of its columns, to
// the fewest segment nodes that cover them.
func (g *cycleGraph) rect(t *Sheet, r Rect) {
	cs, ok := g.sheet(t)
	switch {
	case !ok:
		return
	case r.From == r.To:
		g.cell(t, r.From)
		return
	}
	cols := g.cols[cs.col0:cs.col1]
	c0, _ := slices.BinarySearchFunc(cols, r.From.Col, func(c cycleCol, col int) int { return c.col - col })
	for _, col := range cols[c0:] {
		if col.col > r.To.Col {
			break
		}
		keys := g.keys[col.from : col.from+col.m]
		lo, _ := slices.BinarySearch(keys, cycleKey(Addr{Col: col.col, Row: r.From.Row}))
		hi, _ := slices.BinarySearch(keys, cycleKey(Addr{Col: col.col, Row: r.To.Row})+1)
		for lp, hp := int32(lo)+col.m, int32(hi)+col.m; lp < hp; lp, hp = lp/2, hp/2 {
			if lp&1 == 1 {
				g.edges = append(g.edges, col.node(lp))
				lp++
			}
			if hp&1 == 1 {
				hp--
				g.edges = append(g.edges, col.node(hp))
			}
		}
	}
}

// components runs Tarjan's algorithm from each formula, without
// recursion, and returns the formulas in cycles.
func (g *cycleGraph) components() map[loc]bool {
	n := len(g.start) - 1
	g.index = slices.Grow(g.index, n)[:n]
	g.low = slices.Grow(g.low, n)[:n]
	g.on = slices.Grow(g.on, n)[:n]
	clear(g.index)
	for v := range g.formulas {
		if g.index[v] == 0 {
			g.visit(v)
		}
	}
	return g.cycle
}

// tarjan is the state of Tarjan's algorithm: each node's index, from 1
// (0 while unvisited), and lowest index reached, the nodes on the stack,
// the path being followed, with the next edge of each, and the formulas
// found in cycles.
type tarjan struct {
	index, low  []int32
	on          []bool
	stack, path []int32
	next        []int32
	count       int32
	cycle       map[loc]bool
}

func (g *cycleGraph) push(v int32) {
	g.count++
	g.index[v], g.low[v] = g.count, g.count
	g.stack = append(g.stack, v)
	g.on[v] = true
	g.path = append(g.path, v)
	g.next = append(g.next, g.start[v])
}

func (g *cycleGraph) visit(root int32) {
	g.push(root)
	for len(g.path) > 0 {
		top := len(g.path) - 1
		v := g.path[top]
		if e := g.next[top]; e < g.start[v+1] {
			g.next[top]++
			switch u := g.edges[e]; {
			case g.index[u] == 0:
				g.push(u)
			case g.on[u]:
				g.low[v] = min(g.low[v], g.index[u])
			}
			continue
		}
		g.path, g.next = g.path[:top], g.next[:top]
		if top > 0 {
			p := g.path[top-1]
			g.low[p] = min(g.low[p], g.low[v])
		}
		if g.low[v] == g.index[v] {
			g.pop(v)
		}
	}
}

// pop takes the component rooted at v off the stack, keeping its
// formulas when it is a cycle.
func (g *cycleGraph) pop(v int32) {
	i := len(g.stack) - 1
	for g.stack[i] != v {
		i--
	}
	comp := g.stack[i:]
	g.stack = g.stack[:i]
	for _, u := range comp {
		g.on[u] = false
	}
	if len(comp) == 1 && !slices.Contains(g.edges[g.start[v]:g.start[v+1]], v) {
		return
	}
	if g.cycle == nil {
		g.cycle = map[loc]bool{}
	}
	for _, u := range comp {
		if u < g.formulas {
			g.cycle[g.loc(u)] = true
		}
	}
}

// loc is the cell of the formula node v.
func (g *cycleGraph) loc(v int32) loc {
	for _, cs := range g.sheets {
		if v < cs.to {
			return loc{cs.s, keyAddr(g.keys[v])}
		}
	}
	panic("sheet: no formula node")
}
