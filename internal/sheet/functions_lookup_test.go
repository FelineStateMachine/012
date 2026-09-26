package sheet

import "testing"

func TestLookupFunctions(t *testing.T) {
	checkFormulas(t, fixture(t), []fnCase{
		{`=VLOOKUP("banana", D1:E3, 2, FALSE)`, num(2.5)},
		{`=VLOOKUP("B*", D1:E3, 2, FALSE)`, num(2.5)},
		{`=VLOOKUP("kiwi", D1:E3, 2, FALSE)`, ErrNA},
		{`=VLOOKUP("Apple", D1:E5, 2, FALSE)`, num(1.5)}, // first match
		{`=VLOOKUP("Blueberry", D1:E3, 2)`, num(2.5)},    // sorted: largest <= key
		{`=VLOOKUP("Aardvark", D1:E3, 2, TRUE)`, ErrNA},
		{`=VLOOKUP(2.5, C1:D5, 2)`, txt("Banana")},
		{`=VLOOKUP("Apple", D1:E3, 3, FALSE)`, ErrRef},
		{`=VLOOKUP("Apple", D1:E3, 0, FALSE)`, ErrValue},
		{`=HLOOKUP(1.5, C1:E2, 2, FALSE)`, num(2.5)},
		{`=HLOOKUP("Apple", D1:E2, 2, FALSE)`, txt("Banana")},
		{`=MATCH("Cherry", D1:D3, 0)`, num(3)},
		{`=MATCH(3.2, C1:C7)`, num(3)},
		{`=MATCH(3.2, C1:C7, 1)`, num(3)},
		{`=MATCH(0, C1:C7)`, ErrNA},
		{`=MATCH(25, I1:I3, -1)`, num(1)}, // descending: smallest >= key
		{`=MATCH(10, I1:I3, -1)`, num(3)},
		{`=MATCH(35, I1:I3, -1)`, ErrNA},
		{`=MATCH(2, C1:D2, 0)`, ErrNA}, // not a single row or column
		{"=INDEX(D1:E3, 2, 2)", num(2.5)},
		{"=INDEX(D1:D3, 3)", txt("Cherry")},
		{"=INDEX(C1:E1, 2)", txt("Apple")}, // a single row counts across
		{"=INDEX(D1:E3, 4, 1)", ErrRef},
		{"=INDEX(D1:E3, 2)", ErrValue}, // a whole row
		{`=XLOOKUP("Cherry", D1:D3, E1:E3)`, num(3.5)},
		{`=XLOOKUP("kiwi", D1:D3, E1:E3)`, ErrNA},
		{`=XLOOKUP("kiwi", D1:D3, E1:E3, "none")`, txt("none")},
		{`=XLOOKUP("Apple", D1:D5, E1:E5, , 0, -1)`, num(9)}, // last to first
		{`=XLOOKUP(3.2, C1:C7, D1:D7, , -1)`, txt("Cherry")}, // next smaller
		{`=XLOOKUP(3.2, C1:C7, D1:D7, , 1)`, txt("date")},    // next larger
		{`=XLOOKUP("Ch*", D1:D3, E1:E3, , 2)`, num(3.5)},
		{`=XLOOKUP("Apple", C1:E1, C2:E2)`, txt("Banana")}, // across a row
		{`=XLOOKUP(1, C1:C3, E1:E2)`, ErrValue},
		{`=CHOOSE(2, "a", "b", "c")`, txt("b")},
		{`=CHOOSE(2.9, "a", "b", "c")`, txt("b")},
		{`=CHOOSE(4, "a", "b", "c")`, ErrValue},
		{`=CHOOSE(1, "a", 1/0)`, txt("a")}, // only the chosen value is evaluated
		{"=ROWS(A1:C7)", num(7)},
		{"=COLUMNS(A1:C7)", num(3)},
	})
}
