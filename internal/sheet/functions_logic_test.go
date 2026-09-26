package sheet

import "testing"

func TestLogicFunctions(t *testing.T) {
	checkFormulas(t, fixture(t), []fnCase{
		{`=IFS(A1>20, "big", A1>5, "medium")`, txt("medium")},
		{`=IFS(A1>20, "big")`, ErrNA},
		{`=IFS(A4, 1)`, ErrValue},
		{`=SWITCH(B2, "north", 1, "south", 2)`, num(2)},
		{`=SWITCH(B4, "north", 1, "south", 2, 0)`, num(0)},
		{`=SWITCH(B4, "north", 1)`, ErrNA},
		{`=SWITCH(1, "1", "text", 1, "number")`, txt("number")},
		{"=XOR(TRUE, FALSE)", boolean(true)},
		{"=XOR(TRUE, TRUE)", boolean(false)},
		{"=XOR(A1:A3)", boolean(true)},
		{`=IFNA(NA(), "none")`, txt("none")},
		{"=IFNA(1/0, 0)", ErrDiv0},
		{"=ISBLANK(A5)", boolean(true)},
		{"=ISBLANK(B6)", boolean(false)}, // empty text isn't blank
		{"=ISNUMBER(A1)", boolean(true)},
		{"=ISNUMBER(A4)", boolean(false)},
		{"=ISNUMBER(F1)", boolean(true)}, // dates are numbers
		{"=ISTEXT(A4)", boolean(true)},
		{"=ISLOGICAL(A6)", boolean(true)},
		{"=ISERROR(1/0)", boolean(true)},
		{"=ISERROR(NA())", boolean(true)},
		{"=ISERR(NA())", boolean(false)},
		{"=ISNA(NA())", boolean(true)},
		{"=ISNA(1/0)", boolean(false)},
	})
}

func TestStepArity(t *testing.T) {
	for _, in := range []string{`=SUMIFS(C1:C5, B1:B5)`, `=COUNTIFS(B1:B5, "a", C1:C5)`, `=IFS(TRUE, 1, FALSE)`} {
		if err := New().Set(at("A1"), in); err == nil {
			t.Errorf("Set(%q) accepted", in)
		}
	}
}
