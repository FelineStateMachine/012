package sheet

import "slices"

// Column groups of a pivot: each combination of the column fields' values
// is a group, and so is each combination of the first few fields' values,
// an outer group. Outer groups order the columns when a field is sorted
// by a value's total, and show as subtotal columns after their last
// combination when column totals are on, as Sheets' totals of an outer
// column field.

// colGroup is a combination of the first len(labels) column fields'
// values: all of them for a column of results, fewer for an outer group.
type colGroup struct {
	id      int
	labels  []Value
	formats []Format
}

// colNode finds column groups by each column field's value in turn.
type colNode struct {
	kids  map[groupKey]*colNode
	group *colGroup
}

// colGroupsOf returns the ids of the column groups row counts in, outer
// groups first, or none without column fields.
func (c *pivotCalc) colGroupsOf(row int) []int {
	if len(c.p.Columns) == 0 || len(c.p.Values) == 0 {
		return nil
	}
	c.colIDs = c.colIDs[:0]
	n := &c.colRoot
	for k, g := range c.p.Columns {
		key := keyOf(c.d.value(Addr{Col: g.Col, Row: row}), true)
		kid := n.kids[key]
		if kid == nil {
			if n.kids == nil {
				n.kids = map[groupKey]*colNode{}
			}
			kid = &colNode{group: c.newColGroup(row, k+1)}
			n.kids[key] = kid
			if k == len(c.p.Columns)-1 {
				c.cols = append(c.cols, kid.group)
			}
		}
		n = kid
		c.colIDs = append(c.colIDs, n.group.id)
	}
	return c.colIDs
}

// newColGroup is the group of row's values in the first depth column
// fields.
func (c *pivotCalc) newColGroup(row, depth int) *colGroup {
	cg := &colGroup{id: c.nextCol}
	c.nextCol++
	for _, g := range c.p.Columns[:depth] {
		a := Addr{Col: g.Col, Row: row}
		cg.labels = append(cg.labels, c.d.value(a))
		cg.formats = append(cg.formats, c.d.format(a))
	}
	return cg
}

// outer returns the group of the first len(labels) column fields' values.
func (c *pivotCalc) outer(labels []Value) *colGroup {
	n := &c.colRoot
	for _, v := range labels {
		n = n.kids[keyOf(v, true)]
	}
	return n.group
}

// sortCols orders the column groups by each column field in turn; a field
// sorted by a value's total compares the totals of its outer groups, so
// each outer group's columns stay together.
func (c *pivotCalc) sortCols() {
	slices.SortStableFunc(c.cols, func(a, b *colGroup) int {
		for k, g := range c.p.Columns {
			d := c.order(g, a.labels[k:k+1], b.labels[k:k+1], func(i int) (Value, Value) {
				va, _ := c.result(c.root, c.outer(a.labels[:k+1]).id, i)
				vb, _ := c.result(c.root, c.outer(b.labels[:k+1]).id, i)
				return va, vb
			})
			if d != 0 {
				return d
			}
		}
		return 0
	})
}

// subtotals reports whether outer column groups show as subtotal columns:
// with column totals on and several column fields.
func (c *pivotCalc) subtotals() bool {
	return c.p.ColumnTotals && len(c.p.Columns) > 1
}

// columnOrder lists the column groups as the results show them: each
// combination in order, after each outer group's last one its subtotal
// when subtotals show, innermost first.
func (c *pivotCalc) columnOrder() []*colGroup {
	if !c.subtotals() {
		return slices.Clone(c.cols)
	}
	var out []*colGroup
	for i, cg := range c.cols {
		out = append(out, cg)
		for d := len(c.p.Columns) - 1; d >= 1; d-- {
			if i+1 < len(c.cols) && sameLabels(cg.labels[:d], c.cols[i+1].labels[:d]) {
				break
			}
			out = append(out, c.outer(cg.labels[:d]))
		}
	}
	return out
}

// totalName labels the total of a group, "East Total", as Sheets does.
func totalName(v Value, f Format) string {
	if v.Kind == Empty {
		return "Total"
	}
	return FormatText(v, f) + " Total"
}
