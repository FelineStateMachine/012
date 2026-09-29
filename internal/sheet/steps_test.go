package sheet

import (
	"strings"
	"testing"
)

// shown writes what Steps shows: values in [], the next part in _.
func shown(st *Steps) string {
	var out strings.Builder
	in := false
	for _, sg := range st.Text() {
		if sg.Next != in {
			out.WriteByte('_')
			in = sg.Next
		}
		t := sg.Text
		if sg.Value {
			t = "[" + t + "]"
		}
		out.WriteString(t)
	}
	if in {
		out.WriteByte('_')
	}
	return out.String()
}

// walk steps through the formula at a, returning what it shows before
// each step and at the end.
func walk(t *testing.T, s *Sheet, a string) []string {
	t.Helper()
	st := s.EvaluateSteps(at(a))
	if st == nil {
		t.Fatalf("%s: no steps", a)
	}
	var out []string
	for !st.Done() {
		out = append(out, shown(st))
		st.Step()
	}
	return append(out, shown(st))
}

func TestSteps(t *testing.T) {
	s := sheetOf(t, map[string]string{
		"A1": "5", "A2": "2", "A3": "x",
		"B1": "=A1+A2*3",
		"B2": "=IF(A1>3, A1*2, A2+1)",
		"B3": "=(A1+A2)*2",
		"B4": "=SUM(A1:A2*2)",
		"B5": "=LET(x, A1, x*2)",
		"B6": "=A3&D9",
		"B7": "=IFERROR(1/0, A2)",
		"B8": "=SEQUENCE(2)",
	})
	for _, tc := range []struct {
		cell string
		want string
	}{
		{"B1", "=_A1_+A2*3 | =[5]+_A2_*3 | =[5]+_[2]*3_ | =_[5]+[6]_ | [11]"},
		// The branch IF doesn't take is never computed.
		{"B2", "=IF(_A1_>3,A1*2,A2+1) | =IF(_[5]>3_,A1*2,A2+1) | =IF([TRUE],_A1_*2,A2+1) | =IF([TRUE],_[5]*2_,A2+1) | =_IF([TRUE],[10],A2+1)_ | [10]"},
		// The part computed next keeps its parentheses.
		{"B3", "=(_A1_+A2)*2 | =([5]+_A2_)*2 | =_([5]+[2])_*2 | =_[7]*2_ | [14]"},
		// In an array context a part computes an array.
		{"B4", "=SUM(_A1:A2*2_) | =_SUM([{10;4}])_ | [14]"},
		{"B5", "=LET(x,_A1_,x*2) | =LET(x,[5],_x*2_) | =_LET(x,[5],[10])_ | [10]"},
		{"B6", `=_A3_&D9 | =["x"]&_D9_ | =_["x"]&[0]_ | ["x"]`},
		{"B7", "=IFERROR(_1/0_,A2) | =IFERROR([#DIV/0!],_A2_) | =_IFERROR([#DIV/0!],[2])_ | [2]"},
		{"B8", "=_SEQUENCE(2)_ | [{1;2}]"},
	} {
		if got := strings.Join(walk(t, s, tc.cell), " | "); got != tc.want {
			t.Errorf("%s:\n got %s\nwant %s", tc.cell, got, tc.want)
		}
	}
	if s.EvaluateSteps(at("A1")) != nil {
		t.Error("steps for a number")
	}
}

func TestStepsDecimal(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "0.1", "B1": "=A1+0.2"})
	s.SetDecimal(true)
	if got := strings.Join(walk(t, s, "B1"), " | "); got != "=_A1_+0.2 | =_[0.1]+0.2_ | [0.3]" {
		t.Errorf("decimal: %s", got)
	}
}

func TestStepsInto(t *testing.T) {
	s := sheetOf(t, map[string]string{"A1": "4", "B1": "=A1*2", "C1": "=B1+Rate"})
	s.DefineName("Rate", rect("A1"))
	st := s.EvaluateSteps(at("C1"))
	if expr, v, _ := st.Next(); expr != "B1" || v != "8" {
		t.Errorf("next %q = %q", expr, v)
	}
	in := st.Into()
	if in == nil {
		t.Fatal("no stepping into B1")
	}
	if _, a := in.Cell(); a != at("B1") || shown(in) != "=_A1_*2" {
		t.Errorf("into %v: %s", a, shown(in))
	}
	if in.Into() != nil {
		t.Error("stepped into a number")
	}
	st.Step()
	if expr, v, _ := st.Next(); expr != "Rate" || v != "4" {
		t.Errorf("a name: %q = %q", expr, v)
	}
	if done, total := st.Progress(); done != 1 || total != 3 {
		t.Errorf("progress %d of %d", done, total)
	}
	st.Restart()
	if shown(st) != "=_B1_+Rate" {
		t.Errorf("restart: %s", shown(st))
	}
}
