package sheet

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// The occupancy index agrees with the map: inRange yields exactly the
// stored cells of a range in row-major order, bounds is their bounding
// box and nextRow finds the neighbours, through random sets and deletes
// that cross block boundaries.
func TestCellStoreIndex(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	st := newCellStore()
	rows := []int{0, 1, 63, 64, 1023, 1024, 1025, 5000, 70000, MaxRows - 1}
	for i := range 3000 {
		a := Addr{Col: rng.IntN(40), Row: rows[rng.IntN(len(rows))] + rng.IntN(3)}
		a.Row = min(a.Row, MaxRows-1)
		if rng.IntN(3) == 0 {
			st.delete(a)
		} else {
			input := "x"
			if rng.IntN(4) == 0 {
				input = "" // formatting only
			}
			st.set(a, &Cell{Input: input})
		}
		if i%100 == 0 {
			checkStore(t, &st, rng)
		}
	}
	for a := range st.all() {
		st.delete(a)
	}
	if slices.ContainsFunc(st.stored.cols, func(c *colIndex) bool { return c != nil }) || len(st.stored.colIDs) != 0 || len(st.filled.colIDs) != 0 {
		t.Errorf("empty store keeps %d columns", len(st.stored.colIDs))
	}
}

func checkStore(t *testing.T, st *cellStore, rng *rand.Rand) {
	t.Helper()
	for range 20 {
		r := NewRect(Addr{Col: rng.IntN(45), Row: rng.IntN(80000)}, Addr{Col: rng.IntN(45), Row: rng.IntN(MaxRows)})
		checkRange(t, st, r)
		checkFilled(t, st, r)
		checkNextRow(t, st, rng.IntN(40), rng.IntN(MaxRows))
	}
}

// checkRange compares inRange and bounds with the map.
func checkRange(t *testing.T, st *cellStore, r Rect) {
	t.Helper()
	var want []Addr
	for a := range st.all() {
		if r.Contains(a) {
			want = append(want, a)
		}
	}
	sortAddrs(want)
	var got []Addr
	for a := range st.inRange(r) {
		got = append(got, a)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("inRange(%v): %d cells, want %d", r, len(got), len(want))
	}
	b, ok := st.bounds(r)
	if ok != (len(want) > 0) {
		t.Fatalf("bounds(%v) found %v with %d cells", r, ok, len(want))
	}
	for _, a := range want {
		if !b.Contains(a) {
			t.Fatalf("bounds(%v) = %v misses %v", r, b, a)
		}
	}
	if ok && (!slices.ContainsFunc(want, func(a Addr) bool { return a.Row == b.From.Row }) ||
		!slices.ContainsFunc(want, func(a Addr) bool { return a.Col == b.To.Col })) {
		t.Fatalf("bounds(%v) = %v isn't tight", r, b)
	}
}

// checkFilled compares the index of cells with contents with the map.
func checkFilled(t *testing.T, st *cellStore, r Rect) {
	t.Helper()
	var filled []Addr
	for a, c := range st.all() {
		if r.Contains(a) && !c.Blank() {
			filled = append(filled, a)
		}
		if st.filled.has(a) == c.Blank() {
			t.Fatalf("filled index says %v for %v", st.filled.has(a), a)
		}
	}
	fb, ok := st.filled.bounds(r)
	if ok != (len(filled) > 0) || ok && !slices.ContainsFunc(filled, func(a Addr) bool { return a.Row == fb.To.Row }) {
		t.Fatalf("filled bounds(%v) = %v %v", r, fb, ok)
	}
}

// checkNextRow compares nextRow both ways with the map.
func checkNextRow(t *testing.T, st *cellStore, c, row int) {
	t.Helper()
	next, ok := st.stored.nextRow(c, row, 1)
	prev, okp := st.stored.nextRow(c, row, -1)
	want1, ok1 := MaxRows, false
	want2, ok2 := -1, false
	for a := range st.all() {
		if a.Col != c {
			continue
		}
		if a.Row >= row && a.Row < want1 {
			want1, ok1 = a.Row, true
		}
		if a.Row <= row && a.Row > want2 {
			want2, ok2 = a.Row, true
		}
	}
	if ok != ok1 || ok && next != want1 || okp != ok2 || okp && prev != want2 {
		t.Fatalf("nextRow(%d, %d) = %d %v / %d %v, want %d %v / %d %v", c, row, next, ok, prev, okp, want1, ok1, want2, ok2)
	}
}
