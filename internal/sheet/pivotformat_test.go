package sheet

import "testing"

// One price typed as "$9,000" among "$x.xx" prices keeps the results in
// the column's two-decimal currency, as in Sheets: the pivot takes the
// format most of the column's numbers show in, not the first row's.
func TestPivotValueFormatIgnoresOutlier(t *testing.T) {
	w := sales(t)
	src := w.Lookup("Sales")
	if err := src.Set(at("E2"), "$9,000"); err != nil {
		t.Fatal(err)
	}
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 1}}
		p.Values = []PivotValue{{Col: 4, Summarize: SumBy}}
	})
	checkGrid(t, s,
		"Product|SUM of Price",
		"Ink|$18.00",
		"Pens|$9,005.25",
		"Grand Total|$9,023.25")

	// A frequency table counts, so its values show no currency at all;
	// each label keeps its own cell's format.
	r, _ := src.UsedRange()
	f, err := w.CreatePivot(src, r, "Frequency of Price", FrequencyPivot(src, r, 4))
	if err != nil {
		t.Fatal(err)
	}
	checkGrid(t, f,
		"Price|Count|Percent",
		"$9.00|2|40.00%",
		"$2.50|1|20.00%",
		"$2.75|1|20.00%",
		"$9,000|1|20.00%",
		"Grand Total|5|100.00%")
}

// The column's own format wins over its cells'; without one, a tie goes
// to the format met first.
func TestPivotValueFormatColumnAndTies(t *testing.T) {
	w := bookOf(t, page{"Sales", map[string]string{
		"A1": "K", "B1": "N",
		"A2": "a", "B2": "$1",
		"A3": "a", "B3": "2.5%",
		"A4": "a", "B4": "$3",
		"A5": "a", "B5": "4%",
		"A6": "a", "B6": "x",
	}})
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 0}}
		p.Values = []PivotValue{{Col: 1, Summarize: MaxBy}}
	})
	checkGrid(t, s, "K|MAX of N", "a|$3", "Grand Total|$3")

	w.Lookup("Sales").SetFormat(col(1), Preset(FmtNumber))
	checkGrid(t, s, "K|MAX of N", "a|3.00", "Grand Total|3.00")
}
