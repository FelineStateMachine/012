package formula

import (
	"reflect"
	"strings"
	"testing"
)

// testFunc is a function for the parser, with its signature only.
type testFunc Signature

func (f testFunc) Signature() Signature { return Signature(f) }

// testFuncs is a small function table: what the parser needs to know.
func testFuncs(name string) (Func, bool) {
	sigs := map[string]Signature{
		"SUM":    {Name: "SUM", Args: "value1, [value2, ...]", Min: 1, Max: -1},
		"IF":     {Name: "IF", Args: "condition, value_if_true, [value_if_false]", Min: 2, Max: 3},
		"PI":     {Name: "PI", Max: 0},
		"ROUND":  {Name: "ROUND", Args: "value, [places]", Min: 1, Max: 2},
		"SUMIFS": {Name: "SUMIFS", Args: "sum_range, criteria_range1, criterion1, ...", Min: 3, Max: -1, Step: 2},
	}
	sig, ok := sigs[name]
	return testFunc(sig), ok
}

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

func TestParseAbsoluteRefs(t *testing.T) {
	tests := []struct {
		in   string
		want Node
	}{
		{"=A1", Ref{at("A1"), 0, ""}},
		{"=$A1", Ref{at("A1"), AbsCol, ""}},
		{"=A$1", Ref{at("A1"), AbsRow, ""}},
		{"=$a$1", Ref{at("A1"), AbsCol | AbsRow, ""}},
		{"=$B$3:A1", Range{NewRect(at("A1"), at("B3")), [2]Abs{0, AbsCol | AbsRow}, ""}},
		// Each marker stays with its column or row when corners swap.
		{"=$B1:A$2", Range{NewRect(at("A1"), at("B2")), [2]Abs{0, AbsCol | AbsRow}, ""}},
		{"=A1..$B2", Range{NewRect(at("A1"), at("B2")), [2]Abs{0, AbsCol}, ""}},
		{"=A$$1", Name{"A$$1"}},
		{"=A1$", Name{"A1$"}},
		{"=#REF!+1", Binary{Op: "+", L: RefErr{}, R: Num{1}}},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in, testFuncs)
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Parse(%q) = %#v, %v; want %#v", tt.in, got, err, tt.want)
		}
	}
}

func TestPrintFormula(t *testing.T) {
	tests := []struct{ in, want string }{
		{"=a1+$b$2*C$3", "=A1+$B$2*C$3"},
		{"=@SUM(A1..B3; 2)", "=SUM(A1:B3,2)"},
		{"=(1+2)*3", "=(1+2)*3"},
		{"=1-(2-3)", "=1-(2-3)"},
		{"=(1-2)-3", "=1-2-3"},
		{"=2^3^2", "=2^3^2"},
		{"=2^(3^2)", "=2^(3^2)"},
		{"=-2^2", "=-2^2"},
		{"=(-2)^2", "=-2^2"}, // negation binds tighter than ^
		{"=-(2^2)", "=-(2^2)"},
		{"=-(A1+1)", "=-(A1+1)"},
		{"=50%", "=50%"},
		{"=(A1+1)%", "=(A1+1)%"},
		{"=#NOT#(A1=1) #AND# B1", "=#NOT#A1=1#AND#B1"},
		{"=1+(#NOT#A1)", "=1+(#NOT#A1)"},
		{`="say ""hi"""&A1`, `="say ""hi"""&A1`},
		{"=1.5E3+1e-9+2E21", "=1500+1E-09+2E+21"},
		{"=@PI", "=PI()"},
		{"=IF(A1>=2,TRUE,profit)", "=IF(A1>=2,TRUE,profit)"},
		{"=SUM(#REF!)", "=SUM(#REF!)"},
		{"+A1", "=+A1"},
	}
	for _, tt := range tests {
		n, err := Parse(tt.in, testFuncs)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tt.in, err)
		}
		if got := Text(n); got != tt.want {
			t.Errorf("print(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestShiftRefs(t *testing.T) {
	tests := []struct {
		in     string
		dc, dr int
		want   string
	}{
		{"=A1+B2", 1, 2, "=B3+C4"},
		{"=$A$1+A1", 1, 1, "=$A$1+B2"},
		{"=$A1+A$1", 2, 3, "=$A4+C$1"},
		{"=SUM(A1:B2)", 0, 5, "=SUM(A6:B7)"},
		{"=SUM($A$1:B2)", 1, 1, "=SUM($A$1:C3)"},
		{"=A2", 0, -1, "=A1"},
		{"=A2", 0, -2, "=#REF!"},
		{"=SUM(A1:B2)+1", -1, 0, "=SUM(#REF!)+1"},
		{"=$A2", -1, 0, "=$A2"},
		{"=IV1", 1, 0, "=#REF!"},
		// A mixed range can flip when one corner is anchored.
		{"=SUM($C1:D1)", -2, 0, "=SUM(B1:$C1)"},
	}
	for _, tt := range tests {
		n, _ := Parse(tt.in, testFuncs)
		out, _ := Rewrite(n, Shift(tt.dc, tt.dr))
		if got := Text(out); got != tt.want {
			t.Errorf("shift(%q, %d, %d) = %q, want %q", tt.in, tt.dc, tt.dr, got, tt.want)
		}
	}
}

func TestSpanInterval(t *testing.T) {
	tests := []struct {
		sp         Span
		lo, hi     int
		wLo, wHi   int
		wOK        bool
		name       string
		checkPoint bool
	}{
		{Span{2, 2, 100}, 0, 1, 0, 1, true, "insert below", false},
		{Span{2, 2, 100}, 1, 3, 1, 5, true, "insert inside grows", false},
		{Span{2, 2, 100}, 2, 3, 4, 5, true, "insert at start shifts", false},
		{Span{2, 2, 100}, 50, 99, 52, 99, true, "insert clamps at the edge", false},
		{Span{2, -2, 100}, 1, 4, 1, 2, true, "delete inside shrinks", false},
		{Span{2, -2, 100}, 2, 4, 2, 2, true, "delete top part", false},
		{Span{2, -2, 100}, 1, 2, 1, 1, true, "delete bottom part", false},
		{Span{2, -2, 100}, 3, 6, 2, 4, true, "delete overlapping start", false},
		{Span{2, -2, 100}, 5, 6, 3, 4, true, "delete above shifts", false},
		{Span{2, -2, 100}, 2, 3, 0, 0, false, "delete all of it", false},
	}
	for _, tt := range tests {
		lo, hi, ok := tt.sp.Interval(tt.lo, tt.hi)
		if ok != tt.wOK || (ok && (lo != tt.wLo || hi != tt.wHi)) {
			t.Errorf("%s: interval(%d, %d) = %d, %d, %v", tt.name, tt.lo, tt.hi, lo, hi, ok)
		}
	}
}

func TestParseSheetRefs(t *testing.T) {
	tests := []struct{ in, out, err string }{
		{in: "=Sheet2!A1", out: "=Sheet2!A1"},
		{in: "=sheet2!$a$1+1", out: "=sheet2!$A$1+1"},
		{in: "='My Sheet'!B2:C9", out: "='My Sheet'!B2:C9"},
		{in: "=SUM('Bob''s'!A1:A3)", out: "=SUM('Bob''s'!A1:A3)"},
		{in: "=Data!B3:Data!A1", out: "=Data!A1:B3"},
		{in: "='2026'!A1", out: "='2026'!A1"},
		{in: "='A1'!A1", out: "='A1'!A1"},
		{in: "=Q3.plan!A1..B2", out: "=Q3.plan!A1:B2"},
		{in: "=Sheet2!Sales", err: "Expected a cell after Sheet2!"},
		{in: "=Sheet2!A1:Other!B2", err: "A range can't span sheets"},
		{in: "='Open!A1", err: "Expected ! after a quoted sheet name"},
		{in: "='x'A1", err: "Expected ! after a quoted sheet name"},
	}
	for _, tt := range tests {
		n, err := Parse(tt.in, testFuncs)
		switch {
		case tt.err != "":
			if err == nil || err.Error() != tt.err {
				t.Errorf("%s: error %v, want %q", tt.in, err, tt.err)
			}
		case err != nil:
			t.Errorf("%s: %v", tt.in, err)
		case Text(n) != tt.out:
			t.Errorf("%s prints as %s, want %s", tt.in, Text(n), tt.out)
		}
	}
}

func TestQuoteSheet(t *testing.T) {
	for name, want := range map[string]string{
		"Sheet1": "Sheet1", "Q3_plan": "Q3_plan", "My Sheet": "'My Sheet'", "Bob's": "'Bob''s'",
		"2026": "'2026'", "AB12": "'AB12'", "R1C1": "'R1C1'", "Café": "'Café'",
	} {
		if got := QuoteSheet(name); got != want {
			t.Errorf("QuoteSheet(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestSplitSheet(t *testing.T) {
	tests := []struct{ in, sheet, rest string }{
		{"A1", "", "A1"},
		{"Sheet2!A1:B3", "Sheet2", "A1:B3"},
		{"'Q3 plan'!B2", "Q3 plan", "B2"},
		{"'Bob''s'!C1", "Bob's", "C1"},
		{" Data!Z9 ", "Data", "Z9"},
	}
	for _, tt := range tests {
		if sheet, rest := SplitSheet(tt.in); sheet != tt.sheet || rest != tt.rest {
			t.Errorf("SplitSheet(%q) = %q, %q", tt.in, sheet, rest)
		}
	}
}

// FuzzPrint checks that printing a parsed formula gives text that parses
// back to the same expression.
func FuzzPrint(f *testing.F) {
	for _, s := range []string{
		"=A1*2", "=$A$1+A$2-$B3", "=SUM($A1:B$3)", "=-2^-2%", "=1-(2-3)", "=#NOT#A1=1#OR#B1",
		"=2^(3^2)", `=IF(A1>2,"x""y",#REF!)`, "=1.5E-3^2", "=@SUM(A1..B3)", "=(1+2)%*-A1",
		// At and past the nesting cap.
		"=" + strings.Repeat("(", MaxDepth-1) + "1" + strings.Repeat(")", MaxDepth-1),
		"=" + strings.Repeat("-(", MaxDepth/2) + "1" + strings.Repeat(")", MaxDepth/2),
		"=" + strings.Repeat("(", MaxDepth+1) + "1" + strings.Repeat(")", MaxDepth+1),
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, in string) {
		n, err := Parse(in, testFuncs)
		if err != nil {
			return
		}
		text := Text(n)
		back, err := Parse(text, testFuncs)
		if err != nil {
			t.Fatalf("%q printed as %q, which doesn't parse: %v", in, text, err)
		}
		if !reflect.DeepEqual(n, back) {
			t.Fatalf("%q printed as %q parses differently:\n%#v\n%#v", in, text, n, back)
		}
	})
}

// Nesting is capped at MaxDepth levels, with the error at the level
// past it; anything within the cap parses.
func TestParseDepth(t *testing.T) {
	nested := func(n int) string {
		return "=" + strings.Repeat("(", n) + "1" + strings.Repeat(")", n)
	}
	if _, err := Parse(nested(MaxDepth-1), testFuncs); err != nil {
		t.Errorf("%d parentheses: %v", MaxDepth-1, err)
	}
	for _, in := range []string{
		nested(MaxDepth),
		nested(100_000),
		"=" + strings.Repeat("-", 5000) + "1",
		"=" + strings.Repeat("SUM(", 2000) + "1" + strings.Repeat(")", 2000),
		"=" + strings.Repeat("(", 5000),
	} {
		_, err := Parse(in, testFuncs)
		pe, ok := err.(*ParseError)
		if !ok || !strings.HasPrefix(pe.Msg, "Formula is nested too deeply") {
			t.Errorf("Parse(%.20q...) error = %v", in, err)
			continue
		}
		if pe.Pos < MaxDepth || pe.Pos > len(in) {
			t.Errorf("Parse(%.20q...) error at %d", in, pe.Pos)
		}
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		in  string
		pos int
		msg string
	}{
		{"=A1+", 4, "Formula is incomplete"},
		{"=SUM(A1", 7, "Expected , or ) in SUM"},
		{"=NOPE(1)", 1, "Unknown function NOPE"},
		{"=(1", 3, "Missing )"},
		{"=A1:", 4, "Expected a cell after :"},
		{"=1 2", 3, "Unexpected 2"},
		{`="abc`, 1, "Missing closing quote"},
		{"=ROUND()", 1, "Wrong number of arguments to ROUND(value, [places])"},
		{"=PI(1)", 1, "Wrong number of arguments to PI()"},
		{"=SUMIFS(A1:A3,B1:B3,1,C1:C3)", 1, "Wrong number of arguments to SUMIFS(sum_range, criteria_range1, criterion1, ...)"},
		{"=1#XOR#2", 2, "Unknown operator #XOR#"},
		{"=1#2", 2, "Unexpected #"},
		{"=@(1)", 1, "Missing function name after @"},
		{"=1!", 2, `Unexpected '!'`},
	}
	for _, tt := range tests {
		_, err := Parse(tt.in, testFuncs)
		pe, ok := err.(*ParseError)
		if !ok || pe.Msg != tt.msg || pe.Pos != tt.pos {
			t.Errorf("Parse(%q) error = %#v, want %q at %d", tt.in, err, tt.msg, tt.pos)
		}
	}
}

func TestLexLiterals(t *testing.T) {
	tests := []struct {
		in   string
		want Node
	}{
		{`="say ""hi"""`, Str{`say "hi"`}},
		{`=""""`, Str{`"`}},
		{`="plain"`, Str{"plain"}},
		{"=.5", Num{0.5}},
		{"=1.5e+3", Num{1500}},
		{"=#ref!", RefErr{}},
		{"=SUMIFS(A1:A3,B1:B3,1)", nil},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in, testFuncs)
		if tt.want == nil {
			if err != nil {
				t.Errorf("Parse(%q): %v", tt.in, err)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("Parse(%q) = %#v, %v; want %#v", tt.in, got, err, tt.want)
		}
	}
}
