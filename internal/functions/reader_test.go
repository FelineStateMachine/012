package functions

import (
	"slices"
	"testing"

	"github.com/FelineStateMachine/012/internal/value"
)

// fakeBook is one sheet of values, "", for testing the Reader alone. It
// counts the values it reads and the cells it scans.
type fakeBook struct {
	cells   map[Addr]Value
	read    int // values read, as evaluating them would
	scanned int // cells found by Scan
}

func (b *fakeBook) Cell(sheet string, a Addr) Value {
	if sheet != "" {
		return value.ErrRef
	}
	b.read++
	return b.cells[a]
}

func (b *fakeBook) Scan(sheet string, r Rect, from Addr, addrs []Addr, vals []Value) int {
	if sheet != "" {
		return -1
	}
	var in []Addr
	for a := range b.cells {
		if r.Contains(a) && (a.Row > from.Row || a.Row == from.Row && a.Col >= from.Col) {
			in = append(in, a)
		}
	}
	slices.SortFunc(in, func(x, y Addr) int {
		if x.Row != y.Row {
			return x.Row - y.Row
		}
		return x.Col - y.Col
	})
	n := 0
	for _, a := range in {
		if n == len(addrs) {
			break
		}
		addrs[n] = a
		n++
		b.scanned++
		if vals != nil {
			vals[n-1] = b.Cell(sheet, a)
			if vals[n-1].Kind == value.Error {
				break
			}
		}
	}
	return n
}

func (b *fakeBook) Bounds(string, Rect) (Rect, bool, bool)     { return Rect{}, false, true }
func (b *fakeBook) RangeAgg(string, Rect) (Agg, *Value, bool)  { return Agg{}, nil, false }
func (b *fakeBook) Fold(_ string, _ Rect, s Agg) (Agg, *Value) { return s, nil }
func (b *fakeBook) Ask(RemoteCall) (RemoteAnswer, Value)       { return RemoteAnswer{}, ErrNoRemote }
func newFakeReader(cells map[Addr]Value) (*Reader, *fakeBook) {
	b := &fakeBook{cells: cells}
	depth := 0
	return NewReader(b, &depth, false), b
}

// column is n numbers 1..n down column A, every other row, and a 3-wide
// block of the same beside them in C:E.
func column(n int) map[Addr]Value {
	cells := map[Addr]Value{}
	for i := range n {
		cells[Addr{Row: 2 * i}] = num(float64(i + 1))
		for c := 2; c <= 4; c++ {
			cells[Addr{Col: c, Row: 2 * i}] = num(float64(i + 1))
		}
	}
	return cells
}

func TestReaderCells(t *testing.T) {
	const n = 3000
	rd, _ := newFakeReader(column(n))
	for _, r := range []Rect{
		{To: Addr{Row: 2*n - 1}},                             // one column, in chunks
		{From: Addr{Col: 2}, To: Addr{Col: 4, Row: 2*n - 1}}, // three, resuming mid-row
	} {
		width := r.To.Col - r.From.Col + 1
		var got []float64
		rd.cells("", r, func(a Addr, v Value) bool {
			got = append(got, v.Num)
			return true
		})
		if len(got) != n*width {
			t.Fatalf("%v: read %d cells, want %d", r, len(got), n*width)
		}
		for i, f := range got {
			if f != float64(i/width+1) {
				t.Fatalf("%v: cell %d is %v, want %v", r, i, f, i/width+1)
			}
		}
	}
	if rd.level != 0 {
		t.Errorf("level %d after reads, want 0", rd.level)
	}
}

// Ranges are read ahead in chunks, but no cell past the first error is
// evaluated, as when cells were read one at a time.
func TestReaderStopsAtError(t *testing.T) {
	cells := column(100)
	cells[Addr{Row: 2 * 40}] = value.ErrDiv0
	rd, b := newFakeReader(cells)
	seen := 0
	var e Value
	rd.cells("", Rect{To: Addr{Row: 199}}, func(_ Addr, v Value) bool {
		seen++
		e = v
		return v.Kind != value.Error
	})
	if seen != 41 || e != value.ErrDiv0 || b.read != 41 {
		t.Errorf("saw %d cells, evaluated %d, last %v; want 41, 41 and #DIV/0!", seen, b.read, e)
	}
}

// A lookup that stops early scans about as many cells as it looked at,
// however long the range and however long the last one read was.
func TestReaderStoredStopsEarly(t *testing.T) {
	rd, b := newFakeReader(column(5000))
	whole := Rect{To: Addr{Row: 9999}}
	rd.stored("", whole, func(Addr) bool { return true }) // grows the buffer
	b.scanned = 0
	seen := 0
	rd.stored("", whole, func(Addr) bool { seen++; return seen < 20 })
	if seen != 20 || b.scanned > 2*seen+firstChunk {
		t.Errorf("looked at %d cells and scanned %d; want 20 and at most %d", seen, b.scanned, 2*seen+firstChunk)
	}
	if b.read != 0 {
		t.Errorf("evaluated %d cells, want none", b.read)
	}
}

// A read nested in another (a cell of the range evaluating a range of its
// own) uses a buffer of its own.
func TestReaderNested(t *testing.T) {
	rd, _ := newFakeReader(column(50))
	outer := 0.0
	rd.cells("", Rect{To: Addr{Row: 99}}, func(_ Addr, v Value) bool {
		inner := 0.0
		rd.cells("", Rect{From: Addr{Col: 2}, To: Addr{Col: 4, Row: 99}}, func(_ Addr, w Value) bool {
			inner += w.Num
			return true
		})
		if inner != 3*50*51/2 {
			t.Fatalf("inner sum %v", inner)
		}
		outer += v.Num
		return true
	})
	if outer != 50*51/2 || rd.level != 0 {
		t.Errorf("outer sum %v, level %d", outer, rd.level)
	}
}

func TestReaderMissingSheet(t *testing.T) {
	rd, _ := newFakeReader(column(3))
	var got []Value
	rd.cells("Nowhere", Rect{To: Addr{Row: 9}}, func(_ Addr, v Value) bool {
		got = append(got, v)
		return true
	})
	if len(got) != 1 || got[0] != value.ErrRef {
		t.Errorf("got %v, want one #REF!", got)
	}
	rd.stored("Nowhere", Rect{To: Addr{Row: 9}}, func(Addr) bool {
		t.Error("a missing sheet holds a cell")
		return true
	})
}

func TestFunctionTable(t *testing.T) {
	for _, f := range Funcs() {
		if f.Desc == "" || (f.Max != 0 && f.Args == "") || f.eval == nil {
			t.Errorf("%s is missing its description, signature or implementation", f.Name)
		}
	}
}
