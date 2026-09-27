package sheet

import (
	"maps"
	"strings"
	"testing"
)

func TestInsertDeleteRows(t *testing.T) {
	base := map[string]string{
		"A1": "1", "A2": "2", "A3": "3", "A4": "4",
		"B1": "=SUM(A1:A4)", "B2": "=A3*2", "B3": "=$A$4", "B4": "=SUM(A3:A4)",
	}
	tests := []struct {
		name string
		op   func(s *Sheet)
		want map[string]string
		vals map[string]float64
	}{
		{"insert inside a range", func(s *Sheet) { s.InsertRows(2, 1) }, map[string]string{
			"A1": "1", "A2": "2", "A4": "3", "A5": "4",
			"B1": "=SUM(A1:A5)", "B2": "=A4*2", "B4": "=$A$5", "B5": "=SUM(A4:A5)",
		}, map[string]float64{"B1": 10, "B2": 6, "B4": 4}},
		{"delete a referenced row", func(s *Sheet) { s.DeleteRows(2, 1) }, map[string]string{
			"A1": "1", "A2": "2", "A3": "4",
			"B1": "=SUM(A1:A3)", "B2": "=#REF!*2", "B3": "=SUM(A3:A3)",
		}, map[string]float64{"B1": 7, "B3": 4}},
		{"delete every row of a range", func(s *Sheet) { s.DeleteRows(2, 2) }, map[string]string{
			"A1": "1", "A2": "2", "B1": "=SUM(A1:A2)", "B2": "=#REF!*2",
		}, map[string]float64{"B1": 3}},
	}
	for _, tt := range tests {
		s := sheetOf(t, base)
		tt.op(s)
		if got := inputs(s); !maps.Equal(got, tt.want) {
			t.Errorf("%s:\n got %v\nwant %v", tt.name, got, tt.want)
		}
		for a, v := range tt.vals {
			if got := s.Value(at(a)); got.Kind != Number || got.Num != v {
				t.Errorf("%s: %s = %+v, want %v", tt.name, a, got, v)
			}
		}
		for a, in := range tt.want {
			if strings.Contains(in, "#REF!") && s.Value(at(a)) != ErrRef {
				t.Errorf("%s: %s = %+v, want #REF!", tt.name, a, s.Value(at(a)))
			}
		}
		// Recalc still follows the rewritten references.
		s.Set(at("A1"), "101")
		if got := s.Value(at("B1")).Num; got != tt.vals["B1"]+100 {
			t.Errorf("%s: B1 after edit = %v", tt.name, got)
		}
		// Undo restores the sheet exactly.
		s.Undo()
		s.Undo()
		if got := inputs(s); !maps.Equal(got, base) {
			t.Errorf("%s: undo left %v", tt.name, got)
		}
	}
}

func TestInsertDeleteColsShiftWidths(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "1", "B1": "2", "C1": "=A1+B1"})
	s.SetColWidth(1, 15)
	if err := s.InsertCols(1, 2); err != nil {
		t.Fatal(err)
	}
	if got := inputs(s); got["E1"] != "=A1+D1" || got["D1"] != "2" || s.ColWidth(3) != 15 || s.ColWidth(1) != DefaultWidth {
		t.Errorf("after insert: %v, width D %d", got, s.ColWidth(3))
	}
	s.DeleteCols(0, 1)
	if got := inputs(s)["D1"]; got != "=#REF!+C1" || s.ColWidth(2) != 15 {
		t.Errorf("after delete: D1 %q, width C %d", got, s.ColWidth(2))
	}
	c1, _ := s.Undo()
	c2, _ := s.Undo()
	if c1.Label != "delete 1 column" || c2.Label != "insert 2 columns" || c2.Focus != colRect(1, 2) {
		t.Errorf("undid %+v then %+v", c1, c2)
	}
	if got := inputs(s)["C1"]; got != "=A1+B1" || s.ColWidth(1) != 15 {
		t.Errorf("after undo: C1 %q width B %d", got, s.ColWidth(1))
	}
}

func TestInsertRefusesToPushDataOff(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1048576": "last"})
	if err := s.InsertRows(0, 1); err != ErrPushedOff {
		t.Errorf("err = %v", err)
	}
	if err := s.InsertRows(1048575, 1); err != ErrPushedOff {
		t.Errorf("err = %v", err)
	}
	if err := s.InsertRows(1048576-3, 1); err != ErrPushedOff {
		t.Errorf("err = %v", err)
	}
	s = sheetOf(t, map[string]string{"A1048574": "x", "B1": "=A1048576"})
	if err := s.InsertRows(0, 1); err != nil {
		t.Fatal(err)
	}
	if got := inputs(s)["B2"]; got != "=#REF!" {
		t.Errorf("reference pushed off the sheet = %q", got)
	}
}

func TestCopyPaste(t *testing.T) {
	tests := []struct {
		name   string
		src    string
		dst    string
		values bool
		want   map[string]string
		wrote  string
	}{
		{"relative shift", "C1", "D3", false, map[string]string{"D3": "=C3+$A$1"}, "D3"},
		{"off the sheet", "C1", "A1", false, map[string]string{"A1": "=#REF!+$A$1"}, "A1"},
		{"tiles an exact multiple", "A1:A2", "E1:F4", false, map[string]string{
			"E1": "1", "E2": "2", "E3": "1", "E4": "2", "F1": "1", "F2": "2", "F3": "1", "F4": "2",
		}, "E1:F4"},
		{"pastes once otherwise", "A1:A2", "E1:E3", false, map[string]string{"E1": "1", "E2": "2"}, "E1:E2"},
		{"values only", "B1:C1", "E5", true, map[string]string{"E5": "'12", "F5": "13"}, "E5:F5"},
	}
	for _, tt := range tests {
		s := sheetOf(t, map[string]string{"A1": "1", "A2": "2", "B1": "'12", "C1": "=B1+$A$1"})
		src, _ := ParseRange(tt.src)
		dst, _ := ParseRange(tt.dst)
		before := inputs(s)
		wrote, err := s.Paste(s.Copy(src), dst, tt.values)
		if err != nil || wrote.String() != tt.wrote {
			t.Errorf("%s: wrote %v, %v", tt.name, wrote, err)
		}
		got := inputs(s)
		for a, in := range tt.want {
			if got[a] != in {
				t.Errorf("%s: %s = %q, want %q", tt.name, a, got[a], in)
			}
		}
		if len(got) != len(uniqueKeys(before, tt.want)) {
			t.Errorf("%s: unexpected cells %v", tt.name, got)
		}
		s.Undo()
		if !maps.Equal(inputs(s), before) {
			t.Errorf("%s: undo left %v", tt.name, inputs(s))
		}
	}
}

func uniqueKeys(ms ...map[string]string) map[string]bool {
	out := map[string]bool{}
	for _, m := range ms {
		for k := range m {
			out[k] = true
		}
	}
	return out
}

func TestPasteBlanksAndEdge(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "1", "C1": "keep", "D1": "gone"})
	s.Paste(s.Copy(NewRect(at("A1"), at("B1"))), NewRect(at("C1"), at("C1")), false)
	if got := inputs(s); got["C1"] != "1" || got["D1"] != "" {
		t.Errorf("blank source cells should clear the destination: %v", got)
	}
	if _, err := s.Paste(s.Copy(NewRect(at("A1"), at("B1"))), NewRect(at("XFD1"), at("XFD1")), false); err != ErrPasteEdge {
		t.Errorf("err = %v", err)
	}
}

func TestMove(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"A1": "5", "A2": "=A1*2", "B1": "=A1+A2", "C1": "=SUM(A1:A2)", "C2": "=SUM(A1:A3)", "D1": "old", "E1": "=D1",
	})
	dst, err := s.Move(NewRect(at("A1"), at("A2")), at("D1"))
	if err != nil || dst.String() != "D1:D2" {
		t.Fatalf("Move = %v, %v", dst, err)
	}
	want := map[string]string{
		"D1": "5", "D2": "=D1*2", "B1": "=D1+D2", "C1": "=SUM(D1:D2)",
		"C2": "=SUM(A1:A3)", // not wholly inside the moved cells: unchanged
		"E1": "=#REF!",      // pointed at a cell the move overwrote
	}
	if got := inputs(s); !maps.Equal(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
	if got := s.Value(at("B1")).Num; got != 15 {
		t.Errorf("B1 = %v", got)
	}
	s.Undo()
	if got := inputs(s); got["A2"] != "=A1*2" || got["D1"] != "old" || got["E1"] != "=D1" {
		t.Errorf("undo: %v", got)
	}
}

func TestFill(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "1", "B1": "=A1*2", "A2": "2", "A3": "3"})
	if _, err := s.FillDown(NewRect(at("B1"), at("B3"))); err != nil {
		t.Fatal(err)
	}
	if got := inputs(s); got["B2"] != "=A2*2" || got["B3"] != "=A3*2" || s.Value(at("B3")).Num != 6 {
		t.Errorf("fill down: %v", got)
	}
	s.FillRight(NewRect(at("B1"), at("C2")))
	if got := inputs(s); got["C1"] != "=B1*2" || got["C2"] != "=B2*2" {
		t.Errorf("fill right: %v", got)
	}
	// A single cell fills from its neighbor.
	s.FillDown(NewRect(at("B4"), at("B4")))
	if got := inputs(s)["B4"]; got != "=A4*2" {
		t.Errorf("single-cell fill down: %q", got)
	}
	if err := s.FillEntry(NewRect(at("D1"), at("E2")), at("D1"), "=A1+$A$1"); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"D1": "=A1+$A$1", "E1": "=B1+$A$1", "D2": "=A2+$A$1", "E2": "=B2+$A$1"}
	for a, in := range want {
		if got := inputs(s)[a]; got != in {
			t.Errorf("fill entry %s = %q, want %q", a, got, in)
		}
	}
	s.Undo()
	if got := inputs(s)["D1"]; got != "" {
		t.Errorf("fill entry is one undo step, D1 = %q", got)
	}
}
