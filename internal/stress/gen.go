// Package stress builds synthetic worst-case sheets for the stress
// benchmarks (see docs/contributing/limits.md and `make stress`). Every generator is
// deterministic, so runs compare, and builds through the public API the
// way importers do: Load cell by cell, then one RecalcAll.
package stress

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// Shape is one synthetic sheet: a name for benchmark output, and a
// builder. Rows and Cols describe the populated area.
type Shape struct {
	Name  string
	Build func() *sheet.Sheet
	// Edit is a cell whose change touches the shape's interesting
	// topology, for single-edit latency, and the entry to type there.
	Edit  sheet.Addr
	Input string
}

// Rows and Cols are the size of the shapes: the grid of Lotus 1-2-3 that
// 012 had before it grew to Excel's, so runs before and after compare.
const (
	Rows = 8192
	Cols = 256
)

// MaxRows x MaxCols is sheet.DefaultMaxCells of numbers, the most an
// import keeps by default: a tall table, as imports make.
const (
	MaxRows = 1_000_000
	MaxCols = 10
)

func at(col, row int) sheet.Addr { return sheet.Addr{Col: col, Row: row} }

func load(s *sheet.Sheet, a sheet.Addr, input string) {
	if err := s.Load(a, input, sheet.Format{}, sheet.Style{}); err != nil {
		panic(fmt.Sprintf("%s %q: %v", a, input, err))
	}
}

// Dense fills rows x cols with numbers: the storage and render bound.
func Dense(rows, cols int) *sheet.Sheet {
	s := sheet.New()
	r := rand.New(rand.NewPCG(1, 2))
	for row := range rows {
		for col := range cols {
			load(s, at(col, row), fmt.Sprintf("%.2f", r.Float64()*1000))
		}
	}
	s.RecalcAll()
	return s
}

// Chain is A1=1 and A(n)=A(n-1)+1 down n rows: the deepest dependency
// path, evaluated recursively.
func Chain(n int) *sheet.Sheet {
	s := sheet.New()
	load(s, at(0, 0), "1")
	for row := 1; row < n; row++ {
		load(s, at(0, row), fmt.Sprintf("=A%d+1", row))
	}
	s.RecalcAll()
	return s
}

// FanIn is a column of n numbers and k formulas that each SUM all of
// it: k range users over the same big range.
func FanIn(n, k int) *sheet.Sheet {
	s := sheet.New()
	for row := range n {
		load(s, at(0, row), fmt.Sprint(row%100))
	}
	for i := range k {
		load(s, at(1+i/n, i%n), fmt.Sprintf("=SUM(A1:A%d)", n))
	}
	s.RecalcAll()
	return s
}

// FanInColumns is FanIn with whole-column ranges, =SUM(A:A): the same
// data, read through a range of a million rows.
func FanInColumns(n, k int) *sheet.Sheet {
	s := sheet.New()
	for row := range n {
		load(s, at(0, row), fmt.Sprint(row%100))
	}
	for i := range k {
		load(s, at(1+i/n, i%n), "=SUM(A:A)")
	}
	s.RecalcAll()
	return s
}

// Sparse is a tall, sparse sheet: n numbers every stride rows down
// column A, a label beside each, and k of each of three formulas over
// whole columns: =SUM(A:A), running totals to every thousandth row, and
// =VLOOKUP over A:B. Its cost should follow its n cells, not the million
// rows they span.
func Sparse(n, stride, k int) *sheet.Sheet {
	s := sheet.New()
	for i := range n {
		load(s, at(0, i*stride), fmt.Sprint(i%100))
		load(s, at(1, i*stride), fmt.Sprintf("item %d", i))
	}
	for i := range k {
		load(s, at(2, i), "=SUM(A:A)")
		load(s, at(3, i), fmt.Sprintf("=SUM($A$1:A%d)", (i+1)*1000))
		load(s, at(4, i), fmt.Sprintf("=VLOOKUP(%d, A:B, 2, FALSE)", i%100))
	}
	s.RecalcAll()
	return s
}

// Criteria is a table of n rows (a number, a category, an amount) and k
// conditional aggregates over whole columns of it: SUMIF, COUNTIFS with
// two conditions and AVERAGEIF in turn, each testing every row.
func Criteria(n, k int) *sheet.Sheet {
	s := sheet.New()
	for row := range n {
		load(s, at(0, row), fmt.Sprint(row%100))
		load(s, at(1, row), fmt.Sprintf("c%d", row%10))
		load(s, at(2, row), fmt.Sprint(row%7))
	}
	for i := range k {
		var f string
		switch i % 3 {
		case 0:
			f = fmt.Sprintf(`=SUMIF(B:B, "c%d", C:C)`, i%10)
		case 1:
			f = fmt.Sprintf(`=COUNTIFS(A:A, ">%d", B:B, "c%d")`, i%100, i%10)
		default:
			f = fmt.Sprintf(`=AVERAGEIF(A:A, "<%d", C:C)`, i%100)
		}
		load(s, at(4, i), f)
	}
	s.RecalcAll()
	return s
}

// Lookup is n keys with a value beside each and k exact lookups into
// them: VLOOKUP, MATCH and XLOOKUP in turn, each key found somewhere
// along the column.
func Lookup(n, k int) *sheet.Sheet {
	s := sheet.New()
	for row := range n {
		load(s, at(0, row), fmt.Sprint(row))
		load(s, at(1, row), fmt.Sprintf("v%d", row))
	}
	for i := range k {
		key := (i * 37) % n
		var f string
		switch i % 3 {
		case 0:
			f = fmt.Sprintf("=VLOOKUP(%d, A:B, 2, FALSE)", key)
		case 1:
			f = fmt.Sprintf("=MATCH(%d, A:A, 0)", key)
		default:
			f = fmt.Sprintf("=XLOOKUP(%d, A:A, B:B)", key)
		}
		load(s, at(3, i), f)
	}
	s.RecalcAll()
	return s
}

// FanOut is one cell, A1, read directly by k formulas.
func FanOut(k int) *sheet.Sheet {
	s := sheet.New()
	load(s, at(0, 0), "1")
	for i := range k {
		load(s, at(1+i/Rows, i%Rows), fmt.Sprintf("=$A$1*%d", i))
	}
	s.RecalcAll()
	return s
}

// RunningTotals is n numbers with a running SUM beside each: n
// overlapping ranges of growing size, O(n^2) cells read in all.
func RunningTotals(n int) *sheet.Sheet {
	s := sheet.New()
	for row := range n {
		load(s, at(0, row), fmt.Sprint(row%10))
		load(s, at(1, row), fmt.Sprintf("=SUM($A$1:A%d)", row+1))
	}
	s.RecalcAll()
	return s
}

// Arrays is a column of n numbers beside a column of eight categories,
// and k formulas in row 1 spilling from them, alternating a FILTER of the
// numbers by one category (n/8 rows each) and the UNIQUE categories: k
// arrays over n rows, every number's change recomputing half of them.
func Arrays(n, k int) *sheet.Sheet {
	s := sheet.New()
	cats := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta"}
	for row := range n {
		load(s, at(0, row), fmt.Sprint(row%100))
		load(s, at(1, row), cats[row%len(cats)])
	}
	for i := range k {
		f := fmt.Sprintf(`=FILTER(A1:A%d, B1:B%d="%s")`, n, n, cats[i/2%len(cats)])
		if i%2 == 1 {
			f = fmt.Sprintf("=UNIQUE(B1:B%d)", n)
		}
		load(s, at(2+i, 0), f)
	}
	s.RecalcAll()
	return s
}

// Volatile is k cells alternating TODAY() and RAND(), recalculated on
// every change anywhere.
func Volatile(k int) *sheet.Sheet {
	s := sheet.New()
	for i := range k {
		f := "=RAND()"
		if i%2 == 1 {
			f = "=TODAY()"
		}
		load(s, at(i/Rows, i%Rows), f)
	}
	s.RecalcAll()
	return s
}

// Nested is a formula nested depth levels deep, e.g. =((((1+1)+1)+1)+1),
// in A1: the parser's and evaluator's recursion.
func Nested(depth int) string {
	return "=" + strings.Repeat("(", depth) + "1" + strings.Repeat("+1)", depth)
}

// NestedIF is =IF(A1>0,IF(A1>1,...,0),0) depth levels deep.
func NestedIF(depth int) string {
	var b strings.Builder
	b.WriteString("=")
	for i := range depth {
		fmt.Fprintf(&b, "IF(A1>%d,", i)
	}
	b.WriteString("1")
	b.WriteString(strings.Repeat(",0)", depth))
	return b.String()
}

// LongText puts a length-n line of text, mixing ASCII, accents, CJK and
// emoji, in column A of rows rows and leaves everything else blank, so
// every row overflows across the whole screen.
func LongText(rows, n int) *sheet.Sheet {
	s := sheet.New()
	const alphabet = "The quick brown fox jumps over the lazy dog. Ça déjà vu, naïve café. 東京都の天気は晴れ。 🙂🚀 "
	src := []rune(alphabet)
	for row := range rows {
		var b strings.Builder
		for i := range n {
			b.WriteRune(src[(i+row)%len(src)])
		}
		load(s, at(0, row), b.String())
	}
	s.RecalcAll()
	return s
}

// Names defines k named ranges over a column of 8192 numbers and a
// formula using each: every changed cell is checked against every name.
func Names(k int) *sheet.Sheet {
	s := sheet.New()
	for row := range Rows {
		load(s, at(0, row), fmt.Sprint(row%100))
	}
	for i := range k {
		from := (i * 7) % (Rows - 100)
		name := fmt.Sprintf("Region_%d", i)
		if err := s.DefineName(name, sheet.NewRect(at(0, from), at(0, from+99))); err != nil {
			panic(err)
		}
		load(s, at(1+i/Rows, i%Rows), "=SUM("+name+")")
	}
	s.RecalcAll()
	return s
}

// Table is a filterable, sortable table: a header row and rows rows of
// an id, a category, a number and a date-like text, in cols columns.
func Table(rows, cols int) *sheet.Sheet {
	s := sheet.New()
	r := rand.New(rand.NewPCG(3, 4))
	cats := []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta"}
	for col := range cols {
		load(s, at(col, 0), fmt.Sprintf("H%d", col))
	}
	for row := 1; row <= rows; row++ {
		for col := range cols {
			var v string
			switch col % 4 {
			case 0:
				v = fmt.Sprint(row)
			case 1:
				v = cats[r.IntN(len(cats))]
			case 2:
				v = fmt.Sprintf("%.2f", r.Float64()*1e4)
			default:
				v = fmt.Sprintf("item %d", r.IntN(1000))
			}
			load(s, at(col, row), v)
		}
	}
	s.RecalcAll()
	return s
}

// Charts adds k small column charts over a data block to s.
func Charts(s *sheet.Sheet, k int) {
	for i := range k {
		s.AddChart(sheet.Chart{
			Type: sheet.ChartColumn, Data: sheet.NewRect(at(0, 0), at(2, 20)), Header: true, Labels: true,
			At: at(4+(i%8)*3, (i/8)*3), W: 24, H: 10,
		})
	}
	s.ClearHistory()
}

// Shapes are the edit-latency topologies at their stress sizes.
func Shapes() []Shape {
	return []Shape{
		{"dense-8192x26", func() *sheet.Sheet { return Dense(Rows, 26) }, at(0, 0), "5"},
		{"chain-8192", func() *sheet.Sheet { return Chain(Rows) }, at(0, 0), "2"},
		{"fanin-1000xSUM8192", func() *sheet.Sheet { return FanIn(Rows, 1000) }, at(0, 4000), "7"},
		{"fanout-8192", func() *sheet.Sheet { return FanOut(Rows) }, at(0, 0), "2"},
		{"running-8192", func() *sheet.Sheet { return RunningTotals(Rows) }, at(0, 0), "3"},
		{"volatile-8192", func() *sheet.Sheet { return Volatile(Rows) }, at(200, 0), "1"},
		{"names-1000", func() *sheet.Sheet { return Names(1000) }, at(0, 50), "9"},
		{"fanin-1000xSUM(A:A)", func() *sheet.Sheet { return FanInColumns(Rows, 1000) }, at(0, 4000), "7"},
		{"sparse-1M", func() *sheet.Sheet { return Sparse(10000, 100, 1000) }, at(0, 500000), "5"},
		{"criteria-60xSUMIF8192", func() *sheet.Sheet { return Criteria(Rows, 60) }, at(0, 4000), "7"},
		{"lookup-300xVLOOKUP8192", func() *sheet.Sheet { return Lookup(Rows, 300) }, at(0, 4000), "4000"},
		{"arrays-1000xFILTER8192", func() *sheet.Sheet { return Arrays(Rows, 1000) }, at(0, 4000), "7"},
		{"dense-1Mx10", func() *sheet.Sheet { return Dense(MaxRows, MaxCols) }, at(0, 500000), "5"},
	}
}
