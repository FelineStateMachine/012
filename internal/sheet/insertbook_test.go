package sheet

import (
	"testing"
)

// Inserting an imported workbook adds its sheets after the others,
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

	res, err := w.InsertBook(src, "import book.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if got := namesOf(w); got != "Sales,Notes,Sales 2,Data" {
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
	if namesOf(w) != "Sales,Notes,Sales 2,Data" || show(t, w, "Notes!A1") != "6" {
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
	if err := w.ReplaceSheet(old, src.Sheet(0), "import sales.csv"); err != nil {
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
