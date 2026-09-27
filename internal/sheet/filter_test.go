package sheet

import (
	"bytes"
	"reflect"
	"testing"
)

// fruit is a small table with a header row, for filter tests.
func fruit(t *testing.T) *Sheet {
	return sheetOf(t, map[string]string{
		"A1": "Fruit", "B1": "Qty",
		"A2": "Apple", "B2": "10",
		"A3": "Banana", "B3": "3",
		"A4": "Cherry", "B4": "",
		"A5": "apple pie", "B5": "$25",
		"A6": "Date", "B6": "3",
		"B8": "=SUM(B2:B6)",
	})
}

// shownRows lists the rows (1-based) of 1..8 the filter doesn't hide.
func shownRows(s *Sheet) []int {
	var out []int
	for r := range 8 {
		if !s.RowHidden(r) {
			out = append(out, r+1)
		}
	}
	return out
}

func TestFilterConditions(t *testing.T) {
	tests := []struct {
		name string
		col  int
		cr   Criteria
		want []int
	}{
		{"none", 0, Criteria{}, []int{1, 2, 3, 4, 5, 6, 7, 8}},
		{"values", 1, Criteria{Hidden: []string{"3"}}, []int{1, 2, 4, 5, 7, 8}},
		{"values as formatted, blanks as empty", 1, Criteria{Hidden: []string{"$25", ""}}, []int{1, 2, 3, 6, 7, 8}},
		{"text contains ignores case", 0, Criteria{Cond: Condition{CondContains, "APP"}}, []int{1, 2, 5, 7, 8}},
		{"text does not contain", 0, Criteria{Cond: Condition{CondNotContains, "app"}}, []int{1, 3, 4, 6, 7, 8}},
		{"starts with", 0, Criteria{Cond: Condition{CondStartsWith, "b"}}, []int{1, 3, 7, 8}},
		{"ends with", 0, Criteria{Cond: Condition{CondEndsWith, "e"}}, []int{1, 2, 5, 6, 7, 8}},
		{"exactly", 0, Criteria{Cond: Condition{CondExactly, "apple"}}, []int{1, 2, 7, 8}},
		{"greater than", 1, Criteria{Cond: Condition{CondGreater, "3"}}, []int{1, 2, 5, 7, 8}},
		{"greater than a currency", 1, Criteria{Cond: Condition{CondGreater, "$20"}}, []int{1, 5, 7, 8}},
		{"at least", 1, Criteria{Cond: Condition{CondGreaterEq, "10"}}, []int{1, 2, 5, 7, 8}},
		{"less than skips blanks", 1, Criteria{Cond: Condition{CondLess, "10"}}, []int{1, 3, 6, 7, 8}},
		{"at most", 1, Criteria{Cond: Condition{CondLessEq, "10"}}, []int{1, 2, 3, 6, 7, 8}},
		{"equal", 1, Criteria{Cond: Condition{CondEqual, "3"}}, []int{1, 3, 6, 7, 8}},
		{"not equal keeps blanks", 1, Criteria{Cond: Condition{CondNotEqual, "3"}}, []int{1, 2, 4, 5, 7, 8}},
		{"empty", 1, Criteria{Cond: Condition{CondEmpty, ""}}, []int{1, 4, 7, 8}},
		{"not empty", 1, Criteria{Cond: Condition{CondNotEmpty, ""}}, []int{1, 2, 3, 5, 6, 7, 8}},
		{"values and condition together", 1, Criteria{Hidden: []string{"10"}, Cond: Condition{CondNotEmpty, ""}}, []int{1, 3, 5, 6, 7, 8}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := fruit(t)
			s.CreateFilter(Rect{From: at("A1"), To: at("B6")})
			s.FilterColumn(tt.col, tt.cr)
			if got := shownRows(s); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("shown rows %v, want %v", got, tt.want)
			}
			// Hidden rows still count in formulas, as in Sheets.
			if v := s.Value(at("B8")).Num; v != 41 {
				t.Errorf("SUM = %v, want 41", v)
			}
		})
	}
}

func TestFilterFollowsValuesAndUndo(t *testing.T) {
	s := fruit(t)
	s.CreateFilter(Rect{From: at("A1"), To: at("B6")})
	s.FilterColumn(1, Criteria{Cond: Condition{CondGreater, "5"}})
	if !s.RowHidden(2) || s.HiddenRows() != 3 {
		t.Fatalf("row 3 shown, %d hidden", s.HiddenRows())
	}
	// Changing a value re-applies the filter.
	s.Set(at("B3"), "30")
	if s.RowHidden(2) {
		t.Error("row 3 still hidden after its value passed")
	}
	s.Undo()
	if !s.RowHidden(2) {
		t.Error("undoing the edit didn't hide row 3 again")
	}
	s.Undo() // the condition
	if s.HiddenRows() != 0 || s.Filter() == nil {
		t.Errorf("after undoing the condition: %d hidden, filter %v", s.HiddenRows(), s.Filter())
	}
	s.Undo() // creating the filter
	if s.Filter() != nil {
		t.Error("undo didn't remove the filter")
	}
	s.Redo()
	s.Redo()
	if s.HiddenRows() != 3 {
		t.Errorf("redo: %d hidden", s.HiddenRows())
	}
	s.RemoveFilter()
	if s.Filter() != nil || s.HiddenRows() != 0 {
		t.Error("remove left the filter")
	}
}

func TestFilterValues(t *testing.T) {
	s := fruit(t)
	s.CreateFilter(Rect{From: at("A1"), To: at("B6")})
	s.FilterColumn(1, Criteria{Hidden: []string{"3", "gone"}})
	want := []FilterValue{{"3", 2, false}, {"10", 1, true}, {"$25", 1, true}, {"gone", 0, false}, {"", 1, true}}
	if got := s.FilterValues(1); !reflect.DeepEqual(got, want) {
		t.Errorf("values\n got %v\nwant %v", got, want)
	}
	// Other columns' criteria narrow the list.
	s.FilterColumn(0, Criteria{Cond: Condition{CondContains, "apple"}})
	want = []FilterValue{{"3", 0, false}, {"10", 1, true}, {"$25", 1, true}, {"gone", 0, false}}
	if got := s.FilterValues(1); !reflect.DeepEqual(got, want) {
		t.Errorf("narrowed values\n got %v\nwant %v", got, want)
	}
}

func TestFilterFollowsInsertAndDelete(t *testing.T) {
	s := fruit(t)
	s.CreateFilter(Rect{From: at("A1"), To: at("B6")})
	s.FilterColumn(1, Criteria{Hidden: []string{"3"}})
	s.InsertRows(2, 2) // inside the range: it grows
	if f := s.Filter(); f.Range.String() != "A1:B8" {
		t.Errorf("after insert: %v", f.Range)
	}
	s.InsertCols(0, 1) // before it: it moves, and so do its columns
	f := s.Filter()
	if f.Range.String() != "B1:C8" || f.Cols[2].Hidden[0] != "3" {
		t.Errorf("after insert column: %v %v", f.Range, f.Cols)
	}
	s.DeleteCols(1, 2) // all of it: the filter goes
	if s.Filter() != nil {
		t.Error("filter survived deleting its columns")
	}
	s.Undo()
	if s.Filter() == nil {
		t.Error("undo didn't bring the filter back")
	}
}

func TestEdgeSkipsHiddenRows(t *testing.T) {
	s := fruit(t)
	s.CreateFilter(Rect{From: at("A1"), To: at("B6")})
	s.FilterColumn(1, Criteria{Cond: Condition{CondEqual, "3"}}) // rows 1, 3 and 6 show
	tests := []struct {
		from string
		dr   int
		want string
	}{
		{"A1", 1, "A6"}, // through the visible block 1, 3, 6
		{"A6", -1, "A1"},
		{"B1", 1, "B6"},
		{"B6", 1, "B8"}, // row 7 is blank, then the total
	}
	for _, tt := range tests {
		if got := s.Edge(at(tt.from), 0, tt.dr).String(); got != tt.want {
			t.Errorf("Edge(%s, %d) = %s, want %s", tt.from, tt.dr, got, tt.want)
		}
	}
}

func TestFreeze(t *testing.T) {
	s := New()
	s.SetFrozen(2, 1)
	if r, c := s.Frozen(); r != 2 || c != 1 {
		t.Fatalf("frozen %d, %d", r, c)
	}
	s.InsertRows(1, 3) // inside the frozen rows: they grow
	if r, _ := s.Frozen(); r != 5 {
		t.Errorf("after insert: %d frozen rows", r)
	}
	s.DeleteRows(0, 4)
	if r, _ := s.Frozen(); r != 1 {
		t.Errorf("after delete: %d frozen rows", r)
	}
	s.InsertCols(0, 1) // before the frozen column: still one frozen
	if _, c := s.Frozen(); c != 1 {
		t.Errorf("after insert column: %d frozen columns", c)
	}
	for s.CanUndo() {
		s.Undo()
	}
	if r, c := s.Frozen(); r != 0 || c != 0 {
		t.Errorf("undo all: frozen %d, %d", r, c)
	}
	c, ok := s.Redo()
	if !ok || c.Label != "freeze 2 rows" {
		t.Errorf("redo %v %v", c, ok)
	}
}

func TestViewFileRoundTrip(t *testing.T) {
	s := fruit(t)
	s.SetFrozen(1, 2)
	s.CreateFilter(Rect{From: at("A1"), To: at("B6")})
	s.FilterColumn(1, Criteria{Hidden: []string{"3", ""}, Cond: Condition{CondLess, "$20"}})
	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"version": 3`, `"freeze": {"rows":1,"cols":2}`,
		`"filter": {"range":"A1:B6","columns":{"B":{"hidden":["","3"],"condition":"lt","value":"$20"}}}`} {
		if !bytes.Contains(buf.Bytes(), []byte(want)) {
			t.Errorf("file lacks %s:\n%s", want, buf.String())
		}
	}
	back, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if r, c := back.Frozen(); r != 1 || c != 2 {
		t.Errorf("frozen %d, %d", r, c)
	}
	if !reflect.DeepEqual(back.Filter(), s.Filter()) || !reflect.DeepEqual(shownRows(back), shownRows(s)) {
		t.Errorf("filter %+v, want %+v", back.Filter(), s.Filter())
	}
	if back.CanUndo() {
		t.Error("loading is undoable")
	}
}
