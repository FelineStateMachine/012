package sheet

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

// sales is a small table: region, product, date, units, price.
func sales(t *testing.T) *Workbook {
	t.Helper()
	return bookOf(t, page{"Sales", map[string]string{
		"A1": "Region", "B1": "Product", "C1": "Date", "D1": "Units", "E1": "Price",
		"A2": "East", "B2": "Pens", "C2": "1/5/2026", "D2": "10", "E2": "$2.50",
		"A3": "West", "B3": "Ink", "C3": "1/5/2026", "D3": "4", "E3": "$9.00",
		"A4": "east", "B4": "Ink", "C4": "2/9/2026", "D4": "6", "E4": "$9.00",
		"A5": "North", "B5": "Pens", "C5": "2/9/2026", "D5": "", "E5": "$2.50",
		"A6": "West", "B6": "Pens", "C6": "1/5/2026", "D6": "3", "E6": "$2.75",
	}})
}

// pivotGrid is what a pivot sheet shows, row by row, cells separated by
// |, trailing blanks dropped.
func pivotGrid(s *Sheet) []string {
	out, ok := s.PivotRange()
	if !ok {
		return nil
	}
	var rows []string
	for row := 0; row <= out.To.Row; row++ {
		var cells []string
		for col := 0; col <= out.To.Col; col++ {
			cells = append(cells, s.ShownText(Addr{Col: col, Row: row}))
		}
		rows = append(rows, strings.TrimRight(strings.Join(cells, "|"), "|"))
	}
	return rows
}

func checkGrid(t *testing.T, s *Sheet, want ...string) {
	t.Helper()
	if got := pivotGrid(s); !reflect.DeepEqual(got, want) {
		t.Errorf("pivot shows\n  %s\nwant\n  %s\n(error %q)", strings.Join(got, "\n  "), strings.Join(want, "\n  "), s.PivotError())
	}
}

func newPivot(t *testing.T, w *Workbook, p func(*Pivot)) *Sheet {
	t.Helper()
	src := w.Lookup("Sales")
	used, _ := src.UsedRange()
	pv := NewPivot(src, used)
	p(&pv)
	s, err := w.CreatePivot(src, used, "", pv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPivotGroupsRows(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 0}}
		p.Values = []PivotValue{{Col: 3, Summarize: SumBy}, {Col: 3, Summarize: CountABy}}
	})
	if s.Name() != "Pivot Table 1" || w.Index(s) != 1 {
		t.Errorf("sheet %q at %d, want Pivot Table 1 after Sales", s.Name(), w.Index(s))
	}
	// Text groups ignore case, keeping the first spelling, as Sheets.
	checkGrid(t, s,
		"Region|SUM of Units|COUNTA of Units",
		"East|16|2",
		"North|0|0",
		"West|7|2",
		"Grand Total|23|4")
	if c := s.Cell(at("A5")); !c.Style.Bold {
		t.Error("the Grand Total row isn't bold")
	}
	// A new pivot sheet's columns fit the results, until set by hand.
	if s.ColWidth(0) != 13 || s.ColWidth(2) != 17 {
		t.Errorf("widths %d %d, want 13 17", s.ColWidth(0), s.ColWidth(2))
	}
	s.SetColWidth(0, 20)
	w.Lookup("Sales").Set(at("A2"), "Eastern seaboard region")
	if s.ColWidth(0) != 20 {
		t.Errorf("width %d after setting it by hand", s.ColWidth(0))
	}
}

func TestPivotFilterValues(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) {
		p.Filters = []PivotFilter{
			{Col: 0, Criteria: Criteria{Hidden: []string{"West"}}},
			{Col: 1, Criteria: Criteria{Hidden: []string{"Ink"}}},
		}
	})
	p, _ := s.Pivot()
	// Region's values among the rows Product's filter lets through.
	got := w.PivotFilterValues(p, 0)
	want := []FilterValue{{"East", 1, true}, {"North", 1, true}, {"West", 1, false}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("values %v, want %v", got, want)
	}
}

func TestPivotColumnsAndSubtotals(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 1}, {Col: 0}}
		p.Columns = []PivotGroup{{Col: 2}}
		p.Values = []PivotValue{{Col: 3, Summarize: SumBy}}
	})
	// Dates group by day and show as dates.
	checkGrid(t, s,
		"|Date|1/5/2026|2/9/2026|Grand Total",
		"Product|Region|SUM of Units|SUM of Units|SUM of Units",
		"Ink|east||6|6",
		"|West|4||4",
		"Ink Total||4|6|10",
		"Pens|East|10||10",
		"|North||0|0",
		"|West|3||3",
		"Pens Total||13|0|13",
		"Grand Total||17|6|23")
	p, _ := s.Pivot()
	p.RowTotals, p.ColumnTotals = false, false
	if err := s.SetPivot(p, ""); err != nil {
		t.Fatal(err)
	}
	checkGrid(t, s,
		"|Date|1/5/2026|2/9/2026",
		"Product|Region|SUM of Units|SUM of Units",
		"Ink|east||6",
		"|West|4",
		"Pens|East|10",
		"|North||0",
		"|West|3")
}

func TestPivotSummaries(t *testing.T) {
	w := sales(t)
	var vals []PivotValue
	for _, f := range Summaries() {
		vals = append(vals, PivotValue{Col: 4, Summarize: f})
	}
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 1}}
		p.Values = vals
	})
	// Sums, averages and extremes keep the column's currency format.
	checkGrid(t, s,
		"Product|SUM of Price|COUNTA of Price|COUNT of Price|COUNTUNIQUE of Price|AVERAGE of Price|MAX of Price|MIN of Price",
		"Ink|$18.00|2|2|1|$9.00|$9.00|$9.00",
		"Pens|$7.75|3|3|2|$2.58|$2.75|$2.50",
		"Grand Total|$25.75|5|5|3|$5.15|$9.00|$2.50")
}

func TestPivotMixedKeysAndBlanks(t *testing.T) {
	w := bookOf(t, page{"Sales", map[string]string{
		"A1": "Key", "B1": "N",
		"A2": "1", "B2": "1",
		"A3": "'1", "B3": "2",
		"A4": "TRUE", "B4": "4",
		"A5": "", "B5": "8",
		"A6": "b", "B6": "16",
		"A7": "=1/0", "B7": "32",
		"A8": "B", "B8": "=1/0",
	}})
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 0}}
		p.Values = []PivotValue{{Col: 1, Summarize: SumBy}, {Col: 1, Summarize: CountBy}}
	})
	// Numbers, then text, booleans and errors; blanks last. 1 and "1"
	// are different keys; an error in the values shows in their sums.
	checkGrid(t, s,
		"Key|SUM of N|COUNT of N",
		"1|1|1",
		"1|2|1",
		"b|#DIV/0!|1",
		"TRUE|4|1",
		"#DIV/0!|32|1",
		"(blank)|8|1",
		"Grand Total|#DIV/0!|6")
}

func TestPivotSortByValueAndShowAs(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 0, Desc: true, SortBy: 1}}
		p.Values = []PivotValue{{Col: 3, Summarize: SumBy}, {Col: 3, Summarize: SumBy, ShowAs: ShowPctTotal}}
	})
	checkGrid(t, s,
		"Region|SUM of Units|SUM of Units",
		"East|16|69.57%",
		"West|7|30.43%",
		"North|0|0.00%",
		"Grand Total|23|100.00%")
	p, _ := s.Pivot()
	p.Rows[0] = PivotGroup{Col: 0, Desc: true}
	s.SetPivot(p, "")
	checkGrid(t, s,
		"Region|SUM of Units|SUM of Units",
		"West|7|30.43%",
		"North|0|0.00%",
		"East|16|69.57%",
		"Grand Total|23|100.00%")
}

func TestPivotFilters(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 1}}
		p.Values = []PivotValue{{Col: 3, Summarize: SumBy}}
		p.Filters = []PivotFilter{
			{Col: 0, Criteria: Criteria{Hidden: []string{"North"}}},
			{Col: 3, Criteria: Criteria{Cond: Condition{Op: CondGreater, Arg: "3"}}},
		}
	})
	checkGrid(t, s,
		"Product|SUM of Units",
		"Ink|10",
		"Pens|10",
		"Grand Total|20")
}

func TestPivotFrequency(t *testing.T) {
	w := sales(t)
	src := w.Lookup("Sales")
	r, _ := src.UsedRange()
	s, err := w.CreatePivot(src, r, "Frequency of Product", FrequencyPivot(src, r, 1))
	if err != nil {
		t.Fatal(err)
	}
	checkGrid(t, s,
		"Product|Count|Percent",
		"Pens|3|60.00%",
		"Ink|2|40.00%",
		"Grand Total|5|100.00%")
	// Blanks are counted too.
	src.Set(at("B6"), "")
	checkGrid(t, s,
		"Product|Count|Percent",
		"Ink|2|40.00%",
		"Pens|2|40.00%",
		"(blank)|1|20.00%",
		"Grand Total|5|100.00%")
}

func TestPivotLiveUndoAndReads(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 0}}
		p.Values = []PivotValue{{Col: 3, Summarize: SumBy}}
	})
	src := w.Lookup("Sales")
	// A formula elsewhere reads the pivot's result, and follows it.
	if err := src.Set(at("G1"), "='Pivot Table 1'!B5*2"); err != nil {
		t.Fatal(err)
	}
	if got := show(t, w, "Sales!G1"); got != "46" {
		t.Fatalf("G1 = %s, want 46", got)
	}
	src.Set(at("D2"), "100")
	checkGrid(t, s, "Region|SUM of Units", "East|106", "North|0", "West|7", "Grand Total|113")
	if got := show(t, w, "Sales!G1"); got != "226" {
		t.Errorf("G1 = %s after the edit, want 226", got)
	}
	// Rows inserted inside the source range grow it.
	if err := src.InsertRows(5, 1); err != nil {
		t.Fatal(err)
	}
	src.Set(at("A6"), "South")
	src.Set(at("D6"), "1")
	checkGrid(t, s, "Region|SUM of Units", "East|106", "North|0", "South|1", "West|7", "Grand Total|114")

	for range 4 {
		w.Undo()
	}
	checkGrid(t, s, "Region|SUM of Units", "East|16", "North|0", "West|7", "Grand Total|23")
	if got := show(t, w, "Sales!G1"); got != "46" {
		t.Errorf("G1 = %s after undo, want 46", got)
	}
	// Undoing the pivot's creation removes its sheet; redo brings it back.
	w.Undo()
	w.Undo()
	if w.Lookup("Pivot Table 1") != nil {
		t.Fatal("undo left the pivot sheet")
	}
	w.Redo()
	checkGrid(t, w.Lookup("Pivot Table 1"), "Region|SUM of Units", "East|16", "North|0", "West|7", "Grand Total|23")
}

func TestPivotEditsAreOneStep(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) { p.Rows = []PivotGroup{{Col: 0}} })
	p, _ := s.Pivot()
	p.Values = []PivotValue{{Col: 3, Summarize: MaxBy}}
	if err := s.SetPivot(p, "add a value"); err != nil {
		t.Fatal(err)
	}
	checkGrid(t, s, "Region|MAX of Units", "East|10", "North|0", "West|4", "Grand Total|10")
	c, ok := w.Undo()
	if !ok || c.Label != "add a value" {
		t.Fatalf("undo %q, want add a value", c.Label)
	}
	checkGrid(t, s, "Region", "East", "North", "West")
}

func TestPivotRefusesEdits(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 0}}
		p.Values = []PivotValue{{Col: 3, Summarize: SumBy}}
	})
	if err := s.Set(at("B2"), "5"); err != ErrPivotEdit {
		t.Errorf("Set in the results: %v, want ErrPivotEdit", err)
	}
	if !s.InPivot(NewRect(at("C4"), at("B9"))) || s.InPivot(NewRect(at("C1"), at("D9"))) {
		t.Error("InPivot is wrong about the results' range B1:B5")
	}
	// Next to the results is fine; a result growing into it is an error
	// the pivot shows, as in Sheets.
	if err := s.Set(at("A6"), "note"); err != nil {
		t.Fatal(err)
	}
	src := w.Lookup("Sales")
	src.InsertRows(5, 1)
	src.Set(at("A6"), "South")
	if got := show(t, w, "'Pivot Table 1'!A1"); got != "#REF!" || !strings.Contains(s.ExplainError(at("A1")), "overwrite data in A6") {
		t.Errorf("A1 = %s (%q), want #REF! explaining the overlap", got, s.ExplainError(at("A1")))
	}
	if got := show(t, w, "'Pivot Table 1'!A6"); got != "note" {
		t.Errorf("A6 = %q, want the note kept", got)
	}
	// Clearing the note makes room again.
	if err := s.Set(at("A6"), ""); err != nil {
		t.Fatal(err)
	}
	checkGrid(t, s, "Region|SUM of Units", "East|16", "North|0", "South|0", "West|7", "Grand Total|23")
	w.Undo()
	// Copying results pastes their values.
	w.Undo()
	w.Undo()
	clip := s.Copy(NewRect(at("A2"), at("B2")))
	if _, err := src.Paste(clip, NewRect(at("H1"), at("H1")), false); err != nil {
		t.Fatal(err)
	}
	if got := show(t, w, "Sales!input:I1"); got != "16" || src.Cell(at("I1")).derived {
		t.Errorf("pasted %q, want the plain value 16", got)
	}
}

func TestPivotFollowsSourceSheet(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 1}}
		p.Values = []PivotValue{{Col: 3, Summarize: SumBy}}
	})
	src := w.Lookup("Sales")
	if err := w.RenameSheet(src, "Q1"); err != nil {
		t.Fatal(err)
	}
	if p, _ := s.Pivot(); p.Source != "Q1" {
		t.Errorf("source %q after renaming, want Q1", p.Source)
	}
	// A column inserted before the data moves the fields along.
	if err := src.InsertCols(0, 1); err != nil {
		t.Fatal(err)
	}
	p, _ := s.Pivot()
	if p.Range != NewRect(at("B1"), at("F6")) || p.Rows[0].Col != 2 || p.Values[0].Col != 4 {
		t.Errorf("after inserting a column: %+v", p)
	}
	checkGrid(t, s, "Product|SUM of Units", "Ink|10", "Pens|13", "Grand Total|23")
	// Deleting the value column drops the value.
	src.DeleteCols(4, 1)
	checkGrid(t, s, "Product", "Ink", "Pens")
	// Deleting the source sheet shows #REF!, and undo brings it back.
	if err := w.DeleteSheet(src); err != nil {
		t.Fatal(err)
	}
	if got := show(t, w, "'Pivot Table 1'!A1"); got != "#REF!" || !strings.Contains(s.PivotError(), "doesn't exist") {
		t.Errorf("A1 = %s (%q) without the source", got, s.PivotError())
	}
	w.Undo()
	checkGrid(t, s, "Product", "Ink", "Pens")
}

func TestPivotRefusesLoops(t *testing.T) {
	w := sales(t)
	a := newPivot(t, w, func(p *Pivot) { p.Rows = []PivotGroup{{Col: 0}} })
	b, err := w.CreatePivot(a, NewRect(at("A1"), at("A4")), "", NewPivot(a, NewRect(at("A1"), at("A4"))))
	if err != nil {
		t.Fatal(err)
	}
	p, _ := a.Pivot()
	p.Source = b.Name()
	if err := a.SetPivot(p, ""); err != errPivotLoop {
		t.Errorf("a loop: %v", err)
	}
	p.Source = a.Name()
	if err := a.SetPivot(p, ""); err != errPivotSelf {
		t.Errorf("its own sheet: %v", err)
	}
}

func TestPivotFileRoundTrip(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 0, Desc: true, SortBy: 1}}
		p.Columns = []PivotGroup{{Col: 1}}
		p.Values = []PivotValue{{Col: 3, Summarize: AverageBy, ShowAs: ShowPctRow, Name: "Share"}}
		p.Filters = []PivotFilter{{Col: 2, Criteria: Criteria{Hidden: []string{"2/9/2026"}, Cond: Condition{Op: CondNotEmpty}}}}
		p.ColumnTotals = false
	})
	want := pivotGrid(s)
	var buf bytes.Buffer
	if err := w.Write(&buf); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, `"version": 5`) || !strings.Contains(text, `"pivot": {"source":"Sales!A1:E6"`) {
		t.Fatalf("file:\n%s", text)
	}
	if strings.Contains(text, `"Grand Total"`) {
		t.Error("the pivot's results were saved")
	}
	w2, err := ReadBook(strings.NewReader(text))
	if err != nil {
		t.Fatal(err)
	}
	s2 := w2.Lookup("Pivot Table 1")
	p1, _ := s.Pivot()
	p2, _ := s2.Pivot()
	if !reflect.DeepEqual(p1, p2) {
		t.Errorf("read back\n%+v\nwant\n%+v", p2, p1)
	}
	if got := pivotGrid(s2); !reflect.DeepEqual(got, want) {
		t.Errorf("read back shows %q, want %q", got, want)
	}
	// Without a pivot, the file stays version 4.
	w2.DeleteSheet(s2)
	w2.AddSheet("", 5)
	buf.Reset()
	w2.Write(&buf)
	if !strings.Contains(buf.String(), `"version": 4`) {
		t.Errorf("without pivots: %.40s", buf.String())
	}
}

// The example in docs/files.md reads.
func TestPivotFileExample(t *testing.T) {
	file := `{"version": 5, "sheets": [
	  {"name": "Sales", "cells": {"A1": "Region", "B1": "Item", "C1": "Year", "D1": "Units", "A2": "East", "B2": "Pens", "C2": "2026", "D2": "3"}},
	  {"name": "Pivot Table 1", "cells": {},
	   "pivot": {"source":"Sales!A1:D200","rows":[{"column":"B"},{"column":"A","order":"desc","sortBy":1}],"columns":[{"column":"C"}],"values":[{"column":"D","summarize":"sum"},{"column":"D","summarize":"counta","showAs":"percent_of_total","name":"Share"}],"filters":[{"column":"A","hidden":["North"]}],"rowTotals":true,"columnTotals":false}}
	]}`
	w, err := ReadBook(strings.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	s := w.Lookup("Pivot Table 1")
	checkGrid(t, s,
		"|Year|2026",
		"Item|Region|SUM of Units|Share",
		"Pens|East|3|100.00%",
		"Pens Total||3|100.00%",
		"Grand Total||3|100.00%")
}

func TestPivotDuplicate(t *testing.T) {
	w := sales(t)
	s := newPivot(t, w, func(p *Pivot) { p.Rows = []PivotGroup{{Col: 1}} })
	cp, err := w.DuplicateSheet(s)
	if err != nil {
		t.Fatal(err)
	}
	checkGrid(t, cp, "Product", "Ink", "Pens")
	w.Lookup("Sales").Set(at("B2"), "Paper")
	checkGrid(t, cp, "Product", "Ink", "Paper", "Pens")
}

func TestPivotTelemetry(t *testing.T) {
	var got []PivotInfo
	OnPivot = func(i PivotInfo) { got = append(got, i) }
	defer func() { OnPivot = nil }()
	w := sales(t)
	newPivot(t, w, func(p *Pivot) {
		p.Rows = []PivotGroup{{Col: 0}, {Col: 1}}
		p.Values = []PivotValue{{Col: 3, Summarize: SumBy}}
	})
	if len(got) != 1 || got[0].Records != 5 || got[0].Groups != 8 || got[0].Failed {
		t.Errorf("reported %+v", got)
	}
}
