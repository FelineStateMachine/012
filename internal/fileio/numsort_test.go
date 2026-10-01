package fileio

import (
	"cmp"
	"math/rand/v2"
	"os"
	"slices"
	"testing"
)

// TestNumberSort sorts numbers held and spilled, against sorting them
// in memory, ties in the order they were added, and leaves no files.
func TestNumberSort(t *testing.T) {
	defer func(n int) { sortRunBytes = n }(sortRunBytes)
	for _, run := range []int{1 << 20, 4096} {
		sortRunBytes = run
		dir := t.TempDir()
		r := rand.New(rand.NewPCG(1, uint64(run)))
		s := NewNumberSort(dir)
		var want []numRec
		for i := range 5000 {
			v := float64(r.IntN(700)) - 350.5
			want = append(want, numRec{v, int64(i)})
			if err := s.Add(v, int64(i)); err != nil {
				t.Fatal(err)
			}
		}
		slices.SortStableFunc(want, func(a, b numRec) int { return cmp.Compare(a.v, b.v) })
		got, err := s.Sorted()
		if err != nil {
			t.Fatal(err)
		}
		if got.Len() != len(want) {
			t.Errorf("run %d: %d numbers, want %d", run, got.Len(), len(want))
		}
		for i, w := range want {
			v, pos, ok := got.Next()
			if !ok || v != w.v || pos != w.pos {
				t.Fatalf("run %d: #%d is %v at %d (%v), want %v at %d", run, i, v, pos, ok, w.v, w.pos)
			}
		}
		if _, _, ok := got.Next(); ok || got.Err() != nil {
			t.Errorf("run %d: more past the end, or %v", run, got.Err())
		}
		got.Close()
		if left, _ := os.ReadDir(dir); len(left) != 0 {
			t.Errorf("run %d: %d files left", run, len(left))
		}
	}
}
