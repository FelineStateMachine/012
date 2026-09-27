package ui

import (
	"slices"
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
	"github.com/FelineStateMachine/012/internal/ui/overlay"
)

// What the pivot editor's keys do to the pivot, and how its fields read.

// fieldName is the source field a field line is about.
func (e *pivotEditor) fieldName(m *Model, p sheet.Pivot, it pivotItem) string {
	return m.book().FieldName(p, fieldCol(p, it))
}

func fieldCol(p sheet.Pivot, it pivotItem) int {
	switch it.sect {
	case sectRows:
		return p.Rows[it.i].Col
	case sectColumns:
		return p.Columns[it.i].Col
	case sectValues:
		return p.Values[it.i].Col
	}
	return p.Filters[it.i].Col
}

// groups returns the rows or the columns of p, for editing.
func groups(p *sheet.Pivot, sect int) *[]sheet.PivotGroup {
	if sect == sectColumns {
		return &p.Columns
	}
	return &p.Rows
}

// fieldDetail is the right side of a field line: a group's order, a
// value's summary as a chip to change, a filter's criteria.
func (e *pivotEditor) fieldDetail(m *Model, p sheet.Pivot, it pivotItem, sel bool) string {
	dim, chip := m.th.Muted, m.th.KeyChip
	if sel {
		dim, chip = m.th.MenuSelected, m.th.MenuSelected
	}
	switch it.sect {
	case sectValues:
		v := p.Values[it.i]
		text := chip.Render("‹ " + v.Summarize.Title() + " ›")
		if v.ShowAs != sheet.ShowValue {
			text += dim.Render(" " + v.ShowAs.Title())
		}
		return text
	case sectFilters:
		return dim.Render(criteriaText(p.Filters[it.i].Criteria))
	}
	g := (*groups(&p, it.sect))[it.i]
	return dim.Render(orderText(m, p, g))
}

// orderText says how a group is ordered: A→Z, or by a value's total.
func orderText(m *Model, p sheet.Pivot, g sheet.PivotGroup) string {
	if i := g.SortBy - 1; i >= 0 && i < len(p.Values) {
		arrow := "↑"
		if g.Desc {
			arrow = "↓"
		}
		return m.book().ValueTitle(p, p.Values[i]) + " " + arrow
	}
	return orderName(g.Desc)
}

// criteriaText sums up a filter: the values it hides, or its condition.
func criteriaText(cr sheet.Criteria) string {
	switch {
	case cr.Cond.Op != sheet.CondNone && len(cr.Hidden) > 0:
		return cr.Cond.Op.Title() + ", " + strconv.Itoa(len(cr.Hidden)) + " hidden"
	case cr.Cond.Op != sheet.CondNone:
		return cr.Cond.Op.Title() + " " + cr.Cond.Arg
	case len(cr.Hidden) > 0:
		return strconv.Itoa(len(cr.Hidden)) + " hidden"
	}
	return "showing all"
}

// act does what Space does on a line.
func (e *pivotEditor) act(m *Model, it pivotItem) tea.Cmd {
	switch {
	case it.kind == itemSource:
		e.editSource(m)
	case it.kind == itemSection:
		e.addField(m, it.sect)
	case it.kind == itemToggle:
		e.set(m, "change grand totals", func(p *sheet.Pivot) {
			if it.i == 0 {
				p.RowTotals = !p.RowTotals
			} else {
				p.ColumnTotals = !p.ColumnTotals
			}
		})
	case it.kind == itemField && it.sect == sectFilters:
		e.editFilter(m, it.i)
	case it.kind == itemField:
		e.adjust(m, it, 1)
	}
	return nil
}

// adjust steps a field's setting d places: a group's order (A→Z, Z→A,
// then by each value, smallest and largest first), or a value's summary.
func (e *pivotEditor) adjust(m *Model, it pivotItem, d int) {
	switch {
	case it.kind == itemToggle:
		e.act(m, it)
	case it.kind != itemField || it.sect == sectFilters:
	case it.sect == sectValues:
		e.set(m, "summarize by", func(p *sheet.Pivot) {
			fs := sheet.Summaries()
			v := &p.Values[it.i]
			v.Summarize = fs[wrap(slices.Index(fs, v.Summarize)+d, len(fs))]
			v.Name = ""
		})
	default:
		e.set(m, "order "+sectNames[it.sect], func(p *sheet.Pivot) {
			g := &(*groups(p, it.sect))[it.i]
			n := 2 * (len(p.Values) + 1)
			k := wrap(2*g.SortBy+boolInt(g.Desc)+d, n)
			if g.SortBy > len(p.Values) {
				k = 0
			}
			g.SortBy, g.Desc = k/2, k%2 == 1
		})
	}
}

func wrap(i, n int) int { return ((i % n) + n) % n }

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cycleShowAs steps a value through Sheets' "Show as" choices.
func (e *pivotEditor) cycleShowAs(m *Model, it pivotItem) {
	if it.kind != itemField || it.sect != sectValues {
		return
	}
	e.set(m, "show as", func(p *sheet.Pivot) {
		all := sheet.ShowAsList()
		v := &p.Values[it.i]
		v.ShowAs = all[wrap(slices.Index(all, v.ShowAs)+1, len(all))]
		v.Name = ""
	})
}

// remove takes a field out of its section.
func (e *pivotEditor) remove(m *Model, it pivotItem) {
	if it.kind != itemField {
		return
	}
	e.set(m, "remove "+e.fieldName(m, e.pivot(m), it)+" from "+sectNames[it.sect], func(p *sheet.Pivot) {
		switch it.sect {
		case sectValues:
			p.Values = slices.Delete(p.Values, it.i, it.i+1)
			for _, gs := range [][]sheet.PivotGroup{p.Rows, p.Columns} {
				for k := range gs {
					switch {
					case gs[k].SortBy == it.i+1:
						gs[k].SortBy, gs[k].Desc = 0, false
					case gs[k].SortBy > it.i+1:
						gs[k].SortBy--
					}
				}
			}
		case sectFilters:
			p.Filters = slices.Delete(p.Filters, it.i, it.i+1)
		default:
			gs := groups(p, it.sect)
			*gs = slices.Delete(*gs, it.i, it.i+1)
		}
	})
	e.current(m)
}

// reorder moves a row or column field up or down within its section,
// which nests the groups differently.
func (e *pivotEditor) reorder(m *Model, it pivotItem, down bool) {
	if it.kind != itemField || it.sect == sectFilters {
		return
	}
	p := e.pivot(m)
	j := it.i - 1
	if down {
		j = it.i + 1
	}
	if j < 0 || j >= sectLen(p, it.sect) {
		return
	}
	e.set(m, "move "+e.fieldName(m, p, it), func(p *sheet.Pivot) {
		swap := func(s []sheet.PivotGroup) { s[it.i], s[j] = s[j], s[it.i] }
		if it.sect == sectValues {
			p.Values[it.i], p.Values[j] = p.Values[j], p.Values[it.i]
			return
		}
		swap(*groups(p, it.sect))
	})
	e.selectItem(m, pivotItem{kind: itemField, sect: it.sect, i: j})
}

// addField picks a field of the source to add to a section.
func (e *pivotEditor) addField(m *Model, sect int) {
	p := e.pivot(m)
	used := map[int]bool{}
	switch sect {
	case sectRows, sectColumns:
		for _, g := range slices.Concat(p.Rows, p.Columns) {
			used[g.Col] = true
		}
	case sectFilters:
		for _, f := range p.Filters {
			used[f.Col] = true
		}
	}
	var items []pickItem
	for col := p.Range.From.Col; col <= p.Range.To.Col && !p.Lost; col++ {
		name := m.book().FieldName(p, col)
		items = append(items, pickItem{
			title: name, name: len(name), detail: "column " + sheet.ColName(col), off: used[col],
			desc: "Add " + name + " to " + sectNames[sect],
			pick: func(m *Model) tea.Cmd {
				m.closeOverlay()
				e.add(m, sect, col)
				e.reopen(m)
				return nil
			},
		})
	}
	fp := newPicker(m, "Add to "+sectNames[sect], "Type a field name", 50, items)
	fp.action = "add"
	m.openOverlay(&fieldPicker{picker: fp, back: e})
}

// add puts the field in column col at the end of a section.
func (e *pivotEditor) add(m *Model, sect, col int) {
	name := m.book().FieldName(e.pivot(m), col)
	e.set(m, "add "+name+" to "+sectNames[sect], func(p *sheet.Pivot) {
		switch sect {
		case sectValues:
			p.Values = append(p.Values, sheet.PivotValue{Col: col, Summarize: m.book().DefaultSummarize(*p, col)})
		case sectFilters:
			p.Filters = append(p.Filters, sheet.PivotFilter{Col: col})
		default:
			gs := groups(p, sect)
			*gs = append(*gs, sheet.PivotGroup{Col: col})
		}
	})
	e.selectItem(m, pivotItem{kind: itemField, sect: sect, i: sectLen(e.pivot(m), sect) - 1})
}

// fieldPicker is the picker of fields to add; Esc returns to the editor.
type fieldPicker struct {
	*picker
	back *pivotEditor
}

func (f *fieldPicker) Key(k tea.KeyPressMsg) tea.Cmd {
	m := f.m
	if k.String() == "esc" {
		m.closeOverlay()
		f.back.reopen(m)
		return nil
	}
	return f.picker.Key(k)
}

func (f *fieldPicker) Mouse(e overlay.MouseEvent) tea.Cmd {
	m := f.m
	if e.Box != pickerID && e.Kind == overlay.MousePress {
		m.closeOverlay()
		f.back.reopen(m)
		return nil
	}
	return f.picker.Mouse(e)
}

// editFilter opens the values list of filter i, in the manner of a
// filter's column.
func (e *pivotEditor) editFilter(m *Model, i int) {
	p := e.pivot(m)
	f := p.Filters[i]
	name := m.book().FieldName(p, f.Col)
	x, _, _ := e.box(m)
	fp := m.openValuesPicker("Filter "+name, x, m.book().PivotFilterValues(p, f.Col), f.Criteria.Cond,
		func(m *Model, cr sheet.Criteria) {
			e.set(m, "filter "+name, func(p *sheet.Pivot) { p.Filters[i].Criteria = cr })
			e.reopen(m)
		})
	fp.onCancel = e.reopen
}

// editSource points at the data on its sheet, starting from the current
// range; fields outside the new range are dropped.
func (e *pivotEditor) editSource(m *Model) {
	p := e.pivot(m)
	src, home := m.book().Lookup(p.Source), m.sheet
	if src == nil || src == home {
		e.msg = "The source sheet " + p.Source + " doesn't exist"
		return
	}
	if src.Hidden() {
		e.msg = hiddenMsg(src)
		return
	}
	m.closeOverlay()
	m.showSheet(src)
	if !p.Lost {
		m.selectRect(p.Range)
	}
	back := func(m *Model) {
		m.clearSelection()
		m.showSheet(home)
		e.reopen(m)
	}
	m.openRange("Pivot data:", func(m *Model, r sheet.Rect) tea.Cmd {
		back(m)
		e.set(m, "change the pivot's data to "+r.String(), func(p *sheet.Pivot) {
			p.Source, p.Range, p.Lost = src.Name(), r, false
			in := func(c int) bool { return c >= r.From.Col && c <= r.To.Col }
			keep := func(g sheet.PivotGroup) bool { return !in(g.Col) }
			p.Rows = slices.DeleteFunc(p.Rows, keep)
			p.Columns = slices.DeleteFunc(p.Columns, keep)
			p.Values = slices.DeleteFunc(p.Values, func(v sheet.PivotValue) bool { return !in(v.Col) })
			p.Filters = slices.DeleteFunc(p.Filters, func(f sheet.PivotFilter) bool { return !in(f.Col) })
		})
		return nil
	})
	m.prompt.onCancel = back
}
