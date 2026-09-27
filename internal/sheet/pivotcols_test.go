package sheet

import (
	"bytes"
	"testing"
)

func TestPivotColumnSubtotals(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) {
		p.Columns = []PivotGroup{{Col: 0}, {Col: 1}}
		p.Values = []PivotValue{{Col: 3, Summarize: SumBy, Name: "Units sold"}}
	})
	// A subtotal column after each region's products, named in the
	// Region row; the value's own name heads every column.
	checkGrid(t, s,
		"Region|East||East Total|North|North Total|West||West Total|Grand Total",
		"Product|Ink|Pens||Pens||Ink|Pens",
		"|Units sold|Units sold|Units sold|Units sold|Units sold|Units sold|Units sold|Units sold|Units sold",
		"Grand Total|6|10|16|0|0|4|3|7|23")
	if c := s.Cell(at("D1")); c == nil || !c.Style.Bold {
		t.Error("the subtotal's header isn't bold")
	}
	// Without column totals, neither subtotals nor the grand total show.
	p, _ := s.Pivot()
	p.ColumnTotals = false
	s.SetPivot(p, "")
	checkGrid(t, s,
		"Region|East||North|West",
		"Product|Ink|Pens|Pens|Ink|Pens",
		"|Units sold|Units sold|Units sold|Units sold|Units sold",
		"Grand Total|6|10|0|4|3")
	// Sorting the outer field by its total keeps each region together.
	p.Columns[0] = PivotGroup{Col: 0, Desc: true, SortBy: 1}
	s.SetPivot(p, "")
	checkGrid(t, s,
		"Region|East||West||North",
		"Product|Ink|Pens|Ink|Pens|Pens",
		"|Units sold|Units sold|Units sold|Units sold|Units sold",
		"Grand Total|6|10|4|3|0")
}

func TestPivotBlankGroupsAndNamesRoundTrip(t *testing.T) {
	w := sales(t)
	w.Lookup("Sales").Set(at("A3"), "")
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 0}, {Col: 1}}
		p.Columns = []PivotGroup{{Col: 0}, {Col: 1}}
		p.Values = []PivotValue{{Col: 3, Summarize: SumBy, Name: "Units"}}
	})
	// Blank cells group under a blank label, as in Sheets; their total
	// is just "Total".
	grid := pivotGrid(s)
	found := false
	for _, row := range grid {
		if len(row) >= 6 && row[:6] == "Total|" {
			found = true
		}
	}
	if !found {
		t.Errorf("no Total row for the blank group:\n%v", grid)
	}
	var b bytes.Buffer
	if err := w.Write(&b); err != nil {
		t.Fatal(err)
	}
	back, err := ReadBook(&b)
	if err != nil {
		t.Fatal(err)
	}
	p, _ := back.Lookup(s.Name()).Pivot()
	if p.Values[0].Name != "Units" || !p.ColumnTotals {
		t.Errorf("read back %+v", p)
	}
	checkGrid(t, back.Lookup(s.Name()), grid...)
}
