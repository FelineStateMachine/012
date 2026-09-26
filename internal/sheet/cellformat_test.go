package sheet

import "testing"

// Undo snapshots cells by pointer, so formatting must store new cells
// rather than change stored ones.
func TestFormattingReplacesCells(t *testing.T) {
	s := New()
	s.Set(at("A1"), "5")
	r := NewRect(at("A1"), at("A1"))
	for _, change := range []func(){
		func() { s.SetFormat(r, Preset(FmtCurrency)) },
		func() { s.SetStyle(r, func(st *Style) { st.Bold = true }) },
		func() { s.AdjustDecimals(r, 1) },
		func() { s.ClearFormatting(r) },
		func() { s.EraseRange(r) },
	} {
		before := s.Cell(at("A1"))
		snapshot := *before
		change()
		if before.Format != snapshot.Format || before.Style != snapshot.Style || before.Input != snapshot.Input {
			t.Fatalf("a stored cell changed in place: %+v, was %+v", before, snapshot)
		}
		if s.Cell(at("A1")) == nil {
			s.SetFormat(r, Preset(FmtNumber)) // keep a cell for the next step
		}
	}
}

func TestBatchJoinsFormattingSteps(t *testing.T) {
	s := New()
	r := NewRect(at("A1"), at("B2"))
	s.Set(at("C1"), "=A1")
	s.Set(at("A1"), "5")
	s.Batch(Change{Label: "bold A1:B2", Focus: r}, func() error {
		s.SetStyle(r, func(st *Style) { st.Bold = true })
		s.SetFormat(r, Preset(FmtCurrency))
		return nil
	})
	if c := s.Cell(at("B2")); c == nil || !c.Style.Bold || c.Format.Kind != FmtCurrency {
		t.Errorf("B2 = %+v", c)
	}
	// Formulas following a reformatted cell are recalculated once the
	// batch ends.
	if f := s.DisplayFormat(at("C1")); f.Kind != FmtCurrency {
		t.Errorf("C1 format %+v", f)
	}
}
