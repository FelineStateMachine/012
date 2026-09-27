package sheet

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// fixedNow pins TODAY() and entries without a year to 2026-09-26 14:30.
func fixedNow(t *testing.T) {
	t.Helper()
	old := Now
	Now = func() time.Time { return time.Date(2026, 9, 26, 14, 30, 0, 0, time.Local) }
	t.Cleanup(func() { Now = old })
}

func TestDisplay(t *testing.T) {
	tests := []struct {
		v     Value
		f     Format
		width int
		want  string
		align Align
	}{
		{num(1234.5), Preset(FmtNumber), 10, "1,234.50", AlignRight},
		{num(1234567.5), Preset(FmtNumber), 10, "#########", AlignRight}, // doesn't fit
		{num(0.125), Preset(FmtPercent), 10, "12.50%", AlignRight},
		{num(1234.5), Preset(FmtScientific), 10, "1.23E+03", AlignRight},
		{num(1450), Preset(FmtAccounting), 12, " $ 1,450.00 ", AlignFill},
		{num(-1450), Preset(FmtAccounting), 12, " $(1,450.00)", AlignFill},
		{num(0), Preset(FmtAccounting), 12, " $      -   ", AlignFill},
		{num(1450), Preset(FmtAccounting), 10, "$1,450.00 ", AlignFill},
		{num(-1450), Preset(FmtAccounting), 10, "#########", AlignRight},
		{num(-1450), Preset(FmtFinancial), 12, "(1,450.00)", AlignRight},
		{num(1450), Preset(FmtCurrency), 10, "$1,450.00", AlignRight},
		{num(1450.5), Format{Kind: FmtCurrency}, 10, "$1,451", AlignRight},
		{num(46291), Preset(FmtDate), 10, "9/26/2026", AlignRight},
		{num(46291.6041666667), Preset(FmtTime), 12, "2:30:00 PM", AlignRight},
		{num(46291.6041666667), Preset(FmtDateTime), 20, "9/26/2026 14:30:00", AlignRight},
		{num(1.5), Preset(FmtDuration), 10, "36:00:00", AlignRight},
		{num(42), Preset(FmtText), 10, "42", AlignLeft},
		{num(1.0 / 3), Format{}, 10, "0.3333333", AlignRight},
		{num(0.1 + 0.2), Format{}, 10, "0.3", AlignRight},
		{num(1.10000000001), Format{}, 10, "1.1", AlignRight},
		{txt("Rent"), Preset(FmtCurrency), 10, "Rent", AlignLeft},
		{boolean(true), Preset(FmtCurrency), 10, "TRUE", AlignCenter},
		{ErrNA, Format{}, 10, "#N/A", AlignCenter},
		{num(3), Format{Kind: FmtCustom, Pattern: "0.00"}, 10, "3.00", AlignRight},
	}
	for _, tt := range tests {
		got, al := Display(tt.v, tt.f, tt.width)
		if got != tt.want || al != tt.align {
			t.Errorf("Display(%v, %+v, %d) = %q, %v; want %q, %v", tt.v, tt.f, tt.width, got, al, tt.want, tt.align)
		}
	}
}

func TestEntryDetection(t *testing.T) {
	fixedNow(t)
	tests := []struct {
		in    string
		want  float64
		f     Format
		shown string // in a 30-wide cell
	}{
		{"42", 42, Format{}, "42"},
		{"$1,200", 1200, Format{Kind: FmtCurrency}, "$1,200"},
		{"$1,200.5", 1200.5, Preset(FmtCurrency), "$1,200.50"},
		{"-$5", -5, Format{Kind: FmtCurrency}, "-$5"},
		{"12%", 0.12, Format{Kind: FmtPercent}, "12%"},
		{"12.5%", 0.125, Preset(FmtPercent), "12.50%"},
		{"0.7%", 0.007, Preset(FmtPercent), "0.70%"}, // not 0.006999999999999999
		{"1e3%", 10, Format{Kind: FmtPercent}, "1000%"},
		{"1,234", 1234, Format{Kind: FmtNumber}, "1,234"},
		{"1.5e3", 1500, Preset(FmtScientific), "1.50E+03"},
		{"9/26/2026", 46291, Preset(FmtDate), "9/26/2026"},
		{"9/26/26", 46291, Preset(FmtDate), "9/26/2026"},
		{"9/26", 46291, Preset(FmtDate), "9/26/2026"}, // this year, as Sheets
		{"2026-09-26", 46291, Format{Kind: FmtDate, Pattern: "yyyy-mm-dd"}, "2026-09-26"},
		{"2026/9/26", 46291, Format{Kind: FmtDate, Pattern: "yyyy/mm/dd"}, "2026/09/26"},
		{"9-26-2026", 46291, Preset(FmtDate), "9/26/2026"},
		{"Sep 26, 2026", 46291, Format{Kind: FmtDate, Pattern: "mmm d, yyyy"}, "Sep 26, 2026"},
		{"september 26 2026", 46291, Format{Kind: FmtDate, Pattern: "mmmm d, yyyy"}, "September 26, 2026"},
		{"26 Sep 2026", 46291, Format{Kind: FmtDate, Pattern: "d mmm yyyy"}, "26 Sep 2026"},
		{"1/1/1930", 10959, Preset(FmtDate), "1/1/1930"},
		{"14:30", 14.5 / 24, Format{Kind: FmtTime, Pattern: "h:mm:ss"}, "14:30:00"},
		{"2:30 PM", 14.5 / 24, Preset(FmtTime), "2:30:00 PM"},
		{"2pm", 14.0 / 24, Preset(FmtTime), "2:00:00 PM"},
		{"12:15 am", 0.25 / 24, Preset(FmtTime), "12:15:00 AM"},
		{"1:02:03", (3600 + 120 + 3) / 86400.0, Format{Kind: FmtTime, Pattern: "h:mm:ss"}, "1:02:03"},
		{"25:30", 25.5 / 24, Preset(FmtDuration), "25:30:00"},
		{"9/26/2026 14:30", 46291 + 14.5/24, Preset(FmtDateTime), "9/26/2026 14:30:00"},
		{"9/26/2026 2:30 pm", 46291 + 14.5/24, Format{Kind: FmtDateTime, Pattern: "m/d/yyyy h:mm:ss am/pm"}, "9/26/2026 2:30:00 PM"},
	}
	for _, tt := range tests {
		s := New()
		if err := s.Set(at("A1"), tt.in); err != nil {
			t.Fatalf("Set(%q): %v", tt.in, err)
		}
		c := s.Cell(at("A1"))
		if c.Value.Kind != Number || !near(c.Value.Num, tt.want) || c.Format != tt.f {
			t.Errorf("%q = %+v %+v, want %v %+v", tt.in, c.Value, c.Format, tt.want, tt.f)
			continue
		}
		if got, _ := Display(c.Value, s.DisplayFormat(at("A1")), 30); got != tt.shown {
			t.Errorf("%q shows %q, want %q", tt.in, got, tt.shown)
		}
	}
	// Not dates or numbers: text, as in Sheets.
	for _, in := range []string{"2/30/2026", "13/1/2026", "1:60", "Sep", "May be", "10:5", "1.2.3", "12:30 xm", "Sep 40, 2026", "0x10", "inf", "1_000"} {
		s := New()
		s.Set(at("A1"), in)
		if v := s.Value(at("A1")); v.Kind != Text {
			t.Errorf("%q = %+v, want text", in, v)
		}
	}
}

func near(a, b float64) bool {
	d := a - b
	return d < 1e-9 && d > -1e-9
}

func TestFormatsPersistOnBlankCells(t *testing.T) {
	s := New()
	r := NewRect(at("B2"), at("B3"))
	s.SetFormat(r, Preset(FmtCurrency))
	s.SetStyle(r, func(st *Style) { st.Bold = true })
	if s.Len() != 0 {
		t.Errorf("Len = %d, want 0 for formatted blanks", s.Len())
	}
	if _, ok := s.UsedRange(); ok {
		t.Error("UsedRange counts formatted blanks")
	}
	if got := s.Edge(at("B1"), 0, 1); got != at("B1048576") {
		t.Errorf("Edge over formatted blanks = %v", got)
	}

	// Typing into a formatted blank uses its format, as in Sheets.
	s.Set(at("B2"), "1450")
	if got, _ := Display(s.Value(at("B2")), s.DisplayFormat(at("B2")), 10); got != "$1,450.00" {
		t.Errorf("B2 shows %q", got)
	}
	if !s.Cell(at("B2")).Style.Bold {
		t.Error("B2 lost bold")
	}
	// An entry that implies a format replaces it.
	s.Set(at("B2"), "12%")
	if got := s.Cell(at("B2")).Format; got != (Format{Kind: FmtPercent}) {
		t.Errorf("B2 format after 12%% = %+v", got)
	}

	// Delete clears contents but keeps formatting.
	s.EraseRange(r)
	if c := s.Cell(at("B2")); c == nil || !c.Blank() || c.Format.Kind != FmtPercent || !c.Style.Bold {
		t.Errorf("after erase B2 = %+v", c)
	}
	// Clear formatting drops the blank cells entirely.
	s.ClearFormatting(r)
	if s.Cell(at("B2")) != nil || s.Cell(at("B3")) != nil {
		t.Error("cells remain after clearing formatting")
	}
}

func TestPlainTextFormat(t *testing.T) {
	s := New()
	s.SetFormat(NewRect(at("A1"), at("A1")), Preset(FmtText))
	s.Set(at("A1"), "=1+1")
	if v := s.Value(at("A1")); v != txt("=1+1") {
		t.Errorf("plain text formula = %+v", v)
	}
	s.Set(at("A1"), "007")
	if v := s.Value(at("A1")); v != txt("007") {
		t.Errorf("plain text number = %+v", v)
	}
	// Applying plain text to a number keeps the number but shows it as text.
	s.Set(at("B1"), "12.50")
	s.SetFormat(NewRect(at("B1"), at("B1")), Preset(FmtText))
	if got, al := Display(s.Value(at("B1")), s.DisplayFormat(at("B1")), 10); got != "12.5" || al != AlignLeft {
		t.Errorf("B1 shows %q %v", got, al)
	}
}

func TestAdjustDecimals(t *testing.T) {
	tests := []struct {
		in    string
		f     Format
		delta int
		want  string
	}{
		{"1.5", Format{}, 1, "1.50"},
		{"1.5", Format{}, -1, "2"},
		{"1.2345", Format{}, -1, "1.235"},
		{"3", Format{}, 1, "3.0"},
		{"1234.5", Preset(FmtNumber), 1, "1,234.500"},
		{"1234.5", Preset(FmtNumber), -2, "1,235"},
		{"1234.5", Format{Kind: FmtNumber}, -1, "1,235"}, // stops at zero
		{"0.125", Preset(FmtPercent), -1, "12.5%"},
		{"$5", Format{}, 1, "$5.0"},
		{"9/26/2026", Format{}, 1, "9/26/2026"}, // dates don't change
	}
	for _, tt := range tests {
		s := New()
		r := NewRect(at("A1"), at("A1"))
		s.Set(at("A1"), tt.in)
		if !tt.f.IsZero() {
			s.SetFormat(r, tt.f)
		}
		s.AdjustDecimals(r, tt.delta)
		if got, _ := Display(s.Value(at("A1")), s.DisplayFormat(at("A1")), 20); got != tt.want {
			t.Errorf("%q %+v %+d = %q, want %q", tt.in, tt.f, tt.delta, got, tt.want)
		}
	}
}

func TestFileV2RoundTrip(t *testing.T) {
	fixedNow(t)
	s := New()
	s.Set(at("A1"), "Total")
	s.Set(at("B1"), "$1,200")
	s.Set(at("B2"), "12%")
	s.SetFormat(NewRect(at("B2"), at("B2")), Preset(FmtNumber)) // explicit format beats the entry's
	s.Set(at("C1"), "2026-09-26")
	s.SetStyle(NewRect(at("A1"), at("A1")), func(st *Style) { st.Bold, st.Align = true, AlignCenter })
	s.SetFormat(NewRect(at("D4"), at("D4")), Preset(FmtText)) // formatted blank
	s.Set(at("E1"), "=B1*2")

	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"version": 2`, `"E1": "=B1*2"`, `"A1": {"input":"Total","bold":true,"align":"center"}`,
		`"B1": {"input":"$1,200","format":"currency","decimals":0}`, `"D4": {"format":"text"}`,
		`"C1": {"input":"2026-09-26","format":"date","pattern":"yyyy-mm-dd"}`} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("file lacks %s:\n%s", want, buf.String())
		}
	}
	got, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []string{"A1", "B1", "B2", "C1", "D4", "E1"} {
		w, g := s.Cell(at(a)), got.Cell(at(a))
		if g == nil || g.Input != w.Input || g.Format != w.Format || g.Style != w.Style || g.Value != w.Value {
			t.Errorf("%s = %+v, want %+v", a, g, w)
		}
	}
	if f := got.DisplayFormat(at("E1")); f != (Format{Kind: FmtCurrency}) {
		t.Errorf("E1 inferred format after load = %+v", f)
	}
	// Typing into the loaded plain-text blank keeps text.
	got.Set(at("D4"), "0012")
	if v := got.Value(at("D4")); v != txt("0012") {
		t.Errorf("D4 = %+v", v)
	}
}

func TestFileV1StillLoads(t *testing.T) {
	v1 := `{"version": 1, "widths": {"A": 14}, "cells": {"A1": "Rent", "B1": "$1,450", "B2": "=B1*12"}}`
	s, err := Read(strings.NewReader(v1))
	if err != nil {
		t.Fatal(err)
	}
	if s.ColWidth(0) != 14 || s.Value(at("B2")).Num != 17400 {
		t.Errorf("width %d B2 %+v", s.ColWidth(0), s.Value(at("B2")))
	}
	// Entries imply formats on load, as if typed again.
	if f := s.DisplayFormat(at("B2")); f.Kind != FmtCurrency {
		t.Errorf("B2 format %+v", f)
	}
	for _, bad := range []string{
		`{"version": 4, "cells": {}}`,
		`{"version": 2, "cells": {"A1": {"format": "sparkly"}}}`,
		`{"version": 2, "cells": {"A1": {"align": "diagonal"}}}`,
		`{"version": 2, "cells": {"A1": 5}}`,
	} {
		if _, err := Read(strings.NewReader(bad)); err == nil {
			t.Errorf("Read(%s) succeeded", bad)
		}
	}
}
