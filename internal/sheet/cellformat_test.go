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

// FuzzFormatPattern checks that any pattern and value render without
// panicking, as TEXT() passes user patterns straight through.
func FuzzFormatPattern(f *testing.F) {
	for _, p := range []string{"#,##0.00", "0.0%", "0.00E+00", `"$"#,##0;(#,##0)`, "m/d/yyyy h:mm am/pm", "[h]:mm:ss.00", `\`, `"`, "[", "*", "_"} {
		f.Add(1234.5, p)
		f.Add(-1e300, p)
	}
	f.Fuzz(func(t *testing.T, v float64, pat string) {
		if len(pat) > 200 {
			return
		}
		FormatPattern(v, pat)
		Display(Value{Kind: Number, Num: v}, Format{Kind: FmtCustom, Pattern: pat}, 12)
	})
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

// An entry reads as it was typed until it's typed again: a number or
// formula formatted as Plain text afterwards keeps computing, and text
// typed into a Plain text cell stays text in another format or none,
// in the file too, and a formula that doesn't parse stays text without
// making the file unreadable.
func TestPlainTextFormatKeepsEntries(t *testing.T) {
	s := New()
	s.Set(at("A1"), "=1+1")
	s.Set(at("A2"), "5")
	s.SetFormat(rect("A1:A2"), Format{Kind: FmtText})
	s.SetFormat(rect("B1:B3"), Format{Kind: FmtText})
	s.Set(at("B1"), "=1+1")
	s.Set(at("B2"), "5")
	s.Set(at("B3"), "=SUM(")
	s.SetFormat(rect("B1:B1"), Format{Kind: FmtNumber, Decimals: 1})
	s.ClearFormatting(rect("B2:B3"))
	want := map[string]Value{"A1": num(2), "A2": num(5),
		"B1": {Kind: Text, Str: "=1+1"}, "B2": {Kind: Text, Str: "5"}, "B3": {Kind: Text, Str: "=SUM("}}
	for name, sh := range map[string]*Sheet{"edited": s, "reopened": roundTrip(t, s)} {
		for a, v := range want {
			if got := sh.Value(at(a)); got != v {
				t.Errorf("%s: %s = %+v, want %+v", name, a, got, v)
			}
		}
	}
}
