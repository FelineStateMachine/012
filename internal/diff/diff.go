// Package diff compares and merges .012 workbooks cell by cell, for
// 012 diff and 012 merge-driver (see docs/files/git.md). Compare lists
// what changed between two workbooks: sheets added, removed, renamed or
// moved, each cell's input, value, format and note, notebook cells,
// regions, named ranges, macros and the other fields of sheets and
// workbooks. Merge
// combines two workbooks changed from a common one, and conflicts only
// where both changed the same part of the same cell or field.
//
// Both read files as their JSON fields (raw.go), so nothing a file holds
// goes unseen, and use the engine only to compute formulas' values,
// which the file doesn't store. Nothing here runs a notebook cell or
// asks JEV: values come from what the formulas compute alone.
package diff

import (
	"bytes"
	"fmt"

	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// Book is a workbook read for comparing: its file's fields and the
// workbook they make.
type Book struct {
	raw *rawBook
	wb  *sheet.Workbook // nil for a workbook without sheets
	vol map[*sheet.Sheet]map[sheet.Addr]bool
}

// Read reads a workbook file's contents. Empty contents are a workbook
// without sheets, which is what git hands over for a file that doesn't
// exist on one side.
func Read(data []byte) (*Book, error) {
	raw, err := parseRaw(data)
	if err != nil {
		return nil, err
	}
	b := &Book{raw: raw}
	if len(raw.sheets) > 0 {
		if b.wb, err = sheet.ReadBook(bytes.NewReader(data)); err != nil {
			return nil, err
		}
		b.vol = volatiles(b.wb)
	}
	return b, nil
}

// Kinds of Change.
const (
	KindSheet    = "sheet"    // a sheet added, removed, renamed or moved
	KindCell     = "cell"     // a cell's input, value, format or note
	KindRegion   = "region"   // a notebook region or linked file
	KindTable    = "table"    // a table
	KindLayout   = "layout"   // another field of a sheet: widths, rules, charts
	KindName     = "name"     // a named range
	KindMacro    = "macro"    // a macro
	KindWorkbook = "workbook" // a field of the workbook: locale, arithmetic
)

// Change is one difference. Old or New is null where the thing isn't on
// that side.
type Change struct {
	Kind  string
	Sheet string // the sheet, by its name in the second workbook where it has one
	Item  string // the cell, region, name, macro or field that changed
	// Field is what about it changed: for a cell input, value, format or
	// note; for a sheet added, removed, renamed or moved; for a region,
	// name or macro added, removed or its field's name; for a layout
	// field, its key when it has keys (a column's width), else "".
	Field    string
	Old, New nuon.Value
	// oldText and newText are Old and New as the text output shows them.
	oldText, newText string
}

// Where names the change's place as the text output does: Q3!B7,
// sheet Q3, Q3 region r1, Q3 table Sales, Q3 widths, name Sales.
func (c Change) Where() string {
	switch c.Kind {
	case KindCell:
		return sheet.QuoteSheet(c.Sheet) + "!" + c.Item
	case KindSheet:
		return "sheet " + c.Sheet
	case KindRegion:
		return c.Sheet + " region " + c.Item
	case KindTable:
		return c.Sheet + " table " + c.Item
	case KindLayout, KindNotebook:
		return c.Sheet + " " + c.Item
	}
	return c.Kind + " " + c.Item
}

// Compare lists what changed from a to b: sheets first, then each
// sheet's cells in row order, a notebook tab's cells, its regions and
// other fields, then the workbook's names, macros and settings.
func Compare(a, b *Book) []Change {
	var out []Change
	p := matchSheets(a.raw, b.raw)
	out = append(out, sheetChanges(a.raw, b.raw, p)...)
	empty := &rawSheet{fields: nil, cells: nil}
	for _, i := range p.onlyA {
		sa := a.raw.sheets[i]
		out = append(out, sheetDiff(a.side(sa), side{raw: empty}, sa.name)...)
	}
	for _, pr := range p.pairs {
		sa, sb := a.raw.sheets[pr[0]], b.raw.sheets[pr[1]]
		out = append(out, sheetDiff(a.side(sa), b.side(sb), sb.name)...)
	}
	for _, j := range p.onlyB {
		sb := b.raw.sheets[j]
		out = append(out, sheetDiff(side{raw: empty}, b.side(sb), sb.name)...)
	}
	return append(out, bookChanges(a.raw, b.raw)...)
}

// side is one sheet of a comparison: its fields, and the sheet the
// engine made of them, nil for a sheet that isn't there.
type side struct {
	raw *rawSheet
	s   *sheet.Sheet
	vol map[sheet.Addr]bool // its cells whose values change on every recalculation
}

func (b *Book) side(s *rawSheet) side {
	sd := side{raw: s}
	if b.wb != nil {
		sd.s = b.wb.Lookup(s.name)
		sd.vol = b.vol[sd.s]
	}
	return sd
}

// sheetChanges are the sheets added, removed, renamed and moved.
func sheetChanges(a, b *rawBook, p pairing) []Change {
	var out []Change
	for _, i := range p.onlyA {
		name := a.sheets[i].name
		out = append(out, textChange(KindSheet, name, "", "removed", name, ""))
	}
	for _, j := range p.onlyB {
		name := b.sheets[j].name
		out = append(out, textChange(KindSheet, name, "", "added", "", name))
	}
	for _, pr := range p.pairs {
		na, nb := a.sheets[pr[0]].name, b.sheets[pr[1]].name
		if na != nb {
			out = append(out, textChange(KindSheet, nb, "", "renamed", na, nb))
		}
	}
	// Moves: the fewest sheets that, taken out, leave the rest in the
	// same order on both sides.
	var orderB []int
	for _, pr := range p.pairs {
		orderB = append(orderB, pr[1])
	}
	stay := increasing(orderB)
	for k, pr := range p.pairs {
		if !stay[k] {
			nb := b.sheets[pr[1]].name
			c := textChange(KindSheet, nb, "", "moved", fmt.Sprint(pr[0]+1), fmt.Sprint(pr[1]+1))
			c.Old, c.New = nuon.IntValue(int64(pr[0]+1)), nuon.IntValue(int64(pr[1]+1))
			out = append(out, c)
		}
	}
	return out
}

// increasing marks a longest increasing run (not necessarily adjacent)
// of vs.
func increasing(vs []int) []bool {
	n := len(vs)
	length, prev := make([]int, n), make([]int, n)
	best := -1
	for i := range vs {
		length[i], prev[i] = 1, -1
		for j := range i {
			if vs[j] < vs[i] && length[j]+1 > length[i] {
				length[i], prev[i] = length[j]+1, j
			}
		}
		if best < 0 || length[i] > length[best] {
			best = i
		}
	}
	out := make([]bool, n)
	for i := best; i >= 0; i = prev[i] {
		out[i] = true
	}
	return out
}

// textChange is a change whose sides are text, "" for none.
func textChange(kind, sheetName, item, field, old, new string) Change {
	return Change{Kind: kind, Sheet: sheetName, Item: item, Field: field,
		Old: textValue(old), New: textValue(new), oldText: old, newText: new}
}

func textValue(s string) nuon.Value {
	if s == "" {
		return nuon.NullValue()
	}
	return nuon.StringValue(s)
}
