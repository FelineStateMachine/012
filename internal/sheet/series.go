package sheet

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/FelineStateMachine/012/internal/numfmt"
)

// Fill series: dragging the fill handle in Sheets continues what the
// selected cells start. 1, 2 goes on 3, 4; 2, 4 goes on 6, 8 (with more
// cells, along their linear trend); dates go on by day, or by month when
// they fall on the same day of the month; Jan, Feb goes on Mar and Mon,
// Tue goes on Wed; "Item 1" goes on "Item 2". A single number, and
// anything else (text, formulas), is copied, formulas adjusting their
// references as when pasted.

// FillSeries extends src over dst, which contains it and reaches past it
// along one axis: down or up, right or left. Each column (or row) of src
// is continued on its own. It returns the range filled, as one undo step.
func (s *Sheet) FillSeries(src, dst Rect) (Rect, error) {
	w, h := dst.To.Col-dst.From.Col+1, dst.To.Row-dst.From.Row+1
	if w*h > maxFill {
		return Rect{}, ErrFillTooBig
	}
	vertical := dst.From.Col == src.From.Col && dst.To.Col == src.To.Col
	if !vertical && (dst.From.Row != src.From.Row || dst.To.Row != src.To.Row) || dst == src {
		return src, nil
	}
	var lanes [][2][]Addr // per lane: the source cells in fill order, then the targets
	lane := func(from, step Addr, n int, to Addr, m int) {
		var seq, targets []Addr
		for i := range n {
			seq = append(seq, Addr{Col: from.Col + step.Col*i, Row: from.Row + step.Row*i})
		}
		for i := range m {
			targets = append(targets, Addr{Col: to.Col + step.Col*i, Row: to.Row + step.Row*i})
		}
		lanes = append(lanes, [2][]Addr{seq, targets})
	}
	n := src.To.Row - src.From.Row + 1
	if !vertical {
		n = src.To.Col - src.From.Col + 1
	}
	switch {
	case vertical && dst.To.Row > src.To.Row:
		for c := src.From.Col; c <= src.To.Col; c++ {
			lane(Addr{Col: c, Row: src.From.Row}, Addr{Row: 1}, n, Addr{Col: c, Row: src.To.Row + 1}, dst.To.Row-src.To.Row)
		}
	case vertical:
		for c := src.From.Col; c <= src.To.Col; c++ {
			lane(Addr{Col: c, Row: src.To.Row}, Addr{Row: -1}, n, Addr{Col: c, Row: src.From.Row - 1}, src.From.Row-dst.From.Row)
		}
	case dst.To.Col > src.To.Col:
		for r := src.From.Row; r <= src.To.Row; r++ {
			lane(Addr{Col: src.From.Col, Row: r}, Addr{Col: 1}, n, Addr{Col: src.To.Col + 1, Row: r}, dst.To.Col-src.To.Col)
		}
	default:
		for r := src.From.Row; r <= src.To.Row; r++ {
			lane(Addr{Col: src.To.Col, Row: r}, Addr{Col: -1}, n, Addr{Col: src.From.Col - 1, Row: r}, src.From.Col-dst.From.Col)
		}
	}
	s.change("fill "+dst.String(), dst, func() {
		for _, l := range lanes {
			s.fillLane(l[0], l[1])
		}
	})
	return dst, nil
}

// fillLane continues the cells at seq into targets.
func (s *Sheet) fillLane(seq, targets []Addr) {
	cells := make([]*Cell, len(seq))
	for i, a := range seq {
		cells[i] = s.cells[a]
	}
	next := detectSeries(cells)
	for i, to := range targets {
		k := i % len(seq)
		c := cells[k]
		switch {
		case next != nil:
			nc, err := newCell(next(len(seq)+i), c.Format, c.Style, false)
			if err == nil {
				s.place(to, nc)
				continue
			}
			fallthrough
		case c != nil:
			s.pasteCell(to, c, to.Col-seq[k].Col, to.Row-seq[k].Row, false)
		case s.cells[to] != nil:
			s.place(to, nil)
		}
	}
}

// detectSeries returns the entry for position i of the series cells
// start, or nil if they should be copied instead.
func detectSeries(cells []*Cell) func(i int) string {
	for _, c := range cells {
		if c.Blank() || c.IsFormula() {
			return nil
		}
	}
	if f := numberSeries(cells); f != nil {
		return f
	}
	if f := nameSeries(cells); f != nil {
		return f
	}
	return textNumberSeries(cells)
}

// numberSeries continues numbers along their linear trend (a least
// squares fit, which for evenly spaced numbers is exact), and dates by
// day or by month. A single number is copied; a single date goes on by
// a day.
func numberSeries(cells []*Cell) func(int) string {
	vals := make([]float64, len(cells))
	for i, c := range cells {
		if _, ok := c.expr.(numLit); !ok || c.Value.Kind != Number {
			return nil
		}
		vals[i] = c.Value.Num
	}
	f := cells[0].Format
	date := f.Kind == FmtDate || f.Kind == FmtDateTime
	k := len(vals)
	input := func(i int, v float64) string { return numberInput(v, cells[i%k].Format) }
	if k == 1 {
		if !date {
			return nil
		}
		return func(i int) string { return input(i, vals[0]+float64(i)) }
	}
	if date {
		if step, ok := monthStep(vals); ok {
			y, m, d := numfmt.Civil(int64(vals[0]))
			return func(i int) string {
				y2, m2 := numfmt.AddMonths(y, m, step*i)
				day := min(d, numfmt.DaysIn(y2, m2))
				return input(i, numfmt.DateSerial(y2, m2, day)+vals[0]-math.Floor(vals[0]))
			}
		}
	}
	var mi, mv float64
	for i, v := range vals {
		mi += float64(i)
		mv += v
	}
	mi, mv = mi/float64(k), mv/float64(k)
	var num, den float64
	for i, v := range vals {
		num += (float64(i) - mi) * (v - mv)
		den += (float64(i) - mi) * (float64(i) - mi)
	}
	b := num / den
	a := mv - b*mi
	return func(i int) string { return input(i, a+b*float64(i)) }
}

// monthStep reports whether dates fall on the same day of evenly spaced
// months, and the spacing.
func monthStep(vals []float64) (int, bool) {
	months := func(v float64) (int, int) {
		y, m, d := numfmt.Civil(int64(math.Floor(v)))
		return y*12 + m - 1, d
	}
	m0, d0 := months(vals[0])
	m1, _ := months(vals[1])
	step := m1 - m0
	if step == 0 {
		return 0, false
	}
	for i, v := range vals {
		m, d := months(v)
		if d != d0 || m != m0+step*i {
			return 0, false
		}
	}
	return step, true
}

// numberInput is the entry for v in format f: as the format shows it
// when that reads back exactly ($1,300.00, 10/2/2026), else plain.
func numberInput(v float64, f Format) string {
	// Fifteen significant digits drop float noise (0.1+0.2).
	v, _ = strconv.ParseFloat(strconv.FormatFloat(v, 'g', 15, 64), 64)
	if f.Kind != FmtAuto && f.Kind != FmtText {
		t := FormatText(Value{Kind: Number, Num: v}, f)
		if back, _, ok := ParseValue(t); ok && back == v {
			return t
		}
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}

// A list of names that fills in a cycle: month and day names, long or
// short.
type nameList struct {
	long  []string
	short int // letters in the short form
}

var nameLists = []nameList{{numfmt.MonthNames[:], 3}, {numfmt.DayNames[:], 3}}

// nameAt finds a month or day name: which list, the index, whether it is
// long, and whether it could be either (May).
func nameAt(w string) (list, idx int, long, either, ok bool) {
	lw := strings.ToLower(w)
	for li, l := range nameLists {
		for i, n := range l.long {
			n = strings.ToLower(n)
			full, short := lw == n, lw == n[:l.short]
			if full || short {
				return li, i, full, full && short, true
			}
		}
	}
	return 0, 0, false, false, false
}

// nameSeries continues month or day names in steps, keeping the case and
// length of the first: Jan, Feb goes on Mar; MONDAY goes on TUESDAY.
func nameSeries(cells []*Cell) func(int) string {
	var list, step, first int
	long, known := true, false
	idx := make([]int, len(cells))
	for i, c := range cells {
		if c.Value.Kind != Text {
			return nil
		}
		l, n, lg, either, ok := nameAt(c.Value.Str)
		if !ok || i > 0 && l != list {
			return nil
		}
		if !either {
			if known && lg != long {
				return nil
			}
			long, known = lg, true
		}
		list, idx[i] = l, n
	}
	size := len(nameLists[list].long)
	first = idx[0]
	step = 1
	if len(idx) > 1 {
		step = ((idx[1]-idx[0])%size + size) % size
		for i := 1; i < len(idx); i++ {
			if ((idx[i]-idx[i-1])%size+size)%size != step {
				return nil
			}
		}
	}
	sample := cells[0].Value.Str
	return func(i int) string {
		name := nameLists[list].long[((first+step*i)%size+size)%size]
		if !long {
			name = name[:nameLists[list].short]
		}
		switch {
		case sample == strings.ToUpper(sample):
			name = strings.ToUpper(name)
		case sample == strings.ToLower(sample):
			name = strings.ToLower(name)
		}
		return name
	}
}

var trailingNumber = regexp.MustCompile(`^(.*?)(\d+)$`)

// textNumberSeries continues text ending in a number: Item 1, Item 2 goes
// on Item 3, keeping zero padding (Q01, Q02, Q03). A single one counts up
// by one.
func textNumberSeries(cells []*Cell) func(int) string {
	var prefix, quote string
	width := 0
	nums := make([]int, len(cells))
	for i, c := range cells {
		if c.Value.Kind != Text || c.expr != nil {
			return nil
		}
		m := trailingNumber.FindStringSubmatch(c.Value.Str)
		if m == nil || i > 0 && m[1] != prefix {
			return nil
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			return nil
		}
		prefix, nums[i] = m[1], n
		if strings.HasPrefix(m[2], "0") {
			width = len(m[2])
		}
		if strings.HasPrefix(c.Input, "'") {
			quote = "'"
		}
	}
	step := 1
	if len(nums) > 1 {
		step = nums[1] - nums[0]
		for i := 2; i < len(nums); i++ {
			if nums[i]-nums[i-1] != step {
				return nil
			}
		}
	}
	return func(i int) string {
		n := nums[0] + step*i
		digits := strconv.Itoa(max(n, -n))
		if len(digits) < width {
			digits = strings.Repeat("0", width-len(digits)) + digits
		}
		return quote + prefix + digits
	}
}
