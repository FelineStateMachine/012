// Package oracle checks 012's formula engine against excelize's
// calculation engine: the same data and formulas go into both, and the
// results must agree. It is a separate module so the main binary never
// depends on excelize. Run it with make oracle.
package oracle

import (
	"math"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// fixture is the data both engines read, typed as a user would.
//
//	   A       B       C   D       E    F            G   H      I
//	1  10      north   1   Apple   1.5  9/26/2026    1   -1000  30
//	2  20      south   2   Banana  2.5  2026-01-31   2   300    20
//	3  30      north   3   Cherry  3.5  14:30        2   400    10
//	4  apple   east    4   date    x    2/29/2024    3   500
//	5          north   5   Apple   9    12/31/2025       5
//	6          ""      6           TRUE
//	7  -5      Ab*d    7
var fixture = map[string]string{
	"A1": "10", "A2": "20", "A3": "30", "A4": "apple", "A7": "-5",
	"B1": "north", "B2": "south", "B3": "north", "B4": "east", "B5": "north", "B6": `=""`, "B7": "Ab*d",
	"C1": "1", "C2": "2", "C3": "3", "C4": "4", "C5": "5", "C6": "6", "C7": "7",
	"D1": "Apple", "D2": "Banana", "D3": "Cherry", "D4": "date", "D5": "Apple",
	"E1": "1.5", "E2": "2.5", "E3": "3.5", "E4": "x", "E5": "9", "E6": "TRUE",
	"F1": "9/26/2026", "F2": "2026-01-31", "F3": "14:30", "F4": "2/29/2024", "F5": "12/31/2025",
	"G1": "1", "G2": "2", "G3": "2", "G4": "3",
	"H1": "-1000", "H2": "300", "H3": "400", "H4": "500",
	"I1": "30", "I2": "20", "I3": "10",
}

// formulas are compared between the engines. They use syntax both
// accept; toExcel covers the differences.
var formulas = []string{
	// Math
	`=SUM(A1:A7)`, `=SUM(A4)`, `=SUMIF(B1:B5, "north", C1:C5)`, `=SUMIF(A1:A7, ">15")`,
	`=SUMIF(A1:A7, "<>10")`, `=SUMIF(B1:B5, "n*", C1)`, `=SUMIF(C1:C7, 3)`, `=SUMIF(C1:C7, ">=" & C5)`,
	`=SUMIFS(C1:C5, B1:B5, "north", A1:A5, ">10")`, `=SUMPRODUCT(A1:A3, C1:C3)`, `=SUMPRODUCT(A1:A4, C1:C4)`,
	`=PRODUCT(C1:C5)`, `=PRODUCT(2, "3")`, `=POWER(2, 10)`, `=POWER(0, -1)`,
	`=ROUND(1.005, 2)`, `=ROUND(-2.5)`, `=ROUND(1234.5, -2)`, `=ROUND(2.675, 2)`,
	`=ROUNDUP(2.3, 1)`, `=ROUNDUP(-2.31, 1)`, `=ROUNDUP(1234, -2)`, `=ROUNDDOWN(2.39, 1)`, `=ROUNDDOWN(-2.39, 0)`,
	`=TRUNC(-7.9)`, `=CEILING(2.1, 1)`, `=CEILING(7, 5)`, `=CEILING(0.3, 0.1)`, `=CEILING(-2.5, 1)`,
	`=CEILING(-2.5, -1)`, `=CEILING(2.5, -1)`, `=FLOOR(2.9, 1)`, `=FLOOR(7, 5)`, `=FLOOR(-2.5, 1)`, `=FLOOR(-2.5, -1)`,
	`=SIGN(-3)`, `=SIGN(0)`, `=EXP(1)`, `=LN(EXP(2))`, `=LN(0)`, `=LOG(8, 2)`, `=LOG(1000)`, `=LOG(-1)`,
	`=LOG10(0.01)`, `=QUOTIENT(-7, 2)`, `=QUOTIENT(1, 0)`, `=RANDBETWEEN(3, 3)`, `=MOD(-7, 3)`, `=INT(-1.5)`,
	`=ABS(-2)`, `=SQRT(16)`, `=SQRT(-1)`, `=10%`, `=A1*50%`, `=-2^2`, `=2^-1`, `=SUM(C1:C2, E6)`,
	// Statistics
	`=AVERAGE(A1:A7)`, `=COUNT(A1:A7)`, `=COUNTA(A1:A7)`, `=MIN(A1:A7)`, `=MAX(A1:A7)`,
	`=AVERAGEIF(B1:B5, "north", C1:C5)`, `=AVERAGEIF(A1:A7, ">0")`, `=AVERAGEIF(A1:A7, ">100")`,
	`=AVERAGEIFS(C1:C5, B1:B5, "north", C1:C5, ">1")`, `=COUNTIF(B1:B7, "north")`, `=COUNTIF(B1:B7, "NORTH")`,
	`=COUNTIF(B1:B7, "?o*")`, `=COUNTIF(B1:B7, "Ab~*d")`, `=COUNTIF(A1:A7, "<>10")`, `=COUNTIF(A1:A7, "")`,
	`=COUNTIF(A1:A7, "<>")`, `=COUNTIF(E1:E7, TRUE)`, `=COUNTIF(A1:A7, ">=10")`, `=COUNTIF(B1:B7, ">m")`,
	`=COUNTIFS(B1:B5, "north", C1:C5, "<5")`, `=COUNTBLANK(A1:A7)`, `=COUNTBLANK(B1:B7)`,
	`=MEDIAN(A1:A7)`, `=MEDIAN(C1:C5)`, `=MODE(G1:G4)`, `=MODE(C1:C5)`, `=STDEV(A1:A7)`, `=STDEVP(A1:A7)`,
	`=VAR(A1:A7)`, `=VARP(A1:A7)`, `=STDEV(5)`, `=LARGE(A1:A7, 1)`, `=LARGE(A1:A7, 4)`, `=LARGE(A1:A7, 5)`,
	`=SMALL(C1:C7, 2)`, `=RANK(20, A1:A7)`, `=RANK(20, A1:A7, 1)`, `=RANK(2, G1:G4)`, `=RANK(99, A1:A7)`,
	// Text
	`=CONCATENATE("a", 1, TRUE)`, `=CONCAT(A1, "x")`, `=TEXTJOIN(", ", TRUE, A3:A5)`, `=TEXTJOIN("-", FALSE, A3:A5)`,
	`=LEFT("hello", 2)`, `=LEFT("hello")`, `=LEFT("hi", 10)`, `=LEFT("hi", -1)`, `=RIGHT("hello", 3)`,
	`=MID("spreadsheet", 7, 5)`, `=MID("abc", 5, 2)`, `=MID("abc", 0, 2)`, `=LEN("hello")`, `=LEN(A1)`,
	`=UPPER("abc")`, `=LOWER("ABC")`, `=PROPER("hello wORLD it's")`, `=TRIM("  a   b  ")`,
	`=SUBSTITUTE("a-b-c", "-", "+")`, `=SUBSTITUTE("a-b-c", "-", "+", 2)`, `=SUBSTITUTE("a-b-c", "-", "+", 5)`,
	`=REPLACE("abcdef", 2, 3, "X")`, `=REPLACE("abc", 10, 1, "X")`, `=FIND("b", "abcb")`, `=FIND("b", "abcb", 3)`,
	`=FIND("B", "abc")`, `=SEARCH("B", "abc")`, `=SEARCH("b?d", "abcbxd")`, `=SEARCH("z", "abc")`,
	`=TEXT(1234.567, "$#,##0.00")`, `=TEXT(0.256, "0.0%")`, `=TEXT(F1, "yyyy-mm-dd")`, `=TEXT(F1, "dddd")`,
	`=TEXT(F3, "h:mm AM/PM")`, `=TEXT(1234.5, "0.00E+00")`, `=TEXT(-1234.5, "#,##0.00;(#,##0.00)")`,
	`=TEXT(F1, "mmm d, yyyy")`, `=TEXT(1.5, "[h]:mm")`, `=TEXT(5, "000")`,
	`=VALUE("12%")`, `=VALUE("abc")`, `=VALUE(A1)`, `=REPT("ab", 3)`, `=REPT("ab", -1)`,
	`=EXACT("a", "A")`, `=EXACT("a", "a")`, `=A1&"x"`, `=1/3&""`,
	// Logic
	`=IF(A1>5, "big", "small")`, `=IF(A1>50, 1)`, `=IFERROR(1/0, 0)`, `=AND(A1>5, A2<10)`, `=OR(E6, FALSE)`, `=NOT(A1=10)`,
	`=IFS(A1>20, "big", A1>5, "medium")`, `=IFS(A1>20, "big")`, `=SWITCH(B2, "north", 1, "south", 2)`,
	`=SWITCH(B4, "north", 1, "south", 2, 0)`, `=SWITCH(B4, "north", 1)`, `=XOR(TRUE, FALSE)`, `=XOR(TRUE, TRUE)`,
	`=XOR(A1:A3)`, `=IFNA(NA(), "none")`, `=IFNA(1/0, 0)`, `=ISBLANK(A5)`, `=ISBLANK(B6)`, `=ISNUMBER(A1)`,
	`=ISNUMBER(A4)`, `=ISTEXT(A4)`, `=ISLOGICAL(E6)`, `=ISERROR(1/0)`, `=ISERR(NA())`, `=ISNA(NA())`, `=ISNA(1/0)`,
	// Lookup
	`=VLOOKUP("banana", D1:E3, 2, FALSE)`, `=VLOOKUP("B*", D1:E3, 2, FALSE)`, `=VLOOKUP("kiwi", D1:E3, 2, FALSE)`,
	`=VLOOKUP("Apple", D1:E5, 2, FALSE)`, `=VLOOKUP("Blueberry", D1:E3, 2)`, `=VLOOKUP("Aardvark", D1:E3, 2, TRUE)`,
	`=VLOOKUP(2.5, C1:D5, 2)`, `=VLOOKUP("Apple", D1:E3, 3, FALSE)`, `=HLOOKUP(1.5, C1:E2, 2, FALSE)`,
	`=HLOOKUP("Apple", D1:E2, 2, FALSE)`, `=MATCH("Cherry", D1:D3, 0)`, `=MATCH(3.2, C1:C7)`, `=MATCH(0, C1:C7)`,
	`=MATCH(25, I1:I3, -1)`, `=MATCH(10, I1:I3, -1)`, `=MATCH(35, I1:I3, -1)`, `=INDEX(D1:E3, 2, 2)`,
	`=INDEX(D1:D3, 3)`, `=INDEX(C1:E1, 2)`, `=INDEX(D1:E3, 4, 1)`, `=XLOOKUP("Cherry", D1:D3, E1:E3)`,
	`=XLOOKUP("kiwi", D1:D3, E1:E3)`, `=XLOOKUP("kiwi", D1:D3, E1:E3, "none")`, `=XLOOKUP("Apple", D1:D5, E1:E5, , 0, -1)`,
	`=XLOOKUP(3.2, C1:C7, D1:D7, , -1)`, `=XLOOKUP(3.2, C1:C7, D1:D7, , 1)`, `=XLOOKUP("Ch*", D1:D3, E1:E3, , 2)`,
	`=CHOOSE(2, "a", "b", "c")`, `=CHOOSE(2.9, "a", "b", "c")`, `=CHOOSE(4, "a", "b", "c")`, `=ROWS(A1:C7)`, `=COLUMNS(A1:C7)`,
	// Dates
	`=DATE(2026, 9, 26)`, `=DATE(2026, 14, 1)`, `=DATE(2026, 1, 0)`, `=DATE(26, 1, 1)`, `=DATE(-1, 1, 1)`,
	`=TIME(14, 30, 0)`, `=TIME(25, 0, 0)`, `=YEAR(F1)`, `=MONTH(F1)`, `=DAY(F1)`, `=YEAR("2026-09-26")`,
	`=WEEKDAY(F1)`, `=WEEKDAY(F1, 2)`, `=WEEKDAY(F1, 3)`, `=HOUR(F3)`, `=MINUTE(F3)`, `=SECOND(TIME(1, 2, 3))`,
	`=EDATE(F2, 1)`, `=EDATE(F1, -12)`, `=EOMONTH(F1, 0)`, `=EOMONTH(F4, 12)`, `=DATEDIF(F4, F1, "Y")`,
	`=DATEDIF(F4, F1, "M")`, `=DATEDIF(F4, F1, "D")`, `=DATEDIF(F4, F1, "YM")`, `=DATEDIF(F5, F1, "MD")`,
	`=DATEDIF(F5, F1, "YD")`, `=DATEDIF(F2, F5, "D")`, `=DAYS(F1, F2)`, `=DAYS("2026-01-02", "2026-01-01")`,
	`=NETWORKDAYS(F5, F2)`, `=NETWORKDAYS(F2, F5)`, `=NETWORKDAYS(F5, F2, F5)`, `=DATEVALUE("9/26/2026")`,
	`=DATEVALUE("hello")`, `=TIMEVALUE("2:30 PM")`, `=F1+1`, `="2026-09-27"-F1`,
	// Finance
	`=PMT(0.05/12, 360, 200000)`, `=PMT(0, 10, 1000)`, `=PV(0.05/12, 360, -1000)`, `=PV(0, 10, -100)`,
	`=FV(0.05, 10, -100)`, `=FV(0.05, 10, -100, 0, 1)`, `=FV(0, 10, -100, -500)`, `=NPER(0.01, -1000, 10000)`,
	`=NPER(0, -100, 1000)`, `=NPER(0.01, 100, 10000)`, `=RATE(360, -1073.6432460242795, 200000)`,
	`=RATE(48, -200, 8000)`, `=NPV(0.1, H1:H4)`, `=IRR(H1:H4)`, `=IRR(C1:C5)`,
}

// skipped are formulas where Google Sheets, which 012 follows, and
// Excel disagree, or where excelize departs from Excel. Each says why.
var skipped = map[string]string{
	// Sheets and Excel disagree; 012 follows Sheets.
	`=ROUND(-2.5)`: "Sheets makes ROUND's places optional; Excel requires them",

	// excelize departs from Excel (and Sheets) here.
	`=SUM(C1:C2, E6)`:                          "excelize counts TRUE in a referenced cell as 1; Excel and Sheets ignore it",
	`=SUMIF(B1:B5, "n*", C1)`:                  "excelize doesn't stretch a smaller sum range from its corner as Excel does",
	`=SUMPRODUCT(A1:A4, C1:C4)`:                "excelize gives #VALUE! for text; Excel and Sheets treat it as 0",
	`=CEILING(2.5, -1)`:                        "excelize gives #VALUE!; Excel and Sheets give #NUM!",
	`=LN(0)`:                                   "excelize gives -INF; Excel and Sheets give #NUM!",
	`=COUNTIF(A1:A7, "")`:                      "excelize doesn't count blank cells for an empty criterion",
	`=COUNTIF(A1:A7, "<>10")`:                  "excelize doesn't count blank cells as not equal",
	`=COUNTBLANK(B1:B7)`:                       "excelize doesn't count empty text from a formula as blank",
	`=TRIM("  a   b  ")`:                       "excelize doesn't collapse inner runs of spaces",
	`=1/3&""`:                                  "excelize joins 16 significant digits; Excel and Sheets show 15",
	`=IF(A1>50, 1)`:                            "excelize gives empty text; Excel and Sheets give FALSE",
	`=IFNA(1/0, 0)`:                            "excelize turns other errors into #VALUE!; Excel and Sheets pass them through",
	`=VLOOKUP("Apple", D1:E3, 3, FALSE)`:       "excelize gives #N/A for an index past the range; Excel and Sheets give #REF!",
	`=INDEX(C1:E1, 2)`:                         "excelize doesn't count along a single row",
	`=XLOOKUP("Apple", D1:D5, E1:E5, , 0, -1)`: "excelize misreads an omitted argument",
	`=XLOOKUP(3.2, C1:C7, D1:D7, , -1)`:        "excelize misreads an omitted argument",
	`=XLOOKUP(3.2, C1:C7, D1:D7, , 1)`:         "excelize misreads an omitted argument",
	`=XLOOKUP("Ch*", D1:D3, E1:E3, , 2)`:       "excelize misreads an omitted argument",
	`=CHOOSE(2.9, "a", "b", "c")`:              "excelize doesn't truncate the index",
	`=DATE(26, 1, 1)`:                          "excelize doesn't add 1900 to years below 1900",
	`=DATE(-1, 1, 1)`:                          "excelize accepts negative years; Excel and Sheets give #NUM!",
	`=TIME(25, 0, 0)`:                          "excelize doesn't wrap past midnight",
	`="2026-09-27"-F1`:                         "excelize doesn't read date text in arithmetic",
	`=NPV(0.1, H1:H4)`:                         "excelize reads only the first cell of a range",
}

// toExcel translates a 012 formula to Excel syntax: 1-2-3 style @SUM
// and A1..B2 ranges become SUM and A1:B2, and the leading = goes.
func toExcel(f string) string {
	f = strings.TrimPrefix(f, "=")
	var b strings.Builder
	inStr := false
	for i := 0; i < len(f); i++ {
		c := f[i]
		switch {
		case c == '"':
			inStr = !inStr
		case inStr:
		case c == '@':
			continue
		case c == '.' && i+1 < len(f) && f[i+1] == '.':
			b.WriteByte(':')
			i++
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func TestToExcel(t *testing.T) {
	for in, want := range map[string]string{
		"=@SUM(A1..B2)":       "SUM(A1:B2)",
		`=IF(A1="@x..y",1)`:   `IF(A1="@x..y",1)`,
		"=A1*2":               "A1*2",
		"=XLOOKUP(1,A:A,B:B)": "XLOOKUP(1,A:A,B:B)",
	} {
		if got := toExcel(in); got != want {
			t.Errorf("toExcel(%q) = %q, want %q", in, got, want)
		}
	}
}

// build loads the fixture into 012, then copies the values 012
// computed into an excelize workbook, so both start from the same data.
func build(t *testing.T) (*sheet.Sheet, *excelize.File) {
	t.Helper()
	s := sheet.New()
	for a, in := range fixture {
		if err := s.Set(addr(a), in); err != nil {
			t.Fatalf("%s: %v", a, err)
		}
	}
	x := excelize.NewFile()
	for a, in := range fixture {
		var err error
		switch v := s.Value(addr(a)); {
		case sheet.IsFormulaEntry(in):
			err = x.SetCellFormula("Sheet1", a, toExcel(in))
		case v.Kind == sheet.Number:
			err = x.SetCellValue("Sheet1", a, v.Num)
		case v.Kind == sheet.Bool:
			err = x.SetCellValue("Sheet1", a, v.Num != 0)
		default:
			err = x.SetCellValue("Sheet1", a, v.Str)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return s, x
}

func addr(s string) sheet.Addr {
	a, ok := sheet.ParseAddr(s)
	if !ok {
		panic(s)
	}
	return a
}

func TestAgainstExcelize(t *testing.T) {
	s, x := build(t)
	const cell = "Z1"
	matched := 0
	for _, f := range formulas {
		if why, ok := skipped[f]; ok {
			t.Logf("skip %s: %s", f, why)
			continue
		}
		if err := s.Set(addr(cell), f); err != nil {
			t.Errorf("012 rejects %s: %v", f, err)
			continue
		}
		ours := s.Value(addr(cell))
		if err := x.SetCellFormula("Sheet1", cell, toExcel(f)); err != nil {
			t.Fatal(err)
		}
		theirs, err := x.CalcCellValue("Sheet1", cell, excelize.Options{RawCellValue: true})
		if err != nil && !strings.HasPrefix(theirs, "#") {
			theirs = err.Error()
		}
		if agree(ours, theirs) {
			matched++
			continue
		}
		t.Errorf("%s: 012 %s, excelize %q", f, show(ours), theirs)
	}
	t.Logf("%d of %d formulas match excelize, %d skipped", matched, len(formulas)-len(skipped), len(skipped))
}

// agree compares a 012 value with excelize's text result: numbers to
// about 12 significant digits, booleans as TRUE/FALSE, errors by code.
func agree(v sheet.Value, x string) bool {
	switch v.Kind {
	case sheet.Number:
		n, err := strconv.ParseFloat(x, 64)
		if err != nil {
			return false
		}
		return math.Abs(n-v.Num) <= 1e-9*math.Max(1, math.Abs(v.Num))
	case sheet.Bool:
		return strings.EqualFold(x, v.String())
	case sheet.Empty:
		return x == "" || x == "0"
	}
	return x == v.Str
}

func show(v sheet.Value) string {
	switch v.Kind {
	case sheet.Text:
		return strconv.Quote(v.Str)
	case sheet.Empty:
		return "blank"
	}
	return v.String()
}
