package sheet

import (
	"math"
	"testing"
)

func TestMathFunctions(t *testing.T) {
	checkFormulas(t, fixture(t), []fnCase{
		{`=SUMIF(B1:B5, "north", C1:C5)`, num(9)},
		{`=SUMIF(A1:A7, ">15")`, num(50)},
		{`=SUMIF(A1:A7, "<>10")`, num(45)},  // text and booleans aren't summed
		{`=SUMIF(B1:B5, "n*", C1)`, num(9)}, // sum range resized from its corner
		{`=SUMIF(C1:C7, 3)`, num(3)},
		{`=SUMIF(C1:C7, ">=" & C5)`, num(18)},
		{`=SUMIF(B1:B7, "~*")`, num(0)},
		{`=SUMIFS(C1:C5, B1:B5, "north", A1:A5, ">10")`, num(3)},
		{`=SUMIFS(C1:C5, B1:B5, "north", A1:A4, ">10")`, ErrValue}, // mismatched sizes
		{"=SUMPRODUCT(A1:A3, C1:C3)", num(140)},
		{"=SUMPRODUCT(A1:A4, C1:C4)", num(140)}, // text counts as 0
		{"=SUMPRODUCT(A1:A3, C1:C4)", ErrValue},
		{"=PRODUCT(C1:C5)", num(120)},
		{"=PRODUCT(A4)", num(0)}, // text in references is ignored
		{"=PRODUCT(2, \"3\")", num(6)},
		{"=POWER(2, 10)", num(1024)},
		{"=POWER(0, -1)", ErrDiv0},
		{"=POWER(-8, 1/3)", num(-2)}, // an odd root, as Sheets
		{"=(-8)^(1/3)", num(-2)},
		{"=POWER(-32, 0.2)", num(-2)},
		{"=POWER(-8, 0.5)", ErrNum},
		{"=POWER(-8, 2/3)", ErrNum},
		{"=POWER(-8, 2)", num(64)},
		{"=ROUND(1.005, 2)", num(1.01)},
		{"=ROUND(-2.5)", num(-3)},
		{"=ROUND(1234.5, -2)", num(1200)},
		{"=ROUNDUP(2.3, 1)", num(2.3)}, // not 2.4 from binary noise
		{"=ROUNDUP(-2.31, 1)", num(-2.4)},
		{"=ROUNDUP(1234, -2)", num(1300)},
		{"=ROUNDDOWN(2.39, 1)", num(2.3)},
		{"=ROUNDDOWN(-2.39)", num(-2)},
		{"=TRUNC(-7.9)", num(-7)},
		{"=CEILING(2.1)", num(3)},
		{"=CEILING(7, 5)", num(10)},
		{"=CEILING(0.3, 0.1)", num(0.3)},
		{"=CEILING(-2.5, 1)", num(-2)},
		{"=CEILING(-2.5, -1)", num(-3)},
		{"=CEILING(2.5, -1)", ErrNum},
		{"=CEILING(5, 0)", num(0)},
		{"=FLOOR(2.9)", num(2)},
		{"=FLOOR(7, 5)", num(5)},
		{"=FLOOR(-2.5, 1)", num(-3)},
		{"=FLOOR(-2.5, -1)", num(-2)},
		{"=SIGN(-3)", num(-1)},
		{"=SIGN(0)", num(0)},
		{"=EXP(1)", num(math.E)},
		{"=LN(EXP(2))", num(2)},
		{"=LN(0)", ErrNum},
		{"=LOG(8, 2)", num(3)},
		{"=LOG(1000)", num(3)},
		{"=LOG(8, 1)", ErrDiv0},
		{"=LOG(-1)", ErrNum},
		{"=LOG10(0.01)", num(-2)},
		{"=QUOTIENT(-7, 2)", num(-3)},
		{"=QUOTIENT(1, 0)", ErrDiv0},
		{"=RANDBETWEEN(3, 3)", num(3)},
		{"=RANDBETWEEN(5, 1)", ErrNum},
		{"=AND(RAND()>=0, RAND()<1)", boolean(true)},
	})
}
