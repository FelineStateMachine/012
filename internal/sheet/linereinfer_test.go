package sheet

import "testing"

// Formulas reading blank cells of a column, row or sheet whose format
// changes infer their format again at once, not at the next recalc.
func TestLineFormatReinfers(t *testing.T) {
	w := NewBook()
	s := w.Sheets()[0]
	s.Set(at("D1"), "=B5*2")
	s.Set(at("D2"), "=SUM(B7:B9)")
	s.Set(at("D3"), "=C4")
	o, _ := w.AddSheet("Other", 1)
	o.Set(at("A1"), "=Sheet1!B6")
	s.SetFormat(col(1), Preset(FmtCurrency))
	for _, c := range []struct {
		s *Sheet
		a string
	}{{s, "D1"}, {s, "D2"}, {o, "A1"}} {
		if f := c.s.DisplayFormat(at(c.a)); f.Kind != FmtCurrency {
			t.Errorf("%s shows %v after formatting column B, want currency", c.a, f.Kind)
		}
	}
	if f := s.DisplayFormat(at("D3")); !f.IsZero() {
		t.Errorf("D3 shows %v, reading column C", f.Kind)
	}
	s.SetFormat(rowRect(3, 3), Preset(FmtPercent))
	if f := s.DisplayFormat(at("D3")); f.Kind != FmtPercent {
		t.Errorf("D3 shows %v after formatting row 4, want percent", f.Kind)
	}
	s.Undo()
	if f := s.DisplayFormat(at("D3")); !f.IsZero() {
		t.Errorf("D3 shows %v after undo", f.Kind)
	}
	s.SetFormat(Rect{To: Addr{Col: MaxCols - 1, Row: MaxRows - 1}}, Preset(FmtPercent))
	if f := s.DisplayFormat(at("D3")); f.Kind != FmtPercent {
		t.Errorf("D3 shows %v after formatting the sheet, want percent", f.Kind)
	}
}
