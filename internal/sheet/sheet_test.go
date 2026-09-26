package sheet

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"unicode"
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
	for _, in := range []string{"B3..A1", "a1:b3", "$A$1:$B$3"} {
		r, ok := ParseRange(in)
		if !ok || r.String() != "A1:B3" {
			t.Errorf("ParseRange(%q) = %v, %v", in, r, ok)
		}
	}
}

func txt(s string) Value { return Value{Kind: Text, Str: s} }

func TestFormulas(t *testing.T) {
	s := New()
	for addr, in := range map[string]string{
		"A1": "10", "A2": "20", "A3": "30", "B1": "Name", "B2": "'42", "B3": "TRUE",
	} {
		if err := s.Set(at(addr), in); err != nil {
			t.Fatal(err)
		}
	}
	tests := []struct {
		in   string
		want Value
	}{
		{"=A1+A2*2", num(50)},
		{"=(A1+A2)*2", num(60)},
		{"=-2^2", num(-4)},
		{"=A1/4", num(2.5)},
		{"+A1*2", num(20)}, // Sheets accepts a leading +
		{"=SUM(A1:A3)", num(60)},
		{"=sum(A1:A3, 5)", num(65)},
		{"=@SUM(A1..A3)", num(60)}, // 1-2-3 function syntax still parses
		{"=AVERAGE(A1:B3)", num(20)},
		{"=AVG(A1:A3)", num(20)},
		{"=COUNT(A1:B3)", num(3)},  // numbers only
		{"=COUNTA(A1:B3)", num(6)}, // anything non-empty
		{"=MIN(A1:A3)", num(10)},
		{"=MAX(A1:A3)", num(30)},
		{"=ROUND(PI(), 2)", num(3.14)},
		{"=ROUND(2.5)", num(3)},
		{"=MOD(-7, 3)", num(2)},
		{"=INT(-1.5)", num(-2)},
		{`=IF(A1>5, "big", "small")`, txt("big")},
		{"=IF(A1>50, 1)", boolean(false)},
		{"=A1>5", boolean(true)},
		{"=AND(A1>5, A2<10)", boolean(false)},
		{"=OR(B3, FALSE)", boolean(true)},
		{"=NOT(A1=10)", boolean(false)},
		{"=B2*2", num(84)},     // numeric text coerces
		{"=B1+1", ErrValue},    // other text doesn't
		{"=SUM(B1)", ErrValue}, // nor as a direct argument
		{"=B3+1", num(2)},      // TRUE is 1
		{`=B1&" X"`, txt("Name X")},
		{`="say ""hi"""`, txt(`say "hi"`)},
		{"=A1&B3", txt("10TRUE")},
		{"=A1/0", ErrDiv0},
		{"=SQRT(-1)", ErrNum},
		{"=NA()", ErrNA},
		{"=NA()+1", ErrNA},
		{"=IFERROR(A1/0, 0)", num(0)},
		{"=Z99", Value{}},
		{"=10%", num(0.1)},
		{"=A1*50%", num(5)},
		{"=profit", ErrName},
		{"=1.5E3", num(1500)},
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

func TestEntryKinds(t *testing.T) {
	tests := []struct {
		in      string
		want    Value
		formula bool
	}{
		{"42", num(42), false},
		{"-3.5", num(-3.5), false},
		{"$1,200.50", num(1200.5), false},
		{"12%", num(0.12), false},
		{"1e3", num(1000), false},
		{"true", boolean(true), false},
		{"Rent", txt("Rent"), false},
		{"'42", txt("42"), false},
		{"1,23", txt("1,23"), false}, // bad grouping is text
		{"-hello", Value{Kind: Error, Str: "#NAME?"}, true},
		{"A1+1", txt("A1+1"), false}, // no = means text
		{"=1+1", num(2), true},
		{"-A1", num(0), true},
	}
	for _, tt := range tests {
		s := New()
		if err := s.Set(at("A2"), tt.in); err != nil {
			t.Errorf("Set(%q): %v", tt.in, err)
			continue
		}
		c := s.Cell(at("A2"))
		if c.Value != tt.want || c.IsFormula() != tt.formula || c.Input != tt.in {
			t.Errorf("%q = %+v formula=%v input=%q", tt.in, c.Value, c.IsFormula(), c.Input)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct{ in, msg string }{
		{"=A1+", "Formula is incomplete"},
		{"=SUM(A1", "Expected , or ) in SUM"},
		{"=NOPE(1)", "Unknown function NOPE"},
		{"=(1", "Missing )"},
		{"=A1:", "Expected a cell after :"},
		{"=1 2", "Unexpected 2"},
		{`="abc`, "Missing closing quote"},
		{"=ROUND()", "Wrong number of arguments to ROUND(value, [places])"},
		{"=PI(1)", "Wrong number of arguments to PI()"},
	}
	for _, tt := range tests {
		var pe *ParseError
		err := New().Set(at("A1"), tt.in)
		if !errors.As(err, &pe) || pe.Msg != tt.msg {
			t.Errorf("Set(%q) error = %v, want %q", tt.in, err, tt.msg)
		}
	}
}

func TestRecalcPropagates(t *testing.T) {
	s := New()
	s.Set(at("A1"), "1")
	s.Set(at("A2"), "=A1*2")
	s.Set(at("A3"), "=A2+A1")
	s.Set(at("B1"), "=SUM(A1:A10)")

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
	s.Set(at("A1"), "=B1+1")
	s.Set(at("B1"), "=A1+1")
	if !s.Circular || s.Value(at("A1")) != ErrRef || s.Value(at("B1")) != ErrRef {
		t.Errorf("circular: %v %+v %+v", s.Circular, s.Value(at("A1")), s.Value(at("B1")))
	}
	s.Set(at("B1"), "3")
	if s.Circular || s.Value(at("A1")).Num != 4 {
		t.Errorf("after break: %v %+v", s.Circular, s.Value(at("A1")))
	}
	s.Set(at("C1"), "=SUM(C1:C2)")
	if !s.Circular {
		t.Error("self-referencing range not flagged")
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		v     Value
		width int
		want  string
	}{
		{num(42), 10, "       42 "},
		{num(-3.5), 10, "     -3.5 "},
		{num(1.0 / 3), 10, "0.3333333 "},
		{num(9.9999), 4, " 10 "},
		{num(123456789012), 10, "1.235E+11 "},
		{num(0.000000001), 10, "1.000E-09 "},
		{num(0), 3, " 0 "},
		{num(123456), 3, "## "},
		{boolean(true), 10, "   TRUE   "},
		{ErrDiv0, 10, " #DIV/0!  "},
	}
	for _, tt := range tests {
		if got := FormatValue(tt.v, tt.width); got != tt.want {
			t.Errorf("FormatValue(%v, %d) = %q, want %q", tt.v, tt.width, got, tt.want)
		}
	}
}

func TestParseNumber(t *testing.T) {
	for in, want := range map[string]float64{
		"0": 0, "12": 12, "-12.5": -12.5, "+3": 3, ".5": 0.5, "1,234": 1234,
		"1,234,567.89": 1234567.89, "$99": 99, "-$5": -5, "50%": 0.5, "2.5e2": 250,
	} {
		if got, ok := ParseNumber(in); !ok || got != want {
			t.Errorf("ParseNumber(%q) = %v, %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "abc", "1,2", "12,34", "$", "%", "1.2.3", "--1", "1 2"} {
		if _, ok := ParseNumber(in); ok {
			t.Errorf("ParseNumber(%q) accepted", in)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"=A1*2", "=SUM(A1:B3)", "=IF(AND(A1>2,B1),1,NA())", `="a"&"b"`, "=1.5E-3^2", "$1,200", "12%", "@SUM(A1..B3)"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		s := New()
		printable := strings.ContainsFunc(in, func(r rune) bool { return !unicode.IsControl(r) })
		if err := s.Set(Addr{}, in); err == nil && printable && s.Cell(Addr{}) == nil {
			t.Fatalf("Set(%q) succeeded but cell is blank", in)
		}
	})
}

func TestFileRoundTrip(t *testing.T) {
	s := New()
	s.Set(at("A1"), "Total")
	s.Set(at("B1"), "=SUM(B2:B3)")
	s.Set(at("B2"), "2")
	s.Set(at("B3"), "=B2*10")
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
	if got.Cell(at("A1")).Input != "Total" || got.ColWidth(0) != 14 {
		t.Errorf("A1 = %q, width %d", got.Cell(at("A1")).Input, got.ColWidth(0))
	}
}

func TestStripsControlChars(t *testing.T) {
	s := New()
	s.Set(at("A1"), "evil\x1b]52;c;aGk=\x07label")
	if got := s.Cell(at("A1")).Input; got != "evil]52;c;aGk=label" {
		t.Errorf("Input = %q", got)
	}
}
