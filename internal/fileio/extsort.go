package fileio

import (
	"bufio"
	"container/heap"
	"errors"
	"io"
	"os"
	"slices"
)

// An external merge sort of records of any kind in bounded memory: runs
// of at most sortRunBytes sorted in memory and spilled to files, then
// merged, read back one record at a time. A Parquet view's sort orders
// its rows with it (sourcesort.go), and the order statistics of a
// source's column its numbers (numsort.go).

// sortRunBytes bounds the records a sort holds in memory at once.
var sortRunBytes = 64 << 20

// recCodec is how a sort compares, sizes and stores its records.
type recCodec[T any] struct {
	cmp  func(a, b T) int
	size func(*T) int // about what a record holds in memory
	// fixed is every record's size when they're all one size, so the
	// sort can grow its run to what one holds and no further.
	fixed int
	put   func(*bufio.Writer, *T)
	// get reads a record into the one given, io.EOF at a run's end.
	get func(*bufio.Reader, *T) error
}

// extSort sorts records in bounded memory.
type extSort[T any] struct {
	dir   string
	codec recCodec[T]
	recs  []T
	bytes int
	runs  []string
	limit int
	n     int64
}

func newExtSort[T any](dir string, codec recCodec[T]) *extSort[T] {
	return &extSort[T]{dir: dir, codec: codec, limit: sortRunBytes}
}

// add takes a record, spilling a run when memory is full.
func (s *extSort[T]) add(r T) error {
	if f := s.codec.fixed; f > 0 && len(s.recs) == cap(s.recs) {
		// Grow no further than a run holds: doubling past it would
		// hold twice the run while copying.
		most := max(s.limit/f, 1)
		grown := make([]T, len(s.recs), len(s.recs)+max(min(max(cap(s.recs), 1024), most-len(s.recs)), 1))
		copy(grown, s.recs)
		s.recs = grown
	}
	s.recs = append(s.recs, r)
	s.bytes += s.codec.size(&s.recs[len(s.recs)-1])
	s.n++
	if s.bytes >= s.limit {
		return s.spill()
	}
	return nil
}

func (s *extSort[T]) sort() {
	slices.SortFunc(s.recs, s.codec.cmp)
}

// spill sorts the records in memory and writes them as a run.
func (s *extSort[T]) spill() error {
	s.sort()
	f, err := os.CreateTemp(s.dir, "012-sort-*.run")
	if err != nil {
		return err
	}
	s.runs = append(s.runs, f.Name())
	w := bufio.NewWriterSize(f, 1<<20)
	for i := range s.recs {
		s.codec.put(w, &s.recs[i])
	}
	err = errors.Join(w.Flush(), f.Close())
	clear(s.recs)
	s.recs, s.bytes = s.recs[:0], 0
	return err
}

// sorted is the records in order, read once: those held when none were
// spilled, else the runs merged. Closing it removes the runs.
func (s *extSort[T]) sorted() (*sortedRecs[T], error) {
	out := &sortedRecs[T]{s: s}
	if len(s.runs) == 0 {
		s.sort()
		return out, nil
	}
	if len(s.recs) > 0 {
		if err := s.spill(); err != nil {
			s.remove()
			return nil, err
		}
	}
	out.h = &runHeap[T]{cmp: s.codec.cmp}
	for _, name := range s.runs {
		f, err := os.Open(name)
		if err != nil {
			out.close()
			return nil, err
		}
		r := &runReader[T]{f: f, r: bufio.NewReaderSize(f, 256<<10), get: s.codec.get}
		if r.next() {
			out.h.runs = append(out.h.runs, r)
		} else {
			f.Close()
			out.h.done = append(out.h.done, r)
		}
	}
	heap.Init(out.h)
	return out, nil
}

// remove deletes the runs.
func (s *extSort[T]) remove() {
	for _, name := range s.runs {
		os.Remove(name)
	}
	s.runs = nil
}

// sortedRecs reads a sort's records in order.
type sortedRecs[T any] struct {
	s    *extSort[T]
	h    *runHeap[T] // nil when the records are held
	i    int
	last *runReader[T] // the run the record handed out last came from
}

// next is the next record, valid until the next call; nil at the end
// or on an error, which err says.
func (o *sortedRecs[T]) next() *T {
	if o.h == nil {
		if o.i >= len(o.s.recs) {
			return nil
		}
		o.i++
		return &o.s.recs[o.i-1]
	}
	if o.last != nil { // move past the record handed out last
		if o.last.next() {
			heap.Fix(o.h, 0)
		} else {
			o.last.f.Close()
			heap.Pop(o.h)
		}
		o.last = nil
	}
	if o.h.Len() == 0 {
		return nil
	}
	o.last = o.h.runs[0]
	return &o.last.rec
}

// err is the first error a run met.
func (o *sortedRecs[T]) err() error {
	if o.h == nil {
		return nil
	}
	return o.h.err()
}

// close lets go of the runs.
func (o *sortedRecs[T]) close() {
	if o.h != nil {
		o.h.close()
	}
	o.s.remove()
}

// runReader reads a run's records in order.
type runReader[T any] struct {
	f    *os.File
	r    *bufio.Reader
	get  func(*bufio.Reader, *T) error
	rec  T
	fail error
}

// next reads the next record, false at the end or on an error.
func (r *runReader[T]) next() bool {
	if err := r.get(r.r, &r.rec); err != nil {
		if !errors.Is(err, io.EOF) {
			r.fail = err
		}
		return false
	}
	return true
}

// runHeap merges runs by their next record.
type runHeap[T any] struct {
	runs []*runReader[T]
	cmp  func(a, b T) int
	done []*runReader[T]
}

func (h *runHeap[T]) Len() int           { return len(h.runs) }
func (h *runHeap[T]) Less(i, j int) bool { return h.cmp(h.runs[i].rec, h.runs[j].rec) < 0 }
func (h *runHeap[T]) Swap(i, j int)      { h.runs[i], h.runs[j] = h.runs[j], h.runs[i] }
func (h *runHeap[T]) Push(x any)         { h.runs = append(h.runs, x.(*runReader[T])) }
func (h *runHeap[T]) Pop() any {
	r := h.runs[len(h.runs)-1]
	h.runs = h.runs[:len(h.runs)-1]
	h.done = append(h.done, r)
	return r
}

// err is the first error a run met.
func (h *runHeap[T]) err() error {
	for _, r := range append(h.done, h.runs...) {
		if r.fail != nil {
			return r.fail
		}
	}
	return nil
}

func (h *runHeap[T]) close() {
	for _, r := range h.runs {
		r.f.Close()
	}
	h.runs = nil
}
