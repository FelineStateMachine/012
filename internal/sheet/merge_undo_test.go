package sheet

import "testing"

func TestMergeFormattingUndo(t *testing.T) {
	s := New()
	s.Set(at("A1"), "5")
	r := NewRect(at("A1"), at("A2"))
	s.SetFormat(r, Preset(FmtCurrency))
	s.Batch(Change{Label: "bold A1:A2", Focus: r}, func() error {
		s.SetStyle(r, func(st *Style) { st.Bold = true })
		return nil
	})
	s.EraseRange(r)
	if c := s.Cell(at("A1")); c == nil || !c.Blank() || !c.Style.Bold {
		t.Fatalf("after clear %+v", c)
	}
	for _, want := range []string{"clear A1:A2", "bold A1:A2", "format A1:A2 as currency"} {
		ch, ok := s.Undo()
		if !ok || ch.Label != want {
			t.Fatalf("undo = %q %v, want %q", ch.Label, ok, want)
		}
	}
	if c := s.Cell(at("A1")); c.Format != (Format{}) || c.Style.Bold || c.Input != "5" {
		t.Errorf("A1 after undos %+v", c)
	}
	if s.Cell(at("A2")) != nil {
		t.Errorf("A2 after undos %+v", s.Cell(at("A2")))
	}
	s.Redo()
	if s.Cell(at("A2")) == nil || s.Cell(at("A2")).Format.Kind != FmtCurrency {
		t.Error("redo lost the blank's format")
	}
	// Copying and pasting carries formatting.
	s.Redo()
	s.Paste(s.Copy(NewRect(at("A1"), at("A1"))), NewRect(at("C1"), at("C1")), false)
	if c := s.Cell(at("C1")); c == nil || !c.Style.Bold || c.Format.Kind != FmtCurrency {
		t.Errorf("pasted C1 %+v", c)
	}
}

// A merge that deleting rows takes away frees the array it blocked.
func TestDeletedMergeFreesArray(t *testing.T) {
	s := New()
	s.Merge(NewRect(at("B18"), at("C20")), MergeVertically)
	s.Set(at("B17"), "=SEQUENCE(2)")
	if v := s.Value(at("B17")); v != ErrRef {
		t.Fatalf("B17 over a merge = %v", v)
	}
	s.DeleteRows(18, 3)
	if v := s.Value(at("B18")); v != num(2) {
		t.Errorf("B18 once the merge went = %v", v)
	}
}

// Unmerging frees the arrays blocked by the merges it takes away, where
// they reach past the range unmerged.
func TestUnmergeFreesArrayPastTheRange(t *testing.T) {
	s := New()
	s.Merge(NewRect(at("H11"), at("H13")), MergeVertically)
	s.Set(at("H11"), "=SEQUENCE(2)")
	if v := s.Value(at("H11")); v != ErrRef {
		t.Fatalf("H11 over a merge = %v", v)
	}
	if n := s.Unmerge(NewRect(at("G13"), at("H15"))); n != 1 {
		t.Fatalf("unmerged %d", n)
	}
	if v := s.Value(at("H12")); v != num(2) {
		t.Errorf("H12 once the merge went = %v", v)
	}
}
