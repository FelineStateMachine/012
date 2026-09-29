package sheet

import (
	"strings"
	"testing"
)

// arrCase is a formula and the grid it shows from its cell, rows joined
// with | between cells: one row for a single value.
type arrCase struct {
	in   string
	want []string
}

// checkArrays enters each formula in K10, below and right of the
// fixture's data, and compares what it spills.
func checkArrays(t *testing.T, s *Sheet, tests []arrCase) {
	t.Helper()
	anchor := at("K10")
	for _, tt := range tests {
		if err := s.Set(anchor, tt.in); err != nil {
			t.Errorf("Set(%q): %v", tt.in, err)
			continue
		}
		area, ok := s.SpillArea(anchor)
		if !ok {
			area = Rect{From: anchor, To: anchor}
		}
		var got []string
		for r := area.From.Row; r <= area.To.Row; r++ {
			var row []string
			for c := area.From.Col; c <= area.To.Col; c++ {
				row = append(row, s.Value(Addr{Col: c, Row: r}).String())
			}
			got = append(got, strings.Join(row, "|"))
		}
		if strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
			t.Errorf("%s spills\n  %s\nwant\n  %s", tt.in, strings.Join(got, "\n  "), strings.Join(tt.want, "\n  "))
		}
	}
	s.Set(anchor, "")
}

func TestArrayOperators(t *testing.T) {
	checkArrays(t, fixture(t), []arrCase{
		{"={1,2;3,4}", []string{"1|2", "3|4"}},
		{"={1,2}*10", []string{"10|20"}},
		{"={1;2}+{10,20}", []string{"11|21", "12|22"}},
		{"={1,2,3}+{1,2}", []string{"2|4|#N/A"}},
		{`={"a","b"}&"!"`, []string{"a!|b!"}},
		{"=-{1,2}", []string{"-1|-2"}},
		{"={1,2}/0", []string{"#DIV/0!|#DIV/0!"}},
		{"={C1:C2,G1:G2}", []string{"1|1", "2|2"}},
		{"={C1:C2;G1}", []string{"1", "2", "1"}},
		{"={C1:C2,G1}", []string{"#VALUE!"}},
		{"={1,2;3}", []string{"#VALUE!"}},
		{"=SUM(C1:C3*2)", []string{"12"}},
		{"=SUMPRODUCT(C1:C3*G1:G3)", []string{"11"}},
		{"=SUMPRODUCT((B1:B5=\"north\")*C1:C5)", []string{"9"}},
		{"=SUM({1,2,3})", []string{"6"}},
		{"=COUNT(C:C*1)", []string{"1048576"}},
		{"=MAX(LEN(D1:D5))", []string{"6"}},
		{"=C1:C3*2", []string{"#VALUE!"}}, // row 10: no intersection
		{"=ARRAYFORMULA(C1:C3*2)", []string{"2", "4", "6"}},
		{"=ARRAYFORMULA(LEN(D1:D3))", []string{"5", "6", "6"}},
		{`=ARRAYFORMULA(IF(C1:C4>2, "big", "small"))`, []string{"small", "small", "big", "big"}},
		{"=ARRAYFORMULA(VLOOKUP(G1:G3, C1:D5, 2, FALSE))", []string{"Apple", "Banana", "Banana"}},
		{`=ARRAYFORMULA(SUMIF(B1:B5, {"north";"east"}, C1:C5))`, []string{"9", "4"}},
		{"=ARRAYFORMULA(A1:A3)", []string{"10", "20", "30"}},
		{"=ARRAYFORMULA(ROUND(E1:E3))", []string{"2", "3", "4"}},
		{"=LEN(SEQUENCE(3)*100)", []string{"3"}},
		{"=INDEX(C1:D3, 0, 2)", []string{"Apple", "Banana", "Cherry"}},
		{"=INDEX(C1:D3, 2, 0)", []string{"2|Banana"}},
		{"=INDEX({1,2;3,4}, 2, 1)", []string{"3"}},
		{"=ROWS(SEQUENCE(4))", []string{"4"}},
		{`=IFERROR(FILTER(C1:C5, C1:C5>9), "none")`, []string{"none"}},
		{"=IF(TRUE, SEQUENCE(2), 0)", []string{"1", "2"}},
		{"=MATCH(3, SEQUENCE(5), 0)", []string{"3"}},
		{"=VLOOKUP(2, {1,\"a\";2,\"b\"}, 2, FALSE)", []string{"b"}},
	})
}

func TestArrayFunctions(t *testing.T) {
	checkArrays(t, fixture(t), []arrCase{
		{"=SEQUENCE(3)", []string{"1", "2", "3"}},
		{"=SEQUENCE(2, 3)", []string{"1|2|3", "4|5|6"}},
		{"=SEQUENCE(2, 2, 10, -5)", []string{"10|5", "0|-5"}},
		{"=SEQUENCE(0)", []string{"#NUM!"}}, // as Sheets
		{"=FILTER(D1:D5, C1:C5>3)", []string{"date", "Apple"}},
		{`=FILTER(C1:D5, B1:B5="north", C1:C5>1)`, []string{"3|Cherry", "5|Apple"}},
		{"=FILTER(C1:E1, {TRUE,FALSE,TRUE})", []string{"1|1.5"}},
		{"=FILTER(C1:C5, C1:C5>10)", []string{"#N/A"}},
		{"=FILTER(C1:C5, C1:C4>1)", []string{"#VALUE!"}},
		{"=FILTER(C:C, C:C>5)", []string{"6", "7"}},
		{"=SORT(I1:I3)", []string{"10", "20", "30"}},
		{"=SORT(C1:D3, 2, FALSE)", []string{"3|Cherry", "2|Banana", "1|Apple"}},
		{"=SORT(D1:D5, 1, TRUE)", []string{"Apple", "Apple", "Banana", "Cherry", "date"}},
		{"=SORT(C1:C3, G1:G3, FALSE)", []string{"2", "3", "1"}},
		{"=SORT(A1:A7)", []string{"-5", "10", "20", "30", "apple", "TRUE", ""}},
		{"=SORTN(C1:C7, 2, 0, 1, FALSE)", []string{"7", "6"}},
		{"=SORTN(G1:G4, 2, 1, 1, TRUE)", []string{"1", "2", "2"}},
		{"=SORTN(G1:G4, 2, 2)", []string{"1", "2"}},
		{"=SORTN(G1:G4, 2, 3)", []string{"1", "2", "2"}},
		{"=SORTN(C1:C3)", []string{"1"}},
		{"=UNIQUE(D1:D5)", []string{"Apple", "Banana", "Cherry", "date"}},
		{"=UNIQUE(G1:G4)", []string{"1", "2", "3"}},
		{"=UNIQUE(G1:G4, FALSE, TRUE)", []string{"1", "3"}},
		{`=UNIQUE({"a","a","A"}, TRUE)`, []string{"a|A"}},
		{"=UNIQUE(B:B)", []string{"north", "south", "east", "", "Ab*d"}},
		{"=TRANSPOSE(C1:D2)", []string{"1|2", "Apple|Banana"}},
		{"=FLATTEN(C1:D2)", []string{"1", "Apple", "2", "Banana"}},
		{"=FLATTEN(C1:C2, G1:G2)", []string{"1", "2", "1", "2"}},
		{"=CHOOSECOLS(C1:E2, 3, -3)", []string{"1.5|1", "2.5|2"}},
		{"=CHOOSEROWS(C1:D3, -1, 1)", []string{"3|Cherry", "1|Apple"}},
		{"=CHOOSEROWS(C1:D3, 4)", []string{"#VALUE!"}},
		{"=COUNTA(UNIQUE(B1:B5))", []string{"3"}},
		{`=TEXTJOIN(",", TRUE, FILTER(D1:D5, C1:C5<3))`, []string{"Apple,Banana"}},
		{`=TEXTJOIN("-", TRUE, SEQUENCE(3))`, []string{"1-2-3"}},
	})
}
