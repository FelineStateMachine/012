// Package stress builds synthetic worst-case sheets for the stress
// benchmarks (see docs/limits.md and `make stress`). Every generator is
// deterministic, so runs compare, and builds through the public API the
// way importers do: Load cell by cell, then one RecalcAll.
package stress

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"012/internal/sheet"
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

// FanOut is one cell, A1, read directly by k formulas.
func FanOut(k int) *sheet.Sheet {
	s := sheet.New()
	load(s, at(0, 0), "1")
	for i := range k {
		load(s, at(1+i/sheet.MaxRows, i%sheet.MaxRows), fmt.Sprintf("=$A$1*%d", i))
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

// Volatile is k cells alternating TODAY() and RAND(), recalculated on
// every change anywhere.
func Volatile(k int) *sheet.Sheet {
	s := sheet.New()
	for i := range k {
		f := "=RAND()"
		if i%2 == 1 {
			f = "=TODAY()"
		}
		load(s, at(i/sheet.MaxRows, i%sheet.MaxRows), f)
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
	for row := range sheet.MaxRows {
		load(s, at(0, row), fmt.Sprint(row%100))
	}
	for i := range k {
		from := (i * 7) % (sheet.MaxRows - 100)
		name := fmt.Sprintf("Region_%d", i)
		if err := s.DefineName(name, sheet.NewRect(at(0, from), at(0, from+99))); err != nil {
			panic(err)
		}
		load(s, at(1+i/sheet.MaxRows, i%sheet.MaxRows), "=SUM("+name+")")
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
		{"dense-8192x26", func() *sheet.Sheet { return Dense(sheet.MaxRows, 26) }, at(0, 0), "5"},
		{"chain-8192", func() *sheet.Sheet { return Chain(sheet.MaxRows) }, at(0, 0), "2"},
		{"fanin-1000xSUM8192", func() *sheet.Sheet { return FanIn(sheet.MaxRows, 1000) }, at(0, 4000), "7"},
		{"fanout-8192", func() *sheet.Sheet { return FanOut(sheet.MaxRows) }, at(0, 0), "2"},
		{"running-8192", func() *sheet.Sheet { return RunningTotals(sheet.MaxRows) }, at(0, 0), "3"},
		{"volatile-8192", func() *sheet.Sheet { return Volatile(sheet.MaxRows) }, at(200, 0), "1"},
		{"names-1000", func() *sheet.Sheet { return Names(1000) }, at(0, 50), "9"},
	}
}
