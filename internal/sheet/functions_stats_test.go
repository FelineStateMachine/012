package sheet

import "testing"

func TestStatsFunctions(t *testing.T) {
	checkFormulas(t, fixture(t), []fnCase{
		{`=AVERAGEIF(B1:B5, "north", C1:C5)`, num(3)},
		{`=AVERAGEIF(A1:A7, ">0")`, num(20)},
		{`=AVERAGEIF(A1:A7, ">100")`, ErrDiv0},
		{`=AVERAGEIFS(C1:C5, B1:B5, "north", C1:C5, ">1")`, num(4)},
		{`=COUNTIF(B1:B7, "north")`, num(3)},
		{`=COUNTIF(B1:B7, "NORTH")`, num(3)}, // case-insensitive
		{`=COUNTIF(B1:B7, "?o*")`, num(4)},
		{`=COUNTIF(B1:B7, "Ab~*d")`, num(1)},
		{`=COUNTIF(A1:A7, "<>10")`, num(6)}, // blanks and text count as not equal
		{`=COUNTIF(A1:A7, "")`, num(1)},
		{`=COUNTIF(B1:B7, "")`, num(1)}, // "" from a formula counts as blank
		{`=COUNTIF(A1:A7, "<>")`, num(6)},
		{`=COUNTIF(A1:A7, TRUE)`, num(1)},
		{`=COUNTIF(A1:A7, ">=10")`, num(3)},
		{`=COUNTIF(F1:F5, ">1/1/2026")`, num(2)}, // date criteria compare as dates
		{`=COUNTIF(B1:B7, ">m")`, num(4)},
		{`=COUNTIFS(B1:B5, "north", C1:C5, "<5")`, num(2)},
		{"=COUNTBLANK(A1:A7)", num(1)},
		{"=COUNTBLANK(B1:B7)", num(1)},
		{"=MEDIAN(A1:A7)", num(15)},
		{"=MEDIAN(C1:C5)", num(3)},
		{"=MEDIAN(A4)", ErrNum},
		{`=MEDIAN("a")`, ErrValue},
		{"=MODE(G1:G4)", num(2)},
		{"=MODE(C1:C5)", ErrNA},
		{"=STDEV(A1:A7)", num(14.930394055974098)},
		{"=STDEVP(A1:A7)", num(12.93010054098575)},
		{"=VAR(A1:A7)", num(222.91666666666666)},
		{"=VARP(A1:A7)", num(167.1875)},
		{"=STDEV(5)", ErrDiv0},
		{"=LARGE(A1:A7, 1)", num(30)},
		{"=LARGE(A1:A7, 4)", num(-5)},
		{"=LARGE(A1:A7, 5)", ErrNum},
		{"=SMALL(C1:C7, 2)", num(2)},
		{"=RANK(20, A1:A7)", num(2)},
		{"=RANK(20, A1:A7, 1)", num(3)},
		{"=RANK(2, G1:G4)", num(2)},
		{"=RANK(99, A1:A7)", ErrNA},
		{"=RANK.EQ(20, A1:A7)", num(2)},
		{"=RANK(1/0, A1:A7)", ErrDiv0},
		{"=MODE.SNGL(G1:G4)", num(2)},
		{"=MODE(3, 2, 2, 3, 1)", num(3)}, // the first read of the most common
		{"=PERCENTILE(A1:A7, 0.5)", num(15)},
		{"=PERCENTILE(A1:A7, 0.25)", num(6.25)},
		{"=PERCENTILE.INC(A1:A7, 1)", num(30)},
		{"=PERCENTILE(A1:A7, 1.5)", ErrNum},
		{"=PERCENTILE(A4, 0.5)", ErrNum},
		{"=PERCENTILE.EXC(C1:C5, 0.25)", num(1.5)},
		{"=PERCENTILE.EXC(C1:C5, 0.1)", ErrNum},
		{"=PERCENTILE.EXC(C1:C5, 0)", ErrNum},
		{"=QUARTILE(C1:C5, 1)", num(2)},
		{"=QUARTILE(C1:C5, 0)", num(1)},
		{"=QUARTILE(C1:C5, 4.9)", num(5)},
		{"=QUARTILE(C1:C5, 5)", ErrNum},
		{"=QUARTILE.INC(C1:C5, 3)", num(4)},
		{"=QUARTILE.EXC(C1:C5, 1)", num(1.5)},
		{"=QUARTILE.EXC(C1:C5, 0)", ErrNum},
	})
}
