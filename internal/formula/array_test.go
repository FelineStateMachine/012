package formula

import (
	"strings"
	"testing"
)

// bindFuncs adds LET and LAMBDA to the test functions.
func bindFuncs(name string) (Func, bool) {
	switch name {
	case "LET":
		return testFunc{Name: "LET", Min: 3, Max: -1, Step: 2, Binds: BindLet}, true
	case "LAMBDA":
		return testFunc{Name: "LAMBDA", Min: 1, Max: -1, Binds: BindLambda}, true
	}
	return testFuncs(name)
}

func TestParseArrays(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"={1,2;3,4}", "={1,2;3,4}"},
		{"={ 1 , 2 }*2", "={1,2}*2"},
		{`={"a";"b"}`, `={"a";"b"}`},
		{"={A1:A3,B1:B3}", "={A1:A3,B1:B3}"},
		{"=SUM({1;2},3)", "=SUM({1;2},3)"},
		{"={-1,2+3}", "={-1,2+3}"},
	} {
		n, err := Parse(tt.in, testFuncs)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if got := Text(n); got != tt.want {
			t.Errorf("Text(Parse(%q)) = %q, want %q", tt.in, got, tt.want)
		}
	}
	for _, in := range []string{"={}", "={1,2", "={1 2}", "={1,}"} {
		if _, err := Parse(in, testFuncs); err == nil {
			t.Errorf("Parse(%q) should fail", in)
		}
	}
}

func TestParseLetAndLambda(t *testing.T) {
	for _, tt := range []struct{ in, want string }{
		{"=LET(x, 1, x+1)", "=LET(x,1,x+1)"},
		{"=LET(Total, A1:A3, SUM(total))", "=LET(Total,A1:A3,SUM(total))"},
		{"=LAMBDA(a, b, a*b)(2, 3)", "=LAMBDA(a,b,a*b)(2,3)"},
		{"=LET(f, LAMBDA(v, v*2), f(4))", "=LET(f,LAMBDA(v,v*2),f(4))"},
		{"=LET(x, 1, x) + x", "=LET(x,1,x)+x"},
	} {
		n, err := Parse(tt.in, bindFuncs)
		if err != nil {
			t.Errorf("Parse(%q): %v", tt.in, err)
			continue
		}
		if got := Text(n); got != tt.want {
			t.Errorf("Text(Parse(%q)) = %q, want %q", tt.in, got, tt.want)
		}
	}
	// Names bound by LET are locals; the same name outside is a name.
	n, _ := Parse("=LET(x, 1, x) + x", bindFuncs)
	var names []string
	WalkNames(n, func(nn Name) { names = append(names, nn.Name) })
	if strings.Join(names, ",") != "x" {
		t.Errorf("names outside LET = %v, want the one x", names)
	}
	call := n.(Binary).L.(Call)
	if _, ok := call.Args[2].(Local); !ok {
		t.Errorf("x inside LET = %T, want Local", call.Args[2])
	}
	for _, tt := range []struct{ in, want string }{
		{"=LET(x, 1)", "Wrong number of arguments to LET"},
		{"=LET(1, 2, 3)", "Argument 1 of LET must be a name"},
		{"=LET(A1, 2, A1)", "Argument 1 of LET must be a name"},
		{"=LAMBDA(1, 2)", "Argument 1 of LAMBDA must be a name"},
		{"=f(1)", "Unknown function F"},
	} {
		_, err := Parse(tt.in, bindFuncs)
		if err == nil || !strings.HasPrefix(err.Error(), tt.want) {
			t.Errorf("Parse(%q) = %v, want %q", tt.in, err, tt.want)
		}
	}
}

func TestRewriteArrays(t *testing.T) {
	n, err := Parse("={A1,B1;LET(x,C1,x),1}", bindFuncs)
	if err != nil {
		t.Fatal(err)
	}
	moved, ok := Rewrite(n, Shift(0, 1))
	if !ok || Text(moved) != "={A2,B2;LET(x,C2,x),1}" {
		t.Errorf("shifted = %s (%v)", Text(moved), ok)
	}
	if Text(n) != "={A1,B1;LET(x,C1,x),1}" {
		t.Errorf("the original changed: %s", Text(n))
	}
	var refs []string
	WalkRefs(n, func(_ string, a Addr) { refs = append(refs, a.String()) }, func(string, Rect) {})
	if strings.Join(refs, ",") != "A1,B1,C1" {
		t.Errorf("refs = %v", refs)
	}
}
