package formula

import (
	"strings"
	"testing"
)

func TestScanCaret(t *testing.T) {
	for _, tc := range []struct {
		text string // | marks the caret
		word string
		fn   string
		arg  int
	}{
		{"=SU|", "SU", "", 0},
		{"=@su|", "su", "", 0},
		{"=SUM(|", "", "SUM", 0},
		{"=SUM(A1, B|", "B", "SUM", 1},
		{"=IF(A1>2, SUM(B1; B2), |", "", "IF", 2},
		{"=IF(A1, (1+|", "", "IF", 1},
		{`=IF(A1="a,b(", |`, "", "IF", 1},
		{`=CONCAT("SU|`, "", "", 0},
		{"=SUM (1, |", "", "SUM", 1},
		{"=jev.te|", "jev.te", "", 0},
		{"=A1+1|", "", "", 0},
		{"=SU|M(1)", "", "", 0},
		{"=Sales|", "Sales", "", 0},
		// Sheet names: quoted ones are a word from the quote on, and
		// what's inside the quotes doesn't open calls or arguments.
		{"=SUM('Q3 p|", "'Q3 p", "SUM", 0},
		{"='|", "'", "", 0},
		{"=SUM('a(b, c'!A1, |", "", "SUM", 1},
		{"='Q3 plan'!A|", "A", "", 0},
		{"=Summary!B|", "B", "", 0},
	} {
		i := strings.Index(tc.text, "|")
		buf := []rune(strings.Replace(tc.text, "|", "", 1))
		c := ScanCaret(buf, len([]rune(tc.text[:i])))
		if c.Word != tc.word || c.Fn != tc.fn || c.Arg != tc.arg {
			t.Errorf("%s: word %q fn %q arg %d", tc.text, c.Word, c.Fn, c.Arg)
		}
	}
}

func TestArgPart(t *testing.T) {
	for _, tc := range []struct {
		args     string
		arg      int
		variadic bool
		want     string
	}{
		{"value1, [value2, ...]", 0, true, "value1"},
		{"value1, [value2, ...]", 4, true, "[value2, ...]"},
		{"condition, value_if_true, [value_if_false]", 2, false, "[value_if_false]"},
		{"condition, value_if_true, [value_if_false]", 3, false, ""},
		{"sum_range, criteria_range1, criterion1, [criteria_range2, criterion2, ...]", 5, true, "[criteria_range2, criterion2, ...]"},
	} {
		parts := SplitArgs(tc.args)
		got := ""
		if i := ArgPart(parts, tc.arg, tc.variadic); i >= 0 {
			got = parts[i]
		}
		if got != tc.want {
			t.Errorf("%s arg %d: %q, want %q", tc.args, tc.arg, got, tc.want)
		}
	}
}

func TestCycleRef(t *testing.T) {
	tests := []struct {
		in    string
		caret int
		want  string
		ok    bool
	}{
		{"=a1", 3, "=$A$1", true},
		{"=A1+1", 1, "=$A$1+1", true},
		{"=SUM(A1:B2)", 10, "=SUM($A$1:$B$2)", true},
		{"=SUM(A1:B2)", 7, "=SUM($A$1:$B$2)", true},
		{"=SUM($A$1..B2)", 12, "=SUM(A$1..B$2)", true},
		{`="A1"`, 3, "", false},
		{"=LOG10(2)", 6, "", false},
		{"=1+2", 2, "", false},
	}
	for _, tt := range tests {
		out, pos, ok := CycleRef([]rune(tt.in), tt.caret)
		if ok != tt.ok || string(out) != tt.want {
			t.Errorf("CycleRef(%q, %d) = %q, %v; want %q", tt.in, tt.caret, string(out), ok, tt.want)
		}
		if ok && pos > len(out) {
			t.Errorf("caret %d past end of %q", pos, string(out))
		}
	}
}
