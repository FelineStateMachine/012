package sheet

import (
	"testing"
)

// Inserting an imported workbook adds its sheets where asked, here after
// the first,
// renaming the ones whose names are taken (and the formulas that use
// them), brings its free named ranges, and undoes as one step.
func TestInsertBook(t *testing.T) {
	w := bookOf(t,
		page{"Sales", map[string]string{"A1": "1"}},
		page{"Notes", map[string]string{"A1": "=Sales!A1+Data!A1"}},
	)
	w.DefineName("Taken", w.Sheet(0), NewRect(at("A1"), at("A1")))
	w.ClearHistory()
	src := bookOf(t,
		page{"Sales", map[string]string{"A1": "10", "A2": "=Data!A1*2"}},
		page{"Data", map[string]string{"A1": "5", "A2": "=Sales!A1+1"}},
	)
	src.DefineName("Taken", src.Sheet(1), NewRect(at("A1"), at("A1")))
	src.DefineName("Fresh", src.Sheet(1), NewRect(at("A1"), at("A1")))
	src.hidePlain(src.Sheet(1))

	res, err := w.InsertBook(src, 1, "import book.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if got := namesOf(w); got != "Sales,Sales 2,Data,Notes" {
		t.Fatalf("sheets %s", got)
	}
	if len(res.Sheets) != 2 || res.Renamed["Sales"] != "Sales 2" || res.Names != 1 {
		t.Errorf("result %+v", res)
	}
	for ref, want := range map[string]string{
		"Sales 2!A2":    "10", // Data!A1*2 on the imported sheets
		"Data!A2":       "11", // Sales!A1+1 now reads Sales 2
		"Data!input:A2": "='Sales 2'!A1+1",
		"Notes!A1":      "6", // Data exists now
	} {
		if got := show(t, w, ref); got != want {
			t.Errorf("%s = %s, want %s", ref, got, want)
		}
	}
	if !w.Lookup("Data").Hidden() {
		t.Error("Data lost its hidden flag")
	}
	if n, ok := w.LookupName("Fresh"); !ok || n.Sheet != w.Lookup("Data") {
		t.Errorf("Fresh: %v %+v", ok, n)
	}
	if n, _ := w.LookupName("Taken"); n.Sheet != w.Sheet(0) {
		t.Error("an imported name replaced Taken")
	}

	c, ok := w.Undo()
	if !ok || c.Label != "import book.xlsx" || namesOf(w) != "Sales,Notes" {
		t.Fatalf("undo: %+v, sheets %s", c, namesOf(w))
	}
	if _, ok := w.LookupName("Fresh"); ok || show(t, w, "Notes!A1") != "#REF!" {
		t.Errorf("undo left Fresh %v, Notes!A1 %s", ok, show(t, w, "Notes!A1"))
	}
	w.Redo()
	if namesOf(w) != "Sales,Sales 2,Data,Notes" || show(t, w, "Notes!A1") != "6" {
		t.Errorf("redo: %s", namesOf(w))
	}
}

// Replacing a sheet puts the imported one in its place, with its name,
// so formulas and named ranges reading it read the new data.
func TestReplaceSheet(t *testing.T) {
	w := bookOf(t,
		page{"Sales", map[string]string{"A1": "1", "B1": "old"}},
		page{"Notes", map[string]string{"A1": "=Sales!A1*2"}},
	)
	w.DefineName("First", w.Sheet(0), NewRect(at("A1"), at("A1")))
	w.Sheet(1).Set(at("A2"), "=First")
	w.ClearHistory()
	src := bookOf(t, page{"sales", map[string]string{"A1": "21"}})

	old := w.Sheet(0)
	if _, err := w.ReplaceSheet(old, src.Sheet(0), "import sales.csv"); err != nil {
		t.Fatal(err)
	}
	if namesOf(w) != "Sales,Notes" || old.Live() || show(t, w, "Sales!B1") != "" {
		t.Fatalf("sheets %s", namesOf(w))
	}
	if show(t, w, "Notes!A1") != "42" || show(t, w, "Notes!A2") != "21" {
		t.Errorf("Notes!A1 %s, A2 %s", show(t, w, "Notes!A1"), show(t, w, "Notes!A2"))
	}
	c, _ := w.Undo()
	if c.Label != "import sales.csv" || w.Sheet(0) != old || show(t, w, "Notes!A1") != "2" || show(t, w, "Notes!A2") != "1" {
		t.Errorf("undo: %+v, Notes!A1 %s", c, show(t, w, "Notes!A1"))
	}
	w.Redo()
	if show(t, w, "Notes!A1") != "42" {
		t.Errorf("redo: %s", show(t, w, "Notes!A1"))
	}
}

func namesOf(w *Workbook) string {
	var out string
	for i, s := range w.sheets {
		if i > 0 {
			out += ","
		}
		out += s.name
	}
	return out
}

// hidePlain hides s outside the history, as a loader would.
func (w *Workbook) hidePlain(s *Sheet) { s.tabHidden = true }

// Replacing a sheet keeps the charts that fit the new data: one that
// drew the whole table is re-pointed to the new table when it has as
// many columns and removed when it hasn't; one of part of a table keeps
// its range; one whose range is empty now is removed. Undo brings the
// removed ones back.
func TestReplaceSheetKeepsCharts(t *testing.T) {
	w := bookOf(t, page{"Sales", map[string]string{
		"A1": "Month", "B1": "Total", "A2": "Jan", "B2": "1", "A3": "Feb", "B3": "2",
		"E1": "x", "E2": "3", "H1": "9",
	}})
	old := w.Sheet(0)
	old.AddChart(Chart{Data: NewRect(at("A1"), at("B3")), Title: "Totals"})
	old.AddChart(Chart{Data: NewRect(at("E1"), at("E2"))})
	old.AddChart(Chart{Data: NewRect(at("H1"), at("H1"))})
	old.AddChart(Chart{Data: NewRect(at("B2"), at("B3")), Title: "Part"})
	w.ClearHistory()
	src := bookOf(t, page{"sales", map[string]string{
		"A1": "Month", "B1": "Total", "A2": "Jan", "B2": "4", "A3": "Feb", "B3": "5", "A4": "Mar", "B4": "6",
		"E1": "a", "F1": "b", "E2": "1", "F2": "2",
	}})
	fates, err := w.ReplaceSheet(old, src.Sheet(0), "import sales.csv")
	if err != nil {
		t.Fatal(err)
	}
	want := []ChartFate{
		{Name: "Totals", Was: NewRect(at("A1"), at("B3")), Now: NewRect(at("A1"), at("B4"))},
		{Name: "Chart 2", Was: NewRect(at("E1"), at("E2")), Now: NewRect(at("E1"), at("E2")), Removed: true}, // a column more
		{Name: "Chart 3", Was: NewRect(at("H1"), at("H1")), Now: NewRect(at("H1"), at("H1")), Removed: true}, // empty
		{Name: "Part", Was: NewRect(at("B2"), at("B3")), Now: NewRect(at("B2"), at("B3"))},
	}
	if len(fates) != len(want) {
		t.Fatalf("fates %+v", fates)
	}
	for i := range want {
		if fates[i] != want[i] {
			t.Errorf("chart %d: %+v, want %+v", i+1, fates[i], want[i])
		}
	}
	charts := w.Sheet(0).Charts()
	if len(charts) != 2 || charts[0].Data != want[0].Now || charts[0].Title != "Totals" || charts[1].Title != "Part" {
		t.Errorf("charts on the new sheet: %+v", charts)
	}
	w.Undo()
	if w.Sheet(0) != old || len(old.Charts()) != 4 || old.Charts()[0].Data != want[0].Was {
		t.Errorf("undo: charts %+v", old.Charts())
	}
}
