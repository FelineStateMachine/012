package sheet

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

// decimalSheet returns a sheet with inputs placed down column A from A1
// and the formula in B1, in the given arithmetic.
func decimalSheet(t *testing.T, decimal bool, formula string, inputs ...string) *Sheet {
	t.Helper()
	s := New()
	for i, in := range inputs {
		if err := s.Set(Addr{Row: i}, in); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Set(at("B1"), formula); err != nil {
		t.Fatal(err)
	}
	s.SetDecimal(decimal)
	return s
}

// The cases where binary and decimal arithmetic differ, and some where
// they must agree. Values are compared as General shows them with every
// digit (strconv's shortest form), so 0.30000000000000004 is visible.
func TestDecimalArithmetic(t *testing.T) {
	tenCents := strings.Split(strings.Repeat("0.1,", 10), ",")[:10]
	tests := []struct {
		name          string
		formula       string
		inputs        []string
		binary, decim string
	}{
		{"sum of two", "=0.1+0.2", nil, "0.30000000000000004", "0.3"},
		{"comparison", "=0.1+0.2=0.3", nil, "TRUE", "TRUE"}, // at the 15 digits shown
		{"comparison with zero", "=0.1+0.2-0.3=0", nil, "FALSE", "TRUE"},
		{"residue", "=1.1-1-0.1", nil, "8.326672684688674E-17", "0"},
		{"product", "=0.07*3", nil, "0.21000000000000002", "0.21"},
		{"cents to whole", "=INT(A1*100)", []string{"$4.35"}, "434", "435"},
		{"currency sum", "=SUM(A1:A10)", tenCents, "0.9999999999999999", "1"},
		{"currency sum equals", "=SUM(A1:A10)-1=0", tenCents, "FALSE", "TRUE"},
		{"average", "=AVERAGE(0.1, 0.2)", nil, "0.15000000000000002", "0.15"},
		{"division", "=0.3/0.1", nil, "2.9999999999999996", "3"},
		{"postfix percent", "=1.1%", nil, "0.011000000000000001", "0.011"},
		{"round of a residue", "=ROUND(0.1+0.2-0.3, 20)", nil, "5.551E-17", "0"},
		// Rounding already works on the digits a cell shows, so these agree.
		{"round half up", "=ROUND(2.675, 2)", nil, "2.68", "2.68"},
		{"round 1.005", "=ROUND(1.005, 2)", nil, "1.01", "1.01"},
		{"round negative half", "=ROUND(-2.5)", nil, "-3", "-3"},
		{"round tens", "=ROUND(1234.5, -2)", nil, "1200", "1200"},
		{"roundup", "=ROUNDUP(2.301, 1)", nil, "2.4", "2.4"},
		{"rounddown", "=ROUNDDOWN(-2.39, 1)", nil, "-2.3", "-2.3"},
		{"trunc", "=TRUNC((0.1+0.7)*10)", nil, "8", "8"},
		// Typed percentages are exact decimals on entry in both modes.
		{"typed percent", "=A1", []string{"0.7%"}, "0.007", "0.007"},
		// Division rounds: decimal doesn't make thirds whole again.
		{"thirds", "=1/3*3", nil, "1", "0.9999999999999999"},
		{"one third", "=1/3", nil, "0.3333333333333333", "0.3333333333333333"},
		// Outside the boundary, and errors, nothing changes.
		{"power stays binary", "=1.1^2", nil, "1.2100000000000002", "1.2100000000000002"},
		{"sqrt stays binary", "=SQRT(2)", nil, "1.4142135623730951", "1.4142135623730951"},
		{"div by zero", "=1/0", nil, "#DIV/0!", "#DIV/0!"},
		{"overflow", "=1E308*10", nil, "#NUM!", "#NUM!"},
		{"text", `="a"+1`, nil, "#VALUE!", "#VALUE!"},
		{"sum of text", `=SUM("a")`, nil, "#VALUE!", "#VALUE!"},
		{"average of nothing", "=AVERAGE(A1:A2)", nil, "#DIV/0!", "#DIV/0!"},
		{"sum ignores text in ranges", "=SUM(A1:A3)", []string{"0.1", "x", "0.2"}, "0.30000000000000004", "0.3"},
		{"huge sum falls back", "=SUM(1E308, 1E308)", nil, "#NUM!", "#NUM!"},
		// PRODUCT, SUMIF, SUMIFS and SUMPRODUCT have decimal twins too.
		{"product", "=PRODUCT(A1:A2)", []string{"1.1", "1.1"}, "1.2100000000000002", "1.21"},
		{"product of direct", "=PRODUCT(0.07, 3)", nil, "0.21000000000000002", "0.21"},
		{"product skips text in ranges", "=PRODUCT(A1:A3)", []string{"1.1", "x", "1.1"}, "1.2100000000000002", "1.21"},
		{"product of nothing", "=PRODUCT(A1:A2)", nil, "0", "0"},
		{"product of text", `=PRODUCT("a")`, nil, "#VALUE!", "#VALUE!"},
		{"huge product falls back", "=PRODUCT(1E308, 10)", nil, "#NUM!", "#NUM!"},
		{"sumif", `=SUMIF(A1:A10, ">0")`, tenCents, "0.9999999999999999", "1"},
		{"sumif equals", `=SUMIF(A1:A10, ">0")-1=0`, tenCents, "FALSE", "TRUE"},
		{"sumif with sum range", `=SUMIF(A1:A3, "<>x", A1:A3)`, []string{"0.1", "x", "0.2"}, "0.30000000000000004", "0.3"},
		{"sumif error", `=SUMIF(A1:A2, ">0")`, []string{"1", "=1/0"}, "1", "1"},
		{"sumifs", `=SUMIFS(A1:A10, A1:A10, "0.1")`, tenCents, "0.9999999999999999", "1"},
		{"sumifs mismatched", `=SUMIFS(A1:A10, A1:A9, ">0")`, tenCents, "#VALUE!", "#VALUE!"},
		{"sumproduct", "=SUMPRODUCT(A1:A3, A1:A3)", []string{"0.1", "0.2", "0.3"}, "0.14", "0.14"},
		{"sumproduct of cents", "=SUMPRODUCT(A1:A2, A3:A4)", []string{"0.1", "0.2", "3", "3"}, "0.9000000000000001", "0.9"},
		{"sumproduct mismatched", "=SUMPRODUCT(A1:A2, A1:A3)", []string{"1", "2", "3"}, "#VALUE!", "#VALUE!"},
		{"sumproduct error", "=SUMPRODUCT(A1:A2)", []string{"1", "=1/0"}, "#DIV/0!", "#DIV/0!"},
	}
	show := func(v Value) string {
		if v.Kind == Number {
			return formatShortest(v.Num)
		}
		return v.String()
	}
	for _, tt := range tests {
		for _, dec := range []bool{false, true} {
			want := tt.binary
			if dec {
				want = tt.decim
			}
			s := decimalSheet(t, dec, tt.formula, tt.inputs...)
			if got := show(s.Value(at("B1"))); got != want {
				t.Errorf("%s: %s with decimal=%v = %s, want %s", tt.name, tt.formula, dec, got, want)
			}
		}
	}
}

// Turning the setting on and off recalculates, is one undo step, and
// undo and redo recalculate too.
func TestDecimalUndo(t *testing.T) {
	s := decimalSheet(t, false, "=0.1+0.2-0.3=0")
	s.ClearHistory()
	check := func(when string, want bool) {
		t.Helper()
		if got := s.Value(at("B1")); got != boolean(want) || s.Decimal() != want {
			t.Errorf("%s: B1 = %v, Decimal() = %v; want %v", when, got, s.Decimal(), want)
		}
	}
	check("start", false)
	s.SetDecimal(true)
	check("on", true)
	s.SetDecimal(true) // no-op, no step
	if c, ok := s.Undo(); !ok || c.Label != "turn on decimal arithmetic" {
		t.Fatalf("undo = %+v, %v", c, ok)
	}
	check("undone", false)
	if s.CanUndo() {
		t.Error("the no-op made an undo step")
	}
	s.Redo()
	check("redone", true)
	s.SetDecimal(false)
	check("off", false)
}

// The setting round-trips through the file without a version bump, and
// a file without it computes in binary.
func TestDecimalFile(t *testing.T) {
	s := decimalSheet(t, true, "=A1+A2", "0.1", "0.2")
	var b bytes.Buffer
	if err := s.Write(&b); err != nil {
		t.Fatal(err)
	}
	out := b.String()
	if !strings.Contains(out, `"version": 2,`) || !strings.Contains(out, `"arithmetic": "decimal",`) {
		t.Fatalf("file:\n%s", out)
	}
	r, err := Read(strings.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if !r.Decimal() || r.Value(at("B1")).Num != 0.3 {
		t.Errorf("read back: decimal %v, B1 %v", r.Decimal(), r.Value(at("B1")))
	}
	if r.CanUndo() {
		t.Error("loading made an undo step")
	}
	for _, mode := range []string{"", `"arithmetic": "binary",`, `"arithmetic": "quantum",`} {
		in := strings.Replace(out, `"arithmetic": "decimal",`, mode, 1)
		r, err := Read(strings.NewReader(in))
		if err != nil || r.Decimal() {
			t.Errorf("%q: decimal %v, %v", mode, r != nil && r.Decimal(), err)
		}
	}
	s.SetDecimal(false)
	b.Reset()
	if err := s.Write(&b); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(b.String(), "arithmetic") {
		t.Errorf("binary sheet mentions arithmetic:\n%s", b.String())
	}
}

// Formats inferred from the formula survive the decimal twins: a sum of
// currency is currency in both modes.
func TestDecimalKeepsFormats(t *testing.T) {
	for _, dec := range []bool{false, true} {
		s := decimalSheet(t, dec, "=SUM(A1:A2)", "$1.10", "$2.20")
		if f := s.DisplayFormat(at("B1")); f.Kind != FmtCurrency {
			t.Errorf("decimal=%v: format %v", dec, f)
		}
		if got, _ := Display(s.Value(at("B1")), s.DisplayFormat(at("B1")), 12); got != "$3.30" {
			t.Errorf("decimal=%v: shows %q", dec, got)
		}
	}
}

func formatShortest(f float64) string {
	return strings.ToUpper(strconv.FormatFloat(f, 'g', -1, 64))
}

func BenchmarkArithmetic(b *testing.B) {
	for _, dec := range []bool{false, true} {
		name := "binary"
		if dec {
			name = "decimal"
		}
		b.Run(name, func(b *testing.B) {
			s := New()
			for r := range 1000 {
				s.Set(Addr{Row: r}, "19.99")
				s.Set(Addr{Col: 1, Row: r}, "=A"+strconv.Itoa(r+1)+"*1.0825+0.01")
			}
			s.Set(at("C1"), "=SUM(B1:B1000)")
			s.SetDecimal(dec)
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				s.Set(at("A1"), "20.01")
			}
		})
	}
}
