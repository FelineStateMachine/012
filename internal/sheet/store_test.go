package sheet

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// The store agrees with a map of what was stored: get returns each
// cell's entry, inRange yields exactly the stored cells of a range in
// row-major order, bounds is their bounding box and nextRow finds the
// neighbours, through random sets and deletes of plain and rich cells
// that cross block boundaries.
func TestCellStoreIndex(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	st := newCellStore()
	model := map[Addr]string{}
	rows := []int{0, 1, 63, 64, 1023, 1024, 1025, 5000, 70000, MaxRows - 1}
	inputs := []string{"x", "", "5", "2.50", "=A1", "TRUE"} // "" is formatting only
	for i := range 3000 {
		a := Addr{Col: rng.IntN(40), Row: rows[rng.IntN(len(rows))] + rng.IntN(3)}
		a.Row = min(a.Row, MaxRows-1)
		if rng.IntN(3) == 0 {
			st.delete(a)
			delete(model, a)
		} else {
			input := inputs[rng.IntN(len(inputs))]
			c, err := newCell(input, Format{}, Style{Bold: input == ""}, true)
			if err != nil {
				t.Fatal(err)
			}
			st.set(a, c)
			model[a] = input
		}
		if i%100 == 0 {
			checkStore(t, &st, model, rng)
		}
	}
	for a := range st.all() {
		st.delete(a)
	}
	if st.len() != 0 || slices.ContainsFunc(st.stored.cols, func(c *colIndex) bool { return c != nil }) || len(st.stored.colIDs) != 0 || len(st.filled.colIDs) != 0 {
		t.Errorf("empty store keeps %d columns", len(st.stored.colIDs))
	}
}

func checkStore(t *testing.T, st *cellStore, model map[Addr]string, rng *rand.Rand) {
	t.Helper()
	if st.len() != len(model) {
		t.Fatalf("%d cells stored, want %d", st.len(), len(model))
	}
	for a, input := range model {
		if c := st.get(a); c == nil || c.Input != input {
			t.Fatalf("get(%v) = %+v, want %q", a, c, input)
		}
	}
	for range 20 {
		r := NewRect(Addr{Col: rng.IntN(45), Row: rng.IntN(80000)}, Addr{Col: rng.IntN(45), Row: rng.IntN(MaxRows)})
		checkRange(t, st, model, r)
		checkFilled(t, st, model, r)
		checkNextRow(t, st, model, rng.IntN(40), rng.IntN(MaxRows))
	}
}

// checkRange compares inRange and bounds with the model.
func checkRange(t *testing.T, st *cellStore, model map[Addr]string, r Rect) {
	t.Helper()
	var want []Addr
	for a := range model {
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
	got = got[:0]
	for a := range st.anyKeysIn(r) {
		got = append(got, a)
	}
	sortAddrs(got)
	if !slices.Equal(got, want) {
		t.Fatalf("anyKeysIn(%v): %d cells, want %d", r, len(got), len(want))
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

// checkFilled compares the index of cells with contents with the model.
func checkFilled(t *testing.T, st *cellStore, model map[Addr]string, r Rect) {
	t.Helper()
	var filled []Addr
	for a, input := range model {
		if r.Contains(a) && input != "" {
			filled = append(filled, a)
		}
		if st.filled.has(a) != (input != "") {
			t.Fatalf("filled index says %v for %v", st.filled.has(a), a)
		}
	}
	fb, ok := st.filled.bounds(r)
	if ok != (len(filled) > 0) || ok && !slices.ContainsFunc(filled, func(a Addr) bool { return a.Row == fb.To.Row }) {
		t.Fatalf("filled bounds(%v) = %v %v", r, fb, ok)
	}
}

// checkNextRow compares nextRow both ways with the model.
func checkNextRow(t *testing.T, st *cellStore, model map[Addr]string, c, row int) {
	t.Helper()
	next, ok := st.stored.nextRow(c, row, 1)
	prev, okp := st.stored.nextRow(c, row, -1)
	want1, ok1 := MaxRows, false
	want2, ok2 := -1, false
	for a := range model {
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

// Cells set and deleted while iterating follow a map's rules: a cell
// deleted before it is reached isn't yielded, and every cell stored
// throughout is yielded once.
func TestCellStoreChangeWhileIterating(t *testing.T) {
	st := newCellStore()
	for row := range 3000 {
		for col := range 3 {
			st.set(Addr{Col: col, Row: row}, &Cell{Input: "x"})
		}
	}
	seen := map[Addr]bool{}
	for a := range st.keysIn(NewRect(Addr{}, Addr{Col: 2, Row: 2999})) {
		if seen[a] {
			t.Fatalf("%v yielded twice", a)
		}
		seen[a] = true
		if a.Col == 0 {
			st.delete(Addr{Col: 2, Row: a.Row})     // not yet reached
			st.delete(Addr{Col: 1, Row: a.Row + 1}) // nor this
		}
	}
	for a := range seen {
		if !st.has(a) {
			t.Fatalf("deleted %v was yielded", a)
		}
	}
	if len(seen) != st.len() {
		t.Errorf("%d yielded, %d stored", len(seen), st.len())
	}
}
