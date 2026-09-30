package fileio

import (
	"bufio"
	"cmp"
	"container/heap"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"slices"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A Parquet view's rows are a file of row numbers in the view's order,
// 8 bytes each, which a page reads with ReadAt: a filter writes the rows
// it lets through as it streams them, and a sort is an external merge
// sort, runs of at most sortRunBytes sorted in memory and spilled to
// files, then merged. Memory stays bounded whatever the row count; the
// disk holds the runs and the order, about 8 bytes a row plus the keys.

// sortRunBytes bounds the keys a sort holds in memory at once.
var sortRunBytes = 64 << 20

// sortVal is one key of a row as a sort compares it: its kind's rank
// (numbers, text, booleans, errors, then blanks), and its number or its
// text in lower case.
type sortVal struct {
	rank uint8
	num  float64
	str  string
}

// blankRank is a blank's, last in either direction.
const blankRank = 4

func sortValOf(v sheet.Value) sortVal {
	switch v.Kind {
	case sheet.Number:
		return sortVal{rank: 0, num: v.Num}
	case sheet.Text:
		return sortVal{rank: 1, str: strings.ToLower(v.Str)}
	case sheet.Bool:
		return sortVal{rank: 2, num: v.Num}
	case sheet.Error:
		return sortVal{rank: 3, str: strings.ToLower(v.Str)}
	}
	return sortVal{rank: blankRank}
}

// sortRec is a row being sorted: its number and its keys.
type sortRec struct {
	row  int64
	keys []sortVal
}

// size is about what the record holds in memory.
func (r *sortRec) size() int {
	n := 48 + 32*len(r.keys)
	for _, k := range r.keys {
		n += len(k.str)
	}
	return n
}

// compareRecs orders rows as a sheet's sort does (sheet.compareRows):
// key by key, blanks last in either direction, Z to A reversing the
// rest, and ties in the source's order.
func compareRecs(a, b *sortRec, desc []bool) int {
	for i := range a.keys {
		x, y := &a.keys[i], &b.keys[i]
		if (x.rank == blankRank) != (y.rank == blankRank) {
			if x.rank == blankRank {
				return 1
			}
			return -1
		}
		d := cmp.Compare(x.rank, y.rank)
		if d == 0 {
			switch x.rank {
			case 0, 2:
				d = cmp.Compare(x.num, y.num)
			case 1, 3:
				d = strings.Compare(x.str, y.str)
			}
		}
		if desc[i] {
			d = -d
		}
		if d != 0 {
			return d
		}
	}
	return cmp.Compare(a.row, b.row)
}

// extSorter sorts rows by their keys in bounded memory.
type extSorter struct {
	dir   string
	desc  []bool
	recs  []sortRec
	bytes int
	runs  []string
	limit int
}

func newExtSorter(dir string, desc []bool) *extSorter {
	return &extSorter{dir: dir, desc: desc, limit: sortRunBytes}
}

// add takes a row's keys, spilling a run when memory is full.
func (s *extSorter) add(row int64, keys []sheet.LiveCell) error {
	r := sortRec{row: row, keys: make([]sortVal, len(keys))}
	for i, k := range keys {
		r.keys[i] = sortValOf(k.V)
	}
	s.recs = append(s.recs, r)
	s.bytes += r.size()
	if s.bytes >= s.limit {
		return s.spill()
	}
	return nil
}

func (s *extSorter) sort() {
	slices.SortFunc(s.recs, func(a, b sortRec) int { return compareRecs(&a, &b, s.desc) })
}

// spill sorts the rows in memory and writes them as a run.
func (s *extSorter) spill() error {
	s.sort()
	f, err := os.CreateTemp(s.dir, "012-sort-*.run")
	if err != nil {
		return err
	}
	s.runs = append(s.runs, f.Name())
	w := bufio.NewWriterSize(f, 1<<20)
	for i := range s.recs {
		writeRec(w, &s.recs[i])
	}
	err = errors.Join(w.Flush(), f.Close())
	clear(s.recs)
	s.recs, s.bytes = s.recs[:0], 0
	return err
}

// finish writes the sorted rows' numbers to out and removes the runs.
func (s *extSorter) finish(out io.Writer) (n int64, err error) {
	defer s.remove()
	if len(s.runs) == 0 {
		s.sort()
		for i := range s.recs {
			if err := writeRow(out, s.recs[i].row); err != nil {
				return n, err
			}
			n++
		}
		return n, nil
	}
	if len(s.recs) > 0 {
		if err := s.spill(); err != nil {
			return 0, err
		}
	}
	return s.merge(out)
}

// remove deletes the runs.
func (s *extSorter) remove() {
	for _, name := range s.runs {
		os.Remove(name)
	}
	s.runs = nil
}

// merge merges the runs into out.
func (s *extSorter) merge(out io.Writer) (int64, error) {
	h := &runHeap{desc: s.desc}
	for _, name := range s.runs {
		f, err := os.Open(name)
		if err != nil {
			h.close()
			return 0, err
		}
		r := &runReader{f: f, r: bufio.NewReaderSize(f, 256<<10), keys: len(s.desc)}
		if r.next() {
			h.runs = append(h.runs, r)
		} else {
			f.Close()
		}
	}
	defer h.close()
	heap.Init(h)
	var n int64
	for h.Len() > 0 {
		r := h.runs[0]
		if err := writeRow(out, r.rec.row); err != nil {
			return n, err
		}
		n++
		if r.next() {
			heap.Fix(h, 0)
		} else {
			r.f.Close()
			heap.Pop(h)
		}
	}
	return n, h.err()
}

func writeRow(w io.Writer, row int64) error {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(row))
	_, err := w.Write(b[:])
	return err
}

// writeRec writes a record to a run: its row, then each key's rank and
// its number or text.
func writeRec(w *bufio.Writer, r *sortRec) {
	var b [binary.MaxVarintLen64]byte
	w.Write(b[:binary.PutUvarint(b[:], uint64(r.row))])
	for _, k := range r.keys {
		w.WriteByte(k.rank)
		switch k.rank {
		case 0, 2:
			binary.LittleEndian.PutUint64(b[:8], math.Float64bits(k.num))
			w.Write(b[:8])
		case 1, 3:
			w.Write(b[:binary.PutUvarint(b[:], uint64(len(k.str)))])
			w.WriteString(k.str)
		}
	}
}

// runReader reads a run's records in order.
type runReader struct {
	f    *os.File
	r    *bufio.Reader
	keys int
	rec  sortRec
	fail error
}

// next reads the next record, false at the end or on an error.
func (r *runReader) next() bool {
	row, err := binary.ReadUvarint(r.r)
	if err != nil {
		if !errors.Is(err, io.EOF) {
			r.fail = err
		}
		return false
	}
	r.rec.row = int64(row)
	r.rec.keys = r.rec.keys[:0]
	for range r.keys {
		k, err := r.key()
		if err != nil {
			r.fail = err
			return false
		}
		r.rec.keys = append(r.rec.keys, k)
	}
	return true
}

func (r *runReader) key() (sortVal, error) {
	rank, err := r.r.ReadByte()
	if err != nil {
		return sortVal{}, err
	}
	k := sortVal{rank: rank}
	switch rank {
	case 0, 2:
		var b [8]byte
		if _, err := io.ReadFull(r.r, b[:]); err != nil {
			return k, err
		}
		k.num = math.Float64frombits(binary.LittleEndian.Uint64(b[:]))
	case 1, 3:
		n, err := binary.ReadUvarint(r.r)
		if err != nil {
			return k, err
		}
		b := make([]byte, n)
		if _, err := io.ReadFull(r.r, b); err != nil {
			return k, err
		}
		k.str = string(b)
	}
	return k, nil
}

// runHeap merges runs by their next record.
type runHeap struct {
	runs []*runReader
	desc []bool
	done []*runReader
}

func (h *runHeap) Len() int { return len(h.runs) }
func (h *runHeap) Less(i, j int) bool {
	return compareRecs(&h.runs[i].rec, &h.runs[j].rec, h.desc) < 0
}
func (h *runHeap) Swap(i, j int) { h.runs[i], h.runs[j] = h.runs[j], h.runs[i] }
func (h *runHeap) Push(x any)    { h.runs = append(h.runs, x.(*runReader)) }
func (h *runHeap) Pop() any {
	r := h.runs[len(h.runs)-1]
	h.runs = h.runs[:len(h.runs)-1]
	h.done = append(h.done, r)
	return r
}

// err is the first error a run met.
func (h *runHeap) err() error {
	for _, r := range append(h.done, h.runs...) {
		if r.fail != nil {
			return r.fail
		}
	}
	return nil
}

func (h *runHeap) close() {
	for _, r := range h.runs {
		r.f.Close()
	}
}

// rowFile is a view's rows as a file of row numbers in its order.
type rowFile struct {
	f *os.File
	n int64
}

// newRowFile makes the file rows are written to, in dir.
func newRowFile(dir string) (*os.File, error) { return os.CreateTemp(dir, "012-view-*.rows") }

// read returns the row numbers at positions from to from+n, as many as
// there are.
func (rf *rowFile) read(from int64, n int) ([]int64, error) {
	if from < 0 || from >= rf.n || n <= 0 {
		return nil, nil
	}
	n = int(min(int64(n), rf.n-from))
	b := make([]byte, 8*n)
	if _, err := rf.f.ReadAt(b, 8*from); err != nil {
		return nil, err
	}
	out := make([]int64, n)
	for i := range out {
		out[i] = int64(binary.LittleEndian.Uint64(b[8*i:]))
	}
	return out, nil
}

// close closes and removes the file.
func (rf *rowFile) close() error {
	name := rf.f.Name()
	return errors.Join(rf.f.Close(), os.Remove(name))
}
