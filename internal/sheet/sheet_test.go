package sheet

import (
	"bytes"
	"errors"
	"testing"
)

func at(s string) Addr {
	a, ok := ParseAddr(s)
	if !ok {
		panic("bad address " + s)
	}
	return a
}

func TestAddr(t *testing.T) {
	tests := []struct {
		in   string
		want Addr
		ok   bool
	}{
		{"A1", Addr{0, 0}, true},
		{"z10", Addr{25, 9}, true},
		{"AA1", Addr{26, 0}, true},
		{"IV8192", Addr{255, 8191}, true},
		{"$B$2", Addr{1, 1}, true},
		{"IW1", Addr{}, false},
		{"A0", Addr{}, false},
		{"A8193", Addr{}, false},
		{"ABC1", Addr{}, false},
		{"A+1", Addr{}, false},
		{"1A", Addr{}, false},
	}
	for _, tt := range tests {
		got, ok := ParseAddr(tt.in)
		if ok != tt.ok || (ok && got != tt.want) {
			t.Errorf("ParseAddr(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
	for c := range MaxCols {
		if got, ok := ParseCol(ColName(c)); !ok || got != c {
			t.Fatalf("ParseCol(ColName(%d)) = %d, %v", c, got, ok)
		}
	}
}

func TestParseRange(t *testing.T) {
	for _, in := range []string{"B3..A1", "a1:b3", "$A$1..$B$3"} {
		r, ok := ParseRange(in)
		if !ok || r.String() != "A1..B3" {
			t.Errorf("ParseRange(%q) = %v, %v", in, r, ok)
		}
	}
}

func TestFormulas(t *testing.T) {
	s := New()
	for addr, in := range map[string]string{
		"A1": "10", "A2": "20", "A3": "30", "B1": "Name", "B2": `"right`,
	} {
		if err := s.Set(at(addr), in); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		in   string
		want Value
	}{
		{"+A1+A2*2", num(50)},
		{"(A1+A2)*2", num(60)},
		{"-2^2", num(-4)},
		{"=A1/4", num(2.5)},
		{"@SUM(A1..A3)", num(60)},
		{"@sum(A1:A3,5)", num(65)},
		{"@AVG(A1..A3)", num(20)},
		{"@COUNT(A1..B3)", num(5)},
		{"@MIN(A1..A3)", num(10)},
		{"@MAX(A1..A3)", num(30)},
		{"@ROUND(@PI,2)", num(3.14)},
		{"@MOD(7;3)", num(1)},
		{"@IF(A1>5,1,2)", num(1)},
		{"+A1>5#AND#A2<10", num(0)},
		{"#NOT#A1=10", num(0)},
		{"+B1+1", num(1)}, // labels count as zero
		{`+B1&" X"`, Value{Kind: Label, Str: "Name X"}},
		{"+A1/0", errValue},
		{"@SQRT(-1)", errValue},
		{"@NA", naValue},
		{"+@NA+1", naValue},
		{"1.5E3", num(1500)},
	}
	for _, tt := range tests {
		if err := s.Set(at("C1"), tt.in); err != nil {
			t.Errorf("Set(%q): %v", tt.in, err)
			continue
		}
		if got := s.Value(at("C1")); got != tt.want {
			t.Errorf("%q = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{"+A1+", "@SUM(A1", "@NOPE(1)", "+ZZ1", "(1", "+A1..", "1 2", `+"abc`, "#AND#"} {
		var pe *ParseError
		if err := New().Set(at("A1"), in); !errors.As(err, &pe) {
			t.Errorf("Set(%q) error = %v, want ParseError", in, err)
		}
	}
}

func TestLabels(t *testing.T) {
	s := New()
	s.Set(at("A1"), "Sales")
	s.Set(at("A2"), "^Mid")
	if c := s.Cell(at("A1")); c.Input != "'Sales" || c.Align() != '\'' || c.Value.Str != "Sales" {
		t.Errorf("A1 = %+v", c)
	}
	if c := s.Cell(at("A2")); c.Align() != '^' || c.Value.Str != "Mid" {
		t.Errorf("A2 = %+v", c)
	}
}

func TestRecalcPropagates(t *testing.T) {
	s := New()
	s.Set(at("A1"), "1")
	s.Set(at("A2"), "+A1*2")
	s.Set(at("A3"), "+A2+A1")
	s.Set(at("B1"), "@SUM(A1..A10)")

	s.Set(at("A1"), "5")
	if got := s.Value(at("A3")).Num; got != 15 {
		t.Errorf("A3 = %v, want 15", got)
	}
	if got := s.Value(at("B1")).Num; got != 30 {
		t.Errorf("B1 = %v, want 30", got)
	}

	// Filling a blank cell inside a range updates the range user.
	s.Set(at("A9"), "100")
	if got := s.Value(at("B1")).Num; got != 130 {
		t.Errorf("B1 = %v, want 130", got)
	}

	s.EraseRange(NewRect(at("A1"), at("A1")))
	if got := s.Value(at("A3")).Num; got != 0 {
		t.Errorf("A3 after erase = %v, want 0", got)
	}

	// Replacing a formula drops its old dependency edges.
	s.Set(at("A2"), "7")
	if len(s.dependents[at("A1")]) != 1 { // only A3 remains
		t.Errorf("A1 dependents = %v", s.dependents[at("A1")])
	}
}

func TestCircular(t *testing.T) {
	s := New()
	s.Set(at("A1"), "+B1+1")
	s.Set(at("B1"), "+A1+1")
	if !s.Circular || s.Value(at("A1")) != errValue || s.Value(at("B1")) != errValue {
		t.Errorf("circular: %v %+v %+v", s.Circular, s.Value(at("A1")), s.Value(at("B1")))
	}
	s.Set(at("B1"), "3")
	if s.Circular || s.Value(at("A1")).Num != 4 {
		t.Errorf("after break: %v %+v", s.Circular, s.Value(at("A1")))
	}
	s.Set(at("C1"), "@SUM(C1..C2)")
	if !s.Circular {
		t.Error("self-referencing range not flagged")
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		v     float64
		width int
		want  string
	}{
		{42, 9, "      42 "},
		{-3.5, 9, "    -3.5 "},
		{1.0 / 3, 9, "0.333333 "},
		{9.9999, 4, " 10 "},
		{123456789012, 9, "1.23E+11 "},
		{0.000000001, 9, "1.00E-09 "},
		{0, 3, " 0 "},
		{123456, 3, "** "},
	}
	for _, tt := range tests {
		if got := FormatValue(num(tt.v), tt.width); got != tt.want {
			t.Errorf("FormatValue(%v, %d) = %q, want %q", tt.v, tt.width, got, tt.want)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"+A1*2", "@SUM(A1..B3)", "@IF(A1>2#AND#B1,1,@NA)", `"a"&"b"`, "=1.5E-3^2"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		s := New()
		if err := s.Set(Addr{}, in); err == nil && in != "" && s.Cell(Addr{}) == nil {
			t.Fatalf("Set(%q) succeeded but cell is blank", in)
		}
	})
}

func TestFileRoundTrip(t *testing.T) {
	s := New()
	s.Set(at("A1"), "Total")
	s.Set(at("B1"), "@SUM(B2..B3)")
	s.Set(at("B2"), "2")
	s.Set(at("B3"), "+B2*10")
	s.SetColWidth(0, 14)

	var buf bytes.Buffer
	if err := s.Write(&buf); err != nil {
		t.Fatal(err)
	}
	got, err := Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if v := got.Value(at("B1")).Num; v != 22 {
		t.Errorf("B1 = %v, want 22", v)
	}
	if got.Cell(at("A1")).Input != "'Total" || got.ColWidth(0) != 14 {
		t.Errorf("A1 = %q, width %d", got.Cell(at("A1")).Input, got.ColWidth(0))
	}
}

func TestStripsControlChars(t *testing.T) {
	s := New()
	s.Set(at("A1"), "evil\x1b]52;c;aGk=\x07label")
	if got := s.Cell(at("A1")).Input; got != "'evil]52;c;aGk=label" {
		t.Errorf("Input = %q", got)
	}
}
