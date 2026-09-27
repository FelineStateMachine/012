package sheet

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// Charts float over the grid like Sheets' charts: each draws a range of
// cells, sits with its top-left corner on a cell, and has a size in
// terminal cells. They belong to the sheet, so they're saved with it,
// follow inserted and deleted rows and columns, and every change to them
// is an undo step.

// ChartType is how a chart draws its series. Adding one takes a constant
// here and its name in chartTypeNames; the chart editor, the file format
// and ChartTypes follow from the table, and internal/chart draws it.
type ChartType int

const (
	ChartColumn  ChartType = iota // vertical bars, one group per category
	ChartBar                      // horizontal bars
	ChartLine                     // one line per series
	ChartPie                      // the first series as slices of a whole
	ChartArea                     // lines filled down to the axis
	ChartScatter                  // points at X, Y: the first series is X
)

// chartTypeNames names every type, as files store it, in the order the
// chart editor offers them.
var chartTypeNames = [...]string{ChartColumn: "column", ChartBar: "bar", ChartLine: "line", ChartPie: "pie",
	ChartArea: "area", ChartScatter: "scatter"}

// ChartTypes lists the types in the order the chart editor offers them.
var ChartTypes = func() []ChartType {
	out := make([]ChartType, len(chartTypeNames))
	for i := range out {
		out[i] = ChartType(i)
	}
	return out
}()

func (t ChartType) String() string {
	if t < 0 || int(t) >= len(chartTypeNames) {
		return chartTypeNames[0]
	}
	return chartTypeNames[t]
}

// Title is the type's name as the chart editor shows it, e.g. "Column".
func (t ChartType) Title() string {
	s := t.String()
	return strings.ToUpper(s[:1]) + s[1:]
}

// ParseChartType parses a type name as String writes it.
func ParseChartType(s string) (ChartType, bool) {
	i := slices.Index(chartTypeNames[:], strings.ToLower(s))
	return ChartType(max(i, 0)), i >= 0
}

// Chart is a chart floating over the grid.
type Chart struct {
	Type ChartType
	Data Rect
	// ByRow puts each series in a row instead of a column, like Sheets'
	// "Switch rows / columns".
	ByRow bool
	// Header takes series names from the first row of the data (the first
	// column when ByRow).
	Header bool
	// Labels takes category labels from the first column of the data (the
	// first row when ByRow).
	Labels bool
	Title  string
	At     Addr // the cell under the top-left corner
	W, H   int  // size in terminal cells
	// ChartOptions are the stacking, axis and legend settings of Sheets'
	// Customize tab; see chartopts.go.
	ChartOptions
}

// Chart size limits, in terminal cells.
const (
	MinChartW, MinChartH = 20, 8
	MaxChartW, MaxChartH = 240, 120
)

// ChartSeries is one named run of values. Blank and non-numeric cells are
// NaN, drawn as gaps.
type ChartSeries struct {
	Name   string
	Values []float64
}

// ChartData is what a chart draws: category labels and series of the same
// length, and the number format of the values, for axis labels.
type ChartData struct {
	Categories []string
	Series     []ChartSeries
	Format     Format
}

// maxChartPoints caps the categories a chart reads, so a chart over whole
// columns stays quick to draw, and maxChartSeries its series (the columns
// 1-2-3's grid had), for one over whole rows.
const (
	maxChartPoints = 500
	maxChartSeries = 256
)

// ChartData reads a chart's cells. Series names and labels are the cells'
// text as displayed.
func (s *Sheet) ChartData(c Chart) ChartData {
	r := s.clipToUsed(c.Data)
	// Lay the range out as rows of cells with series going down columns,
	// transposing when series run along rows.
	at := func(i, j int) Addr { return Addr{Col: r.From.Col + j, Row: r.From.Row + i} }
	n, m := r.To.Row-r.From.Row+1, r.To.Col-r.From.Col+1
	if c.ByRow {
		at = func(i, j int) Addr { return Addr{Col: r.From.Col + i, Row: r.From.Row + j} }
		n, m = m, n
	}
	first, col0 := 0, 0
	if c.Header && n > 1 {
		first = 1
	}
	if c.Labels && m > 1 {
		col0 = 1
	}
	n, m = min(n, first+maxChartPoints), min(m, col0+maxChartSeries)
	var d ChartData
	for i := first; i < n; i++ {
		label := strconv.Itoa(i - first + 1)
		if col0 == 1 {
			label = s.displayText(at(i, 0))
		}
		d.Categories = append(d.Categories, label)
	}
	for j := col0; j < m; j++ {
		sr := ChartSeries{Name: "Series " + strconv.Itoa(j-col0+1)}
		if first == 1 {
			if name := s.displayText(at(0, j)); name != "" {
				sr.Name = name
			}
		}
		for i := first; i < n; i++ {
			a := at(i, j)
			v := s.Value(a)
			switch v.Kind {
			case Number:
				sr.Values = append(sr.Values, v.Num)
				if d.Format.IsZero() {
					d.Format = s.DisplayFormat(a)
				}
			default:
				sr.Values = append(sr.Values, math.NaN())
			}
		}
		d.Series = append(d.Series, sr)
	}
	return d
}

// displayText is a cell's value as its format shows it, without padding.
func (s *Sheet) displayText(a Addr) string {
	v := s.Value(a)
	if v.Kind == Text || v.Kind == Empty {
		return v.Str
	}
	text, _ := Display(v, s.DisplayFormat(a), 64)
	return strings.TrimSpace(text)
}

// clipToUsed trims a range to the used part of the sheet, so a chart over
// whole columns reads only the filled rows.
func (s *Sheet) clipToUsed(r Rect) Rect {
	used, ok := s.UsedRange()
	if !ok {
		return Rect{From: r.From, To: r.From}
	}
	r.To.Col = max(min(r.To.Col, used.To.Col), r.From.Col)
	r.To.Row = max(min(r.To.Row, used.To.Row), r.From.Row)
	return r
}

// Region returns the block of data around a, as Sheets picks the range to
// sort or filter when a single cell is selected: filled cells connected
// to a (diagonals count), and anything touching their bounding box, until
// nothing more touches it. A blank cell with no filled neighbors is a
// region of its own.
func (s *Sheet) Region(a Addr) Rect {
	filled := func(a Addr) bool { return a.Valid() && !s.cells.get(a).Blank() }
	seen := map[Addr]bool{}
	var queue []Addr
	visit := func(p Addr) {
		if !seen[p] && filled(p) {
			seen[p] = true
			queue = append(queue, p)
		}
	}
	around := func(p Addr) {
		for dr := -1; dr <= 1; dr++ {
			for dc := -1; dc <= 1; dc++ {
				visit(Addr{Col: p.Col + dc, Row: p.Row + dr})
			}
		}
	}
	around(a)
	if len(queue) == 0 {
		return Rect{From: a, To: a}
	}
	r := Rect{From: queue[0], To: queue[0]}
	for len(queue) > 0 {
		for len(queue) > 0 {
			p := queue[0]
			queue = queue[1:]
			r = union(r, Rect{From: p, To: p})
			around(p)
		}
		// Cells touching the bounding box join too.
		for row := r.From.Row - 1; row <= r.To.Row+1; row++ {
			visit(Addr{Col: r.From.Col - 1, Row: row})
			visit(Addr{Col: r.To.Col + 1, Row: row})
		}
		for col := r.From.Col; col <= r.To.Col; col++ {
			visit(Addr{Col: col, Row: r.From.Row - 1})
			visit(Addr{Col: col, Row: r.To.Row + 1})
		}
	}
	return r
}

// GuessChart sets up a chart over r the way Sheets does: series down
// columns, a header row when the first row is text over numbers, and
// category labels when the first column is text.
func (s *Sheet) GuessChart(r Rect) Chart {
	r = s.clipToUsed(r)
	c := Chart{Type: ChartColumn, Data: r}
	isText := func(a Addr) bool { return s.Value(a).Kind == Text }
	if r.To.Col > r.From.Col {
		labels := false
		s.cells.filled.colScan(r.From.Col, r.From.Row+1, r.To.Row, func(row int) bool {
			labels = isText(Addr{Col: r.From.Col, Row: row})
			return !labels
		})
		c.Labels = labels || r.From.Row == r.To.Row && isText(r.From)
	}
	if r.To.Row > r.From.Row {
		from := r.From.Col
		if c.Labels {
			from++
		}
		texts, nums := 0, 0
		for _, cell := range s.cells.inRange(Rect{From: Addr{Col: from, Row: r.From.Row}, To: Addr{Col: r.To.Col, Row: r.From.Row}}) {
			switch cell.Value.Kind {
			case Text:
				texts++
			case Number, Bool:
				nums++
			}
		}
		c.Header = texts > 0 && nums == 0
	}
	d := s.ChartData(c)
	if len(d.Series) <= 2 && c.Header {
		names := make([]string, len(d.Series))
		for i, sr := range d.Series {
			names[i] = sr.Name
		}
		c.Title = strings.Join(names, " and ")
	}
	return c
}

// Charts returns the sheet's charts, bottom first.
func (s *Sheet) Charts() []Chart { return slices.Clone(s.charts) }

// AddChart adds a chart on top of the others and returns its index.
func (s *Sheet) AddChart(c Chart) int {
	s.change("insert chart", c.Data, func() {
		s.recordCharts()
		s.charts = append(s.charts, c.clamped())
	})
	return len(s.charts) - 1
}

// SetChart replaces chart i, as one undo step described by label, e.g.
// "move chart".
func (s *Sheet) SetChart(i int, c Chart, label string) {
	if i < 0 || i >= len(s.charts) || s.charts[i] == c.clamped() {
		return
	}
	s.change(label, c.Data, func() {
		s.recordCharts()
		s.charts[i] = c.clamped()
	})
}

// DeleteChart removes chart i.
func (s *Sheet) DeleteChart(i int) {
	if i < 0 || i >= len(s.charts) {
		return
	}
	s.change("delete chart", s.charts[i].Data, func() {
		s.recordCharts()
		s.charts = slices.Delete(slices.Clone(s.charts), i, i+1)
	})
}

// clamped keeps a chart's size within limits and its corner on the sheet.
func (c Chart) clamped() Chart {
	c.W = clampInt(c.W, MinChartW, MaxChartW)
	c.H = clampInt(c.H, MinChartH, MaxChartH)
	c.At.Col = clampInt(c.At.Col, 0, MaxCols-1)
	c.At.Row = clampInt(c.At.Row, 0, MaxRows-1)
	return c
}

// shiftCharts moves charts with inserted or deleted rows or columns: the
// data range grows or shrinks as a formula's range would, and the corner
// moves with its cell. A chart whose data is deleted entirely goes too.
func (s *Sheet) shiftCharts(cell func(Addr) (Addr, bool), rng func(Rect) (Rect, bool)) {
	if len(s.charts) == 0 {
		return
	}
	s.recordCharts()
	var next []Chart
	for _, c := range s.charts {
		data, ok := rng(c.Data)
		if !ok {
			continue
		}
		c.Data = data
		if at, ok := cell(c.At); ok {
			c.At = at
		}
		next = append(next, c.clamped())
	}
	s.charts = next
}
