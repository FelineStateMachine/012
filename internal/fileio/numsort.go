package fileio

import (
	"bufio"
	"cmp"
	"encoding/binary"
	"io"
	"math"
)

// NumberSort sorts numbers in bounded memory, the external merge sort
// of a view's rows (extsort.go) over 16-byte records: each number and
// its position in the order it was read. The order statistics of a
// source's column (MEDIAN, PERCENTILE, MODE) read them back in
// ascending order, ties in that order, holding at most a run of them,
// sortRunBytes, while the rest wait on disk.
type NumberSort struct {
	s *extSort[numRec]
}

type numRec struct {
	v   float64
	pos int64
}

var numCodec = recCodec[numRec]{
	cmp: func(a, b numRec) int {
		switch {
		case a.v < b.v:
			return -1
		case a.v > b.v:
			return 1
		}
		return cmp.Compare(a.pos, b.pos)
	},
	size:  func(*numRec) int { return 16 },
	fixed: 16,
	put: func(w *bufio.Writer, r *numRec) {
		var b [16]byte
		binary.LittleEndian.PutUint64(b[:], math.Float64bits(r.v))
		binary.LittleEndian.PutUint64(b[8:], uint64(r.pos))
		w.Write(b[:])
	},
	get: func(r *bufio.Reader, rec *numRec) error {
		var b [16]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return err // io.EOF only where a record would start
		}
		rec.v = math.Float64frombits(binary.LittleEndian.Uint64(b[:]))
		rec.pos = int64(binary.LittleEndian.Uint64(b[8:]))
		return nil
	},
}

// NewNumberSort is a sort spilling its runs to files in dir, "" for the
// system's temporary directory.
func NewNumberSort(dir string) *NumberSort {
	s := newExtSort(dir, numCodec)
	s.limit = max(sortRunBytes/numRunShare, 16)
	return &NumberSort{s: s}
}

// numRunShare is the share of sortRunBytes a run of numbers holds: a
// million numbers, sorted faster in runs of their own and merged, and
// a few times that while the run grows and the collector catches up.
const numRunShare = 4

// Add takes a number read at position pos.
func (n *NumberSort) Add(v float64, pos int64) error { return n.s.add(numRec{v, pos}) }

// Len counts the numbers added.
func (n *NumberSort) Len() int { return int(n.s.n) }

// Close removes what the sort spilled, when Sorted wasn't asked for.
func (n *NumberSort) Close() { n.s.remove() }

// Sorted reads the numbers back in ascending order, once. Closing it
// removes the runs.
func (n *NumberSort) Sorted() (*SortedNumbers, error) {
	o, err := n.s.sorted()
	if err != nil {
		return nil, err
	}
	return &SortedNumbers{o: o, n: n.Len()}, nil
}

// SortedNumbers is a NumberSort's numbers in ascending order.
type SortedNumbers struct {
	o *sortedRecs[numRec]
	n int
}

// Len counts the numbers.
func (s *SortedNumbers) Len() int { return s.n }

// Next is the next number and its position as read; false at the end,
// or on an error, which Err says.
func (s *SortedNumbers) Next() (float64, int64, bool) {
	r := s.o.next()
	if r == nil {
		return 0, 0, false
	}
	return r.v, r.pos, true
}

// Err is the error reading the runs met, if any.
func (s *SortedNumbers) Err() error { return s.o.err() }

// Close removes the runs.
func (s *SortedNumbers) Close() error {
	s.o.close()
	return nil
}
